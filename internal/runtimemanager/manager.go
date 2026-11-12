// Package runtimemanager resolves, downloads, verifies and caches
// generated-app runtimes from a signed registry (spec §5, ADR-003/004/025).
//
// Trust model: registry.json is signed with Ed25519 (detached
// registry.json.sig = base64 of the signature over the exact bytes). See
// trust.go for where the trusted public keys come from. Manifests that are
// expired, mis-signed or whose sequence is lower than the highest sequence
// ever seen (anti-rollback, persisted in the cache) are rejected. Archives
// are checked for size and sha256 before a safe extraction, and the
// extracted binary's own sha256 is stored in a ".verified" marker so cached
// runtimes can be re-verified offline.
//
// Installers ship the host platform's runtime (and a copy of the signed
// manifest) inside the app bundle. SeedFromDir copies those into the cache
// on first launch. The remote registry is fetched only when Ensure needs a
// version or platform that is not already cached.
package runtimemanager

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"mypack.dev/mypack/internal/domain"
)

// DefaultRegistryURL is used when neither an argument nor MYPACK_REGISTRY is given.
const DefaultRegistryURL = "https://github.com/sinhong2011/mypack/releases/download/runtimes/registry.json"

const (
	markerName     = ".verified"
	lastUsedName   = ".lastused"
	registryDir    = "_registry"
	manifestMemo   = 60 * time.Second
	maxManifestLen = 4 << 20
	maxArchiveSize = 512 << 20
)

// Manager is safe for concurrent use.
type Manager struct {
	cacheDir string

	mu          sync.Mutex
	registryURL string
	client      *http.Client
	keys        []ed25519.PublicKey // nil => package-level TrustedKeys()
	now         func() time.Time
	memo        *Manifest
	memoAt      time.Time
	memoURL     string
	flights     map[string]*flight

	stateMu sync.Mutex
}

type flight struct {
	done chan struct{}
	path string
	err  error
}

// marker is the content of the ".verified" file of an installed runtime.
type marker struct {
	Version      string    `json:"version"`
	Key          string    `json:"key"`
	SHA256       string    `json:"sha256"` // archive
	Size         int64     `json:"size"`   // archive
	Binary       string    `json:"binary"`
	BinarySHA256 string    `json:"binarySha256"`
	BinarySize   int64     `json:"binarySize"`
	VerifiedAt   time.Time `json:"verifiedAt"`
}

type state struct {
	LastSequence int64 `json:"lastSequence"`
}

// New creates a manager. cacheDir "" = platform default (see ARCHITECTURE.md).
// registryURL may be https://…/registry.json or file:///…/registry.json;
// "" = $MYPACK_REGISTRY or DefaultRegistryURL.
func New(cacheDir, registryURL string) *Manager {
	if cacheDir == "" {
		cacheDir = DefaultCacheDir()
	}
	if registryURL == "" {
		registryURL = os.Getenv("MYPACK_REGISTRY")
	}
	if registryURL == "" {
		registryURL = DefaultRegistryURL
	}
	return &Manager{
		cacheDir:    cacheDir,
		registryURL: registryURL,
		now:         time.Now,
		flights:     map[string]*flight{},
		client: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				IdleConnTimeout:       30 * time.Second,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many redirects")
				}
				if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
					return errors.New("redirect to unsupported scheme")
				}
				if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
					return errors.New("redirect downgrades https to http")
				}
				return nil
			},
		},
	}
}

// DefaultCacheDir returns the platform runtime cache directory.
func DefaultCacheDir() string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			if h, err := os.UserHomeDir(); err == nil {
				base = filepath.Join(h, "AppData", "Local")
			}
		}
		return filepath.Join(base, "mypack", "cache", "runtimes")
	default:
		base, err := os.UserCacheDir() // macOS ~/Library/Caches, Linux $XDG_CACHE_HOME or ~/.cache
		if err != nil || base == "" {
			base = os.TempDir()
		}
		return filepath.Join(base, "mypack", "runtimes")
	}
}

