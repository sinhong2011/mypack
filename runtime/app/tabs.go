package app

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"  // favicons
	_ "image/jpeg" // favicons
	_ "image/png"  // favicons
	"io"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/runtime/navigation"
)

// Tabs (window.tabs, runtime 1.0.8): the main window shows a tab bar and a
// web view per tab, all in the app's session. A link that would open a new
// window (target=_blank) opens a tab next to the current one, or shows the
// tab that already has that page. Pinned tabs sit first, small, and stay
// loaded so they notify; the others load when first shown after a
// relaunch. Popups that ask for a size (sign-in windows) stay windows, as
// they need window.opener.

const (
	tabBarH       = 38
	tabMaxW       = 220
	tabMinW       = 96
	tabPinW       = 38
	tabTrafficPad = 78 // the traffic lights on macOS
	maxClosedTabs = 20
	maxTabs       = 40
)

// SavedTab is a tab kept in state.json.
type SavedTab struct {
	URL    string `json:"url"`
	Title  string `json:"title,omitempty"`
	Pinned bool   `json:"pinned,omitempty"`
}

type tab struct {
	id     int
	url    string // the page shown (or to load, before its view exists)
	title  string
	pinned bool
	view   *mygo.WebView
	page   *mygo.Page
	icon   ui.ImageSource
	iconOf string // the page URL whose favicon icon is
}

type tabSet struct {
	a *App

	mu     sync.Mutex
	win    *mygo.Window
	list   []*tab
	active int
	nextID int
	closed []SavedTab
}

func (a *App) tabsOn() bool { return a.cfg.Window.Tabs && a.group == nil && !a.menuBarMode() }

// openTabsWindow creates the main window with its tab bar. Main thread.
func (a *App) openTabsWindow(startURL, stateKey string, hidden bool) (*mygo.Window, *mygo.Page) {
	t := &tabSet{a: a}
	a.mu.Lock()
	a.tabs = t
	a.mu.Unlock()
	o := a.windowOptions(true, false)
	o.Page = mygo.PageOptions{}
	o.Hidden, o.StateKey = hidden, stateKey
	if !o.Frameless {
		o.TitleBarStyle = mygo.TitleBarHidden
		o.TrafficLightPosition = &mygo.Point{X: 14, Y: (tabBarH - 14) / 2}
		o.TitleBarHeight = tabBarH
	}
	o.Content = ui.View(t.viewFn)
	w := safeNewWindow(o)
	a.mu.Lock()
	a.windows[w] = nil // an app window before its pages exist
	a.mu.Unlock()
	t.mu.Lock()
	t.win = w
	t.mu.Unlock()

	saved := a.state.Get().Tabs
	active := a.state.Get().TabActive
	if a.cfg.WebView.Incognito {
		saved = nil
	}
	for _, s := range saved {
		if s.URL == "" || a.policy.Navigate(s.URL, false) != navigation.Internal {
			continue
		}
		t.list = append(t.list, &tab{id: t.newID(), url: s.URL, title: s.Title, pinned: s.Pinned})
	}
	if active < 0 || active >= len(t.list) {
		active = 0
	}
	if len(t.list) == 0 {
		t.list = append(t.list, &tab{id: t.newID(), url: startURL})
	} else if startURL != a.home && startURL != "" {
		// Launched with a link: show it, in its own tab unless one has it.
		if i := t.find(startURL); i >= 0 {
			active = i
		} else {
			t.list = append(t.list, &tab{id: t.newID(), url: startURL})
			active = len(t.list) - 1
		}
	}
	// Pinned tabs load now (they notify and badge); the shown one too.
	for i, tb := range t.list {
		if tb.pinned || i == active {
			t.load(tb)
		}
	}
	t.show(active, false)
	p := t.activePage()
	w.OnClosed(func() {
		t.mu.Lock()
		list := append([]*tab(nil), t.list...)
		t.mu.Unlock()
		for _, tb := range list {
			if tb.page != nil {
				a.badges.drop(tb.page)
			}
		}
		a.mu.Lock()
		delete(a.windows, w)
		if a.main == w {
			a.main = nil
			a.lay.cur = ""
		}
		if a.tabs == t {
			a.tabs = nil
		}
		f := a.finds[w]
		delete(a.finds, w)
		a.mu.Unlock()
		if f != nil {
			f.Destroy()
		}
	})
	a.mu.Lock()
	a.windows[w] = p
	a.mu.Unlock()
	return w, p
}

