package kit

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"time"

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
			if focused {
				MarkTyping()
			}
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
	Count string // a small count badge after the label ("" for none)
}

// PillSegmented is a small segmented pill (30 pt): a soft track and one
// raised thumb under the chosen segment. The thumb belongs to the track,
// not to a segment: it is placed from the segments' measured bounds and
// its two edges are sprung apart, the leading edge quick and the trailing
// one a little slower, so it stretches toward the new segment and settles
// there instead of jumping or cross-fading. Pressing the chosen segment
// sinks the thumb a touch. Labels keep their widths (pillLabel), so the
// target never moves while the thumb travels. role is RoleTab when tabs,
// else RoleRadio. Reduce Motion puts the thumb in place at once.
func PillSegmented(c *ui.Context, sel *int, segs []PillSegment, label string, tabs bool) bool {
	p := design.Of(c)
	const h = tb - 6 // segment and thumb height
	changed := false
	// The track: a soft inset trough with a hairline edge.
	grp := ui.Row(c).Height(tb).Padding(3).Gap(2).Radius(tb/2).AlignItems(ui.Center).Background(p.Fill).
		Border(0.5, p.Divider).Label(label)
	if tabs {
		grp.Role(ui.RoleTabList)
	} else {
		grp.Role(ui.RoleRadioGroup)
	}
	track := grp.Bounds()
	grp.Children(func() {
		var on ui.Rect
		pressed := false
		for i, s := range segs {
			chosen := *sel == i
			b := ui.ButtonBase(c.Key(i)).Height(h).Radius(PillRadius(h)).Center().Cursor(ui.CursorPointer).FocusRing(false).
				Transition(design.Transition(c))
			if s.Icon != nil {
				b.Width(32).Label(s.Label).Tooltip(s.Label)
			} else {
				b.Gap(6).Label(strings.TrimSpace(s.Label + " " + s.Count))
				if s.Count != "" {
					b.Padding(0, 6, 0, 12) // the badge's own round end is the right inset
				} else {
					b.PaddingX(12)
				}
			}
			if tabs {
				// Selected also paints a table row's accent fill: the
				// track's thumb shows the choice instead.
				b.Role(ui.RoleTab).Selected(chosen).Background(ui.RGBA(0, 0, 0, 0))
			} else {
				b.Role(ui.RoleRadio).Checked(chosen)
			}
			fg := p.TextSecondary
			hot := b.Hovered() || b.FocusVisible()
			if chosen || hot {
				fg = p.Text
			}
			if hot && !chosen {
				fg = design.Fade(p.Text, 0.85)
			}
			if chosen {
				on = b.Bounds()
				pressed = b.Pressed()
			}
			b.Children(func() {
				FocusMark(c, b.FocusVisible(), 8)
				if s.Icon != nil {
					Glyph(c, s.Icon, 14, fg).Transition(ui.ElementTransition{Animation: &pillFade, Colors: true})
				} else {
					ui.Text(c, s.Label).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(fg).SingleLine().
						Transition(ui.ElementTransition{Animation: &pillFade, Colors: true})
					if s.Count != "" {
						pillCount(c, s.Count, chosen)
					}
				}
			})
			if b.Clicked() && !chosen {
				*sel = i
				changed = true
			}
		}
		pillThumb(c, grp, sel, track, on, pressed)
	})
	return changed
}

var (
	pillFade  = design.Tween(design.MotionFast, design.EaseStd)
	pillLead  = ui.Animation{Spring: ui.SpringOptions{Duration: design.Scaled(230 * time.Millisecond), Bounce: 0.14}}
	pillTrail = ui.Animation{Spring: ui.SpringOptions{Duration: design.Scaled(380 * time.Millisecond), Bounce: 0.04}}
	pillSink  = ui.Animation{Spring: ui.SpringOptions{Duration: design.Scaled(160 * time.Millisecond)}}
	// pillLast is each control's last thumb target (by its selection), to
	// tell which edge leads a move.
	pillLast = map[*int][2]float32{}
)

// pillThumb draws the thumb behind the segments, at on (the chosen
// segment's bounds from the last frame) relative to the track. Before the
// first layout there is nothing to measure: it waits a frame rather than
// sliding in from the track's start.
func pillThumb(c *ui.Context, grp ui.Element, sel *int, track, on ui.Rect, pressed bool) {
	if track.W <= 0 || on.W <= 0 {
		return
	}
	p := design.Of(c)
	l, r := on.X-track.X, on.X-track.X+on.W
	la, ra := pillLead, pillLead
	if last, ok := pillLast[sel]; ok {
		switch {
		case l > last[0]: // moving right: the right edge leads
			la = pillTrail
		case l < last[0]: // moving left: the left edge leads
			ra = pillTrail
		}
	}
	pillLast[sel] = [2]float32{l, r}
	cl := grp.AnimateTo("thumb-l", l, la)
	cr := grp.AnimateTo("thumb-r", r, ra)
	sink := grp.AnimateTo("thumb-sink", map[bool]float32{true: 1.5, false: 0}[pressed], pillSink)
	w := max(cr-cl, on.H*0.6)
	thumb := ui.Box(c.Key("thumb")).Absolute().Left(cl+sink).Top(on.Y-track.Y+sink).Width(w-2*sink).Height(on.H-2*sink).
		Radius((on.H-2*sink)/2).ZIndex(-1).PassThrough().AccessHidden(true).Background(p.Raised).Border(0.5, p.BorderStrong)
	if p.Dark {
		thumb.Background(p.ButtonHover)
	}
	design.Elevate(c, thumb, design.Lifted)
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

// pillCount is a segment's count: a plain number after its label, a
// step quieter than it (no badge, no tint).
func pillCount(c *ui.Context, n string, chosen bool) {
	p := design.Of(c)
	fg := p.TextTertiary
	if chosen {
		fg = p.TextSecondary
	}
	ui.Text(c, n).FontSize(design.FontSmall).TextColor(fg).SingleLine().
		Transition(ui.ElementTransition{Animation: &pillFade, Colors: true})
}
