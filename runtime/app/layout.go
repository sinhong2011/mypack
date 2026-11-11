package app

import (
	"sync"
	"time"

	"github.com/egoist/mygo"
	"mypack.dev/mypack/pkg/config"
)

// Layouts of the main window (window.mode "window", "compact", "sidebar";
// runtime 1.0.6). View > Window / Compact Window / Sidebar switches between
// them while the app runs; the choice is remembered in state.json.
//
//   - window: the normal window, where the user left it.
//   - compact: a small window (panel size) above other windows, on every
//     Space, in the bottom-right corner or where the user left it.
//   - sidebar: docked to the left or right edge of the screen at the full
//     height of its work area, above other windows, on every Space. Its
//     width follows the user (drag its inner edge) and is remembered; a
//     move snaps it back to its edge. With window.sidebarAutoHide it hides
//     as another app takes the focus; the global shortcut or the Dock icon
//     slides it back. Closing it hides it.

const (
	layoutWindow  = config.ModeWindow
	layoutCompact = config.ModeCompact
	layoutSidebar = config.ModeSidebar
)

// Menu item IDs and accelerators of the layouts.
const (
	layoutMenuPrefix = "layout-"
	sidebarEdgeID    = "sidebar-edge"
	compactAccel     = "CmdOrCtrl+Alt+P"
	sidebarAccel     = "CmdOrCtrl+Alt+S"
	windowAccel      = "CmdOrCtrl+Alt+W"
)

const (
	compactMargin   = 16  // gap between a new compact window and the work area's edges
	minPanelSide    = 240 // smallest compact window / sidebar width
	minPageBesides  = 200 // a sidebar leaves at least this much of the screen
	sidebarSettle   = 250 * time.Millisecond
	fullScreenLeave = 900 * time.Millisecond
)

// layoutState is the main window's layout; guarded by App.mu.
type layoutState struct {
	cur   string // current layout of the main window
	dock  *time.Timer
	dockM sync.Mutex
}

// startLayout is the layout the main window starts in: the user's last
// pick under this window.mode, else window.mode.
func (a *App) startLayout() string {
	mode := a.cfg.Window.EffectiveMode()
	if mode == config.ModeMenuBar {
		return layoutWindow
	}
	if st := a.state.Get(); st.Layout != "" && st.LayoutMode == mode && validLayout(st.Layout) {
		return st.Layout
	}
	return mode
}

func validLayout(l string) bool {
	return l == layoutWindow || l == layoutCompact || l == layoutSidebar
}

// layout returns the main window's current layout.
func (a *App) layout() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lay.cur == "" {
		return layoutWindow
	}
	return a.lay.cur
}

// isCompact reports whether the main window is compact now.
func (a *App) isCompact() bool { return a.layout() == layoutCompact }

// pickLayout is the View menu: switch the main window and remember it.
func (a *App) pickLayout(to string) {
	a.mu.Lock()
	w := a.main
	a.mu.Unlock()
	if w == nil || w.IsDestroyed() {
		return
	}
	mode := a.cfg.Window.EffectiveMode()
	a.state.Update(func(s *State) { s.Layout, s.LayoutMode = to, mode })
	a.setLayout(w, to)
	w.Show()
	w.Focus()
}

// toggleCompact switches between compact and normal (the shortcut).
func (a *App) toggleCompact() {
	if a.isCompact() {
		a.pickLayout(layoutWindow)
	} else {
		a.pickLayout(layoutCompact)
	}
}

// setLayout moves w from its current layout to "to". Main thread.
func (a *App) setLayout(w *mygo.Window, to string) {
	if w.IsFullScreen() {
		// Leaving full screen animates; lay out once it has.
		w.SetFullScreen(false)
		time.AfterFunc(fullScreenLeave, func() { mygo.RunOnMain(func() { a.setLayout(w, to) }) })
		return
	}
	from := a.layout()
	if from == to {
		a.syncLayoutMenu()
		return
	}
	// Remember the layout being left.
	switch from {
	case layoutCompact:
		a.saveCompactBounds(w)
	case layoutWindow:
		if !w.IsMaximized() {
			nb := w.Bounds()
			a.state.Update(func(s *State) { s.NormalBounds = toBounds(nb) })
		}
	}
	if w.IsMaximized() {
		w.Unmaximize()
	}
	a.mu.Lock()
	a.lay.cur = to
	a.mu.Unlock()

	cw := a.cfg.Window
	switch to {
	case layoutWindow:
		w.SetAlwaysOnTop(cw.AlwaysOnTop)
		w.SetVisibleOnAllWorkspaces(false)
		w.SetMinimumSize(cw.MinWidth, cw.MinHeight)
		w.SetMaximumSize(0, 0)
		if nb := a.state.Get().NormalBounds; nb != nil && nb.W > 0 && nb.H > 0 {
			w.SetBounds(mygo.Rectangle{X: nb.X, Y: nb.Y, Width: nb.W, Height: nb.H})
		} else {
			w.SetSize(cw.Width, cw.Height)
			w.Center()
		}
	case layoutCompact:
		pw, ph := cw.PanelSize()
		work := mygo.Screen.DisplayMatching(w.Bounds()).WorkArea
		r := compactRect(a.state.Get().CompactBounds, fromRect(work), pw, ph)
		w.SetMaximumSize(0, 0)
		w.SetMinimumSize(minPanelSide, minPanelSide)
		w.SetBounds(mygo.Rectangle{X: r.X, Y: r.Y, Width: r.W, Height: r.H})
		w.SetAlwaysOnTop(true)
		w.SetVisibleOnAllWorkspaces(true)
	case layoutSidebar:
		w.SetMinimumSize(minPanelSide, minPanelSide)
		w.SetAlwaysOnTop(true)
		w.SetVisibleOnAllWorkspaces(true)
		a.dockSidebar(w, a.sidebarWidth())
	}
	a.syncLayoutMenu()
}

