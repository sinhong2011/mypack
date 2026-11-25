package catalog

import (
	"regexp"
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

// The bundled list is a copy of the one the extensions repository
// publishes: every entry is a file of that repository at one commit.
func TestBundledScriptsPinned(t *testing.T) {
	prefix := "https://raw.githubusercontent.com/sinhong2011/mypack-extensions/"
	if !strings.HasPrefix(ScriptsRemoteURL, prefix) || !strings.HasPrefix(HubRepo, "https://github.com/sinhong2011/mypack-extensions") {
		t.Fatal("the hub list and its repository disagree")
	}
	commit := regexp.MustCompile(`^[0-9a-f]{40}/extensions/[a-z0-9-]+/[a-z0-9-]+\.user\.js$`)
	for _, sc := range BundledScripts().Scripts {
		if !strings.HasPrefix(sc.URL, prefix) || !commit.MatchString(strings.TrimPrefix(sc.URL, prefix)) {
			t.Errorf("%s: %s is not a file of the extensions repository at a commit", sc.Name, sc.URL)
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
