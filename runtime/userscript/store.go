package userscript

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// EnvDir overrides the store folder (tests, portable setups).
const EnvDir = "MYPACK_USERSCRIPTS_DIR"

// Size limits keep a bad script or resource from bloating every page.
const (
	MaxScriptBytes   = 5 << 20
	MaxRequireBytes  = 5 << 20
	MaxResourceBytes = 2 << 20
)

// Dir returns the store folder: $MYPACK_USERSCRIPTS_DIR, else
// <UserConfigDir>/mypack-apps/userscripts.
func Dir() string {
	if p := os.Getenv(EnvDir); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "mypack-apps", "userscripts")
}

// Entry is one installed script in index.json.
type Entry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace,omitempty"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	// Apps are the app identifiers the script runs in; empty means all apps.
	Apps      []string         `json:"apps,omitempty"`
	SourceURL string           `json:"sourceUrl,omitempty"`
	Requires  []string         `json:"requires,omitempty"` // files in the script folder, in order
	Resources []StoredResource `json:"resources,omitempty"`
	Installed time.Time        `json:"installed"`
	Updated   time.Time        `json:"updated"`
}

// RunsIn reports whether the entry applies to the app with identifier appID.
func (e Entry) RunsIn(appID string) bool {
	if len(e.Apps) == 0 {
		return true
	}
	for _, a := range e.Apps {
		if strings.EqualFold(a, appID) {
			return true
		}
	}
	return false
}

// StoredResource is a downloaded @resource.
type StoredResource struct {
	Name string `json:"name"`
	File string `json:"file"`
	MIME string `json:"mime"`
}

// Index is index.json.
type Index struct {
	Scripts []Entry `json:"scripts"`
}

// Store is a userscript folder.
type Store struct {
	Dir string
	Now func() time.Time // nil: time.Now
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Store) indexPath() string { return filepath.Join(s.Dir, "index.json") }

// Read loads index.json; a missing store is empty.
func (s Store) Read() (Index, error) {
	b, err := os.ReadFile(s.indexPath())
	if errors.Is(err, os.ErrNotExist) {
		return Index{}, nil
	}
	if err != nil {
		return Index{}, err
	}
	var ix Index
	if err := json.Unmarshal(b, &ix); err != nil {
		return Index{}, fmt.Errorf("userscript index %s: %w", s.indexPath(), err)
	}
	return ix, nil
}

func (s Store) write(ix Index) error {
	sort.Slice(ix.Scripts, func(i, j int) bool { return ix.Scripts[i].ID < ix.Scripts[j].ID })
	b, err := json.MarshalIndent(ix, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.indexPath(), append(b, '\n'))
}

