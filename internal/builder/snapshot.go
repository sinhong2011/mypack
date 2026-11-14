package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mypack.dev/mypack/pkg/config"
)

const maxInjectionBytes = 2 << 20

// writeResources writes mypack.json (injections inlined), icon.png and, when
// needed, bundled local site files / tray icon into the resources directory.
func (j *job) writeResources() error {
	snap := j.cfg.Clone()

	// Inline injection files: the runtime never reads paths.
	for i := range snap.WebView.Injections {
		inj := &snap.WebView.Injections[i]
		if inj.Path == "" {
			continue
		}
		b, err := os.ReadFile(inj.Path)
		if err != nil {
			return &domainErr{code: "INVALID_CONFIG", field: fmt.Sprintf("webview.injections[%d].path", i),
				msg: "Cannot read the injection file " + inj.Path, cause: err, rem: "Check that the file exists and is readable."}
		}
		if len(b) > maxInjectionBytes {
			return &domainErr{code: "INVALID_CONFIG", field: fmt.Sprintf("webview.injections[%d].path", i),
				msg: "Injection file is larger than 2 MB: " + inj.Path, rem: "Use a smaller file."}
		}
		inj.Content = string(b)
		inj.Path = ""
	}
	// The icon ships as icon.png/AppIcon.icns (and Assets.car); the runtime
	// never reads app.icon, but it validates it, and a "file" icon needs a path.
	if snap.App.Icon.Source == "file" {
		snap.App.Icon.Source = "default"
	}
	snap.App.Icon.Path = ""

	// Local sources are bundled next to the config; paths become relative to the resources dir.
	source := snap.App.Source.Type
	if snap.IsGroup() {
		source = "" // a group's members are bundled below; its Source is not opened
	}
	switch source {
	case config.SourceDir:
		dst := filepath.Join(j.resDir, "site")
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		skipped, err := copyTreeNoLinks(j.ctx, snap.App.Source.Value, dst)
		if err != nil {
			return &domainErr{code: "INVALID_CONFIG", field: "app.source.value", msg: "Cannot copy the website folder", cause: err, rem: "Check that the folder exists and is readable."}
		}
		if skipped > 0 {
			j.warn("%d symbolic link(s) in the website folder were not included", skipped)
		}
		snap.App.Source.Value = "site"
	case config.SourceFile:
		// copy the file's parent dir so relative assets keep working
		base := filepath.Base(snap.App.Source.Value)
		dst := filepath.Join(j.resDir, "site")
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		if _, err := copyTreeNoLinks(j.ctx, filepath.Dir(snap.App.Source.Value), dst); err != nil {
			return &domainErr{code: "INVALID_CONFIG", field: "app.source.value", msg: "Cannot copy the HTML file's folder", cause: err, rem: "Check that the file exists and its folder is readable and not huge."}
		}
		snap.App.Source.Value = "site/" + base
	}
	// Group members' icons ship as members/<id>.png next to the config.
	if snap.App.Group != nil {
		for i := range snap.App.Group.Members {
			m := &snap.App.Group.Members[i]
			if m.Icon == "" || strings.HasPrefix(m.Icon, "members/") {
				continue
			}
			rel := "members/" + m.ID + ".png"
			if err := os.MkdirAll(filepath.Join(j.resDir, "members"), 0o755); err != nil {
				return err
			}
			if err := copyFile(j.ctx, m.Icon, filepath.Join(j.resDir, filepath.FromSlash(rel)), 0o644); err != nil {
				j.warn("The icon of %q could not be read; its initial will be shown", m.Name)
				m.Icon = ""
				continue
			}
			m.Icon = rel
		}
		// Local members: their folder (or the HTML file's folder) ships as
		// members/<id>/site, served by the runtime at mypack://<id>/.
		for i := range snap.App.Group.Members {
			m := &snap.App.Group.Members[i]
			if !m.IsLocal() || strings.HasPrefix(m.Local, "members/") {
				continue
			}
			st, err := os.Stat(m.Local)
			if err != nil {
				return &domainErr{code: "INVALID_CONFIG", field: fmt.Sprintf("app.group.members[%d].local", i),
					msg: "Cannot find the folder of " + m.Name, cause: err, rem: "Check that it still exists, or remove it from the group."}
			}
			src, entry := m.Local, m.Entry
			if !st.IsDir() {
				src, entry = filepath.Dir(m.Local), filepath.Base(m.Local)
			}
			rel := "members/" + m.ID + "/site"
			dst := filepath.Join(j.resDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			skipped, err := copyTreeNoLinks(j.ctx, src, dst)
			if err != nil {
				return &domainErr{code: "INVALID_CONFIG", field: fmt.Sprintf("app.group.members[%d].local", i),
					msg: "Cannot copy the folder of " + m.Name, cause: err, rem: "Check that the folder is readable and not huge."}
			}
			if skipped > 0 {
				j.warn("%d symbolic link(s) in the folder of %q were not included", skipped, m.Name)
			}
			m.Local, m.Entry = rel, entry
		}
		// With no website member, Source names the first local one (bundled
		// above), so the snapshot never points at the build machine.
		if snap.App.Source.Type != config.SourceURL {
			for _, m := range snap.App.Group.Members {
				if m.IsLocal() {
					snap.App.Source = config.Source{Type: config.SourceDir, Value: m.Local, Entry: m.Entry}
					break
				}
			}
		}
	}
	if p := snap.Native.TrayIcon; p != "" && !strings.HasPrefix(p, "resources/") {
		if err := copyFile(j.ctx, p, filepath.Join(j.resDir, "tray.png"), 0o644); err != nil {
			j.warn("The tray icon %q could not be read; the app icon will be used", p)
			snap.Native.TrayIcon = ""
		} else {
			snap.Native.TrayIcon = "tray.png"
		}
	}

	b, err := snap.Marshal()
	if err != nil {
		return err
	}
	if _, err := config.Parse(b); err != nil {
		return &domainErr{code: "INVALID_CONFIG", msg: "The generated configuration does not parse", cause: err, rem: "Report this as a bug."}
	}
	if err := os.MkdirAll(j.resDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(j.resDir, "mypack.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(j.resDir, "icon.png"), j.iconPNG, 0o644)
}
