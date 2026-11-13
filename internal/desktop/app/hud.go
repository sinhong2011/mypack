package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
)

// The build HUD: a small black pill at the top centre of the screen, just
// under the menu bar (where the notch is on notched MacBooks), that shows
// the active build while MyPack's own window is hidden or minimised (a
// window merely behind another app's does not count). It never
// takes the focus (shown inactive, always on top, no shadow, no Dock or
// taskbar entry). Hover opens it to the stage, % and Cancel; a success
// says "<App> is ready" with Open for a few seconds, then it retracts; a
// failure stays until dismissed. A click brings MyPack up on that
// project. macOS only (elsewhere it is off); Settings › Builds turns it
// off ("Show build progress near the notch").

const (
	hudW, hudH         = 380, 240 // the window: room for the open state
	hudPillW, hudPillH = 220, 32
	hudOpenW           = 360     // hovered while building (height follows the steps)
	hudDoneW, hudDoneH = 344, 56 // a result: one row
)

type hudState struct {
	win     *mygo.Window
	shown   bool
	job     string    // the build it follows
	endedAt time.Time // when that build finished
	failed  bool
	gone    bool // dismissed (a failure) or retracted (a success)
	ticking bool // a re-check is scheduled
	up      bool // MyPack's window is on screen (not hidden or minimised)
	mem     hudMem
}

// hudEnabled reports whether the HUD may show (macOS, and the setting).
func (a *App) hudEnabled() bool {
	return runtime.GOOS == "darwin" && (a.ui.ShowHUD == nil || *a.ui.ShowHUD)
}

// ---- visibility: a small state machine, tested on its own ----------------------------

// hudIn is what decides the HUD: the setting, whether MyPack's window is
// up (on screen, not hidden or minimised), the build it follows and how
// that ended.
type hudIn struct {
	enabled bool
	up      bool
	job     string // the most recent build ("" none)
	running bool   // it is running (or waiting)
	ended   string // "succeeded", "failed", "cancelled" once it ends
	now     time.Time
}

// hudMem is what the HUD remembers between decisions.
type hudMem struct {
	job     string
	endedAt time.Time // when the followed build ended, seen while away
	failed  bool
	gone    bool // the payload was seen (by returning or clicking) or expired
}

// hudShowFor is how long a result waits in the HUD for the user.
const hudShowFor = 6 * time.Second

// hudNext decides whether the HUD shows: only while a build runs or just
// ended, and only while MyPack's window is hidden or minimised. Bringing back
// the window hides it at once and counts as having seen a result; a
// result waits ~6 s otherwise. A new build starts afresh.
func hudNext(m hudMem, in hudIn) (hudMem, bool) {
	if in.job != m.job {
		m = hudMem{job: in.job}
	}
	if in.ended != "" && m.endedAt.IsZero() {
		m.endedAt, m.failed = in.now, in.ended == "failed"
		if in.ended == "cancelled" {
			m.gone = true
		}
	}
	if in.up && !m.endedAt.IsZero() {
		m.gone = true // the result is seen in the window
	}
	if !m.endedAt.IsZero() && in.now.Sub(m.endedAt) > hudShowFor {
		m.gone = true
	}
	show := in.enabled && !in.up && m.job != "" && !m.gone && (in.running || !m.endedAt.IsZero())
	return m, show
}

// watchWindow follows whether the main window is on screen, so the HUD
// hides the moment it comes back and shows once it is hidden or
// minimised. Focus does not count: a window behind another app's is
// still up.
func (a *App) watchWindow() {
	if a.win == nil {
		return
	}
	a.hud.up = true
	set := func(up bool) func() {
		return func() { a.win.Update(func() { a.hud.up = up; a.syncHUD() }) }
	}
	a.win.OnShow(set(true))
	a.win.OnRestore(set(true))
	a.win.OnHide(set(false))
	a.win.OnMinimize(set(false))
}

