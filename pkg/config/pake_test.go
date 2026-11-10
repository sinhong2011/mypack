package config

import (
	"strings"
	"testing"
)

func TestImportPakeJSON(t *testing.T) {
	in := `{
	  "windows": [{
	    "url": "weread.qq.com", "url_type": "web", "hide_title_bar": true,
	    "fullscreen": true, "width": 1000, "height": 700, "resizable": false,
	    "always_on_top": true, "dark_mode": true,
	    "activation_shortcut": "CmdOrControl+Shift+R", "disabled_web_shortcuts": true,
	    "hide_on_close": true, "incognito": true, "title": "WeRead Window",
	    "enable_wasm": true, "enable_drag_drop": true, "start_to_tray": true,
	    "force_internal_navigation": true, "internal_url_regex": "^https://x", "zoom": 120,
	    "min_width": 300, "min_height": 250, "ignore_certificate_errors": true, "new_window": true,
	    "mystery": 1
	  }],
	  "user_agent": {"macos": "UA-MAC", "linux": "UA-LIN", "windows": "UA-WIN"},
	  "system_tray": {"macos": true, "linux": true, "windows": true},
	  "system_tray_path": "icons/icon.png",
	  "proxy_url": "http://127.0.0.1:7890",
	  "inject": ["a.css", "b.js", "c.txt"],
	  "multi_instance": true, "multi_window": true
	}`
	cfg, warns, err := ImportPake([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App.Source.Type != SourceURL || cfg.App.Source.Value != "https://weread.qq.com/" {
		t.Errorf("source = %+v", cfg.App.Source)
	}
	w := cfg.Window
	if w.Width != 1000 || w.Height != 700 || !w.Fullscreen || !w.AlwaysOnTop || w.TitleBarStyle != "overlay" ||
		w.DarkMode != "dark" || !w.HideOnClose || w.Zoom != 120 || w.MinWidth != 300 || !w.MultiInstance || !w.MultiWindow {
		t.Errorf("window = %+v", w)
	}
	if cfg.App.Name != "WeRead Window" {
		t.Errorf("name = %q", cfg.App.Name)
	}
	if cfg.Native.GlobalShortcut != "CmdOrCtrl+Shift+R" || !cfg.Native.StartToTray || !cfg.Native.Tray || !cfg.Native.DragDrop {
		t.Errorf("native = %+v", cfg.Native)
	}
	if !cfg.WebView.Incognito || !cfg.WebView.WASM || !cfg.WebView.IgnoreCertErrs ||
		cfg.WebView.Navigation.InternalURLRegex != "^https://x" ||
		cfg.WebView.Navigation.ExternalByDefault == nil || *cfg.WebView.Navigation.ExternalByDefault ||
		cfg.WebView.Navigation.Popups != "internal" {
		t.Errorf("webview = %+v", cfg.WebView)
	}
	if cfg.Network.Proxy != "http://127.0.0.1:7890" {
		t.Errorf("proxy = %q", cfg.Network.Proxy)
	}
	if len(cfg.WebView.Injections) != 2 || cfg.WebView.Injections[1].Type != "js" {
		t.Errorf("injections = %+v", cfg.WebView.Injections)
	}
	if cfg.WebView.UserAgent == "" {
		t.Errorf("user agent not imported")
	}
	joined := strings.Join(warns, "\n")
	for _, want := range []string{"resizable", "disabled_web_shortcuts", "system_tray_path", "mystery", "c.txt", "user_agent."} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings missing %q:\n%s", want, joined)
		}
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("imported config invalid: %v", err)
	}
}

func TestImportPakeFlat(t *testing.T) {
	in := `{"name":"Twitter","url":"https://twitter.com","width":900,"hideTitleBar":true,
	"alwaysOnTop":true,"userAgent":"X","showSystemTray":true,"safeDomain":"a.com, b.com",
	"proxyUrl":"socks5://127.0.0.1:1080","inject":"x.css","targets":"dmg","weird":true}`
	cfg, warns, err := ImportPake([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App.Name != "Twitter" || cfg.Window.Width != 900 || cfg.Window.TitleBarStyle != "overlay" ||
		!cfg.Window.AlwaysOnTop || cfg.WebView.UserAgent != "X" || !cfg.Native.Tray ||
		len(cfg.WebView.Navigation.SafeDomains) != 2 || cfg.Network.Proxy != "socks5://127.0.0.1:1080" ||
		len(cfg.WebView.Injections) != 1 {
		t.Errorf("cfg = %+v", cfg)
	}
	if len(warns) != 2 {
		t.Errorf("warnings = %v", warns)
	}
	if !IsPakeJSON([]byte(in)) {
		t.Error("IsPakeJSON false")
	}
}

func TestImportPakeErrors(t *testing.T) {
	if _, _, err := ImportPake([]byte(`{`)); err == nil {
		t.Error("want JSON error")
	}
	if _, _, err := ImportPake([]byte(`{"width":100}`)); err == nil {
		t.Error("want missing url error")
	}
	if IsPakeJSON([]byte(`{"schemaVersion":1,"app":{}}`)) {
		t.Error("mypack.json misdetected as Pake")
	}
}
