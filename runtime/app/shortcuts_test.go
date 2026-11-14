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
