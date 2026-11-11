package kit

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
)

// The build scene: mypack's signature moment, drawn on a canvas. The site,
// as a simplified browser window (its real icon and name, placeholder
// lines in its own color), is checked by a scan line, fed by a stream of
// runtime bits, folded into the open box (the logo's box), sealed, wrapped
// with a ribbon, and finally the finished app icon springs out of the box
// onto a shelf. Failure shakes the box; cancel leaves the window out.
// Every change has a cause; the payoff holds. Under Reduce Motion each
// stage shows its settled pose.

// Scene stages.
const (
	SceneValidate = iota
	SceneDownload
	SceneAssemble
	SceneSign
	SceneVerify
	SceneDone
)

// SceneOpts is what the scene shows.
type SceneOpts struct {
	Stage     int       // Scene*
	StageAt   time.Time // when Stage began (drives its motion)
	Progress  float64   // download fraction, or < 0
	Failed    bool
	Cancelled bool
	Icon      []byte // the app icon (PNG)
	Name      string // the site's name
	Host      string // the site's host
	Tint      ui.Color
	Height    float32
}

// The scene is drawn in a 400×300 space, scaled to fit its box.
const sceneW, sceneH = 400, 300

// BuildScene draws the scene in a box of the given height, filling the
// width of its parent.
func BuildScene(c *ui.Context, o SceneOpts) ui.Element {
	p := design.Of(c)
	h := o.Height
	if h == 0 {
		h = sceneH
	}
	reduced := Reduced(c)
	art := artFor(o.Icon)
	tint := o.Tint
	if tint.A == 0 {
		top, _ := PlaceholderColors(o.Name)
		tint = top
	}
	e := ui.Box(c).Height(h).FillWidth().Role(ui.RoleImage).Label(o.Name)
	e.Draw(func(pt *ui.Painter, r ui.Rect) {
		t := pt.Now().Sub(o.StageAt).Seconds() * float64(design.Speed())
		if reduced || o.StageAt.IsZero() {
			t = 60
		}
		s := min(r.W/sceneW, r.H/sceneH)
		ox := r.X + (r.W-sceneW*s)/2
		oy := r.Y + (r.H-sceneH*s)/2
		// The workbench: a dot grid fading out from the middle, and a soft
		// wash of the site's color behind the box.
		design.PaintDots(pt, r, p, true)
		cx, cy := r.X+r.W/2, oy+190*s
		pt.FillRadialGradient(ui.Rect{X: cx - 220*s, Y: cy - 150*s, W: 440 * s, H: 300 * s}, ui.RadialGradient{Radius: 220 * s,
			Stops: []ui.GradientStop{{Color: design.Fade(tint, 0.16), At: 0}, {Color: tint.Alpha(0), At: 1}}}, 0)
		moving := false
		pt.Transform(ui.Scaling(s, s).Then(ui.Translation(ox, oy)), 0, 0, func() {
			moving = drawScene(pt, p, o, art, tint, t, reduced)
		})
		if moving && !reduced {
			pt.AnimationFrame()
		}
	})
	return e
}

// sceneColors are the materials of the scene for an appearance.
type sceneColors struct {
	boxFront, boxSide, boxInside, flap, flapEdge, paper, ink, line, shadow ui.Color
}

func materials(p *design.Palette) sceneColors {
	if p.Dark {
		return sceneColors{
			boxFront: ui.Hex("#2d3542"), boxSide: ui.Hex("#252c37"), boxInside: ui.Hex("#151a21"),
			flap: ui.Hex("#36404f"), flapEdge: ui.Hex("#465265"), paper: design.Ink[4], ink: design.Ink[11],
			line: ui.RGBA(255, 255, 255, 0.1), shadow: ui.RGBA(0, 0, 0, 0.5),
		}
	}
	return sceneColors{
		boxFront: ui.Hex("#e9eef6"), boxSide: ui.Hex("#dbe3ee"), boxInside: ui.Hex("#b9c6d8"),
		flap: ui.Hex("#f3f6fb"), flapEdge: ui.Hex("#cfd9e6"), paper: ui.Hex("#ffffff"), ink: design.Paper[11],
		line: ui.RGBA(27, 27, 25, 0.09), shadow: ui.RGBA(30, 40, 60, 0.18),
	}
}

