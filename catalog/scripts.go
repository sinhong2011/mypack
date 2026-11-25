package catalog

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// The Extensions hub: the userscripts in HubRepo, where every change is a
// reviewed pull request. After a merge, that repository's CI publishes the
// list (index.json on its hub branch): each entry points at one exact file
// (its address at the merged commit) and its SHA-256, so a script cannot
// change after review: the app refuses a download whose hash differs.
// Like Discover, the list ships in every build (scripts.json, a copy of
// the published one) and is refreshed from there.

//go:embed scripts.json
var bundledScripts []byte

// Script is one hub entry.
type Script struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Author      string   `json:"author,omitempty"`
	Sites       []string `json:"sites"` // hosts it runs on, shown on the card
	URL         string   `json:"url"`   // the script file, pinned to a commit
	SHA256      string   `json:"sha256"`
	Homepage    string   `json:"homepage,omitempty"`
}

// Scripts is scripts.json.
type Scripts struct {
	Version int      `json:"version"`
	Scripts []Script `json:"scripts"`
}

// HubRepo is the repository the hub's extensions come from; its README
// says how they are reviewed and published.
const HubRepo = "https://github.com/sinhong2011/mypack-extensions"

// ScriptsRemoteURL is where the latest hub list is fetched from ("" turns
// remote updates off): the list HubRepo's CI publishes after each merge.
var ScriptsRemoteURL = "https://raw.githubusercontent.com/sinhong2011/mypack-extensions/hub/index.json"

const (
	maxScripts     = 300
	maxDescLen     = 200
	maxScriptSites = 20
)

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

// CheckScript reports why a hub entry is not acceptable, or nil.
func CheckScript(s Script) error {
	if s.Name == "" || utf8.RuneCountInString(s.Name) > maxNameLen {
		return fmt.Errorf("name must be 1 to %d characters", maxNameLen)
	}
	if utf8.RuneCountInString(s.Description) > maxDescLen {
		return fmt.Errorf("description longer than %d characters", maxDescLen)
	}
	if len(s.Sites) == 0 || len(s.Sites) > maxScriptSites {
		return fmt.Errorf("1 to %d sites are required", maxScriptSites)
	}
	for _, h := range s.Sites {
		if h == "" || strings.ContainsAny(h, "/:* ") {
			return fmt.Errorf("site %q must be a host name", h)
		}
	}
	u, err := url.Parse(s.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(s.URL) > maxURLLen {
		return errors.New("url must be a plain https:// address")
	}
	if !sha256Re.MatchString(s.SHA256) {
		return errors.New("sha256 must be 64 lowercase hex digits")
	}
	return nil
}

// ParseScripts decodes and checks a hub list; bad entries are dropped.
func ParseScripts(b []byte) (Scripts, error) {
	if len(b) > maxBytes {
		return Scripts{}, errors.New("scripts catalog: file too large")
	}
	var raw Scripts
	if err := json.Unmarshal(b, &raw); err != nil {
		return Scripts{}, fmt.Errorf("scripts catalog: %w", err)
	}
	if raw.Version != Version {
		return Scripts{}, fmt.Errorf("scripts catalog: unsupported version %d", raw.Version)
	}
	out := Scripts{Version: raw.Version, Scripts: []Script{}}
	seen := map[string]bool{}
	for _, s := range raw.Scripts {
		if len(out.Scripts) == maxScripts {
			break
		}
		s.Name, s.URL = strings.TrimSpace(s.Name), strings.TrimSpace(s.URL)
		if CheckScript(s) != nil || seen[s.URL] {
			continue
		}
		seen[s.URL] = true
		out.Scripts = append(out.Scripts, s)
	}
	return out, nil
}

// BundledScripts is the hub list built into this binary.
func BundledScripts() Scripts {
	s, err := ParseScripts(bundledScripts)
	if err != nil {
		panic(err) // guarded by TestBundledScripts
	}
	return s
}

func scriptsCachePath() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "mypack", "scripts.json")
}

// FetchScripts downloads the hub list, checks it and caches it.
func FetchScripts(ctx context.Context) (Scripts, error) {
	if ScriptsRemoteURL == "" {
		return Scripts{}, errors.New("scripts catalog: remote updates are off")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ScriptsRemoteURL, nil)
	if err != nil {
		return Scripts{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Scripts{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Scripts{}, fmt.Errorf("scripts catalog: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return Scripts{}, err
	}
	s, err := ParseScripts(b)
	if err != nil {
		return Scripts{}, err
	}
	if p := scriptsCachePath(); p != "" && os.MkdirAll(filepath.Dir(p), 0o755) == nil {
		_ = os.WriteFile(p, b, 0o644)
	}
	return s, nil
}

// LatestScripts is a fresh cache, else a fetch, else a stale cache, else
// the bundled list.
func LatestScripts(ctx context.Context) Scripts {
	var cached Scripts
	var cerr error = errors.New("no cache")
	if p := scriptsCachePath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			cached, cerr = ParseScripts(b)
			if st, err := os.Stat(p); cerr == nil && err == nil && time.Since(st.ModTime()) < CacheTTL {
				return cached
			}
		}
	}
	if s, err := FetchScripts(ctx); err == nil {
		return s
	}
	if cerr == nil {
		return cached
	}
	return BundledScripts()
}
