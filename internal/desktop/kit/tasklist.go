package kit

import (
	"fmt"
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
	Key       string
	Label     string
	State     StepState
	Right     string    // running: the live detail ("45%"); done: the time ("2.1s")
	DoneAt    time.Time // when it finished (its check draws on from here)
	StartedAt time.Time // running: when it started (its clock ticks from here when Right is empty)
	Progress  float64   // running: 0…1 for a hairline under the label, <0 none
	Line      string    // running: the latest output line, in monospace
	Error     string    // failed: what went wrong
	Log       bool      // offer "View log" in the opened row
	Body      func()    // failed: drawn in the opened row instead of Error
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

// checkDraw is how long a finished step's check takes to draw on, and
// popDur how long its disc takes to pop in and settle.
const (
	checkDraw = 260 * time.Millisecond
	popDur    = 460 * time.Millisecond
)

// The timeline's geometry: every row is a node on one vertical rail, its
// glyph centred in a 20 pt slot that sits padX in from the row's edge.
const (
	padX     = float32(10)
	slotW    = float32(20)
	railW    = float32(2)
	railSkip = float32(10) // the rail stops this far either side of a glyph's centre
)

// TaskList draws the steps as a live timeline: a rail runs through the
// glyphs and fills in success green behind the work as each step lands;
// the running step sits on a soft accent card, its glyph a turning arc
// with a breathing halo and its time ticking on the right; a finished
// step's disc pops in and its check draws on; rows arrive one after
// another, and labels brighten and dim as the work moves past them.
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
	rowH := float32(32)
	if o.Compact {
		rowH = 24
	}
	reduced := Reduced(c)
	track, fill := design.Fade(third, 0.35), design.Fade(p.Success, 0.75)
	soft := design.SpringSoft
	if reduced {
		soft = ui.Animation{Duration: time.Millisecond}
	}
	// The visible rows: the summary (if any) is the first node, done.
	var visible []TaskStep
	for i, s := range steps {
		if i >= o.Fold {
			visible = append(visible, s)
		}
	}
	hasSummary := o.Summary != ""
	ui.Column(c.Key("tasks")).FillWidth().MaxWidth(w).Gap(0).Role(ui.RoleList).Children(func() {
		if hasSummary {
			var row ui.Element
			if o.Compact {
				row = ui.Row(c.Key("summary"))
			} else {
				b := ui.ButtonBase(c.Key("summary")).Label(o.Summary)
				row = b.Row().Cursor(ui.CursorPointer).FocusRing(false)
				if b.Hovered() || b.FocusVisible() {
					row.Background(design.Fade(text, 0.06))
				}
				res.Summary = b.Clicked()
			}
			row.FillWidth().Height(rowH).Gap(10).AlignItems(ui.Center).Radius(10).Padding(0, padX)
			next := len(visible) > 0
			row.Draw(func(pt *ui.Painter, r ui.Rect) {
				if next {
					rail(pt, r, rowH, false, true, 0, 1, track, fill)
				}
			})
			Reveal(row)
			row.Children(func() {
				taskGlyph(c, StepDone, time.Time{}, reduced, o.OnDark)
				size := float32(13)
				if o.Compact {
					size = 12
				}
				ui.Text(c, o.Summary).FontSize(size).TextColor(second).FontFeatures("tnum").SingleLine().Grow(1).MinWidth(0)
				if !o.Compact {
					// A chevron says the row folds and unfolds.
					ch := ui.Box(c.Key("chev")).Size(10, 10).Shrink(0).AccessHidden(true)
					turn := ch.AnimateTo("turn", map[bool]float32{true: 1, false: 0}[o.Fold == 0], soft)
					ch.Draw(func(pt *ui.Painter, r ui.Rect) {
						cx, cy := r.X+r.W/2, r.Y+r.H/2
						pt.Transform(ui.Rotation(turn*180), cx, cy, func() {
							pt.StrokePath((&ui.Path{}).MoveTo(cx-3.5, cy-1.5).LineTo(cx, cy+2).LineTo(cx+3.5, cy-1.5), 1.4, third)
						})
					})
				}
			})
		}
		for i, s := range visible {
			if !s.DoneAt.IsZero() && s.State == StepDone && !reduced && time.Since(s.DoneAt) < popDur {
				c.After(16 * time.Millisecond)
			}
			prevDone := hasSummary && i == 0
			if i > 0 {
				prevDone = visible[i-1].State == StepDone || visible[i-1].State == StepSkipped
			}
			hasPrev, hasNext := i > 0 || hasSummary, i < len(visible)-1
			done := s.State == StepDone || s.State == StepSkipped
			active := s.State == StepActive

			col := ui.Column(c.Key(s.Key)).FillWidth().Gap(0).Radius(10).Role(ui.RoleListItem).Label(s.Label)
			// Rows arrive one after another, dropping in as they open.
			enter := design.SpringSoft
			enter.Delay = time.Duration(i) * 45 * time.Millisecond
			if reduced {
				Reveal(col)
			} else {
				col.Transition(ui.ElementTransition{Animation: &enter,
					Enter: &ui.Motion{Collapse: true, Opacity: 0, Y: -6}, Exit: &ui.Motion{Collapse: true, Opacity: 0}, ExitDelay: -1, ExitDuration: design.Scaled(120 * time.Millisecond)})
			}
			// The running row rests on a soft accent card.
			cardTo := design.Fade(p.Accent, 0)
			if active && !o.Compact {
				cardTo = design.Fade(p.Accent, 0.09)
				if o.OnDark {
					cardTo = ui.RGBA(255, 255, 255, 0.08)
				}
			}
			col.Background(col.AnimateColor("card", cardTo, soft))
			// The rail fills behind the work: the segment under a step once
			// it lands, then the one into the next step.
			b0, b1 := float32(0), float32(0)
			if prevDone {
				b0 = 1
			}
			if done {
				b1 = 1
			}
			inA, outA := soft, soft
			inA.Delay = design.Scaled(140 * time.Millisecond)
			top := col.AnimateTo("railTop", b0, inA)
			bot := col.AnimateTo("railBot", b1, outA)
			col.Draw(func(pt *ui.Painter, r ui.Rect) {
				rail(pt, r, rowH, hasPrev, hasNext, top, bot, track, fill)
			})
			col.Children(func() {
				ui.Row(c).Height(rowH).Gap(10).AlignItems(ui.Center).Padding(0, padX).Children(func() {
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
					lbl := ui.Text(c.Key("label"), s.Label).FontSize(size).FontWeight(wt).SingleLine().Grow(1).MinWidth(0)
					lbl.TextColor(lbl.AnimateColor("fg", fg, soft))
					right := s.Right
					if right == "" && active && !s.StartedAt.IsZero() {
						// The running step's own clock.
						right = fmt.Sprintf("%.1fs", time.Since(s.StartedAt).Seconds())
						c.After(100 * time.Millisecond)
					}
					if right != "" {
						rc := third
						if active {
							rc = second
						}
						ui.Text(c.Key("right"), right).FontSize(12).TextColor(rc).FontFeatures("tnum").SingleLine().Shrink(0)
					}
				})
				if o.Compact {
					return
				}
				// The opened part: under the label, for the running or failed step.
				open := (active && (s.Progress >= 0 || s.Line != "")) || (s.State == StepFailed && (s.Error != "" || s.Log || s.Body != nil))
				if !open {
					return
				}
				more := ui.Column(c.Key("more")).Gap(6).Padding(0, padX, 10, padX+slotW+10).Clip()
				Reveal(more)
				more.Children(func() {
					if s.State == StepFailed && s.Body != nil {
						s.Body()
						return
					}
					if active && s.Progress >= 0 {
						taskBar(c, s.Progress, third, reduced)
					}
					line, lc := s.Line, second
					if s.State == StepFailed {
						line, lc = s.Error, p.Danger
					}
					if line != "" || s.Log {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							if line != "" {
								t := ui.Text(c, line).FontSize(11).TextColor(lc).SingleLine().Grow(1).MinWidth(0)
								if active {
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

// rail draws a row's piece of the timeline's rail in r: the segment from
// the row's top into its glyph (top, 0…1 filled) when there is a step
// before it, and from its glyph to the row's bottom (bot) when there is
// one after it. Each fills downward, from track to fill.
func rail(pt *ui.Painter, r ui.Rect, rowH float32, hasPrev, hasNext bool, top, bot float32, track, fill ui.Color) {
	x := r.X + padX + slotW/2
	cy := r.Y + rowH/2
	seg := func(y0, y1, k float32) {
		if y1 <= y0 {
			return
		}
		pt.Line(x, y0, x, y1, railW, track)
		if k > 0.001 {
			pt.Line(x, y0, x, y0+(y1-y0)*min(1, k), railW, fill)
		}
	}
	if hasPrev {
		seg(r.Y, cy-railSkip, top)
	}
	if hasNext {
		seg(cy+railSkip, r.Y+r.H, bot)
	}
}

// taskBar is the running row's 3 pt progress bar: it springs forward,
// never back, with a sheen gliding along what is done.
func taskBar(c *ui.Context, f float64, track ui.Color, reduced bool) {
	p := design.Of(c)
	bar := ui.Box(c.Key("bar")).FillWidth().Height(3).Radius(1.5).Background(design.Fade(track, 0.3)).Clip()
	v := bar.AnimateTo("v", float32(min(1, max(0, f))), ui.Animation{Spring: ui.SpringOptions{Duration: design.Scaled(420 * time.Millisecond)}})
	var sheen float32 = -1
	if !reduced {
		sheen = bar.Loop("sheen", 1400*time.Millisecond, ui.Linear)
	}
	bar.Children(func() {
		done := ui.Box(c).Absolute().Left(0).Top(0).Bottom(0).WidthPercent(v * 100).Radius(1.5).Background(p.Accent)
		if sheen >= 0 {
			done.DrawOver(func(pt *ui.Painter, r ui.Rect) {
				x := r.X - 40 + (r.W+80)*sheen
				pt.Clip(r, 1.5, func() {
					pt.FillGradient(ui.Rect{X: x - 30, Y: r.Y, W: 60, H: r.H}, ui.LinearGradient{Angle: 90,
						Stops: []ui.GradientStop{{At: 0, Color: ui.RGBA(255, 255, 255, 0)}, {At: 0.5, Color: ui.RGBA(255, 255, 255, 0.45)}, {At: 1, Color: ui.RGBA(255, 255, 255, 0)}},
					}, 0)
				})
			})
		}
	})
}

// easeOutBack overshoots a little and settles: the pop of a landing disc.
func easeOutBack(t float32) float32 {
	const c1 = 1.9
	t--
	return 1 + (c1+1)*t*t*t + c1*t*t
}

// taskGlyph is a step's status glyph in its 20 pt slot: a hollow ring
// waiting, a turning arc with a breathing halo running, a green disc that
// pops in (a ring bursting off it) as its check draws on, a red ✕, a dash.
func taskGlyph(c *ui.Context, st StepState, doneAt time.Time, reduced, onDark bool) {
	p := design.Of(c)
	third := p.TextTertiary
	if onDark {
		third = ui.RGBA(255, 255, 255, 0.4)
	}
	box := ui.Box(c.Key("glyph")).Size(slotW, slotW).Shrink(0).AccessHidden(true)
	var spin, halo float32
	if st == StepActive && !reduced {
		spin = box.Loop("spin", 900*time.Millisecond, ui.Linear)
		halo = box.Loop("halo", 1600*time.Millisecond, ui.Linear)
	}
	age := float32(1)
	if st == StepDone && !doneAt.IsZero() && !reduced {
		age = min(1, float32(time.Since(doneAt))/float32(design.Scaled(popDur)))
	}
	box.Draw(func(pt *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		white := ui.RGB(255, 255, 255)
		switch st {
		case StepPending:
			pt.StrokePath((&ui.Path{}).Circle(cx, cy, 5.5), 1.5, design.Fade(third, 0.9))
		case StepSkipped:
			pt.Line(cx-4, cy, cx+4, cy, 1.5, third)
		case StepActive:
			if !reduced {
				// The halo breathes out from the ring and fades.
				k := halo
				pt.FillPath((&ui.Path{}).Circle(cx, cy, 6+4*k), design.Fade(p.Accent, 0.28*(1-k)))
			}
			pt.StrokePath((&ui.Path{}).Circle(cx, cy, 6.5), 2, design.Fade(p.Accent, 0.22))
			pt.StrokePath(arc(cx, cy, 6.5, spin, spin+0.3), 2, p.Accent)
			pt.FillPath((&ui.Path{}).Circle(cx, cy, 2.2), p.Accent)
		case StepFailed:
			pt.FillPath((&ui.Path{}).Circle(cx, cy, 8), p.Danger)
			pt.StrokePath((&ui.Path{}).MoveTo(cx-2.6, cy-2.6).LineTo(cx+2.6, cy+2.6), 1.7, white)
			pt.StrokePath((&ui.Path{}).MoveTo(cx+2.6, cy-2.6).LineTo(cx-2.6, cy+2.6), 1.7, white)
		case StepDone:
			// A ring bursts off the disc as it lands.
			if age < 1 {
				k := age
				pt.StrokePath((&ui.Path{}).Circle(cx, cy, 8+2.5*k), 1.5*(1-k)+0.1, design.Fade(p.Success, 0.6*(1-k)))
			}
			sc := float32(1)
			if age < 1 {
				sc = 0.55 + 0.45*easeOutBack(age)
			}
			pt.FillPath((&ui.Path{}).Circle(cx, cy, 8*sc), p.Success)
			drawn := float32(1)
			if age < 1 {
				// The check draws on once the disc is mostly there.
				d := min(1, max(0, (age*float32(popDur)-float32(popDur-checkDraw)*0.4)/float32(checkDraw)))
				drawn = 1 - (1-d)*(1-d)
			}
			if drawn > 0.01 {
				path := (&ui.Path{}).MoveTo(cx-3.6, cy+0.3).LineTo(cx-1, cy+2.9).LineTo(cx+3.8, cy-2.4)
				pt.StrokePath(path.Trim(0, drawn), 1.7, white)
			}
		}
	})
}
