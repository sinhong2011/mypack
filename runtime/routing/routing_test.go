package routing

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func yes(string) bool { return true }

func TestMatch(t *testing.T) {
	es := []Entry{
		{Identifier: "com.x.gmail", BundlePath: "/A/Gmail.app", SourceHost: "mail.google.com", Hosts: []string{"mail.google.com", "google.com"}},
		{Identifier: "com.x.cal", BundlePath: "/A/Cal.app", SourceHost: "calendar.google.com", Hosts: []string{"calendar.google.com", "google.com"}},
		{Identifier: "com.x.slack", BundlePath: "/A/Slack.app", Hosts: []string{"app.slack.com", "slack.com"}},
		{Identifier: "com.x.self", BundlePath: "/A/Self.app", Hosts: []string{"self.dev"}},
	}
	cases := []struct {
		host, self, want string
	}{
		{"mail.google.com", "com.x.self", "com.x.gmail"},
		{"calendar.google.com", "com.x.self", "com.x.cal"},
		{"calendar.google.com", "com.x.cal", "com.x.gmail"}, // never itself; next best claim
		{"docs.google.com", "com.x.self", ""},               // two equal registrable claims: ambiguous
		{"files.slack.com", "com.x.self", "com.x.slack"},
		{"slack.com", "com.x.self", "com.x.slack"},
		{"notslack.com", "com.x.self", ""},
		{"slack.com.evil.io", "com.x.self", ""},
		{"self.dev", "com.x.self", ""},
		{"SLACK.com.", "com.x.self", "com.x.slack"},
		{"", "com.x.self", ""},
	}
	for _, c := range cases {
		got, ok := Match(es, c.host, c.self, yes)
		if c.want == "" {
			if ok {
				t.Errorf("Match(%q) = %s, want none", c.host, got.Identifier)
			}
			continue
		}
		if !ok || got.Identifier != c.want {
			t.Errorf("Match(%q, self=%s) = %v %v, want %s", c.host, c.self, got.Identifier, ok, c.want)
		}
	}
	// Missing bundle on disk is skipped.
	if _, ok := Match(es, "app.slack.com", "x", func(p string) bool { return p != "/A/Slack.app" }); ok {
		t.Error("uninstalled bundle must not be routed to")
	}
}

func TestExpandHosts(t *testing.T) {
	got := ExpandHosts("mail.proton.me", "*.Example.com", "alice.github.io", "bad host", "")
	want := []string{"mail.proton.me", "proton.me", "example.com", "alice.github.io"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCommand(t *testing.T) {
	e := Entry{Identifier: "com.x.slack", BundlePath: "/Users/u/Applications/Slack.app", Hosts: []string{"slack.com"}}
	name, args, err := Command("darwin", e, "https://app.slack.com/client/T1")
	if err != nil || name != "open" || !reflect.DeepEqual(args, []string{"-a", "/Users/u/Applications/Slack.app", "https://app.slack.com/client/T1"}) {
		t.Fatalf("darwin: %v %v %v", name, args, err)
	}
	name, args, _ = Command("darwin", Entry{Identifier: "com.x.slack"}, "https://slack.com/")
	if name != "open" || !reflect.DeepEqual(args, []string{"-b", "com.x.slack", "https://slack.com/"}) {
		t.Fatalf("darwin -b: %v %v", name, args)
	}
	name, args, err = Command("windows", Entry{Identifier: "i", Executable: `C:\Apps\Slack\Slack.exe`}, "https://slack.com/")
	if err != nil || name != `C:\Apps\Slack\Slack.exe` || !reflect.DeepEqual(args, []string{"https://slack.com/"}) {
		t.Fatalf("windows: %v %v %v", name, args, err)
	}
	for _, bad := range []string{"javascript:alert(1)", "file:///etc/passwd", "https://", "https://a.com/ x", "-a /bin/sh", "mailto:a@b.c"} {
		if _, _, err := Command("darwin", e, bad); err == nil {
			t.Errorf("Command accepted %q", bad)
		}
	}
	if _, _, err := Command("linux", Entry{Identifier: "i", BundlePath: "/nonexistent"}, "https://a.com/"); err == nil {
		t.Error("no executable should fail")
	}
}

func TestGuessExecutable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Slack")
	_ = os.MkdirAll(filepath.Join(dir, "resources"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "slack"), []byte("x"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "slack.desktop"), []byte("x"), 0o644)
	if runtime.GOOS != "windows" { // Windows does not store the Unix executable bit
		if got := GuessExecutable("linux", dir); got != filepath.Join(dir, "slack") {
			t.Fatalf("linux: %q", got)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "Slack.exe"), []byte("x"), 0o755)
	if got := GuessExecutable("windows", dir); got != filepath.Join(dir, "Slack.exe") {
		t.Fatalf("windows: %q", got)
	}
}

func TestWriteReadTable(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d", "routes.json")
	es := []Entry{
		{Identifier: "b", BundlePath: "/b", Hosts: []string{"B.com", "b.com"}},
		{Identifier: "a", BundlePath: "/a", SourceHost: "A.com", Hosts: []string{"a.com"}},
		{Identifier: "", BundlePath: "/z", Hosts: []string{"z.com"}}, // dropped
	}
	if err := Write(p, es); err != nil {
		t.Fatal(err)
	}
	got, err := Read(p)
	if err != nil || len(got) != 2 || got[0].Identifier != "a" || got[1].Hosts[0] != "b.com" || len(got[1].Hosts) != 1 {
		t.Fatalf("read back: %+v %v", got, err)
	}
	fi1, _ := os.Stat(p)
	time.Sleep(20 * time.Millisecond)
	if err := Write(p, es); err != nil {
		t.Fatal(err)
	}
	if fi2, _ := os.Stat(p); !fi2.ModTime().Equal(fi1.ModTime()) {
		t.Error("an unchanged routes file must not be rewritten")
	}

	tb := &Table{Path: p, Exists: yes}
	if e, ok := tb.Lookup("www.a.com", "x"); !ok || e.Identifier != "a" {
		t.Fatalf("lookup: %v %v", e, ok)
	}
	// Changing the file is noticed (mtime/size check).
	if err := Write(p, []Entry{{Identifier: "c", BundlePath: "/c", Hosts: []string{"c.org"}}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := tb.Lookup("www.a.com", "x"); ok {
		t.Error("stale cache")
	}
	if e, ok := tb.Lookup("c.org", "x"); !ok || e.Identifier != "c" {
		t.Fatalf("lookup c: %v %v", e, ok)
	}
	os.Remove(p)
	if _, ok := tb.Lookup("c.org", "x"); ok {
		t.Error("deleted file still routes")
	}
	// A corrupt file must never break link opening.
	_ = os.WriteFile(p, []byte("{not json"), 0o644)
	if _, ok := tb.Lookup("c.org", "x"); ok {
		t.Error("corrupt file routes")
	}
}

func TestFilePathEnv(t *testing.T) {
	t.Setenv(EnvFile, "/tmp/x/routes.json")
	if FilePath() != "/tmp/x/routes.json" {
		t.Fatal(FilePath())
	}
	t.Setenv(EnvFile, "")
	if filepath.Base(FilePath()) != "routes.json" || filepath.Base(filepath.Dir(FilePath())) != "mypack-apps" {
		t.Fatal(FilePath())
	}
}
