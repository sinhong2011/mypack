//go:build !mypack_lite

package app

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/runtime/navigation"
)

// Glance (window.glance, runtime 1.0.13, full tier), after Zen Browser's: a
// modifier-click on a link (Alt/Option by default) opens it in a card floating
// over the page instead of navigating away. The card springs out of the
// link along a short arc to 80% of the page's width while the page behind
// shrinks a little and fades; closing (Escape, a click beside the card,
// the close button) sends it back into the link. Expanding it makes it the
// page: a tab of its own in a tabbed window, else the window's page.
//
// The motion is Zen's (src/zen/glance/ZenGlanceManager.mjs): the card is a
// web view laid out at its final size and drawn scaled and moved, so the
// page inside never reflows while it flies; its centre follows a parabola
// whose height is 40% of the distance, at most 30 points and never more
// than 60% of the room above or below; the opening follows a spring with a
// little bounce (0.2), the closing an ease-out (1 - (1 - t)^6), both about
// 350 ms; the page behind goes to 97% and 30% opacity on the same spring.
// With Reduce Motion everything jumps to its end.

const (
	glanceBackScale   = 0.97
	glanceBackOpacity = 0.3
	glanceArcMax      = 30
	glanceArcRatio    = 0.4
	glanceMargin      = 10 // between the card and the top and bottom of the page area
	glanceButtonsW    = 56 // the room the buttons beside the card need
	glanceRadius      = 10
	glanceButton      = 34
	glanceRecentClick = 2 * time.Second
)

// glanceRect is a box in the page area's DIPs.
type glanceRect struct{ X, Y, W, H float32 }

func (r glanceRect) center() (float32, float32) { return r.X + r.W/2, r.Y + r.H/2 }

func lerp(a, b, t float32) float32 { return a + (b-a)*t }

func lerpRect(a, b glanceRect, t float32) glanceRect {
	return glanceRect{lerp(a.X, b.X, t), lerp(a.Y, b.Y, t), lerp(a.W, b.W, t), lerp(a.H, b.H, t)}
}

// glanceMotion is how the card moves (window.glance.animation): the
// length of its motion, the bounce of its opening spring, or none.
type glanceMotion struct {
	d      time.Duration
	bounce float64
	off    bool
}

func motionOf(animation string) glanceMotion {
	switch animation {
	case "quick":
		return glanceMotion{d: 220 * time.Millisecond, bounce: 0.1}
	case "relaxed":
		return glanceMotion{d: 500 * time.Millisecond, bounce: 0.25}
	case "off":
		return glanceMotion{d: 0, off: true}
	}
	return glanceMotion{d: 350 * time.Millisecond, bounce: 0.2} // Zen's
}

// glanceCard is where the card rests in a page area of w by h (top is
// covered by a strip laid over the page, glass), frac of its width wide.
func glanceCard(w, h, top, frac float32) glanceRect {
	// The card takes frac of the width, inside the margins; where that
	// leaves no room beside it, the buttons sit over its corner
	// (drawButtons).
	cw := min(w*frac, w-2*glanceMargin)
	ch := max(h-top-2*glanceMargin, 1)
	return glanceRect{(w - cw) / 2, top + glanceMargin, max(cw, 1), ch}
}

// glanceFrame is the card's box at progress p (0 the link, 1 the card;
// above 1 while the spring overshoots) on its way from from to to, in a
// page area areaH high: its size grows straight and its centre follows
// Zen's arc.
func glanceFrame(from, to glanceRect, p, areaH float32) glanceRect {
	sx, sy := from.center()
	ex, ey := to.center()
	w := lerp(from.W, to.W, p)
	h := lerp(from.H, to.H, p)
	dist := float32(math.Hypot(float64(ex-sx), float64(ey-sy)))
	above := min(sy, ey)
	below := areaH - max(sy, ey)
	down := below > above
	room := above
	if down {
		room = below
	}
	arc := max(min(dist*glanceArcRatio, glanceArcMax, room*0.6), 0)
	if !down {
		arc = -arc
	}
	k := 2*p - 1
	x := lerp(sx, ex, p)
	y := lerp(sy, ey, p) + arc*(1-k*k)
	return glanceRect{x - w/2, y - h/2, w, h}
}

