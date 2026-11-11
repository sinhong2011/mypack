package kit

import (
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit/icons"
)

// Variant is a button style.
type Variant int

const (
	Secondary_     Variant = iota // raised neutral face with a hairline
	Primary                       // solid brand blue
	Ghost                         // borderless: fill on hover only
	Danger                        // solid destructive
	Plain                         // text link in the accent color
	Glass                         // toolbar: quiet raised chip
	GlassProminent                // toolbar: accent chip
)

// ButtonOpts configures Button.
type ButtonOpts struct {
	Variant  Variant
	Size     float32 // height; 0 = design.Control (30)
	Icon     *ui.SVG
	Trailing *ui.SVG
	Disabled bool
	Loading  bool
	Grow     bool
	Tooltip  string
	Hotkey   string // tooltip hotkey hint, e.g. "mod+b"
	Danger   bool   // red text on a neutral face (destructive secondary)
}

// PillRadius is the one radius rule for buttons: every button is a pill
// (half its height) and every icon-only button a circle.
func PillRadius(h float32) float32 { return h / 2 }

// btnGeom is a button's box: its height, its radius, its width when it is
// icon-only (a circle), else its side padding.
type btnGeom struct{ h, radius, w, padX float32 }

// buttonGeometry gives every variant and size its box by the pill rule.
func buttonGeometry(v Variant, h float32, iconOnly bool) btnGeom {
	g := btnGeom{h: h, radius: PillRadius(h), padX: max(12, h*0.5)}
	switch {
	case iconOnly:
		g.w, g.padX = h, 0
	case v == Plain:
		g.padX = max(8, h*0.3) // a link with a hover fill: still a pill
	}
	return g
}

// FocusMark shows keyboard focus on a control without a ring: the caller
// gives it the hover fill, and this adds a short 1.5 pt accent underline
// centred at its foot. Call it inside the control's children, with its
// FocusVisible; width is the underline's (0: 18 pt).
func FocusMark(c *ui.Context, show bool, width float32) {
	if !show {
		return
	}
	if width <= 0 || width > 18 {
		width = 18
	}
	p := design.Of(c)
	ui.Row(c.Key("focus")).Absolute().Left(0).Right(0).Bottom(2.5).Justify(ui.Center).PassThrough().AccessHidden(true).Children(func() {
		ui.Box(c).Width(width).Height(1.5).Radius(0.75).Background(p.Accent)
	})
}

// Button is mypack's button. Hover brightens, press sinks on a spring,
// and Loading morphs the label into a spinner in the same slot (the button
// keeps its width).
func Button(c *ui.Context, label string, o ButtonOpts) ui.Element {
	p := design.Of(c)
	h := o.Size
	if h == 0 {
		h = design.Control
	}
	fs := design.FontBody
	switch {
	case h <= design.ControlSM:
		fs = design.FontSmall
	case h >= design.ControlXL:
		fs = design.FontTitle3
	case h >= design.ControlLG:
		fs = design.FontBodyLG
	}
	g := buttonGeometry(o.Variant, h, label == "")
	b := ui.ButtonBase(c).Label(label).Row().Height(g.h).Gap(6).AlignItems(ui.Center).Justify(ui.Center).
		Radius(g.radius).FontSize(fs).FontWeight(design.WeightMedium).
		Cursor(ui.CursorPointer).Transition(design.Transition(c)).Shrink(0).FocusRing(false)
	padX := g.padX
	if g.w > 0 {
		b.Width(g.w)
	}
	b.PaddingX(padX)
	if o.Grow {
		b.Grow(1)
	}
	disabled := o.Disabled || o.Loading
	b.Disabled(disabled)
	kbd := b.FocusVisible() && !disabled
	hov, pr := (b.Hovered() || kbd) && !disabled, b.Pressed() && !disabled
	var fg ui.Color
	switch o.Variant {
	case Primary, Danger, GlassProminent:
		// Flat: the accent (or red), a step lighter on hover and darker
		// when pressed. No gradient, no highlight, no glow.
		bg := p.Accent
		if o.Variant == Danger {
			bg = p.Danger
		}
		switch {
		case pr:
			bg = bg.MixOklab(ui.RGB(0, 0, 0), 0.08)
		case hov:
			bg = bg.MixOklab(ui.RGB(255, 255, 255), 0.08)
		}
		b.Background(bg)
		fg = p.TextOnAccent
		b.FontWeight(design.WeightSemibold)
	case Ghost:
		fg = p.TextSecondary
		if hov {
			b.Background(p.Hover)
			fg = p.Text
		}
		if pr {
			b.Background(p.Pressed)
		}
	case Plain:
		fg = p.AccentText
		if hov {
			b.Background(p.Hover)
		}
	default:
		// Secondary: a flat subtle fill with a 1 px hairline.
		bg := p.ButtonFill
		if hov {
			bg = p.ButtonHover
		}
		if pr {
			bg = bg.MixOklab(ui.RGB(0, 0, 0), 0.06)
		}
		b.Background(bg).Border(1, p.ButtonStroke)
		fg = p.Text
		if o.Danger {
			fg = p.Danger
		}
	}
	Press(b, false, pr, 0)
	if o.Disabled && !o.Loading {
		b.Opacity(0.45)
	}
	b.TextColor(fg)
	isz := fs + 1
	b.Children(func() {
		FocusMark(c, kbd, g.h/2)
		// The content fades out under the spinner while loading, keeping
		// the button's width.
		show := float32(1)
		if o.Loading {
			show = 0
		}
		content := ui.Row(c).Gap(6).AlignItems(ui.Center)
		content.Opacity(content.AnimateTo("show", show, design.SpringSoft))
		content.Children(func() {
			if o.Icon != nil {
				Glyph(c, o.Icon, isz, fg)
			}
			if label != "" {
				ui.Text(c, label).SingleLine()
			}
			if o.Trailing != nil {
				Glyph(c, o.Trailing, isz-1, fg).Opacity(0.85)
			}
		})
		if o.Loading {
			ui.Row(c).Absolute().Fill().Center().Children(func() { Spinner(c, isz, fg) })
		}
	})
	tip := o.Tooltip
	if tip == "" && o.Hotkey != "" {
		tip = label
	}
	if tip != "" {
		if hk := HotkeyText(o.Hotkey); hk != "" {
			tip += "  " + hk
		}
		b.Tooltip(tip)
	}
	return b
}

