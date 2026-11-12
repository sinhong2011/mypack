package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
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
		{Label: "Mypack", Submenu: []*mygo.MenuItem{
			{Label: T("menu.about"), Click: on(func() { a.showWindow(); a.openSettings(paneAbout) })},
			{Label: T("menu.checkUpdates"), Click: on(a.checkForUpdatesMenu)},
			mygo.Separator(),
			{Label: T("menu.settings"), Accelerator: "CmdOrCtrl+,", Click: on(func() { a.showWindow(); a.openSettings("") })},
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
			{Label: T("menu.newGroup"), Accelerator: "CmdOrCtrl+Shift+N", Click: on(a.newGroup)},
			{Label: T("menu.build"), Accelerator: "CmdOrCtrl+B", Click: on(a.buildShortcut)},
			mygo.Separator(),
			{Label: T("menu.importApp"), Click: on(a.importExistingApp)},
			{Label: T("menu.importConfig"), Click: on(a.importConfig)},
			{Label: T("menu.importApps"), Click: on(a.importApps)},
			{Label: T("menu.exportLibrary"), Click: on(func() { a.exportLibrary(nil, T("library.export.name")) })},
			{Label: T("create.open"), Accelerator: "CmdOrCtrl+O", Click: on(a.openSource)},
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
			{Label: T("menu.palette"), Accelerator: "CmdOrCtrl+K", Click: on(a.togglePalette)},
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
	// A fresh Create, unless Create is already in front with something in
	// hand (an address being typed, a build still running); a finished
	// build there gives way to a new app.
	if a.section != secCreate || (a.create.building != "" && !a.buildingNow(a.create.building)) {
		a.create.reset(a.host)
	}
	a.nav(secCreate)
}

// buildingNow reports whether a project has a build running or waiting.
func (a *App) buildingNow(projectID string) bool {
	for _, j := range a.activeJobs() {
		if j.ProjectID == projectID {
			return true
		}
	}
	return false
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
	// While builds run the glyph sits in a progress ring: the most recent
	// build's %, or a turning arc when it has none.
	icon := kit.TrayProgressPNG(-2, 0)
	if len(active) > 0 {
		pr := active[0].Progress
		if active[0].Status != domain.JobRunning {
			pr = -1
		}
		if pr < 0 {
			pr = -1
			a.trayPhase += 0.08
			if !a.traySpin {
				a.traySpin = true
				time.AfterFunc(90*time.Millisecond, func() {
					if a.win != nil {
						a.win.Update(func() { a.traySpin = false; a.syncTray() })
					}
				})
			}
		}
		icon = kit.TrayProgressPNG(pr, a.trayPhase)
	}
	if a.tray == nil {
		t, err := mygo.NewTray(mygo.TrayOptions{ID: "mypack", Icon: icon, IconIsTemplate: true, ToolTip: "Mypack"})
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
		Width: flyoutWidth, Height: 360, FitContent: true, MinWidth: flyoutWidth, MaxWidth: flyoutWidth,
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

// flyoutView is the menu bar popover, laid out as a native menu: a title
// line with the status, the address field, builds in progress as rows with
// a bar, recent apps as rows, and a footer. Quit sits in the footer's menu.
func (a *App) flyoutView(c *ui.Context) {
	design.Apply(c)
	p := design.Of(c)
	c.Root().Background(ui.Transparent)
	box := ui.Column(c).Width(flyoutWidth).Padding(12, 6, 6, 6).Background(p.Overlay)
	if !c.Vibrancy() {
		box.Radius(14).Border(0.5, p.BorderStrong)
	}
	active := a.activeJobs()
	box.Children(func() {
		// Title line: the app's name, its status, Settings.
		ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(0, 4, 8, 10).Children(func() {
			kit.BrandMark(c, 18)
			kit.Headline(c, "Mypack").Grow(1).MinWidth(0)
			if len(active) > 0 {
				kit.StatusDot(c, T("tray.building", "count", len(active)), kit.ToneAccent)
			} else {
				kit.StatusDot(c, T("tray.ready"), kit.ToneSuccess)
			}
			if kit.IconButton(c.Key("settings"), icons.Settings, T("menu.settings")).Clicked() {
				a.closeFlyoutAnd(func() { a.openSettings("") })
			}
		})
		ui.Column(c).Padding(0, 6, 4, 6).Children(func() {
			r := kit.URLField(c.Key("url"), &a.flyoutURL, kit.URLFieldOpts{
				Placeholder: T("flyout.placeholder"), Label: T("create.url.label"), Height: 36, AutoFocus: true, Compact: true,
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
		})
		if len(active) > 0 {
			flyoutSection(c, T("tray.activity"))
			for _, j := range active {
				j := j
				var png []byte
				if pr := a.project(j.ProjectID); pr != nil {
					png = pr.IconPNG
				}
				// A bar only while it runs: a queued build, or one that
				// waits for approval, has its words and nothing to fill.
				f := -2.0
				if j.Status == domain.JobRunning {
					var definite bool
					if f, definite = overallProgress(j, jobBuildsProject(j)); !definite {
						f = -1
					}
				}
				if flyoutRow(c.Key(j.ID), png, j.Snapshot.App.Name, stageLabel(j), f) {
					a.closeFlyoutAnd(func() { a.openProject(j.ProjectID, 0) })
				}
			}
		}
		if recent := a.flyoutRecent(active); len(recent) > 0 {
			flyoutSection(c, T("tray.recent"))
			for _, pr := range recent {
				id := pr.ID
				if flyoutRow(c.Key(id), pr.IconPNG, pr.Config.App.Name, projectHost(pr), -2) {
					a.closeFlyoutAnd(func() { a.openProject(id, 0) })
				}
			}
		}
		ui.Box(c).Height(0.5).Background(p.Divider).Margin(6, 10).Shrink(0)
		ui.Row(c).Gap(2).AlignItems(ui.Center).Padding(0, 4).Children(func() {
			if kit.Button(c.Key("open"), T("tray.open"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM + 4, Icon: icons.Library}).Clicked() {
				a.closeFlyoutAnd(func() { a.nav(secLibrary) })
			}
			ui.Spacer(c)
			if kit.IconButton(c.Key("builds"), icons.Builds, T("tray.viewBuilds")).Clicked() {
				a.closeFlyoutAnd(func() { a.nav(secBuilds) })
			}
			// The footer menu: the notch HUD toggle (the same as Settings ›
			// Builds), and Quit, where a menu bar app keeps it.
			kit.IconButton(c.Key("more"), icons.More, T("common.more")).Menu(func(m *ui.Menu) {
				on := a.ui.ShowHUD == nil || *a.ui.ShowHUD
				if m.Item(T("hud.setting")).Checked(on).Chosen() {
					v := !on
					a.ui.ShowHUD = &v
					a.ui.save()
					a.syncHUD()
				}
				m.Separator()
				if m.Item(T("menu.quit")).Shortcut(ui.Cmd, ui.KeyQ).Chosen() {
					mygo.App.Quit()
				}
			})
		})
	})
}

const flyoutWidth = 320

// flyoutRecent is the flyout's recent apps: the five most recent projects,
// one per name (a rebuilt copy of an app is not a second app here), less
// those Activity already lists.
func (a *App) flyoutRecent(active []domain.BuildJob) []domain.Project {
	var out []domain.Project
	seen := map[string]bool{}
	busy := map[string]bool{}
	for _, j := range active {
		busy[j.ProjectID] = true
	}
	for _, pr := range a.projects {
		if busy[pr.ID] {
			continue
		}
		k := strings.ToLower(pr.Config.App.Name)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, pr)
		if len(out) == 5 {
			break
		}
	}
	return out
}

// projectHost is the site a project wraps, or its folder's name.
func projectHost(pr domain.Project) string {
	src := pr.Config.App.Source
	if src.Type == config.SourceURL {
		return strings.TrimPrefix(hostOf(src.Value), "www.")
	}
	return filepath.Base(src.Value)
}

// flyoutSection is a section's small heading, aligned with the rows' text.
func flyoutSection(c *ui.Context, s string) {
	kit.Eyebrow(c, s).Padding(10, 10, 4, 10)
}

// flyoutRow is one row of the flyout, as a menu item: the icon, the name,
// a quiet detail on the right, and under the name a bar when progress is
// at least -1 (-1 is indeterminate; -2 shows none). It reports a click.
func flyoutRow(c *ui.Context, png []byte, name, detail string, progress float64) bool {
	p := design.Of(c)
	row := ui.ButtonBase(c).Label(name).Row().Justify(ui.Start).Gap(10).Padding(5, 10).Radius(8).AlignItems(ui.Center).
		FocusRing(false).Transition(design.Transition(c))
	hov := row.Hovered() || row.FocusVisible()
	if hov {
		row.Background(p.Hover)
	}
	row.Children(func() {
		kit.AppIconOpts(c, png, name, 26, kit.IconOpts{})
		ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine().Grow(1).MinWidth(0)
				ui.Text(c, detail).FontSize(design.FontSmall).TextColor(p.TextTertiary).SingleLine().TruncateMiddle().MaxWidth(150)
			})
			if progress >= -1 {
				kit.ProgressBar(c, progress).Height(4)
			}
		})
		col := p.TextTertiary
		if !hov {
			col = ui.Transparent
		}
		kit.Glyph(c, icons.ChevronRt, 13, col)
	})
	return row.Clicked()
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
		{Label: "Mypack — " + status, Disabled: true},
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
	// Pinned apps open straight from the menu bar.
	if pins := a.pinnedItems(); len(pins) > 0 {
		items = append(items, mygo.Separator(), &mygo.MenuItem{Label: T("nav.pinned"), Disabled: true})
		for _, it := range pins {
			it := it
			if it.install != nil && it.install.State != domain.InstallMissing {
				id := it.install.ID
				items = append(items, &mygo.MenuItem{Label: it.name(), Click: func(*mygo.MenuItem, *mygo.Window) { a.openInstall(id) }})
			} else if it.project != nil {
				id := it.project.ID
				items = append(items, &mygo.MenuItem{Label: it.name(), Click: on(func() { a.openProject(id, 0) })})
			}
		}
	}
	if a.launcher.registered != "" {
		items = append(items, &mygo.MenuItem{Label: T("tray.launcher", "keys", launcherKeys()), Click: func(*mygo.MenuItem, *mygo.Window) { a.toggleLauncher() }})
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
		&mygo.MenuItem{Label: T("menu.settings"), Click: on(func() { a.openSettings("") })},
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

// importConfig opens an app config (a Mypack config or Pake's pake.json)
// in Create, to review before building.
func (a *App) importConfig() {
	a.showWindow()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: T("importConfig.title"),
			ButtonLabel: T("importConfig.button"), Filters: []mygo.FileFilter{{Name: T("importConfig.filter"), Extensions: []string{"json", "mypack"}}}})
		if err != nil || len(paths) == 0 {
			return
		}
		if appservice.IsLibraryFile(paths[0]) {
			a.win.Update(func() { a.importLibraryFile(paths[0], false) })
			return
		}
		a.win.Update(func() {
			a.create.reset(a.host)
			a.create.setInput(paths[0])
			a.nav(secCreate)
		})
	}()
}
