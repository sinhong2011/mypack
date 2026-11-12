package runtimemanager

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/runtimemanager/testregistry"
)

func newReg(t *testing.T, o testregistry.Options) (*testregistry.Registry, *Manager) {
	t.Helper()
	reg, err := testregistry.Create(t.TempDir(), o)
	if err != nil {
		t.Fatal(err)
	}
	m := New(t.TempDir(), reg.URL)
	m.SetTrustedKeys(reg.Public)
	return reg, m
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != code {
		t.Fatalf("want error code %s, got %v", code, err)
	}
	if de.Remediation == "" && code != "RUNTIME_NOT_FOUND" {
		t.Errorf("error %s has no remediation", code)
	}
}

func TestEnsureDownloadsVerifiesAndCaches(t *testing.T) {
	_, m := newReg(t, testregistry.Options{Keys: []string{"darwin-arm64", "windows-amd64"}})
	var last, total int64
	exe, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", func(d, tt int64) { last, total = d, tt })
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(exe) != "mypack-runtime" {
		t.Fatal(exe)
	}
	st, err := os.Stat(exe)
	if err != nil || (runtime.GOOS != "windows" && st.Mode()&0o111 == 0) {
		t.Fatalf("not executable: %v %v", st, err)
	}
	if last == 0 || last != total {
		t.Fatalf("progress %d/%d", last, total)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), ".verified")); err != nil {
		t.Fatal("marker missing")
	}
	exeWin, err := m.Ensure(context.Background(), "1.0.0", "windows-amd64", nil)
	if err != nil || filepath.Base(exeWin) != "mypack-runtime.exe" {
		t.Fatalf("%v %v", exeWin, err)
	}
	if err := m.Verify(context.Background(), "1.0.0", "darwin-arm64"); err != nil {
		t.Fatal(err)
	}
	if m.CacheSize() == 0 {
		t.Fatal("cache size")
	}
	// second Ensure must not touch the network: break the registry
	os.RemoveAll(strings.TrimPrefix(strings.TrimSuffix(m.RegistryURL(), "/registry.json"), "file://"))
	m2 := New(m.CacheDir(), m.RegistryURL())
	m2.keys = m.keys
	if p, err := m2.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil); err != nil || p != exe {
		t.Fatalf("offline cached ensure: %v %v", p, err)
	}
}

func TestTamperedBinaryFailsVerify(t *testing.T) {
	_, m := newReg(t, testregistry.Options{})
	exe, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(exe, []byte("evil"), 0o755)
	wantCode(t, m.Verify(context.Background(), "1.0.0", "darwin-arm64"), "RUNTIME_VERIFY_FAILED")
	// Ensure repairs it by re-downloading
	exe2, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(exe2)
	if string(b) == "evil" {
		t.Fatal("not repaired")
	}
}

func TestNotFound(t *testing.T) {
	_, m := newReg(t, testregistry.Options{})
	_, err := m.Ensure(context.Background(), "9.9.9", "darwin-arm64", nil)
	wantCode(t, err, "RUNTIME_NOT_FOUND")
	_, err = m.Ensure(context.Background(), "../x", "darwin-arm64", nil)
	wantCode(t, err, "RUNTIME_NOT_FOUND")
}

func TestBadSignatureRejected(t *testing.T) {
	reg, m := newReg(t, testregistry.Options{})
	// trusted key differs
	other, _ := testregistry.Create(t.TempDir(), testregistry.Options{})
	m.SetTrustedKeys(other.Public)
	_, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	wantCode(t, err, "RUNTIME_VERIFY_FAILED")
	// tampered manifest
	m.SetTrustedKeys(reg.Public)
	p := filepath.Join(reg.Dir, "registry.json")
	b, _ := os.ReadFile(p)
	os.WriteFile(p, bytes.Replace(b, []byte(`"size"`), []byte(`"size" `), 1), 0o644)
	m.SetRegistryURL(reg.URL)
	_, err = m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	wantCode(t, err, "RUNTIME_VERIFY_FAILED")
	// no keys configured at all
	m.SetTrustedKeys()
	m.keys = nil
	t.Setenv("MYPACK_TRUSTED_KEY", "")
	t.Setenv("MYPACK_DEV_TRUST", "")
	ResetTrustedKeys()
	_, err = m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	wantCode(t, err, "RUNTIME_VERIFY_FAILED")
}

