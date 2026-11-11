package app

import (
	"context"
	"os"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
)

// installMenu (re)builds the application menu in the current language.
func (a *App) installMenu() {
	on := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			fn()
			if a.win != nil {
				a.win.Invalidate()
			}
		}
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "mypack", Submenu: []*mygo.MenuItem{
			{Label: T("menu.about"), Click: on(func() { a.nav(secSettings); a.set.scrollAbout = true })},
			mygo.Separator(),
			{Label: T("menu.settings"), Accelerator: "CmdOrCtrl+,", Click: on(func() { a.showWindow(); a.nav(secSettings) })},
			mygo.Separator(),
			{Role: mygo.RoleServices},
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Label: T("menu.quit"), Accelerator: "CmdOrCtrl+Q", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
		}},
		{Label: T("menu.file"), Submenu: []*mygo.MenuItem{
			{Label: T("menu.newApp"), Accelerator: "CmdOrCtrl+N", Click: on(a.newApp)},
			{Label: T("menu.build"), Accelerator: "CmdOrCtrl+B", Click: on(a.buildShortcut)},
			mygo.Separator(),
			{Label: T("menu.importApp"), Click: on(a.importExistingApp)},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Label: T("menu.view"), Submenu: []*mygo.MenuItem{
			{Label: T("nav.library"), Accelerator: "CmdOrCtrl+1", Click: on(func() { a.nav(secLibrary) })},
			{Label: T("nav.discover"), Accelerator: "CmdOrCtrl+2", Click: on(func() { a.nav(secDiscover) })},
			{Label: T("nav.builds"), Accelerator: "CmdOrCtrl+3", Click: on(func() { a.nav(secBuilds) })},
			{Label: T("nav.runtimes"), Accelerator: "CmdOrCtrl+4", Click: on(func() { a.nav(secRuntimes) })},
			mygo.Separator(),
			{Label: T("menu.find"), Accelerator: "CmdOrCtrl+F", Click: on(func() { a.nav(secLibrary); a.lib.focusSearch = true })},
			mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen},
		}},
		{Role: mygo.RoleWindowMenu},
	})
	mygo.App.SetMenu(menu)
}

// newApp opens Create (⌘N).
func (a *App) newApp() {
	a.showWindow()
	if a.section != secCreate {
		a.create.reset(a.host)
	}
	a.nav(secCreate)
}

// buildShortcut builds what's in front (⌘B).
func (a *App) buildShortcut() {
	switch a.section {
	case secCreate:
		a.create.buildRequested = true
	case secProject:
		if p := a.project(a.projectID); p != nil {
			a.startBuild(p.ID, nil)
		}
	}
}

// ---- menu bar (spec §09B) -------------------------------------------------------

// syncTray creates, updates or removes the status item. A click opens the
// glass flyout; a right click shows the classic menu (Linux trays tell no
// clicks, so they keep the menu).
func (a *App) syncTray() {
	if !a.settings.ShowMenuBarIcon {
		if a.flyout != nil {
			a.flyout.Close()
		}
		if a.tray != nil {
			a.tray.Destroy()
			a.tray = nil
		}
		return
	}
	active := a.activeJobs()
	icon := kit.TrayIconPNG(len(active) > 0)
	if a.tray == nil {
		t, err := mygo.NewTray(mygo.TrayOptions{ID: "mypack", Icon: icon, IconIsTemplate: true, ToolTip: "mypack"})
		if err != nil {
			return
		}
		a.tray = t
		if runtime.GOOS != "linux" {
			t.OnClick(a.toggleFlyout)
			t.OnRightClick(func() {
				if a.flyout != nil {
					a.flyout.Close()
				}
				t.SetMenu(a.trayMenu(a.activeJobs()))
				defer t.SetMenu(nil)
				t.PopUpMenu()
			})
		}
	} else {
		_ = a.tray.SetIcon(icon, true)
	}
	if runtime.GOOS == "linux" {
		a.tray.SetMenu(a.trayMenu(active))
	}
	if a.flyout != nil {
		a.flyout.Invalidate()
	}
}

