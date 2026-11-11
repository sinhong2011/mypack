package kit

import (
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit/icons"
)

// ChoiceStyle is the one look of a selectable choice (thumbnail pickers,
// target chips, cards): a hairline at rest, a quiet fill on hover, and when
// selected an accent-tinted fill (with a check badge where it fits). No
// outline ring.
func ChoiceStyle(c *ui.Context, e ui.Element, base ui.Color, selected, hovered bool) ui.Element {
	p := design.Of(c)
	switch {
	case selected:
		e.Background(p.ChoiceFill.Over(base)).Border(1, p.FieldStroke)
	case hovered:
		e.Background(p.Hover.Over(base)).Border(1, p.FieldStroke)
	default:
		e.Background(base).Border(1, p.FieldStroke)
	}
	return e
}

// CheckBadge is the small filled accent check that marks the chosen tile;
// it pops in.
func CheckBadge(c *ui.Context, size float32) ui.Element {
	p := design.Of(c)
	b := ui.Box(c.Key("check")).Size(size, size).Radius(size/2).Center().Background(p.Accent).
		Border(1.5, p.Window).PassThrough().AccessHidden(true)
	b.Children(func() { Glyph(c, icons.Check, size*0.62, p.TextOnAccent) })
	return Pop(b)
}

// ChoiceTile is a selectable preview, as an appearance picker: the picture
// in a tile with ChoiceStyle and a check badge when chosen, its label (and
// detail) under it. The chosen label is primary text at 600; the others
// are secondary. It reports a click.
func ChoiceTile(c *ui.Context, label, sub string, selected bool, w, h float32, pic func()) bool {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(label).Description(sub).Column().AlignItems(ui.Center).Gap(7).Cursor(ui.CursorPointer).
		Role(ui.RoleRadio).Checked(selected).FocusRing(false)
	hov := b.Hovered() || b.FocusVisible()
	b.Children(func() {
		frame := ui.Box(c).Size(w+8, h+8).Padding(3.5).Radius(design.RadiusCard).Transition(design.Transition(c))
		ChoiceStyle(c, frame, p.Raised, selected, hov && !selected)
		kbd := b.FocusVisible()
		frame.Children(func() {
			FocusMark(c, kbd, 16) // keyboard focus: an underline, no ring
			ui.Box(c).Size(w, h).Radius(design.RadiusCard - 3.5).Clip().Children(pic)
			if selected {
				CheckBadge(c, 16).Absolute().Top(-6).Right(-6)
			}
		})
		fg, wt := p.TextSecondary, design.WeightRegular
		if selected {
			fg, wt = p.Text, design.WeightSemibold
		}
		ui.Text(c, label).FontSize(design.FontSmall).FontWeight(wt).TextColor(fg).SingleLine().MaxWidth(w + 16)
		if sub != "" {
			ui.Text(c, sub).FontSize(design.FontMicro).TextColor(p.TextTertiary).SingleLine().FontFeatures("tnum").Margin(-4, 0, 0, 0)
		}
	})
	Lift(b, hov && !selected, b.Pressed())
	if b.IsDisabled() {
		b.Opacity(0.6)
	}
	return b.Clicked()
}

// ChoiceTileSmall is ChoiceTile for a single row of compact options in an
// inspector: a tile exactly slot wide (its picture keeps aspect at h/w),
// a 12 pt label and, when sub is set, a small tabular detail under it.
func ChoiceTileSmall(c *ui.Context, label, sub string, selected bool, slot, aspect float32, pic func()) bool {
	p := design.Of(c)
	w := slot - 6
	h := w * aspect
	b := ui.ButtonBase(c).Label(label).Description(sub).Column().Width(slot).Shrink(0).AlignItems(ui.Center).Gap(0).Cursor(ui.CursorPointer).
		Role(ui.RoleRadio).Checked(selected).FocusRing(false)
	hov := b.Hovered() || b.FocusVisible()
	b.Children(func() {
		frame := ui.Box(c).Size(slot, h+6).Padding(3).Radius(design.RadiusCard - 2).Transition(design.Transition(c))
		ChoiceStyle(c, frame, p.Raised, selected, hov && !selected)
		kbd := b.FocusVisible()
		frame.Children(func() {
			FocusMark(c, kbd, 14)
			ui.Box(c).Size(w, h).Radius(design.RadiusCard - 5).Clip().Children(pic)
			if selected {
				CheckBadge(c, 14).Absolute().Top(-5).Right(-5)
			}
		})
		fg, wt := p.TextSecondary, design.WeightRegular
		if selected {
			fg, wt = p.Text, design.WeightSemibold
		}
		ui.Text(c, label).FontSize(12).FontWeight(wt).TextColor(fg).SingleLine().MaxWidth(slot).Margin(5, 0, 0, 0)
		if sub != "" {
			ui.Text(c, sub).FontSize(10.5).TextColor(p.TextSecondary).FontFeatures("tnum").SingleLine().MaxWidth(slot).Margin(1, 0, 0, 0)
		}
	})
	if b.IsDisabled() {
		b.Opacity(0.6)
	}
	return b.Clicked()
}
