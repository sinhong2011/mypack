package app

import (
	"context"
	"os"
	"strings"
	"testing"

	"mypack.dev/mypack/pkg/config"
	"mypack.dev/mypack/runtime/userscript"
)

func installTestScript(t *testing.T, st userscript.Store, name, match string, apps []string) userscript.Entry {
	t.Helper()
	src := "// ==UserScript==\n// @name " + name + "\n// @version 1.0\n// @description d\n// @match " + match + "\n// ==/UserScript==\n1;\n"
	e, _, err := st.Install(context.Background(), []byte(src), userscript.InstallOptions{Apps: apps})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestSettingsExtensions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(userscript.EnvDir, dir)
	st := userscript.Store{Dir: dir}
	forMail := installTestScript(t, st, "Mail Helper", "https://mail.example/*", nil)
	other := installTestScript(t, st, "Elsewhere", "https://other.example/*", nil)
	scoped := installTestScript(t, st, "Scoped", "https://mail.example/*", []string{"app.other"})
	off := installTestScript(t, st, "Off", "https://*.example/*", nil)
	if _, err := st.SetEnabled(off.ID, false); err != nil {
		t.Fatal(err)
	}

	profile := t.TempDir()
	a := &App{cfg: samplePrefsConfig(), prefsDir: profile}
	a.loadUserScripts(profile)
	if !a.extRunning[forMail.ID] || !a.extRunning[other.ID] || a.extRunning[scoped.ID] || a.extRunning[off.ID] {
		t.Fatalf("running at launch = %v", a.extRunning)
	}

	list, pending := a.extensionsBoot()
	if pending || len(list) != 4 {
		t.Fatalf("boot = %+v pending=%v", list, pending)
	}
	by := map[string]settingsExt{}
	for _, x := range list {
		by[x.ID] = x
	}
	if x := by[forMail.ID]; !x.ForSite || !x.Here || !x.Running || !x.InScope {
		t.Errorf("mail helper = %+v", x)
	}
	if x := by[other.ID]; x.ForSite {
		t.Errorf("an extension for another site counts as this site's: %+v", x)
	}
	if x := by[scoped.ID]; x.InScope {
		t.Errorf("scoped to another app but in scope: %+v", x)
	}
	if x := by[off.ID]; x.Enabled || !x.ForSite {
		t.Errorf("turned off in MyPack = %+v", x)
	}

	// Off in this app: saved in the profile, pending until a restart, then
	// not loaded.
	if err := a.setExtHere(forMail.ID, false); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(profile + "/" + extPrefsFile); !strings.Contains(string(b), forMail.ID) {
		t.Fatalf("extensions.json = %s", b)
	}
	if _, pending := a.extensionsBoot(); !pending {
		t.Fatal("turning one off should ask for a restart")
	}
	a.loadUserScripts(profile)
	if a.extRunning[forMail.ID] {
		t.Fatal("an extension turned off in this app still loaded")
	}
	if _, pending := a.extensionsBoot(); pending {
		t.Fatal("still pending after a restart")
	}
	if err := a.setExtHere(forMail.ID, true); err != nil {
		t.Fatal(err)
	}
	if loadExtPrefs(profile).off(forMail.ID) {
		t.Fatal("turning it back on did not stick")
	}
}

func TestSiteHostMatch(t *testing.T) {
	for _, c := range []struct {
		sites []string
		url   string
		want  bool
	}{
		{[]string{"x.com"}, "https://x.com/home", true},
		{[]string{"*.reddit.com"}, "https://www.reddit.com/", true},
		{[]string{"*.reddit.com"}, "https://reddit.com/", true},
		{[]string{"*"}, "https://anything.test/", true},
		{[]string{"x.com"}, "https://evilx.com/", false},
		{[]string{"x.com"}, "not a url", false},
	} {
		if got := siteHostMatch(c.sites, c.url); got != c.want {
			t.Errorf("siteHostMatch(%v, %s) = %v", c.sites, c.url, got)
		}
	}
}

func TestSettingsHTMLHasExtensions(t *testing.T) {
	html := settingsHTML(settingsBoot{Name: "Mail", settingsForm: settingsForm{WebView: settingsWebViewForm{Blocking: &config.BlockingConfig{}}},
		Extensions: []settingsExt{{ID: "x-1", Name: "</script><b>X", Enabled: true, InScope: true, Here: true}}})
	for _, s := range []string{`["extensions", "Extensions"]`, "mypack:settings:ext:", "mypack:settings:restart", "setExtensions"} {
		if !strings.Contains(html, s) {
			t.Errorf("missing %q", s)
		}
	}
	if strings.Contains(html, "</script><b>") {
		t.Error("an extension name broke out of the boot JSON")
	}
}
