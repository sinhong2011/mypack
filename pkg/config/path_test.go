package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExpandUserPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	if got := ExpandUserPath("  "); got != "" {
		t.Fatalf("empty: %q", got)
	}
	if got := ExpandUserPath("~"); got != home {
		t.Fatalf("~ = %q want %q", got, home)
	}
	want := filepath.Join(home, "Downloads")
	if got := ExpandUserPath("~/Downloads"); got != want {
		t.Fatalf("~/Downloads = %q want %q", got, want)
	}
	if got := expandUserPath("~/Inbox", filepath.Join(home, "fake")); got != filepath.Join(home, "fake", "Inbox") {
		t.Fatalf("expand with supplied home: %q", got)
	}
	if runtime.GOOS == "windows" {
		if got := ExpandUserPath(`~\Downloads`); got != want {
			t.Fatalf(`~\Downloads = %q want %q`, got, want)
		}
	}
	// The bug this covers: Join(home, "/Downloads") is "/Downloads".
	if got := ExpandUserPath("~/Downloads"); got == "/Downloads" || got == `\Downloads` {
		t.Fatalf("home was dropped: %q", got)
	}
	abs := filepath.Join(home, "abs")
	if got := ExpandUserPath(abs); got != filepath.Clean(abs) {
		t.Fatalf("absolute: %q", got)
	}
}

func TestSafeDownloadName(t *testing.T) {
	cases := map[string]string{
		"":                 "download",
		".":                "download",
		"..":               "download",
		"/":                "download",
		`\`:                "download",
		"report.csv":       "report.csv",
		"../../etc/passwd": "passwd",
		`C:\tmp\a.bin`:     "a.bin",
		"/tmp/a.bin":       "a.bin",
		`\\tmp\a.bin`:      "a.bin",
		"foo/..":           "download",
	}
	for in, want := range cases {
		if got := SafeDownloadName(in); got != want {
			t.Errorf("SafeDownloadName(%q) = %q want %q", in, got, want)
		}
	}
}

func TestResolveDownloadDir(t *testing.T) {
	home := t.TempDir()
	osDl := filepath.Join(t.TempDir(), "os-downloads")
	tmp := filepath.Join(t.TempDir(), "tmp")
	custom := filepath.Join(t.TempDir(), "custom")

	mkdirOK := func(string) error { return nil }
	if got := ResolveDownloadDir(custom, home, osDl, tmp, mkdirOK); got != custom {
		t.Fatalf("custom: %q", got)
	}
	if got := ResolveDownloadDir("~/Inbox", home, osDl, tmp, mkdirOK); got != filepath.Join(home, "Inbox") {
		t.Fatalf("~: %q", got)
	}
	if got := ResolveDownloadDir("Inbox", home, osDl, tmp, mkdirOK); got != filepath.Join(home, "Inbox") {
		t.Fatalf("relative: %q", got)
	}
	if got := ResolveDownloadDir("", home, osDl, tmp, mkdirOK); got != osDl {
		t.Fatalf("system: %q", got)
	}

	deny := map[string]bool{custom: true, osDl: true}
	mkdir := func(d string) error {
		if deny[d] {
			return errors.New("denied")
		}
		return nil
	}
	if got := ResolveDownloadDir(custom, home, osDl, tmp, mkdir); got != filepath.Join(home, "Downloads") {
		t.Fatalf("fallback after custom+os fail: %q", got)
	}
	if got := ResolveDownloadDir("", "", "", tmp, mkdirOK); got != tmp {
		t.Fatalf("tmp: %q", got)
	}
	if got := ResolveDownloadDir("", "", "", "", func(string) error { return errors.New("no") }); got != "" {
		t.Fatalf("none: %q", got)
	}
}
