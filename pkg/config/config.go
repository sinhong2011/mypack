// Package config defines mypack.json: the versioned, language-neutral project
// configuration shared by the Builder (GUI, CLI, MCP) and the generated-app
// runtime. Machine-facing keys are stable English identifiers and are never
// localized (spec §9D.3, §11).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SchemaVersion is the current mypack.json schema version.
const SchemaVersion = 1

// DefaultRuntimeVersion is the runtime version new projects pin (ADR-005).
const DefaultRuntimeVersion = "1.0.9"

// Config is the full mypack.json document.
type Config struct {
	SchemaVersion int           `json:"schemaVersion"`
	App           AppConfig     `json:"app"`
	Runtime       RuntimeConfig `json:"runtime"`
	Window        WindowConfig  `json:"window"`
	WebView       WebViewConfig `json:"webview"`
	Network       NetworkConfig `json:"network,omitempty"`
	Native        NativeConfig  `json:"native,omitempty"`
	Build         BuildConfig   `json:"build"`
}

// AppConfig is identity (APP-01..06) and source (SRC-01..04).
type AppConfig struct {
	Name       string `json:"name"`
	Title      string `json:"title,omitempty"` // window title; defaults to Name
	Identifier string `json:"identifier"`      // stable bundle/app id; never silently changed
	ProfileID  string `json:"profileId"`       // stable per-user data dir key
	Version    string `json:"version"`
	Source     Source `json:"source"`
	Icon       Icon   `json:"icon"`
	// Group makes the app a group of websites (runtime 1.0.6+): one window
	// with a rail of the members' icons, each member in its own session.
	// Source is the first website member's url (or, with only local
	// members, the first one's folder), so identifiers, installs and icons
	// work as for any app; the runtime opens the members, not Source.
	Group *GroupConfig `json:"group,omitempty"`
}

// GroupConfig lists the websites of a group app.
type GroupConfig struct {
	Members []GroupMember `json:"members"`
}

// GroupMember is one website of a group app.
type GroupMember struct {
	// ID is stable ([a-z0-9-], at most 32): the member's session (its
	// logins) is keyed by it, so renaming a member keeps them.
	ID   string `json:"id"`
	Name string `json:"name"`
	// URL is a website member's address; Local a local one's folder or HTML
	// file (absolute in a project; "members/<id>/site" in a built app, with
	// Entry the page to open). Exactly one is set.
	URL   string `json:"url,omitempty"`
	Local string `json:"local,omitempty"`
	Entry string `json:"entry,omitempty"`
	// Icon is a PNG: a path in a project (the Builder bundles it as
	// members/<id>.png) or relative to the resources dir in a built app.
	// Empty draws the member's initial.
	Icon string `json:"icon,omitempty"`
	// ProjectID is the Library project the member was made from, if any.
	ProjectID string `json:"projectId,omitempty"`
}

// Group limits.
const (
	MinGroupMembers = 2
	MaxGroupMembers = 12
)

var memberIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// IsLocal reports whether the member is a local folder or file.
func (m GroupMember) IsLocal() bool { return m.Local != "" }

// IsGroup reports whether the app is a group of websites.
func (c *Config) IsGroup() bool { return c.App.Group != nil && len(c.App.Group.Members) > 0 }

// MemberID derives a member ID from a name or URL host: lowercase letters,
// digits and dashes, unique among taken.
func MemberID(name string, taken map[string]bool) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	id := strings.Trim(b.String(), "-")
	if len(id) > 28 {
		id = strings.Trim(id[:28], "-")
	}
	if id == "" {
		id = "site"
	}
	base := id
	for i := 2; taken[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	return id
}

// SourceType is url, file or dir.
type SourceType string

const (
	SourceURL  SourceType = "url"
	SourceFile SourceType = "file"
	SourceDir  SourceType = "dir"
)

type Source struct {
	Type  SourceType `json:"type"`
	Value string     `json:"value"`
	// Entry is the HTML file inside a dir source (default index.html).
	Entry string `json:"entry,omitempty"`
	// Project is the absolute root of the JavaScript project that produces
	// the dir source (Value is then <Project>/<Build.OutDir>). Optional.
	Project string `json:"project,omitempty"`
	// Build, when set, lets the Builder run the project's build script
	// (after explicit user approval) before packaging Value. Optional.
	Build *ProjectBuild `json:"build,omitempty"`
}

// ProjectBuild describes how to build a JavaScript project. It is never a
// free-form shell string: the Builder executes `<packageManager> run <script>`
// with explicit arguments.
type ProjectBuild struct {
	// PackageManager is "npm", "pnpm", "yarn" or "bun".
	PackageManager string `json:"packageManager"`
	// Script is a package.json script name ("build", "generate", …).
	Script string `json:"script"`
	// OutDir is the build output folder, relative to Source.Project.
	OutDir string `json:"outDir"`
	// Framework is informational ("Vite", "Next.js", …).
	Framework string `json:"framework,omitempty"`
}