func (t *tabSet) newID() int { t.nextID++; return t.nextID }

func (t *tabSet) window() *mygo.Window {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.win
}

// load creates tb's web view. Main thread.
func (t *tabSet) load(tb *tab) {
	if tb.view != nil {
		return
	}
	a := t.a
	t.mu.Lock()
	w := t.win
	t.mu.Unlock()
	if w == nil {
		return
	}
	wo := mygo.WebViewOptions{URL: tb.url, Page: a.pageOptions(), BackgroundColor: a.cfg.Window.Background}
	v, err := w.NewWebView(wo)
	if err != nil && wo.BackgroundColor != "" {
		wo.BackgroundColor = ""
		v, err = w.NewWebView(wo)
	}
	if err != nil {
		log.Printf("mypack-runtime: tab: %v", err)
		return
	}
	tb.view, tb.page = v, v.Page()
	a.wirePage(w, tb.page, a.onWillNavigate, func(req mygo.WindowOpenRequest) *mygo.WindowOptions { return t.onOpen(tb, req) })
	title := a.title()
	tb.page.OnPageTitleUpdated(func(e *mygo.TitleEvent) {
		e.PreventDefault() // the window keeps the app's name
		w.SetTitle(title)
		t.mu.Lock()
		tb.title = e.Title
		t.mu.Unlock()
		w.Invalidate()
		t.save()
	})
	tb.page.OnDidNavigate(func(u string) {
		t.mu.Lock()
		tb.url = u
		t.mu.Unlock()
		t.save()
		go t.fetchIcon(tb, u)
	})
}

// fetchIcon finds the page's favicon and shows it on the tab.
func (t *tabSet) fetchIcon(tb *tab, pageURL string) {
	time.Sleep(600 * time.Millisecond) // the page's <head> is in by then
	p := tb.page
	if p == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, err := p.EvalContext(ctx, `(function(){var l=document.querySelector('link[rel~="icon"][sizes="32x32"],link[rel~="icon"],link[rel="apple-touch-icon"]');return l?l.href:location.origin+'/favicon.ico'})()`)
	href, _ := v.(string)
	if err != nil || !strings.HasPrefix(href, "http") {
		return
	}
	t.mu.Lock()
	same := tb.iconOf != "" && sameHost(tb.iconOf, pageURL)
	t.mu.Unlock()
	if same {
		return
	}
	img := loadFavicon(ctx, href)
	if img == nil {
		return
	}
	t.mu.Lock()
	tb.icon, tb.iconOf = img, pageURL
	w := t.win
	t.mu.Unlock()
	if w != nil {
		w.Invalidate()
	}
}

var faviconClient = &http.Client{Timeout: 4 * time.Second}

func loadFavicon(ctx context.Context, href string) ui.ImageSource {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, href, nil)
	if err != nil {
		return nil
	}
	resp, err := faviconClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil || len(b) == 0 {
		return nil
	}
	if img, err := ui.DecodeImage(b); err == nil {
		return img
	}
	// ICO and friends: decode, then hand over as PNG.
	if img, err := icons.Decode(b); err == nil {
		if png, err := icons.EncodePNG(img); err == nil {
			if bm, err := ui.DecodeBitmap(png); err == nil {
				return bm
			}
		}
	}
	_, _, _ = image.Decode(bytes.NewReader(b))
	return nil
}

func sameHost(a, b string) bool {
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	return err1 == nil && err2 == nil && strings.EqualFold(ua.Host, ub.Host)
}

func (t *tabSet) activePage() *mygo.Page {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.active < len(t.list) {
		return t.list[t.active].page
	}
	return nil
}

