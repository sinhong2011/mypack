package kit

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit/icons"
)

// StepState is one timeline step's state.
type StepState int

const (
	StepPending StepState = iota
	StepActive
	StepDone
	StepFailed
	StepSkipped
)

// Step is one visible pipeline stage.
type Step struct {
	Label    string
	Detail   string // right-aligned detail, e.g. "4.2 of 9.8 MB"
	State    StepState
	Progress float64 // 0..1 when measurable, <0 otherwise (active only)
}

// LogLine is one rendered log line.
type LogLine struct {
	Time, Level, Text string
}

// LogViewer shows structured log lines in a monospaced well.
func LogViewer(c *ui.Context, lines []LogLine, maxHeight float32) ui.Element {
	p := design.Of(c)
	return ui.Scroll(c).MaxHeight(maxHeight).Background(p.Well).Radius(design.RadiusCard).Border(0.5, p.GroupBorder).
		Children(func() {
			ui.Column(c).Padding(10, 14).Gap(3).Selectable().Children(func() {
				if len(lines) == 0 {
					Mono(c, "—")
				}
				for i, l := range lines {
					ui.Row(c.Key(i)).Gap(12).AlignItems(ui.Start).Children(func() {
						ui.Text(c, l.Time).Font(MonoFont).FontSize(11).TextColor(p.TextTertiary).Shrink(0)
						col := p.TextSecondary
						switch strings.ToLower(l.Level) {
						case "error":
							col = p.Danger
						case "warn":
							col = p.Warning
						}
						ui.Text(c, l.Text).Font(MonoFont).FontSize(11).TextColor(col).Grow(1).MinWidth(0)
					})
				}
			})
		}).Label("Build log")
}

// RuntimeDownloadInfo is what the first-time runtime card shows.
type RuntimeDownloadInfo struct {
	Title, Body                      string
	SizeLabel, SourceLabel, CacheLbl string
	Size, Source, Cache              string
	Approve, Cancel                  string
	Busy                             bool
	// Inline draws it as the content of the surface it sits on (the
	// build stage), centred, with no card of its own: a glyph, the
	// title, one sentence, the facts in one quiet line (the cache folder
	// in its tooltip) and the two buttons.
	Inline bool
}

// RuntimeDownloadCard asks once for the runtime download (spec §9C.9).
// It returns (approve, cancel).
func RuntimeDownloadCard(c *ui.Context, d RuntimeDownloadInfo) (approve, cancel bool) {
	p := design.Of(c)
	if d.Inline {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(0).Children(func() {
			Pop(ui.Row(c.Key("glyph")).Size(56, 56).Radius(28).Center().Background(p.AccentSoft)).Children(func() {
				Glyph(c, icons.Download, 26, p.AccentText)
			})
			ui.Text(c, d.Title).FontSize(design.FontTitle2).FontWeight(design.WeightSemibold).TextColor(p.Text).
				TextAlign(ui.Center).Margin(16, 0, 0, 0)
			ui.Text(c, d.Body).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.4).
				TextAlign(ui.Center).MaxWidth(400).Margin(6, 0, 0, 0)
			ui.Text(c, d.Size+" · "+d.Source).FontSize(design.FontSmall).TextColor(p.TextTertiary).FontFeatures("tnum").
				TextAlign(ui.Center).Margin(10, 0, 0, 0).Tooltip(d.CacheLbl + ": " + d.Cache)
			ui.Row(c).Gap(8).Justify(ui.Center).Margin(20, 0, 0, 0).Children(func() {
				cancel = Button(c.Key("cancel"), d.Cancel, ButtonOpts{Variant: Ghost, Size: design.ControlLG}).Clicked() || c.Shortcut(0, ui.KeyEscape)
				approve = Button(c.Key("approve"), d.Approve, ButtonOpts{Variant: Primary, Size: design.ControlLG, Loading: d.Busy, Icon: icons.Download}).Clicked() ||
					(!d.Busy && c.Shortcut(0, ui.KeyEnter))
			})
		})
		return
	}
	Card(c).Padding(22).Gap(16).Children(func() {
		ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				Heading(c, d.Title)
				ui.Text(c, d.Body).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.4)
			})
		})
		KeyValues(c, [][2]string{{d.SizeLabel, d.Size}, {d.SourceLabel, d.Source}, {d.CacheLbl, d.Cache}}, map[int]bool{2: true})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			cancel = Button(c.Key("cancel"), d.Cancel, ButtonOpts{}).Clicked()
			approve = Button(c.Key("approve"), d.Approve, ButtonOpts{Variant: Primary, Loading: d.Busy, Icon: icons.Download}).Clicked() || (!d.Busy && c.Shortcut(0, ui.KeyEnter))
		})
	})
	return
}

// TableColumn is one column of a Table header.
type TableColumn struct {
	Title string
	Width float32 // 0 = flexible
	End   bool    // right-aligned
}

// TableHeader draws a Finder-style column header row.
func TableHeader(c *ui.Context, cols []TableColumn, padX float32) {
	p := design.Of(c)
	ui.Row(c).Height(28).PaddingX(padX).Gap(12).AlignItems(ui.Center).Children(func() {
		for _, col := range cols {
			e := ui.Text(c, col.Title).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).TextColor(p.TextSecondary).SingleLine()
			if col.Width > 0 {
				e.Width(col.Width).Shrink(0)
			} else {
				e.Grow(1).Basis(0).MinWidth(0)
			}
			if col.End {
				e.TextAlign(ui.End)
			}
		}
	})
	Hairline(c)
}

// TableRowBG gives table row i its alternating fill and hover.
func TableRowBG(c *ui.Context, row ui.Element, i int, hovered, selected bool) {
	p := design.Of(c)
	switch {
	case selected:
		row.Background(p.AccentSoft)
	case hovered:
		row.Background(p.Hover)
	case i%2 == 1:
		row.Background(p.Alt)
	}
}
