package app

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
)

// libSort is the order of the Library's shelves.
type libSort string

const (
	sortName     libSort = "name"
	sortModified libSort = "modified"
	sortStatus   libSort = "status"
)

func (s libSort) valid() bool { return s == sortName || s == sortModified || s == sortStatus }

// libFilter narrows the Library to one kind of app.
type libFilter int

const (
	filterAll libFilter = iota
	filterUpdates
	filterAttention
	filterDrafts
)

// libEntry is one app or draft as the Library sorts and filters it.
type libEntry struct {
	id       string
	name     string
	tone     kit.Tone
	modified time.Time
	update   bool // an installed app with an update
	draft    bool // a project with no installed app
}

// toneRank puts what needs the user first when sorting by status.
func toneRank(t kit.Tone) int {
	switch t {
	case kit.ToneDanger:
		return 0
	case kit.ToneWarning:
		return 1
	case kit.ToneAccent:
		return 2
	case kit.ToneSuccess:
		return 3
	}
	return 4
}

// needsAttention is a failed, broken or missing app.
func (e libEntry) needsAttention() bool { return e.tone == kit.ToneDanger || e.tone == kit.ToneWarning }

func (e libEntry) matches(f libFilter) bool {
	switch f {
	case filterUpdates:
		return e.update
	case filterAttention:
		return e.needsAttention()
	case filterDrafts:
		return e.draft
	}
	return true
}

// sortEntries orders es in place: by name, newest first, or what needs
// attention first; ties always fall back to the name, then the ID.
func sortEntries(es []libEntry, s libSort) {
	byName := func(a, b libEntry) int {
		if c := strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)); c != 0 {
			return c
		}
		return strings.Compare(a.id, b.id)
	}
	slices.SortStableFunc(es, func(a, b libEntry) int {
		switch s {
		case sortModified:
			if c := b.modified.Compare(a.modified); c != 0 {
				return c
			}
		case sortStatus:
			if c := cmp.Compare(toneRank(a.tone), toneRank(b.tone)); c != 0 {
				return c
			}
		}
		return byName(a, b)
	})
}

// filterCounts counts the entries each filter shows.
func filterCounts(es []libEntry) [4]int {
	var n [4]int
	for _, e := range es {
		for f := filterAll; f <= filterDrafts; f++ {
			if e.matches(f) {
				n[f]++
			}
		}
	}
	return n
}

// libNav is the visual order of the shelves drawn last frame, which the
// arrow keys move through: one slice of IDs per section, cols to a row.
type libNav struct {
	secs [][]string
	cols int
}

func (n *libNav) find(id string) (s, i int, ok bool) {
	for s, sec := range n.secs {
		if i := slices.Index(sec, id); i >= 0 {
			return s, i, true
		}
	}
	return 0, 0, false
}

// step returns the ID the focus moves to from id with an arrow: dx moves
// along the reading order (across sections), dy a row up or down, into
// the section before or after at the same column where a section ends.
// It returns "" where there is nowhere to go.
func (n *libNav) step(id string, dx, dy int) string {
	s, i, ok := n.find(id)
	if !ok {
		return ""
	}
	cols := max(1, n.cols)
	if dx != 0 {
		var flat []string
		for _, sec := range n.secs {
			flat = append(flat, sec...)
		}
		k := slices.Index(flat, id) + dx
		if k < 0 || k >= len(flat) {
			return ""
		}
		return flat[k]
	}
	sec := n.secs[s]
	col := i % cols
	switch {
	case dy > 0:
		if j := i + cols; j < len(sec) {
			return sec[j]
		}
		// A shorter last row below: its last item.
		if i/cols < (len(sec)-1)/cols {
			return sec[len(sec)-1]
		}
		for t := s + 1; t < len(n.secs); t++ {
			if len(n.secs[t]) > 0 {
				return n.secs[t][min(col, len(n.secs[t])-1)]
			}
		}
	case dy < 0:
		if j := i - cols; j >= 0 {
			return sec[j]
		}
		for t := s - 1; t >= 0; t-- {
			if p := n.secs[t]; len(p) > 0 {
				last := (len(p) - 1) / cols * cols
				return p[min(last+col, len(p)-1)]
			}
		}
	}
	return ""
}

// handle is the focus handle of the tile or row of id.
func (l *libState) handle(id string) *ui.Handle {
	if l.handles == nil {
		l.handles = map[string]*ui.Handle{}
	}
	h := l.handles[id]
	if h == nil {
		h = &ui.Handle{}
		l.handles[id] = h
	}
	return h
}

// arrows moves the focus from the tile or row b of id when an arrow is
// pressed on it (in the list only Up and Down).
func (l *libState) arrows(b ui.Element, id string, grid bool) {
	to := ""
	switch {
	case b.Shortcut(0, ui.KeyDown):
		to = l.nav.step(id, 0, 1)
	case b.Shortcut(0, ui.KeyUp):
		to = l.nav.step(id, 0, -1)
	case grid && b.Shortcut(0, ui.KeyRight):
		to = l.nav.step(id, 1, 0)
	case grid && b.Shortcut(0, ui.KeyLeft):
		to = l.nav.step(id, -1, 0)
	}
	if to != "" {
		l.handle(to).Focus()
	}
}

// focusFirst moves the focus to the first tile or row, from the search
// field on Down.
func (l *libState) focusFirst() {
	for _, sec := range l.nav.secs {
		if len(sec) > 0 {
			l.handle(sec[0]).Focus()
			return
		}
	}
}

