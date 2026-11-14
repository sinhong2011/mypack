package app

import (
	"fmt"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/pkg/config"
)

// Demo group windows match the built app: a hidden title bar, a rail of
// member icons, Settings on the rail (Linux/Windows hide the GTK/Win32
// menu when the title bar is gone), and no auto-open of the first site
// in Chrome.

const (
	demoRailWidth = 68
	demoRailIcon  = 40
)

func (d demoLaunch) isGroup() bool {
	return d.cfg != nil && d.cfg.IsGroup()
}

func (d demoLaunch) members() []config.GroupMember {
	if !d.isGroup() {
		return nil
	}
	return d.cfg.App.Group.Members
}

func memberHome(m config.GroupMember) string {
	if m.IsLocal() {
		return m.Local
	}
	return m.URL
}

func (a *App) invalidateDemo(d demoLaunch) {
	if w := a.demoApps[d.key]; w != nil && !w.IsDestroyed() {
		w.Invalidate()
	}
}

func (p *demoPrefs) clamp(n int) {
	if n <= 0 {
		p.Active, p.Split = 0, -1
		return
	}
	if p.Active < 0 || p.Active >= n {
		p.Active = 0
	}
	if p.Split == p.Active || p.Split < 0 || p.Split >= n {
		p.Split = -1
	}
	if p.SplitPct <= 0 || p.SplitPct > 100 {
		p.SplitPct = 50
	}
}

func (p *demoPrefs) show(i, n int) {
	p.clamp(n)
	if i < 0 || i >= n {
		return
	}
	if i == p.Split {
		p.Split = p.Active
	}
	p.Active = i
	if p.Split == p.Active {
		p.Split = -1
	}
}

func (p *demoPrefs) step(d, n int) {
	if n <= 0 {
		return
	}
	p.clamp(n)
	p.show(((p.Active+d)%n+n)%n, n)
}

func (p *demoPrefs) openSplit(n int) {
	if p.Split >= 0 || n < 2 {
		return
	}
	o := p.Active + 1
	if o >= n {
		o = 0
	}
	p.Split = o
}

func (p *demoPrefs) toggleSplit(n int) {
	p.clamp(n)
	if p.Split >= 0 {
		p.Split = -1
		return
	}
	p.openSplit(n)
}

func (p *demoPrefs) setSplitVert(vert bool, n int) {
	p.clamp(n)
	p.openSplit(n)
	if p.Split >= 0 {
		p.SplitVert = vert
	}
}

func (p *demoPrefs) rotateSplit(n int) {
	p.clamp(n)
	if p.Split < 0 {
		p.openSplit(n)
		p.SplitVert = true
		return
	}
	p.SplitVert = !p.SplitVert
}

func (p *demoPrefs) swapSplit() {
	if p.Split < 0 {
		return
	}
	p.Active, p.Split = p.Split, p.Active
}

func (p *demoPrefs) splitWith(i, n int, vert bool) {
	p.clamp(n)
	if i < 0 || i >= n || i == p.Active {
		return
	}
	p.Split = i
	p.SplitVert = vert
}

func (a *App) demoGroupViewMenu(d demoLaunch) *mygo.MenuItem {
	members := d.members()
	n := len(members)
	pref := a.prefsFor(d)
	items := make([]*mygo.MenuItem, 0, n+10)
	for i, m := range members {
		i := i
		it := &mygo.MenuItem{Label: m.Name, Click: func(*mygo.MenuItem, *mygo.Window) {
			a.prefsFor(d).show(i, n)
			a.invalidateDemo(d)
		}}
		if i < 9 {
			it.Accelerator = fmt.Sprintf("CmdOrCtrl+%d", i+1)
		}
		items = append(items, it)
	}
	items = append(items,
		mygo.Separator(),
		&mygo.MenuItem{Label: T("group.next"), Accelerator: "Ctrl+Tab", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.prefsFor(d).step(+1, n)
			a.invalidateDemo(d)
		}},
		&mygo.MenuItem{Label: T("group.prev"), Accelerator: "Ctrl+Shift+Tab", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.prefsFor(d).step(-1, n)
			a.invalidateDemo(d)
		}},
		mygo.Separator(),
		&mygo.MenuItem{Label: T("group.split"), Accelerator: "CmdOrCtrl+\\", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.prefsFor(d).toggleSplit(n)
			a.invalidateDemo(d)
		}},
		&mygo.MenuItem{Label: T("group.split.side"), Type: mygo.MenuItemRadio, Checked: pref.Split >= 0 && !pref.SplitVert, Click: func(*mygo.MenuItem, *mygo.Window) {
			a.prefsFor(d).setSplitVert(false, n)
			a.invalidateDemo(d)
		}},
		&mygo.MenuItem{Label: T("group.split.stack"), Type: mygo.MenuItemRadio, Checked: pref.Split >= 0 && pref.SplitVert, Click: func(*mygo.MenuItem, *mygo.Window) {
			a.prefsFor(d).setSplitVert(true, n)
			a.invalidateDemo(d)
		}},
		&mygo.MenuItem{Label: T("group.split.rotate"), Accelerator: "CmdOrCtrl+Shift+\\", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.prefsFor(d).rotateSplit(n)
			a.invalidateDemo(d)
		}},
		&mygo.MenuItem{Label: T("group.split.swap"), Click: func(*mygo.MenuItem, *mygo.Window) {
			a.prefsFor(d).swapSplit()
			a.invalidateDemo(d)
		}},
		&mygo.MenuItem{Label: T("group.split.close"), Click: func(*mygo.MenuItem, *mygo.Window) {
			p := a.prefsFor(d)
			if p.Split >= 0 {
				p.toggleSplit(n)
				a.invalidateDemo(d)
			}
		}},
	)
	return &mygo.MenuItem{Label: T("menu.view"), Submenu: items}
}

