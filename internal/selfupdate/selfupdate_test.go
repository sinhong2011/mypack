package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"v1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.1.0", "1.0.9", 1},
		{"2.0.0", "1.99.99", 1},
		{"1.10.0", "1.9.0", 1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-beta.2", "1.0.0-beta.10", -1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1},
		{"1.0.0-beta", "1.0.0-alpha", 1},
		{"1.0.0-rc.1", "0.9.9", 1},
		{"1.0.0+build.5", "1.0.0", 0},
		{"garbage", "1.0.0", -1},
		{"1.0.0", "garbage", 1},
		{"1.0", "1.0.0", -1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := Compare(c.b, c.a); got != -c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.b, c.a, got, -c.want)
		}
	}
}

func TestParseVersionRejects(t *testing.T) {
	for _, s := range []string{"", "v", "1", "1.2", "1.2.3.4", "01.2.3", "1.2.3-", "1.2.3-a..b", "a.b.c", "-1.2.3"} {
		if _, ok := ParseVersion(s); ok {
			t.Errorf("ParseVersion(%q) ok", s)
		}
	}
	v, ok := ParseVersion("v0.9.0-beta.1")
	if !ok || v.String() != "0.9.0-beta.1" || !v.IsPrerelease() {
		t.Fatalf("got %v %v", v, ok)
	}
}

func TestIsNewer(t *testing.T) {
	if !IsNewer("0.3.0", "0.2.9") {
		t.Error("0.3.0 > 0.2.9")
	}
	if IsNewer("0.2.9", "0.2.9") || IsNewer("0.2.8", "0.2.9") {
		t.Error("not newer")
	}
	// A development build (0.1.0-dev) is older than its release.
	if !IsNewer("0.1.0", "0.1.0-dev") {
		t.Error("0.1.0 > 0.1.0-dev")
	}
	if IsNewer("9.9.9", "unknown") {
		t.Error("an unparsable current version is never offered updates")
	}
}

func TestDue(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		last time.Time
		want bool
	}{
		{"never checked", time.Time{}, true},
		{"just now", now.Add(-time.Minute), false},
		{"23h ago", now.Add(-23 * time.Hour), false},
		{"exactly a day", now.Add(-24 * time.Hour), true},
		{"two days", now.Add(-48 * time.Hour), true},
		{"clock set back", now.Add(3 * time.Hour), true},
	}
	for _, c := range cases {
		if got := Due(c.last, now, CheckInterval); got != c.want {
			t.Errorf("%s: Due = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPickLatest(t *testing.T) {
	list := []ghRelease{
		{TagName: "v0.4.0-beta.1", Prerelease: true, HTMLURL: "u4b"},
		{TagName: "v0.3.1", HTMLURL: "u31", Body: "notes"},
		{TagName: "v0.5.0", Draft: true},
		{TagName: "desktop-v9.0.0"},
		{TagName: "v0.3.0"},
		{TagName: "v0.10.0-rc.1"}, // a prerelease by its version, not marked
	}
	r := pickLatest(list, false)
	if r == nil || r.Version != "0.3.1" || r.Tag != "v0.3.1" || r.URL != "u31" || r.Notes != "notes" || r.Prerelease {
		t.Fatalf("stable: %+v", r)
	}
	r = pickLatest(list, true)
	if r == nil || r.Version != "0.10.0-rc.1" || !r.Prerelease {
		t.Fatalf("prereleases: %+v", r)
	}
	if pickLatest([]ghRelease{{TagName: "v1.0.0", Draft: true}}, true) != nil {
		t.Fatal("drafts are never offered")
	}
}

func TestCheckerLatest(t *testing.T) {
	var gotPath, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotUA = r.URL.Path, r.Header.Get("User-Agent")
		w.Write([]byte(`[{"tag_name":"v1.2.0","body":"- Faster","html_url":"https://x/v1.2.0","published_at":"2026-11-01T10:00:00Z"},
			{"tag_name":"v1.3.0-beta.1","prerelease":true}]`))
	}))
	defer srv.Close()
	c := Checker{BaseURL: srv.URL, UserAgent: "Mypack/1.0.0"}
	r, err := c.Latest(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/repos/sinhong2011/mypack/releases" || gotUA != "Mypack/1.0.0" {
		t.Fatalf("request %s UA %q", gotPath, gotUA)
	}
	if r.Version != "1.2.0" || r.Published.IsZero() {
		t.Fatalf("%+v", r)
	}
}

func TestCheckerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "limited") {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := Checker{BaseURL: srv.URL, Repo: "x/limited"}.Latest(context.Background(), false)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("rate limit: %v", err)
	}
	if _, err := (Checker{BaseURL: srv.URL}).Latest(context.Background(), false); err == nil {
		t.Fatal("502 is an error")
	}
}

func TestSummary(t *testing.T) {
	notes := "## What's Changed\n\n<!-- generated -->\n- **Faster** builds by @a in [#12](https://x)\n* Fix `tray` icon\n\nPlain line\n- \n- Fourth\n**Full Changelog**: https://x"
	got := Summary(notes, 3)
	want := []string{"• Faster builds by @a in #12", "• Fix tray icon", "Plain line"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", got)
	}
	quoted := "The first build.\n> Pre-release for developers.\n>\n> ```sh\n> git clone x\n> ```\n> Then run it."
	got = Summary(quoted, 5)
	want = []string{"The first build.", "Pre-release for developers.", "Then run it."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("quoted: got %q", got)
	}
	if len(Summary("", 3)) != 0 {
		t.Fatal("empty notes")
	}
}

// fakeFS is a set of paths that exist.
type fakeFS map[string]bool

