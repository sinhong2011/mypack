package components

import (
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
)

// URLHeroOpts configures the hero URL field.
type URLHeroOpts struct {
	Placeholder string
	Label       string
	Loading     bool
	Invalid     bool
	Action      string // embedded trailing button label; "" = none
	ActionOff   bool
	AutoFocus   bool
	Handle      *ui.Handle
	// OnPaste sees a paste before it is inserted (main thread); return true
	// to take it.
	OnPaste func(text string) bool
}

// URLHeroResult reports what happened this frame.
type URLHeroResult struct {
	Changed, Submitted, Action bool
	Dropped                    []string
	DragOver                   bool
}

// URLHero is the big "paste a website" field. It accepts typing, paste
// (auto-detected by the caller) and dropped HTML files/folders.
func URLHero(c *ui.Context, value *string, o URLHeroOpts) URLHeroResult {
	p := design.Of(c)
	var r URLHeroResult
	box := ui.Row(c).Height(50).Padding(0, 6, 0, 16).Gap(10).AlignItems(ui.Center).Radius(12).
		Background(p.SurfaceRaised).Cursor(ui.CursorText).Transition(design.Transition(c))
	var in ui.Element
	box.Children(func() {
		icon := lobeui.IconGlobe()
		if o.Invalid {
			icon = lobeui.IconWarning()
		}
		col := p.TextTertiary
		if o.Invalid {
			col = p.Warning
		}
		ui.Icon(c, icon).FontSize(17).TextColor(col)
		in = ui.TextInputBase(c, value).Grow(1).MinWidth(0).FocusRing(false).FontSize(15).
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
			Spinner(c, 15, p.TextTertiary).Margin(0, 6, 0, 0)
		}
		if o.Action != "" {
			r.Action = Button(c.Key("action"), o.Action, ButtonOpts{Variant: Primary, Size: 36, Disabled: o.ActionOff, Trailing: lobeui.IconArrowRight()}).Clicked()
		} else {
			ui.Box(c).Width(4)
		}
	})
	focused := in.Focused()
	r.DragOver = box.FileDragOver()
	r.Dropped = box.DroppedFiles()
	switch {
	case r.DragOver:
		box.Border(1.5, p.Accent).Background(p.AccentSoft)
	case focused:
		box.Border(1, p.Accent).Shadow(0, 0, 0, 3.5, p.Accent.Alpha(0.16))
	default:
		box.Border(1, p.BorderStrong).Shadow(0, 1, 2, 0, p.Shadow)
	}
	if box.Clicked() {
		in.Focus()
	}
	r.Changed = in.Changed()
	r.Submitted = in.Submitted()
	return r
}

// IconPicker shows the app icon with a menu: choose image, reset.
func IconPicker(c *ui.Context, png []byte, name string, size float32, label string, menu func(m *ui.Menu)) ui.Element {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(label).Size(size, size).Radius(design.AppIconRadius(size)).Cursor(ui.CursorPointer).Shrink(0)
	b.Menu(menu)
	hov := b.Hovered()
	b.Children(func() {
		AppIcon(c, png, name, size)
		if hov {
			ui.Row(c).Absolute().Right(-4).Bottom(-4).Size(22, 22).Radius(11).Background(p.SurfaceRaised).
				Border(1, p.BorderStrong).Center().Shadow(0, 1, 3, 0, p.Shadow).Children(func() {
				ui.Icon(c, lobeui.IconEdit()).FontSize(11).TextColor(p.Text)
			})
		}
	})
	lobeui.Tooltip(c, b, label, "")
	return b
}

// TargetChip is a selectable platform chip with an optional reason.
func TargetChip(c *ui.Context, icon *ui.SVG, label, sub string, selected, disabled bool) ui.Element {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(label).Row().Justify(ui.Start).Height(44).Padding(0, 12, 0, 10).Gap(10).AlignItems(ui.Center).
		Radius(design.RadiusControl + 2).Cursor(ui.CursorPointer).Transition(design.Transition(c)).Disabled(disabled).
		Role(ui.RoleCheckBox).Checked(selected)
	bg, border := p.Surface, p.Border
	if selected {
		bg, border = p.AccentSoft, p.Accent.Alpha(0.55)
	} else if b.Hovered() && !disabled {
		bg = theMix(bg, p.Text, 0.03)
		border = p.BorderStrong
	}
	b.Background(bg).Border(1, border)
	if disabled {
		b.Opacity(0.55).Cursor(ui.CursorNotAllowed)
	}
	b.Children(func() {
		col := p.TextSecondary
		if selected {
			col = p.AccentText
		}
		ui.Icon(c, icon).FontSize(17).TextColor(col)
		ui.Column(c).Gap(1).MinWidth(0).Children(func() {
			ui.Text(c, label).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine()
			if sub != "" {
				ui.Text(c, sub).FontSize(design.FontCaption).TextColor(p.TextTertiary).SingleLine()
			}
		})
		ui.Spacer(c).MinWidth(6)
		if selected {
			ui.Row(c).Size(16, 16).Radius(8).Background(p.Accent).Center().Children(func() {
				ui.Icon(c, lobeui.IconCheck()).FontSize(11).TextColor(p.TextOnAccent)
			})
		} else {
			ui.Box(c).Size(16, 16).Radius(8).Border(1.5, p.BorderStrong)
		}
	})
	return b
}