// sortMenu is the toolbar's Sort By menu.
func (a *App) sortMenu(c *ui.Context) {
	cur := a.lib.sort
	b := kit.ToolbarIcon(c.Key("sort"), icons.Sort, T("library.sort.label")).Tooltip(T("library.sort.label"))
	b.Menu(func(m *ui.Menu) {
		for _, o := range []struct {
			s     libSort
			label string
		}{{sortName, T("library.sort.name")}, {sortModified, T("library.sort.modified")}, {sortStatus, T("library.sort.status")}} {
			if m.Item(o.label).Radio(cur == o.s).Chosen() && cur != o.s {
				a.lib.sort = o.s
				a.ui.LibrarySort = string(o.s)
				a.ui.save()
			}
		}
	})
}

// filterChips are the All · Updates · Needs Attention · Not Installed
// chips over the shelves; a chip with nothing in it is left out.
func (a *App) filterChips(c *ui.Context, n [4]int) {
	labels := [4]string{T("library.filter.all"), T("library.filter.updates"), T("library.filter.attention"), T("library.tab.projects")}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Role(ui.RoleRadioGroup).Label(T("library.filter.label")).FocusGroup(ui.Horizontal).
		Children(func() {
			for f := filterAll; f <= filterDrafts; f++ {
				if f != filterAll && n[f] == 0 {
					continue
				}
				if filterChip(c.Key(int(f)), labels[f], n[f], a.lib.filter == f) {
					a.lib.filter = f
				}
			}
		})
}

func filterChip(c *ui.Context, label string, count int, on bool) bool {
	p := design.Of(c)
	const h float32 = 26
	b := ui.ButtonBase(c).Label(label).Row().Height(h).PaddingX(11).Gap(6).AlignItems(ui.Center).Radius(h / 2).
		Cursor(ui.CursorPointer).Role(ui.RoleRadio).Checked(on).FocusRing(false).Transition(design.Transition(c))
	fg, sub := p.TextSecondary, p.TextTertiary
	switch {
	case on:
		b.Background(p.AccentSoft)
		fg, sub = p.AccentText, p.AccentText
	case b.Hovered() || b.FocusVisible():
		b.Background(p.Hover)
		fg = p.Text
	default:
		b.Background(p.Fill)
	}
	kit.Press(b, false, b.Pressed(), 0)
	b.Children(func() {
		kit.FocusMark(c, b.FocusVisible(), 8)
		w := design.WeightMedium
		if on {
			w = design.WeightSemibold
		}
		ui.Text(c, label).FontSize(design.FontSmall).FontWeight(w).TextColor(fg).SingleLine()
		ui.Text(c, countLabel(count)).FontSize(design.FontSmall).TextColor(sub).SingleLine().FontFeatures("tnum")
	})
	return b.Clicked() && !on
}

// updatable is the managed apps with an update and no build running.
func (a *App) updatable() []domain.Installation {
	var out []domain.Installation
	for _, in := range a.installs {
		if in.State == domain.InstallExternal || in.UpdateReason == "" {
			continue
		}
		if in.ProjectID != "" {
			if j := a.latestJob(in.ProjectID); j != nil && (j.Status == domain.JobRunning || j.Status == domain.JobQueued) {
				continue
			}
		}
		out = append(out, in)
	}
	return out
}

// updatesBanner is the slim bar over My Apps that tells of updates, with
// Review and Update All.
func (a *App) updatesBanner(c *ui.Context, ups []domain.Installation) {
	p := design.Of(c)
	l := &a.lib
	kit.Arrive(ui.Row(c.Key("updates")).Padding(10, 10, 10, 14).Gap(10).AlignItems(ui.Center).Radius(design.RadiusPanel).
		Background(p.AccentSoft).Role(ui.RoleStatus)).Children(func() {
		kit.Glyph(c, icons.Update, 16, p.AccentText)
		ui.Text(c, T("library.updates.banner", "count", len(ups))).FontSize(design.FontBody).FontWeight(design.WeightMedium).
			TextColor(p.Text).Grow(1).MinWidth(0).SingleLine()
		if len(ups) == 1 || l.filter != filterUpdates {
			if kit.Button(c.Key("review"), T("library.updates.review"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM}).Clicked() {
				if len(ups) == 1 {
					a.dlg.openUpdate(&ups[0])
				} else {
					l.filter = filterUpdates
				}
			}
		}
		label := T("library.updates.all")
		if len(ups) == 1 {
			label = T("action.update")
		}
		if kit.Button(c.Key("all"), label, kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlSM, Loading: l.updating}).Clicked() && !l.updating {
			a.updateAll(ups)
		}
	})
}

// updateAll starts an update of each app in ups; each runs as its own job.
func (a *App) updateAll(ups []domain.Installation) {
	l := &a.lib
	l.updating = true
	type pending struct{ id, pid string }
	var todo []pending
	for _, in := range ups {
		todo = append(todo, pending{in.ID, in.ProjectID})
	}
	var first error
	started := 0
	a.do(func(ctx context.Context) error {
		for _, t := range todo {
			j, err := a.svc.UpdateInstallation(ctx, t.id, "gui")
			if err != nil {
				if first == nil {
					first = err
				}
				continue
			}
			started++
			pid := t.pid
			a.win.Update(func() { a.watch[pid] = j.ID; a.upsertJob(j) })
		}
		return first
	}, func(err error) {
		l.updating = false
		if started > 0 {
			a.toast(T("library.updates.started", "count", started), "", 1)
		}
		a.toastErr(err)
	})
}
