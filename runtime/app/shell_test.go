package app

import (
	"testing"

	"github.com/egoist/mygo"

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

func TestGlassWindowOptions(t *testing.T) {
	a := &App{cfg: &config.Config{}}
	a.cfg.Window.TitleBarStyle = "glass"
	o := a.windowOptions(true, true)
	if o.TitleBarStyle != mygo.TitleBarHiddenInset || o.TrafficLightPosition == nil || o.TrafficLightPosition.Y != (stripHeightOverlay-14)/2 {
		t.Fatalf("glass windows are inset like Seamless: %+v %+v", o.TitleBarStyle, o.TrafficLightPosition)
	}
}
