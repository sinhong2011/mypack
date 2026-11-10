package components

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
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
	Detail   string  // right-aligned detail, e.g. "4.2 of 9.8 MB"
	State    StepState
	Progress float64 // 0..1 when measurable, <0 otherwise (active only)
}

// BuildTimeline renders the pipeline stages vertically with honest
// progress: a bar only where the stage has a trustworthy denominator.
func BuildTimeline(c *ui.Context, steps []Step) ui.Element {
	p := design.Of(c)
	return ui.Column(c).Gap(0).Role(ui.RoleList).Children(func() {
		for i, s := range steps {
			last := i == len(steps)-1
			ui.Row(c.Key(i)).AlignItems(ui.Stretch).Gap(12).Role(ui.RoleListItem).Children(func() {
				// Rail: glyph + connector.
				ui.Column(c).Width(20).AlignItems(ui.Center).Shrink(0).Children(func() {
					ui.Row(c).Size(20, 20).Center().Children(func() { stepGlyph(c, p, s.State) })
					if !last {
						col := p.Border
						if s.State == StepDone {
							col = p.Accent.Alpha(0.45)
						}
						ui.Box(c).Width(1.5).Grow(1).MinHeight(10).Background(col).Margin(2, 0)
					}
				})
				bottom := float32(14)
				if last {
					bottom = 0
				}
				ui.Column(c).Grow(1).MinWidth(0).Padding(1, 0, bottom, 0).Gap(6).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
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
						lobeui.Progress(c, s.Progress)
					}
				})
			})
		}
	})
}

func stepGlyph(c *ui.Context, p *design.Palette, st StepState) {
	switch st {
	case StepDone:
		ui.Row(c).Size(18, 18).Radius(9).Background(p.Accent).Center().Children(func() {
			ui.Icon(c, lobeui.IconCheck()).FontSize(12).TextColor(p.TextOnAccent)
		})
	case StepActive:
		ui.Box(c).Size(18, 18).Radius(9).Border(2, p.AccentSoft).Children(func() {
			ring := ui.Box(c).Absolute().Top(-2).Left(-2).Size(18, 18).Radius(9).Border(2, ui.Transparent).
				BorderConicGradient(ui.ConicGradient{Stops: []ui.GradientStop{{Color: p.Accent.Alpha(0), At: 0}, {Color: p.Accent, At: 0.75}, {Color: p.Accent, At: 1}}})
			ring.Rotate(360 * ring.Loop("spin", 1000e6, ui.Linear))
		})
	case StepFailed:
		ui.Row(c).Size(18, 18).Radius(9).Background(p.Danger).Center().Children(func() {
			ui.Icon(c, lobeui.IconClose()).FontSize(12).TextColor(ui.RGB(255, 255, 255))
		})
	case StepSkipped:
		ui.Box(c).Size(8, 8).Radius(4).Background(p.Border)
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
	return ui.Scroll(c).MaxHeight(maxHeight).Background(p.Well).Radius(design.RadiusControl+1).Border(1, p.Border).
		Children(func() {
			ui.Column(c).Padding(10, 12).Gap(3).Selectable().Children(func() {
				if len(lines) == 0 {
					Mono(c, "—")
				}
				for i, l := range lines {
					ui.Row(c.Key(i)).Gap(10).AlignItems(ui.Start).Children(func() {
						ui.Text(c, l.Time).Font("Menlo").FontSize(11).TextColor(p.TextTertiary).Shrink(0)
						col := p.TextSecondary
						switch strings.ToLower(l.Level) {
						case "error":
							col = p.Danger
						case "warn":
							col = p.Warning
						}
						ui.Text(c, l.Text).Font("Menlo").FontSize(11).TextColor(col).Grow(1).MinWidth(0)
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
	Card(c).Padding(20).Gap(16).Children(func() {
		ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
			IconBadge(c, lobeui.IconDownload(), ToneAccent, 36)
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				Heading(c, d.Title)
				ui.Text(c, d.Body).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.4)
			})
		})
		ui.Grid(c).ColumnTracks(ui.FitContent(), ui.Fr(1)).GapX(16).GapY(6).Padding(12, 14).
			Background(p.Well).Radius(design.RadiusControl + 1).Children(func() {
			kv := func(k, v string, mono bool) {
				ui.Text(c, k).FontSize(design.FontSmall).TextColor(p.TextTertiary)
				if mono {
					Mono(c, v).MinWidth(0)
				} else {
					ui.Text(c, v).FontSize(design.FontSmall).TextColor(p.Text).MinWidth(0)
				}
			}
			kv(d.SizeLabel, d.Size, false)
			kv(d.SourceLabel, d.Source, false)
			kv(d.CacheLbl, d.Cache, true)
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			cancel = Button(c.Key("cancel"), d.Cancel, ButtonOpts{}).Clicked()
			approve = Button(c.Key("approve"), d.Approve, ButtonOpts{Variant: Primary, Loading: d.Busy, Icon: lobeui.IconDownload()}).Clicked() || (!d.Busy && c.Shortcut(0, ui.KeyEnter))
		})
	})
	return
}

// ErrorInfo is the content of an ErrorPanel: what happened → why → how to
// fix → retry (spec §9.5).
type ErrorInfo struct {
	What, Why, Fix  string
	WhyLabel        string
	FixLabel        string
	Code            string
	Note            string // e.g. "Your previous build is untouched."
	Retry, Copy     string
	Extra           string // optional third action label (e.g. View Log)
	CopiedDiagnostic string
}

// ErrorPanel renders a structured failure with Retry and Copy Diagnostic.
func ErrorPanel(c *ui.Context, e ErrorInfo) (retry, copy, extra bool) {
	p := design.Of(c)
	Card(c).Padding(20).Gap(14).Border(1, p.Danger.Alpha(0.28)).Children(func() {
		ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
			IconBadge(c, lobeui.IconError(), ToneDanger, 36)
			ui.Column(c).Grow(1).MinWidth(0).Gap(10).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					Heading(c, e.What).Grow(1).MinWidth(0)
					if e.Code != "" {
						ui.Text(c, e.Code).Font("Menlo").FontSize(10.5).TextColor(p.TextTertiary).
							Padding(2, 6).Radius(4).Background(p.Hover).Shrink(0)
					}
				})
				if e.Why != "" {
					ui.Column(c).Gap(2).Children(func() {
						Overline(c, e.WhyLabel)
						ui.Text(c, e.Why).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.4).Selectable()
					})
				}
				if e.Fix != "" {
					ui.Column(c).Gap(2).Children(func() {
						Overline(c, e.FixLabel)
						ui.Text(c, e.Fix).FontSize(design.FontBody).TextColor(p.Text).LineHeight(1.4).Selectable()
					})
				}
			})
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if e.Note != "" {
				ui.Icon(c, lobeui.IconShield()).FontSize(14).TextColor(p.TextTertiary)
				Caption(c, e.Note).MinWidth(0).Shrink(1)
			}
			ui.Spacer(c)
			if e.Extra != "" {
				extra = Button(c.Key("extra"), e.Extra, ButtonOpts{Variant: Ghost, Icon: lobeui.IconLog()}).Clicked()
			}
			copy = Button(c.Key("copy"), e.Copy, ButtonOpts{Icon: lobeui.IconCopy()}).Clicked()
			if e.Retry != "" {
				retry = Button(c.Key("retry"), e.Retry, ButtonOpts{Variant: Primary, Icon: lobeui.IconRetry()}).Clicked()
			}
		})
	})
	return
}