// Box geometry (scene space).
const (
	boxCX   = sceneW / 2
	boxW    = 168
	boxH    = 92
	boxTop  = 186 // the mouth
	flapLen = boxW / 2
	flapT   = 10
	winW    = 224
	winH    = 138
	winTop  = 14
	shelfY  = 104
)

func f(v float64) float32 { return float32(v) }

// drawScene paints the scene at stage time t (seconds); it reports whether
// anything is still moving.
func drawScene(pt *ui.Painter, p *design.Palette, o SceneOpts, art *iconArt, tint ui.Color, t float64, reduced bool) bool {
	m := materials(p)
	moving := false
	stage := o.Stage

	// Flaps: open while things go in; closed from assembling's end until
	// the app springs out.
	flapOpen := 1.0
	switch {
	case stage == SceneAssemble:
		flapOpen = 1 - EaseInOutCubic(Seg(t, 0.85, 1.35))
		moving = moving || t < 1.4
	case stage == SceneSign || stage == SceneVerify:
		flapOpen = 0
	case stage == SceneDone:
		flapOpen = EaseOutBack(Seg(t, 0, 0.35), 1.2)
		moving = moving || t < 2.2
	}
	if o.Failed {
		flapOpen = 0.55
	}
	shake := 0.0
	if o.Failed {
		shake = Shake(t)
		moving = moving || t < 0.5
	}

	// The ground shadow under the box.
	pt.Shadow(ui.Rect{X: boxCX - boxW/2 + 6, Y: boxTop + boxH - 6, W: boxW - 12, H: 8}, 4, 0, 6, 14, 0, m.shadow)

	pt.Transform(ui.Translation(f(shake), 0), 0, 0, func() {
		// The inside back wall, seen above the front while open.
		back := f(14 * flapOpen)
		pt.FillPath(roundRect(boxCX-boxW/2+6, boxTop-back, boxW-12, back+12, 3), m.boxInside)

		// The window: above the box until assembling folds it in.
		switch {
		case stage <= SceneDownload || o.Cancelled || (o.Failed && stage <= SceneAssemble):
			alpha := 1.0
			if o.Cancelled {
				alpha = 0.55
			}
			drawWindow(pt, p, m, o, art, tint, boxCX-winW/2, winTop, 1, alpha)
			if stage == SceneValidate && !o.Failed && !o.Cancelled {
				// The scan line sweeps down the page, again and again.
				k := math.Mod(t, 1.6) / 1.6
				y := winTop + 26 + f(EaseInOutCubic(k))*(winH-32)
				pt.ClipPath(roundRect(boxCX-winW/2, winTop, winW, winH, 10), func() {
					pt.FillGradient(ui.Rect{X: boxCX - winW/2, Y: y - 22, W: winW, H: 22}, ui.LinearGradient{Angle: 180,
						Stops: []ui.GradientStop{{Color: p.Accent.Alpha(0), At: 0}, {Color: p.Accent.Alpha(0.16), At: 1}}}, 0)
					pt.Fill(ui.Rect{X: boxCX - winW/2, Y: y - 1, W: winW, H: 2}, p.Accent.Alpha(0.85), 1)
				})
				moving = true
			}
			if stage == SceneDownload && !o.Failed && !o.Cancelled && !reduced {
				drawStream(pt, p, tint, t, o.Progress)
				moving = true
			}
		case stage == SceneAssemble:
			// Fold the window down into the box.
			k := EaseInOutCubic(Seg(t, 0, 0.9))
			sc := Lerp(1, 0.3, k)
			x := Lerp(boxCX-winW/2, boxCX-winW*sc/2, k)
			y := Lerp(winTop, boxTop-winH*sc*0.35, k)
			if k < 1 {
				drawWindow(pt, p, m, o, art, tint, f(x), f(y), f(sc), 1)
			}
		}

		// The app springs out of the box onto the shelf.
		if stage == SceneDone && !o.Failed {
			drawPayoff(pt, p, m, art, o.Name, t)
		}

		// The right side, receding, for depth.
		side := (&ui.Path{}).MoveTo(boxCX+boxW/2, boxTop).LineTo(boxCX+boxW/2+16, boxTop-9).
			LineTo(boxCX+boxW/2+16, boxTop+boxH-9).LineTo(boxCX+boxW/2, boxTop+boxH).Close()
		pt.FillPath(side, m.boxSide)
		pt.StrokePath(side, 1, m.flapEdge)
		// The front of the box, then its flaps.
		front := roundRect(boxCX-boxW/2, boxTop, boxW, boxH, 0)
		pt.FillPathGradient(front, ui.LinearGradient{From: m.boxFront, To: m.boxSide, Angle: 180})
		pt.StrokePath(front, 1, m.flapEdge)
		// A soft fold line across the front.
		pt.Fill(ui.Rect{X: boxCX - boxW/2, Y: boxTop + 0.5, W: boxW, H: 1}, ui.RGBA(255, 255, 255, 0.5), 0)
		drawFlap(pt, m, boxCX-boxW/2, boxTop, 1, flapOpen)
		drawFlap(pt, m, boxCX+boxW/2, boxTop, -1, flapOpen)

		// Seal and signature.
		if stage >= SceneSign && !o.Failed {
			ts := t
			if stage > SceneSign {
				ts = 60
			}
			moving = drawSeal(pt, p, ts) || moving
		}
		// Ribbon and check.
		if stage >= SceneVerify && !o.Failed {
			tv := t
			if stage > SceneVerify {
				tv = 60
			}
			moving = drawRibbon(pt, p, tv) || moving
		}
		if o.Failed {
			// A small alert tag on the box.
			cx, cy := float32(boxCX), float32(boxTop+boxH/2)
			pt.FillPath((&ui.Path{}).Circle(cx, cy, 15), p.Danger)
			pt.Fill(ui.Rect{X: cx - 1.5, Y: cy - 8, W: 3, H: 10}, ui.RGB(255, 255, 255), 1.5)
			pt.Fill(ui.Rect{X: cx - 1.5, Y: cy + 4, W: 3, H: 3}, ui.RGB(255, 255, 255), 1.5)
		}
	})
	return moving
}

