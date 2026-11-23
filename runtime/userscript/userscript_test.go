package userscript

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `// ==UserScript==
// @name         Thread View
// @name:zh-TW   串視圖
// @namespace    https://example.com
// @version      1.2.0
// @match        https://x.com/*
// @match        *://*.twitter.com/*/status/*
// @exclude      https://x.com/settings*
// @grant        GM_getValue
// @grant        GM.xmlHttpRequest
// @connect      api.example.com
// @require      lib.js
// @resource     css https://cdn.example.com/a.css
// @run-at       document-start
// ==/UserScript==
console.log('hi');
`

func TestParse(t *testing.T) {
	m, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "Thread View" || m.Version != "1.2.0" || m.RunAt != "document-start" {
		t.Fatalf("meta = %+v", m)
	}
	if len(m.Match) != 2 || len(m.Exclude) != 1 || len(m.Require) != 1 || len(m.Resource) != 1 {
		t.Fatalf("lists = %+v", m)
	}
	g := m.Grants()
	if !g["values"] || !g["xmlhttprequest"] || g["setclipboard"] {
		t.Fatalf("grants = %v", g)
	}
	if id := m.ID(); !strings.HasPrefix(id, "thread-view-") || len(id) != len("thread-view-")+8 {
		t.Fatalf("id = %q", id)
	}
	if _, err := Parse("console.log(1)"); !errors.Is(err, ErrNoHeader) {
		t.Fatalf("no header: %v", err)
	}
	if m, _ := Parse("// ==UserScript==\n// @name a\n// ==/UserScript==\n"); m.RunAt != "document-idle" {
		t.Fatalf("default run-at = %q", m.RunAt)
	}
}

func TestRules(t *testing.T) {
	m, _ := Parse(sample)
	inc, exc, err := m.Rules()
	if err != nil {
		t.Fatal(err)
	}
	for u, want := range map[string]bool{
		"https://x.com/home":                    true,
		"https://x.com/settings/account":        false,
		"https://mobile.twitter.com/a/status/1": true,
		"https://twitter.com/a/status/1":        true,
		"https://twitter.com/home":              false,
		"https://evil.com/?https://x.com/":      false,
		"https://x.com.evil.com/":               false,
		"http://x.com/home":                     false,
		"https://eviltwitter.com/a/status/1":    false,
	} {
		if got := Matches(inc, exc, u); got != want {
			t.Errorf("Matches(%s) = %v, want %v", u, got, want)
		}
	}
	if _, err := MatchPattern("x.com/*"); err == nil {
		t.Error("pattern without scheme accepted")
	}
	if r := Glob("/^https:\\/\\/a\\.com/i"); !r.IgnoreCase || !r.Test("HTTPS://A.COM/x") {
		t.Errorf("regex include = %+v", r)
	}
	none, _ := Parse("// ==UserScript==\n// @name a\n// ==/UserScript==\n")
	inc, exc, _ = none.Rules()
	if !Matches(inc, exc, "https://anything.test/") {
		t.Error("a script without @match should run everywhere")
	}
}

