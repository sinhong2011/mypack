package app

import (
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/net/publicsuffix"
	"mypack.dev/mypack/pkg/config"
)

// Group apps (app.group, runtime 1.0.6): one window with a rail of the
// member websites' icons and a web view per member, each in a session of
// its own (separate logins, as containers). Every member loads at start,
// so each notifies and badges while another shows; the rail shows each
// member's unread count and the Dock the total. A link to another
// member's site opens in that member. Split view shows two members side
// by side, with a divider to drag. The group works in every window.mode:
// a window, compact, sidebar or the menu-bar panel.

const (
	railWidth      = 68
	railIcon       = 40
	railTopDarwin  = 40 // room for the traffic lights
	splitMin       = 0.2
	dividerWidth   = 6
	groupSessionID = "." // profileId + "." + member id
)

type groupMember struct {
	cfg   config.GroupMember
	home  string
	host  string // hostname of home
	reg   string // registrable domain of home
	icon  ui.ImageSource
	sess  *mygo.Session
	view  *mygo.WebView
	page  *mygo.Page
	index int
}

type group struct {
	a       *App
	members []*groupMember

	mu     sync.Mutex
	win    *mygo.Window
	active int
	split  int // member shown beside the active one, or -1
	prev   int // the member shown before the active one
	ratio  float32
}

// newGroup prepares the members of cfg's group (sessions are made later,
// on the main thread).
func (a *App) newGroup() *group {
	g := &group{a: a, split: -1, prev: -1, ratio: 0.5}
	for i, m := range a.cfg.App.Group.Members {
		home, err := config.NormalizeURL(m.URL)
		if err != nil {
			continue // validated; never happens
		}
		gm := &groupMember{cfg: m, home: home, index: i}
		if u, err := url.Parse(home); err == nil {
			gm.host = strings.ToLower(u.Hostname())
			gm.reg = siteOf(gm.host)
		}
		if m.Icon != "" {
			p := m.Icon
			if !filepath.IsAbs(p) {
				p = filepath.Join(a.cfgDir, filepath.FromSlash(p))
			}
			if b, err := os.ReadFile(p); err == nil {
				if img, err := ui.DecodeImage(b); err == nil {
					gm.icon = img
				}
			}
		}
		g.members = append(g.members, gm)
	}
	return g
}

// groupSafeDomains are the members' sites, all internal to the app.
func (g *group) safeDomains() []string {
	var out []string
	for _, m := range g.members {
		if m.reg != "" {
			out = append(out, m.reg)
		}
	}
	return out
}

// attach creates the members' web views in w, a window (or flyout) that
// shows the group's view. Main thread.
func (g *group) attach(w *mygo.Window) *mygo.Page {
	a := g.a
	g.mu.Lock()
	g.win = w
	g.mu.Unlock()
	start := g.startMember()
	for _, m := range g.members {
		if a.cfg.WebView.Incognito {
			m.sess = mygo.NewEphemeralSession()
		} else {
			m.sess = mygo.NewSession(a.cfg.App.ProfileID + groupSessionID + m.cfg.ID)
		}
		wo := mygo.WebViewOptions{URL: m.home, Page: a.pageOptionsIn(m.sess), BackgroundColor: a.cfg.Window.Background}
		v, err := w.NewWebView(wo)
		if err != nil && wo.BackgroundColor != "" {
			wo.BackgroundColor = ""
			v, err = w.NewWebView(wo)
		}
		if err != nil {
			log.Printf("mypack-runtime: group member %s: %v", m.cfg.ID, err)
			continue
		}
		m.view, m.page = v, v.Page()
		m := m
		a.wirePage(w, m.page,
			func(e *mygo.NavigateEvent) { g.onNavigate(m, e) },
			func(req mygo.WindowOpenRequest) *mygo.WindowOptions { return g.onOpen(m, req) })
		m.page.OnPageTitleUpdated(func(*mygo.TitleEvent) { w.Invalidate() })
	}
	a.badges.changed = func() {
		if w := g.window(); w != nil {
			w.Invalidate()
		}
	}
	g.show(start, false)
	p := g.activePage()
	a.trackClose(w, p)
	w.OnClosed(func() {
		for _, m := range g.members {
			if m.page != nil {
				a.badges.drop(m.page)
			}
		}
	})
	return p
}

func (g *group) window() *mygo.Window {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.win
}

