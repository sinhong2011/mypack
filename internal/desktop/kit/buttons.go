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
	radius := design.RadiusControl
	if h >= design.ControlXL {
		radius = design.RadiusCard
	}
	if o.Variant == Glass || o.Variant == GlassProminent {
		radius = h / 2
	}
	b := ui.ButtonBase(c).Label(label).Row().Height(h).Gap(6).AlignItems(ui.Center).Justify(ui.Center).
		Radius(radius).FontSize(fs).FontWeight(design.WeightMedium).
		Cursor(ui.CursorPointer).Transition(design.Transition(c)).Shrink(0)
	padX := max(12, h*0.45)
	if label == "" {
		padX = 0
		b.Width(h)
	}
	if o.Variant == Plain {
		padX = 2
		b.Radius(4)
	}
	b.PaddingX(padX)
	if o.Grow {
		b.Grow(1)
	}
	disabled := o.Disabled || o.Loading
	b.Disabled(disabled)
	hov, pr := b.Hovered() && !disabled, b.Pressed() && !disabled
	var fg ui.Color
	switch o.Variant {
	case Primary, Danger, GlassProminent:
		base := p.Accent
		if o.Variant == Danger {
			base = design.Red[1]
		}
		if hov {
			base = base.Mix(ui.RGB(255, 255, 255), 0.08)
		}
		if pr {
			base = base.Mix(ui.RGB(0, 0, 0), 0.1)
		}
		b.LinearGradient(ui.LinearGradient{From: base.Mix(ui.RGB(255, 255, 255), 0.07), To: base, Angle: 180}).
			InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.2)).
			Shadow(0, 1, 2, 0, design.Fade(base, 0.35)).Shadow(0, 3, 8, -2, design.Fade(base, 0.3))
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
			fg = p.AccentHover
		}
	default:
		bg := p.ButtonFill
		if hov {
			bg = p.ButtonHover
		}
		if pr {
			bg = p.Pressed.Over(p.ButtonFill)
		}
		b.Background(bg).Border(0.5, p.BorderStrong)
		design.Elevate(c, b, design.Lifted)
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
	b := ui.ButtonBase(c).Label(title).Size(28, 28).Center().Radius(design.RadiusControl).
		Cursor(ui.CursorPointer).Transition(design.Transition(c)).Shrink(0)
	fg := p.TextSecondary
	if b.Hovered() {
		b.Background(p.Hover)
		fg = p.Text
	}
	if b.Pressed() {
		b.Background(p.Pressed)
	}
	Press(b, false, b.Pressed(), 0)
	b.Children(func() { Glyph(c, ic, 16, fg) })
	b.Tooltip(title)
	return b
}

// GlassIcon is a round toolbar button holding one icon: a raised chip, or
// the accent chip when prominent.
func GlassIcon(c *ui.Context, ic *ui.SVG, title, hotkey string, prominent bool) ui.Element {
	p := design.Of(c)
	const s = 32
	b := ui.ButtonBase(c).Label(title).Size(s, s).Center().Radius(design.RadiusControl + 1).Cursor(ui.CursorPointer).Shrink(0).
		Transition(design.Transition(c))
	fg := p.Text
	hov, pr := b.Hovered(), b.Pressed()
	if prominent {
		base := p.Accent
		if hov {
			base = p.AccentHover
		}
		b.LinearGradient(ui.LinearGradient{From: base.Mix(ui.RGB(255, 255, 255), 0.08), To: base, Angle: 180}).
			InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.22)).Shadow(0, 2, 6, -1, design.Fade(base, 0.45))
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
	b := ui.ButtonBase(c).Label(title).Size(28, 28).Center().Radius(design.RadiusControl).Cursor(ui.CursorPointer).Shrink(0)
	at := ui.Local(b, "copied", func() time.Time { return time.Time{} })
	done := time.Since(*at) < 1400*time.Millisecond
	if b.Hovered() {
		b.Background(p.Hover)
	}
	b.Children(func() {
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