// toggleFlyout opens the menu bar flyout under the status item, or closes it.
func (a *App) toggleFlyout() {
	if a.flyout != nil {
		a.flyout.Close()
		return
	}
	opts := mygo.FlyoutOptions{Tray: a.tray, Placement: mygo.PlacementBottom}
	if a.tray == nil || (isFake() && os.Getenv("MYPACK_FAKE_FLYOUT") == "1") {
		// No status item (or the demo's screenshots): open it under the
		// window's top-right corner instead.
		if a.win == nil {
			return
		}
		w, _ := a.win.Size()
		opts = mygo.FlyoutOptions{Parent: a.win, Anchor: mygo.Rectangle{X: w - 60, Y: 40, Width: 20, Height: 10}, Placement: mygo.PlacementBottomEnd}
	}
	a.flyout = mygo.NewFlyout(mygo.FlyoutOptions{
		Tray: opts.Tray, Parent: opts.Parent, Anchor: opts.Anchor, Placement: opts.Placement, Gap: 4,
		Width: 340, Height: 360, FitContent: true, MinWidth: 340, MaxWidth: 340,
		Focusable: true, Shadow: true, Popover: true,
		Content: ui.View(a.flyoutView),
	})
	a.flyout.OnClosed(func() { a.flyout = nil })
}

// closeFlyoutAnd closes the flyout, brings the main window up and runs fn.
func (a *App) closeFlyoutAnd(fn func()) {
	if a.flyout != nil {
		a.flyout.Close()
	}
	a.showWindow()
	fn()
	if a.win != nil {
		a.win.Invalidate()
	}
}

// flyoutView is the menu bar popover: paste a URL, jump to a recent app,
// watch builds.
func (a *App) flyoutView(c *ui.Context) {
	design.Apply(c)
	p := design.Of(c)
	c.Root().Background(ui.Transparent)
	box := ui.Column(c).Width(340).Padding(16).Gap(14).Background(p.Overlay)
	if !c.Vibrancy() {
		box.Radius(14).Border(0.5, p.BorderStrong)
	}
	active := a.activeJobs()
	box.Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			kit.BrandMark(c, 30)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				kit.Headline(c, "mypack")
				status := T("tray.ready")
				if len(active) > 0 {
					status = T("tray.building", "count", len(active))
				}
				kit.Caption(c, status)
			})
			if kit.IconButton(c.Key("settings"), icons.Settings, T("menu.settings")).Clicked() {
				a.closeFlyoutAnd(func() { a.nav(secSettings) })
			}
		})
		r := kit.URLField(c.Key("url"), &a.flyoutURL, kit.URLFieldOpts{
			Placeholder: T("flyout.placeholder"), Label: T("create.url.label"), Height: 40, AutoFocus: true,
			Paste: T("create.paste"), Action: T("flyout.make"), ActionOff: strings.TrimSpace(a.flyoutURL) == "",
		})
		if r.Paste {
			a.flyoutURL = strings.TrimSpace(c.ReadClipboard())
		}
		if (r.Action || r.Submitted) && strings.TrimSpace(a.flyoutURL) != "" {
			u := a.flyoutURL
			a.flyoutURL = ""
			a.closeFlyoutAnd(func() {
				a.create.reset(a.host)
				a.create.setInput(u)
				a.nav(secCreate)
			})
		}
		if len(active) > 0 {
			ui.Column(c).Gap(8).Children(func() {
				flyoutHeader(c, T("tray.activity"))
				if pr := a.project(active[0].ProjectID); pr != nil {
					j := active[0]
					ui.Box(c).Radius(design.RadiusCard).Clip().Children(func() { a.sceneFor(c, pr, &j, 132) })
				}
				for _, j := range active {
					j := j
					var png []byte
					if pr := a.project(j.ProjectID); pr != nil {
						png = pr.IconPNG
					}
					row := ui.ButtonBase(c.Key(j.ID)).Label(j.Snapshot.App.Name).Row().Justify(ui.Start).Gap(10).Padding(6, 8).Radius(10).AlignItems(ui.Center).Cursor(ui.CursorPointer)
					if row.Hovered() {
						row.Background(p.Hover)
					}
					row.Children(func() {
						kit.AppIconOpts(c, png, j.Snapshot.App.Name, 30, kit.IconOpts{})
						ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
							ui.Row(c).Children(func() {
								ui.Text(c, j.Snapshot.App.Name).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine().Grow(1).MinWidth(0)
								ui.Text(c, stageLabel(j)).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine()
							})
							prog := -1.0
							if j.Progress >= 0 {
								prog = (float64(stepIndex(j.Stage)) + j.Progress) / float64(len(visibleSteps))
							}
							kit.ProgressBar(c, prog)
						})
					})
					if row.Clicked() {
						a.closeFlyoutAnd(func() { a.openProject(j.ProjectID, 0) })
					}
				}
			})
		}
		if len(a.projects) > 0 {
			ui.Column(c).Gap(8).Children(func() {
				flyoutHeader(c, T("tray.recent"))
				ui.Row(c).Gap(4).Children(func() {
					for i, pr := range a.projects {
						if i >= 5 {
							break
						}
						id, name := pr.ID, pr.Config.App.Name
						b := ui.ButtonBase(c.Key(id)).Label(name).Column().Width(58).AlignItems(ui.Center).Gap(5).Padding(6, 0).Radius(10).Cursor(ui.CursorPointer)
						if b.Hovered() {
							b.Background(p.Hover)
						}
						b.Children(func() {
							kit.AppIconOpts(c, pr.IconPNG, name, 40, kit.IconOpts{Shadow: true})
							ui.Text(c, name).FontSize(10).TextColor(p.TextSecondary).SingleLine().MaxWidth(56).TextAlign(ui.Center)
						})
						if b.Clicked() {
							a.closeFlyoutAnd(func() { a.openProject(id, 0) })
						}
					}
				})
			})
		}
		kit.Hairline(c)
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			if kit.Button(c.Key("open"), T("tray.open"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM, Icon: icons.Library}).Clicked() {
				a.closeFlyoutAnd(func() { a.nav(secLibrary) })
			}
			if kit.Button(c.Key("builds"), T("tray.viewBuilds"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM, Icon: icons.Builds}).Clicked() {
				a.closeFlyoutAnd(func() { a.nav(secBuilds) })
			}
			ui.Spacer(c)
			if kit.IconButton(c.Key("quit"), icons.Power, T("menu.quit")).Clicked() {
				mygo.App.Quit()
			}
		})
	})
}