// startMember is the member shown first: the last one shown, else the first.
func (g *group) startMember() int {
	id := g.a.state.Get().GroupActive
	for i, m := range g.members {
		if m.cfg.ID == id {
			return i
		}
	}
	return 0
}

func (g *group) activePage() *mygo.Page {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active < len(g.members) {
		return g.members[g.active].page
	}
	return nil
}

// memberOfPage returns the member showing p, or nil.
func (g *group) memberOfPage(p *mygo.Page) *groupMember {
	for _, m := range g.members {
		if m.page == p {
			return m
		}
	}
	return nil
}

// show makes member i the active one; with focus it also takes the
// keyboard. A member shown beside it (split) swaps sides when picked.
func (g *group) show(i int, focus bool) {
	if i < 0 || i >= len(g.members) {
		return
	}
	g.mu.Lock()
	if i == g.split {
		g.split = g.active
	} else if i != g.active {
		g.prev = g.active
	}
	g.active = i
	if g.split == g.active {
		g.split = -1
	}
	w := g.win
	m := g.members[i]
	g.mu.Unlock()
	a := g.a
	// Menu commands (reload, find, zoom...) act on the active member.
	if w != nil && m.page != nil {
		a.mu.Lock()
		if _, ok := a.windows[w]; ok || a.main == w {
			a.windows[w] = m.page
		}
		a.mu.Unlock()
	}
	a.state.Update(func(s *State) { s.GroupActive = m.cfg.ID })
	if w != nil {
		w.Invalidate()
		if focus && m.view != nil {
			m.view.Focus()
		}
	}
}

// step shows the next (+1) or previous (-1) member.
func (g *group) step(d int) {
	g.mu.Lock()
	n := len(g.members)
	i := ((g.active+d)%n + n) % n
	g.mu.Unlock()
	g.show(i, true)
}

// toggleSplit shows the member shown before (else the next one) beside the
// active one, or closes the split.
func (g *group) toggleSplit() {
	g.mu.Lock()
	if g.split >= 0 {
		g.split = -1
	} else if len(g.members) > 1 {
		o := g.prev
		if o < 0 || o == g.active || o >= len(g.members) {
			o = (g.active + 1) % len(g.members)
		}
		g.split = o
	}
	w := g.win
	g.mu.Unlock()
	if w != nil {
		w.Invalidate()
	}
}

// splitWith shows member i beside the active one.
func (g *group) splitWith(i int) {
	g.mu.Lock()
	if i == g.active {
		g.mu.Unlock()
		return
	}
	g.split = i
	w := g.win
	g.mu.Unlock()
	if w != nil {
		w.Invalidate()
	}
}

// ownerOf returns the member whose site raw belongs to, other than from
// (nil when none, or when it is from's own site). An exact host wins;
// otherwise the one member on the registrable domain, when only one is.
func (g *group) ownerOf(raw string, from *groupMember) *groupMember {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	reg := siteOf(host)
	if from != nil && (host == from.host || (from.reg != "" && reg == from.reg && !g.exactOther(host, from))) {
		return nil
	}
	for _, m := range g.members {
		if m != from && m.host == host {
			return m
		}
	}
	var found *groupMember
	for _, m := range g.members {
		if m != from && m.reg == reg {
			if found != nil {
				return nil // ambiguous: stay where the link was clicked
			}
			found = m
		}
	}
	return found
}

func (g *group) exactOther(host string, from *groupMember) bool {
	for _, m := range g.members {
		if m != from && m.host == host {
			return true
		}
	}
	return false
}

// openIn loads raw in member m and shows it.
func (g *group) openIn(m *groupMember, raw string) {
	if m.page != nil {
		_ = m.page.LoadURL(raw)
	}
	mygo.RunOnMain(func() { g.show(m.index, true) })
}

func (g *group) onNavigate(from *groupMember, e *mygo.NavigateEvent) {
	if e.UserInitiated {
		if to := g.ownerOf(e.URL, from); to != nil {
			e.PreventDefault()
			g.openIn(to, e.URL)
			return
		}
	}
	g.a.onWillNavigate(e)
}

func (g *group) onOpen(from *groupMember, req mygo.WindowOpenRequest) *mygo.WindowOptions {
	if req.UserInitiated {
		if to := g.ownerOf(req.URL, from); to != nil {
			g.openIn(to, req.URL)
			return nil
		}
	}
	return g.a.onWindowOpen(req)
}

