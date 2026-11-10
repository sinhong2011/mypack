package app

import (
	"context"
	"strings"

	"github.com/egoist/mygo"

	"mypack.dev/mypack/internal/desktop/components"
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

// syncTray creates, updates or removes the status item.
func (a *App) syncTray() {
	if !a.settings.ShowMenuBarIcon {
		if a.tray != nil {
			a.tray.Destroy()
			a.tray = nil
		}
		return
	}
	active := a.activeJobs()
	icon := components.TrayIconPNG(len(active) > 0)
	if a.tray == nil {
		t, err := mygo.NewTray(mygo.TrayOptions{ID: "mypack", Icon: icon, IconIsTemplate: true, ToolTip: "mypack"})
		if err != nil {
			return
		}
		a.tray = t
	} else {
		_ = a.tray.SetIcon(icon, true)
	}
	a.tray.SetMenu(a.trayMenu(active))
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
			a.lib.tab = 0
			a.nav(secLibrary)
			a.toast(T("import.done"), "", 1)
		})
	}()
}
