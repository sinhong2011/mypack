package app

import (
	"testing"
)

func TestPaletteItems(t *testing.T) {
	a := launcherApp(t)
	for _, in := range a.installs {
		if in.Name == "Workspace" {
			a.ui.togglePin(installPinKey(&in))
		}
	}
	// No query: Pinned, Recent (newest first), Create and Go To; each app once.
	items := a.paletteItems()
	seen := map[string]int{}
	var sections []string
	for _, it := range items {
		if it.glyph == nil {
			seen[it.title]++
		}
		if len(sections) == 0 || sections[len(sections)-1] != it.section {
			sections = append(sections, it.section)
		}
	}
	for name, n := range seen {
		if n > 1 {
			t.Errorf("%s listed %d times", name, n)
		}
	}
	want := []string{T("nav.pinned"), T("nav.recent"), T("palette.section.create"), T("palette.section.goTo")}
	if len(sections) != len(want) {
		t.Fatalf("sections %v, want %v", sections, want)
	}
	for i := range want {
		if sections[i] != want[i] {
			t.Fatalf("sections %v, want %v", sections, want)
		}
	}
	if items[0].title != "Workspace" {
		t.Errorf("first %q, want the pinned Workspace", items[0].title)
	}
	recent := items[1:5]
	for i := 1; i < len(recent); i++ {
		if recent[i].at.After(recent[i-1].at) {
			t.Errorf("Recent not newest first: %q after %q", recent[i].title, recent[i-1].title)
		}
	}
	// A query: apps, then actions; a draft's Enter views it and ⌘Enter builds.
	items = a.paletteItems()
	a.palette.query = "li"
	items = a.paletteItems()
	if len(items) < 3 || items[0].title != "Linear" || items[0].runLabel != T("action.open") {
		t.Fatalf("li: %+v", items)
	}
	for _, it := range items {
		if it.title == "Excalidraw" && (it.runLabel != T("palette.view") || it.altLabel != T("action.build")) {
			t.Errorf("draft labels %q / %q", it.runLabel, it.altLabel)
		}
	}
	// Nothing matches: no results (the view shows its empty state).
	a.palette.query = "zzqx"
	if got := a.paletteItems(); len(got) != 0 {
		t.Errorf("zzqx: %d results", len(got))
	}
}
