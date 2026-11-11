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

// BuildTimeline renders the pipeline stages as a compact grouped list with
// honest progress: a bar only where the stage has a trustworthy
// denominator.
func BuildTimeline(c *ui.Context, steps []Step) ui.Element {
	p := design.Of(c)
	return GroupBox(c).Role(ui.RoleList).Children(func() {
		for i, s := range steps {
			ui.Column(c.Key(i)).Padding(9, 14).Gap(6).Role(ui.RoleListItem).Children(func() {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Row(c).Size(18, 18).Center().Shrink(0).Children(func() { stepGlyph(c, p, s.State) })
					fg := p.TextSecondary
					w := design.WeightRegular
					switch s.State {
					case StepActive:
						fg, w = p.Text, design.WeightSemibold
					case StepDone:
						fg = p.Text
					case StepFailed:
						fg, w = p.Danger, design.WeightSemibold
					case StepPending, StepSkipped:
						fg = p.TextTertiary
					}
					ui.Text(c, s.Label).FontSize(design.FontBody).FontWeight(w).TextColor(fg).SingleLine()
					ui.Spacer(c)
					if s.Detail != "" {
						ui.Text(c, s.Detail).FontSize(design.FontSmall).TextColor(p.TextTertiary).SingleLine().FontFeatures("tnum")
					}
				})
				if s.State == StepActive && s.Progress >= 0 {
					ui.Column(c).Padding(0, 0, 2, 28).Children(func() { ProgressBar(c, s.Progress) })
				}
			})
		}
	})
}

func stepGlyph(c *ui.Context, p *design.Palette, st StepState) {
	switch st {
	case StepDone:
		Glyph(c, icons.Success, 17, p.Success)
	case StepActive:
		Spinner(c, 15, p.Accent)
	case StepFailed:
		Glyph(c, icons.Failed, 17, p.Danger)
	case StepSkipped:
		Glyph(c, icons.Skipped, 16, p.TextTertiary)
	default:
		ui.Box(c).Size(14, 14).Radius(7).Border(1.5, p.BorderStrong)
	}
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
}

// RuntimeDownloadCard asks once for the runtime download (spec §9C.9).
// It returns (approve, cancel).
func RuntimeDownloadCard(c *ui.Context, d RuntimeDownloadInfo) (approve, cancel bool) {
	p := design.Of(c)
	Card(c).Padding(22).Gap(16).Children(func() {
		ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
			SymbolTile(c, icons.Download, p.Accent, 40)
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

// ErrorInfo is the content of an ErrorPanel: what happened → why → how to
// fix → retry (spec §9.5).
type ErrorInfo struct {
	What, Why, Fix   string
	WhyLabel         string
	FixLabel         string
	Code             string
	Note             string // e.g. "Your previous build is untouched."
	Retry, Copy      string
	Extra            string // optional third action label (e.g. View Log)
	CopiedDiagnostic string
}

// ErrorPanel renders a structured failure with Retry and Copy Diagnostic.
func ErrorPanel(c *ui.Context, e ErrorInfo) (retry, copy, extra bool) {
	p := design.Of(c)
	Card(c).Padding(22).Gap(16).Children(func() {
		ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
			SymbolTile(c, icons.Alert, p.Danger, 40)
			ui.Column(c).Grow(1).MinWidth(0).Gap(10).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					Heading(c, e.What).Grow(1).MinWidth(0)
					if e.Code != "" {
						ui.Text(c, e.Code).Font(MonoFont).FontSize(10.5).TextColor(p.TextSecondary).
							Padding(2, 7).Radius(5).Background(p.Fill).Shrink(0)
					}
				})
				if e.Why != "" {
					ui.Column(c).Gap(2).Children(func() {
						ui.Text(c, e.WhyLabel).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).TextColor(p.TextTertiary)
						ui.Text(c, e.Why).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.4).Selectable()
					})
				}
				if e.Fix != "" {
					ui.Column(c).Gap(2).Children(func() {
						ui.Text(c, e.FixLabel).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).TextColor(p.TextTertiary)
						ui.Text(c, e.Fix).FontSize(design.FontBody).TextColor(p.Text).LineHeight(1.4).Selectable()
					})
				}
			})
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if e.Note != "" {
				Glyph(c, icons.Shield, 14, p.TextTertiary)
				Caption(c, e.Note).MinWidth(0).Shrink(1)
			}
			ui.Spacer(c)
			if e.Extra != "" {
				extra = Button(c.Key("extra"), e.Extra, ButtonOpts{Variant: Ghost, Icon: icons.Log}).Clicked()
			}
			copy = Button(c.Key("copy"), e.Copy, ButtonOpts{Icon: icons.Copy}).Clicked()
			if e.Retry != "" {
				retry = Button(c.Key("retry"), e.Retry, ButtonOpts{Variant: Primary, Icon: icons.Retry}).Clicked()
			}
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
