//go:build !mypack_lite

package app

import (
	"image"
	"runtime"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/pkg/config"
)

func TestParseRGB(t *testing.T) {
	if c, ok := parseRGB("12,34,56"); !ok || c.R != 12 || c.G != 34 || c.B != 56 {
		t.Fatalf("got %v %v", c, ok)
	}
	for _, bad := range []string{"", "1,2", "1,2,300", "a,b,c", "1,2,3,4"} {
		if _, ok := parseRGB(bad); ok {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestUseStrip(t *testing.T) {
	f := false
	cases := []struct {
		style string
		deco  *bool
		strip bool
		h     int
	}{
		{"default", nil, false, 28}, {"hidden", nil, true, 28}, {"overlay", nil, true, 38}, {"glass", nil, true, 38},
		{"glass", &f, false, 38},
		{"overlay", &f, false, 38}, // frameless keeps the injected drag region
	}
	for _, c := range cases {
		a := &App{cfg: &config.Config{}}
		a.cfg.Window.TitleBarStyle, a.cfg.Window.Decorations = c.style, c.deco
		if a.useStrip() != c.strip || a.stripHeight() != c.h {
			t.Errorf("%s deco=%v: strip=%v h=%d", c.style, c.deco, a.useStrip(), a.stripHeight())
		}
		if got := a.framelessBar(); got != (c.deco != nil) {
			t.Errorf("framelessBar %v", got)
		}
	}
}

func TestGlassOverPage(t *testing.T) {
	f := false
	cases := []struct {
		style          string
		deco           *bool
		native, reduce bool
		want           bool
	}{
		{"glass", nil, true, false, true},
		{"glass", nil, false, false, false}, // no system glass: the Seamless strip
		{"glass", nil, true, true, false},   // Reduce Transparency: the Seamless strip
		{"glass", &f, true, false, false},   // frameless: no strip at all
		{"overlay", nil, true, false, false},
		{"default", nil, true, false, false},
	}
	for _, c := range cases {
		a := &App{cfg: &config.Config{}}
		a.cfg.Window.TitleBarStyle, a.cfg.Window.Decorations = c.style, c.deco
		if got := a.glassOverPage(c.native, c.reduce); got != c.want {
			t.Errorf("%s deco=%v native=%v reduce=%v: got %v", c.style, c.deco, c.native, c.reduce, got)
		}
	}
}

func TestStripIconColors(t *testing.T) {
	// YouTube's dark header, and a light page.
	darkRest, _, _, _ := stripIconColors(ui.RGB(15, 15, 15))
	if darkRest.R < 200 {
		t.Fatalf("dark strip wants a light gear, got %+v", darkRest)
	}
	lightRest, _, _, _ := stripIconColors(ui.RGB(255, 255, 255))
	if lightRest.R > 80 {
		t.Fatalf("light strip wants a dark gear, got %+v", lightRest)
	}
	if stripLuma(ui.RGB(0, 0, 0)) > 0.01 || stripLuma(ui.RGB(255, 255, 255)) < 0.99 {
		t.Fatal("luminance endpoints")
	}
}

// TestSeamlessStripSettings is the caption control of a seamless
// (overlay) window. Linux and Windows have no menu bar once the title
// bar is hidden, so Settings is a gear beside the caption buttons.
// macOS leaves that to the menu bar and does not cover the traffic lights.
func TestSeamlessStripSettings(t *testing.T) {
	const (
		W     = 960
		H     = 640
		barH  = 32
		right = 138 // three Windows caption buttons
		left  = 12
	)
	s := seamlessShell(ui.RGB(15, 15, 15))
	tt := newStripTester(t, s, W, H, ui.TitleBar{Height: barH, Left: left, Right: right})
	info, ok := tt.AccessByRole(ui.RoleButton, "Settings")
	if runtime.GOOS == "darwin" {
		if ok {
			t.Fatal("macOS seamless strip should leave Settings to the menu bar")
		}
		return
	}
	if !ok {
		t.Fatal("missing Settings button")
	}
	if info.Description != "Settings…" {
		t.Fatalf("tooltip %q", info.Description)
	}
	if !info.Focusable {
		t.Fatal("settings control should take keyboard focus")
	}
	// The word is the accessible name only, not a second painted label.
	n := 0
	for _, text := range tt.Texts() {
		if text == "Settings" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("Settings texts = %v", tt.Texts())
	}
	b := info.Bounds
	if b.X < left {
		t.Fatalf("button covers the leading caption inset: %+v", b)
	}
	if b.X+b.W > float32(W)-right+0.5 {
		t.Fatalf("button covers the caption buttons: %+v", b)
	}
	gap := float32(W) - right - (b.X + b.W)
	if gap < 0 || gap > 16 {
		t.Fatalf("button should sit just left of the caption buttons, gap %v bounds %+v", gap, b)
	}
	if b.Y < 0 || b.Y+b.H > float32(barH)+0.5 {
		t.Fatalf("button outside the strip: %+v", b)
	}
	if light := brightPixels(tt.Image(), b, 180); light < 8 {
		t.Fatalf("gear is not visible on a dark strip (%d light pixels)", light)
	}

	// Caption buttons on the leading edge (some Linux layouts): the gear
	// stays at the trailing end, clear of both the buttons and the site.
	lead := newStripTester(t, seamlessShell(ui.RGB(255, 255, 255)), W, H, ui.TitleBar{Height: barH, Left: 160, Right: 8})
	leadInfo, ok := lead.AccessByRole(ui.RoleButton, "Settings")
	if !ok {
		t.Fatal("missing Settings button when caption buttons lead")
	}
	lb := leadInfo.Bounds
	if lb.X < 160 {
		t.Fatalf("button covers leading caption buttons: %+v", lb)
	}
	if lb.X < float32(W)/2 {
		t.Fatalf("button still on the site's side of the strip: %+v", lb)
	}
	if dark := darkPixels(lead.Image(), lb, 120); dark < 8 {
		t.Fatalf("gear is not visible on a light strip (%d dark pixels)", dark)
	}

	// Full screen hides the traffic lights and the caption buttons, and
	// the strip with them. The gear must not linger over the page.
	full := seamlessShell(ui.RGB(15, 15, 15))
	full.fullscreen.Store(true)
	ft := newStripTester(t, full, W, H, ui.TitleBar{Height: barH, Left: left, Right: right})
	if _, ok := ft.AccessByRole(ui.RoleButton, "Settings"); ok {
		t.Fatal("fullscreen should hide the strip settings control")
	}
}

func seamlessShell(tint ui.Color) *shell {
	cfg := &config.Config{}
	cfg.Window.TitleBarStyle = "overlay"
	s := &shell{a: &App{cfg: cfg}}
	s.focused.Store(true)
	s.tint, s.have = tint, true
	return s
}

func newStripTester(t *testing.T, s *shell, w, h int, bar ui.TitleBar) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(s.viewFn, w, h)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.SetTitleBar(bar)
	return tt
}

func brightPixels(img *image.RGBA, b ui.Rect, min uint8) int {
	return countPixels(img, b, func(c colorRGBA) bool { return c.r >= min && c.g >= min && c.b >= min && c.a > 200 })
}

func darkPixels(img *image.RGBA, b ui.Rect, max uint8) int {
	return countPixels(img, b, func(c colorRGBA) bool { return c.r <= max && c.g <= max && c.b <= max && c.a > 200 })
}

type colorRGBA struct{ r, g, b, a uint8 }

func countPixels(img *image.RGBA, b ui.Rect, match func(colorRGBA) bool) int {
	if img == nil {
		return 0
	}
	x0, y0 := int(b.X), int(b.Y)
	x1, y1 := int(b.X+b.W), int(b.Y+b.H)
	n := 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
				continue
			}
			c := img.RGBAAt(x, y)
			if match(colorRGBA{c.R, c.G, c.B, c.A}) {
				n++
			}
		}
	}
	return n
}

func TestGlassWindowOptions(t *testing.T) {
	a := &App{cfg: &config.Config{}}
	a.cfg.Window.TitleBarStyle = "glass"
	o := a.windowOptions(true, true)
	if o.TitleBarStyle != mygo.TitleBarHiddenInset || o.TrafficLightPosition == nil || o.TrafficLightPosition.Y != (stripHeightOverlay-14)/2 {
		t.Fatalf("glass windows are inset like Seamless: %+v %+v", o.TitleBarStyle, o.TrafficLightPosition)
	}
}