// find returns the tab showing raw (ignoring the fragment), or -1.
func (t *tabSet) find(raw string) int {
	key := func(s string) string {
		s, _, _ = strings.Cut(s, "#")
		return strings.TrimSuffix(s, "/")
	}
	k := key(raw)
	for i, tb := range t.list {
		u := tb.url
		if tb.page != nil {
			if cur := tb.page.URL(); cur != "" {
				u = cur
			}
		}
		if key(u) == k {
			return i
		}
	}
	return -1
}

// show makes tab i the shown one. Main thread.
func (t *tabSet) show(i int, focus bool) {
	t.mu.Lock()
	if i < 0 || i >= len(t.list) {
		t.mu.Unlock()
		return
	}
	t.active = i
	tb := t.list[i]
	w := t.win
	t.mu.Unlock()
	t.load(tb)
	a := t.a
	if w != nil && tb.page != nil {
		a.mu.Lock()
		a.windows[w] = tb.page // menu commands act on the shown tab
		a.mu.Unlock()
	}
	if w != nil {
		w.Invalidate()
		if focus && tb.view != nil {
			tb.view.Focus()
		}
	}
	t.save()
}

// open shows raw: in the tab that has it, else in a new tab after the
// shown one (or at the end).
func (t *tabSet) open(raw string, focus bool) {
	if i := t.find(raw); i >= 0 {
		t.show(i, focus)
		return
	}
	t.mu.Lock()
	if len(t.list) >= maxTabs {
		t.mu.Unlock()
		if p := t.activePage(); p != nil {
			_ = p.LoadURL(raw)
		}
		return
	}
	tb := &tab{id: t.newID(), url: raw}
	at := t.active + 1
	for at < len(t.list) && t.list[at].pinned {
		at++
	}
	t.list = append(t.list[:at], append([]*tab{tb}, t.list[at:]...)...)
	t.mu.Unlock()
	t.show(at, focus)
}

// newTab opens the app's home page in a new tab.
func (t *tabSet) newTab() {
	t.mu.Lock()
	tb := &tab{id: t.newID(), url: t.a.home}
	t.list = append(t.list, tb)
	i := len(t.list) - 1
	t.mu.Unlock()
	t.show(i, true)
}

// close closes tab i; the last tab closes the window. A pinned tab is not
// closed by the keyboard (pin=false): the next tab shows instead.
func (t *tabSet) close(i int, fromKeyboard bool) {
	t.mu.Lock()
	if i < 0 || i >= len(t.list) {
		t.mu.Unlock()
		return
	}
	tb := t.list[i]
	if fromKeyboard && tb.pinned {
		n := len(t.list)
		next := -1
		for j := 1; j < n; j++ {
			if !t.list[(i+j)%n].pinned {
				next = (i + j) % n
				break
			}
		}
		t.mu.Unlock()
		if next >= 0 {
			t.show(next, true)
		}
		return
	}
	w := t.win
	if len(t.list) == 1 {
		t.mu.Unlock()
		if w != nil {
			w.Close()
		}
		return
	}
	t.closed = append(t.closed, SavedTab{URL: tb.url, Title: tb.title, Pinned: tb.pinned})
	if len(t.closed) > maxClosedTabs {
		t.closed = t.closed[1:]
	}
	t.list = append(t.list[:i:i], t.list[i+1:]...)
	next := t.active
	switch {
	case i < t.active:
		next = t.active - 1
	case i == t.active:
		next = min(i, len(t.list)-1)
	}
	t.active = -1
	t.mu.Unlock()
	if tb.page != nil {
		t.a.badges.drop(tb.page)
	}
	if tb.view != nil {
		tb.view.Destroy()
	}
	t.show(next, true)
}