// Disclosure is a clickable header that toggles *open.
func Disclosure(c *ui.Context, open *bool, title, subtitle string) ui.Element {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(title).Row().Justify(ui.Start).Height(40).PaddingX(4).Gap(8).AlignItems(ui.Center).
		Radius(design.RadiusControl).Cursor(ui.CursorPointer).Role(ui.RoleDisclosure).Expanded(*open)
	if b.Hovered() {
		b.Background(p.Hover)
	}
	b.Children(func() {
		ch := ui.Icon(c, lobeui.IconChevronRt()).FontSize(14).TextColor(p.TextSecondary)
		turn := float32(0)
		if *open {
			turn = 90
		}
		ch.Rotate(ch.AnimateTo("turn", turn, ui.Animation{Duration: design.Dur(c, design.MotionBase), Ease: ui.CubicBezier(0.25, 0.1, 0.25, 1)}))
		ui.Text(c, title).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text)
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(design.FontSmall).TextColor(p.TextTertiary).SingleLine().MinWidth(0).Shrink(1)
		}
	})
	if b.Clicked() {
		*open = !*open
	}
	return b
}

// Group is a settings group card (macOS System Settings style).
func Group(c *ui.Context, title string, body func()) {
	p := design.Of(c)
	ui.Column(c).Gap(8).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(design.FontTitle3).FontWeight(design.WeightSemibold).TextColor(p.Text).Role(ui.RoleHeading)
		}
		ui.Column(c).Background(p.Surface).Border(1, p.Border).Radius(design.RadiusCard - 2).
			Dividers(1, p.Divider).Children(body)
	})
}

// SettingRow is one row in a Group: label + description left, control right.
func SettingRow(c *ui.Context, label, desc string, control func()) {
	p := design.Of(c)
	ui.Row(c).MinHeight(design.RowCompact).Padding(10, 14).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(design.FontBody).TextColor(p.Text)
			if desc != "" {
				ui.Text(c, desc).FontSize(design.FontSmall).TextColor(p.TextTertiary).LineHeight(1.35)
			}
		})
		if control != nil {
			ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
		}
	})
}

// SettingBlock is a full-width row (label above a wide control).
func SettingBlock(c *ui.Context, label, desc string, control func()) {
	p := design.Of(c)
	ui.Column(c).Padding(12, 14).Gap(8).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(design.FontBody).TextColor(p.Text)
			if desc != "" {
				ui.Text(c, desc).FontSize(design.FontSmall).TextColor(p.TextTertiary).LineHeight(1.35)
			}
		})
		control()
	})
}

// Note is an inline callout.
func Note(c *ui.Context, icon *ui.SVG, text string, tone Tone) ui.Element {
	p := design.Of(c)
	fg, bg := tone.colors(p)
	return ui.Row(c).Gap(8).Padding(8, 12).Radius(design.RadiusControl+1).Background(bg).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, icon).FontSize(14).TextColor(fg).Margin(1, 0, 0, 0)
		ui.Text(c, text).FontSize(design.FontSmall).TextColor(p.Text).LineHeight(1.4).Grow(1).MinWidth(0)
	})
}

// EmptyState is a centered illustration-free empty message.
func EmptyState(c *ui.Context, icon *ui.SVG, title, body string, actions func()) {
	p := design.Of(c)
	ui.Column(c).AlignItems(ui.Center).Gap(10).Padding(48, 24).Children(func() {
		ui.Row(c).Size(44, 44).Radius(12).Background(p.Hover).Center().Children(func() {
			ui.Icon(c, icon).FontSize(20).TextColor(p.TextTertiary)
		})
		ui.Text(c, title).FontSize(design.FontTitle3).FontWeight(design.WeightSemibold).TextColor(p.Text).TextAlign(ui.Center)
		if body != "" {
			ui.Text(c, body).FontSize(design.FontBody).TextColor(p.TextSecondary).TextAlign(ui.Center).MaxWidth(380).LineHeight(1.4)
		}
		if actions != nil {
			ui.Row(c).Gap(8).Margin(6, 0, 0, 0).Children(actions)
		}
	})
}
