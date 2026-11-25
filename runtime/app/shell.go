//go:build !mypack_lite

package app

import (
	"image/color"
	"log"
	"math"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// The "safe" window layout used for titleBarStyle hidden / overlay / glass.
//
// A web page that fills a window with a hidden title bar sits under the
// macOS traffic lights, so every site's own header collides with them. The
// window therefore shows native UI instead: a slim strip along the top
// (drag region, double click zooms) with a web view filling the rest. The
// traffic lights live in the strip and the page never goes under them. The
// strip is tinted with the page's own top color, so it reads as part of the
// site.

const (
	stripHeightHidden  = 28 // titleBarStyle "hidden"
	stripHeightOverlay = 38 // titleBarStyle "overlay" (hiddenInset) and "glass"
	tintAnimation      = 150 * time.Millisecond
	tintPollInterval   = 1500 * time.Millisecond
)

// useStrip reports whether windows use the native top strip + web view.
func (a *App) useStrip() bool {
	w := a.cfg.Window
	if w.Decorations != nil && !*w.Decorations {
		return false // frameless windows keep the injected drag region
	}
	return w.TitleBarStyle == "hidden" || w.TitleBarStyle == "overlay" || w.TitleBarStyle == "glass"
}

func (a *App) stripHeight() int {
	if s := a.cfg.Window.TitleBarStyle; s == "overlay" || s == "glass" {
		return stripHeightOverlay
	}
	return stripHeightHidden
}

// openStripWindow creates a window of native UI (strip + web view) loading
// url. Main thread.
func (a *App) openStripWindow(o mygo.WindowOptions, url string) (*mygo.Window, *mygo.Page) {
	s := a.newShell()
	o.Content = ui.View(s.viewFn)
	w := a.makeWindow(o)
	s.win = w
	p, err := s.attach(url)
	if err != nil {
		log.Printf("mypack-runtime: web view: %v", err)
		return w, nil
	}
	a.registerPage(w, p)
	return w, p
}

// shell is the native layout of one window.
type shell struct {
	a    *App
	win  *mygo.Window
	view atomic.Pointer[mygo.WebView]
	page atomic.Pointer[mygo.Page]

	// main-thread state (touched by the view and by win.Update)
	tint  ui.Color
	have  bool
	inset float32 // the web view's top inset last set (glass)

	fullscreen atomic.Bool
	focused    atomic.Bool

	// gl is the window's glance (window.glance), or nil; cover is an
	// expanded glance shown over the page until the page has loaded its
	// URL (main thread).
	gl    *glance
	cover *mygo.WebView

	mu       sync.Mutex
	lastRGB  string
	stopOnce sync.Once
	stop     chan struct{}
	kick     chan struct{}
}

func (a *App) newShell() *shell {
	s := &shell{a: a, stop: make(chan struct{}), kick: make(chan struct{}, 1)}
	s.focused.Store(true)
	// Best first guess: the configured background, the last tint, else the
	// system background (set in the view).
	if bg := strings.TrimSpace(a.cfg.Window.Background); strings.HasPrefix(bg, "#") {
		s.tint, s.have = ui.Hex(bg), true
	}
	if t := a.state.Get().Tint; t != "" {
		if c, ok := parseRGB(t); ok {
			s.tint, s.have = c, true
		}
	}
	return s
}

func parseRGB(v string) (ui.Color, bool) {
	parts := strings.Split(strings.TrimSpace(v), ",")
	if len(parts) != 3 {
		return ui.Color{}, false
	}
	var c [3]uint8
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 || n > 255 {
			return ui.Color{}, false
		}
		c[i] = uint8(n)
	}
	return ui.RGB(c[0], c[1], c[2]), true
}

func (s *shell) height() float32 {
	if s.fullscreen.Load() || !s.a.useStrip() {
		return 0 // traffic lights hide in full screen
	}
	return float32(s.a.stripHeight())
}

// glassOverPage reports whether the strip is the system's glass laid over
// the page (titleBarStyle "glass"). The glass samples the page only where
// the system draws it (macOS 26 and later); MyGo's shader glass elsewhere
// sees none of the page, and with Reduce Transparency the glass is opaque,
// so those windows keep the Seamless strip above the page.
func (a *App) glassOverPage(native, reduceTransparency bool) bool {
	return a.useStrip() && a.cfg.Window.TitleBarStyle == "glass" && native && !reduceTransparency
}

