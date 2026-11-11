package design

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func lum(c ui.Color) float64 {
	ch := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

func contrast(fg, bg ui.Color) float64 {
	fg = fg.Over(bg)
	a, b := lum(fg), lum(bg)
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}

// Text roles meet WCAG AA on every surface they sit on: 4.5:1 for
// primary and secondary text, 3:1 for tertiary text (captions, never body
// copy) and for the accent fill against its label (UI components).
func TestTextContrast(t *testing.T) {
	for name, p := range map[string]*Palette{"light": &Light, "dark": &Dark} {
		for sname, bg := range map[string]ui.Color{"chrome": p.Chrome, "canvas": p.Window, "raised": p.Raised, "overlay": p.Overlay} {
			check := func(role string, fg ui.Color, min float64) {
				if r := contrast(fg, bg); r < min {
					t.Errorf("%s %s on %s: %.2f:1, want ≥ %.1f:1", name, role, sname, r, min)
				}
			}
			check("text/primary", p.Text, 4.5)
			check("text/secondary", p.TextSecondary, 4.5)
			check("text/tertiary", p.TextTertiary, 3)
			check("accent text", p.AccentText, 4.5)
		}
		if r := contrast(p.TextOnAccent, p.Accent); r < 3 {
			t.Errorf("%s: label on accent %.2f:1, want ≥ 3:1", name, r)
		}
	}
}
