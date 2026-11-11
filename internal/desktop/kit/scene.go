package kit

import (
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
)

// The build scene: a website becomes an app. Flat and 2D, drawn on a
// canvas, ported frame for frame from the reference page
// (internal/desktop/assets/scene/scene.html):
//
//   - the site, as a browser window (traffic lights, an address bar whose
//     fill shows the real download progress, its colour and a few lines),
//     floats and is scanned while it is checked;
//   - assembling, the chrome fades and the window morphs into the app's
//     icon (224×150 r16 → 92×92 r22) as its fill turns the site's colour,
//     the favicon growing from the address bar to the icon's middle;
//   - signing, a verified badge springs onto its corner with a ring pulse;
//     verifying, one light sweep crosses it;
//   - done, a flat Dock slides up, the icon shrinks into its empty slot,
//     bounces twice as macOS launches do and its running dot appears.
//
// Failure shakes it back into the window; cancel reverts it, dimmed. The
// labels stay native, under the scene. Springs only, with the reference's
// constants; Reduce Motion shows each state at once; no frames when idle.

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
	StageAt   time.Time // when Stage began; zero for an old result, shown settled
	Progress  float64   // download fraction, or < 0
	Failed    bool
	Cancelled bool
	Icon      []byte // the app icon (PNG)
	Name      string // the site's name
	Host      string // the site's host
	Tint      ui.Color
	Height    float32
	At        float64 // > 0: a still, Stage held at this time (seconds), simulated from the start
	// IconSlot, if set, is given a real icon element laid over the painted
	// one where it rests (the favicon as the scene starts, the app in its
	// Dock slot at the end), for an icon flying in from the screen before
	// or on to the next one.
	IconSlot func(e ui.Element, stage int)
}

// The scene is drawn in a 400×320 space, scaled to fit its box.
const sceneW, sceneH = 400, 320

// ---- springs ------------------------------------------------------------------

type sceneSpring struct{ v, x, vel, k, d float64 }

const (
	spMorph = iota
	spLoad
	spChrome
	spBadge
	spDock
	spDrop
	spRun
	spShine
	spShake
	spFade
	spCount
)

var springRest = [spCount]float64{spChrome: 1, spFade: 1}

type sceneTimer struct {
	at float64
	fn func()
}

// sceneSim is the scene's state between frames: its springs, the stage it
// has reached, its timers and the ring pulse.
type sceneSim struct {
	sp      [spCount]sceneSpring
	reached int
	timers  []sceneTimer
	ring    float64
	t       float64   // scene time, seconds
	last    time.Time // the frame before
	reduce  bool
	applied struct {
		stage    int
		state    string
		progress float64
		ok       bool
	}
	stage int
	state string
}

func newSceneSim(reduce bool) *sceneSim {
	s := &sceneSim{reduce: reduce, reached: -1}
	s.reset()
	return s
}

func (s *sceneSim) reset() {
	s.timers, s.reached, s.ring = nil, -1, 0
	for i := range s.sp {
		s.sp[i] = sceneSpring{v: springRest[i], x: springRest[i], k: 170, d: 16}
	}
}

// to sends spring n to v with stiffness k and damping d (0: 170 and 16).
func (s *sceneSim) to(n int, v, k, d float64) {
	if k == 0 {
		k = 170
	}
	if d == 0 {
		d = 16
	}
	sp := &s.sp[n]
	sp.v, sp.k, sp.d = v, k, d
}

func (s *sceneSim) later(ms float64, fn func()) {
	if s.reduce {
		fn()
		return
	}
	s.timers = append(s.timers, sceneTimer{at: s.t + ms/1000, fn: fn})
}

