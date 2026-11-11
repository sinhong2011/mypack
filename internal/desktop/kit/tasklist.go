package kit

import (
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
)

// The task list: a build's steps as a compact live list, the way agent
// apps show their work. One 28 pt row per step: a 16 pt glyph (hollow
// circle, spinner, a check that draws itself on, a red ✕, a dash), the
// label, and on the right the live detail or the time the step took. The
// running row opens under its label for what it is doing (a progress
// hairline, the latest output line); a failed row opens for the error.
// Leading finished steps can fold into one summary row.

// TaskStep is one row of a TaskList.
type TaskStep struct {
	Key      string
	Label    string
	State    StepState
	Right    string    // running: the live detail ("45%"); done: the time ("2.1s")
	DoneAt   time.Time // when it finished (its check draws on from here)
	Progress float64   // running: 0…1 for a hairline under the label, <0 none
	Line     string    // running: the latest output line, in monospace
	Error    string    // failed: what went wrong
	Log      bool      // offer "View log" in the opened row
	Body     func()    // failed: drawn in the opened row instead of Error
}

// TaskListOpts configures a TaskList.
type TaskListOpts struct {
	Compact  bool   // no opened rows (the flyout, the HUD)
	OnDark   bool   // white text on a black surface (the HUD)
	Fold     int    // leading steps hidden behind the summary row
	Summary  string // the summary row ("3 steps done · 4.2s"); a click folds or unfolds
	ViewLog  string // the "View log" label
	MaxWidth float32
}

// TaskListResult is what the person did in a TaskList this frame.
type TaskListResult struct {
	Summary bool // the summary row was clicked (fold or unfold)
	ViewLog bool
}

// checkDraw is how long a finished step's check takes to draw on.
const checkDraw = 220 * time.Millisecond

// TaskList draws the steps.
func TaskList(c *ui.Context, steps []TaskStep, o TaskListOpts) (res TaskListResult) {
	p := design.Of(c)
	text, second, third := p.Text, p.TextSecondary, p.TextTertiary
	if o.OnDark {
		text, second, third = ui.RGB(255, 255, 255), ui.RGBA(255, 255, 255, 0.62), ui.RGBA(255, 255, 255, 0.4)
	}
	w := o.MaxWidth
	if w == 0 {
		w = 520
	}
	rowH := float32(28)
	if o.Compact {
		rowH = 22
	}
	reduced := Reduced(c)
	ui.Column(c.Key("tasks")).FillWidth().MaxWidth(w).Gap(0).Role(ui.RoleList).Children(func() {
		if o.Summary != "" && o.Compact {
			ui.Row(c.Key("summary")).Height(rowH).Gap(10).AlignItems(ui.Center).Padding(0, 4).Children(func() {
				taskGlyph(c, StepDone, time.Time{}, reduced, o.OnDark)
				ui.Text(c, o.Summary).FontSize(12).TextColor(second).FontFeatures("tnum").SingleLine()
			})
		} else if o.Summary != "" {
			row := ui.ButtonBase(c.Key("summary")).Label(o.Summary).Row().Height(rowH).Gap(10).AlignItems(ui.Center).
				Radius(6).Padding(0, 4).Cursor(ui.CursorPointer).FocusRing(false)
			if row.Hovered() || row.FocusVisible() {
				row.Background(design.Fade(text, 0.06))
			}
			row.Children(func() {
				taskGlyph(c, StepDone, time.Time{}, reduced, o.OnDark)
				ui.Text(c, o.Summary).FontSize(13).TextColor(second).FontFeatures("tnum").SingleLine()
			})
			Reveal(row)
			res.Summary = row.Clicked()
		}
		for i, s := range steps {
			if i < o.Fold {
				continue
			}
			if !s.DoneAt.IsZero() && s.State == StepDone && !reduced && time.Since(s.DoneAt) < checkDraw {
				c.After(16 * time.Millisecond)
			}
			col := ui.Column(c.Key(s.Key)).Gap(0).Role(ui.RoleListItem).Label(s.Label)
			Reveal(col)
			col.Children(func() {
				ui.Row(c).Height(rowH).Gap(10).AlignItems(ui.Center).Padding(0, 4).Children(func() {
					taskGlyph(c, s.State, s.DoneAt, reduced, o.OnDark)
					fg, wt := third, design.WeightRegular
					switch s.State {
					case StepActive:
						fg, wt = text, design.WeightSemibold
					case StepDone:
						fg = second
					case StepFailed:
						fg, wt = text, design.WeightSemibold // red is the ✕ alone
					}
					size := float32(13)
					if o.Compact {
						size = 12
					}
					ui.Text(c, s.Label).FontSize(size).FontWeight(wt).TextColor(fg).SingleLine().Grow(1).MinWidth(0)
					if s.Right != "" {
						rc := third
						if s.State == StepActive {
							rc = second
						}
						ui.Text(c.Key("right"), s.Right).FontSize(12).TextColor(rc).FontFeatures("tnum").SingleLine().Shrink(0)
					}
				})
				if o.Compact {
					return
				}
				// The opened part: under the label, for the running or failed step.
				open := (s.State == StepActive && (s.Progress >= 0 || s.Line != "")) || (s.State == StepFailed && (s.Error != "" || s.Log || s.Body != nil))
				if !open {
					return
				}
				more := ui.Column(c.Key("more")).Gap(6).Padding(0, 4, 8, 30).Clip()
				Reveal(more)
				more.Children(func() {
					if s.State == StepFailed && s.Body != nil {
						s.Body()
						return
					}
					if s.State == StepActive && s.Progress >= 0 {
						taskBar(c, s.Progress, third)
					}
					line, lc := s.Line, second
					if s.State == StepFailed {
						line, lc = s.Error, p.Danger
					}
					if line != "" || s.Log {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							if line != "" {
								t := ui.Text(c, line).FontSize(11).TextColor(lc).SingleLine().Grow(1).MinWidth(0)
								if s.State == StepActive {
									t.Font(MonoFont)
								}
							} else {
								ui.Spacer(c)
							}
							if s.Log && o.ViewLog != "" {
								if Button(c.Key("log"), o.ViewLog, ButtonOpts{Variant: Ghost, Size: 22}).FontWeight(design.WeightRegular).Clicked() {
									res.ViewLog = true
								}
							}
						})
					}
				})
			})
		}
	})
	return res
}