func TestStoreInstallLoad(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	fetched := map[string]int{}
	fetch := func(_ context.Context, u string) ([]byte, string, error) {
		fetched[u]++
		if strings.HasSuffix(u, ".css") {
			return []byte("body{}"), "text/css; charset=utf-8", nil
		}
		return []byte("var lib = 1;"), "", nil
	}
	e, _, err := s.Install(context.Background(), []byte(sample), InstallOptions{SourceURL: "https://example.com/s/thread.user.js", Fetch: fetch})
	if err != nil {
		t.Fatal(err)
	}
	if fetched["https://example.com/s/lib.js"] != 1 {
		t.Fatalf("relative @require not resolved: %v", fetched)
	}
	if len(e.Resources) != 1 || e.Resources[0].MIME != "text/css" {
		t.Fatalf("resources = %+v", e.Resources)
	}
	if _, err := s.SetApps(e.ID, []string{"com.example.x"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Load("com.other"); len(got) != 0 {
		t.Fatalf("loaded in an app out of scope: %d", len(got))
	}
	got, errs := s.Load("com.example.x")
	if len(errs) != 0 || len(got) != 1 || got[0].Requires[0] != "var lib = 1;" || string(got[0].Resources[0].Data) != "body{}" {
		t.Fatalf("load = %+v %v", got, errs)
	}
	// Reinstall keeps scope and enabled state.
	if _, err := s.SetEnabled(e.ID, false); err != nil {
		t.Fatal(err)
	}
	e2, _, err := s.Install(context.Background(), []byte(strings.Replace(sample, "1.2.0", "1.3.0", 1)), InstallOptions{Fetch: fetch})
	if err != nil {
		t.Fatal(err)
	}
	if e2.Enabled || len(e2.Apps) != 1 || e2.Version != "1.3.0" || e2.SourceURL == "" {
		t.Fatalf("reinstall = %+v", e2)
	}
	if got, _ := s.Load("com.example.x"); len(got) != 0 {
		t.Fatal("a disabled script loaded")
	}
	if err := s.Remove(e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, e.ID)); !os.IsNotExist(err) {
		t.Fatalf("files left after remove: %v", err)
	}
	if err := s.Remove(e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second remove: %v", err)
	}
}

func TestInstallFailedDownloadKeepsOld(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	ok := func(context.Context, string) ([]byte, string, error) { return []byte("x"), "", nil }
	if _, _, err := s.Install(context.Background(), []byte(sample), InstallOptions{Fetch: ok}); err != nil {
		t.Fatal(err)
	}
	bad := func(context.Context, string) ([]byte, string, error) { return nil, "", errors.New("offline") }
	if _, _, err := s.Install(context.Background(), []byte(strings.Replace(sample, "hi", "new", 1)), InstallOptions{Fetch: bad}); err == nil {
		t.Fatal("install with a failed @require succeeded")
	}
	got, _ := s.Load("any")
	if len(got) != 1 || !strings.Contains(got[0].Code, "'hi'") {
		t.Fatalf("old copy lost: %+v", got)
	}
}

func TestInstallHashAndSites(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	none := func(context.Context, string) ([]byte, string, error) { return []byte("x"), "", nil }
	if _, _, err := s.Install(context.Background(), []byte(sample), InstallOptions{Fetch: none, SHA256: strings.Repeat("0", 64)}); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("hash mismatch: %v", err)
	}
	e, _, err := s.Install(context.Background(), []byte(sample), InstallOptions{Fetch: none, SHA256: "d9ed1c8b3a8cbd5cc8d4f5b06f8d5c0b2fb8e61dd27dcd2d0c4b4d6c6b0f0b61"})
	if err == nil {
		t.Fatalf("a wrong hash installed %s", e.ID)
	}
	m, _ := Parse(sample)
	if got := strings.Join(m.Sites(), ","); got != "x.com,*.twitter.com" {
		t.Fatalf("sites = %q", got)
	}
	if _, _, err := s.Install(context.Background(), []byte(sample), InstallOptions{Fetch: none}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ReadMeta(m.ID()); err != nil || got.Name != m.Name {
		t.Fatalf("ReadMeta = %+v %v", got, err)
	}
}

func TestParseHomepage(t *testing.T) {
	m, err := Parse("// ==UserScript==\n// @name a\n// @homepageURL https://example.dev/a\n// @website https://other.dev\n// @source javascript:alert(1)\n// ==/UserScript==\n")
	if err != nil || m.Homepage != "https://example.dev/a" {
		t.Fatalf("homepage = %q %v", m.Homepage, err)
	}
	m, _ = Parse("// ==UserScript==\n// @name a\n// @homepage javascript:alert(1)\n// ==/UserScript==\n")
	if m.Homepage != "" {
		t.Fatalf("non-http homepage kept: %q", m.Homepage)
	}
}

func TestEditedSaveReusesFiles(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	calls := 0
	net := func(_ context.Context, u string) ([]byte, string, error) {
		calls++
		return []byte("/*" + u + "*/"), "text/css", nil
	}
	e, _, err := s.Install(context.Background(), []byte(sample), InstallOptions{SourceURL: "https://example.com/s/a.user.js", Fetch: net})
	if err != nil {
		t.Fatal(err)
	}
	before := calls
	edited := strings.Replace(sample, "console.log('hi');", "console.log('edited');", 1)
	e2, _, err := s.Install(context.Background(), []byte(edited), InstallOptions{SourceURL: e.SourceURL, Edited: true, Fetch: s.LocalFetcher(e.ID, net)})
	if err != nil {
		t.Fatal(err)
	}
	if calls != before {
		t.Fatalf("saving an edit downloaded %d files again", calls-before)
	}
	if !e2.Edited || e2.ID != e.ID {
		t.Fatalf("edited entry = %+v", e2)
	}
	got, _ := s.Load("any")
	if len(got) != 1 || !strings.Contains(got[0].Code, "edited") || got[0].Requires[0] != "/*https://example.com/s/lib.js*/" {
		t.Fatalf("load after edit = %+v", got)
	}
	// Reinstalling from the source clears Edited.
	e3, _, _ := s.Install(context.Background(), []byte(sample), InstallOptions{SourceURL: e.SourceURL, Fetch: net})
	if e3.Edited {
		t.Fatal("reinstall kept Edited")
	}
}

func TestCheck(t *testing.T) {
	if ps := Check(sample); len(ps) != 0 {
		t.Fatalf("sample has problems: %+v", ps)
	}
	for src, want := range map[string]string{
		"console.log(1)":                                                       "No // ==UserScript==",
		"// ==UserScript==\n// @name a\n":                                      "not closed",
		"// ==UserScript==\n// @version 1\n// ==/UserScript==\n":               "no @name",
		"// ==UserScript==\n// @name a\n// @match x.com/*\n// ==/UserScript==": "not a match pattern",
	} {
		ps := Check(src)
		if len(ps) == 0 || !strings.Contains(ps[0].Message, want) || !HasErrors(ps) {
			t.Errorf("Check(%q) = %+v, want %q", src, ps, want)
		}
	}
	ps := Check("// ==UserScript==\n// @name a\n// @match x.com/*\n// ==/UserScript==")
	if ps[0].Line != 2 {
		t.Errorf("bad @match on line %d, want 2", ps[0].Line)
	}
	w := Check("// ==UserScript==\n// @name a\n// @grant GM_xmlhttpRequest\n// @run-at soon\n// ==/UserScript==")
	if len(w) != 2 || HasErrors(w) {
		t.Errorf("warnings = %+v", w)
	}
}