type Icon struct {
	// Source is "auto" (website favicon), "file" (Path) or "default".
	Source string `json:"source"`
	Path   string `json:"path,omitempty"`
	// Style dresses the icon up at build time (the icon studio): a new
	// background, letters or an emoji instead of the image, a tint, a
	// badge. Nil keeps the icon as it is.
	Style *IconStyle `json:"style,omitempty"`
}

// IconStyle is how the icon studio draws an app icon. Every field is
// optional; the zero value draws the icon's image on its own colours.
type IconStyle struct {
	// Glyph is what sits in the middle: "image" (the icon's own image, the
	// default), "text" (Text as letters) or "emoji" (Text as one emoji).
	Glyph string `json:"glyph,omitempty"`
	Text  string `json:"text,omitempty"`
	// Font of letters: "bold" (default), "regular", "mono" or "rounded"
	// (the system's rounded face where there is one, else bold).
	Font string `json:"font,omitempty"`
	// Background: "auto" (colours taken from the image, the default),
	// "solid", "gradient", "glass" or "none" (the image as it is, only the
	// tint and badge are added).
	Background string `json:"background,omitempty"`
	// Colors are one or two "#rrggbb" colours of the background (top, then
	// bottom); empty means picked from the image or the name.
	Colors []string `json:"colors,omitempty"`
	// Scale is the glyph's size in percent of the icon (20–100; 0 means the
	// usual 64).
	Scale int `json:"scale,omitempty"`
	// Tint recolours the glyph: "" (as it is), "white", "black" or "color"
	// (TintColor).
	Tint      string `json:"tint,omitempty"`
	TintColor string `json:"tintColor,omitempty"`
	// Badge is a short label on a ribbon at the bottom ("Work", "Beta"), in
	// BadgeColor (default red).
	Badge      string `json:"badge,omitempty"`
	BadgeColor string `json:"badgeColor,omitempty"`
}

// Icon style values.
const (
	GlyphImage = "image"
	GlyphText  = "text"
	GlyphEmoji = "emoji"

	BackgroundAuto     = "auto"
	BackgroundSolid    = "solid"
	BackgroundGradient = "gradient"
	BackgroundGlass    = "glass"
	BackgroundNone     = "none"

	TintWhite = "white"
	TintBlack = "black"
	TintColor = "color"

	MaxIconText  = 3
	MaxIconBadge = 12
)

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// IsHexColor reports whether s is a "#rrggbb" colour.
func IsHexColor(s string) bool { return hexColorRe.MatchString(s) }

func (st *IconStyle) validate(add func(field, code, msg string)) {
	const f = "app.icon.style."
	switch st.Glyph {
	case "", GlyphImage:
	case GlyphText, GlyphEmoji:
		n := utf8.RuneCountInString(strings.TrimSpace(st.Text))
		if n == 0 {
			add(f+"text", "REQUIRED", "letters or an emoji are required")
		} else if st.Glyph == GlyphText && n > MaxIconText {
			add(f+"text", "INVALID_ICON", "use up to 3 letters")
		} else if st.Glyph == GlyphEmoji && n > 8 {
			add(f+"text", "INVALID_ICON", "use a single emoji")
		}
	default:
		add(f+"glyph", "INVALID_ICON", "glyph must be image, text or emoji")
	}
	switch st.Font {
	case "", "bold", "regular", "mono", "rounded":
	default:
		add(f+"font", "INVALID_ICON", "font must be bold, regular, mono or rounded")
	}
	switch st.Background {
	case "", BackgroundAuto, BackgroundSolid, BackgroundGradient, BackgroundGlass, BackgroundNone:
	default:
		add(f+"background", "INVALID_ICON", "background must be auto, solid, gradient, glass or none")
	}
	if len(st.Colors) > 2 {
		add(f+"colors", "INVALID_ICON", "use one or two colours")
	}
	for _, c := range st.Colors {
		if !IsHexColor(c) {
			add(f+"colors", "INVALID_COLOR", "colours must look like #3366ff")
			break
		}
	}
	if st.Scale != 0 && (st.Scale < 20 || st.Scale > 100) {
		add(f+"scale", "INVALID_ICON", "scale must be between 20 and 100")
	}
	switch st.Tint {
	case "", TintWhite, TintBlack:
	case TintColor:
		if !IsHexColor(st.TintColor) {
			add(f+"tintColor", "INVALID_COLOR", "the tint colour must look like #3366ff")
		}
	default:
		add(f+"tint", "INVALID_ICON", "tint must be white, black or color")
	}
	if utf8.RuneCountInString(st.Badge) > MaxIconBadge {
		add(f+"badge", "INVALID_ICON", "the badge can have up to 12 characters")
	}
	if st.BadgeColor != "" && !IsHexColor(st.BadgeColor) {
		add(f+"badgeColor", "INVALID_COLOR", "the badge colour must look like #3366ff")
	}
}

type RuntimeConfig struct {
	Version string `json:"version"`
}

