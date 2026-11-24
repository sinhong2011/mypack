package app

import (
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"mypack.dev/mypack/runtime/userscript"
)

// Settings → Extensions: the extensions installed in MyPack (the shared
// userscript store) as this app sees them, and which of them run here.
// MyPack owns the store (install, turn off everywhere, choose apps); this
// app only keeps its own list of extensions it does not run, in
// <profile>/extensions.json, and never writes the store.

const extPrefsFile = "extensions.json"

type extPrefs struct {
	Disabled []string `json:"disabled,omitempty"` // extension ids this app does not run
}

func loadExtPrefs(dir string) extPrefs {
	var p extPrefs
	if dir == "" {
		return p
	}
	b, err := os.ReadFile(filepath.Join(dir, extPrefsFile))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("mypack-runtime: %s: %v", extPrefsFile, err)
		}
		return p
	}
	if err := json.Unmarshal(b, &p); err != nil {
		log.Printf("mypack-runtime: %s: %v", extPrefsFile, err)
	}
	return p
}

func saveExtPrefs(dir string, p extPrefs) error {
	if dir == "" {
		return errors.New("no profile folder")
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, extPrefsFile+".tmp")
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, extPrefsFile))
}

func (p extPrefs) off(id string) bool {
	for _, d := range p.Disabled {
		if d == id {
			return true
		}
	}
	return false
}

// settingsExt is one extension in Settings → Extensions.
type settingsExt struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	Sites       []string `json:"sites"`
	Edited      bool     `json:"edited,omitempty"`
	// Enabled is MyPack's on/off for every app; InScope is whether MyPack
	// lets it run in this app; Here is this app's own choice.
	Enabled bool `json:"enabled"`
	InScope bool `json:"inScope"`
	Here    bool `json:"here"`
	// ForSite: it matches a page of this app's site(s).
	ForSite bool `json:"forSite"`
	// Running: loaded in the pages open now (at launch).
	Running bool `json:"running"`
}

// appURLs are the addresses this app shows: its source, or a group's sites.
func (a *App) appURLs() []string {
	cfg := a.cfg
	var out []string
	if cfg.IsGroup() {
		for _, m := range cfg.App.Group.Members {
			if !m.IsLocal() && m.URL != "" {
				out = append(out, m.URL)
			}
		}
		return out
	}
	if v := cfg.App.Source.Value; strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		out = append(out, v)
	}
	if a.home != "" && a.home != cfg.App.Source.Value {
		out = append(out, a.home)
	}
	return out
}

// extensionsBoot lists the store's extensions for Settings, and whether a
// restart would change what runs.
func (a *App) extensionsBoot() (list []settingsExt, pending bool) {
	st := userscript.Store{Dir: userscript.Dir()}
	ix, err := st.Read()
	if err != nil {
		log.Printf("mypack-runtime: settings: extensions: %v", err)
		return nil, false
	}
	prefs := loadExtPrefs(a.prefsDir)
	urls := a.appURLs()
	for _, e := range ix.Scripts {
		m, err := st.ReadMeta(e.ID)
		if err != nil {
			continue
		}
		inc, exc, _ := m.Rules()
		forSite := false
		for _, u := range urls {
			if userscript.Matches(inc, exc, u) || siteHostMatch(m.Sites(), u) {
				forSite = true
				break
			}
		}
		s := settingsExt{ID: e.ID, Name: e.Name, Version: e.Version, Description: e.Description, Sites: m.Sites(),
			Edited: e.Edited, Enabled: e.Enabled, InScope: e.RunsIn(a.cfg.App.Identifier), Here: !prefs.off(e.ID),
			ForSite: forSite, Running: a.extRunning[e.ID]}
		if s.Enabled && s.InScope && s.Here != s.Running {
			pending = true
		}
		if s.Running && !(s.Enabled && s.InScope) {
			pending = true
		}
		list = append(list, s)
	}
	return list, pending
}

// siteHostMatch: one of the script's sites ("x.com", "*.reddit.com", "*")
// is rawURL's host, so it runs somewhere on this app's site even when the
// start page itself is not one of its pages.
func siteHostMatch(sites []string, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, s := range sites {
		s = strings.ToLower(s)
		if i := strings.LastIndex(s, ":"); i > 0 && !strings.Contains(s[i:], "]") {
			s = s[:i]
		}
		switch {
		case s == "*":
			return true
		case strings.HasPrefix(s, "*."):
			if d := s[2:]; host == d || strings.HasSuffix(host, "."+d) {
				return true
			}
		case s == host:
			return true
		}
	}
	return false
}

// setExtHere turns an extension on or off in this app (next launch).
func (a *App) setExtHere(id string, on bool) error {
	p := loadExtPrefs(a.prefsDir)
	var next []string
	for _, d := range p.Disabled {
		if d != id {
			next = append(next, d)
		}
	}
	if !on {
		next = append(next, id)
	}
	p.Disabled = next
	return saveExtPrefs(a.prefsDir, p)
}
