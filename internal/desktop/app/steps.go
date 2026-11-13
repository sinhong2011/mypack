package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/domain"
)

// A build's progress as a live step list, the way agent apps show their
// work: one row per pipeline step the build actually uses. The model is
// plain data (tested on its own); the view is kit.TaskList.

type stepKind int

const (
	stepProject stepKind = iota // build the user's JS project
	stepEngine                  // get the app engine (a download)
	stepMake                    // assemble and customize
	stepSign
	stepCheck   // verify and publish
	stepInstall // install & open
)

func (k stepKind) label() string {
	return T([]string{"steps.project", "steps.engine", "steps.make", "steps.sign", "steps.check", "steps.install"}[k])
}

// stepPlan is the steps a build shows: Build project only for a JS
// project, Get app engine only when a download is needed, Install & Open
// only when that switch is on.
func stepPlan(project, download, install bool) []stepKind {
	var out []stepKind
	if project {
		out = append(out, stepProject)
	}
	if download {
		out = append(out, stepEngine)
	}
	out = append(out, stepMake, stepSign, stepCheck)
	if install {
		out = append(out, stepInstall)
	}
	return out
}

// stepAt is the step a pipeline stage belongs to, given the plan: the
// checks before a download count toward the first step after the project.
func stepAt(stage domain.Stage, plan []stepKind) stepKind {
	has := func(k stepKind) bool {
		for _, p := range plan {
			if p == k {
				return true
			}
		}
		return false
	}
	switch stage {
	case domain.StagePreparing:
		return stepProject
	case domain.StageQueued, domain.StageValidating, domain.StageResolving:
		if has(stepEngine) {
			return stepEngine
		}
		return stepMake
	case domain.StageDownloading:
		return stepEngine
	case domain.StageAssembling, domain.StageCustomizing:
		return stepMake
	case domain.StageSigning:
		return stepSign
	}
	return stepCheck
}

// stepPhase is where the build as a whole is.
type stepPhase int

const (
	phaseWaiting      stepPhase = iota // awaiting approval
	phaseRunning                       // queued or running
	phaseFailed                        // the current step failed
	phaseCancelled                     // stopped at the current step
	phaseBuilt                         // the pipeline is done
	phaseInstalling                    // built, Install & Open under way
	phaseInstalled                     // built, installed and opened
	phaseNotInstalled                  // built, Install & Open did not happen
)

// stepStates is each planned step's state, given the current step and
// the phase.
func stepStates(plan []stepKind, cur stepKind, ph stepPhase) []kit.StepState {
	out := make([]kit.StepState, len(plan))
	ci := len(plan)
	for i, k := range plan {
		if k == cur {
			ci = i
			break
		}
	}
	built := ph >= phaseBuilt
	for i, k := range plan {
		switch {
		case k == stepInstall:
			switch ph {
			case phaseBuilt, phaseInstalling:
				out[i] = kit.StepActive
			case phaseInstalled:
				out[i] = kit.StepDone
			case phaseNotInstalled:
				out[i] = kit.StepSkipped
			default:
				out[i] = kit.StepPending
			}
		case built || i < ci:
			out[i] = kit.StepDone
		case i > ci:
			out[i] = kit.StepPending
		case ph == phaseFailed:
			out[i] = kit.StepFailed
		case ph == phaseCancelled:
			out[i] = kit.StepSkipped
		case ph == phaseWaiting:
			out[i] = kit.StepPending
		default:
			out[i] = kit.StepActive
		}
	}
	return out
}

// stepTrack remembers a build's plan and when each step started and
// ended, as observed.
type stepTrack struct {
	plan    []stepKind
	started map[stepKind]time.Time
	ended   map[stepKind]time.Time
	open    bool // the folded summary was opened
	// doneAt is when the list was first seen finished; settled marks a
	// build that was already finished when first shown (no finale).
	doneAt  time.Time
	settled bool
}

func newStepTrack(plan []stepKind) *stepTrack {
	return &stepTrack{plan: plan, started: map[stepKind]time.Time{}, ended: map[stepKind]time.Time{}}
}

// observe records the states at time now: a step starts the first time
// it runs and ends the first time it is done (or failed); a step that
// was never seen running ends with no time of its own.
func (t *stepTrack) observe(states []kit.StepState, now time.Time) {
	for i, k := range t.plan {
		switch states[i] {
		case kit.StepActive:
			if t.started[k].IsZero() {
				t.started[k] = now
			}
		case kit.StepDone, kit.StepFailed:
			if t.ended[k].IsZero() {
				t.ended[k] = now
				if t.started[k].IsZero() {
					t.started[k] = now
				}
			}
		}
	}
}

