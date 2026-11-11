package kit

import (
	"math"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
)

// MockupOpts is what the live app mockup shows: the generated app's
// window as its settings make it.
type MockupOpts struct {
	Icon     []byte
	Name     string
	Tint     ui.Color // the site's theme colour
	Seamless bool     // title bar: seamless (traffic lights over a tinted strip) or standard
	Dark     bool     // the app's appearance
	W, H     int      // window size in points (Fill: Maximized)
	Fill     bool
	Tray     bool // keeps running in the menu bar
	Height   float32
	Launched bool // spring open on arrival (a finished build)
}

// AppMockup draws the app the settings describe, live: a faint screen,
// the app's window sized relative to it, its title bar standard or
// seamless, light or dark, the site's colour and a few placeholder lines;
// a menu-bar glyph when it keeps running there. Every change morphs on a
// spring. Launched, the window springs open as it appears (the result of
// a build: the app, opened).
func AppMockup(c *ui.Context, o MockupOpts) ui.Element {
	p := design.Of(c)
	art := artFor(o.Icon)
	tint := o.Tint
	if tint.A == 0 {
		tint, _ = PlaceholderColors(o.Name)
	}
	tint.A = 255
	e := ui.Box(c).FillWidth().Height(o.Height).Role(ui.RoleImage).Label(o.Name)
	if o.Launched {
		e.Transition(ui.ElementTransition{Animation: &design.Spring, Enter: &ui.Motion{Scale: 0.82, Y: 18}})
	}
	// Springs toward the settings.
	fw, fh := float32(o.W)/1600, float32(o.H)/1000
	if o.Fill {
		fw, fh = 1, 1
	}
	b2f := func(b bool) float32 {
		if b {
			return 1
		}
		return 0
	}
	sw := e.AnimateTo("w", min(fw, 1), design.SpringSoft)
	sh := e.AnimateTo("h", min(fh, 1), design.SpringSoft)
	seam := e.AnimateTo("seam", b2f(o.Seamless), design.SpringSoft)
	dark := e.AnimateTo("dark", b2f(o.Dark), design.SpringSoft)
	tray := e.AnimateTo("tray", b2f(o.Tray), design.Spring)
	e.Draw(func(pt *ui.Painter, r ui.Rect) {
		mix := func(a, b ui.Color, k float32) ui.Color { return a.Mix(b, k) }
		// The stage IS the desktop: a soft wallpaper tinted by the site's
		// colour fills it edge to edge (no screen outline, no nested frames),
		// with a translucent menu bar along the top. The window floats on it.
		// (No Dock: the app's icon shows once on a page, in its identity
		// block; the window's title has the favicon.)
		desk := roundRect(r.X, r.Y, r.W, r.H, 16)
		base := p.Group
		// A colourless site (black, white, grey) would make a grey slab:
		// its wallpaper takes Mypack's accent instead.
		hue := tint
		if chroma(tint) < 40 {
			hue = p.Accent
		}
		hue.A = 255
		glow := float32(0.45)
		if p.Dark {
			glow = 0.6
		}
		pt.ClipPath(desk, func() {
			pt.FillGradient(r, ui.LinearGradient{
				From:  mix(base, hue, 0.22),
				To:    mix(base, hue, 0.06),
				Angle: 160,
			}, 0)
			// Two soft blooms: the hue upper left, a lighter wash lower
			// right, so the window floats over light rather than a flat fill.
			bloom := func(x, y, rad float32, col ui.Color) {
				pt.Shadow(ui.Rect{X: x, Y: y, W: 1, H: 1}, 0, 0, 0, rad, rad*0.35, col)
			}
			bloom(r.X+r.W*0.18, r.Y+r.H*0.22, r.H*0.55, hue.Alpha(glow))
			bloom(r.X+r.W*0.86, r.Y+r.H*0.92, r.H*0.5, mix(hue, ui.Hex("#ffffff"), 0.45).Alpha(glow*0.6))
			// The menu bar, with the app's glyph when it keeps running there.
			barH := float32(16)
			pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: barH}, mix(ui.RGBA(255, 255, 255, 0.45), ui.RGBA(255, 255, 255, 0.06), b2f(p.Dark)), 0)
			if tray > 0.01 {
				gx, gy := r.X+r.W-30, r.Y+4
				pt.FillPath(roundRect(gx, gy+(1-tray)*-8, 8, 8, 2), design.Fade(hue, tray))
			}
		})
		scr := ui.Rect{X: r.X + 20, Y: r.Y + 16, W: r.W - 40, H: r.H - 28}
		// The window, sized to the screen.
		ww, wh := scr.W*sw*0.94, scr.H*sh*0.86
		if o.Fill {
			ww, wh = scr.W*(0.94+0.06*sw)-4, scr.H*(0.86+0.14*sh)-14
		}
		wx, wy := scr.X+(scr.W-ww)/2, scr.Y+10+(scr.H-10-wh)/2
		rad := float32(9)
		paper := mix(ui.Hex("#ffffff"), ui.Hex("#1e2635"), dark)
		chrome := mix(ui.Hex("#eceef2"), ui.Hex("#283142"), dark)
		ink := mix(ui.RGBA(14, 23, 38, .55), ui.RGBA(238, 243, 251, .65), dark)
		line := mix(ui.RGBA(14, 40, 90, .08), ui.RGBA(255, 255, 255, .09), dark)
		win := roundRect(wx, wy, ww, wh, rad)
		pt.Shadow(ui.Rect{X: wx, Y: wy, W: ww, H: wh}, rad, 0, 8, 22, -4, ui.RGBA(11, 42, 107, 0.16))
		pt.FillPath(win, paper)
		pt.ClipPath(win, func() {
			barH := float32(24)
			// Standard: a neutral bar with the favicon and name; seamless:
			// the site's colour runs up under the traffic lights.
			std := 1 - seam
			if std > 0.01 {
				pt.Fill(ui.Rect{X: wx, Y: wy, W: ww, H: barH}, design.Fade(chrome, std), 0)
				pt.Fill(ui.Rect{X: wx, Y: wy + barH, W: ww, H: 1}, design.Fade(line, std), 0)
				span := ui.Span{Text: o.Name, Size: 9.5, Weight: 600, Color: design.Fade(ink, std)}
				tw, th := pt.MeasureText(0, span)
				iconS := float32(12)
				total := iconS + 5 + tw
				tx := wx + (ww-total)/2
				drawIcon(pt, art, o.Name, tx, wy+(barH-iconS)/2, iconS)
				pt.RichText(tx+iconS+5, wy+(barH-th)/2, tw+4, span)
			}
			// The page: the site's colour (taller when seamless), then lines.
			headTop := wy + barH*std
			headH := float32(46) + barH*seam
			pt.Fill(ui.Rect{X: wx, Y: headTop, W: ww, H: headH}, design.Fade(tint, 0.92), 0)
			pt.FillPath(roundRect(wx+16, headTop+headH-24, min(140, ww*0.4), 8, 4), ui.RGBA(255, 255, 255, 0.85))
			y := headTop + headH + 14
			for i, f := range []float32{0.72, 0.5, 0.62, 0.4, 0.56} {
				if y+7 > wy+wh-8 {
					break
				}
				_ = i
				pt.FillPath(roundRect(wx+16, y, (ww-32)*f, 7, 3.5), line)
				y += 15
			}
			// Traffic lights over either bar.
			for i, col := range []ui.Color{ui.Hex("#ff5f57"), ui.Hex("#febc2e"), ui.Hex("#28c840")} {
				pt.FillPath((&ui.Path{}).Circle(wx+12+float32(i)*10, wy+barH/2, 3.4), col)
			}
		})
		pt.StrokePath(roundRect(wx+0.5, wy+0.5, ww-1, wh-1, rad-0.5), 1, line)
	})
	return e
}

