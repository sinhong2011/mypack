package builder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const maxSiteBytes = 500 << 20

func is1024PNG(b []byte) bool {
	cfg, err := png.DecodeConfig(bytes.NewReader(b))
	return err == nil && cfg.Width == 1024 && cfg.Height == 1024
}

// copyFile copies a regular file, creating parents; cancellable.
func copyFile(ctx context.Context, src, dst string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, &ctxReader{ctx, in}); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// copyTree copies a directory recursively preserving symlinks and modes.
func copyTree(ctx context.Context, src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode()&fs.ModeSymlink != 0:
			l, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(l, target)
		case info.Mode().IsRegular():
			return copyFile(ctx, p, target, info.Mode().Perm())
		}
		return nil // skip sockets, devices…
	})
}

// copyTreeNoLinks copies a user-supplied directory, skipping symlinks (they
// could point outside the project) and returning the skipped count.
func copyTreeNoLinks(ctx context.Context, src, dst string) (skipped int, err error) {
	var total int64
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.Type()&fs.ModeSymlink != 0 {
			skipped++
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); rel != "." && (n == ".git" || n == "node_modules") {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if total += info.Size(); total > maxSiteBytes {
			return fmt.Errorf("the website folder is larger than %d MB", maxSiteBytes>>20)
		}
		mode := fs.FileMode(0o644)
		if info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		return copyFile(ctx, p, target, mode)
	})
	return
}

// hashArtifact returns the sha256 and total size of an artifact. For a
// directory (bundle) the hash covers a deterministic manifest of relative
// paths, kinds, executable bits and per-file sha256 values.
func hashArtifact(ctx context.Context, path string) (string, int64, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return "", 0, err
	}
	if !st.IsDir() {
		f, err := os.Open(path)
		if err != nil {
			return "", 0, err
		}
		defer f.Close()
		h := sha256.New()
		n, err := io.Copy(h, &ctxReader{ctx, f})
		return hex.EncodeToString(h.Sum(nil)), n, err
	}
	type ent struct{ rel, line string }
	var ents []ent
	var total int64
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if p == path {
			return nil
		}
		rel, _ := filepath.Rel(path, p)
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			ents = append(ents, ent{rel, "d " + rel})
		case info.Mode()&fs.ModeSymlink != 0:
			l, _ := os.Readlink(p)
			ents = append(ents, ent{rel, "l " + rel + " -> " + l})
		case info.Mode().IsRegular():
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			h := sha256.New()
			n, err := io.Copy(h, &ctxReader{ctx, f})
			f.Close()
			if err != nil {
				return err
			}
			total += n
			x := "-"
			if info.Mode()&0o111 != 0 {
				x = "x"
			}
			ents = append(ents, ent{rel, fmt.Sprintf("f %s %s %s", rel, x, hex.EncodeToString(h.Sum(nil)))})
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].rel < ents[j].rel })
	h := sha256.New()
	for _, e := range ents {
		h.Write([]byte(e.line))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil)), total, nil
}

// run executes a command, logging its output; cancellable.
func (j *job) run(name string, args ...string) error {
	cmd := exec.CommandContext(j.ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	if s := strings.TrimSpace(out.String()); s != "" {
		lvl := "debug"
		if err != nil {
			lvl = "error"
		}
		j.log(lvl, "%s: %s", name, s)
	}
	if err != nil {
		if j.ctx.Err() != nil {
			return j.ctx.Err()
		}
		msg := strings.TrimSpace(out.String())
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		if msg != "" {
			return fmt.Errorf("%s failed: %w: %s", name, err, msg)
		}
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func have(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

var errUnsupported = errors.New("unsupported")