// WindowConfig covers WIN-01..09.
type WindowConfig struct {
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	MinWidth      int    `json:"minWidth,omitempty"`
	MinHeight     int    `json:"minHeight,omitempty"`
	Zoom          int    `json:"zoom,omitempty"` // percent, 30..300
	Maximized     bool   `json:"maximized,omitempty"`
	Fullscreen    bool   `json:"fullscreen,omitempty"`
	AlwaysOnTop   bool   `json:"alwaysOnTop,omitempty"`
	TitleBarStyle string `json:"titleBarStyle,omitempty"` // "default" | "hidden" | "overlay" | "glass"
	Decorations   *bool  `json:"decorations,omitempty"`   // nil = true
	HideOnClose   bool   `json:"hideOnClose,omitempty"`
	MultiWindow   bool   `json:"multiWindow,omitempty"`
	MultiInstance bool   `json:"multiInstance,omitempty"`
	DarkMode      string `json:"darkMode,omitempty"` // "system" | "light" | "dark"
	Background    string `json:"background,omitempty"`
	// Mode is how the app shows (runtime 1.0.6+): "window" (default, empty),
	// "compact" (starts as a small floating window; View > Compact Window
	// switches back), "menubar" (lives in the menu bar / tray: its icon
	// opens the page in a panel, no Dock icon) or "sidebar" (docked to an
	// edge of the screen at full height, above other windows).
	Mode string `json:"mode,omitempty"`
	// Tabs gives the main window a tab bar (runtime 1.0.8+): links that
	// open a new window open a tab instead, tabs can be pinned, and they
	// come back on the next launch. Not for group apps, whose rail already
	// switches sites.
	Tabs bool `json:"tabs,omitempty"`
	// SidebarEdge is the screen edge of a sidebar: "right" (default, empty)
	// or "left".
	SidebarEdge string `json:"sidebarEdge,omitempty"`
	// SidebarAutoHide hides a sidebar when another app takes the focus;
	// the global shortcut or the Dock icon brings it back.
	SidebarAutoHide bool `json:"sidebarAutoHide,omitempty"`
	// PanelWidth and PanelHeight size the menu-bar panel and the compact
	// window (0 = 400 x 640); a sidebar takes PanelWidth as its width.
	PanelWidth  int `json:"panelWidth,omitempty"`
	PanelHeight int `json:"panelHeight,omitempty"`
}

// Window modes (window.mode).
const (
	ModeWindow  = "window"
	ModeCompact = "compact"
	ModeMenuBar = "menubar"
	ModeSidebar = "sidebar"
)

// Sidebar edges (window.sidebarEdge).
const (
	EdgeLeft  = "left"
	EdgeRight = "right"
)

// EffectiveSidebarEdge returns "left" or "right" (empty = "right").
func (w WindowConfig) EffectiveSidebarEdge() string {
	if w.SidebarEdge == EdgeLeft {
		return EdgeLeft
	}
	return EdgeRight
}

// Default panel size of the menu-bar panel and the compact window.
const (
	DefaultPanelWidth  = 400
	DefaultPanelHeight = 640
)

// EffectiveMode returns "window", "compact", "menubar" or "sidebar" (empty = "window").
func (w WindowConfig) EffectiveMode() string {
	if w.Mode == "" {
		return ModeWindow
	}
	return w.Mode
}

// PanelSize returns the size of the menu-bar panel and the compact window.
func (w WindowConfig) PanelSize() (int, int) {
	pw, ph := w.PanelWidth, w.PanelHeight
	if pw == 0 {
		pw = DefaultPanelWidth
	}
	if ph == 0 {
		ph = DefaultPanelHeight
	}
	return pw, ph
}

// WebViewConfig covers WEB-01..13 and CUS-01..03.
type WebViewConfig struct {
	UserAgent string `json:"userAgent,omitempty"`
	// UserAgentPreset picks a realistic browser user agent per OS:
	// "safari" (default; the runtime's platform default), "chrome", "edge",
	// "firefox", "mobile-safari" or "custom" (UserAgent is used verbatim).
	// Empty means "safari", except that a non-empty UserAgent alone means
	// "custom" (backward compatibility).
	UserAgentPreset string           `json:"userAgentPreset,omitempty"`
	Incognito       bool             `json:"incognito,omitempty"`
	RestoreLastURL  bool             `json:"restoreLastUrl,omitempty"`
	Navigation      NavigationConfig `json:"navigation"`
	Injections      []Injection      `json:"injections,omitempty"`
	FindInPage      bool             `json:"findInPage,omitempty"`
	WASM            bool             `json:"wasm,omitempty"`
	IgnoreCertErrs  bool             `json:"ignoreCertificateErrors,omitempty"` // dangerous, debug only
	DisabledKeys    []string         `json:"disabledShortcuts,omitempty"`
	DevTools        bool             `json:"devTools,omitempty"`
	// Blocking is content blocking (runtime 1.0.7+); nil blocks nothing.
	Blocking *BlockingConfig `json:"blocking,omitempty"`
}

