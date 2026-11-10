// Package components holds mypack-owned product components (spec §08A.1):
// buttons, cards, AppCard, BuildTimeline, the URL hero field, IconPicker,
// TargetChip, RuntimeDownloadCard and ErrorPanel. They render state handed
// to them and report user intent; they never call the application service.
package components

import (
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
)

// ---- Text ---------------------------------------------------------------------

// PageTitle is a screen title.
func PageTitle(c *ui.Context, s string) ui.Element {
	p := design.Of(c)
	return ui.Text(c, s).FontSize(design.FontTitle1).FontWeight(design.WeightSemibold).TextColor(p.Text).LetterSpacing(-0.3).Role(ui.RoleHeading)
}

// Heading is a section heading.
func Heading(c *ui.Context, s string) ui.Element {
	p := design.Of(c)
	return ui.Text(c, s).FontSize(design.FontTitle3).FontWeight(design.WeightSemibold).TextColor(p.Text).Role(ui.RoleHeading)
}

// Body is primary body text.
func Body(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(design.FontBody).TextColor(design.Of(c).Text)
}

// Secondary is muted body text.
func Secondary(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(design.FontBody).TextColor(design.Of(c).TextSecondary)
}

// Caption is small tertiary text.
func Caption(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(design.FontSmall).TextColor(design.Of(c).TextTertiary)
}

// Overline is a small uppercase group label.
func Overline(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).
		TextColor(design.Of(c).TextTertiary).LetterSpacing(0.2)
}

// Mono is monospaced text (paths, hashes, logs).
func Mono(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).Font("Menlo").FontSize(design.FontSmall).TextColor(design.Of(c).TextSecondary)
}

// ---- Buttons ------------------------------------------------------------------

// Variant is a button style.
type Variant int

const (
	Secondary_ Variant = iota // bordered neutral
	Primary
	Ghost
	Danger
	Plain // text-only link style
)

// ButtonOpts configures Button.
type ButtonOpts struct {
	Variant  Variant
	Size     float32 // height; 0 = design.Control
	Icon     *ui.SVG
	Trailing *ui.SVG
	Disabled bool
	Loading  bool
	Grow     bool
	Tooltip  string
	Hotkey   string // tooltip hotkey hint, e.g. "mod+b"
}

// Button is mypack's button: compact, precise, restrained accent.
func Button(c *ui.Context, label string, o ButtonOpts) ui.Element {
	p := design.Of(c)
	h := o.Size
	if h == 0 {
		h = design.Control
	}
	b := ui.ButtonBase(c).Label(label).Row().Height(h).Gap(6).AlignItems(ui.Center).Justify(ui.Center).
		Radius(design.RadiusControl).FontSize(design.FontBody).FontWeight(design.WeightMedium).
		Cursor(ui.CursorPointer).Transition(design.Transition(c)).Shrink(0)
	padX := float32(12)
	if h >= design.ControlLG {
		padX = 16
	}
	if label == "" {
		padX = 0
		b.Width(h)
	}
	if o.Variant == Plain {
		padX = 0
	}
	b.PaddingX(padX)
	if o.Grow {
		b.Grow(1)
	}
	disabled := o.Disabled || o.Loading
	b.Disabled(disabled)
	hov, pr := b.Hovered() && !disabled, b.Pressed() && !disabled
	var bg, fg, border ui.Color
	switch o.Variant {
	case Primary:
		bg, fg = p.Accent, p.TextOnAccent
		if hov {
			bg = p.AccentHover
		}
		if pr {
			bg = p.AccentPressed
		}
		b.Shadow(0, 1, 1.5, 0, ui.RGBA(0, 0, 0, 0.12)).InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.14))
	case Danger:
		bg, fg = p.Danger, ui.RGB(255, 255, 255)
		if hov {
			bg = p.DangerHover
		}
		b.Shadow(0, 1, 1.5, 0, ui.RGBA(0, 0, 0, 0.12))
	case Ghost:
		bg, fg = ui.Transparent, p.TextSecondary
		if hov {
			bg, fg = p.Hover, p.Text
		}
		if pr {
			bg = p.Pressed
		}
	case Plain:
		bg, fg = ui.Transparent, p.AccentText
		if hov {
			fg = p.AccentHover
		}
	default:
		bg, fg, border = p.Surface, p.Text, p.BorderStrong
		if p.Dark {
			bg = p.SurfaceRaised
		}
		if hov {
			bg = theMix(bg, p.Text, 0.04)
		}
		if pr {
			bg = theMix(bg, p.Text, 0.08)
		}
		b.Border(1, border).Shadow(0, 1, 1, 0, p.Shadow)
	}
	if disabled {
		b.Opacity(0.5)
	}
	b.Background(bg).TextColor(fg)
	b.Children(func() {
		switch {
		case o.Loading:
			ui.Icon(c, lobeui.IconLoader()).FontSize(14).TextColor(fg).Rotate(360 * b.Loop("spin", 900e6, ui.Linear))
		case o.Icon != nil:
			ui.Icon(c, o.Icon).FontSize(15).TextColor(fg)
		}
		if label != "" {
			ui.Text(c, label).SingleLine()
		}
		if o.Trailing != nil {
			ui.Icon(c, o.Trailing).FontSize(14).TextColor(fg).Opacity(0.8)
		}
	})
	if o.Tooltip != "" {
		lobeui.Tooltip(c, b, o.Tooltip, o.Hotkey)
	} else if o.Hotkey != "" {
		lobeui.Tooltip(c, b, label, o.Hotkey)
	}
	return b
}

