package builder

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"

	"mypack.dev/mypack/internal/icons"
)

func (j *job) customizeWindows() error {
	ico, err := icons.ICO(j.iconPNG)
	if err != nil {
		return fmt.Errorf("ico: %w", err)
	}
	if err := os.WriteFile(filepath.Join(j.resDir, "icon.ico"), ico, 0o644); err != nil {
		return err
	}
	if err := j.patchPE(j.exePath, ico); err != nil {
		j.warn("Could not embed the icon and version info into the .exe (%v); the file icon will be the default one", err)
	}
	return nil
}

// versionParts turns "1.2.3" into 1.2.3.0.
func versionParts(v string) string {
	parts := strings.Split(v, ".")
	for len(parts) < 4 {
		parts = append(parts, "0")
	}
	for i, p := range parts[:4] {
		if _, err := strconv.Atoi(p); err != nil {
			parts[i] = "0"
		}
	}
	return strings.Join(parts[:4], ".")
}

// patchPE embeds the icon and version information into the exe (pure Go,
// github.com/tc-hib/winres). The original file is replaced only on success.
func (j *job) patchPE(exe string, ico []byte) error {
	src, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	rs, err := winres.LoadFromEXE(bytes.NewReader(src))
	if err != nil {
		rs = &winres.ResourceSet{}
	}
	icon, err := winres.LoadICO(bytes.NewReader(ico))
	if err != nil {
		return err
	}
	if err := rs.SetIcon(winres.ID(1), icon); err != nil {
		return err
	}
	vi := version.Info{}
	vi.SetFileVersion(versionParts(j.cfg.App.Version))
	vi.SetProductVersion(versionParts(j.cfg.App.Version))
	for k, v := range map[string]string{
		version.ProductName:      j.cfg.App.Name,
		version.FileDescription:  j.cfg.App.Name,
		version.InternalName:     j.name,
		version.OriginalFilename: j.name + ".exe",
		version.FileVersion:      j.cfg.App.Version,
		version.ProductVersion:   j.cfg.App.Version,
		version.Comments:         "Made with mypack",
	} {
		if err := vi.Set(version.LangDefault, k, v); err != nil {
			return err
		}
	}
	rs.SetVersionInfo(vi)
	var out bytes.Buffer
	if err := rs.WriteToEXE(&out, bytes.NewReader(src)); err != nil {
		return err
	}
	tmp := exe + ".tmp"
	if err := os.WriteFile(tmp, out.Bytes(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, exe)
}
