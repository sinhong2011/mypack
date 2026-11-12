package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"mypack.dev/mypack/pkg/config"
)

func TestConfigCandidates(t *testing.T) {
	c := ConfigCandidates("/x/App.app/Contents/MacOS", "")
	want := []string{
		"/x/App.app/Contents/Resources/mypack.json",
		"/x/App.app/Contents/MacOS/resources/mypack.json",
		"/x/App.app/Contents/MacOS/mypack.json",
	}
	for i := range want {
		if filepath.Clean(c[i]) != filepath.Clean(want[i]) {
			t.Errorf("candidate %d = %q want %q", i, filepath.Clean(c[i]), filepath.Clean(want[i]))
		}
	}
	if got := ConfigCandidates("/x", "/etc/m.json"); len(got) != 1 || got[0] != "/etc/m.json" {
		t.Errorf("env override: %v", got)
	}
}

func TestFindConfigAndParse(t *testing.T) {
	dir := t.TempDir()
	if _, err := FindConfig(dir, ""); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want not found, got %v", err)
	}
	os.MkdirAll(filepath.Join(dir, "resources"), 0o755)
	p := filepath.Join(dir, "resources", "mypack.json")
	os.WriteFile(p, []byte(`{"schemaVersion":1}`), 0o644)
	got, err := FindConfig(dir, "")
	if err != nil || got != p {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := ParseConfig([]byte(`{nope`)); err == nil {
		t.Fatal("broken json must fail")
	}
	if _, err := ParseConfig([]byte(`{"schemaVersion":1,"app":{"name":"","source":{"type":"url","value":"https://example.com"}}}`)); err == nil {
		t.Fatal("empty name must fail")
	}
	cfg, err := ParseConfig([]byte(`{"schemaVersion":1,"app":{"name":"T","identifier":"app.mypack.test","profileId":"app.mypack.test","version":"1.0.0","source":{"type":"url","value":"https://example.com"},"icon":{"source":"default"}},"runtime":{"version":"1.0.0"},"window":{"width":800,"height":600}}`))
	if err != nil {
		t.Fatalf("minimal config without build targets must be accepted: %v", err)
	}
	if cfg.App.Name != "T" {
		t.Fatal("name")
	}
}

