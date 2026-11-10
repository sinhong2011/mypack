package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/pkg/config"
)

// AppInfo is public metadata of an existing app. Used for read-only import.
type AppInfo struct {
	Name, Identifier, Version, SourceURL string
	IconPNG                              []byte
	Managed                              bool // has mypack.json produced by mypack
}

// ReadAppInfo reads public metadata of an existing app (.app Info.plist,
// portable folder resources/mypack.json, or an .exe name) without executing
// it.
func ReadAppInfo(path string) (AppInfo, error) {
	st, err := os.Stat(path)
	if err != nil {
		return AppInfo{}, derr("NOT_FOUND", "The app was not found", err, "Check the path.")
	}
	if st.IsDir() && (isApp(path) || fileExists(filepath.Join(path, "Contents", "Info.plist"))) {
		return readBundle(path)
	}
	if st.IsDir() {
		return readPortable(path)
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return AppInfo{Name: name}, nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func readBundle(app string) (AppInfo, error) {
	info := AppInfo{Name: strings.TrimSuffix(filepath.Base(app), ".app")}
	raw, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		return info, derr("NOT_FOUND", "The app has no Info.plist", err, "This does not look like a macOS app.")
	}
	pl, err := ParsePlist(raw)
	if err != nil {
		return info, derr("INVALID_CONFIG", "The app's Info.plist cannot be read", err, "")
	}
	str := func(k string) string { s, _ := pl[k].(string); return s }
	if n := firstNonEmpty(str("CFBundleDisplayName"), str("CFBundleName")); n != "" {
		info.Name = n
	}
	info.Identifier = str("CFBundleIdentifier")
	info.Version = firstNonEmpty(str("CFBundleShortVersionString"), str("CFBundleVersion"))

	res := filepath.Join(app, "Contents", "Resources")
	applyManaged(&info, filepath.Join(res, "mypack.json"))
	// icon
	if icon := str("CFBundleIconFile"); icon != "" {
		if filepath.Ext(icon) == "" {
			icon += ".icns"
		}
		if b, err := os.ReadFile(filepath.Join(res, filepath.Base(icon))); err == nil {
			if p, err := icons.ICNSLargestPNG(b); err == nil {
				info.IconPNG = p
			}
		}
	}
	if info.IconPNG == nil {
		if b, err := os.ReadFile(filepath.Join(res, "icon.png")); err == nil && info.Managed {
			info.IconPNG = b
		}
	}
	return info, nil
}

func readPortable(dir string) (AppInfo, error) {
	info := AppInfo{Name: filepath.Base(dir)}
	res := filepath.Join(dir, "resources")
	applyManaged(&info, filepath.Join(res, "mypack.json"))
	if b, err := os.ReadFile(filepath.Join(res, "icon.png")); err == nil {
		info.IconPNG = b
	}
	if exe, err := findExecutable(dir); err == nil && !info.Managed {
		info.Name = strings.TrimSuffix(filepath.Base(exe), ".exe")
	}
	return info, nil
}

// applyManaged fills info from mypack.json when present.
func applyManaged(info *AppInfo, path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var c *config.Config
	if parsed, err := config.Parse(b); err == nil {
		c = parsed
	} else {
		// lenient: a newer/older mypack may have fields we do not know
		var l config.Config
		if json.Unmarshal(b, &l) != nil || l.App.Identifier == "" {
			return
		}
		c = &l
	}
	info.Managed = true
	if c.App.Name != "" {
		info.Name = c.App.Name
	}
	if info.Identifier == "" {
		info.Identifier = c.App.Identifier
	}
	if info.Version == "" {
		info.Version = c.App.Version
	}
	if c.App.Source.Type == config.SourceURL {
		info.SourceURL = c.App.Source.Value
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