// syncHUD applies hudNext to the HUD window; job events, the window showing or hiding and
// a ticker while something is on call it (main thread).
func (a *App) syncHUD() {
	h := &a.hud
	in := hudIn{enabled: a.hudEnabled(), up: h.up, now: time.Now()}
	active := a.activeJobs()
	if len(active) > 0 {
		in.job, in.running = active[0].ID, true
	} else if j := a.jobByID(h.mem.job); j != nil {
		in.job = j.ID
		switch j.Status {
		case domain.JobSucceeded:
			in.ended = "succeeded"
		case domain.JobFailed:
			in.ended = "failed"
		case domain.JobCancelled:
			in.ended = "cancelled"
		}
	}
	var want bool
	h.mem, want = hudNext(h.mem, in)
	h.job, h.endedAt, h.failed = h.mem.job, h.mem.endedAt, h.mem.failed
	switch {
	case want && h.win == nil:
		a.openHUD()
	case want && !h.shown:
		h.win.ShowInactive()
		h.shown = true
	case !want && h.shown && h.win != nil:
		h.win.Hide()
		h.shown = false
	}
	if h.win != nil {
		h.win.Invalidate()
	}
	// Re-check while something is on (a result expires).
	if (len(active) > 0 || h.shown) && !h.ticking {
		h.ticking = true
		time.AfterFunc(500*time.Millisecond, func() {
			if a.win != nil {
				a.win.Update(func() { h.ticking = false; a.syncHUD() })
			}
		})
	}
}

// openHUD makes the HUD window at the top centre of the main display,
// just under the menu bar.
func (a *App) openHUD() {
	h := &a.hud
	d := mygo.Screen.PrimaryDisplay()
	x := d.Bounds.X + (d.Bounds.Width-hudW)/2
	y := d.WorkArea.Y // under the menu bar (and the notch)
	h.win = mygo.NewWindow(mygo.WindowOptions{
		Title: "MyPack", X: x, Y: y, Width: hudW, Height: hudH,
		Frameless: true, Transparent: true, AlwaysOnTop: true, SkipTaskbar: true, DisableShadow: true, Hidden: true,
		Content: ui.View(a.hudView),
	})
	h.win.ShowInactive()
	h.shown = true
}

// The HUD's own colours: it floats over whatever is on screen, so it is
// always the dark raised surface of the result bar, whatever the theme.
var (
	hudFill   = design.Dark.Raised
	hudStroke = ui.RGBA(255, 255, 255, 0.09)
	hudText   = ui.RGB(255, 255, 255)
	hudDim    = ui.RGBA(255, 255, 255, 0.55)
)

