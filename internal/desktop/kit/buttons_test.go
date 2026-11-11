package kit

import (
	"testing"

	"mypack.dev/mypack/internal/desktop/design"
)

// Every button is a pill (radius = height/2) and every icon-only button a
// circle, whatever its variant and size.
func TestButtonsArePills(t *testing.T) {
	variants := []Variant{Secondary_, Primary, Ghost, Danger, Plain, Glass, GlassProminent}
	sizes := []float32{design.ControlSM, design.Control, design.ToolbarControl, design.ControlLG, design.ControlXL, 28, 20}
	for _, v := range variants {
		for _, h := range sizes {
			for _, iconOnly := range []bool{false, true} {
				g := buttonGeometry(v, h, iconOnly)
				if g.h != h || g.radius != h/2 {
					t.Errorf("variant %d, %v pt: radius %v, want %v", v, h, g.radius, h/2)
				}
				if iconOnly && g.w != h {
					t.Errorf("variant %d, %v pt icon-only: width %v, want a %v pt circle", v, h, g.w, h)
				}
			}
		}
	}
	if PillRadius(30) != 15 {
		t.Errorf("PillRadius(30) = %v", PillRadius(30))
	}
}
