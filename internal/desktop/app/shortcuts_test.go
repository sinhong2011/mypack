package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAccelFromKey(t *testing.T) {
	cases := []struct {
		key  ui.Key
		mods ui.Modifiers
		mac  bool
		want string
		ok   bool
	}{
		{ui.KeyK, ui.Super, true, "CmdOrCtrl+K", true},
		{ui.KeyK, ui.Ctrl, false, "CmdOrCtrl+K", true},
		{ui.KeyK, ui.Ctrl, true, "Ctrl+K", true},
		{ui.KeySpace, ui.Alt | ui.Shift, true, "Alt+Shift+Space", true},
		{ui.KeyN, ui.Super | ui.Shift, true, "CmdOrCtrl+Shift+N", true},
		{ui.KeySlash, ui.Super, true, "CmdOrCtrl+/", true},
		{ui.KeyF5, 0, true, "F5", true},
		{ui.KeyF12, ui.Shift, false, "Shift+F12", true},
		{ui.KeyA, 0, true, "", true},              // typing
		{ui.KeyA, ui.Shift, true, "", true},       // typing too
		{ui.KeyUnknown, ui.Super, true, "", true}, // a modifier alone
	}
	for _, c := range cases {
		got, ok := accelFromKey(c.key, c.mods, c.mac)
		if got != c.want || ok != c.ok {
			t.Errorf("accelFromKey(%v, %v, mac=%v) = %q, %v; want %q, %v", c.key, c.mods, c.mac, got, ok, c.want, c.ok)
		}
	}
}

func TestNormAccel(t *testing.T) {
	same := [][2]string{
		{"Shift+CmdOrCtrl+N", "CmdOrCtrl+Shift+N"},
		{"Cmd+K", "CmdOrCtrl+k"},
		{"Option+Space", "Alt+Space"},
	}
	for _, s := range same {
		if a, b := normAccel(s[0], true), normAccel(s[1], true); a != b {
			t.Errorf("mac: %q → %q, %q → %q", s[0], a, s[1], b)
		}
	}
	if normAccel("Ctrl+K", false) != normAccel("CmdOrCtrl+K", false) {
		t.Error("Ctrl is the command key off macOS")
	}
	if normAccel("Ctrl+K", true) == normAccel("CmdOrCtrl+K", true) {
		t.Error("Ctrl is not Command on macOS")
	}
}

func TestShortcutDefaultsDistinct(t *testing.T) {
	for _, mac := range []bool{true, false} {
		seen := map[string]string{}
		for _, d := range shortcutDefs() {
			n := normAccel(d.def, mac)
			if other, ok := seen[n]; ok {
				t.Errorf("%s and %s share %s", d.id, other, d.def)
			}
			seen[n] = d.id
			for _, r := range reservedAccels {
				if normAccel(r, mac) == n {
					t.Errorf("%s's default %s is reserved", d.id, d.def)
				}
			}
		}
	}
	if launcherDefault() == "Alt+Space" {
		t.Error("the launcher no longer defaults to Option-Space")
	}
}

func TestShortcutOverrides(t *testing.T) {
	a := &App{}
	a.ui.Shortcuts = map[string]string{scPalette: "CmdOrCtrl+P", scFind: ""}
	if got := a.accel(scPalette); got != "CmdOrCtrl+P" {
		t.Errorf("palette = %q", got)
	}
	if got := a.accel(scFind); got != "" {
		t.Errorf("a cleared shortcut = %q", got)
	}
	if got := a.accel(scNewApp); got != "CmdOrCtrl+N" {
		t.Errorf("default = %q", got)
	}
	if a.shortcutClash(scBuild, "CmdOrCtrl+P") == "" {
		t.Error("CmdOrCtrl+P is the palette's now")
	}
	if a.shortcutClash(scBuild, "CmdOrCtrl+K") != "" {
		t.Error("CmdOrCtrl+K is free once the palette moved")
	}
	if a.shortcutClash(scBuild, "CmdOrCtrl+Q") == "" {
		t.Error("Quit's keys are reserved")
	}
}

func TestLauncherHeight(t *testing.T) {
	if h := launcherHeight(-1); h != launcherFieldH {
		t.Errorf("field alone = %d", h)
	}
	one, three := launcherHeight(1), launcherHeight(3)
	if three-one != 2*(launcherRowH+launcherRowGap) {
		t.Errorf("each row adds %d, got %d", launcherRowH+launcherRowGap, (three-one)/2)
	}
	if launcherHeight(0) != one {
		t.Error("the empty line is one row high")
	}
	if launcherHeight(launcherRows) < launcherFieldH+launcherRows*launcherRowH+launcherFootH {
		t.Error("a full list fits")
	}
}

func TestFilterShortcuts(t *testing.T) {
	secs := []scSection{
		{"View", []scLine{{"Library", "CmdOrCtrl+1"}, {"Find", "CmdOrCtrl+F"}}},
		{"Window", []scLine{{"Close window", "CmdOrCtrl+W"}}},
	}
	if got := filterShortcuts(secs, ""); len(got) != 2 {
		t.Errorf("no filter keeps all, got %d", len(got))
	}
	if got := filterShortcuts(secs, "find"); len(got) != 1 || len(got[0].lines) != 1 || got[0].lines[0].label != "Find" {
		t.Errorf("by label: %+v", got)
	}
	if got := filterShortcuts(secs, "window"); len(got) != 1 || got[0].title != "Window" {
		t.Errorf("by section: %+v", got)
	}
	if got := filterShortcuts(secs, "zzz"); len(got) != 0 {
		t.Errorf("no match: %+v", got)
	}
}

func TestSplitShortcuts(t *testing.T) {
	sec := func(n int) scSection { return scSection{lines: make([]scLine, n)} }
	// 3+5+6+7 | 6+4: the heights a = 21, b = 10 → better 14 | 17.
	secs := []scSection{sec(1), sec(3), sec(4), sec(5), sec(4), sec(2)}
	if got := splitShortcuts(secs); got != 3 {
		t.Errorf("split = %d, want 3", got)
	}
	if got := splitShortcuts(secs[:1]); got != 1 {
		t.Errorf("one section stays in the first column, got %d", got)
	}
}