// closeOthers closes every unpinned tab but i.
func (t *tabSet) closeOthers(i int) {
	t.mu.Lock()
	if i < 0 || i >= len(t.list) {
		t.mu.Unlock()
		return
	}
	keep := t.list[i]
	var drop []*tab
	var kept []*tab
	for _, tb := range t.list {
		if tb == keep || tb.pinned {
			kept = append(kept, tb)
		} else {
			drop = append(drop, tb)
		}
	}
	t.list = kept
	idx := 0
	for j, tb := range kept {
		if tb == keep {
			idx = j
		}
	}
	t.mu.Unlock()
	for _, tb := range drop {
		if tb.page != nil {
			t.a.badges.drop(tb.page)
		}
		if tb.view != nil {
			tb.view.Destroy()
		}
	}
	t.show(idx, true)
}

// reopen brings back the tab closed last.
func (t *tabSet) reopen() {
	t.mu.Lock()
	if len(t.closed) == 0 {
		t.mu.Unlock()
		return
	}
	s := t.closed[len(t.closed)-1]
	t.closed = t.closed[:len(t.closed)-1]
	t.mu.Unlock()
	t.open(s.URL, true)
	if s.Pinned {
		t.mu.Lock()
		i := t.active
		t.mu.Unlock()
		t.togglePin(i)
	}
}

// togglePin pins tab i (moving it after the other pinned tabs) or unpins it.
func (t *tabSet) togglePin(i int) {
	t.mu.Lock()
	if i < 0 || i >= len(t.list) {
		t.mu.Unlock()
		return
	}
	tb := t.list[i]
	activeTab := t.list[max(0, min(t.active, len(t.list)-1))]
	tb.pinned = !tb.pinned
	t.list = append(t.list[:i:i], t.list[i+1:]...)
	at := 0
	for at < len(t.list) && t.list[at].pinned {
		at++
	}
	t.list = append(t.list[:at], append([]*tab{tb}, t.list[at:]...)...)
	for j, x := range t.list {
		if x == activeTab {
			t.active = j
		}
	}
	w := t.win
	t.mu.Unlock()
	if w != nil {
		w.Invalidate()
	}
	t.save()
}

// step shows the next (+1) or previous (-1) tab.
func (t *tabSet) step(d int) {
	t.mu.Lock()
	n := len(t.list)
	i := ((t.active+d)%n + n) % n
	t.mu.Unlock()
	t.show(i, true)
}

// save keeps the tabs for the next launch (never in incognito apps).
func (t *tabSet) save() {
	if t.a.cfg.WebView.Incognito {
		return
	}
	t.mu.Lock()
	out := make([]SavedTab, 0, len(t.list))
	for _, tb := range t.list {
		if strings.HasPrefix(tb.url, "http") || strings.HasPrefix(tb.url, siteScheme+":") {
			out = append(out, SavedTab{URL: tb.url, Title: tb.title, Pinned: tb.pinned})
		}
	}
	active := t.active
	t.mu.Unlock()
	t.a.state.Update(func(s *State) { s.Tabs, s.TabActive = out, active })
}

// onOpen turns a link that opens a new window into a tab. Popups that ask
// for a size keep their window (sign-in flows need window.opener).
func (t *tabSet) onOpen(from *tab, req mygo.WindowOpenRequest) *mygo.WindowOptions {
	if req.UserInitiated && req.Width == 0 && req.Height == 0 &&
		t.a.policy.Popup(req.URL, true) == navigation.Internal && strings.HasPrefix(req.URL, "http") {
		u := req.URL
		mygo.RunOnMain(func() { t.open(u, true) })
		return nil
	}
	return t.a.onWindowOpen(req)
}

// focusPage shows the tab of page p (a notification was clicked).
func (t *tabSet) focusPage(p *mygo.Page) bool {
	t.mu.Lock()
	idx := -1
	for i, tb := range t.list {
		if tb.page == p {
			idx = i
		}
	}
	t.mu.Unlock()
	if idx < 0 {
		return false
	}
	t.a.showMain()
	mygo.RunOnMain(func() { t.show(idx, true) })
	return true
}

// ---- view ----