// BlockingConfig blocks ads and trackers and hides page clutter with the
// web engine's content rules (WebKit content blockers; MyGo matches loads
// itself on Windows).
type BlockingConfig struct {
	// Trackers blocks common ad and tracker domains when a page loads them
	// from another site.
	Trackers bool `json:"trackers,omitempty"`
	// CookieBanners hides the cookie-consent banners of common consent
	// tools.
	CookieBanners bool `json:"cookieBanners,omitempty"`
	// Allow lists sites ("example.com", which takes its subdomains too)
	// where nothing is blocked or hidden, for a site that breaks.
	Allow []string `json:"allow,omitempty"`
	// Hide lists CSS selectors of elements hidden on every page of the app
	// (a promo bar, a sidebar ad).
	Hide []string `json:"hide,omitempty"`
}

// Active reports whether b blocks or hides anything.
func (b *BlockingConfig) Active() bool {
	return b != nil && (b.Trackers || b.CookieBanners || len(b.Hide) > 0)
}

// Blocking limits.
const (
	MaxBlockingAllow = 100
	MaxBlockingHide  = 50
)

var allowDomainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// NormalizeAllowDomain turns what a user types ("https://www.Example.com/x",
// "*.example.com") into the domain an allow entry holds ("example.com"),
// or "" when it is not a domain.
func NormalizeAllowDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimPrefix(strings.TrimPrefix(s, "*."), "www.")
	if !allowDomainRe.MatchString(s) {
		return ""
	}
	return s
}

func (b *BlockingConfig) validate(add func(field, code, msg string)) {
	const f = "webview.blocking."
	if len(b.Allow) > MaxBlockingAllow {
		add(f+"allow", "TOO_MANY", "list up to 100 sites")
	}
	for _, d := range b.Allow {
		if NormalizeAllowDomain(d) != d {
			add(f+"allow", "INVALID_DOMAIN", fmt.Sprintf("%q is not a domain like example.com", d))
			break
		}
	}
	if len(b.Hide) > MaxBlockingHide {
		add(f+"hide", "TOO_MANY", "list up to 50 selectors")
	}
	for _, sel := range b.Hide {
		if t := strings.TrimSpace(sel); t == "" || len(t) > 300 || strings.ContainsAny(t, "{}") {
			add(f+"hide", "INVALID_SELECTOR", fmt.Sprintf("%q is not a CSS selector", sel))
			break
		}
	}
}

// MinRuntime is the earliest runtime that has every feature c turns on.
func (c *Config) MinRuntime() string {
	switch {
	case c.Window.TitleBarStyle == "glass":
		return "1.0.9"
	case c.Window.Tabs:
		return "1.0.8"
	case c.WebView.Blocking.Active():
		return "1.0.7"
	case c.IsGroup() || c.Window.EffectiveMode() != ModeWindow:
		return "1.0.6"
	}
	return ""
}

type NavigationConfig struct {
	// SafeDomains open inside the app; everything else goes to the default browser.
	SafeDomains []string `json:"safeDomains,omitempty"`
	// InternalURLRegex: URLs matching it are forced internal.
	InternalURLRegex string `json:"internalUrlRegex,omitempty"`
	// Popups: "internal" | "external" | "block".
	Popups string `json:"popups,omitempty"`
	// ExternalByDefault: when false, all http(s) navigation stays internal.
	ExternalByDefault *bool `json:"externalByDefault,omitempty"`
}

type Injection struct {
	Type    string `json:"type"` // "css" | "js"
	Content string `json:"content,omitempty"`
	Path    string `json:"path,omitempty"`
}

// NetworkConfig covers NET-01..03. Secrets are never stored here (spec §11).
type NetworkConfig struct {
	Proxy             string `json:"proxy,omitempty"` // http://, https://, socks5://
	BasicAuthUser     string `json:"basicAuthUser,omitempty"`
	DownloadDirectory string `json:"downloadDirectory,omitempty"`
}

// NativeConfig covers NAT-01..07.
type NativeConfig struct {
	Tray           bool   `json:"tray,omitempty"`
	TrayIcon       string `json:"trayIcon,omitempty"`
	StartToTray    bool   `json:"startToTray,omitempty"`
	GlobalShortcut string `json:"globalShortcut,omitempty"` // e.g. "CmdOrCtrl+Shift+G"
	Camera         bool   `json:"camera,omitempty"`
	Microphone     bool   `json:"microphone,omitempty"`
	DragDrop       bool   `json:"dragDrop,omitempty"`
	// Notifications controls the web Notification API shim: "allow"
	// (default, because the user chose to make this app), "ask" (a native
	// prompt once per origin, remembered in the profile) or "deny".
	Notifications string `json:"notifications,omitempty"`
	// Badge controls the Dock/taskbar badge: "auto" (default; the Badging
	// API plus an unread count read from the page title) or "off".
	Badge string `json:"badge,omitempty"`
	// LinkRouting (default true) opens links that belong to another
	// installed Mypack app in that app instead of the default browser.
	LinkRouting *bool `json:"linkRouting,omitempty"`
}

