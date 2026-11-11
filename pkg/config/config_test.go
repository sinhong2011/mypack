package config

import (
	"errors"
	"strings"
	"testing"
)

func TestProjectBuildValidation(t *testing.T) {
	host := Target{OS: "darwin", Arch: "arm64", Format: "app"}
	mk := func(b *ProjectBuild, project string) *Config {
		c := New("App", Source{Type: SourceDir, Value: "/p/dist", Entry: "index.html", Project: project, Build: b}, host)
		return c
	}
	ok := &ProjectBuild{PackageManager: "pnpm", Script: "build:prod", OutDir: "dist/web", Framework: "Vite"}
	if err := mk(ok, "/p").Validate(); err != nil {
		t.Fatalf("valid build rejected: %v", err)
	}
	if err := mk(nil, "").Validate(); err != nil {
		t.Fatalf("plain dir rejected: %v", err)
	}
	bad := map[string]*ProjectBuild{
		"app.source.build.packageManager": {PackageManager: "make", Script: "build", OutDir: "dist"},
		"app.source.build.script":         {PackageManager: "npm", Script: "build && rm -rf ~", OutDir: "dist"},
		"app.source.build.outDir":         {PackageManager: "npm", Script: "build", OutDir: "../dist"},
	}
	for field, b := range bad {
		err := mk(b, "/p").Validate()
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Issues[0].Field != field {
			t.Errorf("%s: %v", field, err)
		}
	}
	for _, s := range []string{"-rf", "--prefix=/x", "a b", "a;b", ""} {
		if err := mk(&ProjectBuild{PackageManager: "npm", Script: s, OutDir: "dist"}, "/p").Validate(); err == nil {
			t.Errorf("script %q accepted", s)
		}
	}
	for _, o := range []string{"/abs", "a/../../b", `..\x`, "C:\\x", ""} {
		if err := mk(&ProjectBuild{PackageManager: "npm", Script: "build", OutDir: o}, "/p").Validate(); err == nil {
			t.Errorf("outDir %q accepted", o)
		}
	}
	if err := mk(ok, "").Validate(); err == nil {
		t.Error("build without project accepted")
	}
	if err := mk(ok, "relative/dir").Validate(); err == nil {
		t.Error("relative project accepted")
	}
	// Old configs (no project/build keys) still parse; new keys round-trip.
	c, err := Parse([]byte(`{"schemaVersion":1,"app":{"name":"A","source":{"type":"dir","value":"/x"}}}`))
	if err != nil || c.App.Source.Build != nil || c.App.Source.Project != "" {
		t.Fatalf("old config: %v %+v", err, c)
	}
	b, _ := mk(ok, "/p").Marshal()
	c2, err := Parse(b)
	if err != nil || c2.App.Source.Build == nil || c2.App.Source.Build.Script != "build:prod" {
		t.Fatalf("round trip: %v", err)
	}
}

func TestNativeAndUAPresetDefaults(t *testing.T) {
	c := New("Mail", Source{Type: SourceURL, Value: "https://mail.example.com"}, Target{OS: "darwin", Arch: "arm64"})
	if c.Runtime.Version != DefaultRuntimeVersion || DefaultRuntimeVersion != "1.0.6" {
		t.Fatalf("runtime version = %q", c.Runtime.Version)
	}
	if c.Native.NotificationsMode() != "allow" || c.Native.BadgeMode() != "auto" || !c.Native.LinkRoutingEnabled() {
		t.Fatal("defaults: notifications=allow badge=auto linkRouting=true")
	}
	// Defaults must not appear in mypack.json (no config-hash churn for old projects).
	b, _ := c.Marshal()
	for _, k := range []string{"notifications", "badge", "linkRouting", "userAgentPreset"} {
		if strings.Contains(string(b), k) {
			t.Errorf("%s serialized by default", k)
		}
	}
	if c.WebView.EffectiveUAPreset() != "safari" {
		t.Fatal("empty preset = safari")
	}
	c.WebView.UserAgent = "Legacy/1"
	if c.WebView.EffectiveUAPreset() != "custom" {
		t.Fatal("legacy userAgent = custom")
	}
	c.WebView.UserAgentPreset = "chrome"
	if c.WebView.EffectiveUAPreset() != "chrome" {
		t.Fatal("explicit preset wins")
	}
}