func writeAtomic(path string, b []byte) error {
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

// Fetcher downloads @require and @resource files.
type Fetcher func(ctx context.Context, rawURL string) (data []byte, contentType string, err error)

// HTTPFetcher fetches over http(s) with a size cap.
func HTTPFetcher(client *http.Client) Fetcher {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return func(ctx context.Context, rawURL string) ([]byte, string, error) {
		u, err := url.Parse(rawURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return nil, "", fmt.Errorf("%s: only http(s) URLs can be downloaded", rawURL)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("User-Agent", "MyPack-userscripts")
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return nil, "", fmt.Errorf("%s: HTTP %d", rawURL, resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, MaxScriptBytes+1))
		if err != nil {
			return nil, "", err
		}
		if len(b) > MaxScriptBytes {
			return nil, "", fmt.Errorf("%s: larger than %d MB", rawURL, MaxScriptBytes>>20)
		}
		return b, resp.Header.Get("Content-Type"), nil
	}
}

// InstallOptions tune Install.
type InstallOptions struct {
	SourceURL string   // where the script came from (base for relative @require; used by update)
	Apps      []string // nil keeps the scope of an installed copy (new: all apps)
	Fetch     Fetcher  // nil: HTTPFetcher(nil)
}

// Install adds or replaces a script (same @namespace and @name). It
// downloads @require and @resource files first, so a failed download leaves
// the installed copy as it was.
func (s Store) Install(ctx context.Context, src []byte, o InstallOptions) (Entry, Meta, error) {
	if len(src) > MaxScriptBytes {
		return Entry{}, Meta{}, fmt.Errorf("the script is larger than %d MB", MaxScriptBytes>>20)
	}
	m, err := Parse(string(src))
	if err != nil {
		return Entry{}, Meta{}, err
	}
	if _, _, err := m.Rules(); err != nil {
		return Entry{}, Meta{}, err
	}
	fetch := o.Fetch
	if fetch == nil {
		fetch = HTTPFetcher(nil)
	}
	id := m.ID()
	stage, err := os.MkdirTemp(s.Dir, "."+id+"-")
	if err != nil {
		if err := os.MkdirAll(s.Dir, 0o755); err != nil {
			return Entry{}, Meta{}, err
		}
		if stage, err = os.MkdirTemp(s.Dir, "."+id+"-"); err != nil {
			return Entry{}, Meta{}, err
		}
	}
	defer os.RemoveAll(stage)

	e := Entry{ID: id, Name: m.Name, Namespace: m.Namespace, Version: m.Version, Description: m.Description,
		Enabled: true, Apps: o.Apps, SourceURL: o.SourceURL}
	if err := os.WriteFile(filepath.Join(stage, "script.user.js"), src, 0o644); err != nil {
		return Entry{}, Meta{}, err
	}
	for i, r := range m.Require {
		b, _, err := fetch(ctx, resolve(o.SourceURL, r))
		if err != nil {
			return Entry{}, Meta{}, fmt.Errorf("@require %s: %w", r, err)
		}
		if len(b) > MaxRequireBytes {
			return Entry{}, Meta{}, fmt.Errorf("@require %s: larger than %d MB", r, MaxRequireBytes>>20)
		}
		name := fmt.Sprintf("require-%d.js", i)
		if err := os.WriteFile(filepath.Join(stage, name), b, 0o644); err != nil {
			return Entry{}, Meta{}, err
		}
		e.Requires = append(e.Requires, name)
	}
	for i, r := range m.Resource {
		u := resolve(o.SourceURL, r.URL)
		b, ct, err := fetch(ctx, u)
		if err != nil {
			return Entry{}, Meta{}, fmt.Errorf("@resource %s: %w", r.Name, err)
		}
		if len(b) > MaxResourceBytes {
			return Entry{}, Meta{}, fmt.Errorf("@resource %s: larger than %d MB", r.Name, MaxResourceBytes>>20)
		}
		name := fmt.Sprintf("resource-%d", i)
		if err := os.WriteFile(filepath.Join(stage, name), b, 0o644); err != nil {
			return Entry{}, Meta{}, err
		}
		e.Resources = append(e.Resources, StoredResource{Name: r.Name, File: name, MIME: guessMIME(u, ct, b)})
	}

	ix, err := s.Read()
	if err != nil {
		return Entry{}, Meta{}, err
	}
	now := s.now().UTC()
	e.Installed, e.Updated = now, now
	pos := -1
	for i, old := range ix.Scripts {
		if old.ID == id {
			pos = i
			e.Installed = old.Installed
			e.Enabled = old.Enabled
			if o.Apps == nil {
				e.Apps = old.Apps
			}
			if e.SourceURL == "" {
				e.SourceURL = old.SourceURL
			}
		}
	}
	final := filepath.Join(s.Dir, id)
	old := final + ".old"
	_ = os.RemoveAll(old)
	if _, err := os.Stat(final); err == nil {
		if err := os.Rename(final, old); err != nil {
			return Entry{}, Meta{}, err
		}
	}
	if err := os.Rename(stage, final); err != nil {
		_ = os.Rename(old, final)
		return Entry{}, Meta{}, err
	}
	_ = os.RemoveAll(old)
	if pos >= 0 {
		ix.Scripts[pos] = e
	} else {
		ix.Scripts = append(ix.Scripts, e)
	}
	if err := s.write(ix); err != nil {
		return Entry{}, Meta{}, err
	}
	return e, m, nil
}

func resolve(base, ref string) string {
	if base == "" {
		return ref
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

func guessMIME(u, ct string, b []byte) string {
	if t, _, err := mime.ParseMediaType(ct); err == nil && t != "" && t != "application/octet-stream" {
		return t
	}
	if p, err := url.Parse(u); err == nil {
		if t := mime.TypeByExtension(filepath.Ext(p.Path)); t != "" {
			return strings.SplitN(t, ";", 2)[0]
		}
	}
	return strings.SplitN(http.DetectContentType(b), ";", 2)[0]
}

// ErrNotFound means no installed script has that id.
var ErrNotFound = errors.New("no installed userscript has that id")

func (s Store) update(id string, fn func(*Entry)) (Entry, error) {
	ix, err := s.Read()
	if err != nil {
		return Entry{}, err
	}
	for i := range ix.Scripts {
		if ix.Scripts[i].ID == id {
			fn(&ix.Scripts[i])
			return ix.Scripts[i], s.write(ix)
		}
	}
	return Entry{}, ErrNotFound
}

// SetEnabled turns a script on or off.
func (s Store) SetEnabled(id string, on bool) (Entry, error) {
	return s.update(id, func(e *Entry) { e.Enabled = on })
}

// SetApps sets the apps a script runs in (empty: all apps).
func (s Store) SetApps(id string, apps []string) (Entry, error) {
	return s.update(id, func(e *Entry) { e.Apps = apps })
}

// Remove deletes a script and its files.
func (s Store) Remove(id string) error {
	ix, err := s.Read()
	if err != nil {
		return err
	}
	out := ix.Scripts[:0]
	found := false
	for _, e := range ix.Scripts {
		if e.ID == id {
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		return ErrNotFound
	}
	ix.Scripts = out
	if err := s.write(ix); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(s.Dir, filepath.Base(id)))
}

// Script is an enabled script read for an app.
type Script struct {
	Entry
	Meta      Meta
	Include   []Rule
	Exclude   []Rule
	Code      string   // the script
	Requires  []string // @require code, in order
	Resources []LoadedResource
}

// LoadedResource is a @resource's bytes.
type LoadedResource struct {
	Name string
	MIME string
	Data []byte
}

// Load reads the enabled scripts that run in app appID. A broken script is
// skipped and reported in errs; the others still load.
func (s Store) Load(appID string) (out []Script, errs []error) {
	ix, err := s.Read()
	if err != nil {
		return nil, []error{err}
	}
	for _, e := range ix.Scripts {
		if !e.Enabled || !e.RunsIn(appID) {
			continue
		}
		sc, err := s.loadOne(e)
		if err != nil {
			errs = append(errs, fmt.Errorf("userscript %s: %w", e.ID, err))
			continue
		}
		out = append(out, sc)
	}
	return out, errs
}

func (s Store) loadOne(e Entry) (Script, error) {
	dir := filepath.Join(s.Dir, filepath.Base(e.ID))
	src, err := os.ReadFile(filepath.Join(dir, "script.user.js"))
	if err != nil {
		return Script{}, err
	}
	m, err := Parse(string(src))
	if err != nil {
		return Script{}, err
	}
	inc, exc, err := m.Rules()
	if err != nil {
		return Script{}, err
	}
	sc := Script{Entry: e, Meta: m, Include: inc, Exclude: exc, Code: string(src)}
	for _, f := range e.Requires {
		b, err := os.ReadFile(filepath.Join(dir, filepath.Base(f)))
		if err != nil {
			return Script{}, err
		}
		sc.Requires = append(sc.Requires, string(b))
	}
	for _, r := range e.Resources {
		b, err := os.ReadFile(filepath.Join(dir, filepath.Base(r.File)))
		if err != nil {
			return Script{}, err
		}
		sc.Resources = append(sc.Resources, LoadedResource{Name: r.Name, MIME: r.MIME, Data: b})
	}
	return sc, nil
}