// CacheDir returns the cache directory in use.
func (m *Manager) CacheDir() string { return m.cacheDir }

// RegistryURL returns the current registry URL.
func (m *Manager) RegistryURL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registryURL
}

// SetRegistryURL changes the registry (settings change).
func (m *Manager) SetRegistryURL(u string) {
	if u == "" {
		u = os.Getenv("MYPACK_REGISTRY")
	}
	if u == "" {
		u = DefaultRegistryURL
	}
	m.mu.Lock()
	m.registryURL = u
	m.memo = nil
	m.mu.Unlock()
}

// SetTrustedKeys pins this manager to exactly these keys, ignoring the
// package-level key sources (used by tests and embedders).
func (m *Manager) SetTrustedKeys(keys ...ed25519.PublicKey) {
	m.mu.Lock()
	m.keys = keys
	m.mu.Unlock()
}

// SetHTTPClient replaces the HTTP client (tests).
func (m *Manager) SetHTTPClient(c *http.Client) {
	m.mu.Lock()
	m.client = c
	m.mu.Unlock()
}

// SetClock replaces the time source (tests).
func (m *Manager) SetClock(now func() time.Time) {
	m.mu.Lock()
	m.now = now
	m.mu.Unlock()
}

func (m *Manager) trusted() []ed25519.PublicKey {
	m.mu.Lock()
	k := m.keys
	m.mu.Unlock()
	if k != nil {
		return k
	}
	return TrustedKeys()
}

func (m *Manager) clock() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now()
}

// ---------------------------------------------------------------- errors

func newErr(code, msg string, cause error, remediation string) *domain.Error {
	e := &domain.Error{Code: code, Message: msg, Remediation: remediation}
	if cause != nil {
		e.Cause = cause.Error()
	}
	return e
}

// unreachable marks failures to reach the registry (as opposed to a registry
// that answered with something we must not trust).
type unreachable struct{ error }

func (u unreachable) Unwrap() error { return u.error }

func isCtxErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

const remediationOffline = "Check your internet connection, or set a different registry with MYPACK_REGISTRY / Settings → Runtime registry."

// ------------------------------------------------------------- manifest

func (m *Manager) registryDir() string { return filepath.Join(m.cacheDir, registryDir) }

func (m *Manager) readState() state {
	var s state
	if b, err := os.ReadFile(filepath.Join(m.registryDir(), "state.json")); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

func writeFileAtomic(p string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".w-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	_ = os.Chmod(name, perm)
	if err := os.Rename(name, p); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// readURL reads a small resource over http(s) or file://.
func (m *Manager) readURL(ctx context.Context, u string, limit int64) ([]byte, error) {
	rc, _, err := m.open(ctx, u)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, unreachable{err}
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("response larger than %d bytes", limit)
	}
	return b, nil
}

// open opens an http(s) or file:// URL for streaming. size is -1 if unknown.
func (m *Manager) open(ctx context.Context, raw string) (io.ReadCloser, int64, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, 0, err
	}
	switch u.Scheme {
	case "file":
		p := u.Path
		if runtime.GOOS == "windows" && len(p) >= 3 && p[0] == '/' && p[2] == ':' {
			p = p[1:]
		}
		if u.Host != "" && u.Host != "localhost" {
			p = `\\` + u.Host + p
		}
		f, err := os.Open(filepath.FromSlash(p))
		if err != nil {
			return nil, 0, unreachable{err}
		}
		st, _ := f.Stat()
		sz := int64(-1)
		if st != nil {
			sz = st.Size()
		}
		return f, sz, nil
	case "http", "https":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("User-Agent", "mypack-builder")
		m.mu.Lock()
		c := m.client
		m.mu.Unlock()
		resp, err := c.Do(req)
		if err != nil {
			return nil, 0, unreachable{err}
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, 0, unreachable{fmt.Errorf("HTTP %d from %s", resp.StatusCode, u.Host)}
		}
		return resp.Body, resp.ContentLength, nil
	}
	return nil, 0, fmt.Errorf("unsupported URL scheme %q", u.Scheme)
}