// User-agent presets (webview.userAgentPreset).
const (
	UAPresetSafari       = "safari"
	UAPresetChrome       = "chrome"
	UAPresetEdge         = "edge"
	UAPresetFirefox      = "firefox"
	UAPresetMobileSafari = "mobile-safari"
	UAPresetCustom       = "custom"
)

// EffectiveUAPreset resolves the preset: an empty preset with a non-empty
// UserAgent is "custom", an empty preset alone is "safari".
func (w WebViewConfig) EffectiveUAPreset() string {
	if w.UserAgentPreset != "" {
		return w.UserAgentPreset
	}
	if strings.TrimSpace(w.UserAgent) != "" {
		return UAPresetCustom
	}
	return UAPresetSafari
}

// NotificationsMode returns "allow", "ask" or "deny" (empty = "allow").
func (n NativeConfig) NotificationsMode() string {
	if n.Notifications == "" {
		return "allow"
	}
	return n.Notifications
}

// BadgeMode returns "auto" or "off" (empty = "auto").
func (n NativeConfig) BadgeMode() string {
	if n.Badge == "" {
		return "auto"
	}
	return n.Badge
}

// LinkRoutingEnabled reports native.linkRouting (default true).
func (n NativeConfig) LinkRoutingEnabled() bool { return n.LinkRouting == nil || *n.LinkRouting }

type BuildConfig struct {
	Targets []Target `json:"targets"`
}

// Target is one os/arch/format triple. Format: "app" (macOS bundle),
// "portable" (directory), "dmg", "zip", "deb", "appimage", "exe".
type Target struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Format string `json:"format"`
}

// Key returns the runtime artifact key, e.g. "darwin-arm64".
func (t Target) Key() string { return t.OS + "-" + t.Arch }

// New returns a config with recommended defaults for a source.
func New(name string, src Source, host Target) *Config {
	c := &Config{SchemaVersion: SchemaVersion}
	c.App.Name = name
	c.App.Source = src
	c.ApplyDefaults(host)
	return c
}

// ApplyDefaults fills zero values with recommended defaults. It never changes
// an existing Identifier or ProfileID (spec §9C.2).
func (c *Config) ApplyDefaults(host Target) {
	if c.SchemaVersion == 0 {
		c.SchemaVersion = SchemaVersion
	}
	if c.App.Version == "" {
		c.App.Version = "1.0.0"
	}
	if c.App.Icon.Source == "" {
		c.App.Icon.Source = "auto"
	}
	if c.App.Identifier == "" {
		c.App.Identifier = SuggestIdentifier(c.App.Name, c.App.Source)
	}
	if c.App.ProfileID == "" {
		c.App.ProfileID = c.App.Identifier
	}
	if c.Runtime.Version == "" {
		c.Runtime.Version = DefaultRuntimeVersion
	}
	// A feature turned on in a project pinned to an older runtime raises the
	// pin, or the built app would quietly lack it. Pins are never lowered.
	if min := c.MinRuntime(); min != "" && versionRe.MatchString(c.Runtime.Version) && !RuntimeAtLeast(c.Runtime.Version, min) {
		c.Runtime.Version = min
	}
	w := &c.Window
	if w.Width == 0 {
		w.Width = 1200
	}
	if w.Height == 0 {
		w.Height = 780
	}
	if w.MinWidth == 0 {
		w.MinWidth = 480
	}
	if w.MinHeight == 0 {
		w.MinHeight = 360
	}
	if w.Zoom == 0 {
		w.Zoom = 100
	}
	if w.TitleBarStyle == "" {
		w.TitleBarStyle = "default"
	}
	if w.DarkMode == "" {
		w.DarkMode = "system"
	}
	n := &c.WebView.Navigation
	if n.Popups == "" {
		n.Popups = "internal"
	}
	if len(n.SafeDomains) == 0 && c.App.Source.Type == SourceURL {
		if h := RegistrableHost(c.App.Source.Value); h != "" {
			n.SafeDomains = []string{h}
		}
	}
	if len(c.Build.Targets) == 0 && host.OS != "" {
		c.Build.Targets = []Target{DefaultTarget(host)}
	}
}

// DefaultTarget is the conservative format for a host (spec §9C.1).
func DefaultTarget(host Target) Target {
	f := "portable"
	if host.OS == "darwin" {
		f = "app"
	}
	return Target{OS: host.OS, Arch: host.Arch, Format: f}
}

// Load reads and migrates a mypack.json file.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse decodes, migrates and strictly validates JSON (CFG-01..03).
func Parse(b []byte) (*Config, error) {
	var probe struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, &ValidationError{Issues: []Issue{{Field: "", Code: "INVALID_JSON", Message: err.Error()}}}
	}
	if probe.SchemaVersion > SchemaVersion {
		return nil, &ValidationError{Issues: []Issue{{Field: "schemaVersion", Code: "SCHEMA_TOO_NEW",
			Message: fmt.Sprintf("schema version %d is newer than supported %d", probe.SchemaVersion, SchemaVersion)}}}
	}
	b, err := Migrate(probe.SchemaVersion, b)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, &ValidationError{Issues: []Issue{{Field: "", Code: "INVALID_FIELD", Message: err.Error()}}}
	}
	return &c, nil
}