func TestNativeAndUAPresetValidation(t *testing.T) {
	ok := func(mut func(*Config)) error {
		c := New("Mail", Source{Type: SourceURL, Value: "https://mail.example.com"}, Target{OS: "darwin", Arch: "arm64"})
		mut(c)
		return c.Validate()
	}
	for _, good := range []string{"", "ask", "allow", "deny"} {
		if err := ok(func(c *Config) { c.Native.Notifications = good }); err != nil {
			t.Errorf("notifications %q: %v", good, err)
		}
	}
	if err := ok(func(c *Config) { c.Native.Notifications = "sometimes" }); err == nil {
		t.Error("bad notifications accepted")
	}
	if err := ok(func(c *Config) { c.Native.Badge = "on" }); err == nil {
		t.Error("bad badge accepted")
	}
	for _, p := range []string{"", "safari", "chrome", "edge", "firefox", "mobile-safari"} {
		if err := ok(func(c *Config) { c.WebView.UserAgentPreset = p }); err != nil {
			t.Errorf("preset %q: %v", p, err)
		}
	}
	if err := ok(func(c *Config) { c.WebView.UserAgentPreset = "netscape" }); err == nil {
		t.Error("bad preset accepted")
	}
	if err := ok(func(c *Config) { c.WebView.UserAgentPreset = "custom" }); err == nil {
		t.Error("custom without a user agent accepted")
	}
	if err := ok(func(c *Config) { c.WebView.UserAgentPreset, c.WebView.UserAgent = "custom", "X/1" }); err != nil {
		t.Errorf("custom with a user agent: %v", err)
	}
	// Round trip through strict parsing.
	c := New("Mail", Source{Type: SourceURL, Value: "https://mail.example.com"}, Target{OS: "darwin", Arch: "arm64"})
	off := false
	c.Native.Notifications, c.Native.Badge, c.Native.LinkRouting, c.WebView.UserAgentPreset = "ask", "off", &off, "firefox"
	b, _ := c.Marshal()
	back, err := Parse(b)
	if err != nil || back.Native.Notifications != "ask" || back.Native.LinkRoutingEnabled() || back.WebView.UserAgentPreset != "firefox" {
		t.Fatalf("round trip: %+v %v", back, err)
	}
}

func TestWindowModeAndPanel(t *testing.T) {
	c := New("Chat", Source{Type: SourceURL, Value: "https://chat.example.com"}, Target{OS: "darwin", Arch: "arm64"})
	if c.Window.EffectiveMode() != ModeWindow {
		t.Fatalf("default mode = %q", c.Window.EffectiveMode())
	}
	if w, h := c.Window.PanelSize(); w != DefaultPanelWidth || h != DefaultPanelHeight {
		t.Fatalf("default panel = %dx%d", w, h)
	}
	b, _ := c.Marshal()
	for _, k := range []string{`"mode"`, `"panelWidth"`, `"panelHeight"`} {
		if strings.Contains(string(b), k) {
			t.Fatalf("default %s written to mypack.json", k)
		}
	}
	for _, m := range []string{"", ModeWindow, ModeCompact, ModeMenuBar, ModeSidebar} {
		c.Window.Mode = m
		if err := c.Validate(); err != nil {
			t.Fatalf("mode %q: %v", m, err)
		}
	}
	c.Window.Mode, c.Window.SidebarEdge = ModeSidebar, EdgeLeft
	if err := c.Validate(); err != nil || c.Window.EffectiveSidebarEdge() != EdgeLeft {
		t.Fatalf("sidebar left: %v", err)
	}
	c.Window.SidebarEdge = "top"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "window.sidebarEdge") {
		t.Fatalf("bad edge accepted: %v", err)
	}
	c.Window.SidebarEdge = ""
	if c.Window.EffectiveSidebarEdge() != EdgeRight {
		t.Fatal("default edge must be right")
	}
	c.Window.Mode = "floating"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "window.mode") {
		t.Fatalf("bad mode accepted: %v", err)
	}
	c.Window.Mode, c.Window.PanelWidth, c.Window.PanelHeight = ModeMenuBar, 100, 2000
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "window.panelWidth") || !strings.Contains(err.Error(), "window.panelHeight") {
		t.Fatalf("bad panel accepted: %v", err)
	}
	c.Window.PanelWidth, c.Window.PanelHeight = 360, 0
	if w, h := c.Window.PanelSize(); w != 360 || h != DefaultPanelHeight {
		t.Fatalf("panel = %dx%d", w, h)
	}
}

func TestRuntimeAtLeast(t *testing.T) {
	for _, tc := range []struct {
		v, min string
		want   bool
	}{
		{"1.0.6", "1.0.6", true}, {"1.0.10", "1.0.6", true}, {"1.1", "1.0.6", true}, {"2.0.0", "1.0.6", true},
		{"1.0.5", "1.0.6", false}, {"1.0", "1.0.6", false}, {"", "1.0.6", false}, {"x", "1.0.6", false},
	} {
		if got := RuntimeAtLeast(tc.v, tc.min); got != tc.want {
			t.Errorf("RuntimeAtLeast(%q, %q) = %v", tc.v, tc.min, got)
		}
	}
}