// setInset gives the web view a top inset of top points, so that the top
// of the page starts under the glass strip. Main thread.
func (s *shell) setInset(top float32) {
	if top == s.inset || s.view.Load() == nil {
		return
	}
	s.inset = top
	setWebViewTopInset(s.win, float64(top))
}

// view draws the strip and the web view. Main thread.
func (s *shell) viewFn(c *ui.Context) {
	t := c.Theme()
	target := t.Background
	if s.have {
		target = s.tint
	}
	h := s.height()
	if runtime.GOOS != "darwin" && s.a.useStrip() {
		h = max(float32(c.TitleBar().Height), 32)
		if s.fullscreen.Load() {
			h = 0
		}
	}
	if s.a.glassOverPage(nativeGlassAvailable(), c.Preferences().ReduceTransparency) {
		// The page fills the window and scrolls under the strip, which the
		// system's glass fills; the web view's inset starts the page below it.
		s.setInset(h)
		ui.Box(c).Fill().Children(func() {
			s.drawPage(c, ui.Box(c.Key("page")).Fill())
			if h > 0 {
				ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(h).AlignItems(ui.Center).Material(glass.Glass{Native: true}).DragWindow().Children(func() {
					s.drawStripSettings(c, target, h)
				})
			}
			s.a.drawDownloadNotice(c)
		})
		return
	}
	s.setInset(0)
	ui.Column(c).Fill().Gap(0).Children(func() {
		if h > 0 {
			strip := ui.Row(c).FillWidth().Height(h).Shrink(0).AlignItems(ui.Center)
			col := target
			if !c.Preferences().ReduceMotion {
				col = strip.AnimateColor("tint", target, ui.Animation{Duration: tintAnimation, Ease: ui.EaseInOut})
			}
			strip.Background(col).DragWindow().Children(func() {
				s.drawStripSettings(c, col, h)
			})
		}
		s.drawPage(c, ui.Box(c.Key("page")).Grow(1).FillWidth())
		s.a.drawDownloadNotice(c)
	})
}