// drawIcon paints an app icon (its normalised art, or a letter tile).
func drawIcon(pt *ui.Painter, art *iconArt, name string, x, y, s float32) {
	pt.ClipPath(roundRect(x, y, s, s, s*0.2237), func() {
		if art != nil {
			bw, bh := art.x1-art.x0, art.y1-art.y0
			iw, ih := s/bw, s/bh
			pt.Image(art.bmp, ui.Rect{X: x - art.x0*iw, Y: y - art.y0*ih, W: iw, H: ih}, ui.Cover)
			return
		}
		top, bottom := PlaceholderColors(name)
		pt.FillGradient(ui.Rect{X: x, Y: y, W: s, H: s}, ui.LinearGradient{From: top, To: bottom, Angle: 180}, 0)
		l := sceneLetter(name, "")
		size := s * 0.6
		pt.RichText(x+(s-size*0.62)/2, y+s*0.14, s, ui.Span{Text: l, Size: size, Weight: 700, Color: ui.RGB(255, 255, 255)})
	})
}

var _ = math.Pi

// chroma is a colour's max−min channel spread: near 0 for greys.
func chroma(c ui.Color) int {
	hi := max(c.R, c.G, c.B)
	lo := min(c.R, c.G, c.B)
	return int(hi) - int(lo)
}
