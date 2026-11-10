package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ImportPake converts a Pake configuration into a mypack Config (spec §11
// "Pake Config Import: map the semantics and list unsupported fields").
//
// Two shapes are accepted and may even be mixed:
//
//   - pake.json, as shipped in Pake projects: a "windows" array whose first
//     entry carries url, width, hide_title_bar … plus top-level user_agent,
//     system_tray, inject, proxy_url, multi_instance …
//   - Pake CLI style flat keys: name, url, width, hideTitleBar, userAgent,
//     showSystemTray, safeDomain, proxyUrl …
//
// Keys are compared case-insensitively with underscores removed, so
// "hide_title_bar" and "hideTitleBar" are the same field.
//
// The returned warnings list every field that could not be mapped (or was
// only partially mapped); the config is returned even when warnings exist.
func ImportPake(b []byte) (*Config, []string, error) {
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, nil, &ValidationError{Issues: []Issue{{Code: "INVALID_JSON", Message: err.Error()}}}
	}
	imp := &pakeImport{cfg: &Config{SchemaVersion: SchemaVersion}}

	// Process top-level keys in a stable order so warnings are deterministic.
	keys := make([]string, 0, len(root))
	for k := range root {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := root[k]
		if pakeKey(k) == "windows" {
			imp.windows(v)
			continue
		}
		imp.field(k, v)
	}
	if err := imp.finish(); err != nil {
		return nil, nil, err
	}
	return imp.cfg, imp.warnings, nil
}

// ImportPakeFile reads and converts a Pake config file.
func ImportPakeFile(path string) (*Config, []string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return ImportPake(b)
}

// IsPakeJSON reports whether a JSON document looks like a Pake config rather
// than a mypack.json (which always has app/schemaVersion).
func IsPakeJSON(b []byte) bool {
	var root map[string]any
	if json.Unmarshal(b, &root) != nil {
		return false
	}
	if _, ok := root["schemaVersion"]; ok {
		return false
	}
	if _, ok := root["app"]; ok {
		return false
	}
	for k := range root {
		switch pakeKey(k) {
		case "windows", "url", "systemtray", "useragent", "proxyurl", "hidetitlebar", "multiinstance":
			return true
		}
	}
	return false
}

type pakeImport struct {
	cfg      *Config
	warnings []string
	url      string
	local    bool // url_type == "local"
	name     string
	title    string
}

func pakeKey(k string) string {
	return strings.ToLower(strings.ReplaceAll(k, "_", ""))
}

func (p *pakeImport) warn(format string, a ...any) {
	p.warnings = append(p.warnings, fmt.Sprintf(format, a...))
}

func (p *pakeImport) windows(v any) {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		p.warn("windows: expected a non-empty array")
		return
	}
	if len(arr) > 1 {
		p.warn("windows: only the first window is imported (%d ignored)", len(arr)-1)
	}
	w, ok := arr[0].(map[string]any)
	if !ok {
		p.warn("windows[0]: expected an object")
		return
	}
	keys := make([]string, 0, len(w))
	for k := range w {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p.field(k, w[k])
	}
}

func asBool(v any) (bool, bool) { b, ok := v.(bool); return b, ok }
func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}
func asInt(v any) (int, bool) {
	f, ok := v.(float64)
	return int(f), ok
}