// drawPage draws the web view in el, under the window's glance if it has
// one. Main thread.
func (s *shell) drawPage(c *ui.Context, el ui.Element) {
	v := s.view.Load()
	page := func() {
		if v != nil {
			ui.WebView(c, v).Fill()
		}
	}
	if s.gl == nil {
		el.Children(page)
		return
	}
	s.gl.draw(c, el, func() {
		page()
		if s.cover != nil {
			// An expanded glance stays over the page while the page
			// loads its URL, so the swap does not show.
			ui.Box(c.Key("cover")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Children(func() {
				ui.WebView(c, s.cover).Fill()
			})
		}
	})
}

// drawStripSettings lays out the native strip: room for the window
// controls, a drag region between them, and on Linux and Windows
// (where a hidden title bar hides the menu) the settings control.
// macOS keeps Settings in the menu bar, so the strip stays clear of
// the traffic lights.
//
// The control is a compact gear at the trailing end of the free band,
// just before the trailing caption inset (bar.Right). On Windows that
// is immediately left of minimize, maximize and close. Linux layouts
// that put those buttons on the left keep the gear at the trailing
// end, so it covers neither the buttons nor the site's header. A
// "Settings" label at the leading edge sits on that header (the
// YouTube logo, for example) because the strip is tinted to the page.
// The gear is the same icon as the group rail. Full screen hides the
// strip (h == 0) and the control with it.
func (s *shell) drawStripSettings(c *ui.Context, bg ui.Color, h float32) {
	bar := c.TitleBar()
	ui.Box(c).Width(max(bar.Left, 8)).Shrink(0)
	ui.Spacer(c).Grow(1)
	if runtime.GOOS != "darwin" && h >= stripSettingsSide {
		s.stripSettingsButton(c, bg)
	}
	ui.Box(c).Width(max(bar.Right, 8)).Shrink(0)
}

// stripSettingsSide is the gear's box. Visible strips are at least 32
// DIPs on Linux and Windows, so the control stays inside the caption
// band and does not spill over the page.
const stripSettingsSide = 28

// stripSettingsButton opens the same Settings window as the menu item.
// It is quiet at rest (no label, no fill) and washes on hover or
// keyboard focus. Ink follows the strip, which is the page's color,
// so a dark site stays readable on a light desktop and the reverse.
func (s *shell) stripSettingsButton(c *ui.Context, bg ui.Color) {
	rest, hot, wash, pressed := stripIconColors(bg)
	btn := ui.ButtonBase(c.Key("settings")).Size(stripSettingsSide, stripSettingsSide).Radius(8).
		Margin(0, 4, 0, 0).Center().
		Label("Settings").Tooltip("Settings…").FocusRing(false).Cursor(ui.CursorPointer)
	fg := rest
	switch {
	case btn.Pressed():
		btn.Background(pressed)
		fg = hot
	case btn.Hovered() || btn.FocusVisible():
		btn.Background(wash)
		fg = hot
	}
	btn.Children(func() {
		ui.Icon(c, settingsGlyph).FontSize(16).TextColor(fg)
	})
	if btn.Clicked() {
		s.a.openSettings()
	}
}

// stripIconColors picks a caption gear for a strip tinted bg. Light
// pages get dark ink; dark pages get light ink. The wash is a hover
// or press scrim in the same direction.
func stripIconColors(bg ui.Color) (rest, hot, wash, pressed ui.Color) {
	if stripLuma(bg) >= 0.55 {
		return ui.RGBA(28, 28, 30, 0.88), ui.RGB(28, 28, 30),
			ui.RGBA(0, 0, 0, 0.08), ui.RGBA(0, 0, 0, 0.14)
	}
	return ui.RGBA(255, 255, 255, 0.92), ui.RGB(255, 255, 255),
		ui.RGBA(255, 255, 255, 0.14), ui.RGBA(255, 255, 255, 0.22)
}

// stripLuma is the relative luminance of c, 0 for black and 1 for white.
func stripLuma(c ui.Color) float64 {
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// expand loads the glance's URL in the window's page, showing the glance
// over it until it has loaded. Main thread (glanceHost).
func (s *shell) expand(g *glance, v *mygo.WebView, url string) bool {
	p := s.page.Load()
	if p == nil {
		return false
	}
	if s.cover != nil {
		s.cover.Destroy()
	}
	s.cover = v
	var once sync.Once
	uncover := func() {
		once.Do(func() {
			s.win.Update(func() {
				if s.cover == v {
					s.cover = nil
					v.Destroy()
					if pv := p.WebView(); pv != nil {
						pv.Focus()
					}
				}
			})
		})
	}
	var off func()
	off = p.OnDidFinishLoad(func() {
		// One more frame of paint before the cover goes.
		time.AfterFunc(80*time.Millisecond, uncover)
		if off != nil {
			off()
		}
	})
	time.AfterFunc(2500*time.Millisecond, uncover) // a slow page: show it loading
	if err := p.LoadURL(url); err != nil {
		log.Printf("mypack-runtime: glance: expand: %v", err)
		uncover()
	}
	return true
}

// shownPage is the window's page (glanceHost).
func (s *shell) shownPage() *mygo.Page { return s.page.Load() }

// pageTint is the colour sampled at the top of the page (glanceHost).
func (s *shell) pageTint() (ui.Color, bool) { return s.tint, s.have }

// topInset is the strip laid over the page, glass (glanceHost).
func (s *shell) topInset() float32 { return s.inset }

// hasPage reports whether p is the window's page.
func (s *shell) hasPage(p *mygo.Page) bool { return p != nil && s.page.Load() == p }

// attach creates the web view once the window exists.
func (s *shell) attach(url string) (*mygo.Page, error) {
	a := s.a
	wo := mygo.WebViewOptions{URL: url, Page: a.pageOptions(), BackgroundColor: a.cfg.Window.Background}
	v, err := s.win.NewWebView(wo)
	if err != nil && wo.BackgroundColor != "" {
		wo.BackgroundColor = ""
		v, err = s.win.NewWebView(wo)
	}
	if err != nil {
		return nil, err
	}
	p := v.Page()
	s.page.Store(p)
	s.view.Store(v)
	if a.glanceOn() {
		s.gl = a.newGlance(s.win, s)
	}

	w := s.win
	w.OnEnterFullScreen(func() { s.fullscreen.Store(true); w.Invalidate() })
	w.OnLeaveFullScreen(func() { s.fullscreen.Store(false); w.Invalidate() })
	w.OnFocus(func() { s.focused.Store(true); s.trigger() })
	w.OnBlur(func() { s.focused.Store(false) })
	w.OnClosed(s.close)
	p.OnDidNavigate(func(string) { s.burst() })
	p.OnDOMReady(s.burst)
	p.OnDidFinishLoad(s.burst)
	p.OnThemeColorChanged(func(_ color.NRGBA, _ bool) { s.burst() })
	go s.loop()
	s.e2eHook()
	w.Invalidate()
	return p, nil
}

func (s *shell) close() { s.stopOnce.Do(func() { close(s.stop) }) }

func (s *shell) trigger() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// burst samples now and again shortly after (single-page apps paint late).
func (s *shell) burst() {
	s.trigger()
	for _, d := range []time.Duration{350 * time.Millisecond, 1200 * time.Millisecond} {
		time.AfterFunc(d, s.trigger)
	}
}

// loop samples on demand and, while the window is focused, every 1.5 s. An
// idle (unfocused, hidden) window costs nothing.
func (s *shell) loop() {
	tk := time.NewTicker(tintPollInterval)
	defer tk.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-s.kick:
		case <-tk.C:
			if !s.focused.Load() {
				continue
			}
		}
		s.sample()
	}
}