// hudView draws the pill: collapsed (icon, name, a hairline of progress
// along its foot) or, on hover, open (stage, %, Cancel); then the result.
// The pill springs between its sizes while its content cross-fades, so a
// finished build reads as the same object settling, not a new one.
func (a *App) hudView(c *ui.Context) {
	h := &a.hud
	c.Root().Background(ui.Transparent)
	j := a.jobByID(h.job)
	if j == nil {
		return
	}
	pr := a.project(j.ProjectID)
	name := j.Snapshot.App.Name
	var icon []byte
	if pr != nil {
		icon, name = pr.IconPNG, pr.Config.App.Name
	}
	ended := !h.endedAt.IsZero()
	reduce := c.Preferences().ReduceMotion
	ui.Row(c).Absolute().Fill().Justify(ui.Center).AlignItems(ui.Start).Padding(6, 0, 0, 0).Children(func() {
		pill := ui.ButtonBase(c.Key("pill")).Label(name).Column().Gap(6).Cursor(ui.CursorPointer).FocusRing(false).
			Background(hudFill).Border(1, hudStroke).Clip()
		pill.Shadow(0, 10, 28, -4, ui.RGBA(0, 0, 0, 0.45))
		hovered := pill.Hovered()
		open := hovered || ended
		w, hh := float32(hudPillW), float32(hudPillH)
		switch {
		case ended:
			w, hh = hudDoneW, hudDoneH
		case open:
			// Room for the steps, compact.
			_, states := a.jobSteps(pr, j)
			rows := len(states) - foldAt(states, false)
			if foldAt(states, false) > 0 {
				rows++
			}
			w, hh = hudOpenW, 32+float32(rows)*22+36
		}
		anim := design.SpringHero
		if reduce {
			anim = design.Tween(design.MotionBase, design.EaseStd)
		}
		pad := float32(14)
		if ended {
			pad = 12 // the icon sits as far from the edge as from the top
		}
		pill.Width(pill.AnimateTo("w", w, anim)).Height(pill.AnimateTo("h", hh, anim)).Radius(min(hh/2, 28)).
			Padding(0, 10, 0, pad).Justify(ui.Center)
		// It drops in from under the menu bar as it appears.
		pill.Transition(ui.ElementTransition{Animation: &design.SpringPop,
			Enter: &ui.Motion{Y: -14, Scale: 0.9, Opacity: 0}, Exit: &ui.Motion{Y: -8, Scale: 0.96, Opacity: 0}})
		// A result waits while the pointer is on it; its countdown starts
		// again once the pointer leaves.
		if ended && !h.failed && hovered {
			h.mem.endedAt = time.Now()
		}
		pill.Children(func() {
			if ended {
				a.hudResult(c, j.ProjectID, icon, name, reduce)
				if !h.failed {
					left := 1 - float32(time.Since(h.mem.endedAt))/float32(hudShowFor)
					ui.Box(c.Key("countdown")).Absolute().Left(0).Bottom(0).Height(2).WidthPercent(max(0, left) * 100).
						Background(ui.RGBA(255, 255, 255, 0.18)).PassThrough()
				}
				return
			}
			// The progress hairline along its foot.
			if j.Progress >= 0 {
				bar := ui.Box(c.Key("bar")).Absolute().Left(0).Bottom(0).Height(2).Background(design.LogoMid).PassThrough()
				bar.WidthPercent(bar.AnimateTo("p", float32(j.Progress)*100, design.SpringSoft))
			}
			ui.Row(c.Key("building")).Gap(8).AlignItems(ui.Center).Transition(hudSwap(0, reduce)).Children(func() {
				kit.AppIconOpts(c, icon, name, 16, kit.IconOpts{})
				ui.Text(c, name).FontSize(design.FontSmall).FontWeight(design.WeightSemibold).TextColor(hudText).SingleLine().Grow(1).MinWidth(0)
				if n := len(a.activeJobs()); n > 1 {
					ui.Text(c, "+"+itoa(n-1)).FontSize(design.FontCaption).TextColor(hudDim)
				}
				if !open && j.Progress >= 0 {
					ui.Text(c, itoa(int(j.Progress*100))+"%").FontSize(design.FontCaption).TextColor(hudDim).FontFeatures("tnum")
				}
			})
			if open {
				ui.Column(c.Key("steps")).Gap(6).Transition(hudSwap(60*time.Millisecond, reduce)).Children(func() {
					a.buildSteps(c, pr, j, true, true)
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Spacer(c)
						id := j.ID
						if hudChip(c, "cancel", T("common.cancel"), false) {
							a.cancelJob(id)
						}
					})
				})
			}
		})
		if pill.Clicked() {
			a.hudFocus(j.ProjectID)
		}
	})
	switch {
	case ended && !h.failed:
		c.After(33 * time.Millisecond) // the countdown runs smoothly
	case !ended:
		c.After(500 * time.Millisecond)
	}
}

// hudSwap is how the pill's content comes in and goes: a short rise out
// of a blur, after delay, so it lands as the pill finishes resizing.
func hudSwap(delay time.Duration, reduce bool) ui.ElementTransition {
	if reduce {
		return ui.ElementTransition{Duration: -1}
	}
	in := design.SpringSoft
	in.Delay = delay
	return ui.ElementTransition{Animation: &in,
		Enter: &ui.Motion{Opacity: 0, Y: 6, Blur: 4}, Exit: &ui.Motion{Opacity: 0, Blur: 4},
		ExitDuration: 120 * time.Millisecond}
}