// field maps one Pake key (from either a window object or the root).
func (p *pakeImport) field(rawKey string, v any) {
	c := p.cfg
	bad := func() { p.warn("%s: unexpected value type, ignored", rawKey) }
	boolField := func(dst *bool) {
		if b, ok := asBool(v); ok {
			*dst = b
		} else {
			bad()
		}
	}
	intField := func(dst *int) {
		if n, ok := asInt(v); ok {
			*dst = n
		} else {
			bad()
		}
	}
	strField := func(dst *string) {
		if s, ok := asString(v); ok {
			*dst = s
		} else {
			bad()
		}
	}

	switch pakeKey(rawKey) {
	case "name":
		strField(&p.name)
	case "url":
		strField(&p.url)
	case "urltype":
		if s, _ := asString(v); s == "local" {
			p.local = true
		}
	case "width":
		intField(&c.Window.Width)
	case "height":
		intField(&c.Window.Height)
	case "minwidth":
		intField(&c.Window.MinWidth)
	case "minheight":
		intField(&c.Window.MinHeight)
	case "zoom":
		intField(&c.Window.Zoom)
	case "fullscreen":
		boolField(&c.Window.Fullscreen)
	case "alwaysontop":
		boolField(&c.Window.AlwaysOnTop)
	case "hidetitlebar":
		if b, ok := asBool(v); ok {
			if b {
				c.Window.TitleBarStyle = "overlay"
			}
		} else {
			bad()
		}
	case "resizable":
		if b, ok := asBool(v); ok && !b {
			p.warn("resizable=false: fixed-size windows are not supported")
		}
	case "darkmode":
		if b, ok := asBool(v); ok {
			if b {
				c.Window.DarkMode = "dark"
			}
		} else {
			bad()
		}
	case "activationshortcut":
		if s, ok := asString(v); ok {
			c.Native.GlobalShortcut = strings.ReplaceAll(s, "CmdOrControl", "CmdOrCtrl")
		} else {
			bad()
		}
	case "disabledwebshortcuts":
		if b, ok := asBool(v); ok && b {
			p.warn("disabled_web_shortcuts: mypack disables individual shortcuts (webview.disabledShortcuts); the all-or-nothing flag was not mapped")
		}
	case "hideonclose":
		boolField(&c.Window.HideOnClose)
	case "incognito":
		boolField(&c.WebView.Incognito)
	case "title":
		strField(&p.title)
	case "enablewasm":
		boolField(&c.WebView.WASM)
	case "enabledragdrop":
		boolField(&c.Native.DragDrop)
	case "starttotray":
		if b, ok := asBool(v); ok {
			c.Native.StartToTray = b
			if b {
				c.Native.Tray = true
			}
		} else {
			bad()
		}
	case "forceinternalnavigation":
		if b, ok := asBool(v); ok {
			if b {
				f := false
				c.WebView.Navigation.ExternalByDefault = &f
			}
		} else {
			bad()
		}
	case "internalurlregex":
		strField(&c.WebView.Navigation.InternalURLRegex)
	case "ignorecertificateerrors":
		boolField(&c.WebView.IgnoreCertErrs)
	case "newwindow":
		if b, ok := asBool(v); ok && b {
			c.WebView.Navigation.Popups = "internal"
		}
	case "multiinstance":
		boolField(&c.Window.MultiInstance)
	case "multiwindow":
		boolField(&c.Window.MultiWindow)
	case "proxyurl":
		strField(&c.Network.Proxy)
	case "debug":
		boolField(&c.WebView.DevTools)
	case "icon":
		if s, ok := asString(v); ok && s != "" {
			c.App.Icon = Icon{Source: "file", Path: s}
		}
	case "useragent":
		p.userAgent(v)
	case "showsystemtray":
		boolField(&c.Native.Tray)
	case "systemtray":
		p.systemTray(v)
	case "systemtraypath":
		if s, _ := asString(v); s != "" {
			p.warn("system_tray_path: tray icon paths from Pake projects are not imported")
		}
	case "systemtrayicon":
		if s, _ := asString(v); s != "" {
			p.warn("systemTrayIcon: tray icon paths from Pake projects are not imported")
		}
	case "safedomain":
		switch d := v.(type) {
		case string:
			for _, s := range strings.Split(d, ",") {
				if s = strings.TrimSpace(s); s != "" {
					c.WebView.Navigation.SafeDomains = append(c.WebView.Navigation.SafeDomains, s)
				}
			}
		case []any:
			for _, e := range d {
				if s, ok := e.(string); ok && s != "" {
					c.WebView.Navigation.SafeDomains = append(c.WebView.Navigation.SafeDomains, s)
				}
			}
		default:
			bad()
		}
	case "inject":
		p.inject(v)
	case "targets", "targetsuffix":
		p.warn("%s: packaging targets are chosen in mypack build settings", rawKey)
	default:
		p.warn("%s: unsupported Pake field, ignored", rawKey)
	}
}