func (s *shell) sample() {
	p := s.page.Load()
	if p == nil {
		return
	}
	v, err := p.Eval(sampleJS)
	if err != nil {
		return
	}
	str, _ := v.(string)
	if str == "" {
		return
	}
	s.mu.Lock()
	same := str == s.lastRGB
	s.lastRGB = str
	s.mu.Unlock()
	if same {
		return
	}
	c, ok := parseRGB(str)
	if !ok {
		return
	}
	s.win.Update(func() { s.tint, s.have = c, true })
	s.a.state.Update(func(st *State) { st.Tint = str })
}

// sampleJS returns "r,g,b": the effective background painted at the top of
// the page (what the user sees right under the strip), else the canvas
// default (white, or dark with color-scheme:dark). <meta name=theme-color> is
// deliberately NOT used: it is meant for browser chrome and routinely
// disagrees with what the page paints (github.com landing page, excalidraw).
const sampleJS = `(function () {
  try {
    var dark = matchMedia('(prefers-color-scheme: dark)').matches;
    var cx = document.createElement('canvas').getContext('2d');
    function css(v) {
      if (!v || !cx) return null;
      cx.fillStyle = '#010203'; cx.fillStyle = v;
      var o = cx.fillStyle;
      if (o === '#010203' && !/^\s*#010203\s*$/i.test(v)) { cx.fillStyle = '#040506'; cx.fillStyle = v; if (cx.fillStyle === '#040506') return null; o = cx.fillStyle; }
      var m = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(o);
      if (m) return [parseInt(m[1], 16), parseInt(m[2], 16), parseInt(m[3], 16), 1];
      m = /^rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)(?:\s*,\s*([\d.]+))?/.exec(o);
      return m ? [+m[1], +m[2], +m[3], m[4] === undefined ? 1 : +m[4]] : null;
    }
    function out(c) { return Math.round(c[0]) + ',' + Math.round(c[1]) + ',' + Math.round(c[2]); }
    var layers = [], opaque = false, seen = [];
    function take(el) {
      if (!el || seen.indexOf(el) >= 0) return;
      seen.push(el);
      var c = css(getComputedStyle(el).backgroundColor);
      if (c && c[3] > 0) { layers.push(c); if (c[3] >= 0.99) opaque = true; }
    }
    var px = Math.max(1, innerWidth / 2);
    var el = document.elementFromPoint(px, 2) || document.body;
    if (el.tagName === 'CANVAS') {
      // Canvas apps (excalidraw, figma...) paint their own background.
      try {
        var r0 = el.getBoundingClientRect(), t = document.createElement('canvas');
        t.width = t.height = 1;
        var tc = t.getContext('2d', { willReadFrequently: true });
        tc.drawImage(el, (px - r0.left) * el.width / r0.width, (2 - r0.top) * el.height / r0.height, 1, 1, 0, 0, 1, 1);
        var d = tc.getImageData(0, 0, 1, 1).data;
        if (d[3] > 0) { layers.push([d[0], d[1], d[2], d[3] / 255]); if (d[3] >= 250) opaque = true; }
      } catch (e) {}
    }
    for (; el && el.nodeType === 1 && !opaque; el = el.parentElement) take(el);
    if (!opaque) { take(document.body); }
    if (!opaque) { take(document.documentElement); }
    var base = [255, 255, 255];
    if (!opaque) {
      var cs = getComputedStyle(document.documentElement).colorScheme || '';
      if (dark && /dark/.test(cs)) base = [24, 24, 24];
    }
    var r = base;
    for (var j = layers.length - 1; j >= 0; j--) {
      var l = layers[j], a = l[3];
      r = [l[0] * a + r[0] * (1 - a), l[1] * a + r[1] * (1 - a), l[2] * a + r[2] * (1 - a)];
    }
    return out(r);
  } catch (e) { return ''; }
})()`