// ---- sidebar ----

func (a *App) sidebarEdge() string {
	if e := a.state.Get().SidebarEdge; e == config.EdgeLeft || e == config.EdgeRight {
		return e
	}
	return a.cfg.Window.EffectiveSidebarEdge()
}

func (a *App) sidebarWidth() int {
	if sw := a.state.Get().SidebarWidth; sw >= minPanelSide {
		return sw
	}
	pw, _ := a.cfg.Window.PanelSize()
	return pw
}

// dockSidebar puts w at its edge of the display it is on, at width.
func (a *App) dockSidebar(w *mygo.Window, width int) {
	work := fromRect(mygo.Screen.DisplayMatching(w.Bounds()).WorkArea)
	r := sidebarRect(work, a.sidebarEdge(), width)
	if b := w.Bounds(); b.X == r.X && b.Y == r.Y && b.Width == r.W && b.Height == r.H {
		return
	}
	w.SetBounds(mygo.Rectangle{X: r.X, Y: r.Y, Width: r.W, Height: r.H})
}

// redockSoon re-docks the sidebar once the user has stopped resizing or
// moving it: a new width is kept (and remembered), a move snaps back.
func (a *App) redockSoon(w *mygo.Window) {
	if a.layout() != layoutSidebar {
		return
	}
	a.lay.dockM.Lock()
	defer a.lay.dockM.Unlock()
	if a.lay.dock != nil {
		a.lay.dock.Stop()
	}
	a.lay.dock = time.AfterFunc(sidebarSettle, func() {
		mygo.RunOnMain(func() {
			if w.IsDestroyed() || a.layout() != layoutSidebar || w.IsFullScreen() {
				return
			}
			width := w.Bounds().Width
			if width >= minPanelSide && width != a.state.Get().SidebarWidth {
				a.state.Update(func(s *State) { s.SidebarWidth = width })
			}
			a.dockSidebar(w, width)
		})
	})
}

// flipSidebarEdge moves the sidebar to the other edge (View menu).
func (a *App) flipSidebarEdge() {
	e := config.EdgeLeft
	if a.sidebarEdge() == config.EdgeLeft {
		e = config.EdgeRight
	}
	a.state.Update(func(s *State) { s.SidebarEdge = e })
	a.mu.Lock()
	w := a.main
	a.mu.Unlock()
	if w != nil && !w.IsDestroyed() && a.layout() == layoutSidebar {
		a.dockSidebar(w, w.Bounds().Width)
	}
	a.syncLayoutMenu()
}

// sidebarRect is a sidebar of width at edge of the work area, at its full
// height. The width stays between minPanelSide and what leaves
// minPageBesides of the work area to the rest of the screen.
func sidebarRect(work Bounds, edge string, width int) Bounds {
	maxW := max(minPanelSide, work.W-minPageBesides)
	width = max(minPanelSide, min(width, maxW))
	width = min(width, max(work.W, 1))
	r := Bounds{Y: work.Y, W: width, H: work.H, X: work.X}
	if edge != config.EdgeLeft {
		r.X = work.X + work.W - width
	}
	return r
}

// ---- wiring ----

// watchLayout wires the main window's layout: the sidebar's re-docking and
// auto-hide, and remembering the compact window's place as it closes.
func (a *App) watchLayout(w *mygo.Window) {
	w.OnResize(func() { a.redockSoon(w) })
	w.OnMove(func() { a.redockSoon(w) })
	// The saved window place is applied as the window shows, after the
	// first docking: dock again then.
	w.OnShow(func() { a.redockSoon(w) })
	a.redockSoon(w)
	if a.cfg.Window.SidebarAutoHide {
		mygo.App.OnDidResignActive(func() {
			if a.layout() == layoutSidebar && !w.IsDestroyed() && w.IsVisible() {
				w.Hide()
			}
		})
	}
	w.OnClose(func(*mygo.CloseEvent) {
		if a.isCompact() {
			a.saveCompactBounds(w)
		}
	})
}