func (a *App) demoGroupView(c *ui.Context, d demoLaunch) {
	p := design.Of(c)
	design.Apply(c)
	members := d.members()
	pref := a.prefsFor(d)
	pref.clamp(len(members))
	top := float32(8)
	if runtime.GOOS == "darwin" {
		top = 40
	}
	ui.Row(c).Fill().Gap(0).Background(p.CanvasBot).Children(func() {
		rail := ui.Column(c.Key("rail")).Width(demoRailWidth).FillHeight().Shrink(0).Gap(8).AlignItems(ui.Center).
			Padding(top, 0, 12, 0).Background(p.Raised).DragWindow()
		rail.Children(func() {
			for i, m := range members {
				a.demoRailItem(c.Key("m-"+m.ID), d, m, i, i == pref.Active, i == pref.Split)
			}
			ui.Spacer(c).Grow(1)
			a.demoRailSplit(c, d, pref)
			a.demoRailSettings(c, d)
		})
		ui.Box(c).Width(1).FillHeight().Background(p.Divider)
		ui.Column(c).Grow(1).FillHeight().MinWidth(0).Gap(0).Children(func() {
			if ownWindowButtons {
				ui.Row(c.Key("caption")).FillWidth().Height(32).Shrink(0).Padding(0, 8).AlignItems(ui.Center).
					Background(p.Raised).DragWindow().Children(func() {
					ui.Spacer(c)
					windowButtons(c, a.demoApps[d.key])
				})
				ui.Box(c).Height(1).FillWidth().Background(p.Divider)
			}
			a.demoGroupMembers(c, d, members, pref)
		})
	})
}

func (a *App) demoRailSettings(c *ui.Context, d demoLaunch) {
	p := design.Of(c)
	btn := ui.Box(c.Key("settings")).Size(demoRailWidth, demoRailIcon).Radius(10).
		Role(ui.RoleButton).Label(T("demo.settings")).Tooltip(T("demo.settings")).Focusable().
		AlignItems(ui.Center).Justify(ui.Center)
	if btn.Hovered() || btn.FocusVisible() {
		btn.Background(p.Hover)
	}
	btn.Children(func() {
		kit.Glyph(c, icons.Settings, 18, p.TextSecondary)
	})
	if btn.Clicked() {
		a.openDemoSettings(d)
	}
}

func (a *App) demoRailItem(c *ui.Context, d demoLaunch, m config.GroupMember, i int, active, beside bool) {
	p := design.Of(c)
	n := len(d.members())
	pref := a.prefsFor(d)
	ui.Box(c).Size(demoRailWidth, demoRailIcon).Children(func() {
		if active || beside {
			bar := ui.Box(c).Absolute().Left(0).Top(8).Size(3, demoRailIcon-16).Radius(1.5)
			if active {
				bar.Background(p.Accent)
			} else {
				bar.Background(p.TextSecondary)
			}
		}
		btn := ui.Box(c.Key("btn")).Absolute().Left((demoRailWidth-demoRailIcon)/2).Top(0).Size(demoRailIcon, demoRailIcon).Radius(10).Clip().
			Role(ui.RoleButton).Label(m.Name).Tooltip(m.Name).Focusable()
		btn.Children(func() {
			kit.AppIconOpts(c, a.memberIcon(m), m.Name, demoRailIcon, kit.IconOpts{})
		})
		if !active {
			btn.Opacity(0.82)
		}
		if btn.Clicked() {
			pref.show(i, n)
		}
		btn.ContextMenu(func(menu *ui.Menu) {
			if menu.Item(T("demo.split.side")).Disabled(active).Chosen() {
				pref.splitWith(i, n, false)
			}
			if menu.Item(T("demo.split.stack")).Disabled(active).Chosen() {
				pref.splitWith(i, n, true)
			}
			if beside && menu.Item(T("demo.split.close")).Chosen() {
				pref.toggleSplit(n)
			}
			menu.Separator()
			if home := memberHome(m); home != "" && menu.Item(T("demo.open.browser")).Chosen() {
				if err := openSiteApp(home); err != nil {
					a.toastErr(err)
				}
			}
		})
	})
}