// step advances the springs by dt and runs the timers due; it reports
// whether anything still moves.
func (s *sceneSim) step(dt float64) bool {
	s.t += dt
	for {
		due := -1
		for i, tm := range s.timers {
			if tm.at <= s.t && (due < 0 || tm.at < s.timers[due].at) {
				due = i
			}
		}
		if due < 0 {
			break
		}
		fn := s.timers[due].fn
		s.timers = append(s.timers[:due], s.timers[due+1:]...)
		fn()
	}
	moving := len(s.timers) > 0
	for i := range s.sp {
		sp := &s.sp[i]
		if s.reduce {
			sp.x, sp.vel = sp.v, 0
			continue
		}
		sp.vel += (-sp.k*(sp.x-sp.v) - sp.d*sp.vel) * dt
		sp.x += sp.vel * dt
		if math.Abs(sp.vel) > .002 || math.Abs(sp.x-sp.v) > .002 {
			moving = true
		} else {
			sp.x, sp.vel = sp.v, 0
		}
	}
	if s.ring > 0 {
		s.ring += dt * 1.6
		if s.ring >= 1 {
			s.ring = 0
		}
	}
	return moving || s.ring > 0
}

// apply turns the build's state into motion (the reference's apply()).
func (s *sceneSim) apply(stage int, state string, progress float64) {
	restart := !s.applied.ok || (stage == SceneValidate && state == "running" && (s.stage != SceneValidate || s.state != "running" || s.reached > 0))
	s.stage, s.state = stage, state
	if restart {
		s.reset()
	}
	i := stage
	if state == "done" {
		i = SceneDone
	}
	switch state {
	case "failed":
		s.to(spMorph, 0, 0, 0)
		s.to(spChrome, 1, 0, 0)
		s.to(spBadge, 0, 0, 0)
		s.shake()
	case "cancelled":
		s.to(spMorph, 0, 0, 0)
		s.to(spChrome, 1, 0, 0)
		s.to(spLoad, 0, 0, 0)
		s.to(spBadge, 0, 0, 0)
		s.to(spFade, .6, 0, 0)
	default:
		if i == SceneDownload {
			s.to(spLoad, Clamp01(progress), 220, 24)
		}
		if i >= SceneAssemble && s.reached < SceneAssemble {
			s.to(spLoad, 1, 220, 24)
			s.to(spChrome, 0, 200, 22)
			s.later(120, func() { s.to(spMorph, 1, 120, 13) })
		}
		if i >= SceneSign && s.reached < SceneSign {
			s.later(200, func() { s.to(spBadge, 1, 320, 13); s.ring = .001 })
		}
		if i >= SceneVerify && s.reached < SceneVerify {
			s.later(150, func() { s.sp[spShine] = sceneSpring{}; s.to(spShine, 1, 30, 9) })
		}
		if i == SceneDone && s.reached < SceneDone {
			s.to(spDock, 1, 160, 18)
			s.later(520, func() { s.to(spBadge, 0, 240, 20); s.to(spDrop, 1, 110, 12) })
			s.later(1050, func() { s.bounce(); s.to(spRun, 1, 160, 14) })
		}
		s.reached = max(s.reached, i)
	}
	s.applied.stage, s.applied.state, s.applied.progress, s.applied.ok = stage, state, progress, true
}

// bounce hops the icon twice in its Dock slot, as macOS launches do.
func (s *sceneSim) bounce() {
	if s.reduce {
		return
	}
	var hop func(n int)
	hop = func(n int) {
		if n >= 2 {
			return
		}
		s.to(spShake, 0, 140, 9)
		s.sp[spShake].vel = -170
		s.later(520, func() { hop(n + 1) })
	}
	hop(0)
}

// shake knocks the window, once.
func (s *sceneSim) shake() {
	if s.reduce {
		return
	}
	for j, v := range []float64{7, -7, 5, -4, 0} {
		v := v
		s.later(float64(j)*70, func() { s.sp[spShake].x, s.sp[spShake].v, s.sp[spShake].vel = 0, 0, v*40 })
	}
}

// ---- the element -----------------------------------------------------------------

func sceneState(o SceneOpts) string {
	switch {
	case o.Failed:
		return "failed"
	case o.Cancelled:
		return "cancelled"
	case o.Stage >= SceneDone:
		return "done"
	}
	return "running"
}

