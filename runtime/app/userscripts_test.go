package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newGMHarness(t *testing.T, grants map[string]bool, connect []string) (*harness, *gmService, *string) {
	t.Helper()
	h := newHarness("allow")
	var clip string
	gm := &gmService{
		dir:     t.TempDir(),
		client:  http.DefaultClient,
		scripts: map[string]*gmScript{"tok": {id: "s-1", name: "S", grants: grants, connect: connect}},
		vals:    map[string]map[string]json.RawMessage{},
		clip:    func(s string) { clip = s },
	}
	h.b.gm = gm
	return h, gm, &clip
}

func TestGMValues(t *testing.T) {
	h, gm, _ := newGMHarness(t, map[string]bool{"values": true}, nil)
	ctx := context.Background()
	if err := h.b.GMSetValue(ctx, "tok", "k", `{"a":1}`); err != nil {
		t.Fatal(err)
	}
	if err := h.b.GMSetValue(ctx, "tok", "bad", `{nope`); err == nil {
		t.Fatal("invalid JSON stored")
	}
	if _, err := h.b.GMValues(ctx, "other"); err == nil {
		t.Fatal("unknown token accepted")
	}
	// Persisted: a fresh service reads the file back.
	gm2 := &gmService{dir: gm.dir, vals: map[string]map[string]json.RawMessage{}}
	gm2.mu.Lock()
	v := gm2.values("s-1")
	gm2.mu.Unlock()
	if string(v["k"]) != `{"a":1}` {
		t.Fatalf("persisted values = %v", v)
	}
	if err := h.b.GMDeleteValue(ctx, "tok", "k"); err != nil {
		t.Fatal(err)
	}
	if got, _ := h.b.GMValues(ctx, "tok"); len(got) != 0 {
		t.Fatalf("after delete = %v", got)
	}
	h.url = "https://evil.example.net/"
	if err := h.b.GMSetValue(ctx, "tok", "k", "1"); err == nil {
		t.Fatal("a page outside the app wrote values")
	}
}

func TestGMValuesNeedGrant(t *testing.T) {
	h, _, _ := newGMHarness(t, map[string]bool{}, nil)
	if err := h.b.GMSetValue(context.Background(), "tok", "k", "1"); err == nil {
		t.Fatal("set without grant")
	}
	if v, err := h.b.GMValues(context.Background(), "tok"); err != nil || len(v) != 0 {
		t.Fatalf("values without grant = %v %v", v, err)
	}
}

func TestGMXhr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Seen", r.Header.Get("X-Test")+"|"+r.Method+"|"+string(b)+"|"+r.Header.Get("Cookie"))
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	host := mustHost(srv.URL)
	h, _, _ := newGMHarness(t, map[string]bool{"xmlhttprequest": true}, []string{host})
	ctx := context.Background()
	r, err := h.b.GMXhr(ctx, "tok", GMXhrRequest{Method: "post", URL: srv.URL + "/x", Data: "hi", Headers: map[string]string{"X-Test": "1", "Host": "evil"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != 200 || r.Text != `{"ok":true}` || r.MIME != "application/json" || !strings.Contains(r.Headers, "x-seen: 1|POST|hi|\r\n") {
		t.Fatalf("resp = %+v", r)
	}
	r, err = h.b.GMXhr(ctx, "tok", GMXhrRequest{URL: srv.URL, ResponseType: "arraybuffer"})
	if err != nil || r.BodyBase64 == "" || r.Text != "" {
		t.Fatalf("arraybuffer = %+v %v", r, err)
	}
	if _, err := h.b.GMXhr(ctx, "tok", GMXhrRequest{URL: "https://not-listed.test/"}); err == nil || !strings.Contains(err.Error(), "connect-denied") {
		t.Fatalf("unlisted host: %v", err)
	}
	if _, err := h.b.GMXhr(ctx, "tok", GMXhrRequest{URL: "file:///etc/passwd"}); err == nil {
		t.Fatal("file URL fetched")
	}
	if _, err := h.b.GMXhr(ctx, "tok", GMXhrRequest{URL: srv.URL, Method: "TRACE"}); err == nil {
		t.Fatal("TRACE allowed")
	}
	h2, _, _ := newGMHarness(t, map[string]bool{}, []string{"*"})
	if _, err := h2.b.GMXhr(ctx, "tok", GMXhrRequest{URL: srv.URL}); err == nil {
		t.Fatal("xhr without grant")
	}
}

func TestConnectAllowed(t *testing.T) {
	page := "https://x.com/home"
	for host, want := range map[string]bool{
		"x.com": true, "api.x.com": true, "evilx.com": false,
		"cdn.example.com": true, "example.com.evil": false,
	} {
		if got := connectAllowed([]string{"example.com"}, page, host); got != want {
			t.Errorf("connectAllowed(%s) = %v", host, got)
		}
	}
	if !connectAllowed([]string{"*"}, page, "anything.test") {
		t.Error("* should allow every host")
	}
}

func TestGMClipboard(t *testing.T) {
	h, _, clip := newGMHarness(t, map[string]bool{"setclipboard": true}, nil)
	if err := h.b.GMSetClipboard(context.Background(), "tok", "copied"); err != nil || *clip != "copied" {
		t.Fatalf("clipboard = %q %v", *clip, err)
	}
}

func TestValuesFileName(t *testing.T) {
	gm := &gmService{dir: t.TempDir(), vals: map[string]map[string]json.RawMessage{}}
	gm.mu.Lock()
	gm.values("../../escape")["k"] = json.RawMessage("1")
	err := gm.save("../../escape")
	gm.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(gm.dir, "escape.json")); err != nil {
		t.Fatalf("values written outside the folder: %v", err)
	}
}

func mustHost(raw string) string {
	u, _ := url.Parse(raw)
	return u.Hostname()
}
