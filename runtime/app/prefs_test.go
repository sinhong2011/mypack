package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mypack.dev/mypack/pkg/config"
)

func TestApplyUserPrefsKeepsIdentity(t *testing.T) {
	cfg := samplePrefsConfig()
	raw := []byte(`{
		"app":{"name":"Hacked","title":"My title","identifier":"evil.id","source":{"type":"url","value":"https://evil.example"}},
		"runtime":{"version":"0.0.1"},
		"window":{"alwaysOnTop":true,"width":1400},
		"network":{"downloadDirectory":"~/Downloads"},
		"build":{"targets":[{"os":"linux","arch":"amd64","format":"zip"}]}
	}`)
	if err := applyUserPrefs(cfg, raw); err != nil {
		t.Fatal(err)
	}
	if cfg.App.Name != "Mail" || cfg.App.Identifier != "app.mypack.mail" || cfg.App.Source.Value != "https://mail.example" {
		t.Fatalf("identity mutated: %+v", cfg.App)
	}
	if cfg.Runtime.Version != "1.0.10" {
		t.Fatalf("runtime mutated: %s", cfg.Runtime.Version)
	}
	if cfg.App.Title != "My title" || !cfg.Window.AlwaysOnTop || cfg.Window.Width != 1400 {
		t.Fatalf("overlay not applied: title=%q aot=%v w=%d", cfg.App.Title, cfg.Window.AlwaysOnTop, cfg.Window.Width)
	}
	if cfg.Network.DownloadDirectory != "~/Downloads" {
		t.Fatalf("downloads: %q", cfg.Network.DownloadDirectory)
	}
	if cfg.Window.Height != 780 {
		t.Fatalf("unmentioned height must stay: %d", cfg.Window.Height)
	}
}

func TestPrefsOverlayRoundTrip(t *testing.T) {
	bundled := samplePrefsConfig()
	cur := bundled.Clone()
	cur.App.Title = "Work"
	cur.Window.AlwaysOnTop = true
	cur.Window.Zoom = 125
	cur.Network.DownloadDirectory = "~/Downloads"
	cur.Native.Notifications = "ask"
	off := false
	cur.Native.LinkRouting = &off
	cur.WebView.Navigation.Popups = "external"
	cur.WebView.Navigation.SafeDomains = []string{"mail.example", "accounts.example"}

	overlay, err := prefsOverlay(bundled, cur)
	if err != nil || overlay == nil {
		t.Fatalf("overlay: %s %v", overlay, err)
	}
	var got map[string]any
	if err := json.Unmarshal(overlay, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["runtime"]; ok {
		t.Fatalf("runtime must not be in overlay: %s", overlay)
	}
	if app, _ := got["app"].(map[string]any); app["name"] != nil {
		t.Fatalf("name must not be in overlay: %s", overlay)
	}

	restored := bundled.Clone()
	if err := applyUserPrefs(restored, overlay); err != nil {
		t.Fatal(err)
	}
	if restored.App.Title != "Work" || !restored.Window.AlwaysOnTop || restored.Window.Zoom != 125 {
		t.Fatalf("round trip window: %+v", restored.Window)
	}
	if restored.Network.DownloadDirectory != "~/Downloads" {
		t.Fatal(restored.Network.DownloadDirectory)
	}
	if restored.Native.Notifications != "ask" || restored.Native.LinkRoutingEnabled() {
		t.Fatalf("native: %+v", restored.Native)
	}
	if restored.WebView.Navigation.Popups != "external" || len(restored.WebView.Navigation.SafeDomains) != 2 {
		t.Fatalf("nav: %+v", restored.WebView.Navigation)
	}
	if restored.Window.Height != bundled.Window.Height {
		t.Fatalf("height should stay bundled: %d", restored.Window.Height)
	}
}

func TestPrefsOverlayRevertBool(t *testing.T) {
	bundled := samplePrefsConfig()
	bundled.Window.AlwaysOnTop = true
	cur := bundled.Clone()
	cur.Window.AlwaysOnTop = false
	overlay, err := prefsOverlay(bundled, cur)
	if err != nil {
		t.Fatal(err)
	}
	restored := bundled.Clone()
	if err := applyUserPrefs(restored, overlay); err != nil {
		t.Fatal(err)
	}
	if restored.Window.AlwaysOnTop {
		t.Fatalf("false overlay not applied: %s", overlay)
	}
}

func TestPrefsOverlayEmptyWhenEqual(t *testing.T) {
	cfg := samplePrefsConfig()
	overlay, err := prefsOverlay(cfg, cfg.Clone())
	if err != nil || overlay != nil {
		t.Fatalf("got %s %v", overlay, err)
	}
}

func TestSaveUserPrefsRoundTripFile(t *testing.T) {
	dir := t.TempDir()
	bundled := samplePrefsConfig()
	cur := bundled.Clone()
	cur.Network.DownloadDirectory = "/tmp/dl"
	if err := saveUserPrefs(bundled, cur, dir); err != nil {
		t.Fatal(err)
	}
	loaded := bundled.Clone()
	loadUserPrefs(loaded, dir)
	if loaded.Network.DownloadDirectory != "/tmp/dl" {
		t.Fatalf("loaded %q", loaded.Network.DownloadDirectory)
	}
	cur.Network.DownloadDirectory = bundled.Network.DownloadDirectory
	if err := saveUserPrefs(bundled, cur, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, prefsFile)); !os.IsNotExist(err) {
		t.Fatalf("empty overlay should remove the file: %v", err)
	}
}

func samplePrefsConfig() *config.Config {
	return &config.Config{
		SchemaVersion: 1,
		App: config.AppConfig{
			Name: "Mail", Identifier: "app.mypack.mail", ProfileID: "app.mypack.mail",
			Version: "1.0.0", Source: config.Source{Type: config.SourceURL, Value: "https://mail.example"},
		},
		Runtime: config.RuntimeConfig{Version: "1.0.10"},
		Window:  config.WindowConfig{Width: 1200, Height: 780, Zoom: 100, DarkMode: "system", TitleBarStyle: "default"},
		WebView: config.WebViewConfig{
			Navigation: config.NavigationConfig{SafeDomains: []string{"mail.example"}, Popups: "internal"},
		},
	}
}
