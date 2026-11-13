package kit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
)

// The magic icon: a build shown as its app icon coming to life. A frosted
// veil covers the icon; the icon's own color rises under it like a liquid,
// a live wave on its surface and bubbles in it, while the veil breaks into
// sparkles that drift off the waterline. Each finished step lifts the
// liquid with a spring and a spray of sparkles; when the build is done the
// wave settles, a shine sweeps across, the icon pops and sparkles burst
// around it.

// MagicOpts configures a MagicIcon.
type MagicOpts struct {
	Icon     []byte  // the app icon (PNG)
	Name     string  // for the placeholder when there is no icon
	Size     float32 // the icon's side
	Level    float64 // 0…1: how far the color has risen
	LandedAt time.Time
	// DoneAt is when the build finished (zero while it runs); Settled
	// shows a finished build at rest, with no outro.
	DoneAt  time.Time
	Settled bool
	Tint    ui.Color
}

const (
	sparkLife  = 1.1  // seconds a drifting sparkle lives
	sparkEvery = 0.06 // seconds between sparkles
	landDur    = 0.8  // the spray when a step lands
	outroDur   = 1.1  // the finale
	popDur2    = 0.55
)

// magicEpoch is the clock the wave and sparkles run on.
var magicEpoch = time.Now()

// MagicIcon draws the icon in a box with room around it for the sparkles.
func MagicIcon(c *ui.Context, o MagicOpts) ui.Element {
	s := o.Size
	m := s * 0.38 // room for sparkles to fly
	reduced := Reduced(c)
	done := !o.DoneAt.IsZero() || o.Settled
	// since is how long ago t was at now (forever for none, or settled).
	since := func(now, t time.Time) float64 {
		if t.IsZero() || o.Settled {
			return 1e9
		}
		return now.Sub(t).Seconds()
	}
	dt := since(time.Now(), o.DoneAt)

	outer := ui.Box(c.Key("magic")).Size(s+2*m, s+2*m).Shrink(0).AccessHidden(true)
	target := float32(min(1, max(0, o.Level)))
	if done {
		target = 1
	}
	lift := design.SpringPop
	if reduced {
		lift = ui.Animation{Duration: time.Millisecond}
	}
	lvl := outer.AnimateTo("level", target, lift)
	if !reduced && math.Abs(float64(lvl-target)) > 0.001 {
		c.After(16 * time.Millisecond) // the liquid springs up
	}

	// The icon pops when the build is done.
	scale := float32(1)
	if !reduced && dt < popDur2 {
		k := float32(dt / popDur2)
		scale = 0.94 + 0.06*easeOutBack(k)
		if k < 0.18 {
			scale = 1 - 0.06*(k/0.18)
		}
	}
	if scale != 1 {
		c.After(16 * time.Millisecond) // the pop
	}

	outer.Draw(func(pt *ui.Painter, r ui.Rect) {
		// A soft glow of the icon's color breathes behind it.
		t := pt.Now().Sub(magicEpoch).Seconds()
		breath := float32(0.5 + 0.5*math.Sin(t*2))
		if reduced {
			breath = 0.5
		}
		a := 0.18 + 0.10*breath
		if done {
			a = 0.22
		}
		pt.FillRadialGradient(r, ui.RadialGradient{Radius: s * 0.95, Stops: []ui.GradientStop{
			{At: 0, Color: design.Fade(o.Tint, a)}, {At: 1, Color: design.Fade(o.Tint, 0)}}}, 0)
	})
	outer.Children(func() {
		ic := AppIconOpts(c, o.Icon, o.Name, s, IconOpts{Shadow: true})
		ic.Absolute().Left(m).Top(m)
		if scale != 1 {
			ic.Scale(scale, scale)
		}
	})
	outer.DrawOver(func(pt *ui.Painter, r ui.Rect) {
		now := pt.Now()
		t := now.Sub(magicEpoch).Seconds()
		dt := since(now, o.DoneAt)
		landed := since(now, o.LandedAt)
		if !reduced && (!done || dt < outroDur || landed < landDur) {
			pt.AnimationFrame()
		}
		ir := ui.Rect{X: r.X + m, Y: r.Y + m, W: s, H: s}
		if scale != 1 {
			cx, cy := ir.X+s/2, ir.Y+s/2
			ir = ui.Rect{X: cx - s*scale/2, Y: cy - s*scale/2, W: s * scale, H: s * scale}
		}
		k := s / 160 // the drawing is tuned at 160 pt
		// The wave calms as the build finishes.
		amp := float32(5)
		if done {
			amp *= float32(max(0, 1-dt*2.5))
		}
		if reduced {
			amp = 0
		}
		surface := ir.Y + ir.H - (ir.H+12*k)*lvl
		wave := func(x float32) float32 {
			xx := float64((x - ir.X) / k)
			return surface + k*amp*float32(math.Sin(xx/18+t*4.2)+0.45*math.Sin(xx/9-t*2.7))
		}
		icon := roundRect(ir.X, ir.Y, ir.W, ir.H, design.AppIconRadius(ir.W))

		pt.ClipPath(icon, func() {
			if lvl < 0.999 {
				// The veil: everything above the waterline, frosted.
				veil := (&ui.Path{}).MoveTo(ir.X, ir.Y-1)
				veil.LineTo(ir.X+ir.W, ir.Y-1)
				for x := ir.X + ir.W; x >= ir.X-0.1; x -= 3 * k {
					veil.LineTo(x, wave(x))
				}
				veil.Close()
				pt.FillPath(veil, ui.RGBA(22, 22, 28, 0.72))
				pt.FillPath(veil, ui.RGBA(255, 255, 255, 0.06))
				// A shimmer glides across the veil.
				if !reduced {
					sh := float32(math.Mod(t*0.55, 1.4)) - 0.2
					pt.FillPathGradient(veil, ui.LinearGradient{Angle: 110, Stops: []ui.GradientStop{
						{At: clamp01(sh - 0.18), Color: ui.RGBA(255, 255, 255, 0)},
						{At: clamp01(sh), Color: ui.RGBA(255, 255, 255, 0.13)},
						{At: clamp01(sh + 0.18), Color: ui.RGBA(255, 255, 255, 0)}}})
				}
				// The waterline catches the light.
				line := &ui.Path{}
				for x := ir.X; x <= ir.X+ir.W+0.1; x += 3 * k {
					if x == ir.X {
						line.MoveTo(x, wave(x))
					} else {
						line.LineTo(x, wave(x))
					}
				}
				pt.StrokePath(line, 1.6*k, ui.RGBA(255, 255, 255, 0.75))
				// Bubbles rise through the color.
				if !reduced && lvl > 0.04 {
					for i := 0; i < 6; i++ {
						f := float32(math.Mod(t*0.5+float64(i)/6, 1))
						bx := ir.X + ir.W*(0.15+0.14*float32(i)) + 4*k*float32(math.Sin(t*3+float64(i)))
						by := ir.Y + ir.H - (ir.Y+ir.H-surface)*f
						pt.FillPath((&ui.Path{}).Circle(bx, by, 2.3*k), ui.RGBA(255, 255, 255, 0.5*(1-f)))
					}
				}
			}
			// The finale's shine.
			if !reduced && dt < 0.8 {
				f := float32(dt / 0.8)
				f = 1 - (1-f)*(1-f)
				x := ir.X - ir.W*0.5 + f*ir.W*2
				band := ui.Rect{X: x - ir.W*0.3, Y: ir.Y, W: ir.W * 0.6, H: ir.H}
				pt.FillGradient(band, ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
					{At: 0, Color: ui.RGBA(255, 255, 255, 0)}, {At: 0.5, Color: ui.RGBA(255, 255, 255, 0.55*(1-f))}, {At: 1, Color: ui.RGBA(255, 255, 255, 0)}}}, 0)
			}
		})
		if reduced {
			return
		}
		gold := ui.RGB(255, 211, 107)
		white := ui.RGB(255, 255, 255)
		// Sparkles drift up off the waterline while the build runs.
		if !done && lvl < 0.999 {
			n0 := int(math.Floor((t - sparkLife) / sparkEvery))
			n1 := int(math.Floor(t / sparkEvery))
			for n := n0 + 1; n <= n1; n++ {
				age := t - float64(n)*sparkEvery
				f := float32(age / sparkLife)
				x := ir.X + ir.W*(0.06+0.88*hash01(n))
				y := wave(x) - 34*k*ease3(f)
				x += 9 * k * float32(math.Sin(float64(n)*1.7+age*2.4))
				al := (1 - f)
				if f < 0.12 {
					al = f / 0.12
				}
				col := white
				if n%3 == 0 {
					col = gold
				}
				sparkle(pt, x, y, (1.6+2.4*hash01(n*7))*k, float32(age)*2.5, design.Fade(col, al*0.95))
			}
		}
		// A spray when a step lands.
		if l := landed; l < landDur && !done {
			f := float32(l / landDur)
			for i := 0; i < 14; i++ {
				x := ir.X + ir.W*(0.05+0.9*hash01(i*13+int(o.LandedAt.UnixMilli()%997)))
				y := wave(x) - (26+40*hash01(i*5))*k*ease3(f)
				x += (hash01(i*3) - 0.5) * 30 * k * f
				col := white
				if i%2 == 0 {
					col = gold
				}
				sparkle(pt, x, y, (2.5+3*hash01(i*11))*k*(1-0.4*f), f*4, design.Fade(col, 1-f))
			}
		}
		// The finale: sparkles burst out all around the icon.
		if dt < outroDur {
			f := float32(dt / outroDur)
			cx, cy := ir.X+ir.W/2, ir.Y+ir.H/2
			for i := 0; i < 26; i++ {
				ang := float64(i)/26*2*math.Pi + float64(hash01(i*17))*0.4
				d := ir.W * (0.5 + (0.32+0.25*hash01(i*29))*ease3(f))
				x, y := cx+d*float32(math.Cos(ang)), cy+d*float32(math.Sin(ang))
				col := white
				switch i % 3 {
				case 0:
					col = gold
				case 1:
					col = o.Tint
				}
				sparkle(pt, x, y, (3+3.5*hash01(i*31))*k*(1-f*0.6), f*5, design.Fade(col, 1-f*f))
			}
		}
	})
	return outer
}

// sparkle draws a four-pointed star of radius rad, turned by rot.
func sparkle(pt *ui.Painter, x, y, rad, rot float32, col ui.Color) {
	if rad <= 0.1 || col.A == 0 {
		return
	}
	p := &ui.Path{}
	for i := 0; i < 8; i++ {
		rr := rad
		if i%2 == 1 {
			rr = rad * 0.32
		}
		a := float64(rot) + float64(i)*math.Pi/4
		px, py := x+rr*float32(math.Cos(a)), y+rr*float32(math.Sin(a))
		if i == 0 {
			p.MoveTo(px, py)
		} else {
			p.LineTo(px, py)
		}
	}
	pt.FillPath(p.Close(), col)
}

func ease3(f float32) float32 {
	f = clamp01(f)
	return 1 - (1-f)*(1-f)*(1-f)
}

func clamp01(f float32) float32 { return min(1, max(0, f)) }

// hash01 is a steady pseudo-random 0…1 for n.
func hash01(n int) float32 {
	x := uint32(n)*2654435761 + 0x9e3779b9
	x ^= x >> 15
	x *= 0x85ebca6b
	x ^= x >> 13
	return float32(x&0xffff) / 65535
}