// Migrate upgrades older schema documents to the current version.
func Migrate(from int, b []byte) ([]byte, error) {
	switch from {
	case 0, 1:
		// v0 (unversioned drafts) is shape-compatible with v1.
		return b, nil
	}
	return nil, fmt.Errorf("no migration from schema %d", from)
}

// Marshal returns stable, indented JSON.
func (c *Config) Marshal() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

// Save writes the config atomically.
func (c *Config) Save(path string) error {
	b, err := c.Marshal()
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Clone deep-copies the config (used for immutable build snapshots).
func (c *Config) Clone() *Config {
	b, _ := json.Marshal(c)
	var out Config
	_ = json.Unmarshal(b, &out)
	return &out
}

// Issue is one field-level validation problem.
type Issue struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationError aggregates issues.
type ValidationError struct{ Issues []Issue }

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Issues))
	for i, is := range e.Issues {
		parts[i] = is.Field + ": " + is.Message
	}
	return "invalid config: " + strings.Join(parts, "; ")
}

var (
	identRe   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*(\.[A-Za-z0-9][A-Za-z0-9-]*)+$`)
	versionRe = regexp.MustCompile(`^\d+(\.\d+){0,3}$`)
	validOS   = map[string]bool{"darwin": true, "windows": true, "linux": true}
	validArch = map[string]bool{"amd64": true, "arm64": true, "universal": true}
	scriptRe  = regexp.MustCompile(`^[A-Za-z0-9:_][A-Za-z0-9:_-]*$`) // no leading "-": never a flag
	validPM   = map[string]bool{"npm": true, "pnpm": true, "yarn": true, "bun": true}
	validFmt  = map[string]bool{"app": true, "portable": true, "dmg": true, "zip": true, "deb": true, "appimage": true, "exe": true}
)

// Validate performs strict semantic validation (CFG-02).
func (c *Config) Validate() error {
	var is []Issue
	add := func(f, code, msg string) { is = append(is, Issue{f, code, msg}) }
	if c.SchemaVersion != SchemaVersion {
		add("schemaVersion", "SCHEMA_VERSION", fmt.Sprintf("must be %d", SchemaVersion))
	}
	if strings.TrimSpace(c.App.Name) == "" {
		add("app.name", "REQUIRED", "app name is required")
	} else if len(c.App.Name) > 64 {
		add("app.name", "TOO_LONG", "app name must be at most 64 characters")
	} else if strings.ContainsAny(c.App.Name, `/\:*?"<>|`) {
		add("app.name", "INVALID_CHARS", `app name cannot contain / \ : * ? " < > |`)
	}
	if !identRe.MatchString(c.App.Identifier) {
		add("app.identifier", "INVALID_IDENTIFIER", "identifier must look like com.example.app")
	}
	if !identRe.MatchString(c.App.ProfileID) {
		add("app.profileId", "INVALID_IDENTIFIER", "profile id must look like com.example.app")
	}
	if !versionRe.MatchString(c.App.Version) {
		add("app.version", "INVALID_VERSION", "version must look like 1.0.0")
	}
	switch c.App.Source.Type {
	case SourceURL:
		if _, err := NormalizeURL(c.App.Source.Value); err != nil {
			add("app.source.value", "INVALID_URL", err.Error())
		}
	case SourceFile, SourceDir:
		if c.App.Source.Value == "" {
			add("app.source.value", "REQUIRED", "a local path is required")
		}
	default:
		add("app.source.type", "INVALID_SOURCE", "source type must be url, file or dir")
	}
	c.validateProjectBuild(add)
	if b := c.WebView.Blocking; b != nil {
		b.validate(add)
	}
	switch c.App.Icon.Source {
	case "auto", "default":
	case "file":
		if c.App.Icon.Path == "" {
			add("app.icon.path", "REQUIRED", "icon path is required when icon source is file")
		}
	default:
		add("app.icon.source", "INVALID_ICON", "icon source must be auto, file or default")
	}
	if st := c.App.Icon.Style; st != nil {
		st.validate(add)
	}
	if !versionRe.MatchString(c.Runtime.Version) {
		add("runtime.version", "INVALID_VERSION", "runtime version must look like 1.0.0")
	}
	w := c.Window
	if w.Width < 200 || w.Width > 10000 || w.Height < 200 || w.Height > 10000 {
		add("window", "INVALID_SIZE", "width and height must be between 200 and 10000")
	}
	if w.MinWidth > w.Width || w.MinHeight > w.Height {
		add("window", "INVALID_MIN_SIZE", "minimum size cannot exceed the initial size")
	}
	if w.Zoom != 0 && (w.Zoom < 30 || w.Zoom > 300) {
		add("window.zoom", "OUT_OF_RANGE", "zoom must be between 30 and 300")
	}
	switch w.TitleBarStyle {
	case "", "default", "hidden", "overlay", "glass":
	default:
		add("window.titleBarStyle", "INVALID_ENUM", "must be default, hidden, overlay or glass")
	}
	switch w.DarkMode {
	case "", "system", "light", "dark":
	default:
		add("window.darkMode", "INVALID_ENUM", "must be system, light or dark")
	}
	c.validateGroup(add)
	if c.Window.Tabs && c.IsGroup() {
		add("window.tabs", "INVALID_TABS", "group apps switch sites with their rail and cannot have tabs")
	}
	switch w.Mode {
	case "", ModeWindow, ModeCompact, ModeMenuBar, ModeSidebar:
	default:
		add("window.mode", "INVALID_ENUM", "must be window, compact, menubar or sidebar")
	}
	switch w.SidebarEdge {
	case "", EdgeLeft, EdgeRight:
	default:
		add("window.sidebarEdge", "INVALID_ENUM", "must be left or right")
	}
	if w.PanelWidth != 0 && (w.PanelWidth < 240 || w.PanelWidth > 1600) {
		add("window.panelWidth", "OUT_OF_RANGE", "panel width must be between 240 and 1600")
	}
	if w.PanelHeight != 0 && (w.PanelHeight < 240 || w.PanelHeight > 1600) {
		add("window.panelHeight", "OUT_OF_RANGE", "panel height must be between 240 and 1600")
	}
	switch c.Native.Notifications {
	case "", "ask", "allow", "deny":
	default:
		add("native.notifications", "INVALID_ENUM", "must be ask, allow or deny")
	}
	switch c.Native.Badge {
	case "", "auto", "off":
	default:
		add("native.badge", "INVALID_ENUM", "must be auto or off")
	}
	switch c.WebView.UserAgentPreset {
	case "", UAPresetSafari, UAPresetChrome, UAPresetEdge, UAPresetFirefox, UAPresetMobileSafari:
	case UAPresetCustom:
		if strings.TrimSpace(c.WebView.UserAgent) == "" {
			add("webview.userAgent", "REQUIRED", "a user agent string is required when the preset is custom")
		}
	default:
		add("webview.userAgentPreset", "INVALID_ENUM", "must be safari, chrome, edge, firefox, mobile-safari or custom")
	}
	switch c.WebView.Navigation.Popups {
	case "", "internal", "external", "block":
	default:
		add("webview.navigation.popups", "INVALID_ENUM", "must be internal, external or block")
	}
	if r := c.WebView.Navigation.InternalURLRegex; r != "" {
		if _, err := regexp.Compile(r); err != nil {
			add("webview.navigation.internalUrlRegex", "INVALID_REGEX", err.Error())
		}
	}
	for i, d := range c.WebView.Navigation.SafeDomains {
		if d == "" || strings.ContainsAny(d, "/: ") {
			add(fmt.Sprintf("webview.navigation.safeDomains[%d]", i), "INVALID_DOMAIN", "must be a bare domain like example.com")
		}
	}
	for i, inj := range c.WebView.Injections {
		f := fmt.Sprintf("webview.injections[%d]", i)
		if inj.Type != "css" && inj.Type != "js" {
			add(f+".type", "INVALID_ENUM", "must be css or js")
		}
		if inj.Content == "" && inj.Path == "" {
			add(f, "REQUIRED", "content or path is required")
		}
	}
	if p := c.Network.Proxy; p != "" {
		u, err := url.Parse(p)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") || u.Host == "" {
			add("network.proxy", "INVALID_PROXY", "proxy must be http://, https:// or socks5://host:port")
		} else if u.User != nil {
			if _, has := u.User.Password(); has {
				add("network.proxy", "SECRET_IN_CONFIG", "do not store proxy passwords in mypack.json")
			}
		}
	}
	if len(c.Build.Targets) == 0 {
		add("build.targets", "REQUIRED", "at least one build target is required")
	}
	for i, t := range c.Build.Targets {
		f := fmt.Sprintf("build.targets[%d]", i)
		if !validOS[t.OS] {
			add(f+".os", "INVALID_ENUM", "os must be darwin, windows or linux")
		}
		if !validArch[t.Arch] || (t.Arch == "universal" && t.OS != "darwin") {
			add(f+".arch", "INVALID_ENUM", "arch must be amd64 or arm64 (universal on darwin)")
		}
		if !validFmt[t.Format] {
			add(f+".format", "INVALID_ENUM", "unknown format "+t.Format)
		}
	}
	if len(is) > 0 {
		return &ValidationError{Issues: is}
	}
	return nil
}