// hudResult draws a finished build as one row, in the result bar's
// language: the app's icon with a check (or a cross) springing onto its
// corner, the outcome over a quiet second line, a hairline, then the
// action as a soft chip and a close button.
func (a *App) hudResult(c *ui.Context, projectID string, icon []byte, name string, reduce bool) {
	h := &a.hud
	title, sub := T("hud.ready", "name", name), T("hud.ready.sub")
	badge, glyph := design.Dark.Success, icons.Check
	if h.failed {
		title, sub = T("hud.failed", "name", name), T("hud.failed.sub")
		badge, glyph = design.Dark.Danger, icons.Close
	}
	ui.Row(c.Key("result")).FillWidth().Gap(12).AlignItems(ui.Center).Transition(hudSwap(90*time.Millisecond, reduce)).Children(func() {
		const is, bs = 32, 16
		ui.Box(c).Size(is, is).Shrink(0).Children(func() {
			kit.AppIconOpts(c, icon, name, is, kit.IconOpts{})
			b := ui.Row(c.Key("badge")).Absolute().Right(-5).Bottom(-5).Size(bs, bs).Radius(bs/2).Center().
				Background(badge).Border(2, hudFill).AccessHidden(true)
			if !reduce {
				pop := design.SpringPop
				pop.Delay = 260 * time.Millisecond
				b.Transition(ui.ElementTransition{Animation: &pop, Enter: &ui.Motion{Scale: 0.2, Opacity: 0}})
			}
			b.Children(func() { kit.Glyph(c, glyph, 9, hudText) })
		})
		ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(design.FontSmall).FontWeight(design.WeightSemibold).TextColor(hudText).SingleLine()
			ui.Text(c, sub).FontSize(design.FontCaption).TextColor(hudDim).SingleLine()
		})
		ui.Box(c).Width(1).Height(22).Background(ui.RGBA(255, 255, 255, 0.1)).Shrink(0)
		ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
			if h.failed {
				if hudChip(c, "view", T("hud.view"), true) {
					a.hudFocus(projectID)
				}
			} else if hudChip(c, "open", T("action.open"), true) {
				if in := a.installFor(projectID); in != nil {
					a.openInstall(in.ID)
				} else {
					a.hudFocus(projectID)
				}
			}
			x := ui.ButtonBase(c.Key("x")).Label(T("common.close")).Size(30, 30).Radius(15).Center().
				Cursor(ui.CursorPointer).FocusRing(false).Transition(design.Transition(c))
			col := hudDim
			if x.Hovered() {
				x.Background(ui.RGBA(255, 255, 255, 0.08))
				col = hudText
			}
			x.Children(func() { kit.Glyph(c, icons.Close, 13, col) })
			if x.Clicked() {
				h.mem.gone = true
				a.syncHUD()
			}
		})
	})
}

// hudChip is the HUD's button: a soft white-on-dark pill, as the result
// bar's "Open again" chip, filled when it is the main action.
func hudChip(c *ui.Context, key, label string, main bool) bool {
	b := ui.ButtonBase(c.Key(key)).Label(label).Row().Height(32).PaddingX(14).Radius(16).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).FocusRing(false).Transition(design.Transition(c))
	fill := float32(0)
	if main {
		fill = 0.1
	}
	switch {
	case b.Pressed():
		fill += 0.1
	case b.Hovered():
		fill += 0.06
	}
	if fill > 0 {
		b.Background(ui.RGBA(255, 255, 255, fill))
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(design.FontSmall).FontWeight(design.WeightMedium).TextColor(hudText).SingleLine()
	})
	return b.Clicked()
}

// hudFocus brings MyPack to the front on the build's project.
func (a *App) hudFocus(projectID string) {
	a.showWindow()
	if a.section != secCreate || a.create.building != projectID {
		a.openProject(projectID, 0)
	}
	a.syncHUD()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
