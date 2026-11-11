package app

import (
	"fmt"
	"time"

	"mypack.dev/mypack/internal/domain"
)

// A build's status clock: the stage title (changes queued behind a
// running roll), the detail line (updated in place, at most 8 times a
// second) and the overall progress, which only ever grows across the whole
// run. The step list reads its detail from here, and the scene's address
// bar its progress: one source of truth.

// statusState is what the status block remembers per build.
type statusState struct {
	title      string    // the title shown (rolls when it changes)
	titleAt    time.Time // when it last changed
	pending    string    // the latest title waiting for the roll to finish
	detail     string
	detailAt   time.Time
	progress   float64 // the furthest overall fraction reached
	indefinite bool    // no reliable fraction right now
}

const (
	titleRoll   = 320 * time.Millisecond // a title's roll; the next waits for it
	detailEvery = 125 * time.Millisecond // ≤ 8 detail updates a second
)

// stageTitle is the build's current stage in words.
func stageTitle(j domain.BuildJob) string {
	switch {
	case j.Status == domain.JobAwaitingApproval:
		return T("stage.awaitingApproval")
	case j.Status == domain.JobQueued:
		return T("stage.queued")
	case j.Stage == domain.StagePreparing:
		return T("status.title.prepare")
	}
	switch stepIndex(j.Stage) {
	case 0:
		return T("status.title.validate")
	case 1:
		return T("status.title.download")
	case 2:
		return T("status.title.assemble")
	case 3:
		return T("status.title.sign")
	default:
		return T("status.title.verify")
	}
}

// Weights of the stages in the overall progress.
const (
	wValidate = 0.05
	wPrepare  = 0.30
	wDownload = 0.45
	wFinish   = 0.20 // assemble, sign, verify
)

// overallProgress maps a build's stage and its own fraction onto one
// continuous 0…1 across the run; definite is false when the current stage
// has no reliable fraction.
func overallProgress(j domain.BuildJob, project bool) (f float64, definite bool) {
	total := wValidate + wDownload + wFinish
	prep := 0.0
	if project {
		prep = wPrepare
		total += wPrepare
	}
	frac := j.Progress
	switch {
	case j.Status == domain.JobSucceeded:
		return 1, true
	case j.Stage == domain.StagePreparing:
		return 0, false
	}
	at := 0.0
	switch i := stepIndex(j.Stage); i {
	case 0:
		at = 0
		return (prep + at) / total, false
	case 1:
		if frac < 0 {
			return (prep + wValidate) / total, false
		}
		return (prep + wValidate + wDownload*frac) / total, true
	default:
		// Assemble, sign, verify: a third of the rest each.
		step := float64(min(i, 4)-2) / 3
		return (prep + wValidate + wDownload + wFinish*step) / total, false
	}
}

// advance moves the status to the build's present state: the progress only
// forward, the title queued behind a running roll, the detail at most
// 8 times a second. It reports whether it needs another frame soon.
func (st *statusState) advance(title, detail string, f float64, definite bool, now time.Time) (again bool) {
	if f > st.progress {
		st.progress = f
	}
	st.indefinite = !definite
	switch {
	case st.title == "":
		st.title, st.titleAt = title, now
	case title != st.title:
		st.pending = title
	}
	if st.pending != "" && now.Sub(st.titleAt) >= titleRoll {
		st.title, st.titleAt, st.pending = st.pending, now, ""
	}
	if detail != st.detail && now.Sub(st.detailAt) >= detailEvery {
		st.detail, st.detailAt = detail, now
	}
	return st.pending != "" || detail != st.detail
}

// statusDetail is the line under the title: the bytes and % of a download,
// the latest output line of a project's build, else the step.
func (a *App) statusDetail(j domain.BuildJob) string {
	switch {
	case j.Stage == domain.StagePreparing:
		if ls := a.logs[j.ID]; len(ls) > 0 {
			return ls[len(ls)-1].Message
		}
		return ""
	case j.Stage == domain.StageDownloading && j.Progress >= 0:
		var total int64
		for _, t := range j.Targets {
			if rt := a.runtimeInfo(t); rt != nil {
				total += rt.Size
			}
		}
		pct := fmt.Sprintf("%d%%", int(j.Progress*100))
		if total > 0 {
			return T("status.detail.download", "done", bytesNum(int64(float64(total)*j.Progress)), "total", bytesLabel(total), "pct", pct)
		}
		return pct
	}
	n := len(visibleSteps)
	i := stepIndex(j.Stage) + 1
	if jobBuildsProject(j) {
		n++
		i++
	}
	return T("status.detail.step", "n", min(i, n), "of", n)
}

// bytesNum is a byte count's number in the unit its total uses ("4.4").
func bytesNum(b int64) string {
	l := bytesLabel(b)
	for i, r := range l {
		if r == ' ' {
			return l[:i]
		}
	}
	return l
}

// statusProgress is the overall fraction for the scene's address bar: the
// same single source as the status bar.
func (a *App) statusProgress(j domain.BuildJob) float64 {
	if st := a.status[j.ID]; st != nil {
		return st.progress
	}
	f, _ := overallProgress(j, jobBuildsProject(j))
	return f
}
