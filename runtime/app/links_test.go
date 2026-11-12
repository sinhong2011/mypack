package app

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"mypack.dev/mypack/pkg/config"
	"mypack.dev/mypack/runtime/navigation"
	"mypack.dev/mypack/runtime/routing"
)

func testApp(t *testing.T, routesFile string) *App {
	t.Helper()
	cfg := &config.Config{}
	cfg.App.Identifier = "com.test.self"
	a, _ := New(cfg, t.TempDir())
	p, err := navigation.New([]string{"self.example"}, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	a.policy = p
	a.routes = &routing.Table{Path: routesFile}
	return a
}

func writeRoutes(t *testing.T, es ...routing.Entry) string {
	t.Helper()
	dir := t.TempDir()
	for i := range es {
		if es[i].BundlePath == "" {
			es[i].BundlePath = filepath.Join(dir, es[i].Identifier+".app")
		}
		_ = os.MkdirAll(es[i].BundlePath, 0o755)
	}
	p := filepath.Join(dir, "routes.json")
	if err := routing.Write(p, es); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRouteTarget(t *testing.T) {
	p := writeRoutes(t,
		routing.Entry{Identifier: "com.test.other", Hosts: []string{"other.example"}},
		routing.Entry{Identifier: "com.test.self", Hosts: []string{"self.example"}},
	)
	a := testApp(t, p)
	if e, ok := a.routeTarget("https://www.other.example/x?y=1"); !ok || e.Identifier != "com.test.other" {
		t.Fatalf("other: %+v %v", e, ok)
	}
	if _, ok := a.routeTarget("https://self.example/x"); ok {
		t.Fatal("must never route to itself")
	}
	if _, ok := a.routeTarget("https://nobody.example/"); ok {
		t.Fatal("unknown host")
	}
	if _, ok := a.routeTarget("mailto:a@other.example"); ok {
		t.Fatal("non-web scheme")
	}
	off := false
	a.cfg.Native.LinkRouting = &off
	if _, ok := a.routeTarget("https://other.example/"); ok {
		t.Fatal("linkRouting=false must disable routing")
	}
}

func TestOpenLinkCommand(t *testing.T) {
	p := writeRoutes(t, routing.Entry{Identifier: "com.test.other", Hosts: []string{"other.example"}})
	// darwin launches with `open -a <bundle>`. Windows and Linux need a real
	// executable in the folder, or openLink falls through to the browser and
	// blocks on the UI loop.
	bundle := filepath.Join(filepath.Dir(p), "com.test.other.app")
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(filepath.Join(bundle, filepath.Base(bundle)+".exe"), []byte("MZ"), 0o755); err != nil {
			t.Fatal(err)
		}
	} else if runtime.GOOS != "darwin" {
		if err := os.WriteFile(filepath.Join(bundle, "app"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := testApp(t, p)
	got := make(chan []string, 1)
	old := routing.Start
	routing.Start = func(name string, args ...string) error {
		got <- append([]string{name}, args...)
		return nil
	}
	defer func() { routing.Start = old }()
	a.openLink("https://other.example/doc/1")
	var cmd []string
	select {
	case cmd = <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("link was not handed to the other app")
	}
	e, _ := a.routes.Lookup("other.example", "x")
	wantName, wantArgs, _ := routing.Command("darwin", e, "https://other.example/doc/1")
	if cmd[0] == "" || len(cmd) < 2 || cmd[len(cmd)-1] != "https://other.example/doc/1" {
		t.Fatalf("cmd = %v (darwin form would be %s %v)", cmd, wantName, wantArgs)
	}
}

func TestIncomingAction(t *testing.T) {
	a := testApp(t, "")
	cases := map[string]navigation.Action{
		"https://self.example/inbox":     navigation.Internal,
		"https://www.self.example/a":     navigation.Internal,
		"https://other.example/":         navigation.External,
		"https://accounts.google.com/x":  navigation.Internal, // identity provider
		"javascript:alert(1)":            navigation.Block,
		"file:///etc/passwd":             navigation.Block,
		"mailto:a@b.c":                   navigation.Block,
		"https://self.example/has space": navigation.Block,
		"":                               navigation.Block,
	}
	for in, want := range cases {
		if got := a.incomingAction(in); got != want {
			t.Errorf("incomingAction(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestHandleIncomingBeforeWindow(t *testing.T) {
	a := testApp(t, "")
	a.startURL = "https://self.example/"
	a.handleIncomingURL("https://self.example/deep/link")
	if a.startURL != "https://self.example/deep/link" {
		t.Fatalf("startURL = %q", a.startURL)
	}
}

func TestFirstWebURL(t *testing.T) {
	args := []string{"--flag", "/tmp/file.txt", "https://self.example/x", "http://later.example/"}
	if got := firstWebURL(args); got != "https://self.example/x" {
		t.Fatalf("got %q", got)
	}
	if firstWebURL([]string{"-psn_0_123", "javascript:1", "ftp://x/y"}) != "" {
		t.Fatal("only http(s)")
	}
	if got := redactArgs([]string{"-a", "/A.app", "https://x.example/private?q=1"}, "https://x.example/private?q=1"); !reflect.DeepEqual(got, "-a /A.app <url:x.example>") {
		t.Fatalf("redact = %q", got)
	}
}
