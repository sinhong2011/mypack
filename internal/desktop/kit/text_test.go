package kit

import "testing"

func TestHotkeyText(t *testing.T) {
	cases := []struct {
		in       string
		mac, win string
	}{
		{"mod+shift+n", "⇧⌘N", "Ctrl+Shift+N"},
		{"mod+,", "⌘,", "Ctrl+,"},
		{"CmdOrCtrl+Shift+N", "⇧⌘N", "Ctrl+Shift+N"},
		{"Alt+Shift+Space", "⌥⇧Space", "Alt+Shift+Space"},
		{"Ctrl+Alt+Space", "⌃⌥Space", "Ctrl+Alt+Space"},
		{"Shift+Alt+CmdOrCtrl+K", "⌥⇧⌘K", "Ctrl+Alt+Shift+K"},
		{"CmdOrCtrl+/", "⌘/", "Ctrl+/"},
		{"CmdOrCtrl++", "⌘+", "Ctrl++"},
		{"F5", "F5", "F5"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := hotkeyText(c.in, true); got != c.mac {
			t.Errorf("mac %q = %q, want %q", c.in, got, c.mac)
		}
		if got := hotkeyText(c.in, false); got != c.win {
			t.Errorf("other %q = %q, want %q", c.in, got, c.win)
		}
	}
}

func TestHotkeyParts(t *testing.T) {
	got := hotkeyParts("Alt+Shift+Space", true)
	if len(got) != 3 || got[0] != "⌥" || got[1] != "⇧" || got[2] != "Space" {
		t.Errorf("mac parts = %q", got)
	}
	got = hotkeyParts("CmdOrCtrl+K", false)
	if len(got) != 2 || got[0] != "Ctrl" || got[1] != "K" {
		t.Errorf("other parts = %q", got)
	}
}
