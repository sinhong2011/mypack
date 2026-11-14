package app

import (
	"log"
	"runtime"

	"github.com/egoist/mygo"
	"mypack.dev/mypack/pkg/config"
)

// Window modes (window.mode, runtime 1.0.6):
//
//   - "menubar": the app lives in the menu bar (tray on Windows/Linux). A
//     click on its icon opens the page in a panel below the icon (a popover
//     on macOS); a click elsewhere, Escape or another click on the icon
//     hides it. The panel is hidden, never closed, so the page keeps its
//     state (a chat stays signed in and keeps notifying). No Dock icon
//     (LSUIElement, written by the Builder); the unread count shows next
//     to the menu-bar icon instead.
//   - "compact" and "sidebar": layouts of the main window (layout.go).

func (a *App) menuBarMode() bool { return a.cfg.Window.EffectiveMode() == config.ModeMenuBar }

// startMenuBar creates the menu-bar icon and its (hidden) panel. It reports
// false when no icon could be made: the caller then shows a normal window,
// so the user is never left with no way into the app.
func (a *App) startMenuBar(startURL string) bool {
	if !a.createMenuBarTray() {
		return false
	}
	pw, ph := a.cfg.Window.PanelSize()
	fo := mygo.FlyoutOptions{
		Tray:      a.tray,
		Placement: mygo.PlacementBottom,
		Gap:       4,
		Width:     pw,
		Height:    ph,
		Focusable: true,
		Shadow:    true,
		Popover:   true,
		Hidden:    true,
	}
	if a.group != nil {
		fo.Content = a.group.content()
	} else {
		fo.URL, fo.Page = startURL, a.pageOptions()
	}
	panel := mygo.NewFlyout(fo)
	var p *mygo.Page
	if a.group != nil {
		a.mu.Lock()
		a.windows[panel] = nil
		a.mu.Unlock()
		p = a.group.attach(panel)
		a.mu.Lock()
		a.windows[panel] = p
		a.mu.Unlock()
	} else if p = panel.Page(); p != nil {
		a.registerPage(panel, p)
	}
	a.mu.Lock()
	a.main = panel
	a.shown = true
	a.mu.Unlock()
	// The Builder writes LSUIElement, so there is no Dock icon from launch
	// on; this covers a bundle without it (a development run).
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	a.attachMainPage(panel, p)
	// Clicks elsewhere, Escape and a click on the icon close a focusable
	// popover: hide it instead, so the page lives on. Hide after AppKit has
	// finished asking (refusing alone would keep the popover open).
	panel.OnClose(func(e *mygo.CloseEvent) {
		a.mu.Lock()
		quitting := a.quitting
		a.mu.Unlock()
		if quitting {
			return
		}
		e.PreventDefault()
		go panel.Hide()
	})
	return true
}

// createMenuBarTray makes the icon of a menu-bar app: a click toggles the
// panel, a right click shows the app's menu. Linux's tray reports no
// clicks, so there the menu (with "Open") is the icon's only action.
func (a *App) createMenuBarTray() bool {
	icon, ok := a.trayIconPNG()
	if !ok {
		return false
	}
	t, err := mygo.NewTray(mygo.TrayOptions{ID: a.cfg.App.Identifier, Icon: icon, ToolTip: a.title()})
	if err != nil {
		log.Printf("mypack-runtime: tray: %v", err)
		return false
	}
	a.tray = t
	menu := a.menuBarMenu()
	if runtime.GOOS == "linux" {
		t.SetMenu(menu)
		return true
	}
	t.OnClick(a.togglePanel)
	t.OnRightClick(func() {
		a.hidePanel()
		// The menu for this popup only, so that clicks keep toggling.
		t.SetMenu(menu)
		defer t.SetMenu(nil)
		t.PopUpMenu()
	})
	return true
}

func (a *App) menuBarMenu() *mygo.Menu {
	page := func(fn func(p *mygo.Page)) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			if _, p := a.pageFor(nil); p != nil {
				fn(p)
			}
		}
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Open " + a.title(), Click: func(*mygo.MenuItem, *mygo.Window) { a.showMain() }},
		{Label: "Open in a Window", Click: func(*mygo.MenuItem, *mygo.Window) { a.openPanelInWindow() }},
		mygo.Separator(),
		{Label: "Reload", Click: page(func(p *mygo.Page) { p.Reload() })},
		{Label: "Home", Click: page(func(p *mygo.Page) { _ = p.LoadURL(a.homeOf(p)) })},
		mygo.Separator(),
		{Label: "Quit " + a.title(), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}

// panel returns the menu-bar panel, or nil.
func (a *App) panel() *mygo.Window {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.main == nil || a.main.IsDestroyed() {
		return nil
	}
	return a.main
}

func (a *App) togglePanel() {
	w := a.panel()
	if w == nil {
		return
	}
	if w.IsVisible() {
		w.Hide()
		return
	}
	w.Show()
	w.Focus()
}

func (a *App) hidePanel() {
	if w := a.panel(); w != nil && w.IsVisible() {
		w.Hide()
	}
}

// openPanelInWindow opens what the panel shows in a regular window, for
// the times a panel is too small.
func (a *App) openPanelInWindow() {
	u := a.home
	if _, p := a.pageFor(nil); p != nil {
		if cur := p.URL(); cur != "" && a.policy.IsAppURL(cur) {
			u = cur
		}
	}
	a.hidePanel()
	w, _ := a.openWindow(false, u, "", false)
	if w != nil {
		w.Focus()
	}
}
