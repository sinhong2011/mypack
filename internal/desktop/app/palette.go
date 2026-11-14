package app

import (
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// The command palette (⌘K): one keyboard-first place for every app,
// project and action. Typing a website address offers to make an app of
// it. It springs in under the toolbar over the dimmed window.

type paletteState struct {
	open  bool
	query string
	sel   int
	moved bool // the arrows moved the selection: scroll it into view
	focus bool
}

// cmdItem is one result. Every app is one row (installed or not); run is
// Enter, alt ⌘Enter (labelled by runLabel and altLabel in the hint bar).
type cmdItem struct {
	key      string
	section  string
	icon     []byte  // app icon, or nil
	glyph    *ui.SVG // action glyph
	name     string  // for the placeholder icon
	title    string
	sub      string // website, size or status
	subTone  kit.Tone
	hotkey   string // an accelerator, e.g. "CmdOrCtrl+N"
	run      func()
	runLabel string
	alt      func()
	altLabel string
	pid      string    // a project, whose icon flies to its page
	at       time.Time // last managed or edited, for Recent
}

// fuzzy scores how well q matches s: 0 = no match; higher is better
// (prefix and word-start matches first).
func fuzzy(q, s string) int {
	q, s = strings.ToLower(strings.TrimSpace(q)), strings.ToLower(s)
	if q == "" {
		return 1
	}
	if strings.HasPrefix(s, q) {
		return 1000 - len(s)
	}
	if i := strings.Index(s, q); i >= 0 {
		if i > 0 && !unicode.IsLetter(rune(s[i-1])) {
			return 800 - len(s)
		}
		return 600 - len(s)
	}
	// Subsequence.
	j, score := 0, 0
	for i := 0; i < len(s) && j < len(q); i++ {
		if s[i] == q[j] {
			j++
			score += 10
		}
	}
	if j < len(q) {
		return 0
	}
	return score - len(s)
}

// paletteApps is every app once: the installed ones, then the projects
// not installed, with what Enter and ⌘Enter do to each.
func (a *App) paletteApps() []cmdItem {
	var out []cmdItem
	for i := range a.installs {
		in := a.installs[i]
		site := a.installDomain(&in)
		if pr := a.project(in.ProjectID); pr != nil && pr.Config.IsGroup() {
			site = groupLine(pr.Config)
		}
		it := cmdItem{key: "app:" + in.ID, icon: in.IconPNG, name: in.Name, title: in.Name, sub: site, pid: in.ProjectID, at: in.LastManagedAt,
			run: func() { a.openInstall(in.ID) }, runLabel: T("action.open"),
			alt: func() { a.dlg.details = detailsDialog{open: true, id: in.ID} }, altLabel: T("action.details")}
		if st, tone := a.installStatus(&in); tone != kit.ToneSuccess && tone != kit.ToneNeutral {
			it.sub, it.subTone = st, tone
		}
		if in.State == domain.InstallMissing {
			it.run, it.runLabel = it.alt, T("action.locate")
		}
		out = append(out, it)
	}
	for i := range a.projects {
		pr := &a.projects[i]
		if a.installFor(pr.ID) != nil {
			continue
		}
		id := pr.ID
		sub := domainOf(pr.Config.App.Source)
		if pr.Config.IsGroup() {
			sub = groupLine(pr.Config)
		}
		st, tone, _ := a.projectStatus(pr)
		it := cmdItem{key: "prj:" + id, pid: id, icon: pr.IconPNG, name: pr.Config.App.Name, title: pr.Config.App.Name, at: pr.UpdatedAt,
			sub: st + " · " + sub, subTone: kit.ToneNeutral,
			run: func() { a.openProject(id, 0) }, runLabel: T("palette.view"),
			alt: func() { a.startBuild(id, nil) }, altLabel: T("action.build")}
		if tone == kit.ToneDanger {
			it.subTone = tone
		}
		out = append(out, it)
	}
	return out
}

// paletteActions is what the palette can do besides opening apps:
// create, go to and settings, with their menu shortcuts.
func (a *App) paletteActions() []cmdItem {
	create, goTo, settings := T("palette.section.create"), T("palette.section.goTo"), T("nav.settings")
	act := func(sec string, glyph *ui.SVG, title, hotkey string, run func()) cmdItem {
		return cmdItem{key: "act:" + title, section: sec, glyph: glyph, title: title, hotkey: hotkey, run: run, runLabel: T("palette.hint.run")}
	}
	return []cmdItem{
		act(create, icons.Plus, T("menu.newApp"), a.accel(scNewApp), a.newApp),
		act(create, icons.Layers, T("menu.newGroup"), a.accel(scNewGroup), a.newGroup),
		act(create, icons.Import, T("menu.importApp"), "", a.importExistingApp),
		act(goTo, icons.Library, T("nav.library"), a.accel(scLibrary), func() { a.nav(secLibrary) }),
		act(goTo, icons.Discover, T("nav.discover"), a.accel(scDiscover), func() { a.nav(secDiscover) }),
		act(goTo, icons.Builds, T("nav.builds"), a.accel(scBuilds), func() { a.nav(secBuilds) }),
		act(goTo, icons.Runtimes, T("nav.runtimes"), a.accel(scRuntimes), func() { a.nav(secRuntimes) }),
		act(goTo, icons.Settings, T("nav.settings"), a.accel(scSettings), func() { a.openSettings("") }),
		act(settings, icons.Appearance, T("palette.appearance", "mode", T("appearance.light")), "", func() { a.setAppearance("light") }),
		act(settings, icons.Appearance, T("palette.appearance", "mode", T("appearance.dark")), "", func() { a.setAppearance("dark") }),
		act(settings, icons.Appearance, T("palette.appearance", "mode", T("appearance.system")), "", func() { a.setAppearance("system") }),
		act(settings, icons.Globe, T("settings.language"), "", func() { a.openSettings(paneGeneral) }),
		act(settings, icons.Keyboard, T("menu.shortcuts"), a.accel(scShortcuts), a.showShortcuts),
		act(settings, icons.Sliders, T("menu.customizeShortcuts"), "", func() { a.openSettings(paneShortcuts) }),
	}
}

// paletteItems is the results in their sections. With no query: Pinned,
// recent apps, Create and Go To. With one: making an app of a typed
// address, the matching apps (best first, pinned ahead of an equal
// match), then the matching actions.
func (a *App) paletteItems() []cmdItem {
	q := strings.TrimSpace(a.palette.query)
	apps, actions := a.paletteApps(), a.paletteActions()
	pinRank := map[string]int{}
	for i, k := range a.ui.Pinned {
		pinRank[k] = i
	}
	pinKey := func(it cmdItem) string {
		if it.pid != "" {
			return it.pid
		}
		return strings.TrimPrefix(it.key, "app:")
	}
	var out []cmdItem
	if q == "" {
		var pinned, rest []cmdItem
		for _, it := range apps {
			if _, ok := pinRank[pinKey(it)]; ok {
				it.section = T("nav.pinned")
				pinned = append(pinned, it)
			} else {
				it.section = T("nav.recent")
				rest = append(rest, it)
			}
		}
		sort.SliceStable(pinned, func(i, j int) bool { return pinRank[pinKey(pinned[i])] < pinRank[pinKey(pinned[j])] })
		sort.SliceStable(rest, func(i, j int) bool { return rest[i].at.After(rest[j].at) })
		out = append(out, pinned...)
		out = append(out, rest[:min(len(rest), 4)]...)
		for _, ac := range actions {
			if ac.section != T("nav.settings") {
				out = append(out, ac)
			}
		}
		return out
	}
	if !strings.ContainsAny(q, " \t") && strings.Contains(q, ".") {
		if u, err := config.NormalizeURL(q); err == nil {
			out = append(out, cmdItem{key: "make", section: T("palette.section.create"), glyph: icons.Plus,
				title: T("palette.make", "url", config.RegistrableHost(u)), runLabel: T("palette.hint.run"), run: func() {
					a.create.reset(a.host)
					a.create.setInput(u)
					a.nav(secCreate)
				}})
		}
	}
	type scored struct {
		it cmdItem
		s  int
	}
	rank := func(list []cmdItem, text func(cmdItem) string, boost func(cmdItem) int, limit int) {
		var all []scored
		for _, it := range list {
			if s := fuzzy(q, text(it)); s > 0 {
				all = append(all, scored{it, s + boost(it)})
			}
		}
		sort.SliceStable(all, func(i, j int) bool { return all[i].s > all[j].s })
		for i, s := range all {
			if i == limit {
				break
			}
			out = append(out, s.it)
		}
	}
	for i := range apps {
		apps[i].section = T("launcher.results")
	}
	rank(apps, func(it cmdItem) string { return it.title }, func(it cmdItem) int {
		if _, ok := pinRank[pinKey(it)]; ok {
			return 50
		}
		return 0
	}, 6)
	rank(actions, func(it cmdItem) string { return it.title }, func(cmdItem) int { return 0 }, 6)
	return out
}

func (a *App) setAppearance(mode string) {
	a.settings.Appearance = mode
	applyAppearance(mode)
	a.saveSettings()
}

func (a *App) paletteView(c *ui.Context) {
	ps := &a.palette
	if !ps.open {
		return
	}
	p := design.Of(c)
	items := a.paletteItems()
	if ps.sel >= len(items) {
		ps.sel = max(0, len(items)-1)
	}
	run := func(i int, alt bool) {
		if i < 0 || i >= len(items) {
			return
		}
		it := items[i]
		if alt && it.alt == nil {
			return
		}
		ps.open = false
		if alt {
			it.alt()
		} else {
			it.run()
		}
	}
	move := func(d int) {
		if len(items) > 0 {
			ps.sel = (ps.sel + d + len(items)) % len(items)
			ps.moved = true
		}
	}
	ui.DialogBase(c.Key("palette"), &ps.open, func(backdrop, panel ui.Element) {
		fade := design.Tween(design.MotionBase, design.EaseStd)
		kit.MarkModal()
		backdrop.Background(p.Scrim).Justify(ui.Start).Padding(88, 24, 24, 24).
			Transition(ui.ElementTransition{Animation: &fade, Enter: &ui.Motion{}, Exit: &ui.Motion{}})
		panel.Width(640).MaxWidthPercent(100).Gap(0).Radius(design.RadiusPanel+2).Background(p.Overlay).Border(0.5, p.BorderStrong).Clip().
			Transition(ui.ElementTransition{Animation: &design.Spring, Enter: &ui.Motion{Y: -8, Scale: 0.97}, Exit: &ui.Motion{Y: -4, Scale: 0.97}}).
			Label(T("palette.placeholder"))
		design.Elevate(c, panel, design.Modal)
		// The field.
		ui.Row(c).Height(56).Padding(0, 16, 0, 18).Gap(12).AlignItems(ui.Center).Children(func() {
			kit.Glyph(c, icons.Search, 18, p.TextTertiary)
			in := ui.TextInputBase(c.Key("q"), &ps.query).Grow(1).MinWidth(0).FocusRing(false).FontSize(design.FontTitle3).
				Placeholder(T("palette.placeholder")).PlaceholderColor(p.TextTertiary).TextColor(p.Text).Label(T("palette.placeholder")).AutoFocus()
			in.OnTextKey(func(k ui.TextKey) bool {
				switch k.Key {
				case ui.KeyDown:
					move(1)
					return true
				case ui.KeyUp:
					move(-1)
					return true
				case ui.KeyEnter:
					run(ps.sel, k.Mods&ui.Cmd != 0)
					return true
				}
				return false
			})
			if in.Changed() {
				ps.sel = 0
			}
			if ps.query != "" {
				if kit.IconButton(c.Key("clear"), icons.Close, T("palette.clear")).Size(22, 22).Clicked() {
					ps.query, ps.sel = "", 0
				}
			}
		})
		kit.Hairline(c)
		// The results, in sections; they scroll past ~9 rows.
		q := strings.TrimSpace(ps.query)
		ui.Scroll(c).MaxHeight(420).Children(func() {
			ui.Column(c).Padding(6, 8, 8, 8).Gap(2).Role(ui.RoleListBox).Label(T("palette.placeholder")).Children(func() {
				if len(items) == 0 {
					ui.Column(c).AlignItems(ui.Center).Gap(6).Padding(28, 16).Children(func() {
						kit.Glyph(c, icons.Search, 20, p.TextTertiary)
						ui.Text(c, T("palette.empty")).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.TextSecondary)
						kit.Footnote(c, T("palette.empty.hint"))
					})
					return
				}
				section := ""
				for i, it := range items {
					i, it := i, it
					if it.section != section {
						section = it.section
						top := float32(12)
						if i == 0 {
							top = 6
						}
						ui.Text(c.Key("sec-"+section), section).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).
							TextColor(p.TextTertiary).Padding(top, 10, 6, 10).Role(ui.RoleHeading)
					}
					on := i == ps.sel
					row := ui.ButtonBase(c.Key(it.key)).Label(it.title).Row().Justify(ui.Start).Height(44).PaddingX(10).Gap(12).
						AlignItems(ui.Center).Radius(design.RadiusCard - 2).Cursor(ui.CursorPointer).Role(ui.RoleOption).Selected(on).
						FocusRing(false).Description(it.sub)
					if on {
						row.Background(p.Selected)
						if ps.moved {
							row.ScrollIntoView()
						}
					}
					if row.Hovered() && !ps.moved {
						ps.sel = i
					}
					row.Children(func() {
						if it.glyph != nil {
							fg, bg := p.TextSecondary, p.Fill
							if on {
								fg, bg = p.Text, p.Raised
							}
							ui.Row(c).Size(28, 28).Center().Radius(8).Background(bg).Shrink(0).Children(func() {
								kit.Glyph(c, it.glyph, 15, fg)
							})
						} else {
							ui.Box(c).Size(28, 28).Shrink(0).Children(func() {
								kit.AppIconOpts(c, it.icon, it.name, 28, kit.IconOpts{})
							})
						}
						ui.Column(c).Grow(1).MinWidth(0).Gap(0).Children(func() {
							paletteTitle(c, it.title, q)
							if it.sub != "" {
								col := p.TextTertiary
								if it.subTone != kit.ToneNeutral && it.subTone != kit.ToneSuccess {
									col, _ = kit.ToneColors(p, it.subTone)
								}
								ui.Text(c, it.sub).FontSize(design.FontCaption).TextColor(col).SingleLine()
							}
						})
						switch {
						case on:
							selHint(c, it.runLabel)
						case it.hotkey != "":
							kit.Kbd(c, it.hotkey)
						}
					})
					if row.Clicked() {
						run(i, false)
					}
				}
			})
		})
		ps.moved = false
		// What the keys do to the row chosen.
		ui.Row(c).Height(36).Padding(0, 14).Gap(16).AlignItems(ui.Center).Background(p.Sunken).Children(func() {
			hint := func(keys, label string) {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, keys).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).TextColor(p.TextSecondary).
						Padding(1, 6).Radius(5).Background(p.Fill)
					kit.Footnote(c, label)
				})
			}
			hint("↑↓", T("palette.hint.move"))
			if ps.sel < len(items) {
				it := items[ps.sel]
				hint("↩", it.runLabel)
				if it.alt != nil {
					hint(cmdEnter(), it.altLabel)
				}
			}
			ui.Spacer(c)
			hint("esc", T("launcher.close"))
		})
	})
}