func TestProfileDir(t *testing.T) {
	env := map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`}
	get := func(k string) string { return env[k] }
	if got := ProfileDir("darwin", "app.x", get, "/Users/u"); got != filepath.Join("/Users/u", "Library", "Application Support", "mypack-apps", "app.x") {
		t.Error(got)
	}
	if got := ProfileDir("linux", "app.x", get, "/home/u"); got != filepath.Join("/home/u", ".local", "share", "mypack-apps", "app.x") {
		t.Error(got)
	}
	xdg := "/data"
	if runtime.GOOS == "windows" {
		xdg = `C:\data` // "/data" is not an absolute path on Windows
	}
	env["XDG_DATA_HOME"] = xdg
	if got := ProfileDir("linux", "app.x", get, "/home/u"); got != filepath.Join(xdg, "mypack-apps", "app.x") {
		t.Error(got)
	}
	if got := ProfileDir("darwin", "../../etc", get, "/Users/u"); !strings.HasSuffix(got, filepath.Join("mypack-apps", "etc")) {
		t.Errorf("traversal: %s", got)
	}
}

func TestStateAndRestore(t *testing.T) {
	dir := t.TempDir()
	s := OpenStore(dir)
	s.Update(func(st *State) { st.LastURL = "https://example.com/a?b=1"; st.Zoom = 120; st.ZoomBase = 100 })
	s2 := OpenStore(dir)
	if s2.Get().LastURL != "https://example.com/a?b=1" || s2.Get().Zoom != 120 {
		t.Fatalf("state not persisted: %+v", s2.Get())
	}
	src := "https://example.com/"
	if ChooseStartURL(src, "https://example.com/a", true) != "https://example.com/a" {
		t.Error("same origin must restore")
	}
	if ChooseStartURL(src, "https://evil.com/a", true) != src {
		t.Error("other origin must not restore")
	}
	if ChooseStartURL(src, "http://example.com/a", true) != src {
		t.Error("scheme change must not restore")
	}
	if ChooseStartURL(src, "https://example.com/a", false) != src {
		t.Error("disabled")
	}
	if EffectiveZoom(100, State{Zoom: 120, ZoomBase: 100}) != 120 {
		t.Error("saved zoom")
	}
	if EffectiveZoom(110, State{Zoom: 120, ZoomBase: 100}) != 110 {
		t.Error("new configured zoom must win")
	}
}

func TestResolveLocal(t *testing.T) {
	root := t.TempDir()
	site := filepath.Join(root, "site")
	os.MkdirAll(filepath.Join(site, "sub"), 0o755)
	os.WriteFile(filepath.Join(site, "index.html"), []byte("<h1>hi</h1>"), 0o644)
	os.WriteFile(filepath.Join(site, "sub", "a b.html"), []byte("x"), 0o644)
	r, e, err := ResolveLocal(root, config.Source{Type: "dir", Value: "site"})
	if err != nil || r != site || e != "index.html" {
		t.Fatalf("%q %q %v", r, e, err)
	}
	if _, e, err = ResolveLocal(root, config.Source{Type: "dir", Value: "site", Entry: "../../etc/passwd"}); err == nil {
		t.Fatalf("entry traversal accepted: %q", e)
	}
	r, e, err = ResolveLocal(root, config.Source{Type: "file", Value: "site/sub/a b.html"})
	if err != nil || r != filepath.Join(site, "sub") || e != "a b.html" {
		t.Fatalf("%q %q %v", r, e, err)
	}
	if entryURLPath("sub/a b.html") != "sub/a%20b.html" {
		t.Error(entryURLPath("sub/a b.html"))
	}
	if _, _, err := ResolveLocal(root, config.Source{Type: "dir", Value: "missing"}); err == nil {
		t.Fatal("missing dir must fail")
	}
}

func TestLoopbackHandler(t *testing.T) {
	h := LoopbackHandler("tok", "127.0.0.1:1234", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("path=" + r.URL.Path))
	}))
	do := func(host, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "http://"+host+path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if r := do("127.0.0.1:1234", "/tok/css/a.css"); r.Code != 200 || r.Body.String() != "path=/css/a.css" {
		t.Errorf("valid: %d %s", r.Code, r.Body)
	}
	if r := do("127.0.0.1:1234", "/other/css/a.css"); r.Code != 404 {
		t.Errorf("no token: %d", r.Code)
	}
	if r := do("127.0.0.1:1234", "/tokx/a"); r.Code != 404 {
		t.Errorf("token prefix confusion: %d", r.Code)
	}
	if r := do("evil.example", "/tok/a"); r.Code != 403 {
		t.Errorf("rebinding host: %d", r.Code)
	}
}

func TestUniquePath(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "a.txt")
	os.WriteFile(p, nil, 0o644)
	if got := UniquePath(p); got != filepath.Join(d, "a (2).txt") {
		t.Error(got)
	}
}

func TestSiteHandlerNoRedirects(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("ROOT"), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "index.html"), []byte("SUB"), 0o644)
	os.WriteFile(filepath.Join(dir, "a.css"), []byte("CSS"), 0o644)
	h := siteHandler(os.DirFS(dir))
	for path, want := range map[string]string{"/": "ROOT", "/index.html": "ROOT", "/sub": "SUB", "/sub/": "SUB", "/sub/index.html": "SUB", "/a.css": "CSS", "/../../etc/passwd": "ROOT"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "http://x"+path, nil))
		if rec.Code != 200 || rec.Body.String() != want {
			t.Errorf("%s: %d %q want %q", path, rec.Code, rec.Body.String(), want)
		}
	}
	if entryURLPath("index.html") != "" || entryURLPath("sub/index.html") != "sub/" {
		t.Error("entry url")
	}
}

func TestSiteHandlerSPAFallback(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>shell</title>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	h := siteHandler(fsys)
	get := func(p string) (int, string) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		return rec.Code, rec.Body.String()
	}
	if code, body := get("/dashboard/settings"); code != 200 || !strings.Contains(body, "shell") {
		t.Fatalf("client route: %d %q", code, body)
	}
	if code, _ := get("/assets/app.js"); code != 200 {
		t.Fatalf("asset: %d", code)
	}
	if code, _ := get("/assets/missing.js"); code != 404 {
		t.Fatalf("missing asset must 404, got %d", code)
	}
}