// fetchManifest downloads, verifies and persists the registry manifest.
func (m *Manager) fetchManifest(ctx context.Context) (*Manifest, error) {
	ru := m.RegistryURL()
	fctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	raw, err := m.readURL(fctx, ru, maxManifestLen)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	sig, err := m.readURL(fctx, SigURL(ru), 4096)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var un unreachable
		if errors.As(err, &un) {
			// A registry without a signature is not trustworthy, not "offline".
			return nil, newErr("RUNTIME_VERIFY_FAILED", "The runtime registry has no signature", err,
				"The registry must publish registry.json.sig next to registry.json.")
		}
		return nil, err
	}
	man, err := m.checkManifest(raw, sig)
	if err != nil {
		return nil, err
	}
	m.persistManifest(raw, sig, man)
	return man, nil
}

func (m *Manager) persistManifest(raw, sig []byte, man *Manifest) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	dir := m.registryDir()
	_ = writeFileAtomic(filepath.Join(dir, "registry.json"), raw, 0o644)
	_ = writeFileAtomic(filepath.Join(dir, "registry.json.sig"), sig, 0o644)
	if st := m.readState(); man.Sequence > st.LastSequence {
		if b, err := json.Marshal(state{LastSequence: man.Sequence}); err == nil {
			_ = writeFileAtomic(filepath.Join(dir, "state.json"), b, 0o644)
		}
	}
}