func (p *pakeImport) userAgent(v any) {
	switch ua := v.(type) {
	case string:
		p.cfg.WebView.UserAgent = ua
	case map[string]any:
		key := map[string]string{"darwin": "macos", "linux": "linux", "windows": "windows"}[runtime.GOOS]
		if s, _ := ua[key].(string); s != "" {
			p.cfg.WebView.UserAgent = s
		}
		for k, other := range ua {
			if k != key {
				if s, _ := other.(string); s != "" {
					p.warn("user_agent.%s: mypack keeps a single user agent; only the current platform's value was imported", k)
				}
			}
		}
	default:
		p.warn("user_agent: unexpected value type, ignored")
	}
}

func (p *pakeImport) systemTray(v any) {
	switch t := v.(type) {
	case bool:
		p.cfg.Native.Tray = t
	case map[string]any:
		key := map[string]string{"darwin": "macos", "linux": "linux", "windows": "windows"}[runtime.GOOS]
		if b, ok := t[key].(bool); ok {
			p.cfg.Native.Tray = b
		}
	default:
		p.warn("system_tray: unexpected value type, ignored")
	}
}

func (p *pakeImport) inject(v any) {
	var files []string
	switch in := v.(type) {
	case string:
		for _, s := range strings.Split(in, ",") {
			if s = strings.TrimSpace(s); s != "" {
				files = append(files, s)
			}
		}
	case []any:
		for _, e := range in {
			if s, ok := e.(string); ok && s != "" {
				files = append(files, s)
			}
		}
	default:
		p.warn("inject: unexpected value type, ignored")
		return
	}
	for _, f := range files {
		switch strings.ToLower(filepath.Ext(f)) {
		case ".css":
			p.cfg.WebView.Injections = append(p.cfg.WebView.Injections, Injection{Type: "css", Path: f})
		case ".js":
			p.cfg.WebView.Injections = append(p.cfg.WebView.Injections, Injection{Type: "js", Path: f})
		default:
			p.warn("inject: %q is neither .css nor .js, ignored", f)
		}
	}
}

// finish resolves the source and name, then applies defaults.
func (p *pakeImport) finish() error {
	c := p.cfg
	if strings.TrimSpace(p.url) == "" {
		return &ValidationError{Issues: []Issue{{Field: "url", Code: "REQUIRED", Message: "the Pake config has no url"}}}
	}
	if p.local {
		typ := SourceFile
		if st, err := os.Stat(p.url); err == nil && st.IsDir() {
			typ = SourceDir
		} else if err != nil && filepath.Ext(p.url) == "" {
			typ = SourceDir
		}
		c.App.Source = Source{Type: typ, Value: p.url}
	} else {
		u, err := NormalizeURL(p.url)
		if err != nil {
			return &ValidationError{Issues: []Issue{{Field: "url", Code: "INVALID_URL", Message: err.Error()}}}
		}
		c.App.Source = Source{Type: SourceURL, Value: u}
	}
	c.App.Name = firstNonEmpty(p.name, p.title)
	if c.App.Name == "" {
		c.App.Name = nameFromSource(c.App.Source)
	}
	if p.title != "" && p.title != c.App.Name {
		c.App.Title = p.title
	}
	c.ApplyDefaults(Target{OS: runtime.GOOS, Arch: runtime.GOARCH})
	return nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// nameFromSource guesses an app name: "www.github.com" -> "Github".
func nameFromSource(src Source) string {
	if src.Type == SourceURL {
		if h := RegistrableHost(src.Value); h != "" {
			first := strings.Split(h, ".")[0]
			if first != "" {
				return strings.ToUpper(first[:1]) + first[1:]
			}
		}
	}
	base := strings.TrimSuffix(filepath.Base(src.Value), filepath.Ext(src.Value))
	if base == "" || base == "." || base == "/" {
		return "App"
	}
	return base
}