// paletteTitle is a result's title with the part the query matched in
// the accent.
func paletteTitle(c *ui.Context, title, q string) {
	p := design.Of(c)
	base := ui.Span{Size: design.FontBody, Weight: design.WeightMedium, Color: p.Text}
	i := -1
	if q != "" && len(strings.ToLower(title)) == len(title) {
		i = strings.Index(strings.ToLower(title), strings.ToLower(q))
	}
	if i < 0 {
		ui.Text(c, title).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine()
		return
	}
	hit := base
	hit.Text, hit.Weight, hit.Color = title[i:i+len(q)], design.WeightBold, p.AccentText
	before, after := base, base
	before.Text, after.Text = title[:i], title[i+len(q):]
	ui.RichText(c, before, hit, after).SingleLine()
}

func (a *App) togglePalette() {
	ps := &a.palette
	ps.open = !ps.open
	if ps.open {
		ps.query, ps.sel = "", 0
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

// selHint is what Enter does to the chosen row, quietly at its right:
// the word, then a ↩ key chip.
func selHint(c *ui.Context, label string) {
	p := design.Of(c)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, label).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine()
		ui.Text(c, "↩").FontSize(design.FontCaption).FontWeight(design.WeightSemibold).TextColor(p.TextSecondary).
			Padding(1, 6).Radius(5).Background(p.Fill).Border(0.5, p.Border).AccessHidden(true)
	})
}
