package app

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/fakesvc"
	"mypack.dev/mypack/internal/desktop/kit"
)

func TestSortEntries(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	es := []libEntry{
		{id: "1", name: "beta", tone: kit.ToneSuccess, modified: t0},
		{id: "2", name: "Alpha", tone: kit.ToneDanger, modified: t0.Add(time.Hour)},
		{id: "3", name: "gamma", tone: kit.ToneAccent, modified: t0.Add(2 * time.Hour)},
		{id: "4", name: "delta", tone: kit.ToneWarning, modified: t0.Add(time.Hour)},
	}
	for _, tc := range []struct {
		s    libSort
		want []string
	}{
		{sortName, []string{"2", "1", "4", "3"}},
		{sortModified, []string{"3", "2", "4", "1"}}, // ties by name
		{sortStatus, []string{"2", "4", "3", "1"}},
	} {
		got := slices.Clone(es)
		sortEntries(got, tc.s)
		if ids := ids(got); !slices.Equal(ids, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.s, ids, tc.want)
		}
	}
}

func TestFilterCounts(t *testing.T) {
	es := []libEntry{
		{id: "a", tone: kit.ToneAccent, update: true},
		{id: "b", tone: kit.ToneDanger},
		{id: "c", tone: kit.ToneWarning},
		{id: "d", tone: kit.ToneNeutral, draft: true},
		{id: "e", tone: kit.ToneDanger, draft: true},
		{id: "g", tone: kit.ToneNeutral, draft: true, group: true}, // on the Groups shelf, not Not Installed
	}
	if got, want := filterCounts(es), [4]int{6, 1, 3, 2}; got != want {
		t.Errorf("counts %v, want %v", got, want)
	}
	if got := ids(keep(es, filterAttention)); !slices.Equal(got, []string{"b", "c", "e"}) {
		t.Errorf("attention %v", got)
	}
}

func TestNavStep(t *testing.T) {
	// Two sections in rows of 3:
	//   a b c
	//   d e
	//   ---
	//   x y
	n := libNav{secs: [][]string{{"a", "b", "c", "d", "e"}, {"x", "y"}}, cols: 3}
	for _, tc := range []struct {
		from   string
		dx, dy int
		want   string
	}{
		{"a", 1, 0, "b"},
		{"e", 1, 0, "x"}, // reading order crosses sections
		{"x", -1, 0, "e"},
		{"a", -1, 0, ""}, // nothing before the first
		{"a", 0, 1, "d"},
		{"c", 0, 1, "e"}, // the shorter row below: its last
		{"d", 0, 1, "x"}, // into the next section, same column
		{"e", 0, 1, "y"},
		{"x", 0, -1, "d"}, // up into the last row before
		{"y", 0, -1, "e"},
		{"b", 0, -1, ""},
		{"y", 0, 1, ""},
		{"zz", 1, 0, ""},
	} {
		if got := n.step(tc.from, tc.dx, tc.dy); got != tc.want {
			t.Errorf("step(%s, %d, %d) = %q, want %q", tc.from, tc.dx, tc.dy, got, tc.want)
		}
	}
	// The list: one column.
	l := libNav{secs: [][]string{{"a", "b"}, {"x"}}, cols: 1}
	if got := l.step("b", 0, 1); got != "x" {
		t.Errorf("list down = %q", got)
	}
}

// TestLibraryArrowKeys drives the Library headlessly: the arrows walk the
// grid across both shelves, and Down in the list crosses into the drafts.
func TestLibraryArrowKeys(t *testing.T) {
	svc := fakesvc.New()
	ctx := context.Background()
	a := &App{svc: svc}
	a.projects, _ = svc.ListProjects(ctx)
	a.installs, _ = svc.ListInstallations(ctx)
	a.jobs, _ = svc.ListJobs(ctx, "", 200)
	a.settings, _ = svc.Settings(ctx)
	a.loaded = true
	name := map[string]string{}
	for _, in := range a.installs {
		name[in.ID] = in.Name
	}
	for _, p := range a.projects {
		name[p.ID] = p.Config.App.Name
	}
	focused := ""
	tt := ui.NewTester(func(c *ui.Context) {
		a.libraryView(c)
		focused = ""
		for id, h := range a.lib.handles {
			if h.Focused(c) {
				focused = name[id]
			}
		}
	}, 1100, 820)
	settle := func() { tt.Frame(); tt.Frame() }
	settle()
	a.lib.handle(a.lib.nav.secs[0][0]).Focus()
	settle()
	got := []string{focused}
	for _, k := range []ui.Key{ui.KeyRight, ui.KeyRight, ui.KeyDown, ui.KeyLeft, ui.KeyUp, ui.KeyDown, ui.KeyDown} {
		tt.Key(0, k)
		settle()
		got = append(got, focused)
	}
	// My Apps: ChatGPT Linear WhatsApp YouTube Music; Groups: Social
	// Workspace; Not Installed: Discord Excalidraw Figma Notion Spotify /
	// Todoist (5 to a row).
	want := []string{"ChatGPT", "Linear", "WhatsApp", "Workspace", "Social", "ChatGPT", "Social", "Discord"}
	if !slices.Equal(got, want) {
		t.Errorf("grid focus path %v, want %v", got, want)
	}
	a.lib.view = 1
	tt.Frame()
	a.lib.handle(a.lib.nav.secs[0][3]).Focus()
	settle()
	tt.Key(0, ui.KeyDown)
	settle()
	if focused != "Social" {
		t.Errorf("list Down from the last app focused %q, want Social", focused)
	}
	// The groups have their own shelf, apart from the single websites.
	if got := a.lib.nav.secs; len(got) != 3 || len(got[1]) != 2 {
		t.Errorf("shelves %v, want My Apps, Groups (2), Not Installed", got)
	}
}