// applyStartLayout lays a new main window out as it starts.
func (a *App) applyStartLayout(w *mygo.Window) {
	if l := a.startLayout(); l != layoutWindow {
		a.setLayout(w, l)
	}
	a.watchLayout(w)
}

// hidesOnClose reports whether closing the main window only hides it.
func (a *App) hidesOnClose() bool {
	return a.cfg.Window.HideOnClose || a.cfg.Window.EffectiveMode() == config.ModeSidebar
}

// saveCompactBounds remembers where the compact window is.
func (a *App) saveCompactBounds(w *mygo.Window) {
	if w == nil || w.IsDestroyed() {
		return
	}
	b := w.Bounds()
	if b.Width <= 0 || b.Height <= 0 {
		return
	}
	a.state.Update(func(s *State) { s.CompactBounds = toBounds(b) })
}

// saveLayoutOnQuit keeps the compact window's place across quits.
func (a *App) saveLayoutOnQuit() {
	a.mu.Lock()
	w := a.main
	a.mu.Unlock()
	if a.isCompact() {
		a.saveCompactBounds(w)
	}
}

// layoutMenuItems are the View menu's layout items (not for menu-bar apps).
func (a *App) layoutMenuItems() []*mygo.MenuItem {
	if a.menuBarMode() {
		return nil
	}
	start := a.startLayout()
	item := func(id, label, accel string) *mygo.MenuItem {
		return &mygo.MenuItem{ID: layoutMenuPrefix + id, Label: label, Type: mygo.MenuItemRadio, Checked: start == id,
			Accelerator: a.acc(accel), Click: func(*mygo.MenuItem, *mygo.Window) { a.pickLayout(id) }}
	}
	edge := "Move Sidebar to Left"
	if a.sidebarEdge() == config.EdgeLeft {
		edge = "Move Sidebar to Right"
	}
	return []*mygo.MenuItem{
		mygo.Separator(),
		item(layoutWindow, "Window", windowAccel),
		item(layoutCompact, "Compact Window", compactAccel),
		item(layoutSidebar, "Sidebar", sidebarAccel),
		{ID: sidebarEdgeID, Label: edge, Disabled: start != layoutSidebar, Click: func(*mygo.MenuItem, *mygo.Window) { a.flipSidebarEdge() }},
	}
}

func (a *App) syncLayoutMenu() {
	if a.menu == nil {
		return
	}
	cur := a.layout()
	for _, id := range []string{layoutWindow, layoutCompact, layoutSidebar} {
		if it := a.menu.ItemByID(layoutMenuPrefix + id); it != nil {
			it.SetChecked(cur == id)
		}
	}
	if it := a.menu.ItemByID(sidebarEdgeID); it != nil {
		it.SetEnabled(cur == layoutSidebar)
		if a.sidebarEdge() == config.EdgeLeft {
			it.SetLabel("Move Sidebar to Right")
		} else {
			it.SetLabel("Move Sidebar to Left")
		}
	}
}

// ---- geometry ----

// compactRect is where a compact window of w x h goes: where it was last
// (when that still overlaps the work area by a usable amount, clamped
// inside it), else in the bottom-right corner of the work area.
func compactRect(saved *Bounds, work Bounds, w, h int) Bounds {
	if work.W <= 0 || work.H <= 0 {
		return Bounds{X: 0, Y: 0, W: w, H: h}
	}
	if saved != nil && saved.W >= minPanelSide && saved.H >= minPanelSide && overlap(*saved, work) >= 0.5 {
		return clampInto(*saved, work)
	}
	r := Bounds{W: min(w, work.W), H: min(h, work.H)}
	r.X = work.X + work.W - r.W - compactMargin
	r.Y = work.Y + work.H - r.H - compactMargin
	return clampInto(r, work)
}

// overlap is the share of r inside work (0..1).
func overlap(r, work Bounds) float64 {
	x0, y0 := max(r.X, work.X), max(r.Y, work.Y)
	x1, y1 := min(r.X+r.W, work.X+work.W), min(r.Y+r.H, work.Y+work.H)
	if x1 <= x0 || y1 <= y0 || r.W <= 0 || r.H <= 0 {
		return 0
	}
	return float64((x1-x0)*(y1-y0)) / float64(r.W*r.H)
}

// clampInto moves (and if needed shrinks) r to lie inside work.
func clampInto(r, work Bounds) Bounds {
	r.W, r.H = min(r.W, work.W), min(r.H, work.H)
	r.X = max(work.X, min(r.X, work.X+work.W-r.W))
	r.Y = max(work.Y, min(r.Y, work.Y+work.H-r.H))
	return r
}

func toBounds(r mygo.Rectangle) *Bounds {
	return &Bounds{X: r.X, Y: r.Y, W: r.Width, H: r.Height}
}

func fromRect(r mygo.Rectangle) Bounds {
	return Bounds{X: r.X, Y: r.Y, W: r.Width, H: r.Height}
}