// validateProjectBuild checks Source.Project / Source.Build. Whether the
// project folder exists is only checked when a build runs.
func (c *Config) validateProjectBuild(add func(f, code, msg string)) {
	src := c.App.Source
	if src.Project != "" && !filepath.IsAbs(src.Project) {
		add("app.source.project", "INVALID_PATH", "project must be an absolute path")
	}
	b := src.Build
	if b == nil {
		return
	}
	if src.Type != SourceDir {
		add("app.source.build", "INVALID_SOURCE", "a project build requires a dir source")
	}
	if src.Project == "" {
		add("app.source.project", "REQUIRED", "project is required when a build is configured")
	}
	if !validPM[b.PackageManager] {
		add("app.source.build.packageManager", "INVALID_ENUM", "must be npm, pnpm, yarn or bun")
	}
	if !scriptRe.MatchString(b.Script) || len(b.Script) > 100 {
		add("app.source.build.script", "INVALID_SCRIPT", "must be a package.json script name (letters, digits, : _ -)")
	}
	if !SafeRelPath(b.OutDir) {
		add("app.source.build.outDir", "INVALID_PATH", "must be a relative folder inside the project, without ..")
	}
}

// SafeRelPath reports whether p is a non-empty relative path that stays
// inside its base (no absolute path, no ".." segment).
func SafeRelPath(p string) bool {
	if p == "" || filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) ||
		(len(p) > 1 && p[1] == ':') {
		return false
	}
	for _, seg := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." {
			return false
		}
	}
	return true
}

