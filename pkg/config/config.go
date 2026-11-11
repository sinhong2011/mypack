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
	"strings"
	"unicode"
)

// SchemaVersion is the current mypack.json schema version.
const SchemaVersion = 1

// DefaultRuntimeVersion is the runtime version new projects pin (ADR-005).
const DefaultRuntimeVersion = "1.0.5"

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
	TitleBarStyle string `json:"titleBarStyle,omitempty"` // "default" | "hidden" | "overlay"
	Decorations   *bool  `json:"decorations,omitempty"`   // nil = true
	HideOnClose   bool   `json:"hideOnClose,omitempty"`
	MultiWindow   bool   `json:"multiWindow,omitempty"`
	MultiInstance bool   `json:"multiInstance,omitempty"`
	DarkMode      string `json:"darkMode,omitempty"` // "system" | "light" | "dark"
	Background    string `json:"background,omitempty"`
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
	switch c.App.Icon.Source {
	case "auto", "default":
	case "file":
		if c.App.Icon.Path == "" {
			add("app.icon.path", "REQUIRED", "icon path is required when icon source is file")
		}
	default:
		add("app.icon.source", "INVALID_ICON", "icon source must be auto, file or default")
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
	case "", "default", "hidden", "overlay":
	default:
		add("window.titleBarStyle", "INVALID_ENUM", "must be default, hidden or overlay")
	}
	switch w.DarkMode {
	case "", "system", "light", "dark":
	default:
		add("window.darkMode", "INVALID_ENUM", "must be system, light or dark")
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
