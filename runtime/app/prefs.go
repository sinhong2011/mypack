package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"mypack.dev/mypack/pkg/config"
)

// prefsFile is the user-edited overlay of mypack.json, kept in the profile
// so a built app can change its settings without rewriting the bundle.
const prefsFile = "settings.json"

// loadUserPrefs overlays <profile>/settings.json onto cfg. Identity, source,
// runtime and packaging stay as they were built; a broken file is ignored.
func loadUserPrefs(cfg *config.Config, dir string) {
	b, err := os.ReadFile(filepath.Join(dir, prefsFile))
	if err != nil || len(bytes.TrimSpace(b)) == 0 {
		return
	}
	if err := applyUserPrefs(cfg, b); err != nil {
		fmt.Fprintf(os.Stderr, "mypack-runtime: settings.json: %v\n", err)
	}
}

func saveUserPrefs(bundled, current *config.Config, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, prefsFile)
	overlay, err := prefsOverlay(bundled, current)
	if err != nil {
		return err
	}
	if len(overlay) == 0 {
		_ = os.Remove(path)
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(overlay, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// applyUserPrefs merges a settings.json overlay into cfg. Fields the user
// must not change in a running app (identity, source, runtime, build) are
// restored afterwards, even if the file names them.
func applyUserPrefs(cfg *config.Config, raw []byte) error {
	if cfg == nil {
		return fmt.Errorf("no configuration")
	}
	snap := freezeIdentity(cfg)
	if err := json.Unmarshal(raw, cfg); err != nil {
		return err
	}
	restoreIdentity(cfg, snap)
	return nil
}

type frozenIdentity struct {
	schema  int
	name    string
	ident   string
	profile string
	version string
	source  config.Source
	icon    config.Icon
	group   *config.GroupConfig
	runtime config.RuntimeConfig
	build   config.BuildConfig
}

func freezeIdentity(cfg *config.Config) frozenIdentity {
	g := cfg.App.Group
	if g != nil {
		cp := *g
		cp.Members = append([]config.GroupMember(nil), g.Members...)
		g = &cp
	}
	return frozenIdentity{
		schema:  cfg.SchemaVersion,
		name:    cfg.App.Name,
		ident:   cfg.App.Identifier,
		profile: cfg.App.ProfileID,
		version: cfg.App.Version,
		source:  cfg.App.Source,
		icon:    cfg.App.Icon,
		group:   g,
		runtime: cfg.Runtime,
		build:   cfg.Build,
	}
}

func restoreIdentity(cfg *config.Config, f frozenIdentity) {
	cfg.SchemaVersion = f.schema
	cfg.App.Name = f.name
	cfg.App.Identifier = f.ident
	cfg.App.ProfileID = f.profile
	cfg.App.Version = f.version
	cfg.App.Source = f.source
	cfg.App.Icon = f.icon
	cfg.App.Group = f.group
	cfg.Runtime = f.runtime
	cfg.Build = f.build
}

// prefsOverlay is the JSON of fields current changed from bundled, or nil
// when they match. json.Unmarshal of the result onto a clone of bundled
// reconstructs current for the overlayable groups.
func prefsOverlay(bundled, current *config.Config) ([]byte, error) {
	d := diffMaps(overlayableMap(bundled), overlayableMap(current))
	if len(d) == 0 {
		return nil, nil
	}
	return json.MarshalIndent(d, "", "  ")
}

func overlayableMap(c *config.Config) map[string]any {
	if c == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}
	}
	out := map[string]any{}
	if app, ok := m["app"].(map[string]any); ok {
		a := map[string]any{}
		if t, ok := app["title"]; ok {
			a["title"] = t
		}
		if len(a) > 0 {
			out["app"] = a
		}
	}
	for _, k := range []string{"window", "webview", "network", "native"} {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}

func diffMaps(base, next map[string]any) map[string]any {
	out := map[string]any{}
	keys := map[string]struct{}{}
	for k := range base {
		keys[k] = struct{}{}
	}
	for k := range next {
		keys[k] = struct{}{}
	}
	for k := range keys {
		bv, bOk := base[k]
		nv, nOk := next[k]
		switch {
		case !nOk:
			out[k] = zeroJSON(bv)
		case !bOk:
			out[k] = nv
		default:
			bm, bMap := bv.(map[string]any)
			nm, nMap := nv.(map[string]any)
			if bMap && nMap {
				if d := diffMaps(bm, nm); len(d) > 0 {
					out[k] = d
				}
				continue
			}
			if !jsonEqual(bv, nv) {
				out[k] = nv
			}
		}
	}
	return out
}

func jsonEqual(a, b any) bool { return reflect.DeepEqual(a, b) }

func zeroJSON(v any) any {
	switch v.(type) {
	case bool:
		return false
	case float64:
		return float64(0)
	case string:
		return ""
	case []any:
		return []any{}
	case map[string]any:
		return map[string]any{}
	}
	return nil
}
