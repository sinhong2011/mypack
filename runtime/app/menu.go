package app

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
	"mypack.dev/mypack/runtime/inject"
)

// acc returns an accelerator unless the user disabled that shortcut.
func (a *App) acc(s string) string {
	if a.disabled[inject.Normalize(s)] {
		return ""
	}
	return s
}

func (a *App) buildMenu() *mygo.Menu {
	click := func(fn func(win *mygo.Window, p *mygo.Page)) func(*mygo.MenuItem, *mygo.Window) {
		return func(_ *mygo.MenuItem, win *mygo.Window) {
			w, p := a.pageFor(win)
			if p == nil {
				return
			}
			fn(w, p)
		}
	}
	zoomIn := func(*mygo.MenuItem, *mygo.Window) { a.setZoom(+10, false) }

	settingsItem := func() *mygo.MenuItem {
		return &mygo.MenuItem{Label: "Settings…", Accelerator: a.acc("CmdOrCtrl+,"),
			Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }}
	}

	file := []*mygo.MenuItem{}
	if runtime.GOOS != "darwin" {
		// Accelerator lives on the app menu so Ctrl+, is registered once.
		file = append(file, &mygo.MenuItem{Label: "Settings…",
			Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }}, mygo.Separator())
	}
	if a.cfg.Window.MultiWindow {
		file = append(file, &mygo.MenuItem{Label: "New Window", Accelerator: a.acc("CmdOrCtrl+N"),
			Click: func(*mygo.MenuItem, *mygo.Window) { a.newWindow() }})
		file = append(file, mygo.Separator())
	}
	file = append(file, &mygo.MenuItem{Label: "Show Downloads", Accelerator: a.acc("CmdOrCtrl+Shift+J"),
		Click: func(*mygo.MenuItem, *mygo.Window) { go a.revealDownloads() }})
	file = append(file, mygo.Separator())
	if a.tabsOn() {
		file = append(file, a.tabMenuItems()...)
	} else {
		file = append(file, &mygo.MenuItem{Role: mygo.RoleClose})
	}
	if runtime.GOOS != "darwin" { // macOS has Quit in the app menu
		file = append(file, mygo.Separator(), &mygo.MenuItem{Role: mygo.RoleQuit})
	}

	edit := []*mygo.MenuItem{
		{Role: mygo.RoleUndo}, {Role: mygo.RoleRedo}, mygo.Separator(),
		{Role: mygo.RoleCut}, {Role: mygo.RoleCopy}, {Role: mygo.RolePaste},
		{Role: mygo.RolePasteAndMatchStyle}, {Role: mygo.RoleDelete}, {Role: mygo.RoleSelectAll},
	}
	if a.cfg.WebView.FindInPage {
		edit = append(edit, mygo.Separator(), &mygo.MenuItem{Label: "Find…", Accelerator: a.acc("CmdOrCtrl+F"),
			Click: func(_ *mygo.MenuItem, win *mygo.Window) { a.openFind(win) }})
	}

	view := []*mygo.MenuItem{
		{Label: "Reload", Accelerator: a.acc("CmdOrCtrl+R"), Click: click(func(_ *mygo.Window, p *mygo.Page) { p.Reload() })},
		{Label: "Hard Reload", Accelerator: a.acc("CmdOrCtrl+Shift+R"), Click: click(func(_ *mygo.Window, p *mygo.Page) { p.ReloadIgnoringCache() })},
		mygo.Separator(),
		{Label: "Back", Accelerator: a.acc("CmdOrCtrl+["), Click: click(func(_ *mygo.Window, p *mygo.Page) {
			if p.CanGoBack() {
				p.GoBack()
			}
		})},
		{Label: "Forward", Accelerator: a.acc("CmdOrCtrl+]"), Click: click(func(_ *mygo.Window, p *mygo.Page) {
			if p.CanGoForward() {
				p.GoForward()
			}
		})},
		{Label: "Home", Accelerator: a.acc("CmdOrCtrl+Shift+H"), Click: click(func(_ *mygo.Window, p *mygo.Page) { _ = p.LoadURL(a.homeOf(p)) })},
		mygo.Separator(),
		{Label: "Zoom In", Accelerator: a.acc("CmdOrCtrl+="), Click: zoomIn},
		{Label: "Zoom In", Accelerator: a.acc("CmdOrCtrl+Plus"), Hidden: true, Click: zoomIn},
		{Label: "Zoom Out", Accelerator: a.acc("CmdOrCtrl+-"), Click: func(*mygo.MenuItem, *mygo.Window) { a.setZoom(-10, false) }},
		{Label: "Actual Size", Accelerator: a.acc("CmdOrCtrl+0"), Click: func(*mygo.MenuItem, *mygo.Window) { a.setZoom(0, true) }},
		mygo.Separator(),
		{Role: mygo.RoleToggleFullScreen},
	}
	view = append(view, a.groupMenuItems()...)
	if a.tabsOn() {
		view = append(view, a.tabSwitchItems()...)
	}
	view = append(view, a.layoutMenuItems()...)
	if a.cfg.WebView.DevTools {
		// Not RoleToggleDevTools: in a window of native UI (strip layout) that
		// role opens MyGo's UI inspector, not the page's.
		acc := "Alt+Cmd+I"
		if runtime.GOOS != "darwin" {
			acc = "Ctrl+Shift+I"
		}
		view = append(view, mygo.Separator(), &mygo.MenuItem{Label: "Toggle Developer Tools", Accelerator: a.acc(acc),
			Click: click(func(_ *mygo.Window, p *mygo.Page) { p.ToggleDevTools() })})
	}

	// First menu is always the app name with Settings, not RoleAppMenu: that
	// role has no Settings, so Linux/Windows users looking at the leftmost
	// menu would miss it.
	appItems := []*mygo.MenuItem{
		{Role: mygo.RoleAbout},
		mygo.Separator(),
		settingsItem(),
		mygo.Separator(),
	}
	if runtime.GOOS == "darwin" {
		appItems = append(appItems,
			&mygo.MenuItem{Role: mygo.RoleServices},
			mygo.Separator(),
			&mygo.MenuItem{Role: mygo.RoleHide},
			&mygo.MenuItem{Role: mygo.RoleHideOthers},
			&mygo.MenuItem{Role: mygo.RoleUnhide},
			mygo.Separator(),
			&mygo.MenuItem{Role: mygo.RoleQuit},
		)
	} else {
		appItems = append(appItems, &mygo.MenuItem{Role: mygo.RoleQuit})
	}
	items := []*mygo.MenuItem{{Label: a.cfg.App.Name, Submenu: appItems}}
	items = append(items,
		&mygo.MenuItem{Label: "File", Submenu: file},
		&mygo.MenuItem{Label: "Edit", Submenu: edit},
		&mygo.MenuItem{Label: "View", Submenu: view},
		&mygo.MenuItem{Role: mygo.RoleWindowMenu},
	)
	return mygo.NewMenu(items)
}

