package builder

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mypack.dev/mypack/internal/icons"
)

// desktopTemplate is the portable .desktop file. __INSTALL_DIR__ is replaced
// by the installer (internal/installer) with the real install directory.
func (j *job) desktopFile(exec, icon string) []byte {
	return []byte(fmt.Sprintf(`[Desktop Entry]
Type=Application
Version=1.0
Name=%s
Exec=%s
Icon=%s
Terminal=false
Categories=Network;
StartupWMClass=%s
`, j.cfg.App.Name, exec, icon, j.cfg.App.Name))
}

func (j *job) customizeLinux() error {
	return os.WriteFile(filepath.Join(j.root, j.slug+".desktop"),
		j.desktopFile("__INSTALL_DIR__/"+j.slug, "__INSTALL_DIR__/resources/icon.png"), 0o644)
}

// debPkg is the Debian package name (lowercase, ≥2 chars).
func (j *job) debPkg() string {
	p := j.slug
	if len(p) < 2 {
		p = "app-" + p
	}
	return p
}

func (j *job) debFileName() string {
	return fmt.Sprintf("%s_%s_%s.deb", j.debPkg(), j.cfg.App.Version, j.target.Arch)
}

type tarBuilder struct {
	tw   *tar.Writer
	zw   *gzip.Writer
	buf  *bytes.Buffer
	now  time.Time
	dirs map[string]bool
}

func newTarBuilder(now time.Time) *tarBuilder {
	buf := &bytes.Buffer{}
	zw, _ := gzip.NewWriterLevel(buf, gzip.BestCompression)
	return &tarBuilder{tw: tar.NewWriter(zw), zw: zw, buf: buf, now: now, dirs: map[string]bool{}}
}

func (t *tarBuilder) mkdirs(name string) error {
	// name like "./opt/x/y" (file path); creates parents
	parts := strings.Split(strings.TrimPrefix(name, "./"), "/")
	cur := "."
	if !t.dirs["./"] {
		t.dirs["./"] = true
		if err := t.tw.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: t.now, Uname: "root", Gname: "root", Format: tar.FormatGNU}); err != nil {
			return err
		}
	}
	for _, p := range parts[:len(parts)-1] {
		cur += "/" + p
		if t.dirs[cur+"/"] {
			continue
		}
		t.dirs[cur+"/"] = true
		if err := t.tw.WriteHeader(&tar.Header{Name: cur + "/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: t.now, Uname: "root", Gname: "root", Format: tar.FormatGNU}); err != nil {
			return err
		}
	}
	return nil
}

func (t *tarBuilder) file(name string, data []byte, mode int64) error {
	if err := t.mkdirs(name); err != nil {
		return err
	}
	if err := t.tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: mode, Size: int64(len(data)), ModTime: t.now, Uname: "root", Gname: "root", Format: tar.FormatGNU}); err != nil {
		return err
	}
	_, err := t.tw.Write(data)
	return err
}

func (t *tarBuilder) symlink(name, target string) error {
	if err := t.mkdirs(name); err != nil {
		return err
	}
	return t.tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeSymlink, Linkname: target, Mode: 0o777, ModTime: t.now, Uname: "root", Gname: "root", Format: tar.FormatGNU})
}

func (t *tarBuilder) finish() ([]byte, error) {
	if err := t.tw.Close(); err != nil {
		return nil, err
	}
	if err := t.zw.Close(); err != nil {
		return nil, err
	}
	return t.buf.Bytes(), nil
}

func (j *job) packDeb(out string) error {
	pkg := j.debPkg()
	now := time.Now().UTC().Truncate(time.Second)
	opt := "opt/" + pkg
	data := newTarBuilder(now)
	md5s := map[string]string{}
	var installed int64
	addFile := func(path string, b []byte, mode int64) error {
		sum := md5.Sum(b)
		md5s[strings.TrimPrefix(path, "./")] = hex.EncodeToString(sum[:])
		installed += int64(len(b))
		return data.file(path, b, mode)
	}
	// the app tree → /opt/<pkg>
	var rels []string
	_ = filepath.WalkDir(j.root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(j.root, p)
			rels = append(rels, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(rels)
	for _, rel := range rels {
		if rel == j.slug+".desktop" {
			continue
		}
		if err := j.ctx.Err(); err != nil {
			return err
		}
		b, err := os.ReadFile(filepath.Join(j.root, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		mode := int64(0o644)
		if rel == j.slug {
			mode = 0o755
		}
		if err := addFile("./"+opt+"/"+rel, b, mode); err != nil {
			return err
		}
	}
	if err := data.symlink("./usr/bin/"+pkg, "/"+opt+"/"+j.slug); err != nil {
		return err
	}
	if err := addFile("./usr/share/applications/"+pkg+".desktop", j.desktopFile("/"+opt+"/"+j.slug, pkg), 0o644); err != nil {
		return err
	}
	icon, err := icons.Resize(j.iconPNG, 512)
	if err != nil {
		return err
	}
	if err := addFile("./usr/share/icons/hicolor/512x512/apps/"+pkg+".png", icon, 0o644); err != nil {
		return err
	}
	dataTGZ, err := data.finish()
	if err != nil {
		return err
	}

	// control
	var control strings.Builder
	fmt.Fprintf(&control, "Package: %s\nVersion: %s\nArchitecture: %s\nMaintainer: %s <noreply@mypack.dev>\nInstalled-Size: %d\n",
		pkg, j.cfg.App.Version, j.target.Arch, j.cfg.App.Name, (installed+1023)/1024)
	control.WriteString("Depends: libwebkit2gtk-4.1-0 | libwebkit2gtk-4.0-37, libgtk-3-0\nSection: web\nPriority: optional\n")
	fmt.Fprintf(&control, "Description: %s\n Desktop app made with mypack.\n", j.cfg.App.Name)
	var mdsum strings.Builder
	keys := make([]string, 0, len(md5s))
	for k := range md5s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&mdsum, "%s  %s\n", md5s[k], k)
	}
	ctl := newTarBuilder(now)
	if err := ctl.file("./control", []byte(control.String()), 0o644); err != nil {
		return err
	}
	if err := ctl.file("./md5sums", []byte(mdsum.String()), 0o644); err != nil {
		return err
	}
	ctlTGZ, err := ctl.finish()
	if err != nil {
		return err
	}

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := writeAr(f, now, [][2]any{{"debian-binary", []byte("2.0\n")}, {"control.tar.gz", ctlTGZ}, {"data.tar.gz", dataTGZ}}); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// writeAr writes a Unix ar archive (the container format of .deb files).
func writeAr(w io.Writer, mtime time.Time, members [][2]any) error {
	if _, err := io.WriteString(w, "!<arch>\n"); err != nil {
		return err
	}
	for _, m := range members {
		name, data := m[0].(string), m[1].([]byte)
		hdr := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8s%-10d`\n", name, mtime.Unix(), 0, 0, "100644", len(data))
		if len(hdr) != 60 {
			return fmt.Errorf("bad ar header for %s", name)
		}
		if _, err := io.WriteString(w, hdr); err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		if len(data)%2 == 1 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
	}
	return nil
}
