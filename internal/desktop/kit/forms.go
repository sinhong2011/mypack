package kit

import (
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit/icons"
)

// ---- Field chrome --------------------------------------------------------------

// fieldChrome styles a field box: a raised face with a hairline, which on
// focus takes an accent edge and a soft glow that grows in, and which
// shakes once when it turns invalid.
func fieldChrome(c *ui.Context, box ui.Element, focused, invalid bool, radius float32) {
	box.Radius(radius).Transition(ui.ElementTransition{Duration: design.Dur(c, 140*time.Millisecond), Ease: design.EaseStd, Colors: true, Shadows: true})
	FieldStyle(c, box, focused, invalid)
	// Shake once on turning invalid.
	since := ui.Local(box, "invalidAt", func() time.Time { return time.Time{} })
	was := ui.Local(box, "invalid", func() bool { return false })
	if invalid && !*was && !Reduced(c) {
		*since = time.Now()
	}
	*was = invalid
	if t := time.Since(*since).Seconds(); !since.IsZero() && t < 0.45 {
		box.Translate(float32(Shake(t)), 0)
		c.Invalidate()
	}
}

// FieldStyle is the one look of a text field: a sunken fill with a 1 px
// hairline at rest; focused, the fill brightens a step and the hairline
// darkens a little. Nothing outside the field: no ring, no glow.
func FieldStyle(c *ui.Context, box ui.Element, focused, invalid bool) ui.Element {
	p := design.Of(c)
	switch {
	case invalid:
		return box.Background(p.FieldFocus).Border(1, design.Fade(p.Danger, 0.75))
	case focused:
		return box.Background(p.FieldFocus).Border(1, p.BorderStrong)
	}
	return box.Background(p.FieldRest).Border(1, p.FieldStroke)
}

// ---- Hero URL field ----------------------------------------------------------------

// URLFieldOpts configures URLField.
type URLFieldOpts struct {
	Placeholder string
	Label       string
	Loading     bool
	Invalid     bool
	Action      string // trailing prominent button label; "" = none
	ActionOff   bool
	Paste       string // trailing Paste button label (shown while empty); "" = none
	AutoFocus   bool
	Handle      *ui.Handle
	Height      float32 // 0 = 52
	// OnPaste sees a paste before it is inserted (main thread); return true
	// to take it.
	OnPaste func(text string) bool
}

// URLFieldResult reports what happened this frame.
type URLFieldResult struct {
	Changed, Submitted, Action, Paste bool
	Dropped                           []string
	DragOver                          bool
}

// URLField is the big "paste a website" field: a raised slab with a globe,
// the address in 17 pt, and a Paste or Continue button inside it. It
// accepts typing, paste (auto-detected by the caller) and dropped HTML
// files.
func URLField(c *ui.Context, value *string, o URLFieldOpts) URLFieldResult {
	p := design.Of(c)
	var r URLFieldResult
	h := o.Height
	if h == 0 {
		h = 52
	}
	box := ui.Row(c).Height(h).Padding(0, 7, 0, 18).Gap(12).AlignItems(ui.Center).Cursor(ui.CursorText)
	var in ui.Element
	box.Children(func() {
		ic, col := icons.Globe, p.TextTertiary
		if o.Invalid {
			ic, col = icons.Warning, p.Warning
		}
		Glyph(c, ic, 18, col)
		in = ui.TextInputBase(c, value).Grow(1).MinWidth(0).FocusRing(false).FontSize(design.FontHeroField).
			Placeholder(o.Placeholder).PlaceholderColor(p.TextTertiary).Label(o.Label).TextColor(p.Text)
		if o.OnPaste != nil {
			in.OnPaste(o.OnPaste)
		}
		if o.AutoFocus {
			in.AutoFocus()
		}
		if o.Handle != nil {
			in.Bind(o.Handle)
		}
		if o.Loading {
			Spinner(c, 16, p.TextTertiary).Margin(0, 8, 0, 0)
		}
		bh := h - 14
		switch {
		case o.Paste != "" && *value == "":
			r.Paste = Button(c.Key("paste"), o.Paste, ButtonOpts{Size: bh, Icon: icons.Paste}).Clicked()
		case o.Action != "":
			r.Action = Button(c.Key("action"), o.Action, ButtonOpts{Variant: Primary, Size: bh, Disabled: o.ActionOff, Trailing: icons.ArrowRight}).Clicked()
		default:
			ui.Box(c).Width(6)
		}
	})
	focused := in.Focused()
	r.DragOver = box.FileDragOver()
	r.Dropped = box.DroppedFiles()
	fieldChrome(c, box, focused || r.DragOver, false, design.RadiusField)
	if !focused && !r.DragOver {
		design.Elevate(c, box, design.Lifted)
	}
	if r.DragOver {
		box.Background(p.AccentSoft.Over(p.FieldFocus))
	}
	if box.Clicked() {
		in.Focus()
	}
	r.Changed = in.Changed()
	r.Submitted = in.Submitted()
	return r
}