// installAppMenu (re)builds the application menu and puts it on every
// window. Linux and Windows draw the bar per window; App.SetMenu alone is
// not enough for windows that already exist, or for a hidden title bar
// (those hide the bar — the strip/rail then has its own Settings control).
func (a *App) installAppMenu() {
	a.menu = a.buildMenu()
	mygo.App.SetMenu(a.menu)
	if runtime.GOOS == "darwin" {
		return
	}
	a.forEachAppWindow(func(w *mygo.Window) { w.SetMenu(a.menu) })
}

func (a *App) applyWindowMenu(w *mygo.Window) {
	if w == nil || a.menu == nil || runtime.GOOS == "darwin" {
		return
	}
	w.SetMenu(a.menu)
}

func (a *App) forEachAppWindow(fn func(*mygo.Window)) {
	a.mu.Lock()
	seen := map[*mygo.Window]bool{}
	var wins []*mygo.Window
	add := func(w *mygo.Window) {
		if w == nil || w == a.setWin || seen[w] {
			return
		}
		seen[w] = true
		wins = append(wins, w)
	}
	add(a.main)
	for w := range a.windows {
		add(w)
	}
	a.mu.Unlock()
	for _, w := range wins {
		if !w.IsDestroyed() {
			fn(w)
		}
	}
}

// createTray builds the tray icon; false when it could not be made.
func (a *App) createTray() bool {
	b, ok := a.trayIconPNG()
	if !ok {
		return false
	}
	menu := a.trayMenu()
	t, err := mygo.NewTray(mygo.TrayOptions{ID: a.cfg.App.Identifier, Icon: b, ToolTip: a.title(), Menu: menu})
	if err != nil {
		log.Printf("mypack-runtime: tray: %v", err)
		return false
	}
	a.tray = t
	return true
}

// trayIconPNG reads the tray icon (native.trayIcon, else the app icon).
func (a *App) trayIconPNG() ([]byte, bool) {
	icon := a.cfg.Native.TrayIcon
	if icon == "" {
		icon = "icon.png"
	}
	if !filepath.IsAbs(icon) {
		icon = filepath.Join(a.cfgDir, icon)
	}
	b, err := os.ReadFile(icon)
	if err != nil {
		log.Printf("mypack-runtime: tray icon: %v", err)
		return nil, false
	}
	return b, true
}

func (a *App) trayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Show / Hide", Click: func(*mygo.MenuItem, *mygo.Window) { a.toggleMain() }},
		{Label: "Settings…", Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }},
		{Label: "Reload", Click: func(*mygo.MenuItem, *mygo.Window) {
			if _, p := a.pageFor(nil); p != nil {
				p.Reload()
			}
		}},
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	})
}