// glanceOut is the closing curve: Zen's "easeOutCubic", 1 - (1 - t)^6.
func glanceOut(t float32) float32 {
	u := 1 - t
	u3 := u * u * u
	return 1 - u3*u3
}

// glance phases.
const (
	glanceIdle = iota
	glanceOpen
	glanceClosing
	glanceExpanding
)

// glance is the glance of one window. Its fields are the main thread's,
// apart from host, which the window's owner sets once.
type glance struct {
	a   *App
	win *mygo.Window

	// host is the window's page area: the page a click came from shows
	// in it, and expand makes the glance the page.
	host glanceHost

	phase  int
	gen    int // a new glance restarts the motion
	fresh  bool
	view   *mygo.WebView
	page   *mygo.Page
	owner  *mygo.Page // the page the glance was opened from
	url    string
	from   glanceRect // the link
	card   glanceRect // where the card rests (last frame)
	area   glanceRect // the page area (last frame)
	copied time.Time
	tint   ui.Color // the glanced page's background, once sampled
	tinted bool

	mu         sync.Mutex
	glancePage *mygo.Page          // page, for other goroutines
	adopted    map[*mygo.Page]bool // glance pages that became pages of the host
}

// glanceHost is what a window's layout does for its glance.
type glanceHost interface {
	// expand makes the glance page the window's; it reports whether the
	// host took the web view (else the glance destroys it).
	expand(g *glance, v *mygo.WebView, url string) bool
	// shownPage is the page shown under the glance.
	shownPage() *mygo.Page
	// topInset is the height of a strip laid over the top of the page.
	topInset() float32
	// pageTint is the colour the host's page paints at its top, if known:
	// the card's colour until the glanced page's own is known.
	pageTint() (ui.Color, bool)
}

func (a *App) glanceOn() bool {
	return a.cfg.Window.Glance.Enabled && !a.framelessBar() && a.group == nil
}

func (a *App) newGlance(w *mygo.Window, host glanceHost) *glance {
	g := &glance{a: a, win: w, host: host, adopted: map[*mygo.Page]bool{}}
	a.mu.Lock()
	a.glances = append(a.glances, g)
	a.mu.Unlock()
	w.OnClosed(func() {
		a.mu.Lock()
		for i, x := range a.glances {
			if x == g {
				a.glances = append(a.glances[:i], a.glances[i+1:]...)
				break
			}
		}
		a.mu.Unlock()
	})
	return g
}

// glanceOf finds the glance of the window showing page p, or of the glance
// showing p.
func (a *App) glanceOf(p *mygo.Page) *glance {
	if p == nil {
		return nil
	}
	a.mu.Lock()
	list := append([]*glance(nil), a.glances...)
	a.mu.Unlock()
	for _, g := range list {
		if g.owns(p) {
			return g
		}
	}
	return nil
}