// ---- Grouped rows ---------------------------------------------------------------

// Group is a titled box of rows.
func Group(c *ui.Context, title string, body func()) { GroupWith(c, title, "", body) }

// GroupWith is Group with a footer note under the box.
func GroupWith(c *ui.Context, title, footer string, body func()) {
	p := design.Of(c)
	ui.Column(c).Gap(8).Children(func() {
		if title != "" {
			Eyebrow(c, title).Padding(0, 2)
		}
		GroupBox(c).Children(body)
		if footer != "" {
			ui.Text(c, footer).FontSize(design.FontCaption).TextColor(p.TextSecondary).Padding(0, 2).LineHeight(1.35)
		}
	})
}

// GroupBox is the box of a group: a raised sheet with hairline rows.
func GroupBox(c *ui.Context) ui.Element {
	p := design.Of(c)
	b := ui.Column(c).Background(p.Raised).Border(0.5, p.GroupBorder).Radius(design.RadiusCard).
		Dividers(0.5, p.Divider).Clip()
	return design.Elevate(c, b, design.Lifted)
}

// SettingRow is one row in a Group: label (and description) left, control
// right.
func SettingRow(c *ui.Context, label, desc string, control func()) {
	SettingIconRow(c, nil, ui.Color{}, label, desc, control)
}

// SettingIconRow is a SettingRow led by an icon.
func SettingIconRow(c *ui.Context, ic *ui.SVG, tint ui.Color, label, desc string, control func()) {
	p := design.Of(c)
	ui.Row(c).MinHeight(design.Row+4).Padding(10, 14).Gap(12).AlignItems(ui.Center).Children(func() {
		if ic != nil {
			SymbolTile(c, ic, tint, 28)
		}
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text)
			if desc != "" {
				ui.Text(c, desc).FontSize(design.FontSmall).TextColor(p.TextSecondary).LineHeight(1.35)
			}
		})
		if control != nil {
			ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
		}
	})
}

// SymbolTile is a quiet tile holding an icon in a tint: tinted ink on a
// soft wash of the same hue, never a loud gradient.
func SymbolTile(c *ui.Context, ic *ui.SVG, tint ui.Color, size float32) ui.Element {
	p := design.Of(c)
	if tint.A == 0 {
		tint = p.TextSecondary
	}
	wash := tint.Alpha(0.12)
	if p.Dark {
		wash = tint.Alpha(0.18)
	}
	return ui.Row(c).Size(size, size).Radius(size * 0.28).Center().Shrink(0).Background(wash).
		Children(func() { Glyph(c, ic, size*0.56, tint) }).AccessHidden(true)
}

// SettingBlock is a full-width row (label above a wide control).
func SettingBlock(c *ui.Context, label, desc string, control func()) {
	p := design.Of(c)
	ui.Column(c).Padding(12, 14).Gap(8).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text)
			if desc != "" {
				ui.Text(c, desc).FontSize(design.FontSmall).TextColor(p.TextSecondary).LineHeight(1.35)
			}
		})
		control()
	})
}

// ---- Controls ------------------------------------------------------------------

// Option is one PopUp choice.
type Option struct {
	Value, Label string
}

// PopUp is a pop-up button choosing one option's Value into *selected.
func PopUp(c *ui.Context, selected *string, options []Option, label string) ui.Element {
	p := design.Of(c)
	labels := make([]string, len(options))
	cur := ""
	for i, o := range options {
		labels[i] = o.Label
		if o.Value == *selected {
			cur = o.Label
		}
	}
	if cur == "" && len(options) > 0 {
		cur = options[0].Label
	}
	old := cur
	e := ui.Select(c, &cur, labels).Label(label).Height(design.Control).Radius(design.RadiusControl).
		Background(p.ButtonFill).Border(0.5, p.BorderStrong)
	design.Elevate(c, e, design.Lifted)
	if cur != old {
		for _, o := range options {
			if o.Label == cur {
				*selected = o.Value
			}
		}
	}
	return e
}

