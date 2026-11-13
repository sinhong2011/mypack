package app

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
)

// A running build is its app icon coming to life (kit.MagicIcon) in the
// stage, with a caption under it: the step's name morphing from one to
// the next, what it is doing, and a pill per step. A failed build keeps
// the step list instead, its failed row open.

// magicOutro is how long the finished icon stays for its finale before
// the stage gives way to the app's window.
const magicOutro = 1300 * time.Millisecond

// magicActive reports whether the stage shows the magic icon: while the
// build runs, while its Install & Open runs, and for the finale.
func (a *App) magicActive(pr *domain.Project, j *domain.BuildJob) bool {
	switch j.Status {
	case domain.JobQueued, domain.JobRunning:
		return true
	case domain.JobSucceeded:
		t, states := a.jobSteps(pr, j)
		if !stepsDone(states) {
			return true
		}
		return !t.settled && time.Since(t.doneAt) < magicOutro
	}
	return false
}

// magicLevel is how far the icon's color has risen: the finished steps,
// and the running one's part, by its download or as time passes.
func (a *App) magicLevel(t *stepTrack, states []kit.StepState, j *domain.BuildJob) float64 {
	n := len(t.plan)
	if n == 0 {
		return 0
	}
	var lvl float64
	for i, k := range t.plan {
		switch states[i] {
		case kit.StepDone, kit.StepSkipped:
			lvl++
		case kit.StepActive:
			if k == stepEngine && j.Progress >= 0 {
				lvl += 0.9 * min(1, j.Progress)
			} else if s := t.started[k]; !s.IsZero() {
				lvl += 0.85 * (1 - math.Exp(-time.Since(s).Seconds()/2.5))
			}
		}
	}
	return lvl / float64(n)
}

// magicStageIf draws the magic icon in a stage of height h when it is
// due, and reports whether it did. framed gives it the stage's own card.
func (a *App) magicStageIf(c *ui.Context, pr *domain.Project, j *domain.BuildJob, h float32, framed bool) bool {
	if pr == nil || !a.magicActive(pr, j) {
		return false
	}
	t, states := a.jobSteps(pr, j)
	var landed time.Time
	for _, e := range t.ended {
		if e.After(landed) {
			landed = e
		}
	}
	o := kit.MagicOpts{Icon: pr.IconPNG, Name: pr.Config.App.Name, Size: min(h*0.44, 128),
		Level: a.magicLevel(t, states, j), LandedAt: landed, Settled: t.settled,
		Tint: kit.IconTint(pr.IconPNG, pr.Config.App.Name)}
	if stepsDone(states) {
		o.DoneAt = t.doneAt
		if left := magicOutro - time.Since(t.doneAt); left > 0 {
			c.After(left + 16*time.Millisecond) // then the app's window
		}
	} else {
		c.After(100 * time.Millisecond) // the level creeps up with time
	}
	box := ui.Column(c.Key("magic-stage")).FillWidth().Height(h).Center().Clip()
	if framed {
		box.Radius(16).Background(design.Of(c).Group)
	}
	kit.PageSwap(box)
	box.Children(func() { kit.MagicIcon(c, o) })
	return true
}

// magicCaption is the caption under the magic icon: the step's name, what
// it is doing, a pill per step, and Cancel (Esc) while it runs.
func (a *App) magicCaption(c *ui.Context, pr *domain.Project, j *domain.BuildJob) {
	p := design.Of(c)
	t, states := a.jobSteps(pr, j)
	if a.status == nil {
		a.status = map[string]*statusState{}
	}
	st := a.status[j.ID]
	if st == nil {
		st = &statusState{}
		a.status[j.ID] = st
	}
	f, definite := overallProgress(*j, jobBuildsProject(*j))
	if st.advance(stageTitle(*j), a.statusDetail(*j), f, definite, time.Now()) {
		c.After(detailEvery)
	}
	cur, idx := stepCheck, len(t.plan)-1
	for i, s := range states {
		if s == kit.StepActive || s == kit.StepPending {
			cur, idx = t.plan[i], i
			break
		}
	}
	sub := T("status.detail.step", "n", idx+1, "of", len(t.plan))
	switch {
	case cur == stepEngine && st.detail != "":
		sub = st.detail
	case !t.started[cur].IsZero():
		sub += " · " + fmt.Sprintf("%.1fs", time.Since(t.started[cur]).Seconds())
		c.After(100 * time.Millisecond)
	}
	ui.Column(c.Key("magic-caption")).AlignItems(ui.Center).Gap(6).Margin(4, 0, 0, 0).Width(420).MaxWidthPercent(100).
		Role(ui.RoleStatus).Label(cur.label()).Children(func() {
		ui.Text(c.Key("step"), cur.label()).FontSize(17).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine().
			ContentTransition(ui.ContentTransition{})
		ui.Text(c.Key("sub"), sub).FontSize(12).TextColor(p.TextSecondary).FontFeatures("tnum").SingleLine()
		if cur == stepProject {
			if ls := a.logs[j.ID]; len(ls) > 0 {
				ui.Text(c.Key("line"), ls[len(ls)-1].Message).Font(kit.MonoFont).FontSize(11).TextColor(p.TextTertiary).SingleLine().MaxWidthPercent(100)
			}
		}
		a.stepPills(c, states)
		if j.Status == domain.JobRunning || j.Status == domain.JobQueued {
			id := j.ID
			if kit.Button(c.Key("cancel"), T("common.cancel"), kit.ButtonOpts{Variant: kit.Ghost, Size: 26}).FontWeight(design.WeightRegular).Margin(4, 0, 0, 0).Clicked() {
				a.cancelJob(id)
			}
			if c.Shortcut(0, ui.KeyEscape) {
				a.cancelJob(id)
			}
		}
	})
}

