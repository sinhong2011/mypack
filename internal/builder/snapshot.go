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
	snap.App.Icon.Path = "" // the icon ships as icon.png/AppIcon.icns

	// Local sources are bundled next to the config; paths become relative to the resources dir.
	switch snap.App.Source.Type {
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