// Refresh fetches and persists the remote registry. List and a cache hit
// on Ensure do not call this; it is for updates and tests.
func (m *Manager) Refresh(ctx context.Context) (*Manifest, error) {
	man, err := m.fetchManifest(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.memo, m.memoAt, m.memoURL = man, m.now(), m.registryURL
	m.mu.Unlock()
	return man, nil
}

// checkManifest verifies signature, expiry and anti-rollback.
func (m *Manager) checkManifest(raw, sig []byte) (*Manifest, error) {
	keys := m.trusted()
	if len(keys) == 0 {
		return nil, newErr("RUNTIME_VERIFY_FAILED", "No trusted registry key is configured", nil,
			"Use an official build of mypack, or set MYPACK_TRUSTED_KEY (base64 Ed25519 public key) / MYPACK_DEV_TRUST=1 for a development registry.")
	}
	man, err := VerifyManifest(raw, sig, keys)
	if err != nil {
		return nil, newErr("RUNTIME_VERIFY_FAILED", "The runtime registry could not be verified", err,
			"Do not use this registry. Check the registry URL in Settings, or update mypack to get new trusted keys.")
	}
	if man.Expires.IsZero() || !m.clock().Before(man.Expires) {
		return nil, newErr("RUNTIME_VERIFY_FAILED", "The runtime registry manifest has expired",
			fmt.Errorf("expired %s", man.Expires.Format(time.RFC3339)),
			"The registry operator must publish a fresh manifest. Check the registry URL or try again later.")
	}
	m.stateMu.Lock()
	last := m.readState().LastSequence
	m.stateMu.Unlock()
	if man.Sequence < last {
		return nil, newErr("RUNTIME_VERIFY_FAILED", "The runtime registry manifest is older than one already seen (rollback)",
			fmt.Errorf("sequence %d < %d", man.Sequence, last),
			"The registry may have been tampered with or is serving stale data. Do not use it until the operator republishes.")
	}
	return man, nil
}

// cachedManifest returns the last verified manifest from disk, re-checking its
// signature. expired reports whether it is past its expiry.
func (m *Manager) cachedManifest() (man *Manifest, expired bool, err error) {
	dir := m.registryDir()
	raw, err := os.ReadFile(filepath.Join(dir, "registry.json"))
	if err != nil {
		return nil, false, err
	}
	sig, err := os.ReadFile(filepath.Join(dir, "registry.json.sig"))
	if err != nil {
		return nil, false, err
	}
	keys := m.trusted()
	if len(keys) == 0 {
		return nil, false, errors.New("no trusted keys")
	}
	man, err = VerifyManifest(raw, sig, keys)
	if err != nil {
		return nil, false, err
	}
	return man, man.Expires.IsZero() || !m.clock().Before(man.Expires), nil
}

// bestManifest returns a fresh manifest when the registry is reachable, else
// the cached one (stale=true). A registry that answers with an untrustworthy
// manifest is a hard error; only unreachability falls back to the cache.
func (m *Manager) bestManifest(ctx context.Context) (man *Manifest, stale bool, err error) {
	m.mu.Lock()
	if m.memo != nil && m.memoURL == m.registryURL && m.now().Sub(m.memoAt) < manifestMemo {
		man = m.memo
		m.mu.Unlock()
		return man, false, nil
	}
	m.mu.Unlock()
	man, err = m.fetchManifest(ctx)
	if err == nil {
		m.mu.Lock()
		m.memo, m.memoAt, m.memoURL = man, m.now(), m.registryURL
		m.mu.Unlock()
		return man, false, nil
	}
	if isCtxErr(err) && ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	var un unreachable
	if !errors.As(err, &un) {
		if _, isDomain := err.(*domain.Error); isDomain {
			return nil, false, err
		}
		return nil, false, newErr("RUNTIME_DOWNLOAD_FAILED", "Could not read the runtime registry", err, remediationOffline)
	}
	if cm, _, cerr := m.cachedManifest(); cerr == nil {
		return cm, true, nil
	}
	return nil, false, newErr("RUNTIME_DOWNLOAD_FAILED", "Could not reach the runtime registry", err, remediationOffline)
}

// CachedEntry returns an entry from the on-disk manifest without any network access.
func (m *Manager) CachedEntry(version, key string) (Entry, bool) {
	man, _, err := m.cachedManifest()
	if err != nil {
		return Entry{}, false
	}
	return man.Find(version, key)
}

// Entry looks up a runtime in the registry (network, falling back to the
// cached manifest when offline).
func (m *Manager) Entry(ctx context.Context, version, key string) (Entry, error) {
	if err := checkVK(version, key); err != nil {
		return Entry{}, err
	}
	man, _, err := m.bestManifest(ctx)
	if err != nil {
		return Entry{}, err
	}
	e, ok := man.Find(version, key)
	if !ok {
		return Entry{}, notFound(version, key, man)
	}
	return e, nil
}

func notFound(version, key string, man *Manifest) *domain.Error {
	var avail []string
	if man != nil {
		for _, e := range man.Runtimes {
			if e.Key() == key {
				avail = append(avail, e.Version)
			}
		}
	}
	rem := "Pick a runtime version that the registry offers for this platform."
	if len(avail) > 0 {
		rem = "Available versions for " + key + ": " + strings.Join(avail, ", ") + "."
	}
	return newErr("RUNTIME_NOT_FOUND", fmt.Sprintf("Runtime %s for %s is not available in the registry", version, key), nil, rem)
}

func checkVK(version, key string) error {
	if !safeComponent(version) || !safeComponent(key) {
		return newErr("RUNTIME_NOT_FOUND", fmt.Sprintf("Invalid runtime version or platform: %q %q", version, key), nil,
			"Use a version like 1.0.0 and a platform key like darwin-arm64.")
	}
	return nil
}

// --------------------------------------------------------------- local cache

func (m *Manager) runtimeDir(version, key string) string {
	return filepath.Join(m.cacheDir, version, key)
}

func readMarker(dir string) (marker, error) {
	var mk marker
	b, err := os.ReadFile(filepath.Join(dir, markerName))
	if err != nil {
		return mk, err
	}
	if err := json.Unmarshal(b, &mk); err != nil {
		return mk, err
	}
	return mk, nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// checkLocal re-hashes the cached binary against its marker (and the marker
// against entry, when given). Returns the executable path.
func (m *Manager) checkLocal(version, key string, entry *Entry) (string, error) {
	dir := m.runtimeDir(version, key)
	mk, err := readMarker(dir)
	if err != nil {
		return "", errors.New("not cached")
	}
	if mk.Version != version || mk.Key != key {
		return "", errors.New("marker does not match the cache location")
	}
	if entry != nil {
		if !strings.EqualFold(mk.SHA256, entry.SHA256) || mk.Size != entry.Size {
			return "", errors.New("cached runtime does not match the registry manifest")
		}
	}
	if mk.Binary != ExecutableName(key) {
		return "", errors.New("marker names an unexpected binary")
	}
	exe := filepath.Join(dir, mk.Binary)
	st, err := os.Lstat(exe)
	if err != nil || !st.Mode().IsRegular() {
		return "", errors.New("cached runtime binary is missing")
	}
	sum, n, err := hashFile(exe)
	if err != nil {
		return "", err
	}
	if sum != mk.BinarySHA256 || n != mk.BinarySize {
		return "", errors.New("cached runtime binary has been modified")
	}
	return exe, nil
}

func touch(dir string) { _ = os.WriteFile(filepath.Join(dir, lastUsedName), nil, 0o644) }

func (m *Manager) info(version, key string, entry *Entry) domain.RuntimeInfo {
	ri := domain.RuntimeInfo{Version: version, Key: key, Source: m.RegistryURL()}
	if entry != nil {
		ri.Size = entry.Size
	}
	dir := m.runtimeDir(version, key)
	if _, err := readMarker(dir); err == nil {
		ri.Cached = true
		ri.CacheSize = dirSize(dir)
		ri.Path = filepath.Join(dir, ExecutableName(key))
		if _, err := m.checkLocal(version, key, entry); err == nil {
			ri.Verified = true
		}
		if st, err := os.Stat(filepath.Join(dir, lastUsedName)); err == nil {
			ri.LastUsed = st.ModTime()
		}
		if ri.Size == 0 {
			if mk, err := readMarker(dir); err == nil {
				ri.Size = mk.Size
			}
		}
	}
	return ri
}

func dirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if fi, e := d.Info(); e == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

// localRuntimes lists "<version>/<key>" pairs present in the cache.
func (m *Manager) localRuntimes() [][2]string {
	var out [][2]string
	vers, _ := os.ReadDir(m.cacheDir)
	for _, v := range vers {
		if !v.IsDir() || !safeComponent(v.Name()) {
			continue
		}
		keys, _ := os.ReadDir(filepath.Join(m.cacheDir, v.Name()))
		for _, k := range keys {
			if k.IsDir() && safeComponent(k.Name()) {
				if _, err := readMarker(filepath.Join(m.cacheDir, v.Name(), k.Name())); err == nil {
					out = append(out, [2]string{v.Name(), k.Name()})
				}
			}
		}
	}
	return out
}

// ------------------------------------------------------------------- API

// List returns every runtime in the cached manifest merged with local cache
// state. It does not contact the remote registry; call Refresh or Ensure for
// a version that is not cached.
func (m *Manager) List(ctx context.Context) ([]domain.RuntimeInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	man, _, _ := m.cachedManifest()
	var out []domain.RuntimeInfo
	seen := map[[2]string]bool{}
	if man != nil {
		for _, e := range man.Runtimes {
			e := e
			out = append(out, m.info(e.Version, e.Key(), &e))
			seen[[2]string{e.Version, e.Key()}] = true
		}
	}
	for _, vk := range m.localRuntimes() {
		if seen[vk] {
			continue
		}
		ri := m.info(vk[0], vk[1], nil)
		ri.Source = "cache"
		out = append(out, ri)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Version != out[j].Version {
			return versionLess(out[j].Version, out[i].Version) // newest first
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}

func versionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			fmt.Sscanf(pa[i], "%d", &x)
		}
		if i < len(pb) {
			fmt.Sscanf(pb[i], "%d", &y)
		}
		if x != y {
			return x < y
		}
	}
	return a < b
}

// Status reports one runtime. A cached copy or a cached-manifest entry needs
// no network. An unknown version fetches the remote registry (an update).
func (m *Manager) Status(ctx context.Context, version, key string) (domain.RuntimeInfo, error) {
	if err := checkVK(version, key); err != nil {
		return domain.RuntimeInfo{}, err
	}
	var entry *Entry
	if e, ok := m.CachedEntry(version, key); ok {
		entry = &e
	}
	ri := m.info(version, key, entry)
	if ri.Cached || entry != nil {
		return ri, nil
	}
	e, err := m.Entry(ctx, version, key)
	if err == nil {
		return m.info(version, key, &e), nil
	}
	if de, ok := err.(*domain.Error); ok {
		return ri, de
	}
	return ri, newErr("RUNTIME_NOT_FOUND", fmt.Sprintf("Runtime %s for %s is not cached and no manifest lists it", version, key), nil,
		"Run a build with downloads allowed, or list runtimes to refresh the registry manifest.")
}

// Ensure returns the path of the verified runtime executable, downloading it
// if missing. progress may be nil. Errors are *domain.Error with codes
// RUNTIME_NOT_FOUND, RUNTIME_DOWNLOAD_FAILED, RUNTIME_VERIFY_FAILED, except
// that cancellation returns ctx.Err() unchanged. Concurrent calls for the same
// version/key coalesce into one download. A verified cache hit (with or
// without a cached manifest) does not contact the remote registry.
func (m *Manager) Ensure(ctx context.Context, version, key string, progress func(done, total int64)) (string, error) {
	if err := checkVK(version, key); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if ce, ok := m.CachedEntry(version, key); ok {
		if exe, err := m.checkLocal(version, key, &ce); err == nil {
			touch(m.runtimeDir(version, key))
			return exe, nil
		}
	} else if exe, err := m.checkLocal(version, key, nil); err == nil {
		touch(m.runtimeDir(version, key))
		return exe, nil
	}
	return m.do(ctx, version+"/"+key, func(ctx context.Context) (string, error) {
		return m.install(ctx, version, key, progress)
	})
}

func (m *Manager) do(ctx context.Context, id string, fn func(context.Context) (string, error)) (string, error) {
	for {
		m.mu.Lock()
		if f, ok := m.flights[id]; ok {
			m.mu.Unlock()
			select {
			case <-f.done:
				if f.err != nil && isCtxErr(f.err) && ctx.Err() == nil {
					continue // the leader was cancelled, not us: take over
				}
				return f.path, f.err
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		f := &flight{done: make(chan struct{})}
		m.flights[id] = f
		m.mu.Unlock()
		f.path, f.err = fn(ctx)
		m.mu.Lock()
		delete(m.flights, id)
		m.mu.Unlock()
		close(f.done)
		return f.path, f.err
	}
}

func (m *Manager) install(ctx context.Context, version, key string, progress func(done, total int64)) (string, error) {
	if man, _, err := m.cachedManifest(); err == nil {
		if entry, ok := man.Find(version, key); ok {
			if exe, err := m.checkLocal(version, key, &entry); err == nil {
				touch(m.runtimeDir(version, key))
				return exe, nil
			}
			return m.download(ctx, entry, progress)
		}
	}
	man, stale, err := m.bestManifest(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		// Offline with no usable manifest: a previously verified local copy still works.
		var de *domain.Error
		if errors.As(err, &de) && de.Code == "RUNTIME_DOWNLOAD_FAILED" {
			if exe, lerr := m.checkLocal(version, key, nil); lerr == nil {
				touch(m.runtimeDir(version, key))
				return exe, nil
			}
		}
		return "", err
	}
	entry, ok := man.Find(version, key)
	if !ok {
		return "", notFound(version, key, man)
	}
	if exe, err := m.checkLocal(version, key, &entry); err == nil {
		touch(m.runtimeDir(version, key))
		return exe, nil
	}
	if stale {
		return "", newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot download the runtime while the registry is unreachable", nil, remediationOffline)
	}
	if entry.Size <= 0 || entry.Size > maxArchiveSize || len(entry.SHA256) != 64 {
		return "", newErr("RUNTIME_VERIFY_FAILED", "The registry entry for this runtime is malformed", fmt.Errorf("size=%d sha256=%q", entry.Size, entry.SHA256),
			"Report this to the registry operator.")
	}
	return m.download(ctx, entry, progress)
}

func (m *Manager) resolveEntryURL(e Entry) (string, error) {
	ru := m.RegistryURL()
	base, err := url.Parse(ru)
	if err != nil {
		return "", err
	}
	ref := e.URL
	if ref == "" {
		ref = e.Version + "/" + e.Filename
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	abs := base.ResolveReference(r)
	switch abs.Scheme {
	case "https", "http":
	case "file":
		if base.Scheme != "file" {
			return "", errors.New("a remote registry may not point to local files")
		}
	default:
		return "", fmt.Errorf("unsupported URL scheme %q", abs.Scheme)
	}
	return abs.String(), nil
}

func (m *Manager) download(ctx context.Context, e Entry, progress func(done, total int64)) (string, error) {
	if e.Size <= 0 || e.Size > maxArchiveSize || len(e.SHA256) != 64 {
		return "", newErr("RUNTIME_VERIFY_FAILED", "The registry entry for this runtime is malformed", fmt.Errorf("size=%d sha256=%q", e.Size, e.SHA256),
			"Report this to the registry operator.")
	}
	if err := os.MkdirAll(m.cacheDir, 0o755); err != nil {
		return "", newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot create the runtime cache directory", err, "Check permissions of "+m.cacheDir+".")
	}
	m.cleanStaleTemp()
	tmp, err := os.MkdirTemp(m.cacheDir, ".tmp-")
	if err != nil {
		return "", newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot create a temporary directory in the cache", err, "Check free disk space and permissions of "+m.cacheDir+".")
	}
	defer os.RemoveAll(tmp)

	src, err := m.resolveEntryURL(e)
	if err != nil {
		return "", newErr("RUNTIME_VERIFY_FAILED", "The registry entry has an invalid download URL", err, "Report this to the registry operator.")
	}
	archive := filepath.Join(tmp, "archive.tar.gz")
	if err := m.fetchArchive(ctx, src, e, archive, progress); err != nil {
		return "", err
	}
	return m.materialize(e, archive, tmp)
}

func (m *Manager) materialize(e Entry, archive, tmp string) (string, error) {
	version, key := e.Version, e.Key()
	extracted := filepath.Join(tmp, "x")
	if err := os.Mkdir(extracted, 0o755); err != nil {
		return "", newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot create a temporary directory", err, "")
	}
	if err := ExtractTarGz(archive, extracted); err != nil {
		return "", newErr("RUNTIME_VERIFY_FAILED", "The runtime archive is invalid or unsafe", err,
			"The download was discarded. Try again; if it keeps failing the registry may be compromised.")
	}
	exeName := ExecutableName(key)
	exe := filepath.Join(extracted, exeName)
	st, err := os.Lstat(exe)
	if err != nil || !st.Mode().IsRegular() {
		return "", newErr("RUNTIME_VERIFY_FAILED", "The runtime archive does not contain "+exeName, err, "Report this to the registry operator.")
	}
	_ = os.Chmod(exe, 0o755)
	bsum, bsize, err := hashFile(exe)
	if err != nil {
		return "", newErr("RUNTIME_VERIFY_FAILED", "Cannot hash the runtime binary", err, "")
	}
	mk := marker{Version: version, Key: key, SHA256: strings.ToLower(e.SHA256), Size: e.Size, Binary: exeName,
		BinarySHA256: bsum, BinarySize: bsize, VerifiedAt: m.clock().UTC()}
	mb, _ := json.MarshalIndent(mk, "", "  ")
	if err := os.WriteFile(filepath.Join(extracted, markerName), mb, 0o644); err != nil {
		return "", newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot write the verification marker", err, "")
	}
	_ = os.WriteFile(filepath.Join(extracted, lastUsedName), nil, 0o644)

	final := m.runtimeDir(version, key)
	if err := swapDir(extracted, final); err != nil {
		return "", newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot install the runtime into the cache", err, "Check permissions and free disk space of "+m.cacheDir+".")
	}
	return filepath.Join(final, exeName), nil
}

// fetchArchive streams the archive to dst while hashing, then verifies size and sha256.
func (m *Manager) fetchArchive(ctx context.Context, src string, e Entry, dst string, progress func(done, total int64)) error {
	rc, _, err := m.open(ctx, src)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return newErr("RUNTIME_DOWNLOAD_FAILED", "Could not download the runtime", err, remediationOffline)
	}
	defer rc.Close()
	out, err := os.Create(dst)
	if err != nil {
		return newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot write the download", err, "Check free disk space.")
	}
	defer out.Close()
	h := sha256.New()
	var done int64
	buf := make([]byte, 64<<10)
	lr := io.LimitReader(rc, e.Size+1)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, rerr := lr.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			if _, werr := out.Write(buf[:n]); werr != nil {
				return newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot write the download", werr, "Check free disk space.")
			}
			done += int64(n)
			if progress != nil {
				progress(done, e.Size)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return newErr("RUNTIME_DOWNLOAD_FAILED", "The download was interrupted", rerr, remediationOffline)
		}
	}
	if err := out.Close(); err != nil {
		return newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot write the download", err, "")
	}
	if done != e.Size {
		return newErr("RUNTIME_VERIFY_FAILED", "The downloaded runtime has the wrong size",
			fmt.Errorf("got %d bytes (or more), manifest says %d", done, e.Size), "The download was discarded. Try again.")
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, e.SHA256) {
		return newErr("RUNTIME_VERIFY_FAILED", "The downloaded runtime does not match its checksum",
			fmt.Errorf("sha256 %s, manifest says %s", got, e.SHA256),
			"The download was discarded. Try again; if it keeps failing the registry or your network may be tampering with it.")
	}
	return nil
}

// swapDir atomically installs newDir at final, keeping any previous copy
// until the new one is in place.
func swapDir(newDir, final string) error {
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return err
	}
	if _, err := os.Lstat(final); err != nil {
		return os.Rename(newDir, final)
	}
	var rnd [4]byte
	_, _ = rand.Read(rnd[:])
	old := fmt.Sprintf("%s.old-%x", final, rnd)
	if err := os.Rename(final, old); err != nil {
		return err
	}
	if err := os.Rename(newDir, final); err != nil {
		_ = os.Rename(old, final) // restore
		return err
	}
	_ = os.RemoveAll(old)
	return nil
}

func (m *Manager) cleanStaleTemp() {
	ents, _ := os.ReadDir(m.cacheDir)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) > 24*time.Hour {
				_ = os.RemoveAll(filepath.Join(m.cacheDir, e.Name()))
			}
		}
	}
}