// NormalizeURL turns user input ("github.com", " https://x.y/ ") into an
// absolute http(s) URL, or explains why it cannot (spec §9.4 Paste).
func NormalizeURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("enter a website address")
	}
	if len(s) > 2048 {
		return "", errors.New("the address is too long")
	}
	low := strings.ToLower(s)
	for _, bad := range []string{"javascript:", "data:", "file:", "about:", "chrome:", "vbscript:"} {
		if strings.HasPrefix(low, bad) {
			return "", errors.New("only http and https websites are supported")
		}
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", errors.New("that doesn't look like a website address")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("only http and https websites are supported")
	}
	host := u.Hostname()
	if host == "" || strings.ContainsAny(host, " _") {
		return "", errors.New("that doesn't look like a website address")
	}
	if !strings.Contains(host, ".") && host != "localhost" && !strings.Contains(host, ":") {
		return "", errors.New("add a domain ending, like example.com")
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

// RegistrableHost returns a host without a leading "www.".
func RegistrableHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// SuggestIdentifier creates a stable reverse-DNS identifier.
func SuggestIdentifier(name string, src Source) string {
	slug := Slug(name)
	if slug == "" {
		slug = "app"
	}
	if src.Type == SourceURL {
		if h := RegistrableHost(src.Value); h != "" {
			parts := strings.Split(h, ".")
			for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
				parts[i], parts[j] = parts[j], parts[i]
			}
			for i := range parts {
				parts[i] = Slug(parts[i])
				if parts[i] == "" || unicode.IsDigit(rune(parts[i][0])) {
					parts[i] = "x" + parts[i]
				}
			}
			return "app.mypack." + strings.Join(parts, ".")
		}
	}
	if unicode.IsDigit(rune(slug[0])) {
		slug = "x" + slug
	}
	return "app.mypack." + slug
}

// Slug lowercases and keeps [a-z0-9-].
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// ExecutableName is a filesystem-safe binary name derived from the app name.
func ExecutableName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "App"
	}
	return out
}

// RuntimeAtLeast reports whether runtime version v (like "1.0.6") is min or
// later. A version that does not parse is never at least min.
func RuntimeAtLeast(v, min string) bool {
	a, ok1 := parseVersion(v)
	b, ok2 := parseVersion(min)
	if !ok1 || !ok2 {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return true
}

func parseVersion(v string) ([4]int, bool) {
	var out [4]int
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) == 0 || len(parts) > 4 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func (c *Config) validateGroup(add func(f, code, msg string)) {
	g := c.App.Group
	if g == nil {
		return
	}

	if n := len(g.Members); n < MinGroupMembers || n > MaxGroupMembers {
		add("app.group.members", "INVALID_GROUP", fmt.Sprintf("a group has %d to %d websites", MinGroupMembers, MaxGroupMembers))
	}
	seen := map[string]bool{}
	for i, m := range g.Members {
		f := fmt.Sprintf("app.group.members[%d]", i)
		switch {
		case !memberIDRe.MatchString(m.ID):
			add(f+".id", "INVALID_ID", "member id must be lowercase letters, digits and dashes (at most 32)")
		case seen[m.ID]:
			add(f+".id", "DUPLICATE", "member ids must be unique")
		}
		seen[m.ID] = true
		if strings.TrimSpace(m.Name) == "" {
			add(f+".name", "REQUIRED", "member name is required")
		} else if len(m.Name) > 64 {
			add(f+".name", "TOO_LONG", "member name must be at most 64 characters")
		}
		switch {
		case m.URL != "" && m.Local != "":
			add(f, "INVALID_MEMBER", "a member is a website (url) or a local folder (local), not both")
		case m.Local != "":
			if strings.Contains(filepath.ToSlash(m.Local), "..") && !filepath.IsAbs(m.Local) {
				add(f+".local", "INVALID_PATH", "a relative member path cannot leave the app")
			}
		default:
			if _, err := NormalizeURL(m.URL); err != nil {
				add(f+".url", "INVALID_URL", err.Error())
			}
		}
	}
}
