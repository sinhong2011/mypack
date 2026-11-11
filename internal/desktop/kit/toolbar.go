package kit

import (
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit/icons"
)

// The toolbar's controls: one quiet cluster of pills, all 30 pt high and
// 8 pt apart, sitting on the canvas with no box of their own.

const tb = design.ToolbarControl

// ToolbarSearchOpts configures ToolbarSearch.
type ToolbarSearchOpts struct {
	Placeholder string
	Hotkey      string // the palette's hotkey, shown as a chip ("mod+k")
	Rest, Open  float32
	// Query, when set, makes it a field that filters in place; nil makes
	// it a button that opens the palette (OnPalette).
	Query     *string
	Handle    *ui.Handle
	OnPalette func()
}

// ToolbarSearch is the search pill: a magnifier, "Search" and a ⌘K chip
// on a sunken pill that widens smoothly while it is in use. As a field it
// filters in place (the chip still opens the palette); as a button it
// opens the palette.
func ToolbarSearch(c *ui.Context, o ToolbarSearchOpts) ui.Element {
	p := design.Of(c)
	rest, open := o.Rest, o.Open
	if rest == 0 {
		rest = 176
	}
	if open == 0 {
		open = 280
	}
	var box ui.Element
	if o.Query == nil {
		box = ui.ButtonBase(c).Label(o.Placeholder).Cursor(ui.CursorPointer)
	} else {
		box = ui.Row(c).Cursor(ui.CursorText)
	}
	box.Row().Height(tb).Radius(tb/2).Padding(0, 5, 0, 11).Gap(7).AlignItems(ui.Center).Shrink(1).MinWidth(120)
	var in ui.Element
	focused := false
	box.Children(func() {
		Glyph(c, icons.Search, 14, p.TextTertiary)
		if o.Query == nil {
			ui.Text(c, o.Placeholder).FontSize(design.FontBody).TextColor(p.TextTertiary).SingleLine().Grow(1).MinWidth(0)
		} else {
			in = ui.TextInputBase(c, o.Query).Grow(1).MinWidth(0).FocusRing(false).FontSize(design.FontBody).
				Placeholder(o.Placeholder).PlaceholderColor(p.TextTertiary).Label(o.Placeholder).TextColor(p.Text)
			if o.Handle != nil {
				in.Bind(o.Handle)
			}
			focused = in.Focused()
		}
		if o.Query != nil && *o.Query != "" {
			x := ui.ButtonBase(c.Key("clear")).Label(o.Placeholder).Size(16, 16).Radius(8).Center().Background(p.TextTertiary).Cursor(ui.CursorPointer)
			Pop(x)
			x.Children(func() { Glyph(c, icons.Close, 10, p.Window) })
			if x.Clicked() {
				*o.Query = ""
			}
		} else if o.Hotkey != "" {
			chip := ui.ButtonBase(c.Key("k")).Label(HotkeyText(o.Hotkey)).Height(20).PaddingX(6).Radius(PillRadius(20)).Center().Cursor(ui.CursorPointer).FocusRing(false).
				Background(p.Fill).Transition(design.Transition(c))
			if chip.Hovered() {
				chip.Background(p.Pressed)
			}
			chip.Children(func() {
				ui.Text(c, HotkeyText(o.Hotkey)).FontSize(design.FontCaption).FontWeight(design.WeightMedium).TextColor(p.TextSecondary).SingleLine()
			})
			if chip.Clicked() && o.OnPalette != nil {
				o.OnPalette()
			}
		}
	})
	wide := focused || (o.Query != nil && *o.Query != "")
	target := rest
	if wide {
		target = open
	}
	box.Width(box.AnimateTo("w", target, design.Spring))
	box.Transition(ui.ElementTransition{Duration: design.Dur(c, 140*1e6), Ease: design.EaseStd, Colors: true, Shadows: true})
	FieldStyle(c, box, focused, false)
	if !focused && box.Hovered() {
		box.Background(p.FieldRest.Mix(p.FieldFocus, 0.5))
	}
	if box.Clicked() {
		if o.Query == nil {
			if o.OnPalette != nil {
				o.OnPalette()
			}
		} else if in != (ui.Element{}) {
			in.Focus()
		}
	}
	return box
}

