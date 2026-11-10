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
	return s.s
}

// Update mutates and saves the state.
func (s *Store) Update(fn func(*State)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.s
	fn(&s.s)
	if s.s == old {
		return
	}
	b, err := json.Marshal(s.s)
	if err != nil {
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
