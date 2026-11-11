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
// the active build while Mypack's own window is not in front. It never
// takes the focus (shown inactive, always on top, no shadow, no Dock or
// taskbar entry). Hover opens it to the stage, % and Cancel; a success
// says "<App> is ready" with Open for a few seconds, then it retracts; a
// failure stays until dismissed. A click brings Mypack up on that
// project. macOS only (elsewhere it is off); Settings › Builds turns it
// off ("Show build progress near the notch").

const (
	hudW, hudH         = 380, 240 // the window: room for the open state
	hudPillW, hudPillH = 220, 32
	hudOpenW, hudOpenH = 360, 88
)

type hudState struct {
	win     *mygo.Window
	shown   bool
	job     string    // the build it follows
	endedAt time.Time // when that build finished
	failed  bool
	gone    bool // dismissed (a failure) or retracted (a success)
	ticking bool // a re-check is scheduled
	front   bool // Mypack's window is in front (key and visible)
	mem     hudMem
}

// hudEnabled reports whether the HUD may show (macOS, and the setting).
func (a *App) hudEnabled() bool {
	return runtime.GOOS == "darwin" && (a.ui.ShowHUD == nil || *a.ui.ShowHUD)
}

// ---- visibility: a small state machine, tested on its own ----------------------------

// hudIn is what decides the HUD: the setting, whether Mypack's window is
// in front (key and visible), the build it follows and how that ended.
type hudIn struct {
	enabled bool
	front   bool
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
// ended, and only while Mypack's window is not in front. Coming back to
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
	if in.front && !m.endedAt.IsZero() {
		m.gone = true // the result is seen in the window
	}
	if !m.endedAt.IsZero() && in.now.Sub(m.endedAt) > hudShowFor {
		m.gone = true
	}
	show := in.enabled && !in.front && m.job != "" && !m.gone && (in.running || !m.endedAt.IsZero())
	return m, show
}

// watchFront follows the main window's focus, so the HUD hides the moment
// Mypack comes to the front and shows when it goes behind.
func (a *App) watchFront() {
	if a.win == nil {
		return
	}
	a.hud.front = true
	set := func(front bool) func() {
		return func() { a.win.Update(func() { a.hud.front = front; a.syncHUD() }) }
	}
	a.win.OnFocus(set(true))
	a.win.OnShow(set(true))
	a.win.OnRestore(set(true))
	a.win.OnBlur(set(false))
	a.win.OnHide(set(false))
	a.win.OnMinimize(set(false))
}

// syncHUD applies hudNext to the HUD window; job events, focus changes and
// a ticker while something is on call it (main thread).
func (a *App) syncHUD() {
	h := &a.hud
	in := hudIn{enabled: a.hudEnabled(), front: h.front, now: time.Now()}
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
		Title: "Mypack", X: x, Y: y, Width: hudW, Height: hudH,
		Frameless: true, Transparent: true, AlwaysOnTop: true, SkipTaskbar: true, DisableShadow: true, Hidden: true,
		Content: ui.View(a.hudView),
	})
	h.win.ShowInactive()
	h.shown = true
}

// hudView draws the pill: collapsed (icon, name, a hairline of progress
// along its foot) or, on hover, open (stage, %, Cancel); then the result.
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
	ui.Row(c).Absolute().Fill().Justify(ui.Center).AlignItems(ui.Start).Children(func() {
		pill := ui.ButtonBase(c.Key("pill")).Label(name).Column().Gap(6).Cursor(ui.CursorPointer).FocusRing(false).
			Background(ui.RGB(0, 0, 0)).Clip()
		open := pill.Hovered() || ended
		w, hh := float32(hudPillW), float32(hudPillH)
		if open {
			w, hh = hudOpenW, hudOpenH
			if !ended {
				// Room for the steps, compact.
				_, states := a.jobSteps(pr, j)
				rows := len(states) - foldAt(states, false)
				if foldAt(states, false) > 0 {
					rows++
				}
				hh = 32 + float32(rows)*22 + 36
			}
		}
		anim := design.Spring
		if c.Preferences().ReduceMotion {
			anim = design.Tween(design.MotionBase, design.EaseStd)
		}
		pill.Width(pill.AnimateTo("w", w, anim)).Height(pill.AnimateTo("h", hh, anim)).Radius(min(hh/2, 22)).
			Padding(0, 14, 0, 14).Justify(ui.Center)
		// It emerges from the top edge as it appears.
		kit.Pop(pill)
		white, dim := ui.RGB(255, 255, 255), ui.RGBA(255, 255, 255, 0.6)
		pill.Children(func() {
			// The progress hairline along its foot.
			if !ended && j.Progress >= 0 {
				ui.Box(c.Key("bar")).Absolute().Left(0).Bottom(0).Height(2).WidthPercent(float32(j.Progress) * 100).
					Background(design.LogoMid).PassThrough()
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				kit.AppIconOpts(c, icon, name, 16, kit.IconOpts{})
				label := name
				switch {
				case ended && h.failed:
					label = T("hud.failed", "name", name)
				case ended:
					label = T("hud.ready", "name", name)
				}
				ui.Text(c, label).FontSize(design.FontSmall).FontWeight(design.WeightSemibold).TextColor(white).SingleLine().Grow(1).MinWidth(0)
				if n := len(a.activeJobs()); n > 1 {
					ui.Text(c, "+"+itoa(n-1)).FontSize(design.FontCaption).TextColor(dim)
				}
				if !open && !ended && j.Progress >= 0 {
					ui.Text(c, itoa(int(j.Progress*100))+"%").FontSize(design.FontCaption).TextColor(dim).FontFeatures("tnum")
				}
			})
			if open && !ended {
				a.buildSteps(c, pr, j, true, true)
			}
			if open {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					switch {
					case ended && h.failed:
						ui.Text(c, T("hud.failed.sub")).FontSize(design.FontCaption).TextColor(dim).Grow(1)
						if kit.Button(c.Key("view"), T("hud.view"), kit.ButtonOpts{Variant: kit.Ghost, Size: 24}).Clicked() {
							a.hudFocus(j.ProjectID)
						}
						if kit.IconButton(c.Key("x"), icons.Close, T("common.close")).Clicked() {
							h.mem.gone = true
							a.syncHUD()
						}
					case ended:
						ui.Spacer(c)
						if kit.Button(c.Key("open"), T("action.open"), kit.ButtonOpts{Variant: kit.Primary, Size: 26}).Clicked() {
							if in := a.installFor(j.ProjectID); in != nil {
								a.openInstall(in.ID)
							} else {
								a.hudFocus(j.ProjectID)
							}
						}
					default:
						ui.Spacer(c)
						id := j.ID
						if kit.Button(c.Key("cancel"), T("common.cancel"), kit.ButtonOpts{Variant: kit.Ghost, Size: 24}).TextColor(white).Clicked() {
							a.cancelJob(id)
						}
					}
				})
			}
		})
		if pill.Clicked() {
			a.hudFocus(j.ProjectID)
		}
	})
	if !ended || (ended && !h.failed) {
		c.After(500 * time.Millisecond)
	}
}

// hudFocus brings Mypack to the front on the build's project.
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