func (f fakeFS) exists(p string) bool { return f[p] }
func (f fakeFS) glob(pattern string) []string {
	var out []string
	prefix := strings.TrimSuffix(pattern, "*")
	for p := range f {
		if strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	return out
}

func TestDetect(t *testing.T) {
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	cases := []struct {
		name  string
		goos  string
		exe   string
		env   map[string]string
		fs    fakeFS
		want  Manager
		cmd   string
		check func(Source) bool
	}{
		{name: "mac dmg", goos: "darwin", exe: "/Applications/Mypack.app/Contents/MacOS/mypack", want: ManagerNone},
		{name: "mac brew cask", goos: "darwin", exe: "/Applications/Mypack.app/Contents/MacOS/mypack",
			fs: fakeFS{"/opt/homebrew/Caskroom/mypack": true}, want: ManagerHomebrew, cmd: "brew upgrade --cask mypack"},
		{name: "mac brew intel", goos: "darwin", exe: "/Applications/Mypack.app/Contents/MacOS/mypack",
			fs: fakeFS{"/usr/local/Caskroom/mypack": true}, want: ManagerHomebrew},
		{name: "mac brew custom prefix", goos: "darwin", exe: "/Users/a/Applications/Mypack.app/Contents/MacOS/mypack",
			env: map[string]string{"HOMEBREW_PREFIX": "/Users/a/brew"}, fs: fakeFS{"/Users/a/brew/Caskroom/mypack": true}, want: ManagerHomebrew},
		{name: "mac other cask only", goos: "darwin", exe: "/Applications/Mypack.app/Contents/MacOS/mypack",
			fs: fakeFS{"/opt/homebrew/Caskroom/mypack-nightly": true}, want: ManagerNone},
		{name: "win nsis per-user", goos: "windows", exe: `C:\Users\a\AppData\Local\Programs\Mypack\mypack.exe`,
			env: map[string]string{"LOCALAPPDATA": `C:\Users\a\AppData\Local`}, want: ManagerNone},
		{name: "win scoop", goos: "windows", exe: `C:\Users\a\scoop\apps\mypack\current\mypack.exe`,
			want: ManagerScoop, cmd: "scoop update mypack"},
		{name: "win scoop custom root", goos: "windows", exe: `D:\tools\apps\mypack\1.0.0\mypack.exe`,
			env: map[string]string{"SCOOP": `D:\Tools`}, want: ManagerScoop},
		{name: "win choco lib", goos: "windows", exe: `C:\Users\a\AppData\Local\Programs\Mypack\mypack.exe`,
			fs: fakeFS{`C:\ProgramData\chocolatey\lib\mypack`: true}, want: ManagerChocolatey, cmd: "choco upgrade mypack"},
		{name: "win winget packages", goos: "windows", exe: `C:\Users\a\AppData\Local\Programs\Mypack\mypack.exe`,
			env:  map[string]string{"LOCALAPPDATA": `C:\Users\a\AppData\Local`},
			fs:   fakeFS{`C:\Users\a\AppData\Local\Microsoft\WinGet\Packages\Mypack.Mypack_Microsoft.Winget.Source_8wekyb3d8bbwe`: true},
			want: ManagerWinget, cmd: "winget upgrade Mypack.Mypack"},
		{name: "win winget portable", goos: "windows", exe: `C:\Users\a\AppData\Local\Microsoft\WinGet\Packages\Mypack.Mypack_x\mypack.exe`,
			want: ManagerWinget},
		{name: "linux install.sh", goos: "linux", exe: "/home/a/.local/mypack.app/mypack", fs: fakeFS{"/usr/bin/apt": true}, want: ManagerNone},
		{name: "linux deb", goos: "linux", exe: "/opt/mypack/mypack",
			fs: fakeFS{"/var/lib/dpkg/info/mypack.list": true}, want: ManagerApt, cmd: "sudo apt update && sudo apt install --only-upgrade mypack"},
		{name: "linux rpm dnf", goos: "linux", exe: "/opt/mypack/mypack", fs: fakeFS{"/usr/bin/dnf": true}, want: ManagerDnf, cmd: "sudo dnf upgrade mypack"},
		{name: "linux rpm zypper", goos: "linux", exe: "/opt/mypack/mypack", fs: fakeFS{"/usr/bin/zypper": true}, want: ManagerZypper},
		{name: "linux aur", goos: "linux", exe: "/opt/mypack/mypack",
			fs: fakeFS{"/var/lib/pacman/local/mypack-bin-1.2.0-1": true}, want: ManagerAUR, cmd: "yay -Syu mypack-bin"},
		{name: "linux unknown package", goos: "linux", exe: "/opt/mypack/mypack", want: ManagerLinuxPackage, cmd: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env = c.env
			fs := c.fs
			if fs == nil {
				fs = fakeFS{}
			}
			s := Detect(Env{GOOS: c.goos, Exe: c.exe, Getenv: getenv, Exists: fs.exists, Glob: fs.glob, Home: `C:\Users\a`})
			if s.Manager != c.want {
				t.Fatalf("manager %q, want %q", s.Manager, c.want)
			}
			if c.cmd != "" && s.Command != c.cmd {
				t.Fatalf("command %q, want %q", s.Command, c.cmd)
			}
			if s.Managed() != (c.want != ManagerNone) {
				t.Fatal("Managed")
			}
		})
	}
}

func TestPacmanName(t *testing.T) {
	for in, want := range map[string]string{
		"mypack-1.0.0-1":     "mypack",
		"mypack-bin-1.0.0-2": "mypack-bin",
		"mypack-git-r10-1":   "mypack-git",
		"mypackage-1.0-1":    "",
		"mypack":             "",
		"mypack-cli-0.1.0-1": "",
	} {
		if got := pacmanName(in); got != want {
			t.Errorf("pacmanName(%q) = %q, want %q", in, got, want)
		}
	}
}