func TestExpiredManifestRejected(t *testing.T) {
	_, m := newReg(t, testregistry.Options{Expires: time.Now().Add(-time.Hour), Now: time.Now().Add(-2 * time.Hour)})
	_, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	wantCode(t, err, "RUNTIME_VERIFY_FAILED")
	if !strings.Contains(err.Error(), "expired") {
		t.Fatal(err)
	}
}

func TestRollbackRejected(t *testing.T) {
	reg, m := newReg(t, testregistry.Options{Sequence: 5})
	if _, err := m.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	// registry now serves sequence 3 (older, validly signed)
	mf := reg.Manifest
	mf.Sequence = 3
	if err := reg.Write(mf); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.memo = nil
	m.mu.Unlock()
	_, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	wantCode(t, err, "RUNTIME_VERIFY_FAILED")
	if !strings.Contains(err.Error(), "rollback") {
		t.Fatal(err)
	}
	// equal or higher is fine
	mf.Sequence = 6
	reg.Write(mf)
	if _, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil); err != nil {
		t.Fatal(err)
	}
}

func TestOfflineListUsesCachedManifest(t *testing.T) {
	reg, m := newReg(t, testregistry.Options{Keys: []string{"darwin-arm64", "linux-amd64"}})
	if _, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(reg.Dir) // registry gone
	m.mu.Lock()
	m.memo = nil
	m.mu.Unlock()
	list, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("%+v", list)
	}
	cached := 0
	for _, ri := range list {
		if ri.Cached && ri.Verified {
			cached++
		}
	}
	if cached != 1 {
		t.Fatalf("%+v", list)
	}
	// ensure for something not cached while offline → download failed
	_, err = m.Ensure(context.Background(), "1.0.0", "linux-amd64", nil)
	wantCode(t, err, "RUNTIME_DOWNLOAD_FAILED")
	st, err := m.Status(context.Background(), "1.0.0", "darwin-arm64")
	if err != nil || !st.Cached || !st.Verified || st.Path == "" {
		t.Fatalf("%+v %v", st, err)
	}
	if _, err := m.Status(context.Background(), "7.7.7", "darwin-arm64"); err == nil {
		t.Fatal("expected not found")
	}
}

func TestDelete(t *testing.T) {
	_, m := newReg(t, testregistry.Options{})
	exe, _ := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	if err := m.Delete("1.0.0", "darwin-arm64"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(exe); !os.IsNotExist(err) {
		t.Fatal("still there")
	}
	if err := m.Delete("1.0.0", "darwin-arm64"); err != nil {
		t.Fatal("second delete should be a no-op")
	}
	if err := m.Delete("..", "x"); err == nil {
		t.Fatal("traversal must be refused")
	}
}

// ---- malicious archives ----

type tarEntry struct {
	name, link string
	typ        byte
	body       string
	mode       int64
}

func makeTarGz(entries []tarEntry) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Linkname: e.link, Typeflag: e.typ, Mode: e.mode, Size: int64(len(e.body))}
		if e.typ != tar.TypeReg {
			h.Size = 0
		}
		tw.WriteHeader(h)
		if e.typ == tar.TypeReg {
			tw.Write([]byte(e.body))
		}
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