func flyoutHeader(c *ui.Context, s string) {
	kit.Eyebrow(c, s).Padding(0, 2)
}

func (a *App) trayMenu(active []domain.BuildJob) *mygo.Menu {
	on := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			a.showWindow()
			fn()
			if a.win != nil {
				a.win.Invalidate()
			}
		}
	}
	status := T("tray.ready")
	if len(active) > 0 {
		status = T("tray.building", "count", len(active))
	}
	items := []*mygo.MenuItem{
		{Label: "mypack — " + status, Disabled: true},
		mygo.Separator(),
		{Label: T("tray.pasteURL"), Click: on(a.pasteToBuild)},
		{Label: T("menu.newApp"), Click: on(a.newApp)},
		{Label: T("tray.openLibrary"), Click: on(func() { a.nav(secLibrary) })},
		{Label: T("tray.viewBuilds"), Click: on(func() { a.nav(secBuilds) })},
	}
	if len(active) > 0 {
		items = append(items, mygo.Separator(), &mygo.MenuItem{Label: T("tray.activity"), Disabled: true})
		for _, j := range active {
			j := j
			label := j.Snapshot.App.Name + " — " + stageLabel(j)
			items = append(items, &mygo.MenuItem{Label: label, Click: on(func() { a.openProject(j.ProjectID, 0) })})
		}
	}
	if len(a.projects) > 0 {
		items = append(items, mygo.Separator(), &mygo.MenuItem{Label: T("tray.recent"), Disabled: true})
		for i, p := range a.projects {
			if i >= 3 {
				break
			}
			id := p.ID
			items = append(items, &mygo.MenuItem{Label: p.Config.App.Name, Click: on(func() { a.openProject(id, 0) })})
		}
	}
	items = append(items, mygo.Separator(),
		&mygo.MenuItem{Label: T("tray.open"), Click: on(func() {})},
		&mygo.MenuItem{Label: T("menu.settings"), Click: on(func() { a.nav(secSettings) })},
		&mygo.MenuItem{Label: T("menu.quit"), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	)
	return mygo.NewMenu(items)
}

// pasteToBuild reads the clipboard only to check for an http(s) URL; any
// other content is ignored and never sent anywhere (spec §09B).
func (a *App) pasteToBuild() {
	s := strings.TrimSpace(mygo.Clipboard.ReadText())
	a.create.reset(a.host)
	low := strings.ToLower(s)
	if (strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://")) && !strings.ContainsAny(s, " \n\t") {
		a.create.setInput(s)
	}
	a.nav(secCreate)
}

// importExistingApp asks for an .app/.exe and registers it read-only.
func (a *App) importExistingApp() {
	a.showWindow()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: T("import.title"),
			ButtonLabel: T("import.button"), Message: T("import.message"),
			Filters: []mygo.FileFilter{{Name: T("import.filter"), Extensions: []string{"app", "exe", "AppImage"}}}})
		if err != nil || len(paths) == 0 {
			return
		}
		a.do(func(ctx context.Context) error {
			_, err := a.svc.ImportExistingApp(ctx, paths[0], "gui")
			return err
		}, func(err error) {
			if err != nil {
				a.toastErr(err)
				return
			}
			a.nav(secLibrary)
			a.toast(T("import.done"), "", 1)
		})
	}()
}
