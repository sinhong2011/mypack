package design

import (
	"time"

	"github.com/egoist/mygo/ui"
)

var (
	lightTheme = Light.theme()
	darkTheme  = Dark.theme()
)

func (p *Palette) theme() *ui.Theme {
	return &ui.Theme{
		Dark:            p.Dark,
		Background:      p.Bg,
		Surface:         p.Well,
		SurfaceHover:    p.Hover,
		SurfacePressed:  p.Pressed,
		Border:          p.BorderStrong,
		Text:            p.Text,
		TextMuted:       p.TextSecondary,
		Accent:          p.Accent,
		AccentHover:     p.AccentHover,
		AccentPressed:   p.AccentPressed,
		AccentText:      p.TextOnAccent,
		Danger:          p.Danger,
		Warning:         p.Warning,
		Success:         p.Success,
		Selection:       p.AccentSoft,
		Focus:           p.Focus,
		FocusRingWidth:  2.5,
		FocusRingOffset: 1,
		Scrollbar:       p.TextTertiary.Alpha(0.5),
		ScrollbarWidth:  6,
		Radius:          RadiusControl,
		Spacing:         4,
		FontSize:        FontBody,
		TooltipDelay:    450 * time.Millisecond,
	}
}

// Apply installs mypack's theme for the current appearance. Call it first
// in every view. Lobe MyGo components key their own tokens off Theme.Dark,
// which this preserves.
func Apply(c *ui.Context) {
	if c.Theme().Dark {
		c.SetTheme(darkTheme)
	} else {
		c.SetTheme(lightTheme)
	}
}

// Of returns the palette for the current appearance.
func Of(c *ui.Context) *Palette {
	if c.Theme().Dark {
		return &Dark
	}
	return &Light
}

// Dur returns d, or 0 when the user asked for reduced motion.
func Dur(c *ui.Context, d time.Duration) time.Duration {
	if c.Preferences().ReduceMotion {
		return 0
	}
	return d
}

// Transition is the standard color/shadow transition for interactive
// surfaces.
func Transition(c *ui.Context) ui.ElementTransition {
	return ui.ElementTransition{Duration: Dur(c, MotionFast), Ease: ui.CubicBezier(0.25, 0.1, 0.25, 1), Colors: true, Shadows: true}
}