// IconButton is a square, borderless icon button with a tooltip.
func IconButton(c *ui.Context, svg *ui.SVG, title string) ui.Element {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(title).Size(28, 28).Center().Radius(design.RadiusSM).
		Cursor(ui.CursorPointer).Transition(design.Transition(c))
	fg := p.TextSecondary
	if b.Hovered() {
		b.Background(p.Hover)
		fg = p.Text
	}
	if b.Pressed() {
		b.Background(p.Pressed)
	}
	b.Children(func() { ui.Icon(c, svg).FontSize(16).TextColor(fg) })
	lobeui.Tooltip(c, b, title, "")
	return b
}

func theMix(a, b ui.Color, t float32) ui.Color { return a.Mix(b, t) }

// ---- Surfaces -----------------------------------------------------------------

// Card is a bordered surface with the card radius.
func Card(c *ui.Context) ui.Element {
	p := design.Of(c)
	return ui.Column(c).Background(p.Surface).Border(1, p.Border).Radius(design.RadiusCard).
		Shadow(0, 1, 2, 0, p.Shadow)
}

// Hairline is a 1 DIP divider.
func Hairline(c *ui.Context) ui.Element {
	return ui.Box(c).Height(1).FillWidth().Background(design.Of(c).Divider).Shrink(0)
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

func (t Tone) colors(p *design.Palette) (fg, bg ui.Color) {
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
	return p.TextSecondary, p.Hover
}

// ToneColors returns the foreground and soft background of a tone.
func ToneColors(p *design.Palette, t Tone) (fg, bg ui.Color) { return t.colors(p) }

// StatusPill is a compact status label with a dot.
func StatusPill(c *ui.Context, label string, tone Tone) ui.Element {
	p := design.Of(c)
	fg, bg := tone.colors(p)
	return ui.Row(c).Height(20).PaddingX(7).Gap(5).Radius(10).Background(bg).AlignItems(ui.Center).Shrink(0).AlignSelf(ui.Start).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(fg)
		ui.Text(c, label).FontSize(design.FontCaption).FontWeight(design.WeightMedium).TextColor(fg).SingleLine()
	})
}

// IconBadge is a tinted rounded square holding an icon.
func IconBadge(c *ui.Context, svg *ui.SVG, tone Tone, size float32) ui.Element {
	p := design.Of(c)
	fg, bg := tone.colors(p)
	return ui.Row(c).Size(size, size).Radius(size*0.28).Background(bg).Center().Shrink(0).Children(func() {
		ui.Icon(c, svg).FontSize(size * 0.5).TextColor(fg)
	})
}

// Spinner is a small indeterminate spinner.
func Spinner(c *ui.Context, size float32, color ui.Color) ui.Element {
	e := ui.Icon(c, lobeui.IconLoader()).FontSize(size).TextColor(color)
	return e.Rotate(360 * e.Loop("spin", 900e6, ui.Linear))
}

// Kbd shows a keyboard shortcut hint ("mod+n").
func Kbd(c *ui.Context, keys string) ui.Element {
	return lobeui.Hotkey(c, keys).FontSize(design.FontSmall).TextColor(design.Of(c).TextTertiary)
}
