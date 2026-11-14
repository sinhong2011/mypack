package app

import (
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"mypack.dev/mypack/pkg/config"
)

func TestBuildMenuIncludesSettings(t *testing.T) {
	a := &App{cfg: samplePrefsConfig(), disabled: map[string]bool{}, state: OpenStore(t.TempDir())}
	m := a.buildMenu()
	if m == nil {
		t.Fatal("nil menu")
	}
	var found bool
	var walk func([]*mygo.MenuItem)
	walk = func(items []*mygo.MenuItem) {
		for _, it := range items {
			if it == nil {
				continue
			}
			if it.Label == "Settings…" {
				found = true
			}
			walk(it.Submenu)
		}
	}
	walk(m.Items())
	if !found {
		t.Fatal("Settings… is missing from the menu")
	}
	items := m.Items()
	if len(items) == 0 || items[0] == nil || items[0].Label != "Mail" {
		t.Fatal("the first menu should be the app name")
	}
	appHas := false
	for _, x := range items[0].Submenu {
		if x != nil && x.Label == "Settings…" {
			appHas = true
		}
	}
	if !appHas {
		t.Fatal("Settings… should be in the app menu")
	}
	fileHas := false
	for _, it := range items {
		if it != nil && it.Label == "File" {
			for _, x := range it.Submenu {
				if x != nil && x.Label == "Settings…" {
					fileHas = true
				}
			}
		}
	}
	if runtime.GOOS == "darwin" && fileHas {
		t.Fatal("Settings… should be in the app menu on macOS, not File")
	}
	if runtime.GOOS != "darwin" && !fileHas {
		t.Fatal("Settings… should be in File on Windows and Linux")
	}
}

func TestApplySettingsForm(t *testing.T) {
	cfg := samplePrefsConfig()
	off := false
	f := settingsForm{
		Title: "Work Mail",
		Window: config.WindowConfig{
			Width: 1400, Height: 900, MinWidth: 480, MinHeight: 360, Zoom: 110,
			AlwaysOnTop: true, DarkMode: "dark", TitleBarStyle: "default", Mode: "compact",
		},
		WebView: settingsWebViewForm{
			UserAgentPreset:   config.UAPresetChrome,
			RestoreLastURL:    true,
			ExternalByDefault: false,
			Navigation: config.NavigationConfig{
				SafeDomains:      []string{"https://Mail.Example/", "mail.example"},
				InternalURLRegex: `^https://mail\.example/`,
				Popups:           "external",
			},
			CSS: "body{font-size:15px}",
			Blocking: &config.BlockingConfig{
				Trackers: true,
				Allow:    []string{"https://www.cdn.example/x"},
			},
		},
		Network: config.NetworkConfig{DownloadDirectory: "~/Downloads"},
		Native: settingsNativeForm{
			Tray: true, Notifications: "ask", Badge: "off", LinkRouting: false,
			GlobalShortcut: "CmdOrCtrl+Shift+M",
		},
	}
	if err := applySettingsForm(cfg, f); err != nil {
		t.Fatal(err)
	}
	if cfg.App.Title != "Work Mail" || cfg.App.Name != "Mail" {
		t.Fatalf("title/name: %q %q", cfg.App.Title, cfg.App.Name)
	}
	if cfg.Window.Width != 1400 || !cfg.Window.AlwaysOnTop || cfg.Window.Mode != config.ModeCompact {
		t.Fatalf("window: %+v", cfg.Window)
	}
	if cfg.WebView.EffectiveUAPreset() != config.UAPresetChrome || cfg.WebView.UserAgent != "" {
		t.Fatalf("ua: %+v", cfg.WebView)
	}
	if cfg.WebView.Navigation.ExternalByDefault == nil || *cfg.WebView.Navigation.ExternalByDefault {
		t.Fatal("links should stay in the app")
	}
	if got := cfg.WebView.Navigation.SafeDomains; len(got) != 1 || got[0] != "mail.example" {
		t.Fatalf("domains: %v", got)
	}
	if inlineInjection(cfg, "css") != "body{font-size:15px}" {
		t.Fatal("css")
	}
	if cfg.WebView.Blocking == nil || !cfg.WebView.Blocking.Trackers || cfg.WebView.Blocking.Allow[0] != "cdn.example" {
		t.Fatalf("blocking: %+v", cfg.WebView.Blocking)
	}
	if cfg.Network.DownloadDirectory != "~/Downloads" {
		t.Fatal(cfg.Network.DownloadDirectory)
	}
	if cfg.Native.NotificationsMode() != "ask" || cfg.Native.BadgeMode() != "off" || cfg.Native.LinkRoutingEnabled() {
		t.Fatalf("native: %+v", cfg.Native)
	}
	_ = off
}

func TestApplySettingsFormRejectsBadRegex(t *testing.T) {
	cfg := samplePrefsConfig()
	err := applySettingsForm(cfg, settingsForm{
		Window: config.WindowConfig{Width: 800, Height: 600, Zoom: 100},
		WebView: settingsWebViewForm{
			UserAgentPreset:   config.UAPresetSafari,
			Navigation:        config.NavigationConfig{InternalURLRegex: "("},
			ExternalByDefault: true,
		},
		Native: settingsNativeForm{Notifications: "allow", Badge: "auto", LinkRouting: true},
	})
	if err == nil || !strings.Contains(err.Error(), "regular expression") {
		t.Fatalf("got %v", err)
	}
}

func TestSettingsHTMLHasPanes(t *testing.T) {
	html := settingsHTML(settingsBoot{
		Name: "Mail", Version: "1.0.0", Identifier: "app.mypack.mail",
		settingsForm: settingsForm{
			Window: config.WindowConfig{Width: 1200, Height: 780, Zoom: 100},
			Native: settingsNativeForm{Notifications: "allow", Badge: "auto", LinkRouting: true},
			WebView: settingsWebViewForm{UserAgentPreset: "safari", ExternalByDefault: true,
				Blocking: &config.BlockingConfig{}},
		},
	})
	for _, s := range []string{"Settings", "Downloads", "Safe domains", "Custom CSS", "Settings…", "mypack:settings:save"} {
		if s == "Settings…" {
			continue
		}
		if !strings.Contains(html, s) {
			t.Errorf("missing %q", s)
		}
	}
	if !strings.Contains(html, "app.mypack.mail") {
		t.Error("boot identifier")
	}
}

func TestSettingsNeedReloadAndRestart(t *testing.T) {
	a := samplePrefsConfig()
	b := a.Clone()
	b.Network.DownloadDirectory = "~/Downloads"
	if settingsNeedReload(a, b) || settingsNeedRestart(a, b) {
		t.Fatal("downloads only should apply live")
	}
	b.WebView.UserAgentPreset = config.UAPresetChrome
	if !settingsNeedReload(a, b) {
		t.Fatal("user agent should reload")
	}
	b = a.Clone()
	b.Window.TitleBarStyle = "hidden"
	if !settingsNeedRestart(a, b) {
		t.Fatal("title bar should restart")
	}
}

func TestCopyUserPrefsLeavesIdentity(t *testing.T) {
	dst := samplePrefsConfig()
	src := dst.Clone()
	src.App.Name = "Nope"
	src.App.Title = "Shown"
	src.Window.AlwaysOnTop = true
	copyUserPrefs(dst, src)
	if dst.App.Name != "Mail" || dst.App.Title != "Shown" || !dst.Window.AlwaysOnTop {
		t.Fatalf("%+v", dst.App)
	}
}
