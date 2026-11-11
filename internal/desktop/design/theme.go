package design

import (
	"time"

	"github.com/egoist/mygo/ui"
)

var (
	lightTheme = Light.theme()
	darkTheme  = Dark.theme()
	current    = &Light
)

func (p *Palette) theme() *ui.Theme {
	return &ui.Theme{
		Dark:            p.Dark,
		Background:      p.Window,
		Surface:         p.Raised,
		SurfaceHover:    p.Hover.Over(p.Raised),
		SurfacePressed:  p.Pressed.Over(p.Raised),
		Border:          p.BorderStrong,
		Text:            p.Text,
		TextMuted:       p.TextSecondary,
		Accent:          p.Accent,
		AccentHover:     p.AccentHover,
		AccentPressed:   p.AccentPressed,
		AccentText:      p.TextOnAccent,
		InactiveAccent:  p.Selected.Over(p.Window),
		Danger:          p.Danger,
		Warning:         p.Warning,
		Success:         p.Success,
		Selection:       p.Accent.Alpha(0.24),
		Focus:           p.Focus,
		FocusRingWidth:  3,
		FocusRingOffset: 1,
		Inverse:         p.Text,
		InverseText:     p.Window,
		Scrollbar:       p.TextTertiary.Alpha(0.55),
		ScrollbarWidth:  7,
		Radius:          RadiusControl,
		Spacing:         4,
		FontSize:        FontBody,
		TooltipDelay:    550 * time.Millisecond,
	}
}

// Apply installs mypack's theme for the current appearance. Call it first
// in every view. mypack keeps its own brand accent on every platform.
func Apply(c *ui.Context) {
	if c.Theme().Dark {
		current = &Dark
		c.SetTheme(darkTheme)
	} else {
		current = &Light
		c.SetTheme(lightTheme)
	}
}

// Of returns the semantic palette for the current appearance.
func Of(c *ui.Context) *Palette {
	if current.Dark != c.Theme().Dark {
		Apply(c)
	}
	return current
}

// Fade returns col with its alpha multiplied by k.
func Fade(col ui.Color, k float32) ui.Color { return col.Alpha(float32(col.A) / 255 * k) }

// ---- Motion tokens ----------------------------------------------------------------

// speed scales every motion token (1 = as designed); the kit gallery's
// slider sets it to watch motion slowly.
var speed float32 = 1

// SetSpeed sets the motion speed factor (0.1–1); 1 is the design.
func SetSpeed(f float32) {
	speed = max(0.05, min(1, f))
	buildSprings()
}

// Speed returns the motion speed factor.
func Speed() float32 { return speed }

// Scaled returns d at the current motion speed.
func Scaled(d time.Duration) time.Duration { return time.Duration(float32(d) / speed) }

func spring(d time.Duration, bounce float64) ui.Animation {
	return ui.Animation{Spring: ui.SpringOptions{Duration: Scaled(d), Bounce: bounce}}
}

// Spring presets.
var (
	// Spring is the standard: quick with a hint of give (hover, press,
	// selection pills, indicators).
	Spring ui.Animation
	// SpringSoft settles without overshoot (sizes, sheets, progress).
	SpringSoft ui.Animation
	// SpringPop overshoots playfully (icons arriving, success).
	SpringPop ui.Animation
	// SpringGlide is a long, calm travel (things flying to a slot).
	SpringGlide ui.Animation
)

func buildSprings() {
	Spring = spring(260*time.Millisecond, 0.16)
	SpringSoft = spring(320*time.Millisecond, 0)
	SpringPop = spring(540*time.Millisecond, 0.42)
	SpringGlide = spring(620*time.Millisecond, 0.06)
}

func init() { buildSprings() }

// Tween is a curve animation of d (scaled) on ease.
func Tween(d time.Duration, ease ui.Easing) ui.Animation {
	return ui.Animation{Duration: Scaled(d), Ease: ease}
}

// Dur returns d at the current speed, or 0 when the user asked for
// reduced motion.
func Dur(c *ui.Context, d time.Duration) time.Duration {
	if c.Preferences().ReduceMotion {
		return 0
	}
	return Scaled(d)
}

// Transition is the standard color/shadow transition for interactive
// surfaces.
func Transition(c *ui.Context) ui.ElementTransition {
	return ui.ElementTransition{Duration: Dur(c, MotionFast), Ease: EaseStd, Colors: true, Shadows: true}
}

// ---- Elevation ------------------------------------------------------------------

// Elevation is how far a surface floats.
type Elevation int

const (
	Flat     Elevation = iota
	Lifted             // cards, buttons: a contact shadow
	Floating           // popovers, toasts, the canvas over the chrome
	Modal              // sheets
)

// Elevate gives e layered shadows (a tight contact shadow plus a soft
// ambient one) and, on raised surfaces, the 1 px inner top highlight that
// makes them read as physical.
func Elevate(c *ui.Context, e ui.Element, level Elevation) ui.Element {
	p := Of(c)
	switch level {
	case Lifted:
		e.Shadow(0, 1, 2, 0, p.Shadow).Shadow(0, 2, 6, -1, Fade(p.Shadow, 0.7))
	case Floating:
		e.Shadow(0, 1, 2, 0, p.Shadow).Shadow(0, 8, 24, -4, Fade(p.ShadowDeep, 0.7))
	case Modal:
		e.Shadow(0, 2, 6, 0, p.Shadow).Shadow(0, 24, 64, -8, p.ShadowDeep)
	}
	if level != Flat {
		e.InsetShadow(0, 1, 0, 0, p.Highlight)
	}
	return e
}
