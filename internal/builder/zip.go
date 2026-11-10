package builder

import (
	"archive/zip"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// zipTree zips directory src (including the directory itself as the top-level
// entry), preserving symlinks and executable bits. Pure Go fallback for ditto.
func zipTree(ctx context.Context, src, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	top := filepath.Base(src)
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		name := top
		if rel != "." {
			name = top + "/" + filepath.ToSlash(rel)
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = name
		switch {
		case d.IsDir():
			h.Name += "/"
			h.Method = zip.Store
			_, err = zw.CreateHeader(h)
			return err
		case info.Mode()&fs.ModeSymlink != 0:
			l, err := os.Readlink(p)
			if err != nil {
				return err
			}
			h.Method = zip.Store
			w, err := zw.CreateHeader(h)
			if err != nil {
				return err
			}
			_, err = w.Write([]byte(l))
			return err
		case info.Mode().IsRegular():
			h.Method = zip.Deflate
			w, err := zw.CreateHeader(h)
			if err != nil {
				return err
			}
			in, err := os.Open(p)
			if err != nil {
				return err
			}
			defer in.Close()
			_, err = io.Copy(w, &ctxReader{ctx, in})
			return err
		}
		return nil
	})
	if err != nil {
		zw.Close()
		f.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