// drawWindow paints the simplified site at (x, y) scaled by sc.
func drawWindow(pt *ui.Painter, p *design.Palette, m sceneColors, o SceneOpts, art *iconArt, tint ui.Color, x, y, sc float32, alpha float64) {
	a := f(alpha)
	pt.Transform(ui.Scaling(sc, sc).Then(ui.Translation(x, y)), 0, 0, func() {
		body := roundRect(0, 0, winW, winH, 10)
		pt.Shadow(ui.Rect{W: winW, H: winH}, 10, 0, 10, 26, -4, design.Fade(m.shadow, a))
		pt.FillPath(body, design.Fade(m.paper, a))
		pt.ClipPath(body, func() {
			// Title bar: quiet dots, the site's icon and name.
			pt.Fill(ui.Rect{W: winW, H: 24}, design.Fade(m.line, a*0.6), 0)
			for i := 0; i < 3; i++ {
				pt.FillPath((&ui.Path{}).Circle(12+float32(i)*10, 12, 3.2), design.Fade(m.ink, a*0.18))
			}
			ix := float32(52)
			if art != nil {
				pt.ClipPath(roundRect(ix, 6, 12, 12, 3), func() { pt.Image(art.bmp, ui.Rect{X: ix, Y: 6, W: 12, H: 12}, ui.Cover) })
				ix += 17
			}
			pt.RichText(ix, 6, winW-ix-10, ui.Span{Text: o.Name, Size: 9.5, Weight: 600, Color: design.Fade(m.ink, a*0.85)})
			// The page: a band in the site's color, then placeholder lines.
			pt.Fill(ui.Rect{X: 12, Y: 34, W: winW - 24, H: 40}, design.Fade(tint, a*0.9), 6)
			pt.Fill(ui.Rect{X: 22, Y: 46, W: 70, H: 6}, ui.RGBA(255, 255, 255, uint8a(0.85*float64(a))), 3)
			pt.Fill(ui.Rect{X: 22, Y: 58, W: 44, H: 5}, ui.RGBA(255, 255, 255, uint8a(0.55*float64(a))), 2.5)
			for i, w := range []float32{150, 120, 136} {
				pt.Fill(ui.Rect{X: 12, Y: 86 + float32(i)*11, W: w, H: 5}, design.Fade(m.line, a*1.6), 2.5)
			}
			for i := 0; i < 3; i++ {
				pt.Fill(ui.Rect{X: 12 + float32(i)*72, Y: 122, W: 64, H: 18}, design.Fade(m.line, a), 4)
			}
		})
		pt.StrokePath(body, 0.75, design.Fade(m.line, a*1.4))
	})
}