// stillSim simulates the scene from its start to o.Stage at o.At seconds,
// for stills: each stage before it plays for the reference demo's length.
func stillSim(o SceneOpts) *sceneSim {
	s := newSceneSim(false)
	lengths := [...]float64{1.7, 2.0, 1.4, 0.9, 1.0, 3.2}
	const dt = 1.0 / 120
	for st := SceneValidate; st <= o.Stage; st++ {
		state := "running"
		if st == SceneDone {
			state = "done"
		}
		if st == o.Stage {
			state = sceneState(o)
		}
		d := lengths[min(st, len(lengths)-1)]
		if st == o.Stage {
			d = o.At
		}
		s.apply(st, state, 0)
		for t := 0.0; t < d; t += dt {
			if st == SceneDownload {
				pr := t / lengths[SceneDownload]
				if st == o.Stage && o.Progress >= 0 {
					pr = min(pr, o.Progress)
				}
				s.apply(st, state, pr)
			}
			s.step(dt)
		}
	}
	return s
}

// BuildScene draws the scene in a box of the given height, filling the
// width of its parent.
func BuildScene(c *ui.Context, o SceneOpts) ui.Element {
	p := design.Of(c)
	h := o.Height
	if h == 0 {
		h = 300
	}
	reduced := Reduced(c)
	art := artFor(o.Icon)
	tint := o.Tint
	if tint.A == 0 {
		top, _ := PlaceholderColors(o.Name)
		tint = top
	}
	tint.A = 255
	e := ui.Box(c).Height(h).FillWidth().Role(ui.RoleImage).Label(o.Name)
	var sim *sceneSim
	if o.At > 0 {
		sim = stillSim(o)
	} else {
		sim = *ui.Local(e, "sim", func() *sceneSim { return newSceneSim(reduced) })
		first := !sim.applied.ok
		sim.reduce = reduced || (first && o.StageAt.IsZero()) // an old result shows settled
		state := sceneState(o)
		if first || sim.applied.stage != o.Stage || sim.applied.state != state || (o.Stage == SceneDownload && o.Progress != sim.applied.progress) {
			sim.apply(o.Stage, state, o.Progress)
		}
		if first && sim.reduce {
			sim.step(0)
		}
		sim.reduce = reduced
	}
	still := o.At > 0
	details := h >= 160
	e.Draw(func(pt *ui.Painter, r ui.Rect) {
		moving := false
		if !still {
			now := pt.Now()
			dt := 0.0
			if !sim.last.IsZero() {
				dt = math.Min(.05, now.Sub(sim.last).Seconds()*float64(design.Speed()))
			}
			sim.last = now
			moving = sim.step(dt)
		}
		s := min(r.W/sceneW, r.H/sceneH)
		ox := r.X + (r.W-sceneW*s)/2
		oy := r.Y + (r.H-sceneH*s)/2
		pt.Transform(ui.Scaling(s, s).Then(ui.Translation(ox, oy)), 0, 0, func() {
			drawScene(pt, p, sim, o, art, tint, details)
		})
		idle := !sim.reduce && sim.state == "running" && (sim.stage == SceneValidate || sim.sp[spMorph].x < .5)
		switch {
		case still:
		case moving || idle:
			pt.AnimationFrame()
		default:
			sim.last = time.Time{} // idle: no frames until something changes
		}
	})
	if o.IconSlot != nil && !still {
		if g := sim.geometry(); g.iconRest {
			if b := e.Bounds(); b.W > 0 {
				s := min(b.W/sceneW, h/sceneH)
				ox, oy := (b.W-sceneW*s)/2, (h-sceneH*s)/2
				e.Children(func() {
					ic := AppIconOpts(c, o.Icon, o.Name, g.fav.W*s, IconOpts{})
					ic.Absolute().Left(ox + g.fav.X*s).Top(oy + g.fav.Y*s).PassThrough().AccessHidden(true)
					o.IconSlot(ic, o.Stage)
				})
			}
		}
	}
	return e
}

// sceneGeom is the morphing shape and the favicon in scene space.
type sceneGeom struct {
	shape    ui.Rect
	radius   float32
	fav      ui.Rect
	iconRest bool // the icon is at rest where a real element can stand in for it
}

func sceneLerp(a, b, t float64) float64 { return a + (b-a)*t }