// focusMember shows the member of page p (a notification was clicked).
func (g *group) focusMember(p *mygo.Page) bool {
	m := g.memberOfPage(p)
	if m == nil {
		return false
	}
	g.a.showMain()
	mygo.RunOnMain(func() { g.show(m.index, true) })
	return true
}

// handleIncoming opens a link handed to the app in the member it belongs
// to; false when none claims it.
func (g *group) handleIncoming(raw string) bool {
	to := g.ownerOf(raw, nil)
	if to == nil {
		return false
	}
	g.a.showMain()
	g.openIn(to, raw)
	return true
}

// ---- view ----

// viewFn draws the rail and the shown members. Main thread.
func (g *group) viewFn(c *ui.Context) {
	t := c.Theme()
	g.mu.Lock()
	active, split, ratio := g.active, g.split, g.ratio
	g.mu.Unlock()
	top := float32(8)
	if runtime.GOOS == "darwin" && !g.a.menuBarMode() {
		top = railTopDarwin
	}
	// The rail: a row along the top of the narrow menu-bar panel; down
	// the inner edge elsewhere (the right one of a sidebar docked left).
	horizontal := g.a.menuBarMode()
	railRight := g.a.layout() == layoutSidebar && g.a.sidebarEdge() == config.EdgeLeft
	drawRail := func() {
		var rail ui.Element
		if horizontal {
			rail = ui.Row(c.Key("rail")).Height(railIcon+16).FillWidth().Shrink(0).Gap(4).AlignItems(ui.Center).Padding(0, 8).Background(t.Surface)
		} else {
			rail = ui.Column(c.Key("rail")).Width(railWidth).FillHeight().Shrink(0).Gap(8).AlignItems(ui.Center).
				Padding(top, 0, 12, 0).Background(t.Surface).DragWindow()
		}
		rail.Children(func() {
			for i, m := range g.members {
				g.railItem(c.Key("m-"+m.cfg.ID), m, i == active, i == split, horizontal)
			}
		})
		if horizontal {
			ui.Box(c).Height(1).FillWidth().Background(t.Border)
		} else {
			ui.Box(c).Width(1).FillHeight().Background(t.Border)
		}
	}
	var outer ui.Element
	if horizontal {
		outer = ui.Column(c).Fill().Gap(0)
	} else {
		outer = ui.Row(c).Fill().Gap(0)
	}
	outer.Children(func() {
		if !railRight {
			drawRail()
		}
		defer func() {
			if railRight {
				drawRail()
			}
		}()
		if active >= len(g.members) || g.members[active].view == nil {
			ui.Spacer(c)
			return
		}
		if split < 0 || split >= len(g.members) || g.members[split].view == nil {
			ui.WebView(c, g.members[active].view).Grow(1)
			return
		}
		area := ui.Row(c).Grow(1).FillHeight().Gap(0)
		area.Children(func() {
			ui.WebView(c, g.members[active].view).Grow(ratio)
			div := ui.Box(c.Key("divider")).Width(dividerWidth).FillHeight().Background(t.Border).Cursor(ui.CursorResizeEW).Label("Resize split view")
			if dx, _, ok := div.Dragged(); ok && dx != 0 {
				if wdt := area.Bounds().W; wdt > 0 {
					g.mu.Lock()
					g.ratio = min(1-splitMin, max(splitMin, g.ratio+dx/wdt))
					g.mu.Unlock()
				}
			}
			if div.DoubleClicked() {
				g.mu.Lock()
				g.ratio = 0.5
				g.mu.Unlock()
			}
			ui.WebView(c, g.members[split].view).Grow(1 - ratio)
		})
	})
}