// PillSegment is one choice of a PillSegmented: an icon or a label.
type PillSegment struct {
	Icon  *ui.SVG
	Label string // the visible text; with Icon, the tooltip
}

// PillSegmented is a small segmented pill (30 pt): a soft track with no
// border and a raised thumb that glides to the chosen segment. glide
// pairs the thumb across segments; role is RoleRadio or RoleTab.
func PillSegmented(c *ui.Context, sel *int, segs []PillSegment, label string, glide any, tabs bool) bool {
	p := design.Of(c)
	changed := false
	grp := ui.Row(c).Height(tb).Padding(3).Gap(0).Radius(tb / 2).AlignItems(ui.Center).Background(p.Fill).Label(label)
	if tabs {
		grp.Role(ui.RoleTabList)
	} else {
		grp.Role(ui.RoleRadioGroup)
	}
	grp.Children(func() {
		for i, s := range segs {
			on := *sel == i
			b := ui.ButtonBase(c.Key(i)).Height(tb - 6).Radius(PillRadius(tb - 6)).Center().Cursor(ui.CursorPointer).FocusRing(false)
			if s.Icon != nil {
				b.Width(32).Label(s.Label).Tooltip(s.Label)
			} else {
				b.PaddingX(12).Label(s.Label)
			}
			if tabs {
				b.Role(ui.RoleTab).Selected(on)
			} else {
				b.Role(ui.RoleRadio).Checked(on)
			}
			fg := p.TextSecondary
			switch {
			case on:
				fg = p.Text
			case b.Hovered() || b.FocusVisible():
				fg = p.Text
				b.Background(p.Hover)
			}
			b.Children(func() {
				if on {
					thumb := ui.Box(c).Absolute().Fill().Radius((tb-6)/2).ZIndex(-1).Background(p.Raised).Border(1, p.Border)
					if p.Dark {
						thumb.Background(p.ButtonHover)
					}
					Glide(thumb, glide)
				}
				FocusMark(c, b.FocusVisible(), 8)
				if s.Icon != nil {
					Glyph(c, s.Icon, 14, fg)
				} else {
					w := design.WeightMedium
					if on {
						w = design.WeightSemibold
					}
					ui.Text(c, s.Label).FontSize(design.FontBody).FontWeight(w).TextColor(fg).SingleLine()
				}
			})
			if b.Clicked() && !on {
				*sel = i
				changed = true
			}
		}
	})
	return changed
}

// NewPill is the toolbar's primary action: a labelled accent pill with a
// plus, or a 30 pt circle when compact.
func NewPill(c *ui.Context, label, hotkey string, compact bool) ui.Element {
	if compact {
		b := Button(c, "", ButtonOpts{Variant: Primary, Size: tb, Icon: icons.Plus, Tooltip: label, Hotkey: hotkey})
		return b.Label(label)
	}
	return Button(c, label, ButtonOpts{Variant: Primary, Size: tb, Icon: icons.Plus, Hotkey: hotkey})
}

// ToolbarIcon is a quiet 30 pt circle holding one icon (more, back): no
// fill until hovered.
func ToolbarIcon(c *ui.Context, ic *ui.SVG, title string) ui.Element {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(title).Size(tb, tb).Radius(PillRadius(tb)).Center().Cursor(ui.CursorPointer).Shrink(0).FocusRing(false).
		Transition(design.Transition(c))
	fg := p.TextSecondary
	if b.Hovered() || b.FocusVisible() {
		b.Background(p.Hover)
		fg = p.Text
	}
	if b.Pressed() {
		b.Background(p.Pressed)
	}
	Press(b, false, b.Pressed(), 0)
	b.Children(func() { Glyph(c, ic, 16, fg); FocusMark(c, b.FocusVisible(), 9) })
	b.Tooltip(title)
	return b
}