// geometry is the reference's shape and favicon maths.
func (s *sceneSim) geometry() sceneGeom {
	m := Clamp01(s.sp[spMorph].x)
	bob := 0.0
	if !s.reduce && s.state == "running" {
		bob = math.Sin(s.t*2) * 2.5 * (1 - m)
	}
	w, h, rad := sceneLerp(224, 92, m), sceneLerp(150, 92, m), sceneLerp(16, 22, m)
	cx, cy := 200.0, sceneLerp(132, 128, m)+bob
	drop := Clamp01(s.sp[spDrop].x)
	const slotY = 236 - 4
	if drop > 0 {
		w = sceneLerp(92, 36, drop)
		h, rad = w, sceneLerp(22, 10, drop)
		cx, cy = sceneLerp(200, 222, drop), sceneLerp(128, slotY-4, drop)
	}
	cy += s.sp[spShake].x
	x, y := cx-w/2, cy-h/2
	fs := sceneLerp(12, 54, m)
	if drop > 0 {
		fs *= sceneLerp(1, 36.0/92, drop)
	}
	fx, fy := sceneLerp(x+58, cx-fs/2, m), sceneLerp(y+8, cy-fs/2, m)
	rest := (s.stage == SceneValidate && s.t < 0.9 && m == 0) ||
		(s.state == "done" && drop >= 1 && s.sp[spDrop].vel == 0 && s.sp[spShake].vel == 0 && len(s.timers) == 0)
	return sceneGeom{shape: ui.Rect{X: f(x), Y: f(y), W: f(w), H: f(h)}, radius: f(rad),
		fav: ui.Rect{X: f(fx), Y: f(fy), W: f(fs), H: f(fs)}, iconRest: rest}
}

func f(v float64) float32 { return float32(v) }

// sceneAlpha returns c at a times its own opacity.
func sceneAlpha(c ui.Color, a float64) ui.Color {
	c.A = uint8(math.Round(float64(c.A) * Clamp01(a)))
	return c
}

