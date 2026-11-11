package app

import (
	"testing"

	"mypack.dev/mypack/pkg/config"
)

// Create → Build keeps the Create view: the same project builds in its
// stage, and the URL, name and settings stay as they were.
func TestCreateBuildKeepsTheView(t *testing.T) {
	var s createState
	s.reset(config.Target{OS: "darwin", Arch: "arm64"})
	s.setInput("tradermemos.example.com")
	s.name, s.nameEdited = "TraderMemos", true
	s.autoInstall, s.autoSet = true, true
	s.buildStarted("prj_1")
	if s.building != "prj_1" {
		t.Fatalf("building = %q, want prj_1", s.building)
	}
	if s.input != "tradermemos.example.com" || !s.ready() {
		t.Errorf("input reset: %q (ready %v)", s.input, s.ready())
	}
	if s.name != "TraderMemos" || !s.nameEdited || !s.autoInstall {
		t.Errorf("name or settings reset: %+v", s)
	}
}