// stepPills is a pill per step: green once done, a longer accent pill for
// the running one, faint for those to come. They spring between states.
func (a *App) stepPills(c *ui.Context, states []kit.StepState) {
	p := design.Of(c)
	soft := design.SpringPop
	if kit.Reduced(c) {
		soft = ui.Animation{Duration: time.Millisecond}
	}
	ui.Row(c.Key("pills")).Gap(6).AlignItems(ui.Center).Margin(6, 0, 0, 0).AccessHidden(true).Children(func() {
		for i, s := range states {
			pill := ui.Box(c.Key(i)).Height(6).Radius(3)
			w, col := float32(6), design.Fade(p.TextTertiary, 0.45)
			switch s {
			case kit.StepDone, kit.StepSkipped:
				col = p.Success
			case kit.StepActive:
				w, col = 22, p.Accent
			}
			pill.Width(pill.AnimateTo("w", w, soft)).Background(pill.AnimateColor("c", col, soft))
		}
	})
}

// asksRuntime reports whether a waiting build waits for the runtime
// download (not for the project's own build).
func asksRuntime(j *domain.BuildJob) bool {
	return j.Status == domain.JobAwaitingApproval && (j.Approval == nil || j.Approval.Kind != "project_build")
}

// magicWaiting is the stage while a build waits for the runtime download:
// the app's icon, frosted and still empty, the first frame of the build it
// is about to become. It shares the running stage's key, so approving
// fills this same icon instead of swapping it for another.
func (a *App) magicWaiting(c *ui.Context, pr *domain.Project, h float32) {
	box := ui.Column(c.Key("magic-stage")).FillWidth().Height(h).Center().Clip()
	kit.PageSwap(box)
	box.Children(func() {
		kit.MagicIcon(c, kit.MagicOpts{Icon: pr.IconPNG, Name: pr.Config.App.Name, Size: min(h*0.44, 128),
			Tint: kit.IconTint(pr.IconPNG, pr.Config.App.Name)})
	})
}

// runtimeAsk is the caption while a build waits for the runtime download,
// laid out as the running caption (magicCaption) it turns into: the
// question where the step's name goes, the size where its detail goes,
// then Cancel and Download & Continue. Return downloads, Esc cancels.
func (a *App) runtimeAsk(c *ui.Context, j *domain.BuildJob) {
	p := design.Of(c)
	var size int64
	for _, t := range j.Targets {
		if rt := a.runtimeInfo(t); rt != nil && !rt.Cached {
			size += rt.Size
		}
	}
	id := j.ID
	busy := a.dlg.approving == id
	title := T("runtime.ask.title")
	ui.Column(c.Key("magic-caption")).AlignItems(ui.Center).Gap(6).Margin(4, 0, 0, 0).Width(420).MaxWidthPercent(100).
		Role(ui.RoleStatus).Label(title).Children(func() {
		ui.Text(c.Key("step"), title).FontSize(17).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine().
			ContentTransition(ui.ContentTransition{})
		ui.Text(c.Key("sub"), T("runtime.ask.detail", "size", bytesLabel(size))).FontSize(12).TextColor(p.TextSecondary).
			FontFeatures("tnum").SingleLine().Tooltip(T("runtime.first.cache") + " " + a.runtimeCacheDir())
		// Side by side, as macOS orders them, so the caption keeps the
		// running caption's height.
		ui.Row(c.Key("ask")).Gap(6).AlignItems(ui.Center).Margin(8, 0, 0, 0).Children(func() {
			if kit.Button(c.Key("cancel"), T("common.cancel"), kit.ButtonOpts{Variant: kit.Ghost, Size: 32}).FontWeight(design.WeightRegular).
				Clicked() || c.Shortcut(0, ui.KeyEscape) {
				a.cancelJob(id)
			}
			if kit.Button(c.Key("approve"), T("runtime.first.approve"), kit.ButtonOpts{Variant: kit.Primary, Size: 32,
				Loading: busy, Icon: icons.Download}).Clicked() || (!busy && !kit.Typing() && c.Shortcut(0, ui.KeyEnter)) {
				a.approveJob(id)
			}
		})
	})
}