// drawScene paints the scene (the reference's draw()).
func drawScene(pt *ui.Painter, p *design.Palette, s *sceneSim, o SceneOpts, icon *iconArt, site ui.Color, details bool) {
	dk := p.Dark
	m := Clamp01(s.sp[spMorph].x)
	chrome := Clamp01(s.sp[spChrome].x)
	const dockY = 236

	// The halo: a soft radial light in the site's colour, and nothing else.
	// (As SVG's objectBoundingBox radius: an ellipse 55 % of the width by
	// 55 % of the height, so it fades out inside the scene's edges.)
	const hcx, hcy, hr = sceneW * .5, sceneH * .46, sceneW * .55
	pt.Transform(ui.Translation(-hcx, -hcy).Then(ui.Scaling(1, sceneH/sceneW)).Then(ui.Translation(hcx, hcy)), 0, 0, func() {
		pt.FillRadialGradient(ui.Rect{X: hcx - hr, Y: hcy - hr, W: 2 * hr, H: 2 * hr}, ui.RadialGradient{Radius: hr,
			Stops: []ui.GradientStop{{Color: sceneAlpha(site, .16), At: 0}, {Color: sceneAlpha(site, 0), At: 1}}}, 0)
	})

	// The Dock, sliding up at the end.
	if d1 := Clamp01(s.sp[spDock].x); d1 > .01 {
		ty := f(sceneLerp(70, 0, d1))
		fill, stroke, slot, ink := ui.Hex("#ffffff"), ui.RGBA(14, 40, 90, .08), ui.Hex("#dde5f1"), ui.Hex("#0e1726")
		if dk {
			fill, stroke, slot, ink = ui.Hex("#1f2633"), ui.RGBA(255, 255, 255, .08), ui.Hex("#2b3444"), ui.Hex("#eef3fb")
		}
		dock := ui.Rect{X: 92, Y: dockY - 34 + ty, W: 216, H: 60}
		pt.Shadow(dock, 22, 0, 6, 16, 0, sceneAlpha(ui.RGBA(11, 42, 107, .14), d1))
		pt.FillPath(roundRect(dock.X, dock.Y, dock.W, dock.H, 22), sceneAlpha(fill, d1))
		pt.StrokePath(roundRect(dock.X+.5, dock.Y+.5, dock.W-1, dock.H-1, 21.5), 1, sceneAlpha(stroke, d1))
		for _, x := range []float32{118, 162, 282} {
			pt.FillPath(roundRect(x-18, dockY-22+ty, 36, 36, 10), sceneAlpha(slot, d1))
		}
		if run := Clamp01(s.sp[spRun].x); run > 0 {
			pt.FillPath((&ui.Path{}).Circle(222, dockY+20+ty, f(2.4*run)), sceneAlpha(ink, .7*run*d1))
		}
	}

	g := s.geometry()
	x, y, w, h, r := g.shape.X, g.shape.Y, g.shape.W, g.shape.H, g.radius
	fade := Clamp01(s.sp[spFade].x)
	paper := ui.Hex("#ffffff")
	if dk {
		paper = ui.Hex("#1e2635")
	}
	fill := paper
	if m > 0 {
		fill = paper.Mix(site, f(math.Pow(m, .8)))
	}
	shape := roundRect(x, y, w, h, r)
	// One soft shadow lifts it.
	pt.Shadow(ui.Rect{X: x, Y: y, W: w, H: h}, r, 0, 12, 24, 0, sceneAlpha(ui.RGBA(11, 42, 107, .18), fade))
	pt.FillPath(shape, sceneAlpha(fill, fade))
	if m < .2 {
		hair := ui.RGBA(14, 40, 90, .08)
		if dk {
			hair = ui.RGBA(255, 255, 255, .08)
		}
		pt.StrokePath(roundRect(x+.5, y+.5, w-1, h-1, r-.5), 1, sceneAlpha(hair, (1-float64(m)*5)*fade))
	}

	pt.ClipPath(shape, func() {
		// The browser's chrome: traffic lights, the address bar with the
		// download's progress, and the page.
		if chrome > .01 {
			a := chrome * fade
			for i, col := range []ui.Color{ui.Hex("#ff5f57"), ui.Hex("#febc2e"), ui.Hex("#28c840")} {
				pt.FillPath((&ui.Path{}).Circle(x+15+float32(i)*11, y+14, 3.6), sceneAlpha(col, a))
			}
			bx, bw, by, bh := x+52, w-64, y+6, float32(16)
			bar := ui.Hex("#eef2f8")
			if dk {
				bar = ui.Hex("#283142")
			}
			pt.FillPath(roundRect(bx, by, bw, bh, 8), sceneAlpha(bar, a))
			if load := Clamp01(s.sp[spLoad].x); load > .005 {
				pt.FillPath(roundRect(bx, by, bw*f(load), bh, 8), sceneAlpha(site, .2*a))
				pt.FillPath(roundRect(bx+6, by+bh-2.5, max(0, (bw-12)*f(load)), 2, 1), sceneAlpha(site, a))
			}
			rule, line, ink := ui.RGBA(14, 40, 90, .06), ui.RGBA(14, 40, 90, .07), ui.RGBA(14, 23, 38, .55)
			if dk {
				rule, line, ink = ui.RGBA(255, 255, 255, .06), ui.RGBA(255, 255, 255, .08), ui.RGBA(238, 243, 251, .6)
			}
			if details {
				host := o.Host
				if host == "" {
					host = "website"
				}
				// Centred in the pill by its measured height, clipped to the
				// pill with an ellipsis.
				tx, tw := bx+22, bw-28
				span := ui.Span{Text: host, Size: 9, Color: sceneAlpha(ink, a)}
				w, lh := pt.MeasureText(0, span)
				for w > tw && len([]rune(span.Text)) > 2 {
					r := []rune(strings.TrimSuffix(span.Text, "…"))
					span.Text = string(r[:len(r)-1]) + "…"
					w, _ = pt.MeasureText(0, span)
				}
				pt.Clip(ui.Rect{X: bx, Y: by, W: bw, H: bh}, 8, func() {
					pt.RichText(tx, by+(bh-lh)/2, tw+4, span)
				})
			}
			pt.Fill(ui.Rect{X: x, Y: y + 28, W: w, H: 1}, sceneAlpha(rule, a), 0)
			pt.FillPath(roundRect(x+14, y+40, w-28, 34, 10), sceneAlpha(site, .9*a))
			if details {
				for _, l := range [][2]float32{{.82, 86}, {.6, 99}, {.7, 112}} {
					pt.FillPath(roundRect(x+14, y+l[1], (w-28)*l[0], 7, 3.5), sceneAlpha(line, a))
				}
			}
			if s.stage == SceneValidate && s.state == "running" && !s.reduce {
				ph := math.Mod(s.t*.75, 1)
				sy := y + 30 + f(ph)*(h-36)
				pt.Fill(ui.Rect{X: x, Y: sy - 18, W: w, H: 18}, sceneAlpha(site, .08*a), 0)
				pt.FillPath(roundRect(x+8, sy, w-16, 2, 1), sceneAlpha(site, a))
			}
		}

		// The favicon: small in the address bar, large in the icon. The
		// real icon, as its normalised squircle; a letter only without one.
		fv := g.fav
		if icon != nil {
			// A real icon grows to fill the whole shape, so the finished app
			// IS the site's icon (no icon nested inside a coloured tile).
			k := f(m)
			lerpf := func(a, b float32) float32 { return a + (b-a)*k }
			fv = ui.Rect{X: lerpf(fv.X, x), Y: lerpf(fv.Y, y), W: lerpf(fv.W, w), H: lerpf(fv.H, h)}
			rr := lerpf(fv.W*0.2237, g.radius)
			pt.ClipPath(roundRect(fv.X, fv.Y, fv.W, fv.H, rr), func() {
				bw, bh := icon.x1-icon.x0, icon.y1-icon.y0
				iw, ih := fv.W/bw, fv.H/bh
				pt.Image(icon.bmp, ui.Rect{X: fv.X - icon.x0*iw, Y: fv.Y - icon.y0*ih, W: iw, H: ih}, ui.Cover)
			})
		} else {
			bg := site
			if m > .5 {
				bg = ui.RGBA(255, 255, 255, .22)
			}
			pt.FillPath(roundRect(fv.X, fv.Y, fv.W, fv.H, fv.W*.26), sceneAlpha(bg, fade))
			size := fv.W * .66
			pt.RichText(fv.X+(fv.W-size*.62)/2, fv.Y+fv.H*.12, fv.W, ui.Span{Text: sceneLetter(o.Name, o.Host), Size: size, Weight: 700,
				Color: ui.RGB(255, 255, 255)})
		}

		// One light sweep across the finished icon.
		if sh := s.sp[spShine].x; sh > 0 && sh < 1 {
			sx := f(sceneLerp(float64(x)-40, float64(x+w)+40, sh))
			band := (&ui.Path{}).MoveTo(sx, y).LineTo(sx+22, y).LineTo(sx-8, y+h).LineTo(sx-30, y+h).Close()
			pt.FillPath(band, ui.RGBA(255, 255, 255, .35))
		}
	})

	// The verified badge, springing onto the corner, and its ring pulse.
	if b := s.sp[spBadge].x; b > .01 && Clamp01(s.sp[spDrop].x) < .5 {
		cx, cy := x+w-6, y+6
		rim := ui.Hex("#ffffff")
		if dk {
			rim = ui.Hex("#1e2635")
		}
		k := f(b)
		pt.FillPath((&ui.Path{}).Circle(cx, cy, 16.5*k), rim)
		pt.FillPath((&ui.Path{}).Circle(cx, cy, 13.5*k), design.LogoMid)
		chk := (&ui.Path{}).MoveTo(cx-6*k, cy+.5*k).LineTo(cx-1.8*k, cy+4.6*k).LineTo(cx+6.5*k, cy-4*k).Cap(ui.RoundCap).Join(ui.RoundJoin)
		pt.StrokePath(chk, 3*k, ui.RGB(255, 255, 255))
		if s.ring > 0 {
			pt.StrokePath((&ui.Path{}).Circle(cx, cy, f(15+s.ring*22)*k), 2, sceneAlpha(design.LogoMid, (1-s.ring)*.6))
		}
	}
}

// sceneLetter is the fallback favicon's letter.
func sceneLetter(name, host string) string {
	for _, r := range strings.TrimSpace(name + host) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "M"
}

// roundRect is a rounded rectangle path.
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