// taskBar is the running row's 2 pt hairline: it springs forward, never back.
func taskBar(c *ui.Context, f float64, track ui.Color) {
	p := design.Of(c)
	bar := ui.Box(c.Key("bar")).FillWidth().Height(2).Radius(1).Background(design.Fade(track, 0.35)).Clip()
	v := bar.AnimateTo("v", float32(min(1, max(0, f))), ui.Animation{Spring: ui.SpringOptions{Duration: design.Scaled(420 * time.Millisecond)}})
	bar.Children(func() {
		ui.Box(c).Absolute().Left(0).Top(0).Bottom(0).WidthPercent(v * 100).Radius(1).Background(p.Accent)
	})
}

// taskGlyph is a step's 16 pt status glyph.
func taskGlyph(c *ui.Context, st StepState, doneAt time.Time, reduced, onDark bool) {
	p := design.Of(c)
	third := p.TextTertiary
	if onDark {
		third = ui.RGBA(255, 255, 255, 0.4)
	}
	box := ui.Box(c.Key("glyph")).Size(16, 16).Shrink(0).Center().AccessHidden(true)
	box.Children(func() {
		switch st {
		case StepActive:
			Spinner(c, 13, p.Accent)
		case StepPending:
			ui.Box(c).Size(12, 12).Radius(6).Border(1.5, design.Fade(third, 0.9))
		case StepSkipped:
			ui.Box(c).Size(8, 1.5).Radius(1).Background(third)
		case StepDone, StepFailed:
			bg := p.Success
			if st == StepFailed {
				bg = p.Danger
			}
			drawn := float32(1)
			if st == StepDone && !doneAt.IsZero() && !reduced {
				drawn = min(1, float32(time.Since(doneAt))/float32(checkDraw))
				drawn = 1 - (1-drawn)*(1-drawn) // ease out
			}
			mark := ui.Box(c).Size(16, 16).Radius(8).Background(bg)
			mark.DrawOver(func(pt *ui.Painter, r ui.Rect) {
				white := ui.RGB(255, 255, 255)
				if st == StepFailed {
					pt.StrokePath((&ui.Path{}).MoveTo(r.X+5.5, r.Y+5.5).LineTo(r.X+10.5, r.Y+10.5), 1.6, white)
					pt.StrokePath((&ui.Path{}).MoveTo(r.X+10.5, r.Y+5.5).LineTo(r.X+5.5, r.Y+10.5), 1.6, white)
					return
				}
				if drawn <= 0.01 {
					return
				}
				path := (&ui.Path{}).MoveTo(r.X+4.4, r.Y+8.3).LineTo(r.X+7, r.Y+10.9).LineTo(r.X+11.8, r.Y+5.6)
				pt.StrokePath(path.Trim(0, drawn), 1.7, white)
			})
		}
	})
}