func (t *tabSet) viewFn(c *ui.Context) {
	th := c.Theme()
	t.mu.Lock()
	list := append([]*tab(nil), t.list...)
	active := t.active
	t.mu.Unlock()
	left := float32(8)
	if runtime.GOOS == "darwin" && !t.a.cfg.Window.Fullscreen {
		left = tabTrafficPad
	}
	ui.Column(c).Fill().Gap(0).Children(func() {
		bar := ui.Row(c.Key("tabbar")).FillWidth().Height(tabBarH).Shrink(0).Gap(4).AlignItems(ui.Center).
			Padding(0, 8, 0, left).Background(th.Surface).DragWindow().Role(ui.RoleTabList).Label("Tabs")
		bar.Children(func() {
			for i, tb := range list {
				t.tabItem(c.Key(tabKey(tb.id)), tb, i, i == active, len(list))
			}
			plus := ui.Box(c.Key("new")).Size(26, 26).Radius(7).Center().Shrink(0).Role(ui.RoleButton).Label("New Tab").
				Tooltip("New Tab").Focusable()
			if plus.Hovered() {
				plus.Background(th.SurfaceHover)
			}
			plus.Children(func() { ui.Text(c, "+").FontSize(18).TextColor(th.TextMuted) })
			if plus.Clicked() {
				t.newTab()
			}
			ui.Spacer(c)
		})
		if bar.DoubleClicked() {
			t.newTab()
		}
		ui.Box(c).Height(1).FillWidth().Background(th.Border)
		if active >= 0 && active < len(list) && list[active].view != nil {
			ui.WebView(c, list[active].view).Grow(1)
		} else {
			ui.Spacer(c)
		}
	})
}

