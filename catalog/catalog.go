// Package catalog is the Discover catalog: popular websites people can turn
// into apps with one click. The list lives in discover.json, which ships
// inside every build (so Discover works offline) and is also fetched from
// the project's repository, so new sites reach users without a release.
//
// An entry only prefills the Create screen with a name and an address. It
// carries no scripts, styles or settings, so nothing in the catalog can run
// code; Parse rejects anything but plain https addresses.
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
	"strings"
	"time"
	"unicode/utf8"
)

//go:embed discover.json
var bundled []byte

// Categories are the shelves of Discover, in display order.
var Categories = []string{"work", "dev", "chat", "social", "ai", "design", "media"}

// App is one catalog entry.
type App struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Category string `json:"category"`
}

// Catalog is the whole file.
type Catalog struct {
	Version int   `json:"version"`
	Apps    []App `json:"apps"`
}

// Version is the catalog format this build understands.
const Version = 1

// Limits that keep a bad or hostile remote file harmless.
const (
	maxBytes   = 512 << 10
	maxApps    = 500
	maxNameLen = 40
	maxURLLen  = 300
)

// RemoteURL is where the latest catalog is fetched from. Release builds can
// override it at link time:
//
//	-ldflags "-X mypack.dev/mypack/catalog.RemoteURL=https://…/discover.json"
//
// An empty value turns remote updates off; the bundled catalog is used.
var RemoteURL = "https://raw.githubusercontent.com/sinhong2011/mypack/main/catalog/discover.json"

// Parse decodes and checks a catalog. Entries that break a rule are
// dropped; a file with no usable entry, or of another format version, is an
// error.
func Parse(b []byte) (Catalog, error) {
	if len(b) > maxBytes {
		return Catalog{}, errors.New("catalog: file too large")
	}
	var raw Catalog
	if err := json.Unmarshal(b, &raw); err != nil {
		return Catalog{}, fmt.Errorf("catalog: %w", err)
	}
	if raw.Version != Version {
		return Catalog{}, fmt.Errorf("catalog: unsupported version %d", raw.Version)
	}
	out := Catalog{Version: raw.Version}
	seen := map[string]bool{}
	for _, a := range raw.Apps {
		if len(out.Apps) == maxApps {
			break
		}
		a.Name, a.URL = strings.TrimSpace(a.Name), strings.TrimSpace(a.URL)
		if Check(a) != nil || seen[a.URL] {
			continue
		}
		seen[a.URL] = true
		out.Apps = append(out.Apps, a)
	}
	if len(out.Apps) == 0 {
		return Catalog{}, errors.New("catalog: no valid apps")
	}
	return out, nil
}

// Check reports why an entry is not acceptable, or nil.
func Check(a App) error {
	if a.Name == "" || utf8.RuneCountInString(a.Name) > maxNameLen {
		return fmt.Errorf("name must be 1 to %d characters", maxNameLen)
	}
	known := false
	for _, c := range Categories {
		known = known || c == a.Category
	}
	if !known {
		return fmt.Errorf("unknown category %q", a.Category)
	}
	if len(a.URL) > maxURLLen {
		return errors.New("address too long")
	}
	u, err := url.Parse(a.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errors.New("address must be a plain https:// URL")
	}
	return nil
}

// Bundled returns the catalog built into this binary.
func Bundled() Catalog {
	c, err := Parse(bundled)
	if err != nil {
		panic(err) // guarded by TestBundled
	}
	return c
}

// CacheTTL is how long a fetched catalog is used before fetching again.
const CacheTTL = 24 * time.Hour

func cachePath() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "mypack", "discover.json")
}

// Cached returns the last fetched catalog and whether it is still fresh.
func Cached() (Catalog, bool, error) {
	p := cachePath()
	if p == "" {
		return Catalog{}, false, errors.New("catalog: no cache folder")
	}
	st, err := os.Stat(p)
	if err != nil {
		return Catalog{}, false, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Catalog{}, false, err
	}
	c, err := Parse(b)
	return c, time.Since(st.ModTime()) < CacheTTL, err
}

// Fetch downloads the catalog from RemoteURL, checks it and caches it.
func Fetch(ctx context.Context) (Catalog, error) {
	if RemoteURL == "" || strings.Contains(RemoteURL, "/OWNER/") {
		return Catalog{}, errors.New("catalog: remote updates are off")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, RemoteURL, nil)
	if err != nil {
		return Catalog{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Catalog{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Catalog{}, fmt.Errorf("catalog: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return Catalog{}, err
	}
	c, err := Parse(b)
	if err != nil {
		return Catalog{}, err
	}
	if p := cachePath(); p != "" && os.MkdirAll(filepath.Dir(p), 0o755) == nil {
		_ = os.WriteFile(p, b, 0o644)
	}
	return c, nil
}

// Latest returns the newest catalog available without waiting long: a
// fresh cache, else a fetch, else a stale cache, else the bundled list.
func Latest(ctx context.Context) Catalog {
	cached, fresh, cerr := Cached()
	if cerr == nil && fresh {
		return cached
	}
	if c, err := Fetch(ctx); err == nil {
		return c
	}
	if cerr == nil {
		return cached
	}
	return Bundled()
}