// Switch is mypack's switch: the thumb travels on a spring and stretches
// while pressed, the track fills with the accent.
func Switch(c *ui.Context, on *bool, label string) ui.Element {
	p := design.Of(c)
	const w, h, pad = 36, 21, 2
	track := ui.SwitchBase(c, on).Size(w, h).Padding(pad).Radius(h / 2).Cursor(ui.CursorPointer).Label(label).FocusRing(false)
	bg := p.Fill.Over(p.Raised)
	if p.Dark {
		bg = p.BorderStrong
	}
	if *on {
		bg = p.Accent
	}
	track.Background(track.AnimateColor("bg", bg, design.Tween(design.MotionBase, design.EaseStd)))
	if track.IsDisabled() {
		track.Opacity(0.45)
	}
	pressed := track.Pressed()
	track.Children(func() {
		knob := float32(h - 2*pad)
		width := knob
		if pressed {
			width = knob + 5
		}
		width = track.AnimateTo("w", width, design.Spring)
		x := float32(0)
		if *on {
			x = w - 2*pad - width
		}
		th := ui.Box(c).Size(width, knob).Radius(knob/2).Background(ui.RGB(255, 255, 255)).Border(0.5, ui.RGBA(0, 0, 0, 0.1))
		th.Translate(track.AnimateTo("x", x, design.Spring), 0)
		FocusMark(c, track.FocusVisible(), 8)
	})
	return track
}

