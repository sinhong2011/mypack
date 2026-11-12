package runtimemanager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"mypack.dev/mypack/internal/runtimemanager/testregistry"
)

func TestParseArchiveName(t *testing.T) {
	v, osName, arch, ok := ParseArchiveName("mypack-runtime-1.0.8-darwin-arm64.tar.gz")
	if !ok || v != "1.0.8" || osName != "darwin" || arch != "arm64" {
		t.Fatalf("%s %s %s %v", v, osName, arch, ok)
	}
	if _, _, _, ok := ParseArchiveName("mypack-1.0.8-darwin-arm64.tar.gz"); ok {
		t.Fatal("should reject")
	}
}

func TestListDoesNotHitRegistry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	m := New(t.TempDir(), srv.URL+"/registry.json")
	list, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("%+v", list)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("List contacted the registry %d times", hits)
	}
}

func TestSeedFromDirOfflineEnsure(t *testing.T) {
	reg, err := testregistry.Create(t.TempDir(), testregistry.Options{Keys: []string{"darwin-arm64", "linux-amd64"}})
	if err != nil {
		t.Fatal(err)
	}
	bundle := t.TempDir()
	for _, name := range []string{"registry.json", "registry.json.sig"} {
		b, err := os.ReadFile(filepath.Join(reg.Dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bundle, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	arcName := "mypack-runtime-1.0.0-darwin-arm64.tar.gz"
	b, err := os.ReadFile(filepath.Join(reg.Dir, "1.0.0", arcName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, arcName), b, 0o644); err != nil {
		t.Fatal(err)
	}

	var registryHits, archiveHits int32
	fs := http.FileServer(http.Dir(reg.Dir))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "registry.json") || strings.HasSuffix(r.URL.Path, "registry.json.sig") {
			atomic.AddInt32(&registryHits, 1)
		}
		if strings.HasSuffix(r.URL.Path, ".tar.gz") {
			atomic.AddInt32(&archiveHits, 1)
		}
		fs.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(t.TempDir(), srv.URL+"/registry.json")
	m.SetTrustedKeys(reg.Public)
	if err := m.SeedFromDir(bundle); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&registryHits) != 0 || atomic.LoadInt32(&archiveHits) != 0 {
		t.Fatalf("SeedFromDir hit the network registry=%d archive=%d", registryHits, archiveHits)
	}
	list, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("%+v", list)
	}
	var cached, listed int
	for _, ri := range list {
		if ri.Version != "1.0.0" {
			t.Fatalf("%+v", ri)
		}
		if ri.Cached && ri.Verified && ri.Key == "darwin-arm64" {
			cached++
		}
		if ri.Key == "linux-amd64" && !ri.Cached {
			listed++
		}
	}
	if cached != 1 || listed != 1 {
		t.Fatalf("cached=%d listed-uncached=%d list=%+v", cached, listed, list)
	}
	exe, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(exe) != "mypack-runtime" {
		t.Fatal(exe)
	}
	if atomic.LoadInt32(&registryHits) != 0 || atomic.LoadInt32(&archiveHits) != 0 {
		t.Fatalf("Ensure of bundled runtime hit the network registry=%d archive=%d", registryHits, archiveHits)
	}
	if _, err := m.Ensure(context.Background(), "1.0.0", "linux-amd64", nil); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&registryHits) != 0 {
		t.Fatalf("cross-platform Ensure fetched registry.json %d times", registryHits)
	}
	if atomic.LoadInt32(&archiveHits) != 1 {
		t.Fatalf("cross-platform Ensure downloaded archives %d times, want 1", archiveHits)
	}
}

func TestSeedFromDirHelperBinary(t *testing.T) {
	reg, err := testregistry.Create(t.TempDir(), testregistry.Options{Keys: []string{"darwin-arm64"}})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	runtimes := filepath.Join(root, "runtimes")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(runtimes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"registry.json", "registry.json.sig"} {
		b, err := os.ReadFile(filepath.Join(reg.Dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runtimes, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(runtimes, "bundled.json"), []byte(`{"version":"1.0.0","keys":["darwin-arm64"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Bytes differ from the archive (as after codesign of the helper).
	if err := os.WriteFile(filepath.Join(bin, "mypack-runtime"), []byte("signed-helper"), 0o755); err != nil {
		t.Fatal(err)
	}

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(t.TempDir(), srv.URL+"/registry.json")
	m.SetTrustedKeys(reg.Public)
	if err := m.SeedFromDir(runtimes); err != nil {
		t.Fatal(err)
	}
	exe, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "signed-helper" {
		t.Fatalf("seeded %q", got)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("helper seed hit the network %d times", hits)
	}
}

func TestSeedFromDirMissingIsNoop(t *testing.T) {
	m := New(t.TempDir(), "https://127.0.0.1:1/registry.json")
	if err := m.SeedFromDir(""); err != nil {
		t.Fatal(err)
	}
	if err := m.SeedFromDir(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultBundledDirEnv(t *testing.T) {
	t.Setenv("MYPACK_BUNDLED_RUNTIMES", "/tmp/bundled-runtimes-test")
	if DefaultBundledDir() != "/tmp/bundled-runtimes-test" {
		t.Fatal(DefaultBundledDir())
	}
}

func TestStatusUnknownVersionFetches(t *testing.T) {
	reg, m := newReg(t, testregistry.Options{})
	if _, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil); err != nil {
		t.Fatal(err)
	}
	var hits int32
	fs := http.FileServer(http.Dir(reg.Dir))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		fs.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	m.SetRegistryURL(srv.URL + "/registry.json")
	m.mu.Lock()
	m.memo = nil
	m.mu.Unlock()
	if _, err := m.Status(context.Background(), "1.0.0", "darwin-arm64"); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("Status of cached runtime hit the network %d times", hits)
	}
	if _, err := m.Status(context.Background(), "9.9.9", "darwin-arm64"); err == nil {
		t.Fatal("expected not found")
	}
	if atomic.LoadInt32(&hits) == 0 {
		t.Fatal("Status of an unknown version should fetch the registry")
	}
}
