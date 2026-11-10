package runtimemanager

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// MaxExtractedSize caps the total size of an extracted runtime archive.
const MaxExtractedSize = 200 << 20

var maxExtract int64 = MaxExtractedSize

// ExtractTarGz safely extracts a .tar.gz archive into dest (which must exist
// and be empty/new). It rejects absolute paths, ".." elements, symlinks,
// hard links, device/FIFO entries and archives that expand beyond
// MaxExtractedSize. Executable bits are normalized (0755 / 0644).
func ExtractTarGz(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("not a gzip archive: %w", err)
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	var total int64
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("corrupt tar archive: %w", err)
		}
		name := h.Name
		if err := checkEntryName(name); err != nil {
			return err
		}
		clean := path.Clean(strings.TrimPrefix(name, "./"))
		if clean == "." {
			continue
		}
		target := filepath.Join(destAbs, filepath.FromSlash(clean))
		if rel, err := filepath.Rel(destAbs, target); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("archive entry escapes destination: %q", name)
		}
		switch h.Typeflag {
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if h.Size < 0 || total+h.Size > maxExtract {
				return fmt.Errorf("archive expands beyond the %d MB limit", maxExtract>>20)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return err
			}
			// Read at most the declared size; never trust the stream for more.
			n, err := io.Copy(out, io.LimitReader(tr, h.Size))
			cerr := out.Close()
			if err != nil {
				return err
			}
			if cerr != nil {
				return cerr
			}
			if n != h.Size {
				return errors.New("truncated archive entry " + name)
			}
			total += n
			_ = os.Chmod(target, mode)
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("archive contains a link (%q), which is not allowed", name)
		default:
			return fmt.Errorf("archive contains a special file (%q type %q), which is not allowed", name, string(h.Typeflag))
		}
	}
}

func checkEntryName(name string) error {
	if name == "" {
		return errors.New("archive entry with empty name")
	}
	if strings.ContainsRune(name, 0) || strings.Contains(name, `\`) {
		return fmt.Errorf("archive entry has an invalid name: %q", name)
	}
	if strings.HasPrefix(name, "/") || (len(name) >= 2 && name[1] == ':') {
		return fmt.Errorf("archive entry has an absolute path: %q", name)
	}
	for _, el := range strings.Split(name, "/") {
		if el == ".." {
			return fmt.Errorf("archive entry contains '..': %q", name)
		}
	}
	return nil
}