func uint8a(a float64) float32 { return float32(Clamp01(a)) }

// drawFlap paints a flap hinged at (hx, hy): dir 1 for the left flap, -1
// for the right. open 1 splays it up and outward, 0 lays it flat.
func drawFlap(pt *ui.Painter, m sceneColors, hx, hy float32, dir float32, open float64) {
	angle := -dir * f(150*open)
	pt.Transform(ui.Rotation(angle).Then(ui.Translation(hx, hy)), 0, 0, func() {
		x := float32(0)
		if dir < 0 {
			x = -flapLen
		}
		r := ui.Rect{X: x, Y: -flapT, W: flapLen, H: flapT}
		pt.FillPath(roundRect(r.X, r.Y, r.W, r.H, 2), m.flap)
		pt.StrokePath(roundRect(r.X, r.Y, r.W, r.H, 2), 1, m.flapEdge)
	})
}

// drawStream paints runtime bits raining in from above the scene,
// curving past the window and pouring into the box's mouth.
func drawStream(pt *ui.Painter, p *design.Palette, tint ui.Color, t, progress float64) {
	const n = 34
	rng := rand.New(rand.NewPCG(3, 5))
	for i := 0; i < n; i++ {
		phase := rng.Float64()
		speed := 0.8 + rng.Float64()*0.5
		side := 1.0
		if i%2 == 0 {
			side = -1
		}
		spread := 0.4 + rng.Float64()*0.6
		k := math.Mod(t*speed/1.4+phase, 1)
		x0, y0 := boxCX+side*(winW/2+20+spread*40), -10.0
		x1, y1 := boxCX+side*spread*30, float64(boxTop+8)
		cx, cy := boxCX+side*(winW/2+30+spread*30), float64(boxTop-30)
		col := p.Accent
		if i%3 == 0 {
			col = tint
		}
		sz := 3 + 3*rng.Float64()
		// A short trail behind each bit.
		for tr := 0; tr < 3; tr++ {
			u := EaseInOutCubic(Clamp01(k - float64(tr)*0.025))
			x := (1-u)*(1-u)*x0 + 2*(1-u)*u*cx + u*u*x1
			y := (1-u)*(1-u)*y0 + 2*(1-u)*u*cy + u*u*y1
			a := math.Sin(k*math.Pi) * (1 - float64(tr)*0.33)
			z := sz * (1 - float64(tr)*0.22)
			pt.Fill(ui.Rect{X: f(x - z/2), Y: f(y - z/2), W: f(z), H: f(z)}, design.Fade(col, f(a)), f(z*0.3))
		}
	}
	// A glow at the mouth that brightens with progress.
	g := 0.25
	if progress >= 0 {
		g = 0.18 + 0.4*progress
	}
	pt.FillRadialGradient(ui.Rect{X: boxCX - 100, Y: boxTop - 40, W: 200, H: 70}, ui.RadialGradient{Radius: 100,
		Stops: []ui.GradientStop{{Color: p.Accent.Alpha(float32(g)), At: 0}, {Color: p.Accent.Alpha(0), At: 1}}}, 0)
}