// IconButton is a borderless square icon button with a tooltip.
func IconButton(c *ui.Context, ic *ui.SVG, title string) ui.Element {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(title).Size(28, 28).Center().Radius(PillRadius(28)).FocusRing(false).
		Cursor(ui.CursorPointer).Transition(design.Transition(c)).Shrink(0)
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

// GlassIcon is a round toolbar button holding one icon: a raised chip, or
// the accent chip when prominent.
func GlassIcon(c *ui.Context, ic *ui.SVG, title, hotkey string, prominent bool) ui.Element {
	p := design.Of(c)
	const s = design.ToolbarControl
	b := ui.ButtonBase(c).Label(title).Size(s, s).Center().Radius(PillRadius(s)).FocusRing(false).Cursor(ui.CursorPointer).Shrink(0).
		Transition(design.Transition(c))
	fg := p.Text
	hov, pr := b.Hovered() || b.FocusVisible(), b.Pressed()
	if prominent {
		bg := p.Accent
		switch {
		case pr:
			bg = bg.MixOklab(ui.RGB(0, 0, 0), 0.08)
		case hov:
			bg = bg.MixOklab(ui.RGB(255, 255, 255), 0.08)
		}
		b.Background(bg)
		fg = p.TextOnAccent
	} else {
		fg = p.TextSecondary
		if hov {
			b.Background(p.Hover)
			fg = p.Text
		}
		if pr {
			b.Background(p.Pressed)
		}
	}
	Press(b, false, pr, 0)
	b.Children(func() {
		FocusMark(c, b.FocusVisible(), s/2-5)
		g := Glyph(c, ic, 17, fg)
		if prominent {
			// The plus turns a quarter on hover: the same strokes, a hint
			// of "make".
			turn := float32(0)
			if hov {
				turn = 90
			}
			g.Rotate(g.AnimateTo("turn", turn, design.Spring))
		}
	})
	tip := title
	if hk := HotkeyText(hotkey); hk != "" {
		tip += "  " + hk
	}
	b.Tooltip(tip)
	return b
}

// CopyButton copies content to the clipboard and reports it; its icon
// becomes a check for a moment.
func CopyButton(c *ui.Context, content, title string) bool {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(title).Size(28, 28).Center().Radius(PillRadius(28)).FocusRing(false).Cursor(ui.CursorPointer).Shrink(0)
	at := ui.Local(b, "copied", func() time.Time { return time.Time{} })
	done := time.Since(*at) < 1400*time.Millisecond
	if b.Hovered() || b.FocusVisible() {
		b.Background(p.Hover)
	}
	b.Children(func() {
		FocusMark(c, b.FocusVisible(), 9)
		if done {
			Pop(Glyph(c.Key("ok"), icons.Check, 15, p.Success))
			c.After(1400*time.Millisecond - time.Since(*at))
		} else {
			Glyph(c.Key("cp"), icons.Copy, 15, p.TextSecondary)
		}
	})
	b.Tooltip(title)
	if b.Clicked() {
		c.WriteClipboard(content)
		*at = time.Now()
		return true
	}
	return false
}

// Kbd shows a keyboard shortcut as key chips.
func Kbd(c *ui.Context, keys string) ui.Element {
	p := design.Of(c)
	return ui.Text(c, HotkeyText(keys)).FontSize(design.FontCaption).FontWeight(design.WeightMedium).TextColor(p.TextSecondary).
		Padding(1, 6).Radius(5).Background(p.Fill).Border(0.5, p.Border).AccessHidden(true)
}