func TestExtractRejectsMalicious(t *testing.T) {
	bad := map[string][]tarEntry{
		"traversal":    {{name: "../evil", typ: tar.TypeReg, body: "x", mode: 0o755}},
		"nested-trav":  {{name: "a/../../evil", typ: tar.TypeReg, body: "x", mode: 0o644}},
		"absolute":     {{name: "/etc/evil", typ: tar.TypeReg, body: "x", mode: 0o644}},
		"symlink":      {{name: "mypack-runtime", link: "/bin/sh", typ: tar.TypeSymlink}},
		"hardlink":     {{name: "mypack-runtime", link: "other", typ: tar.TypeLink}},
		"device":       {{name: "dev", typ: tar.TypeChar}},
		"fifo":         {{name: "pipe", typ: tar.TypeFifo}},
		"windows-path": {{name: `..\evil`, typ: tar.TypeReg, body: "x", mode: 0o644}},
	}
	for name, es := range bad {
		dir := t.TempDir()
		arc := filepath.Join(dir, "a.tar.gz")
		os.WriteFile(arc, makeTarGz(es), 0o644)
		out := filepath.Join(dir, "out")
		os.Mkdir(out, 0o755)
		if err := ExtractTarGz(arc, out); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
		if _, err := os.Stat(filepath.Join(dir, "evil")); err == nil {
			t.Errorf("%s: file escaped", name)
		}
	}
	// a benign archive with ./ prefix and a dir works
	dir := t.TempDir()
	arc := filepath.Join(dir, "a.tar.gz")
	os.WriteFile(arc, makeTarGz([]tarEntry{{name: "./", typ: tar.TypeDir, mode: 0o755}, {name: "./mypack-runtime", typ: tar.TypeReg, body: "ok", mode: 0o755}}), 0o644)
	out := filepath.Join(dir, "out")
	os.Mkdir(out, 0o755)
	if err := ExtractTarGz(arc, out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "mypack-runtime")); string(b) != "ok" {
		t.Fatal("content")
	}
}

func TestExtractSizeCap(t *testing.T) {
	old := maxExtract
	maxExtract = 4 << 20
	defer func() { maxExtract = old }()
	dir := t.TempDir()
	arc := filepath.Join(dir, "a.tar.gz")
	big := strings.Repeat("\x00", int(maxExtract)/4)
	os.WriteFile(arc, makeTarGz([]tarEntry{
		{name: "a", typ: tar.TypeReg, body: big, mode: 0o644}, {name: "b", typ: tar.TypeReg, body: big, mode: 0o644},
		{name: "c", typ: tar.TypeReg, body: big, mode: 0o644}, {name: "d", typ: tar.TypeReg, body: big, mode: 0o644},
		{name: "e", typ: tar.TypeReg, body: "x", mode: 0o644},
	}), 0o644)
	out := filepath.Join(dir, "out")
	os.Mkdir(out, 0o755)
	if err := ExtractTarGz(arc, out); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected size cap error, got %v", err)
	}
}

// A signed registry that points to a malicious archive (right hash, evil content)
// must be rejected and must not destroy an already-verified cache.
func TestMaliciousArchiveViaRegistryKeepsOldCache(t *testing.T) {
	reg, m := newReg(t, testregistry.Options{})
	exe, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(exe)
	// registry re-publishes 1.0.0 with a traversal archive
	evil := makeTarGz([]tarEntry{{name: "../../evil", typ: tar.TypeReg, body: "x", mode: 0o755}})
	fn := reg.Manifest.Runtimes[0].URL
	os.WriteFile(filepath.Join(reg.Dir, filepath.FromSlash(fn)), evil, 0o644)
	sum := sha256.Sum256(evil)
	mf := reg.Manifest
	mf.Sequence++
	mf.Runtimes = append([]testregistry.Entry(nil), mf.Runtimes...)
	mf.Runtimes[0].SHA256 = hex.EncodeToString(sum[:])
	mf.Runtimes[0].Size = int64(len(evil))
	reg.Write(mf)
	m.mu.Lock()
	m.memo = nil
	m.mu.Unlock()
	// Ensure serves the verified cache without network while the cached manifest agrees;
	// refreshing the manifest (e.g. List) makes the cache stale and forces a re-download.
	if _, err = m.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	wantCode(t, err, "RUNTIME_VERIFY_FAILED")
	after, rerr := os.ReadFile(exe)
	if rerr != nil || !bytes.Equal(before, after) {
		t.Fatal("existing cache was damaged")
	}
	// the cache dir has no stray temp dirs
	ents, _ := os.ReadDir(m.CacheDir())
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("temp dir left behind: %s", e.Name())
		}
	}
}