// drawSeal stamps the seal onto the box and draws the signature.
func drawSeal(pt *ui.Painter, p *design.Palette, t float64) bool {
	cx, cy := float32(boxCX+42), float32(boxTop+boxH/2+2)
	k := Seg(t, 0, 0.42)
	sc := f(Lerp(1.9, 1, EaseOutCubic(k)))
	a := f(EaseOutCubic(k))
	if k < 1 {
		pt.Shadow(ui.Rect{X: cx - 20*sc, Y: cy - 20*sc + 6*(sc-1)*8, W: 40 * sc, H: 40 * sc}, 20*sc, 0, 4, 14, 0, ui.RGBA(0, 0, 0, uint8a(0.25*float64(a))))
	}
	// Dust ring at impact.
	if d := Seg(t, 0.42, 0.95); d > 0 && d < 1 {
		pt.StrokePath((&ui.Path{}).Circle(cx, cy, f(20+22*EaseOutCubic(d))), 1.5, p.Accent.Alpha(f(0.5*(1-d))))
	}
	seal := (&ui.Path{}).Circle(cx, cy, 19*sc)
	pt.FillPath(seal, design.Fade(p.Accent, a))
	pt.StrokePath((&ui.Path{}).Circle(cx, cy, 14*sc), 1.2, ui.RGBA(255, 255, 255, uint8a(0.55*float64(a))))
	// A four-point spark pressed into the wax.
	if a > 0.4 {
		r := 7 * sc
		star := (&ui.Path{}).MoveTo(cx, cy-r).QuadTo(cx, cy, cx+r, cy).QuadTo(cx, cy, cx, cy+r).QuadTo(cx, cy, cx-r, cy).QuadTo(cx, cy, cx, cy-r).Close()
		pt.FillPath(star, ui.RGBA(255, 255, 255, uint8a(float64(a))))
	}
	// The signature: one confident stroke across the front.
	if s := Seg(t, 0.5, 1.3); s > 0 {
		x0, y0 := float32(boxCX-boxW/2+18), float32(boxTop+boxH-26)
		sig := (&ui.Path{}).MoveTo(x0, y0).
			CubeTo(x0+10, y0-14, x0+18, y0-14, x0+22, y0-2).
			CubeTo(x0+25, y0+8, x0+30, y0+4, x0+36, y0-6).
			CubeTo(x0+42, y0-14, x0+48, y0+2, x0+56, y0-2).
			CubeTo(x0+64, y0-6, x0+70, y0+2, x0+84, y0-4)
		pt.StrokePath(sig.Trim(0, f(EaseInOutCubic(s))), 1.8, design.Fade(p.Text, 0.55))
	}
	return t < 1.4
}

// drawRibbon wraps a band around the box and pops a check on it.
func drawRibbon(pt *ui.Painter, p *design.Palette, t float64) bool {
	k := EaseInOutCubic(Seg(t, 0, 0.7))
	y := float32(boxTop + 30)
	w := f(float64(boxW) * k)
	pt.Fill(ui.Rect{X: boxCX - boxW/2, Y: y, W: w, H: 9}, design.Fade(p.Success, 0.9), 0)
	pt.Fill(ui.Rect{X: boxCX - boxW/2, Y: y + 1, W: w, H: 1}, ui.RGBA(255, 255, 255, 0.4), 0)
	if c := Seg(t, 0.65, 1.15); c > 0 {
		sc := f(EaseOutBack(c, 1.4))
		cx, cy := float32(boxCX-48), y+4.5
		pt.FillPath((&ui.Path{}).Circle(cx, cy, 11*sc), p.Success)
		chk := (&ui.Path{}).MoveTo(cx-4.5*sc, cy).LineTo(cx-1*sc, cy+3.5*sc).LineTo(cx+5*sc, cy-3.5*sc)
		pt.StrokePath(chk, 2, ui.RGB(255, 255, 255))
	}
	return t < 1.2
}