// railItem is a member's icon: its picture (else its initial), a marker
// when it shows, and its unread count.
func (g *group) railItem(c *ui.Context, m *groupMember, active, beside, horizontal bool) {
	t := c.Theme()
	label := ""
	if m.page != nil {
		label = g.a.badges.pageLabel(m.page)
	}
	slotW := float32(railWidth)
	if horizontal {
		slotW = railIcon + 8
	}
	ui.Box(c).Size(slotW, railIcon).Children(func() {
		if active || beside {
			bar := ui.Box(c).Absolute().Left(0).Top(8).Size(3, railIcon-16).Radius(1.5)
			if horizontal {
				bar = ui.Box(c).Absolute().Left(12).Top(railIcon+3).Size(railIcon-16, 3).Radius(1.5)
			}
			if active {
				bar.Background(t.Accent)
			} else {
				bar.Background(t.TextMuted)
			}
		}
		btn := ui.Box(c.Key("btn")).Absolute().Left((slotW-railIcon)/2).Top(0).Size(railIcon, railIcon).Radius(10).Clip().
			Role(ui.RoleButton).Label(m.cfg.Name).Tooltip(m.cfg.Name).Focusable()
		if m.icon != nil {
			btn.Children(func() { ui.Image(c, m.icon).Size(railIcon, railIcon).Fit(ui.Cover) })
		} else {
			btn.Background(t.Accent.Mix(t.Surface, 0.75)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, initial(m.cfg.Name)).FontSize(17).Bold().TextColor(t.Accent)
			})
		}
		if !active {
			btn.Opacity(0.82)
		}
		if btn.Clicked() {
			g.show(m.index, true)
		}
		btn.ContextMenu(func(menu *ui.Menu) {
			if menu.Item("Open in Split View").Disabled(active).Chosen() {
				g.splitWith(m.index)
			}
			if beside && menu.Item("Close Split View").Chosen() {
				g.toggleSplit()
			}
			menu.Separator()
			if menu.Item("Reload").Chosen() && m.page != nil {
				m.page.Reload()
			}
			if menu.Item("Go to Home Page").Chosen() && m.page != nil {
				_ = m.page.LoadURL(m.home)
			}
		})
		if label != "" {
			ui.Box(c).Absolute().Right(max(0, (slotW-railIcon)/2-6)).Top(-4).Children(func() {
				ui.Text(c, label).FontSize(10).Bold().TextColor(ui.RGB(255, 255, 255)).Background(ui.RGB(235, 64, 52)).
					Radius(8).Padding(1, 5).SingleLine()
			})
		}
	})
}

func initial(name string) string {
	for _, r := range strings.TrimSpace(name) {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// ---- menu ----

// groupMenuItems are the View menu's members (⌘1…⌘9), next/previous and
// split view.
func (a *App) groupMenuItems() []*mygo.MenuItem {
	if a.group == nil {
		return nil
	}
	g := a.group
	items := []*mygo.MenuItem{mygo.Separator()}
	for i, m := range g.members {
		i := i
		it := &mygo.MenuItem{Label: m.cfg.Name, Click: func(*mygo.MenuItem, *mygo.Window) { g.show(i, true) }}
		if i < 9 {
			it.Accelerator = a.acc("CmdOrCtrl+" + string(rune('1'+i)))
		}
		items = append(items, it)
	}
	items = append(items,
		&mygo.MenuItem{Label: "Next Site", Accelerator: a.acc("Ctrl+Tab"), Click: func(*mygo.MenuItem, *mygo.Window) { g.step(+1) }},
		&mygo.MenuItem{Label: "Previous Site", Accelerator: a.acc("Ctrl+Shift+Tab"), Click: func(*mygo.MenuItem, *mygo.Window) { g.step(-1) }},
		&mygo.MenuItem{Label: "Toggle Split View", Accelerator: a.acc("CmdOrCtrl+\\"), Click: func(*mygo.MenuItem, *mygo.Window) { g.toggleSplit() }},
	)
	return items
}

// openGroupWindow creates the group's window: native UI with the rail, the
// traffic lights over it on macOS. Main thread.
func (a *App) openGroupWindow(stateKey string, hidden bool) (*mygo.Window, *mygo.Page) {
	o := a.windowOptions(true, false)
	o.Page = mygo.PageOptions{}
	o.Hidden, o.StateKey = hidden, stateKey
	if !o.Frameless {
		o.TitleBarStyle = mygo.TitleBarHidden
		o.TrafficLightPosition = &mygo.Point{X: 14, Y: 14}
		o.TitleBarHeight = 32
	}
	o.Content = ui.View(a.group.viewFn)
	w := safeNewWindow(o)
	a.mu.Lock()
	a.windows[w] = nil // marks it an app window before its pages exist
	a.mu.Unlock()
	p := a.group.attach(w)
	a.mu.Lock()
	a.windows[w] = p
	a.mu.Unlock()
	return w, p
}

// homeOf is the home page of page p: its member's in a group.
func (a *App) homeOf(p *mygo.Page) string {
	if a.group != nil {
		if m := a.group.memberOfPage(p); m != nil {
			return m.home
		}
	}
	return a.home
}

// siteOf is the registrable domain of host ("mail.google.com" ->
// "google.com"), else host itself (localhost, IP addresses).
func siteOf(host string) string {
	host = strings.TrimPrefix(host, "www.")
	if s, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return s
	}
	return host
}