// Verify re-hashes a cached runtime against its marker and, when present, the
// cached manifest. It does not contact the remote registry.
func (m *Manager) Verify(ctx context.Context, version, key string) error {
	if err := checkVK(version, key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := m.runtimeDir(version, key)
	if _, err := readMarker(dir); err != nil {
		return newErr("RUNTIME_NOT_FOUND", fmt.Sprintf("Runtime %s for %s is not cached", version, key), err,
			"Download it first (build with downloads allowed).")
	}
	var entry *Entry
	if e, ok := m.CachedEntry(version, key); ok {
		entry = &e
	}
	if _, err := m.checkLocal(version, key, entry); err != nil {
		return newErr("RUNTIME_VERIFY_FAILED", "The cached runtime failed verification", err,
			"Delete the cached runtime and download it again.")
	}
	return nil
}

// Delete removes a cached runtime.
func (m *Manager) Delete(version, key string) error {
	if err := checkVK(version, key); err != nil {
		return err
	}
	dir := m.runtimeDir(version, key)
	if _, err := os.Lstat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	_ = os.Remove(filepath.Dir(dir)) // only succeeds when empty
	return nil
}

// CacheSize returns total bytes used by the runtime cache.
func (m *Manager) CacheSize() int64 { return dirSize(m.cacheDir) }
