package kit

import (
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit/icons"
)

// Sheet shows a modal sheet while *open is true: the window dims, and an
// overlay panel drops in from under the toolbar on a soft spring (and
// leaves the same way). Escape or a click outside closes it; the dialog
// keeps the keyboard (focus trap) while open.
func Sheet(c *ui.Context, open *bool, width float32, body func()) ui.Element {
	p := design.Of(c)
	return ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		fade := design.Tween(design.MotionBase, design.EaseStd)
		backdrop.Background(p.Scrim).Justify(ui.Start).Padding(design.TitleBarH+4, 24, 24, 24).
			Transition(ui.ElementTransition{Animation: &fade, Enter: &ui.Motion{}, Exit: &ui.Motion{}})
		panel.Width(width).MaxWidthPercent(100).Padding(22, 22, 18, 22).Gap(14).Radius(design.RadiusPanel).
			Background(p.Overlay).Border(0.5, p.BorderStrong).
			Transition(ui.ElementTransition{Animation: &design.SpringSoft, Enter: &ui.Motion{Y: -14, Scale: 0.985}, Exit: &ui.Motion{Y: -8, Scale: 0.985}})
		design.Elevate(c, panel, design.Modal)
		body()
	})
}

// SheetHeader is a sheet's title (and optional lead on the left).
func SheetHeader(c *ui.Context, title string, lead func()) {
	p := design.Of(c)
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		if lead != nil {
			lead()
		}
		ui.Text(c, title).FontSize(design.FontTitle3).FontWeight(design.WeightBold).TextColor(p.Text).Role(ui.RoleHeading).
			LetterSpacing(design.TrackTitle).Grow(1).MinWidth(0)
	})
}

// SheetBody is a sheet's explanatory text.
func SheetBody(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(design.FontBody).TextColor(design.Of(c).TextSecondary).LineHeight(1.45)
}

// SheetFooter draws right-aligned Cancel + default action buttons (the
// default answers Return). It reports which was chosen.
func SheetFooter(c *ui.Context, cancel, action string, v Variant, busy, disabled bool) (ok, dismissed bool) {
	ui.Row(c).Gap(8).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
		dismissed = Button(c.Key("cancel"), cancel, ButtonOpts{}).MinWidth(84).Clicked()
		ok = Button(c.Key("ok"), action, ButtonOpts{Variant: v, Loading: busy, Disabled: disabled}).MinWidth(84).Clicked() ||
			(!busy && !disabled && c.Shortcut(0, ui.KeyEnter))
	})
	return
}

// KeyValues is a two-column list of labels and values in a group box.
func KeyValues(c *ui.Context, rows [][2]string, mono map[int]bool) ui.Element {
	p := design.Of(c)
	return GroupBox(c).Children(func() {
		for i, kv := range rows {
			ui.Row(c.Key(i)).MinHeight(32).Padding(7, 12).Gap(16).AlignItems(ui.Center).Children(func() {
				ui.Text(c, kv[0]).FontSize(design.FontSmall).TextColor(p.TextSecondary).Width(110).Shrink(0)
				if mono[i] {
					Mono(c, kv[1]).MinWidth(0).Grow(1).Selectable()
				} else {
					ui.Text(c, kv[1]).FontSize(design.FontSmall).TextColor(p.Text).MinWidth(0).Grow(1).Selectable()
				}
			})
		}
	})
}

// Strings are the few words the kit says itself, set by the app in the
// user's language.
var Strings = struct{ Close string }{Close: "Close"}

// Toasts draws the window's toasts: overlay cards stacked at the bottom
// center, each sliding up on a spring and leaving downward, with an icon
// for its type and an optional action.
func Toasts(c *ui.Context) {
	p := design.Of(c)
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Absolute().Left(0).Right(0).Bottom(20).AlignItems(ui.Center).Gap(8).PassThrough()
		for _, t := range toasts {
			parts := ui.ToastBase(c.Key(t.ID), t)
			card := parts.Root.Row().Gap(10).Padding(10, 12, 10, 12).Radius(design.RadiusCard).AlignItems(ui.Center).
				Background(p.Overlay).Border(0.5, p.BorderStrong).MaxWidth(460).
				Transition(ui.ElementTransition{Animation: &design.Spring, Enter: &ui.Motion{Y: 18, Scale: 0.96}, Exit: &ui.Motion{Y: 12, Collapse: true}})
			design.Elevate(c, card, design.Floating)
			card.Children(func() {
				ic, col := icons.Info, p.TextSecondary
				switch t.Type {
				case "success":
					ic, col = icons.Success, p.Success
				case "error":
					ic, col = icons.Alert, p.Danger
				}
				Glyph(c, ic, 16, col)
				ui.Column(c).Gap(1).MinWidth(0).Shrink(1).Children(func() {
					ui.Text(c, t.Title).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text)
					if t.Description != "" {
						ui.Text(c, t.Description).FontSize(design.FontSmall).TextColor(p.TextSecondary)
					}
				})
				if t.Action != "" {
					parts.ActionButton().Height(24).PaddingX(10).Radius(6).Background(p.Fill).Cursor(ui.CursorPointer).Children(func() {
						ui.Text(c, t.Action).FontSize(design.FontSmall).FontWeight(design.WeightSemibold).TextColor(p.AccentText)
					})
				}
				parts.CloseButton().Label(Strings.Close).Size(20, 20).Radius(10).Center().Cursor(ui.CursorPointer).Children(func() {
					Glyph(c, icons.Close, 12, p.TextTertiary)
				})
			})
		}
	})
}
