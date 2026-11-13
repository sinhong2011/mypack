// Package routing implements "open links in the right app": the Builder keeps
// a routes file listing the hosts every installed MyPack app owns, and a
// generated app consults it when the user clicks a link that leaves the app,
// handing the URL to the MyPack app that owns the host instead of the default
// browser. Only MyPack apps are targets; the system browser registration is
// never touched.
package routing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"
)

// EnvFile overrides the routes file location (tests, portable setups).
const EnvFile = "MYPACK_ROUTES_FILE"

// Entry is one installed MyPack app.
type Entry struct {
	// Identifier is the app's bundle id (config app.identifier).
	Identifier string `json:"identifier"`
	// BundlePath is the installed .app bundle (macOS) or app folder.
	BundlePath string `json:"bundlePath"`
	// Executable is the app binary (Windows/Linux); optional, guessed
	// from BundlePath when empty.
	Executable string `json:"executable,omitempty"`
	// SourceHost is the host of the app's start URL; a link to exactly
	// this host beats a broader match by another app.
	SourceHost string `json:"sourceHost,omitempty"`
	// Hosts are the domains the app owns: safe domains, the source host
	// and their registrable domains. A subdomain of an entry matches.
	Hosts []string `json:"hosts"`
}

// FilePath returns the routes file: $MYPACK_ROUTES_FILE, else
// <UserConfigDir>/mypack-apps/routes.json.
func FilePath() string {
	if p := os.Getenv(EnvFile); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "mypack-apps", "routes.json")
}

// NormalizeHost lower-cases a host and drops a trailing dot, port and
// "*." prefix. Returns "" for anything that is not a bare host name.
func NormalizeHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.TrimPrefix(h, "*.")
	if u, err := url.Parse("//" + h); err == nil && u.Hostname() != "" {
		h = u.Hostname()
	}
	h = strings.TrimSuffix(h, ".")
	if h == "" || strings.ContainsAny(h, "/ \t\r\n") {
		return ""
	}
	return h
}