// drawPayoff springs the app icon out of the box onto the shelf, with a
// burst of sparks, and holds it there.
func drawPayoff(pt *ui.Painter, p *design.Palette, m sceneColors, art *iconArt, name string, t float64) {
	const size = 84
	// A glow blooms behind where the icon lands, and its shadow falls on
	// the box below it.
	sa := f(EaseOutCubic(Seg(t, 0.2, 0.8)))
	pt.FillRadialGradient(ui.Rect{X: boxCX - 130, Y: shelfY - 36 - 130, W: 260, H: 260}, ui.RadialGradient{Radius: 130,
		Stops: []ui.GradientStop{{Color: design.Fade(p.Accent, 0.13*sa), At: 0}, {Color: p.Accent.Alpha(0), At: 1}}}, 0)
	k := SpringAt(t-0.15, 0.75, 0.38)
	y := Lerp(boxTop+10, shelfY-size/2+6, k)
	sc := Lerp(0.35, 1, Clamp01(k))
	s := f(size * sc)
	x := f(boxCX) - s/2
	yy := f(y) - s/2
	if t > 0.15 {
		pt.Shadow(ui.Rect{X: x, Y: yy, W: s, H: s}, s*0.2237, 0, s*0.06, s*0.16, 0, design.Fade(m.shadow, 1.4))
		r := s * 0.2237
		pt.ClipPath(roundRect(x, yy, s, s, r), func() {
			if art != nil {
				bw, bh := art.x1-art.x0, art.y1-art.y0
				iw, ih := s/bw, s/bh
				pt.Image(art.bmp, ui.Rect{X: x - art.x0*iw, Y: yy - art.y0*ih, W: iw, H: ih}, ui.Cover)
			} else {
				top, bottom := PlaceholderColors(name)
				pt.FillGradient(ui.Rect{X: x, Y: yy, W: s, H: s}, ui.LinearGradient{From: top, To: bottom, Angle: 180}, 0)
			}
		})
	}
	// Sparks burst as it clears the box.
	if b := Seg(t, 0.35, 1.5); b > 0 && b < 1 {
		rng := rand.New(rand.NewPCG(9, 2))
		for i := 0; i < 18; i++ {
			ang := (float64(i)/18 + rng.Float64()*0.04) * 2 * math.Pi
			d := 50 + 46*EaseOutCubic(b) + rng.Float64()*12
			px := boxCX + math.Cos(ang)*d
			py := shelfY + math.Sin(ang)*d*0.75
			col := p.Accent
			if i%3 == 1 {
				col = p.Success
			} else if i%3 == 2 {
				col = design.Amber[1]
			}
			r := f(2.2 * (1 - b))
			pt.FillPath((&ui.Path{}).Circle(f(px), f(py), r+0.6), design.Fade(col, f(1-b)))
		}
	}
}

// roundRect is a rounded rectangle path in scene space.
func roundRect(x, y, w, h, r float32) *ui.Path {
	var p ui.Path
	if r <= 0 {
		p.MoveTo(x, y).LineTo(x+w, y).LineTo(x+w, y+h).LineTo(x, y+h).Close()
		return &p
	}
	r = min(r, w/2, h/2)
	k := r * 0.552
	p.MoveTo(x+r, y).LineTo(x+w-r, y).CubeTo(x+w-r+k, y, x+w, y+r-k, x+w, y+r).
		LineTo(x+w, y+h-r).CubeTo(x+w, y+h-r+k, x+w-r+k, y+h, x+w-r, y+h).
		LineTo(x+r, y+h).CubeTo(x+r-k, y+h, x, y+h-r+k, x, y+h-r).
		LineTo(x, y+r).CubeTo(x, y+r-k, x+r-k, y, x+r, y).Close()
	return &p
}

// RoundRectPath is roundRect for screens drawing their own chrome.
func RoundRectPath(x, y, w, h, r float32) *ui.Path { return roundRect(x, y, w, h, r) }
