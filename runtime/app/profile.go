package app

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// ProfileDir is where one app's data lives, OUTSIDE the bundle
// (ARCHITECTURE.md): macOS ~/Library/Application Support/mypack-apps/<id>,
// Windows %APPDATA%\mypack-apps\<id>, Linux $XDG_DATA_HOME/mypack-apps/<id>.
func ProfileDir(goos, profileID string, getenv func(string) string, home string) string {
	id := filepath.Base(filepath.Clean("/" + strings.ReplaceAll(profileID, `\`, "/")))
	var base string
	switch goos {
	case "darwin":
		base = filepath.Join(home, "Library", "Application Support")
	case "windows":
		base = getenv("APPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
	default:
		base = getenv("XDG_DATA_HOME")
		if base == "" || !filepath.IsAbs(base) {
			base = filepath.Join(home, ".local", "share")
		}
	}
	return filepath.Join(base, "mypack-apps", id)
}

func defaultProfileDir(profileID string) string {
	home, _ := os.UserHomeDir()
	return ProfileDir(runtime.GOOS, profileID, os.Getenv, home)
}

// State is the small per-profile state file (state.json).
type State struct {
	LastURL string `json:"lastUrl,omitempty"`
	// Zoom is the user's zoom (percent) chosen with the zoom shortcuts;
	// ZoomBase is the configured zoom it was chosen against, so a new
	// configured zoom (rebuilt app) wins over a stale user choice.
	Zoom     int `json:"zoom,omitempty"`
	ZoomBase int `json:"zoomBase,omitempty"`
	// Tint is the last top-strip color ("r,g,b"), so the next launch starts
	// with the right color instead of flashing the default.
	Tint string `json:"tint,omitempty"`
	// Notify remembers the answers of native.notifications "ask" per origin
	// ("granted" | "denied").
	Notify map[string]string `json:"notify,omitempty"`
	// Layout is the main window's layout the user last picked in the View
	// menu: "window", "compact" or "sidebar" ("" = follow window.mode).
	// LayoutMode is the window.mode it was picked under: a rebuilt app with
	// another mode starts in that mode again.
	Layout     string `json:"layout,omitempty"`
	LayoutMode string `json:"layoutMode,omitempty"`
	// CompactBounds is where the compact window was; NormalBounds is the
	// normal window. SidebarWidth and SidebarEdge are the sidebar's.
	CompactBounds *Bounds `json:"compactBounds,omitempty"`
	NormalBounds  *Bounds `json:"normalBounds,omitempty"`
	SidebarWidth  int     `json:"sidebarWidth,omitempty"`
	SidebarEdge   string  `json:"sidebarEdge,omitempty"`
	// GroupActive is the group member shown last (its id).
	GroupActive string `json:"groupActive,omitempty"`
}

// Bounds is a window rectangle in screen coordinates.
type Bounds struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Store persists State atomically.
type Store struct {
	mu   sync.Mutex
	path string
	s    State
}

// OpenStore loads (or starts) the state file in dir.
func OpenStore(dir string) *Store {
	st := &Store{path: filepath.Join(dir, "state.json")}
	if b, err := os.ReadFile(st.path); err == nil {
		_ = json.Unmarshal(b, &st.s)
	}
	return st
}

// Get returns a copy of the state.
func (s *Store) Get() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.s
	if s.s.Notify != nil {
		out.Notify = make(map[string]string, len(s.s.Notify))
		for k, v := range s.s.Notify {
			out.Notify[k] = v
		}
	}
	return out
}

// Update mutates and saves the state.
func (s *Store) Update(fn func(*State)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	before, _ := json.Marshal(s.s)
	fn(&s.s)
	b, err := json.Marshal(s.s)
	if err != nil || string(b) == string(before) {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, s.path)
	}
}

// SameOrigin reports whether two URLs share scheme, host and port.
func SameOrigin(a, b string) bool {
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	if err1 != nil || err2 != nil || ua.Host == "" || ub.Host == "" {
		return false
	}
	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		if strings.EqualFold(u.Scheme, "https") {
			return "443"
		}
		return "80"
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) && strings.EqualFold(ua.Hostname(), ub.Hostname()) && port(ua) == port(ub)
}

// ChooseStartURL applies restoreLastUrl: the saved URL is used only when it
// is an http(s) URL on the same origin as the configured source.
func ChooseStartURL(source, last string, restore bool) string {
	if restore && last != "" && SameOrigin(source, last) {
		return last
	}
	return source
}

// EffectiveZoom resolves the zoom percent from the config and saved state.
func EffectiveZoom(configured int, st State) int {
	if configured == 0 {
		configured = 100
	}
	if st.Zoom != 0 && st.ZoomBase == configured {
		return clampZoom(st.Zoom)
	}
	return clampZoom(configured)
}

func clampZoom(z int) int {
	if z < 30 {
		return 30
	}
	if z > 300 {
		return 300
	}
	return z
}
