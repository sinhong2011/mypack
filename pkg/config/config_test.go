package config

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// hostAbs is an absolute path on the machine running the test.
func hostAbs(parts ...string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(append([]string{`C:\`}, parts...)...)
	}
	return filepath.Join(append([]string{"/"}, parts...)...)
}

func TestProjectBuildValidation(t *testing.T) {
	host := Target{OS: "darwin", Arch: "arm64", Format: "app"}
	mk := func(b *ProjectBuild, project string) *Config {
		c := New("App", Source{Type: SourceDir, Value: "/p/dist", Entry: "index.html", Project: project, Build: b}, host)
		return c
	}
	proj := hostAbs("p")
	ok := &ProjectBuild{PackageManager: "pnpm", Script: "build:prod", OutDir: "dist/web", Framework: "Vite"}
	if err := mk(ok, proj).Validate(); err != nil {
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
		err := mk(b, proj).Validate()
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Issues[0].Field != field {
			t.Errorf("%s: %v", field, err)
		}
	}
	for _, s := range []string{"-rf", "--prefix=/x", "a b", "a;b", ""} {
		if err := mk(&ProjectBuild{PackageManager: "npm", Script: s, OutDir: "dist"}, proj).Validate(); err == nil {
			t.Errorf("script %q accepted", s)
		}
	}
	for _, o := range []string{"/abs", "a/../../b", `..\x`, "C:\\x", ""} {
		if err := mk(&ProjectBuild{PackageManager: "npm", Script: "build", OutDir: o}, proj).Validate(); err == nil {
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
	if c.Runtime.Version != DefaultRuntimeVersion || DefaultRuntimeVersion != "1.0.9" {
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

func TestGroupConfig(t *testing.T) {
	c := New("Work", Source{Type: SourceURL, Value: "https://app.slack.com"}, Target{OS: "darwin", Arch: "arm64"})
	if c.IsGroup() {
		t.Fatal("a plain app is not a group")
	}
	b, _ := c.Marshal()
	if strings.Contains(string(b), `"group"`) {
		t.Fatal("no group must not be written")
	}
	c.App.Group = &GroupConfig{Members: []GroupMember{
		{ID: "slack", Name: "Slack", URL: "https://app.slack.com"},
		{ID: "gmail", Name: "Gmail", URL: "https://mail.google.com", Icon: "/tmp/g.png"},
	}}
	if err := c.Validate(); err != nil || !c.IsGroup() {
		t.Fatalf("valid group: %v", err)
	}
	b, _ = c.Marshal()
	c2, err := Parse(b)
	if err != nil || len(c2.App.Group.Members) != 2 || c2.App.Group.Members[1].Icon != "/tmp/g.png" {
		t.Fatalf("round trip: %v", err)
	}
	bad := c.Clone()
	bad.App.Group.Members[1].ID = "slack"
	bad.App.Group.Members[0].URL = "not a url"
	err = bad.Validate()
	if err == nil || !strings.Contains(err.Error(), "members[1].id") || !strings.Contains(err.Error(), "members[0].url") {
		t.Fatalf("bad members accepted: %v", err)
	}
	one := c.Clone()
	one.App.Group.Members = one.App.Group.Members[:1]
	if err := one.Validate(); err == nil || !strings.Contains(err.Error(), "app.group.members") {
		t.Fatalf("one member accepted: %v", err)
	}
	local := c.Clone()
	local.App.Group.Members[1] = GroupMember{ID: "docs", Name: "Docs", Local: "/tmp/docs", Entry: "index.html"}
	if err := local.Validate(); err != nil || !local.App.Group.Members[1].IsLocal() {
		t.Fatalf("local member: %v", err)
	}
	local.App.Group.Members[1].URL = "https://x.example"
	if err := local.Validate(); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("url and local accepted: %v", err)
	}
	local.App.Group.Members[1] = GroupMember{ID: "docs", Name: "Docs", Local: "../../etc"}
	if err := local.Validate(); err == nil || !strings.Contains(err.Error(), "cannot leave") {
		t.Fatalf("escaping path accepted: %v", err)
	}
}

func TestMemberID(t *testing.T) {
	taken := map[string]bool{}
	for _, tc := range []struct{ in, want string }{
		{"Slack", "slack"}, {"Google Calendar", "google-calendar"}, {"Slack", "slack-2"}, {"  ", "site"}, {"日本", "site-2"},
	} {
		got := MemberID(tc.in, taken)
		if got != tc.want {
			t.Errorf("MemberID(%q) = %q, want %q", tc.in, got, tc.want)
		}
		taken[got] = true
	}
}

func TestIconStyleValidation(t *testing.T) {
	c := New("Chat", Source{Type: SourceURL, Value: "https://chat.example.com"}, Target{OS: "darwin", Arch: "arm64"})
	b, _ := c.Marshal()
	if strings.Contains(string(b), `"style"`) {
		t.Fatal("no style written by default")
	}
	ok := []IconStyle{
		{},
		{Glyph: GlyphText, Text: "GM", Font: "mono", Background: BackgroundGlass, Colors: []string{"#112233", "#AABBCC"}},
		{Glyph: GlyphEmoji, Text: "🚀", Background: BackgroundSolid, Colors: []string{"#000000"}, Scale: 80},
		{Background: BackgroundNone, Tint: TintColor, TintColor: "#ff0000", Badge: "Work", BadgeColor: "#2563eb"},
	}
	for i, st := range ok {
		st := st
		c.App.Icon.Style = &st
		if err := c.Validate(); err != nil {
			t.Fatalf("ok[%d]: %v", i, err)
		}
	}
	bad := map[string]IconStyle{
		"text":       {Glyph: GlyphText},
		"toolong":    {Glyph: GlyphText, Text: "ABCD"},
		"glyph":      {Glyph: "svg"},
		"font":       {Font: "comic"},
		"background": {Background: "plaid"},
		"color":      {Colors: []string{"red"}},
		"colors":     {Colors: []string{"#000000", "#000000", "#000000"}},
		"scale":      {Scale: 5},
		"tint":       {Tint: TintColor},
		"badge":      {Badge: "A very long badge"},
		"badgeColor": {Badge: "x", BadgeColor: "blue"},
	}
	for name, st := range bad {
		st := st
		c.App.Icon.Style = &st
		if err := c.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestBlockingConfig(t *testing.T) {
	c := New("Mail", Source{Type: SourceURL, Value: "https://mail.example.com"}, Target{OS: "darwin", Arch: "arm64"})
	b, _ := c.Marshal()
	if strings.Contains(string(b), `"blocking"`) {
		t.Fatal("no blocking section by default")
	}
	for in, want := range map[string]string{
		"example.com": "example.com", "https://www.Example.com/a?b": "example.com", "*.example.co.uk": "example.co.uk",
		"example.com:8443": "example.com", "localhost": "", "not a domain": "", "": "",
	} {
		if got := NormalizeAllowDomain(in); got != want {
			t.Errorf("NormalizeAllowDomain(%q) = %q, want %q", in, got, want)
		}
	}
	c.WebView.Blocking = &BlockingConfig{Trackers: true, Allow: []string{"bank.example"}, Hide: []string{".promo"}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, bc := range map[string]BlockingConfig{
		"allow": {Allow: []string{"https://x.com"}},
		"hide":  {Hide: []string{".a { color: red }"}},
		"empty": {Hide: []string{"  "}},
	} {
		bc := bc
		c.WebView.Blocking = &bc
		if err := c.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestMinRuntimeRaisesPin(t *testing.T) {
	host := Target{OS: "darwin", Arch: "arm64"}
	c := New("Mail", Source{Type: SourceURL, Value: "https://mail.example.com"}, host)
	c.Runtime.Version = "1.0.5"
	c.ApplyDefaults(host)
	if c.Runtime.Version != "1.0.5" {
		t.Fatalf("a plain app keeps its pin, got %s", c.Runtime.Version)
	}
	c.Window.Mode = ModeSidebar
	c.ApplyDefaults(host)
	if c.Runtime.Version != "1.0.6" {
		t.Fatalf("sidebar mode needs 1.0.6, got %s", c.Runtime.Version)
	}
	c.WebView.Blocking = &BlockingConfig{CookieBanners: true}
	c.ApplyDefaults(host)
	if c.Runtime.Version != "1.0.7" {
		t.Fatalf("blocking needs 1.0.7, got %s", c.Runtime.Version)
	}
	c.Runtime.Version = "1.2.0"
	c.ApplyDefaults(host)
	if c.Runtime.Version != "1.2.0" {
		t.Fatal("a newer pin is never lowered")
	}
}

func TestTabs(t *testing.T) {
	host := Target{OS: "darwin", Arch: "arm64"}
	c := New("Docs", Source{Type: SourceURL, Value: "https://docs.example.com"}, host)
	c.Runtime.Version = "1.0.6"
	c.Window.Tabs = true
	c.ApplyDefaults(host)
	if c.Runtime.Version != "1.0.8" {
		t.Fatalf("tabs need 1.0.8, got %s", c.Runtime.Version)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.App.Group = &GroupConfig{Members: []GroupMember{{ID: "a", Name: "A", URL: "https://a.example.com"}, {ID: "b", Name: "B", URL: "https://b.example.com"}}}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "window.tabs") {
		t.Fatalf("tabs in a group: %v", err)
	}
}

func TestGlassTitleBar(t *testing.T) {
	host := Target{OS: "darwin", Arch: "arm64"}
	c := New("Docs", Source{Type: SourceURL, Value: "https://docs.example.com"}, host)
	c.Runtime.Version = "1.0.8"
	c.Window.TitleBarStyle = "glass"
	c.ApplyDefaults(host)
	if err := c.Validate(); err != nil {
		t.Fatalf("glass must validate: %v", err)
	}
	if c.Runtime.Version != "1.0.9" {
		t.Fatalf("glass needs 1.0.9, got %s", c.Runtime.Version)
	}
	c.Window.TitleBarStyle = "frosted"
	if err := c.Validate(); err == nil {
		t.Fatal("an unknown title bar style must be refused")
	}
	c.Window.TitleBarStyle = "overlay"
	if got := c.MinRuntime(); got != "" {
		t.Fatalf("seamless needs no newer runtime, got %q", got)
	}
}

// The installer bundles DefaultRuntimeVersion. A feature that needs a newer
// runtime makes every project using it download one on first build, even on
// a fresh install, so the default must keep up with MinRuntime.
func TestDefaultRuntimeCoversEveryFeature(t *testing.T) {
	host := Target{OS: "darwin", Arch: "arm64"}
	for name, on := range map[string]func(*Config){
		"glass":   func(c *Config) { c.Window.TitleBarStyle = "glass" },
		"tabs":    func(c *Config) { c.Window.Tabs = true },
		"menuBar": func(c *Config) { c.Window.Mode = ModeMenuBar },
	} {
		c := New("Docs", Source{Type: SourceURL, Value: "https://docs.example.com"}, host)
		on(c)
		if min := c.MinRuntime(); min == "" || !RuntimeAtLeast(DefaultRuntimeVersion, min) {
			t.Errorf("%s needs runtime %q, newer than DefaultRuntimeVersion %s: bump it and publish that runtime", name, min, DefaultRuntimeVersion)
		}
	}
}