// owns reports whether p is a page of g's window. Any goroutine.
func (g *glance) owns(p *mygo.Page) bool {
	if g.host != nil {
		if hp, ok := g.host.(interface{ hasPage(*mygo.Page) bool }); ok && hp.hasPage(p) {
			return true
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.glancePage == p
}

// openGlance glances at r.URL from page p. Any goroutine.
func (a *App) openGlance(p *mygo.Page, r GlanceRequest) bool {
	g := a.glanceOf(p)
	if g == nil || !isWebURL(r.URL) {
		return false
	}
	zoom := float32(1)
	if p != nil {
		if z := p.ZoomFactor(); z > 0 {
			zoom = float32(z)
		}
	}
	from := glanceRect{float32(r.X) * zoom, float32(r.Y) * zoom, float32(r.W) * zoom, float32(r.H) * zoom}
	url := r.URL
	g.win.Update(func() { g.open(p, url, from) })
	return true
}

// glanceExternal opens a link leaving the app in a glance
// (glance.externalLinks) from where the page's last click on a link was.
// It reports whether it took the link.
func (a *App) glanceExternal(p *mygo.Page, raw string) bool {
	gc := a.cfg.Window.Glance
	if !a.glanceOn() || !isWebURL(raw) {
		return false
	}
	if !gc.ExternalLinks && !a.pinnedTabPage(p) {
		// Zen glances at every link leaving the site from its Essentials
		// and pinned tabs; elsewhere only with externalLinks.
		return false
	}
	if _, routed := a.routeTarget(raw); routed {
		return false // another MyPack app owns the site
	}
	g := a.glanceOf(p)
	if g == nil {
		return false
	}
	go func() {
		r := GlanceRequest{URL: raw}
		if last, ok := lastGlanceClick(p); ok {
			r.X, r.Y, r.W, r.H = last.X, last.Y, last.W, last.H
		} else {
			// No link box: grow out of the middle of the page, as Zen does.
			r.X, r.Y = -1, -1
		}
		a.openGlance(p, r)
	}()
	return true
}

// lastGlanceClick reads the box of the link the page last saw clicked, if
// that was a moment ago.
func lastGlanceClick(p *mygo.Page) (GlanceRequest, bool) {
	if p == nil {
		return GlanceRequest{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	v, err := p.EvalContext(ctx, `(function(){var l=window.__mypackGlanceLast;return l&&Date.now()-l.t<`+itoa(int(glanceRecentClick/time.Millisecond))+`?JSON.stringify(l):''})()`)
	s, _ := v.(string)
	if err != nil || s == "" {
		return GlanceRequest{}, false
	}
	var r GlanceRequest
	if json.Unmarshal([]byte(s), &r) != nil {
		return GlanceRequest{}, false
	}
	return r, true
}

// open shows url in the glance, coming out of from (the link, in page
// DIPs). Main thread.
func (g *glance) open(owner *mygo.Page, url string, from glanceRect) {
	if g.phase == glanceExpanding {
		return
	}
	if owner != nil && owner == g.page && g.phase == glanceOpen {
		// A glance from the glance: it goes there.
		if err := g.page.LoadURL(url); err != nil {
			log.Printf("mypack-runtime: glance: %v", err)
		}
		g.url = url
		return
	}
	if g.view != nil {
		g.drop() // a glance closing, or another one: the new one takes its place
	}
	v, err := g.newView(url)
	if err != nil {
		log.Printf("mypack-runtime: glance: %v", err)
		g.a.openLink(url)
		return
	}
	top := g.host.topInset()
	if from.X < 0 || from.Y < 0 {
		cx, cy := g.area.center()
		from = glanceRect{cx, cy, 0, 0}
	} else {
		from.Y += top
	}
	from.W, from.H = max(from.W, 8), max(from.H, 8)
	g.view, g.page, g.owner, g.url, g.from = v, v.Page(), owner, url, from
	g.mu.Lock()
	g.glancePage = g.page
	g.mu.Unlock()
	g.phase, g.fresh = glanceOpen, true
	g.tint, g.tinted = g.host.pageTint()
	g.gen++
	log.Printf("mypack-runtime: glance: open host=%s", hostOf(url))
	v.Focus()
	g.win.Invalidate()
}

// newView makes the glance's web view: the app's session and preload (a
// modifier-click inside it glances there), every http(s) link followed in
// place, other schemes handed to the system.
func (g *glance) newView(url string) (*mygo.WebView, error) {
	a := g.a
	wo := mygo.WebViewOptions{URL: url, Page: a.pageOptions(), BackgroundColor: a.cfg.Window.Background}
	v, err := g.win.NewWebView(wo)
	if err != nil && wo.BackgroundColor != "" {
		wo.BackgroundColor = ""
		v, err = g.win.NewWebView(wo)
	}
	if err != nil {
		return nil, err
	}
	p := v.Page()
	p.OnWillNavigate(func(e *mygo.NavigateEvent) {
		if g.isAdopted(p) {
			return // the host's own handlers decide now
		}
		if strings.HasPrefix(e.URL, "http:") || strings.HasPrefix(e.URL, "https:") || strings.HasPrefix(e.URL, "about:") || strings.HasPrefix(e.URL, "blob:") || strings.HasPrefix(e.URL, "data:") {
			return
		}
		e.PreventDefault()
		if e.UserInitiated && a.policy.Navigate(e.URL, true) == navigation.OS {
			openExternal(e.URL)
		}
	})
	p.OnDidNavigate(func(u string) {
		if !g.isAdopted(p) {
			g.win.Update(func() {
				if g.page == p {
					g.url = u
				}
			})
		}
	})
	sample := func() {
		go func() {
			v, err := p.Eval(sampleJS)
			str, _ := v.(string)
			col, ok := parseRGB(str)
			if err != nil || !ok {
				return
			}
			g.win.Update(func() {
				if g.page == p {
					g.tint, g.tinted = col, true
				}
			})
		}()
	}
	p.OnDOMReady(sample)
	p.OnDidFinishLoad(sample)
	p.SetWindowOpenHandler(a.onWindowOpen)
	p.SetPermissionHandler(a.onPermission)
	p.SetAuthHandler(func(req mygo.AuthRequest) mygo.AuthResponse { return a.onAuth(g.win, req) })
	return v, nil
}

func (g *glance) isAdopted(p *mygo.Page) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.adopted[p]
}

// close sends the glance back into its link. Main thread.
func (g *glance) close() {
	if g.phase != glanceOpen {
		return
	}
	g.phase = glanceClosing
	g.backstop(glanceClosing, g.motion().d)
	log.Printf("mypack-runtime: glance: close")
	if p := g.host.shownPage(); p != nil {
		if v := p.WebView(); v != nil {
			v.Focus()
		}
	}
	g.win.Invalidate()
}

// expandNow makes the glance the page. Main thread.
func (g *glance) expandNow() {
	if g.phase != glanceOpen {
		return
	}
	g.phase = glanceExpanding
	g.backstop(glanceExpanding, g.motion().d*5/7)
	g.win.Invalidate()
}

// backstop ends a motion of phase that should be over after d, in case
// the window stopped drawing frames on the way (hidden, minimized). Main
// thread.
func (g *glance) backstop(phase int, d time.Duration) {
	gen := g.gen
	time.AfterFunc(d+400*time.Millisecond, func() {
		g.win.Update(func() {
			if g.gen != gen || g.phase != phase {
				return
			}
			if phase == glanceClosing {
				g.drop()
			} else {
				g.finishExpand()
			}
		})
	})
}

// finishExpand hands the web view to the host once the card fills the
// page area. Main thread.
func (g *glance) finishExpand() {
	v, url := g.view, g.url
	g.mu.Lock()
	g.adopted[g.page] = true
	g.mu.Unlock()
	if g.host.expand(g, v, url) {
		g.view = nil // the host's now
	}
	g.drop()
}

// drop removes the glance at once. Main thread.
func (g *glance) drop() {
	if g.view != nil {
		g.view.Destroy()
	}
	g.mu.Lock()
	g.glancePage = nil
	g.mu.Unlock()
	g.view, g.page, g.owner, g.phase = nil, nil, nil, glanceIdle
	g.win.Invalidate()
}

func (g *glance) motion() glanceMotion { return motionOf(g.a.cfg.Window.Glance.EffectiveAnimation()) }

func (g *glance) active() bool { return g.phase != glanceIdle && g.view != nil }

// draw lays out the page area: the host's page (built by page, filling
// its box) and, over it, the glance. el is the page area's element, which
// the caller sizes. Main thread.
func (g *glance) draw(c *ui.Context, el ui.Element, page func()) {
	th := c.Theme()
	b := el.LayoutBounds()
	if b.W > 0 && b.H > 0 {
		g.area = glanceRect{0, 0, b.W, b.H}
	}
	top := g.host.topInset()
	gc := g.a.cfg.Window.Glance
	g.card = glanceCard(g.area.W, g.area.H, top, float32(gc.EffectiveWidth())/100)
	m := g.motion()
	off := m.off
	reduce := c.Preferences().ReduceMotion || off
	d := max(m.d, time.Millisecond)
	gen := g.gen
	pKey, bKey, eKey, uKey, fKey := [2]int{gen, 0}, [2]int{gen, 1}, [2]int{gen, 2}, [2]int{gen, 3}, [2]int{gen, 4}
	if g.fresh {
		el.Jump(pKey, 0)
		el.Jump(bKey, 0)
		el.Jump(eKey, 0)
		el.Jump(uKey, 0)
		el.Jump(fKey, 0)
		g.fresh = false
	}
	spring := ui.Animation{Spring: ui.SpringOptions{Duration: d, Bounce: m.bounce}}
	var pT, bT, eT, uT, fT float32
	pA, bA, uA, fA := spring, spring, ui.Animation{Spring: ui.SpringOptions{Duration: 200 * time.Millisecond}, Delay: max(d-200*time.Millisecond, 0), NonMotion: true},
		ui.Animation{Duration: d / 4, Ease: ui.EaseInOut, NonMotion: true}
	eA := ui.Animation{Duration: d * 5 / 7, Ease: ui.EaseInOut}
	switch g.phase {
	case glanceOpen:
		pT, bT, uT, fT = 1, 1, 1, 1
	case glanceClosing:
		pT, bT, uT, fT = 0, 0, 0, 0
		pA = ui.Animation{Duration: d, Ease: glanceOut}
		bA = ui.Animation{Spring: ui.SpringOptions{Duration: d * 2 / 3}}
		uA = ui.Animation{Duration: 120 * time.Millisecond, NonMotion: true}
		fA = ui.Animation{Duration: 150 * time.Millisecond, Ease: ui.EaseInOut, NonMotion: true}
	case glanceExpanding:
		pT, bT, eT, uT, fT = 1, 0, 1, 0, 1
		bA = ui.Animation{Duration: d * 5 / 7, Ease: ui.EaseInOut}
		uA = ui.Animation{Duration: 120 * time.Millisecond, NonMotion: true}
	}
	p, back, ex, btn, fade := float32(0), float32(0), float32(0), float32(0), float32(0)
	if g.phase != glanceIdle {
		p = el.AnimateTo(pKey, pT, pA)
		back = el.AnimateTo(bKey, bT, bA)
		ex = el.AnimateTo(eKey, eT, eA)
		btn = el.AnimateTo(uKey, uT, uA)
		fade = el.AnimateTo(fKey, fT, fA)
		if reduce {
			p, ex = pT, eT
		}
		if off { // animation "off": no fades either
			p, back, ex, btn, fade = el.Jump(pKey, pT), el.Jump(bKey, bT), el.Jump(eKey, eT), el.Jump(uKey, uT), el.Jump(fKey, fT)
		}
	}

	// What the page fades into: Zen's toolbar colour. It is laid over the
	// page, never under it: a native web view shows through nothing
	// painted beneath it, so an underlay would hide the page.
	under := ui.RGB(226, 226, 230)
	if th.Dark {
		under = ui.RGB(30, 30, 33)
	}
	// The card is the glanced page's colour (Zen's is the page's own
	// background), so a white page never flies out of a dark card.
	cardBg := th.Background
	if g.tinted {
		cardBg = g.tint
	}
	el.Children(func() {
		pg := ui.Box(c.Key("gl-page")).Fill()
		if back > 0.001 {
			s := lerp(1, glanceBackScale, back)
			pg.Scale(s, s)
		}
		pg.Children(func() {
			page()
			if back > 0.001 {
				// Zen fades the page to 30% over its toolbar. A native
				// web view does not take a fractional opacity well, so
				// the toolbar's colour is laid over the page instead,
				// at 70%: the same blend.
				ui.Box(c.Key("gl-dim")).Absolute().Left(0).Top(0).Right(0).Bottom(0).
					Background(withAlpha(under, (1-glanceBackOpacity)*back))
			}
		})
		if !g.active() {
			return
		}
		if g.phase == glanceOpen {
			// A click beside the card closes it; so does Escape, wherever
			// the focus is.
			bd := ui.Box(c.Key("gl-backdrop")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Label("Close Glance")
			if bd.Clicked() {
				g.close()
			}
			c.OnWindowShortcut(0, ui.KeyEscape, g.close)
		}
		full := glanceRect{0, top, g.area.W, max(g.area.H-top, 1)}
		rest := lerpRect(g.card, full, ex)
		f := glanceFrame(g.from, rest, p, g.area.H)
		radius := lerp(glanceRadius, 0, ex)
		sx, sy := f.W/rest.W, f.H/rest.H
		// The card's background, its web view and the veil over it are
		// siblings, each placed where the card is and scaled about its
		// top-left corner. A web view inside a transformed box is clipped
		// in that box's own coordinates, which cut the page short of the
		// card's bottom; placed directly, it is clipped by the page area.
		place := func(e ui.Element) ui.Element {
			e = e.Absolute().Left(f.X).Top(f.Y).Size(rest.W, rest.H).Radius(radius)
			if math.Abs(float64(sx-1)) > 0.0005 || math.Abs(float64(sy-1)) > 0.0005 {
				// Only while it flies: at rest the card has no transform.
				e = e.TransformOrigin(0, 0).Scale(max(sx, 0.001), max(sy, 0.001))
			}
			return e
		}
		card := place(ui.Box(c.Key("gl-card"))).Background(cardBg)
		if ex < 0.999 {
			// Zen's big shadow: centred and soft. One dropped below the
			// card read as a dark band under it on the dimmed page.
			card.Shadow(0, 0, 18, 0, ui.RGBA(0, 0, 0, 0.22*(1-ex)))
		}
		if sx == 1 && sy == 1 || math.Abs(float64(sx-1)) <= 0.0005 && math.Abs(float64(sy-1)) <= 0.0005 {
			// At rest the web view is laid out, not placed: a layer over
			// the page area padded to the card, the web view filling it.
			// Placed absolutely at a size, a web view lost the bottom of
			// its page on macOS. The layer takes the clicks beside the
			// card (its padding) and closes the glance, as the backdrop
			// under it does while the card flies.
			layer := ui.Box(c.Key("gl-rest")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Label("Close Glance").
				Padding(f.Y, max(g.area.W-f.X-f.W, 0), max(g.area.H-f.Y-f.H, 0), f.X)
			layer.Children(func() { ui.WebView(c, g.view).Fill().Radius(radius) })
			if layer.Clicked() && g.phase == glanceOpen {
				g.close()
			}
		} else {
			place(ui.WebView(c, g.view))
		}
		if fade < 0.999 {
			// The page fades in (and out) under the card's colour.
			place(ui.Box(c.Key("gl-veil"))).Background(withAlpha(cardBg, 1-fade))
		}
		if btn > 0.01 {
			g.drawButtons(c, th, btn, rest)
		}
	})

	// The motion ends: a closed glance goes, an expanded one becomes the page.
	switch {
	case g.phase == glanceClosing && (reduce || (p <= 0.001 && el.Settled(pKey))):
		g.win.Update(func() {
			if g.phase == glanceClosing && g.gen == gen {
				g.drop()
			}
		})
	case g.phase == glanceExpanding && (reduce || (ex >= 0.999 && el.Settled(eKey))):
		g.win.Update(func() {
			if g.phase == glanceExpanding && g.gen == gen {
				g.finishExpand()
			}
		})
	}
}

// drawButtons draws the column of round buttons beside the card: close,
// expand, copy link. They slide in from the card once it has opened.
func (g *glance) drawButtons(c *ui.Context, th *ui.Theme, shown float32, card glanceRect) {
	bg, hover, fg := ui.RGB(28, 28, 28), ui.RGB(41, 41, 41), ui.RGB(255, 255, 255)
	if th.Dark {
		bg, hover, fg = ui.RGB(226, 226, 226), ui.RGB(204, 204, 204), ui.RGB(28, 28, 28)
	}
	x := card.X + card.W + 12
	inside := x+glanceButton > g.area.W
	if inside {
		x = card.X + card.W - glanceButton - 10 // no room beside: over the card's corner
	}
	col := ui.Column(c.Key("gl-buttons")).Absolute().Left(x).Top(card.Y+12).Gap(10).
		Translate(-20*(1-shown), 0).Opacity(shown)
	col.Children(func() {
		copied := time.Since(g.copied) < 1500*time.Millisecond
		copyLabel := "Copy Link"
		if copied {
			copyLabel = "Copied"
		}
		expandLabel := "Open"
		if g.a.policy != nil && g.a.policy.Navigate(g.url, true) != navigation.Internal {
			expandLabel = "Open in Browser"
		}
		items := []struct {
			key, label string
			icon       *ui.SVG
			do         func()
		}{
			{"close", "Close", glanceIconClose, g.close},
			{"expand", expandLabel, glanceIconExpand, g.expandOrOpen},
			{"copy", copyLabel, glanceIconLink, g.copyLink},
		}
		for _, it := range items {
			b := ui.Box(c.Key("gl-b-"+it.key)).Size(glanceButton, glanceButton).Radius(glanceButton/2).Center().
				Role(ui.RoleButton).Label(it.label).Tooltip(it.label).Shadow(0, 0, 12, 1, ui.RGBA(0, 0, 0, 0.07))
			s := float32(1)
			switch {
			case b.Pressed():
				s = 0.98
				b.Background(hover)
			case b.Hovered():
				s = 1.02
				b.Background(hover)
			default:
				b.Background(bg)
			}
			b.Scale(s, s)
			icon := it.icon
			if it.key == "copy" && copied {
				icon = glanceIconCheck
			}
			b.Children(func() { ui.Icon(c, icon).FontSize(16).TextColor(fg) })
			if b.Clicked() {
				it.do()
			}
		}
	})
}

// expandOrOpen expands an app page into the window and sends a page that
// leaves the app to the browser. Main thread.
func (g *glance) expandOrOpen() {
	if g.a.policy != nil && g.a.policy.Navigate(g.url, true) != navigation.Internal {
		g.a.openLink(g.url)
		g.close()
		return
	}
	g.expandNow()
}

func (g *glance) copyLink() {
	mygo.Clipboard.WriteText(g.url)
	g.copied = time.Now()
	time.AfterFunc(1600*time.Millisecond, g.win.Invalidate)
}

// Lucide icons (ISC licence).
var (
	glanceIconClose  = glanceSVG(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	glanceIconExpand = glanceSVG(`<path d="M15 3h6v6"/><path d="m21 3-7 7"/><path d="m3 21 7-7"/><path d="M9 21H3v-6"/>`)
	glanceIconLink   = glanceSVG(`<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>`)
	glanceIconCheck  = glanceSVG(`<path d="M20 6 9 17l-5-5"/>`)
)

func glanceSVG(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`))
}

// withAlpha is col at opacity a (0..1).
func withAlpha(col ui.Color, a float32) ui.Color {
	return ui.RGBA(col.R, col.G, col.B, max(0, min(1, a)))
}

// pinnedTabPage reports whether p is the page of a pinned tab.
func (a *App) pinnedTabPage(p *mygo.Page) bool {
	a.mu.Lock()
	t := a.tabs
	a.mu.Unlock()
	return t != nil && t.pinnedPage(p)
}
