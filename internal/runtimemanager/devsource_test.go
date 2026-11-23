package runtimemanager

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDevSource(t *testing.T) {
	root := FindCheckout(".")
	if root == "" {
		t.Fatal("this test runs inside the MyPack checkout")
	}
	m := New(t.TempDir(), "https://registry.invalid/registry.json")
	if !m.SetDevSource(root) || m.DevSource() != root {
		t.Fatal("checkout not accepted")
	}
	if m.SetDevSource(t.TempDir()) {
		t.Fatal("a folder that is not a checkout was accepted")
	}
	m.SetDevSource(root)
	host := runtime.GOOS + "-" + runtime.GOARCH
	other := "windows-arm64"
	if host == other {
		other = "linux-amd64"
	}
	// A version no registry has: dev mode does not ask one.
	st, err := m.Status(context.Background(), "9.9.9", host)
	if err != nil || !st.Cached || !st.Verified || st.Source != SourceDev {
		t.Fatalf("status = %+v %v", st, err)
	}
	if e, ok := m.CachedEntry("9.9.9", host+"-lite"); !ok || e.Tier != TierLite || len(e.Capabilities) != 0 {
		t.Fatalf("lite entry = %+v %v", e, ok)
	}
	if _, ok := m.CachedEntry("9.9.9", other); ok {
		t.Fatal("another platform was served from source")
	}
	if testing.Short() {
		t.Skip("building the runtime takes a while")
	}
	exe, err := m.Ensure(context.Background(), "9.9.9", host, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(filepath.Dir(exe)) != filepath.Join(m.cacheDir, "_dev") {
		t.Fatalf("dev build outside _dev: %s", exe)
	}
	if fi, err := os.Stat(exe); err != nil || fi.Size() < 1<<20 {
		t.Fatalf("runtime binary = %v %v", fi, err)
	}
	if _, err := os.Stat(m.runtimeDir("9.9.9", host)); !os.IsNotExist(err) {
		t.Fatal("a source build went into the verified cache")
	}
}

func TestDevSourceFromEnv(t *testing.T) {
	t.Setenv(EnvDevRuntime, "0")
	if devSourceFromEnv() != "" {
		t.Fatal("0 turned dev mode on")
	}
	t.Setenv(EnvDevRuntime, "/nonexistent/place")
	if devSourceFromEnv() != "" {
		t.Fatal("a missing checkout turned dev mode on")
	}
	t.Setenv(EnvDevRuntime, "1")
	if devSourceFromEnv() == "" {
		t.Fatal("1 inside the checkout did not find it")
	}
}