func (a *App) demoGroupMembers(c *ui.Context, d demoLaunch, members []config.GroupMember, pref *demoPrefs) {
	active, split, vert := pref.Active, pref.Split, pref.SplitVert
	ui.Box(c.Key("members")).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Column().Gap(0).Children(func() {
		if active < 0 || active >= len(members) {
			ui.Spacer(c)
			return
		}
		if split < 0 || split >= len(members) {
			a.demoMemberPane(c.Key("main"), d, members[active], true)
			return
		}
		first := func() { a.demoMemberPane(c.Key("left"), d, members[active], true) }
		second := func() { a.demoMemberPane(c.Key("right"), d, members[split], false) }
		var panes ui.Element
		if vert {
			panes = ui.SplitVerticalPercent(c.Key("split-v"), &pref.SplitPct, first, second)
		} else {
			panes = ui.SplitPercent(c.Key("split-h"), &pref.SplitPct, first, second)
		}
		panes.SplitReset(50).Grow(1).FillWidth().FillHeight().MinWidth(0).MinHeight(0)
	})
}

func (a *App) demoRailSplit(c *ui.Context, d demoLaunch, pref *demoPrefs) {
	p := design.Of(c)
	n := len(d.members())
	on := pref.Split >= 0
	ic := icons.Columns2
	if on && pref.SplitVert {
		ic = icons.Rows2
	}
	btn := ui.ButtonBase(c.Key("split")).Size(demoRailWidth, demoRailIcon).Radius(10).
		Label(T("group.split")).Tooltip(T("group.split")).FocusRing(false).Cursor(ui.CursorPointer).
		AlignItems(ui.Center).Justify(ui.Center)
	if on {
		btn.Background(p.Accent.Alpha(0.18))
	} else if btn.Hovered() || btn.FocusVisible() {
		btn.Background(p.Hover)
	}
	btn.Children(func() {
		fg := p.TextSecondary
		if on {
			fg = p.Accent
		}
		kit.Glyph(c, ic, 18, fg)
	})
	btn.PopoverMenuButton(func(m *ui.Menu) {
		if m.Item(T("group.split.side")).Radio(on && !pref.SplitVert).Chosen() {
			pref.setSplitVert(false, n)
			a.invalidateDemo(d)
		}
		if m.Item(T("group.split.stack")).Radio(on && pref.SplitVert).Chosen() {
			pref.setSplitVert(true, n)
			a.invalidateDemo(d)
		}
		m.Separator()
		if m.Item(T("group.split.swap")).Disabled(!on).Chosen() {
			pref.swapSplit()
			a.invalidateDemo(d)
		}
		if m.Item(T("group.split.close")).Disabled(!on).Chosen() {
			pref.toggleSplit(n)
			a.invalidateDemo(d)
		}
	}, ui.PopoverMenuButtonOptions{At: ui.AnchorTopEnd, Self: ui.AnchorTopStart})
}

func (a *App) demoMemberPane(c *ui.Context, d demoLaunch, m config.GroupMember, focused bool) {
	p := design.Of(c)
	home := memberHome(m)
	col := ui.Column(c).Grow(1).Fill().MinWidth(0).MinHeight(0).Background(p.CanvasBot).Center().Gap(14).Padding(28)
	if focused {
		col.Border(2, p.Accent)
	} else {
		col.Border(1, p.Divider)
	}
	col.Children(func() {
		kit.AppIconOpts(c, a.memberIcon(m), m.Name, 72, kit.IconOpts{Shadow: true})
		kit.LargeTitle(c, m.Name)
		if home != "" {
			kit.Secondary(c, home).SingleLine()
		}
		kit.Caption(c, T("demo.group.body"))
		ui.Row(c).Gap(10).Children(func() {
			if kit.Button(c.Key("set"), T("demo.settings"), kit.ButtonOpts{Size: design.ControlLG, Icon: icons.Settings}).Clicked() {
				a.openDemoSettings(d)
			}
			if home != "" {
				if kit.Button(c.Key("open"), T("demo.open.browser"), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlLG, Icon: icons.Globe}).Clicked() {
					if err := openSiteApp(home); err != nil {
						a.toastErr(err)
					}
				}
			}
		})
	})
}
