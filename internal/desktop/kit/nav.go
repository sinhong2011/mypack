package kit

import (
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
)

// NavRow styles a sidebar row: a 4 % fill on hover, nothing at rest. The
// selected row gets NavPill instead.
func NavRow(c *ui.Context, row ui.Element, active bool) ui.Element {
	p := design.Of(c)
	row.Radius(design.RadiusControl).Transition(design.Transition(c)).FocusRing(false)
	if !active && (row.Hovered() || row.FocusVisible()) {
		row.Background(p.NavHover)
	}
	return row
}

// NavPill is the sidebar's selection: a soft pill of primary text at 7 %
// (9 % in dark), radius 8, no border or shadow, that glides on a spring
// between rows sharing the glide key.
func NavPill(c *ui.Context, glide any) ui.Element {
	p := design.Of(c)
	pill := ui.Box(c).Absolute().Fill().Radius(design.RadiusControl).ZIndex(-1).Background(p.NavSelected).PassThrough()
	return Glide(pill, glide)
}

// NavCount is a sidebar count: a small neutral pill, or accent when it
// counts something to act on (updates).
func NavCount(c *ui.Context, text string, accent bool) ui.Element {
	p := design.Of(c)
	t := ui.Text(c.Key("badge"), text).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).FontFeatures("tnum").
		Padding(1, 6).Radius(8)
	if accent {
		t.TextColor(p.TextOnAccent).Background(p.Accent)
	} else {
		t.TextColor(p.TextSecondary).Background(p.Fill)
	}
	return Pop(t)
}
