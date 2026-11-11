package app

import (
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/pkg/config"
)

// The command palette (⌘K): one keyboard-first place for every app,
// project and action. Typing a website address offers to make an app of
// it. It springs in under the toolbar over the dimmed window.

type paletteState struct {
	open  bool
	query string
	sel   int
	focus bool
	// iconAt is where each project's row icon showed (for its flight).
	iconAt map[string]ui.Rect
}

type cmdItem struct {
	key   string
	icon  []byte  // app icon, or nil
	glyph *ui.SVG // action glyph
	name  string  // for the placeholder icon
	title string
	kind  string
	run   func()
	pid   string // a project, whose icon flies to its page
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

func (a *App) paletteItems() []cmdItem {
	ps := &a.palette
	var items []cmdItem
	q := strings.TrimSpace(ps.query)
	if q != "" && !strings.ContainsAny(q, " \t") && strings.Contains(q, ".") {
		if u, err := config.NormalizeURL(q); err == nil {
			items = append(items, cmdItem{key: "make", glyph: icons.Plus, title: T("palette.make", "url", config.RegistrableHost(u)),
				kind: T("palette.kind.action"), run: func() {
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
	var all []scored
	add := func(it cmdItem, text string) {
		if s := fuzzy(q, text); s > 0 {
			all = append(all, scored{it, s})
		}
	}
	for _, in := range a.installs {
		in := in
		add(cmdItem{key: "app:" + in.ID, icon: in.IconPNG, name: in.Name, title: T("palette.open", "name", in.Name), kind: T("palette.kind.app"),
			run: func() { a.openInstall(in.ID) }}, in.Name)
	}
	for i := range a.projects {
		pr := &a.projects[i]
		id := pr.ID
		add(cmdItem{key: "prj:" + id, pid: id, icon: pr.IconPNG, name: pr.Config.App.Name, title: pr.Config.App.Name, kind: T("palette.kind.project"),
			run: func() {
				a.flyFrom(iconKey(id), a.palette.iconAt[id], pr.IconPNG, pr.Config.App.Name)
				a.openProject(id, 0)
			}}, pr.Config.App.Name+" "+domainOf(pr.Config.App.Source))
	}
	actions := []struct {
		glyph *ui.SVG
		title string
		run   func()
	}{
		{icons.Plus, T("menu.newApp"), a.newApp},
		{icons.Library, T("nav.library"), func() { a.nav(secLibrary) }},
		{icons.Discover, T("nav.discover"), func() { a.nav(secDiscover) }},
		{icons.Builds, T("nav.builds"), func() { a.nav(secBuilds) }},
		{icons.Runtimes, T("nav.runtimes"), func() { a.nav(secRuntimes) }},
		{icons.Settings, T("nav.settings"), func() { a.nav(secSettings) }},
		{icons.Appearance, T("palette.appearance", "mode", T("appearance.light")), func() { a.setAppearance("light") }},
		{icons.Appearance, T("palette.appearance", "mode", T("appearance.dark")), func() { a.setAppearance("dark") }},
		{icons.Appearance, T("palette.appearance", "mode", T("appearance.system")), func() { a.setAppearance("system") }},
		{icons.Globe, T("settings.language"), func() { a.nav(secSettings) }},
		{icons.Import, T("menu.importApp"), a.importExistingApp},
	}
	for i, ac := range actions {
		ac := ac
		add(cmdItem{key: "act:" + ac.title, glyph: ac.glyph, title: ac.title, kind: T("palette.kind.action"), run: ac.run}, ac.title)
		_ = i
	}
	// Best first (stable for equal scores).
	for i := 1; i < len(all); i++ {
		for k := i; k > 0 && all[k].s > all[k-1].s; k-- {
			all[k], all[k-1] = all[k-1], all[k]
		}
	}
	for _, s := range all {
		items = append(items, s.it)
		if len(items) >= 9 {
			break
		}
	}
	return items
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
	run := func(i int) {
		if i < 0 || i >= len(items) {
			return
		}
		ps.open = false
		items[i].run()
	}
	if ps.iconAt == nil {
		ps.iconAt = map[string]ui.Rect{}
	}
	ui.DialogBase(c.Key("palette"), &ps.open, func(backdrop, panel ui.Element) {
		fade := design.Tween(design.MotionBase, design.EaseStd)
		kit.MarkModal()
		backdrop.Background(p.Scrim).Justify(ui.Start).Padding(96, 24, 24, 24).
			Transition(ui.ElementTransition{Animation: &fade, Enter: &ui.Motion{}, Exit: &ui.Motion{}})
		panel.Width(620).MaxWidthPercent(100).Gap(0).Radius(design.RadiusPanel+2).Background(p.Overlay).Border(0.5, p.BorderStrong).Clip().
			Transition(ui.ElementTransition{Animation: &design.Spring, Enter: &ui.Motion{Y: -8, Scale: 0.96}, Exit: &ui.Motion{Y: -4, Scale: 0.96}}).
			Label(T("palette.placeholder"))
		design.Elevate(c, panel, design.Modal)
		ui.Row(c).Height(58).Padding(0, 18).Gap(12).AlignItems(ui.Center).Children(func() {
			kit.Glyph(c, icons.Search, 19, p.TextTertiary)
			in := ui.TextInputBase(c.Key("q"), &ps.query).Grow(1).MinWidth(0).FocusRing(false).FontSize(design.FontTitle2).
				Placeholder(T("palette.placeholder")).PlaceholderColor(p.TextTertiary).TextColor(p.Text).Label(T("palette.placeholder")).AutoFocus()
			in.OnTextKey(func(k ui.TextKey) bool {
				switch k.Key {
				case ui.KeyDown:
					ps.sel = min(ps.sel+1, len(items)-1)
					return true
				case ui.KeyUp:
					ps.sel = max(ps.sel-1, 0)
					return true
				case ui.KeyEnter:
					run(ps.sel)
					return true
				}
				return false
			})
			if in.Changed() {
				ps.sel = 0
			}
			kit.Kbd(c, "esc")
		})
		kit.Hairline(c)
		ui.Column(c).Padding(6).Gap(1).Role(ui.RoleListBox).Children(func() {
			if len(items) == 0 {
				ui.Text(c, T("palette.empty")).FontSize(design.FontBody).TextColor(p.TextTertiary).Padding(18, 14)
			}
			for i, it := range items {
				i, it := i, it
				row := ui.ButtonBase(c.Key(it.key)).Label(it.title).Row().Justify(ui.Start).Height(42).PaddingX(12).Gap(12).
					AlignItems(ui.Center).Radius(design.RadiusCard - 2).Cursor(ui.CursorPointer).Role(ui.RoleOption).Selected(i == ps.sel)
				row.Transition(design.Transition(c))
				if i == ps.sel {
					row.Background(p.AccentSoft)
				}
				if row.Hovered() {
					ps.sel = i
				}
				row.Children(func() {
					if it.glyph != nil {
						ui.Row(c).Size(26, 26).Center().Radius(7).Background(p.Fill).Children(func() {
							kit.Glyph(c, it.glyph, 15, p.TextSecondary)
						})
					} else {
						ic := kit.AppIconOpts(c, it.icon, it.name, 26, kit.IconOpts{})
						if it.pid != "" {
							ps.iconAt[it.pid] = ic.Bounds()
						}
					}
					ui.Text(c, it.title).FontSize(design.FontBodyLG).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine().Grow(1).MinWidth(0)
					ui.Text(c, it.kind).FontSize(design.FontCaption).TextColor(p.TextTertiary).SingleLine()
					if i == ps.sel {
						kit.Glyph(c, icons.ArrowRight, 13, p.AccentText)
					}
				})
				if row.Clicked() {
					run(i)
				}
			}
		})
		ui.Row(c).Height(34).Padding(0, 16).Gap(14).AlignItems(ui.Center).Background(p.Sunken).Children(func() {
			for _, h := range [][2]string{{"↑↓", T("palette.hint.move")}, {"↩", T("palette.hint.run")}} {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, h[0]).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).TextColor(p.TextSecondary).
						Padding(1, 6).Radius(5).Background(p.Fill)
					kit.Footnote(c, h[1])
				})
			}
		})
	})
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