func tabKey(id int) string { return "t" + itoa(id) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// tabItem draws one tab: favicon, title and (hovered or shown) a close
// button; a pinned tab is its icon alone.
func (t *tabSet) tabItem(c *ui.Context, tb *tab, i int, active bool, n int) {
	th := c.Theme()
	t.mu.Lock()
	title, icon, pinned := tb.title, tb.icon, tb.pinned
	t.mu.Unlock()
	if title == "" {
		title = hostOf(tb.url)
	}
	if title == "" {
		title = "New Tab"
	}
	item := ui.Row(c).Height(28).Radius(7).Gap(7).AlignItems(ui.Center).Role(ui.RoleTab).AccessSelected(active).
		Label(title).Tooltip(title).Focusable().Cursor(ui.CursorDefault)
	if pinned {
		item.Width(tabPinW).Shrink(0).Justify(ui.Center)
	} else {
		item.Grow(1).MinWidth(tabMinW).MaxWidth(tabMaxW).Padding(0, 6, 0, 10)
	}
	hov := item.Hovered()
	switch {
	case active:
		item.Background(th.Background).Border(0.5, th.Border)
	case hov:
		item.Background(th.SurfaceHover)
	}
	item.Children(func() {
		if icon != nil {
			ui.Image(c, icon).Size(16, 16).Radius(3)
		} else {
			ui.Box(c).Size(16, 16).Radius(4).Background(th.Accent.Mix(th.Surface, 0.7)).Center().Children(func() {
				ui.Text(c, initial(title)).FontSize(10).Bold().TextColor(th.Accent)
			})
		}
		if pinned {
			if p := tb.page; p != nil {
				if l := t.a.badges.pageLabel(p); l != "" {
					ui.Box(c).Absolute().Right(2).Top(2).Size(7, 7).Radius(3.5).Background(ui.RGB(235, 64, 52))
				}
			}
			return
		}
		fg := th.TextMuted
		if active {
			fg = th.Text
		}
		ui.Text(c, title).FontSize(12).TextColor(fg).SingleLine().Grow(1).MinWidth(0)
		if hov || active {
			x := ui.Box(c.Key("x")).Size(18, 18).Radius(5).Center().Shrink(0).Role(ui.RoleButton).Label("Close Tab")
			if x.Hovered() {
				x.Background(th.SurfacePressed)
			}
			x.Children(func() { ui.Text(c, "×").FontSize(14).TextColor(th.TextMuted) })
			if x.Clicked() {
				t.close(i, false)
			}
		}
	})
	if item.Clicked() {
		t.show(i, true)
	}
	item.ContextMenu(func(menu *ui.Menu) {
		label := "Pin Tab"
		if pinned {
			label = "Unpin Tab"
		}
		if menu.Item(label).Chosen() {
			t.togglePin(i)
		}
		if menu.Item("Duplicate Tab").Chosen() {
			u := tb.url
			if tb.page != nil {
				u = tb.page.URL()
			}
			t.mu.Lock()
			d := &tab{id: t.newID(), url: u}
			t.list = append(t.list[:i+1], append([]*tab{d}, t.list[i+1:]...)...)
			t.mu.Unlock()
			t.show(i+1, true)
		}
		if menu.Item("Reload").Chosen() && tb.page != nil {
			tb.page.Reload()
		}
		menu.Separator()
		if menu.Item("Close Tab").Chosen() {
			t.close(i, false)
		}
		if menu.Item("Close Other Tabs").Disabled(n < 2).Chosen() {
			t.closeOthers(i)
		}
	})
}

// ---- menu ----

// tabMenuItems are the File menu's tab commands.
func (a *App) tabMenuItems() []*mygo.MenuItem {
	on := func(fn func(t *tabSet)) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			a.mu.Lock()
			t := a.tabs
			a.mu.Unlock()
			if t == nil {
				a.showMain()
				return
			}
			fn(t)
		}
	}
	active := func(t *tabSet) int { t.mu.Lock(); defer t.mu.Unlock(); return t.active }
	return []*mygo.MenuItem{
		{Label: "New Tab", Accelerator: a.acc("CmdOrCtrl+T"), Click: on(func(t *tabSet) { t.newTab() })},
		{Label: "Reopen Closed Tab", Accelerator: a.acc("CmdOrCtrl+Shift+T"), Click: on(func(t *tabSet) { t.reopen() })},
		{Label: "Pin Tab", Click: on(func(t *tabSet) { t.togglePin(active(t)) })},
		mygo.Separator(),
		{Label: "Close Tab", Accelerator: a.acc("CmdOrCtrl+W"), Click: func(_ *mygo.MenuItem, w *mygo.Window) {
			a.mu.Lock()
			t := a.tabs
			a.mu.Unlock()
			if t == nil || (w != nil && w != t.window()) {
				if w != nil {
					w.Close() // a popup or another window: ⌘W closes it
				}
				return
			}
			t.close(active(t), true)
		}},
		{Label: "Close Window", Accelerator: a.acc("CmdOrCtrl+Shift+W"), Click: func(_ *mygo.MenuItem, w *mygo.Window) {
			if w != nil {
				w.Close()
			}
		}},
	}
}

// tabSwitchItems are the View menu's tab switching (⌘1…⌘9, ⌃Tab).
func (a *App) tabSwitchItems() []*mygo.MenuItem {
	on := func(fn func(t *tabSet)) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			a.mu.Lock()
			t := a.tabs
			a.mu.Unlock()
			if t != nil {
				fn(t)
			}
		}
	}
	items := []*mygo.MenuItem{mygo.Separator(),
		{Label: "Show Next Tab", Accelerator: a.acc("Ctrl+Tab"), Click: on(func(t *tabSet) { t.step(+1) })},
		{Label: "Show Previous Tab", Accelerator: a.acc("Ctrl+Shift+Tab"), Click: on(func(t *tabSet) { t.step(-1) })},
	}
	for i := 0; i < 9; i++ {
		i := i
		items = append(items, &mygo.MenuItem{Label: "Tab " + itoa(i+1), Accelerator: a.acc("CmdOrCtrl+" + itoa(i+1)), Hidden: true,
			Click: on(func(t *tabSet) {
				t.mu.Lock()
				n := len(t.list)
				t.mu.Unlock()
				if i == 8 {
					t.show(n-1, true) // ⌘9 is the last tab, as in browsers
				} else if i < n {
					t.show(i, true)
				}
			})})
	}
	return items
}
