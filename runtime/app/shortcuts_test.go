package app

import (
	"testing"

	"mypack.dev/mypack/pkg/config"
)

func TestAccUsesThisAppsShortcuts(t *testing.T) {
	a := &App{cfg: samplePrefsConfig(), disabled: map[string]bool{}}
	if got := a.acc("CmdOrCtrl+R"); got != "CmdOrCtrl+R" {
		t.Fatalf("default %q", got)
	}
	a.cfg.Native.Shortcuts = map[string]string{"reload": "Alt+R"}
	if got := a.acc("CmdOrCtrl+R"); got != "Alt+R" {
		t.Fatalf("remap %q", got)
	}
	a.cfg.Native.Shortcuts["reload"] = ""
	if got := a.acc("CmdOrCtrl+R"); got != "" {
		t.Fatalf("off %q", got)
	}
}

func TestTidyShortcutsDropsDefaults(t *testing.T) {
	got := tidyShortcuts(map[string]string{"reload": "CmdOrCtrl+R", "settings": "Alt+,", "nope": "X"})
	if len(got) != 1 || got["settings"] != "Alt+," {
		t.Fatalf("%+v", got)
	}
}

func TestShortcutsBootHidesGroupOnSingleSite(t *testing.T) {
	a := &App{cfg: samplePrefsConfig()}
	a.cfg.Window.Tabs = true
	var next, tabs bool
	for _, s := range a.shortcutsBoot() {
		if s.ID == "nextSite" {
			next = true
		}
		if s.ID == "newTab" {
			tabs = true
		}
	}
	if next {
		t.Fatal("single site should not list Next Site")
	}
	if !tabs {
		t.Fatal("tabs app should list New Tab")
	}
}

func TestShortcutsBootListsGroupWhenGrouped(t *testing.T) {
	cfg := samplePrefsConfig()
	cfg.Window.Tabs = true
	cfg.App.Group = &config.GroupConfig{Members: []config.GroupMember{
		{ID: "a", Name: "A", URL: "https://a.example"},
		{ID: "b", Name: "B", URL: "https://b.example"},
	}}
	a := &App{cfg: cfg}
	var next, split, tabs bool
	for _, s := range a.shortcutsBoot() {
		switch s.ID {
		case "nextSite":
			next = true
		case "split":
			split = true
		case "newTab":
			tabs = true
		}
	}
	if !next || !split {
		t.Fatal("group should list Next Site and Toggle Split View")
	}
	if tabs {
		t.Fatal("group should not list New Tab")
	}
}

func TestTidyShortcutsKeepsGroupIDs(t *testing.T) {
	got := tidyShortcuts(map[string]string{"nextSite": "Alt+Tab", "split": "CmdOrCtrl+\\", "nope": "X"})
	if len(got) != 1 || got["nextSite"] != "Alt+Tab" {
		t.Fatalf("%+v", got)
	}
}

func TestApplySettingsFormClearsGroupTabs(t *testing.T) {
	cfg := samplePrefsConfig()
	cfg.App.Group = &config.GroupConfig{Members: []config.GroupMember{
		{ID: "a", Name: "A", URL: "https://a.example"},
		{ID: "b", Name: "B", URL: "https://b.example"},
	}}
	err := applySettingsForm(cfg, settingsForm{
		Window:  config.WindowConfig{Width: 800, Height: 600, Zoom: 100, Tabs: true},
		WebView: settingsWebViewForm{UserAgentPreset: config.UAPresetSafari, ExternalByDefault: true},
		Native:  settingsNativeForm{Notifications: "allow", Badge: "auto", LinkRouting: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Window.Tabs {
		t.Fatal("groups cannot use tabs")
	}
}

func TestApplyDownloadDirDefault(t *testing.T) {
	cfg := &config.Config{}
	config.ApplyDownloadDirDefault(cfg, "~/Inbox")
	if cfg.Network.DownloadDirectory != "~/Inbox" {
		t.Fatal(cfg.Network.DownloadDirectory)
	}
	config.ApplyDownloadDirDefault(cfg, "~/Other")
	if cfg.Network.DownloadDirectory != "~/Inbox" {
		t.Fatal("must not overwrite a set folder")
	}
}