// Checkbox is mypack's checkbox: the box fills with the accent and the
// check draws itself on.
func Checkbox(c *ui.Context, checked *bool, label string) ui.Element {
	p := design.Of(c)
	row := ui.CheckboxBase(c, checked).Row().Gap(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(label).FocusRing(false)
	row.Children(func() {
		FocusMark(c, row.FocusVisible(), 0)
		box := ui.Box(c).Size(16, 16).Radius(4.5).Transition(design.Transition(c))
		if *checked {
			box.Background(p.Accent)
		} else {
			box.Background(p.FieldFill).Border(1, p.BorderStrong)
		}
		drawn := float32(0)
		if *checked {
			drawn = 1
		}
		drawn = box.AnimateTo("check", drawn, design.Tween(design.MotionBase, design.EaseArrive))
		onAccent := p.TextOnAccent
		box.DrawOver(func(pt *ui.Painter, r ui.Rect) {
			if drawn <= 0.01 {
				return
			}
			path := (&ui.Path{}).MoveTo(r.X+4, r.Y+8.3).LineTo(r.X+7, r.Y+11.2).LineTo(r.X+12.2, r.Y+5)
			pt.StrokePath(path.Trim(0, drawn), 1.9, onAccent)
		})
		ui.Text(c, label).FontSize(design.FontBody).TextColor(p.Text)
	})
	return row
}

// FieldOpts configures TextField.
type FieldOpts struct {
	Placeholder string
	Invalid     bool
	Mono        bool
}

// TextField is mypack's text field: raised face, focus glow, shake on
// invalid.
func TextField(c *ui.Context, value *string, label string, o FieldOpts) ui.Element {
	p := design.Of(c)
	in := ui.TextInputBase(c, value).Placeholder(o.Placeholder).PlaceholderColor(p.TextTertiary).Label(label).
		Height(design.Control).Padding(0, 10).FontSize(design.FontBody).TextColor(p.Text).FocusRing(false)
	if o.Mono {
		in.Font(MonoFont).FontSize(design.FontSmall)
	}
	fieldChrome(c, in, in.Focused(), o.Invalid, design.RadiusField)
	return in
}

// TextArea is the multi-line field.
func TextArea(c *ui.Context, value *string, label, placeholder string, minLines, maxLines int, mono bool) ui.Element {
	p := design.Of(c)
	in := ui.TextAreaBase(c, value).Placeholder(placeholder).PlaceholderColor(p.TextTertiary).Label(label).
		Lines(minLines, maxLines).Padding(8, 10).FontSize(design.FontBody).TextColor(p.Text).FocusRing(false)
	if mono {
		in.Font(MonoFont).FontSize(design.FontSmall)
	}
	fieldChrome(c, in, in.Focused(), false, design.RadiusField)
	return in
}

// segMatch pairs the sliding indicator across frames.
type segMatch struct{ selected *int }

// Segmented is a segmented control over labels; *selected is an index.
// The raised indicator glides to the chosen segment on a spring.
func Segmented(c *ui.Context, selected *int, labels []string, small bool) ui.Element {
	p := design.Of(c)
	h, pad, fs := float32(28), float32(13), design.FontBody
	if small {
		h, pad, fs = 24, 10, design.FontSmall
	}
	parts := ui.SegmentedBase(c, selected, len(labels))
	track := parts.Track.Padding(2).Gap(0).Radius(design.RadiusControl + 1).Background(p.Fill).AlignSelf(ui.Start)
	track.Children(func() {
		for i, l := range labels {
			seg := parts.Segment(i).Height(h).PaddingX(pad).Radius(design.RadiusControl - 1).FontSize(fs).
				FontWeight(design.WeightMedium).Cursor(ui.CursorPointer)
			fg := p.TextSecondary
			if i == *selected || seg.Hovered() {
				fg = p.Text
			}
			seg.TextColor(fg)
			seg.Children(func() {
				if i == *selected {
					ind := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(design.RadiusControl - 1).ZIndex(-1).
						Background(p.Raised)
					design.RaisedControl(c, ind)
					Glide(ind, segMatch{selected})
				}
				ui.Text(c, l).SingleLine()
			})
		}
	})
	return track
}

// SearchField is a compact search field.
func SearchField(c *ui.Context, query *string, placeholder string, h *ui.Handle, width float32) ui.Element {
	p := design.Of(c)
	box := ui.Row(c).Height(design.Control).Padding(0, 8, 0, 10).Gap(7).AlignItems(ui.Center).Cursor(ui.CursorText)
	if width > 0 {
		box.Width(width)
	}
	var in ui.Element
	box.Children(func() {
		Glyph(c, icons.Search, 14, p.TextTertiary)
		in = ui.TextInputBase(c, query).Grow(1).MinWidth(0).FocusRing(false).FontSize(design.FontBody).
			Placeholder(placeholder).PlaceholderColor(p.TextTertiary).Label(placeholder).TextColor(p.Text)
		if h != nil {
			in.Bind(h)
		}
		if *query != "" {
			x := ui.ButtonBase(c.Key("clear")).Label(placeholder).Size(16, 16).Radius(8).Center().Background(p.TextTertiary).Cursor(ui.CursorPointer)
			Pop(x)
			x.Children(func() { Glyph(c, icons.Close, 10, p.Window) })
			if x.Clicked() {
				*query = ""
			}
		}
	})
	focused := in.Focused()
	fieldChrome(c, box, focused, false, design.RadiusField)
	if !focused {
		design.Well(c, box)
	}
	if box.Clicked() {
		in.Focus()
	}
	return box
}

// Disclosure is a clickable header that toggles *open; put its content in
// Reveal so it opens its height.
func Disclosure(c *ui.Context, open *bool, title, subtitle string) ui.Element {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(title).Row().Justify(ui.Start).Height(36).PaddingX(8).Gap(8).AlignItems(ui.Center).
		Radius(design.RadiusControl).Cursor(ui.CursorPointer).Role(ui.RoleDisclosure).Expanded(*open).Transition(design.Transition(c)).FocusRing(false)
	if b.Hovered() || b.FocusVisible() {
		b.Background(p.Hover)
	}
	b.Children(func() {
		FocusMark(c, b.FocusVisible(), 8)
		ch := Glyph(c, icons.ChevronRt, 14, p.TextSecondary)
		turn := float32(0)
		if *open {
			turn = 90
		}
		ch.Rotate(ch.AnimateTo("turn", turn, design.Spring))
		ui.Text(c, title).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(design.FontBody).TextColor(p.TextTertiary).SingleLine().MinWidth(0).Shrink(1)
		}
	})
	if b.Clicked() {
		*open = !*open
	}
	return b
}

// TargetChip is a selectable platform tile with an optional reason.
func TargetChip(c *ui.Context, ic *ui.SVG, label, sub string, selected, disabled bool) ui.Element {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(label).Row().Justify(ui.Start).Height(52).Padding(0, 12).Gap(10).AlignItems(ui.Center).
		Radius(design.RadiusCard).Cursor(ui.CursorPointer).Transition(design.Transition(c)).Disabled(disabled).
		Role(ui.RoleCheckBox).Checked(selected).FocusRing(false)
	hov := (b.Hovered() || b.FocusVisible()) && !disabled
	ChoiceStyle(c, b, p.Raised, selected, hov && !selected)
	Lift(b, hov && !selected, b.Pressed())
	if disabled {
		b.Opacity(0.5).Cursor(ui.CursorNotAllowed)
	}
	b.Children(func() {
		FocusMark(c, b.FocusVisible(), 12)
		col := p.TextSecondary
		if selected {
			col = p.AccentText
		}
		Glyph(c, ic, 20, col)
		ui.Column(c).Gap(1).MinWidth(0).Children(func() {
			ui.Text(c, label).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine()
			if sub != "" {
				ui.Text(c, sub).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine()
			}
		})
		ui.Spacer(c).MinWidth(8)
		dot := ui.Row(c).Size(18, 18).Radius(9).Center()
		if selected {
			dot.Background(p.Accent)
			Pop(Glyph(c.Key("on"), icons.Check, 11, p.TextOnAccent))
		} else {
			dot.Border(1.5, p.BorderStrong)
		}
	})
	return b
}