// ExpandHosts normalizes domains and adds their registrable domain (eTLD+1),
// so a sign-in or app sibling host of the same site matches too. Shared
// hosting suffixes (github.io, ...) are never widened.
func ExpandHosts(domains ...string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(h string) {
		if h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	for _, d := range domains {
		h := NormalizeHost(d)
		if h == "" {
			continue
		}
		add(h)
		if site, err := publicsuffix.EffectiveTLDPlusOne(h); err == nil && site != h {
			add(site)
		}
	}
	return out
}

// Marshal returns the canonical file content for entries: sorted by
// identifier, duplicates dropped, hosts normalized.
func Marshal(entries []Entry) ([]byte, error) {
	byID := map[string]Entry{}
	for _, e := range entries {
		e.Identifier = strings.TrimSpace(e.Identifier)
		if e.Identifier == "" || e.BundlePath == "" {
			continue
		}
		e.SourceHost = NormalizeHost(e.SourceHost)
		var hs []string
		seen := map[string]bool{}
		for _, h := range e.Hosts {
			if h = NormalizeHost(h); h != "" && !seen[h] {
				seen[h] = true
				hs = append(hs, h)
			}
		}
		e.Hosts = hs
		if len(hs) == 0 && e.SourceHost == "" {
			continue
		}
		if e.Hosts == nil {
			e.Hosts = []string{}
		}
		byID[e.Identifier] = e
	}
	out := make([]Entry, 0, len(byID))
	for _, e := range byID {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Identifier < out[j].Identifier })
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Write stores entries atomically at path. The file is left untouched (no
// mtime churn for readers) when its content would not change.
func Write(path string, entries []Entry) error {
	b, err := Marshal(entries)
	if err != nil {
		return err
	}
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Read parses a routes file. A missing file is an empty table.
func Read(path string) ([]Entry, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var es []Entry
	if err := json.Unmarshal(b, &es); err != nil {
		return nil, fmt.Errorf("routes file %s: %w", path, err)
	}
	return es, nil
}

func hostMatch(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// Match returns the app that owns host, never selfID. The most specific
// claim wins: a link to exactly an app's start host beats a broader
// registrable-domain claim of another app; a longer matching domain beats a
// shorter one. When two different apps claim equally, nothing is returned
// (the link goes to the browser rather than to a guess).
func Match(entries []Entry, host, selfID string, exists func(path string) bool) (Entry, bool) {
	host = NormalizeHost(host)
	if host == "" {
		return Entry{}, false
	}
	var best Entry
	bestScore, tie := 0, false
	for _, e := range entries {
		if e.Identifier == selfID || e.Identifier == "" {
			continue
		}
		if exists != nil && !exists(e.BundlePath) {
			continue
		}
		score := 0
		consider := func(d string) {
			d = NormalizeHost(d)
			if d == "" || !hostMatch(host, d) {
				return
			}
			s := len(d) * 2
			if e.SourceHost != "" && host == e.SourceHost {
				s += 10000
			}
			if s > score {
				score = s
			}
		}
		for _, d := range e.Hosts {
			consider(d)
		}
		consider(e.SourceHost)
		switch {
		case score == 0:
		case score > bestScore:
			best, bestScore, tie = e, score, false
		case score == bestScore && e.Identifier != best.Identifier:
			tie = true
		}
	}
	if bestScore == 0 || tie {
		return Entry{}, false
	}
	return best, true
}

// Table is a lazily read, mtime-checked view of the routes file.
type Table struct {
	Path   string // "" = FilePath()
	Exists func(string) bool

	mu      sync.Mutex
	mtime   time.Time
	size    int64
	loaded  bool
	entries []Entry
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func (t *Table) path() string {
	if t.Path != "" {
		return t.Path
	}
	return FilePath()
}

// Entries returns the current entries, re-reading the file only when its
// modification time or size changed.
func (t *Table) Entries() []Entry {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.path()
	fi, err := os.Stat(p)
	if err != nil {
		t.entries, t.loaded, t.mtime, t.size = nil, false, time.Time{}, 0
		return nil
	}
	if t.loaded && fi.ModTime().Equal(t.mtime) && fi.Size() == t.size {
		return t.entries
	}
	es, err := Read(p)
	if err != nil {
		es = nil
	}
	t.entries, t.loaded, t.mtime, t.size = es, true, fi.ModTime(), fi.Size()
	return es
}

// Lookup finds the MyPack app owning host (never selfID).
func (t *Table) Lookup(host, selfID string) (Entry, bool) {
	ex := t.Exists
	if ex == nil {
		ex = fileExists
	}
	return Match(t.Entries(), host, selfID, ex)
}

// Command builds the process that hands rawURL to the app e.
//
//	macOS:          open -a <bundlePath> <url>   (or open -b <identifier> <url>)
//	Windows/Linux:  <app executable> <url>       (the running instance gets it
//	                                              through the single-instance lock)
func Command(goos string, e Entry, rawURL string) (name string, args []string, err error) {
	u, perr := url.Parse(rawURL)
	if perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.ContainsAny(rawURL, " \t\r\n\x00") {
		return "", nil, fmt.Errorf("routing: refusing to hand %q to an app", rawURL)
	}
	switch goos {
	case "darwin":
		if e.BundlePath != "" {
			return "open", []string{"-a", e.BundlePath, rawURL}, nil
		}
		if e.Identifier != "" {
			return "open", []string{"-b", e.Identifier, rawURL}, nil
		}
		return "", nil, errors.New("routing: entry has neither bundlePath nor identifier")
	default:
		exe := e.Executable
		if exe == "" {
			exe = GuessExecutable(goos, e.BundlePath)
		}
		if exe == "" {
			return "", nil, fmt.Errorf("routing: no executable found for %s", e.Identifier)
		}
		return exe, []string{rawURL}, nil
	}
}

// GuessExecutable finds the app binary inside a Windows/Linux app folder.
func GuessExecutable(goos, dir string) string {
	if dir == "" {
		return ""
	}
	base := filepath.Base(dir)
	if goos == "windows" {
		if p := filepath.Join(dir, base+".exe"); fileExists(p) {
			return p
		}
		m, _ := filepath.Glob(filepath.Join(dir, "*.exe"))
		sort.Strings(m)
		for _, p := range m {
			if !strings.Contains(strings.ToLower(filepath.Base(p)), "unins") {
				return p
			}
		}
		return ""
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, de := range ents {
		if de.IsDir() || filepath.Ext(de.Name()) != "" {
			continue
		}
		if fi, err := de.Info(); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0 {
			return filepath.Join(dir, de.Name())
		}
	}
	return ""
}

// Start launches name with args detached (it must not outlive-block us).
var Start = func(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// Open hands rawURL to the app e using the host platform's mechanism.
func Open(goos string, e Entry, rawURL string) error {
	name, args, err := Command(goos, e, rawURL)
	if err != nil {
		return err
	}
	return Start(name, args...)
}
