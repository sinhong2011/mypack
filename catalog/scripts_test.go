package catalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundledScripts(t *testing.T) {
	s := BundledScripts()
	if len(s.Scripts) == 0 {
		t.Fatal("the hub ships empty")
	}
	raw, _ := ParseScripts(bundledScripts)
	if len(raw.Scripts) != len(s.Scripts) {
		t.Fatal("a bundled hub entry was dropped by CheckScript")
	}
}

// A hub entry that points into this repository must match the file here,
// so a script edit without a new pin fails CI instead of every install.
func TestBundledScriptHashes(t *testing.T) {
	const repo = "https://raw.githubusercontent.com/sinhong2011/mypack/"
	for _, sc := range BundledScripts().Scripts {
		if !strings.HasPrefix(sc.URL, repo) {
			continue
		}
		rest := strings.TrimPrefix(sc.URL, repo)
		i := strings.Index(rest, "/")
		if i < 0 {
			t.Fatalf("%s: no path", sc.URL)
		}
		if ref := rest[:i]; len(ref) != 40 {
			t.Errorf("%s: pin a commit, not %q", sc.Name, ref)
		}
		b, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(rest[i+1:])))
		if err != nil {
			t.Fatalf("%s: %v", sc.Name, err)
		}
		// GitHub serves the file with LF endings; a Windows checkout may
		// have CRLF.
		b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != sc.SHA256 {
			t.Errorf("%s: the file in the repo has sha256 %s, the hub says %s (update the pin and hash)", sc.Name, got, sc.SHA256)
		}
	}
}

func TestCheckScript(t *testing.T) {
	good := Script{Name: "A", Sites: []string{"x.com"}, URL: "https://e.com/a.user.js", SHA256: strings.Repeat("a", 64)}
	if err := CheckScript(good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Script{
		{Name: "A", Sites: []string{"x.com"}, URL: "http://e.com/a.js", SHA256: good.SHA256},
		{Name: "A", Sites: []string{"x.com"}, URL: good.URL, SHA256: "abc"},
		{Name: "A", Sites: []string{"https://x.com/"}, URL: good.URL, SHA256: good.SHA256},
		{Name: "A", URL: good.URL, SHA256: good.SHA256},
	} {
		if CheckScript(bad) == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