// Tone is a semantic status color.
type Tone int

const (
	ToneNeutral Tone = iota
	ToneAccent
	ToneSuccess
	ToneWarning
	ToneDanger
)

// ToneColors returns the foreground and soft background of a tone.
func ToneColors(p *design.Palette, t Tone) (fg, bg ui.Color) {
	switch t {
	case ToneAccent:
		return p.AccentText, p.AccentSoft
	case ToneSuccess:
		return p.Success, p.SuccessSoft
	case ToneWarning:
		return p.Warning, p.WarningSoft
	case ToneDanger:
		return p.Danger, p.DangerSoft
	}
	return p.TextSecondary, p.Fill
}

// StatusDot is a colored dot and label.
func StatusDot(c *ui.Context, label string, tone Tone) ui.Element {
	p := design.Of(c)
	fg, _ := ToneColors(p, tone)
	dot := fg
	if tone == ToneNeutral {
		dot = p.TextTertiary
	}
	return ui.Row(c).Gap(7).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Box(c).Size(7, 7).Radius(3.5).Background(dot)
		ui.Text(c, label).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine()
	})
}

// StatusPill is a compact pill label.
func StatusPill(c *ui.Context, label string, tone Tone) ui.Element {
	p := design.Of(c)
	fg, bg := ToneColors(p, tone)
	return ui.Row(c).Height(20).PaddingX(8).Gap(5).Radius(6).Background(bg).AlignItems(ui.Center).Shrink(0).AlignSelf(ui.Start).Children(func() {
		ui.Text(c, label).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).TextColor(fg).SingleLine()
	})
}

// Note is an inline callout: a tinted rule on the left, quiet text.
func Note(c *ui.Context, ic *ui.SVG, text string, tone Tone) ui.Element {
	p := design.Of(c)
	fg, bg := ToneColors(p, tone)
	return ui.Row(c).Gap(10).Padding(10, 12).Radius(design.RadiusControl + 2).Background(bg).AlignItems(ui.Start).Children(func() {
		Glyph(c, ic, 15, fg).Margin(1, 0, 0, 0)
		ui.Text(c, text).FontSize(design.FontSmall).TextColor(p.Text).LineHeight(1.4).Grow(1).MinWidth(0)
	})
}

// EmptyState is a centered, calm empty message on a patch of the
// workbench dot grid.
func EmptyState(c *ui.Context, ic *ui.SVG, title, body string, actions func()) {
	p := design.Of(c)
	ui.Column(c).AlignItems(ui.Center).Gap(8).Padding(44, 24).Children(func() {
		tile := ui.Row(c).Size(52, 52).Radius(14).Center().Background(p.Raised).Border(0.5, p.GroupBorder).Margin(0, 0, 8, 0)
		design.Elevate(c, tile, design.Floating)
		tile.Children(func() { Glyph(c, ic, 22, p.TextSecondary) })
		ui.Text(c, title).FontSize(design.FontTitle3).FontWeight(design.WeightSemibold).TextColor(p.Text).TextAlign(ui.Center)
		if body != "" {
			ui.Text(c, body).FontSize(design.FontBody).TextColor(p.TextSecondary).TextAlign(ui.Center).MaxWidth(380).LineHeight(1.45)
		}
		if actions != nil {
			ui.Row(c).Gap(8).Margin(10, 0, 0, 0).Children(actions)
		}
	})
}

// Hairline is a 0.5 DIP separator.
func Hairline(c *ui.Context) ui.Element {
	return ui.Box(c).Height(0.5).FillWidth().Background(design.Of(c).Divider).Shrink(0)
}

// Card is a raised surface with a hairline, elevation and inner
// highlight.
func Card(c *ui.Context) ui.Element {
	p := design.Of(c)
	b := ui.Column(c).Background(p.Raised).Border(0.5, p.GroupBorder).Radius(design.RadiusPanel)
	return design.Elevate(c, b, design.Lifted)
}