func TestChecksumMismatch(t *testing.T) {
	reg, m := newReg(t, testregistry.Options{})
	p := filepath.Join(reg.Dir, "1.0.0", reg.Manifest.Runtimes[0].Filename)
	b, _ := os.ReadFile(p)
	b[len(b)-1] ^= 0xff
	os.WriteFile(p, b, 0o644)
	_, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	wantCode(t, err, "RUNTIME_VERIFY_FAILED")
	if _, err := os.Stat(m.runtimeDir("1.0.0", "darwin-arm64")); err == nil {
		t.Fatal("must not install")
	}
}

func TestHTTPRegistryAndCoalescing(t *testing.T) {
	reg, _ := newReg(t, testregistry.Options{})
	var archiveHits int32
	fs := http.FileServer(http.Dir(reg.Dir))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".tar.gz") {
			atomic.AddInt32(&archiveHits, 1)
			time.Sleep(150 * time.Millisecond)
		}
		fs.ServeHTTP(w, r)
	}))
	defer srv.Close()
	m := New(t.TempDir(), srv.URL+"/registry.json")
	m.SetTrustedKeys(reg.Public)
	var wg sync.WaitGroup
	paths := make([]string, 8)
	errs := make([]error, 8)
	for i := range paths {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			paths[i], errs[i] = m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
		}(i)
	}
	wg.Wait()
	for i := range paths {
		if errs[i] != nil || paths[i] != paths[0] {
			t.Fatalf("%d: %v %v", i, paths[i], errs[i])
		}
	}
	if n := atomic.LoadInt32(&archiveHits); n != 1 {
		t.Fatalf("archive downloaded %d times, want 1", n)
	}
}

func TestCancelLeavesNoDamage(t *testing.T) {
	reg, _ := newReg(t, testregistry.Options{})
	fs := http.FileServer(http.Dir(reg.Dir))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".tar.gz") {
			select {
			case <-time.After(5 * time.Second):
			case <-r.Context().Done():
				return
			}
		}
		fs.ServeHTTP(w, r)
	}))
	defer srv.Close()
	m := New(t.TempDir(), srv.URL+"/registry.json")
	m.SetTrustedKeys(reg.Public)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := m.Ensure(ctx, "1.0.0", "darwin-arm64", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline exceeded, got %v", err)
	}
	if _, err := os.Stat(m.runtimeDir("1.0.0", "darwin-arm64")); err == nil {
		t.Fatal("partial install")
	}
}

func TestTrustKeySources(t *testing.T) {
	ResetTrustedKeys()
	t.Cleanup(ResetTrustedKeys)
	// Every build trusts the embedded production key.
	base := len(parseKeys(productionKeyB64))
	if base != 1 {
		t.Fatalf("embedded production key: got %d keys, want 1", base)
	}
	reg, _ := newReg(t, testregistry.Options{})
	m := New(t.TempDir(), reg.URL) // no per-manager keys
	t.Setenv("MYPACK_DEV_TRUST", "")
	t.Setenv("MYPACK_TRUSTED_KEY", "")
	if _, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil); err == nil {
		t.Fatal("must fail with no keys")
	}
	AddTrustedKey(reg.Public)
	if _, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil); err != nil {
		t.Fatal(err)
	}
	ResetTrustedKeys()
	// env var
	t.Setenv("MYPACK_TRUSTED_KEY", encodeKey(reg.Public))
	if len(TrustedKeys()) != base+1 {
		t.Fatal("env key not used")
	}
	t.Setenv("MYPACK_TRUSTED_KEY", "")
	// dev trust file
	f := filepath.Join(t.TempDir(), "registry.pub")
	os.WriteFile(f, []byte(encodeKey(reg.Public)+"\n"), 0o644)
	t.Setenv("MYPACK_DEV_PUBKEY_FILE", f)
	if len(TrustedKeys()) != base+0 {
		t.Fatal("dev key must require MYPACK_DEV_TRUST=1")
	}
	t.Setenv("MYPACK_DEV_TRUST", "1")
	if len(TrustedKeys()) != base+1 {
		t.Fatal("dev key not used")
	}
	// ldflags-style list with rotation
	trustedKeysB64 = encodeKey(reg.Public) + "," + encodeKey(reg.Public)
	defer func() { trustedKeysB64 = "" }()
	if len(TrustedKeys()) != base+3 {
		t.Fatalf("got %d keys", len(TrustedKeys()))
	}
}
