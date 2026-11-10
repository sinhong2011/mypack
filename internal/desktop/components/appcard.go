package components

import (
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
)

// AppCardData is what an AppCard / AppRow shows.
type AppCardData struct {
	Icon     []byte
	Name     string
	Domain   string
	Meta     string // e.g. "Built 2 hours ago"
	Status   string
	Tone     Tone
	Busy     bool   // a build is running
	Progress float64
	Primary  string // primary inline action label (list rows), "" = none
	Menu     func(m *ui.Menu)
	MoreText string // a11y label of the more button
}

// AppCardResult reports intent.
type AppCardResult struct {
	Open    bool // click / double click
	Primary bool
}

// AppCard is the grid tile for a project or installed app.
func AppCard(c *ui.Context, d AppCardData) AppCardResult {
	p := design.Of(c)
	var res AppCardResult
	card := ui.ButtonBase(c).Label(d.Name).Column().Padding(16).Gap(0).Radius(design.RadiusCard).AlignItems(ui.Stretch).Justify(ui.Start).
		Background(p.Surface).Border(1, p.Border).Cursor(ui.CursorPointer).Transition(design.Transition(c)).
		Height(172)
	hov := card.Hovered()
	if hov {
		card.Border(1, p.BorderStrong).Shadow(0, 4, 14, 0, p.Shadow).Shadow(0, 1, 2, 0, p.Shadow)
	} else {
		card.Shadow(0, 1, 2, 0, p.Shadow)
	}
	if d.Menu != nil {
		card.ContextMenu(d.Menu)
	}
	card.Children(func() {
		ui.Row(c).AlignItems(ui.Start).Children(func() {
			AppIcon(c, d.Icon, d.Name, 48)
			ui.Spacer(c)
			if d.Menu != nil {
				more := IconButton(c.Key("more"), lobeui.IconMore(), d.MoreText).Menu(d.Menu).Margin(-6, -8, 0, 0)
				if !hov && !more.Focused() {
					more.Opacity(0)
				}
			}
		})
		ui.Spacer(c)
		ui.Text(c, d.Name).FontSize(design.FontBodyLG).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine().MaxLines(1)
		ui.Text(c, d.Domain).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine().MaxLines(1).Margin(2, 0, 0, 0)
		if d.Meta != "" {
			ui.Text(c, d.Meta).FontSize(design.FontCaption).TextColor(p.TextTertiary).SingleLine().MaxLines(1).Margin(1, 0, 0, 0)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(10, 0, 0, 0).Children(func() {
			if d.Busy {
				Spinner(c, 12, p.AccentText)
				ui.Text(c, d.Status).FontSize(design.FontCaption).FontWeight(design.WeightMedium).TextColor(p.AccentText).SingleLine()
			} else if d.Status != "" {
				StatusPill(c, d.Status, d.Tone)
			}
		})
	})
	res.Open = card.Clicked()
	return res
}

// AppRow is the list-view row for a project or installed app.
func AppRow(c *ui.Context, d AppCardData) AppCardResult {
	p := design.Of(c)
	var res AppCardResult
	row := ui.ButtonBase(c).Label(d.Name).Row().Height(design.RowTall).PaddingX(12).Gap(12).AlignItems(ui.Center).Justify(ui.Start).
		Radius(design.RadiusControl + 1).Cursor(ui.CursorPointer).Transition(design.Transition(c))
	hov := row.Hovered()
	if hov {
		row.Background(p.Hover)
	}
	if d.Menu != nil {
		row.ContextMenu(d.Menu)
	}
	row.Children(func() {
		AppIcon(c, d.Icon, d.Name, 34)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, d.Name).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
			ui.Text(c, d.Domain).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine()
		})
		ui.Row(c).Width(150).Shrink(0).Children(func() {
			if d.Busy {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					Spinner(c, 12, p.AccentText)
					ui.Text(c, d.Status).FontSize(design.FontCaption).FontWeight(design.WeightMedium).TextColor(p.AccentText).SingleLine()
				})
			} else if d.Status != "" {
				StatusPill(c, d.Status, d.Tone)
			}
		})
		ui.Text(c, d.Meta).FontSize(design.FontSmall).TextColor(p.TextTertiary).SingleLine().Width(140).Shrink(0)
		ui.Row(c).Width(96).Shrink(0).Justify(ui.End).Gap(2).AlignItems(ui.Center).Children(func() {
			if d.Primary != "" && hov {
				res.Primary = Button(c.Key("primary"), d.Primary, ButtonOpts{Size: design.ControlSM}).Clicked()
			}
			if d.Menu != nil {
				IconButton(c.Key("more"), lobeui.IconMore(), d.MoreText).Menu(d.Menu)
			}
		})
	})
	res.Open = row.Clicked()
	return res
}