// took is how long a finished step took (0 when not finished).
func (t *stepTrack) took(k stepKind) time.Duration {
	s, e := t.started[k], t.ended[k]
	if s.IsZero() || e.IsZero() {
		return 0
	}
	return e.Sub(s)
}

// secs formats a step's time ("2.1s").
func secs(d time.Duration) string {
	if d >= time.Minute {
		return duration(d)
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// foldAt is how many leading finished steps fold into the summary row:
// all of them once three or more are done (unless opened).
func foldAt(states []kit.StepState, open bool) int {
	n := 0
	for _, s := range states {
		if s != kit.StepDone {
			break
		}
		n++
	}
	if n < 3 || open {
		return 0
	}
	return n
}

// ---- the app side ---------------------------------------------------------------------

// jobNeedsDownload reports whether the build gets an app engine it lacks.
func (a *App) jobNeedsDownload(j domain.BuildJob) bool {
	if j.Stage == domain.StageDownloading {
		return true
	}
	for _, l := range a.logs[j.ID] {
		if l.Stage == domain.StageDownloading {
			return true
		}
	}
	if stepIndex(j.Stage) > 1 || j.Status == domain.JobSucceeded {
		return false // it went past without one
	}
	for _, t := range j.Targets {
		if rt := a.runtimeInfo(t); rt != nil && !rt.Cached {
			return true
		}
	}
	return false
}

// jobInstalls reports whether the build ends with Install & Open.
func (a *App) jobInstalls(j domain.BuildJob) bool {
	if !a.ui.autoInstall(j.ProjectID) {
		return false
	}
	for _, t := range j.Targets {
		if t.OS == a.host.OS {
			return true
		}
	}
	return false
}

// jobSteps is the build's step list now, its track updated.
func (a *App) jobSteps(pr *domain.Project, j *domain.BuildJob) (*stepTrack, []kit.StepState) {
	if a.steps == nil {
		a.steps = map[string]*stepTrack{}
	}
	t := a.steps[j.ID]
	plan := stepPlan(jobBuildsProject(*j), a.jobNeedsDownload(*j), a.jobInstalls(*j))
	fresh := t == nil
	if t == nil {
		t = newStepTrack(plan)
		a.steps[j.ID] = t
	} else if len(plan) > len(t.plan) {
		t.plan = plan // a download turned up after all
	}
	ph := phaseRunning
	switch j.Status {
	case domain.JobAwaitingApproval:
		ph = phaseWaiting
	case domain.JobFailed:
		ph = phaseFailed
	case domain.JobCancelled:
		ph = phaseCancelled
	case domain.JobSucceeded:
		ph = phaseBuilt
		if pr != nil {
			opened, auto := a.autoOpen[j.ID]
			ph = builtPhase(a.successOf(pr, j).installed, auto, !opened.IsZero(), a.jobInstalls(*j))
		}
	}
	states := stepStates(t.plan, stepAt(j.Stage, t.plan), ph)
	now := time.Now()
	t.observe(states, now)
	if stepsDone(states) && t.doneAt.IsZero() {
		t.doneAt, t.settled = now, fresh
	}
	return t, states
}

// builtPhase is where a built build is with its Install & Open: installed
// (this build's app is the one installed), installing (an automatic
// Install & Open started and has not opened yet), or not installed. An
// automatic Install & Open that already opened is over: once that app is
// uninstalled or replaced, the build is back to not installed, not
// installing for ever.
func builtPhase(installed, auto, opened, installs bool) stepPhase {
	switch {
	case auto && !opened:
		return phaseInstalling
	case installed:
		return phaseInstalled
	case !installs:
		return phaseBuilt
	}
	return phaseNotInstalled
}

// stepsDone reports whether the list has given way to the result: the
// build is built and Install & Open (if any) has finished.
func stepsDone(states []kit.StepState) bool {
	for _, s := range states {
		if s == kit.StepActive || s == kit.StepPending {
			return false
		}
	}
	return true
}

// buildSteps draws the live step list for a build, centred under the
// stage, with Cancel (Esc) under it while it runs. compact drops the
// opened rows and Cancel (the flyout, the HUD).
func (a *App) buildSteps(c *ui.Context, pr *domain.Project, j *domain.BuildJob, compact, onDark bool) {
	if !compact && j.Status != domain.JobFailed && j.Status != domain.JobCancelled && j.Status != domain.JobAwaitingApproval {
		a.magicCaption(c, pr, j)
		return
	}
	t, states := a.jobSteps(pr, j)
	// The status block's clock still runs: it is the scene's single
	// source of truth for its progress, and it throttles the detail.
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
	rows := make([]kit.TaskStep, len(t.plan))
	for i, k := range t.plan {
		r := kit.TaskStep{Key: fmt.Sprint(int(k)), Label: k.label(), State: states[i], Progress: -1}
		switch states[i] {
		case kit.StepDone:
			r.Right = secs(t.took(k))
			r.DoneAt = t.ended[k]
		case kit.StepActive:
			r.StartedAt = t.started[k]
			switch k {
			case stepEngine:
				r.Right = st.detail
				if j.Progress >= 0 {
					r.Progress = j.Progress
				}
			case stepProject:
				if ls := a.logs[j.ID]; len(ls) > 0 {
					r.Line = ls[len(ls)-1].Message
				}
				r.Log = true
			}
		case kit.StepFailed:
			// What happened, its fixes and the details, in the row itself
			// (Retry is the result bar's).
			e, pid := j.Error, ""
			if pr != nil {
				pid = pr.ID
			}
			r.Body = func() { a.failureBody(c, e, j.ID, a.fixesFor(e, pid, nil)) }
		}
		rows[i] = r
	}
	// Three or more finished steps fold into one summary row, which stays
	// as the way to fold them again once opened.
	opts := kit.TaskListOpts{Compact: compact, OnDark: onDark, ViewLog: T("build.viewLog"), MaxWidth: 520}
	if n := foldAt(states, false); n > 0 {
		var d time.Duration
		for _, k := range t.plan[:n] {
			d += t.took(k)
		}
		opts.Summary = T("steps.summary", "n", n, "time", secs(d))
		opts.Fold = foldAt(states, t.open)
	}
	box := ui.Column(c.Key("steps")).FillWidth().MaxWidth(520).AlignItems(ui.Center).Gap(0)
	if !compact {
		box.Width(420).MaxWidthPercent(100)
	}
	kit.Reveal(box)
	box.Children(func() {
		res := kit.TaskList(c, rows, opts)
		if res.Summary {
			t.open = !t.open
		}
		if res.ViewLog {
			a.logOpen[j.ID] = !a.logOpen[j.ID]
		}
		if compact {
			return
		}
		if j.Status == domain.JobRunning || j.Status == domain.JobQueued || j.Status == domain.JobAwaitingApproval {
			id := j.ID
			if kit.Button(c.Key("cancel"), T("common.cancel"), kit.ButtonOpts{Variant: kit.Ghost, Size: 26}).FontWeight(design.WeightRegular).Margin(8, 0, 0, 0).Clicked() {
				a.cancelJob(id)
			}
			if c.Shortcut(0, ui.KeyEscape) {
				a.cancelJob(id)
			}
		}
	})
}

// buildFooter is what sits under the stage: the step list while the
// build (and its Install & Open) runs, collapsing into the result bar when
// it is done; a failure keeps the list, its failed row open, over the
// result bar in its failed state.
func (a *App) buildFooter(c *ui.Context, pr *domain.Project, j *domain.BuildJob) {
	_, states := a.jobSteps(pr, j)
	switch {
	case j.Status == domain.JobCancelled:
	case j.Status == domain.JobAwaitingApproval:
		// The runtime download is asked here, as the caption; the
		// project's own build asks in the stage.
		if asksRuntime(j) {
			a.runtimeAsk(c, j)
		}
	case j.Status == domain.JobFailed:
		ui.Column(c.Key("failed")).AlignItems(ui.Center).Gap(14).Children(func() {
			a.buildSteps(c, pr, j, false, false)
			a.resultBar(c, pr, j)
		})
	case j.Status == domain.JobSucceeded && stepsDone(states):
		a.resultBar(c, pr, j)
	default:
		a.buildSteps(c, pr, j, false, false)
	}
}

// failedStep is "Step 3 of 5" for the step a build failed at.
func (a *App) failedStep(pr *domain.Project, j *domain.BuildJob) string {
	t, states := a.jobSteps(pr, j)
	for i, st := range states {
		if st == kit.StepFailed {
			return T("status.detail.step", "n", i+1, "of", len(t.plan))
		}
	}
	return ""
}
