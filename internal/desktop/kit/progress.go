package kit

import (
	"math"
	"time"

	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
)

// arc builds a circular arc path from turn a0 to a1 (0 = 12 o'clock,
// clockwise, 1 = a full turn).
func arc(cx, cy, r, a0, a1 float32) *ui.Path {
	var p ui.Path
	n := int(math.Max(8, math.Ceil(float64((a1-a0)*96))))
	for i := 0; i <= n; i++ {
		t := float64(a0 + (a1-a0)*float32(i)/float32(n))
		x := cx + r*float32(math.Sin(2*math.Pi*t))
		y := cy - r*float32(math.Cos(2*math.Pi*t))
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	return &p
}

// Spinner is the indeterminate indicator shown beside "busy" text:
// loading.dev's Loading, drawn by MyGO's loading plugin so it honours
// Reduce Motion and stops drawing when off screen. Callers always pair it with a label, so it stays hidden from
// assistive technologies rather than announcing a second, unnamed progress.
func Spinner(c *ui.Context, size float32, col ui.Color) ui.Element {
	return loading.Loading(c, loading.Options{Size: size, Color: col}).
		Shrink(0).AccessHidden(true)
}

// BuildingSpinner marks an app that is being built: loading.dev's Blocks,
// whose grid of blocks reads as something being assembled. Like Spinner it
// is decorative beside a label or a row that already says what is happening.
func BuildingSpinner(c *ui.Context, size float32, col ui.Color) ui.Element {
	return loading.Blocks(c, loading.Options{Size: size, Color: col}).
		Shrink(0).AccessHidden(true)
}

// ProgressRing draws a thin ring around a box of size: value 0..1, or
// negative for indeterminate. It is drawn over its children, so place the
// thing it surrounds inside it.
func ProgressRing(c *ui.Context, size, width float32, value float64, col, track ui.Color) ui.Element {
	e := ui.Box(c).Size(size, size).Shrink(0).Role(ui.RoleProgress)
	var a0, a1 float32
	if value < 0 {
		t := e.Loop("ring", 1100*time.Millisecond, ui.Linear)
		a0, a1 = t, t+0.28
	} else {
		v := e.AnimateTo("v", float32(min(1, value)), design.SpringSoft)
		a0, a1 = 0, max(0.004, v)
		e.Range(0, 1, value)
	}
	e.DrawOver(func(p *ui.Painter, r ui.Rect) {
		cx, cy, rad := r.X+r.W/2, r.Y+r.H/2, r.W/2-width/2
		p.StrokePath((&ui.Path{}).Circle(cx, cy, rad), width, track)
		p.StrokePath(arc(cx, cy, rad, a0, a1), width, col)
	})
	return e
}

// ProgressBar is a thin capsule bar; value < 0 is indeterminate.
func ProgressBar(c *ui.Context, value float64) ui.Element {
	p := design.Of(c)
	track := ui.Box(c).Height(6).FillWidth().Radius(3).Background(p.Fill).Clip()
	track.Children(func() {
		if value < 0 {
			bar := ui.Box(c).Height(6).WidthPercent(30).Radius(3).Background(p.Accent)
			t := bar.Loop("indeterminate", 1400*time.Millisecond, ui.CubicBezier(0.45, 0, 0.55, 1))
			bar.TranslatePercent(-100+t*430, 0)
			return
		}
		v := float32(max(0, min(1, value)))
		bar := ui.Box(c).Height(6).Radius(3).Background(p.Accent)
		bar.WidthPercent(bar.AnimateTo("w", v*100, design.SpringSoft))
	})
	return track.Role(ui.RoleProgress).Range(0, 1, max(0, value))
}

// Shimmer is a skeleton placeholder with a light sweeping across it.
func Shimmer(c *ui.Context, w, h, radius float32) ui.Element {
	p := design.Of(c)
	e := ui.Box(c).Radius(radius).Background(p.Fill).Shrink(0).AccessHidden(true)
	if w > 0 {
		e.Width(w)
	}
	if h > 0 {
		e.Height(h)
	}
	if c.Preferences().ReduceMotion {
		return e
	}
	t := e.Loop("shimmer", 1500*time.Millisecond, ui.EaseInOut)
	hi := ui.RGBA(255, 255, 255, 0.55)
	if p.Dark {
		hi = ui.RGBA(255, 255, 255, 0.09)
	}
	e.Draw(func(pt *ui.Painter, r ui.Rect) {
		band := r.W * 0.6
		x := r.X - band + (r.W+band)*t
		pt.Clip(r, radius, func() {
			pt.FillGradient(ui.Rect{X: x, Y: r.Y, W: band, H: r.H}, ui.LinearGradient{Angle: 90,
				Stops: []ui.GradientStop{{Color: hi.Alpha(0), At: 0}, {Color: hi, At: 0.5}, {Color: hi.Alpha(0), At: 1}}}, 0)
		})
	})
	return e
}
