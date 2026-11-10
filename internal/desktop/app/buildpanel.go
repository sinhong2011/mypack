package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/components"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// The five visible stages (spec §9C.4: Validate → Download → Assemble →
// Sign → Verify) and the engine stages each covers.
var visibleSteps = []struct {
	id     string
	stages []domain.Stage
}{
	{"step.validate", []domain.Stage{domain.StageQueued, domain.StageValidating, domain.StageResolving}},
	{"step.download", []domain.Stage{domain.StageDownloading}},
	{"step.assemble", []domain.Stage{domain.StageAssembling, domain.StageCustomizing}},
	{"step.sign", []domain.Stage{domain.StageSigning}},
	{"step.verify", []domain.Stage{domain.StagePackaging, domain.StageVerifying, domain.StagePublishing}},
}

func stepIndex(s domain.Stage) int {
	if s == domain.StageCompleted {
		return len(visibleSteps)
	}
	for i, v := range visibleSteps {
		for _, st := range v.stages {
			if st == s {
				return i
			}
		}
	}
	return 0
}

func visibleStepLabel(s domain.Stage) string {
	i := stepIndex(s)
	if i >= len(visibleSteps) {
		return "status.built"
	}
	return visibleSteps[i].id + ".active"
}

// timelineSteps converts a job into honest timeline steps.
func (a *App) timelineSteps(j domain.BuildJob) []components.Step {
	cur := stepIndex(j.Stage)
	downloaded := false
	for _, l := range a.logs[j.ID] {
		if l.Stage == domain.StageDownloading {
			downloaded = true
		}
	}
	if j.Stage == domain.StageDownloading {
		downloaded = true
	}
	out := make([]components.Step, len(visibleSteps))
	for i, v := range visibleSteps {
		st := components.Step{Label: T(v.id), Progress: -1}
		switch {
		case j.Status == domain.JobAwaitingApproval:
			st.State = components.StepPending
			if i == 0 {
				st.State = components.StepDone
			}
		case i < cur:
			st.State = components.StepDone
		case i == cur && j.Status == domain.JobFailed:
			st.State = components.StepFailed
		case i == cur && (j.Status == domain.JobRunning || j.Status == domain.JobQueued):
			st.State = components.StepActive
			if j.Progress >= 0 {
				st.Progress = j.Progress
			}
		case j.Status == domain.JobSucceeded:
			st.State = components.StepDone
		}
		if i == 1 {
			if (st.State == components.StepDone || j.Status == domain.JobSucceeded) && !downloaded && cur > 1 {
				st.State = components.StepSkipped
				st.Detail = T("step.download.cached")
			}
			if st.State == components.StepActive && j.Progress >= 0 {
				var total int64
				for _, t := range j.Targets {
					if rt := a.runtimeInfo(t); rt != nil {
						total += rt.Size
					}
				}
				if total > 0 {
					st.Detail = T("step.download.detail", "done", bytesLabel(int64(float64(total)*j.Progress)), "total", bytesLabel(total))
				}
			}
		}
		out[i] = st
	}
	return out
}

// buildPanel renders the inline build state for a project's watched job.
// It returns false when there's nothing to show.
func (a *App) buildPanel(c *ui.Context, pr *domain.Project, j *domain.BuildJob) {
	p := design.Of(c)
	if _, ok := a.logs[j.ID]; !ok {
		a.logs[j.ID] = nil
		id := j.ID
		go func() {
			lines, err := a.svc.JobLog(context.Background(), id)
			if err == nil && a.win != nil {
				a.win.Update(func() { a.logs[id] = lines })
			}
		}()
	}
	switch j.Status {
	case domain.JobAwaitingApproval:
		a.approvalCard(c, j)
	case domain.JobSucceeded:
		a.successPanel(c, pr, j)
	case domain.JobFailed:
		a.failurePanel(c, pr, j)
	default:
		components.Card(c).Padding(20).Gap(16).Children(func() {
			ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
				components.AppIcon(c, pr.IconPNG, pr.Config.App.Name, 40)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					title := T("build.progress.title", "device", targetsWord(a.host, j.Targets))
					if j.Status == domain.JobCancelled {
						title = T("build.cancelled.title")
					}
					components.Heading(c, title)
					sub := T("build.progress.sub")
					if j.Status == domain.JobCancelled {
						sub = T("build.cancelled.sub")
					}
					components.Caption(c, sub)
				})
				switch j.Status {
				case domain.JobRunning, domain.JobQueued:
					if components.Button(c.Key("cancel"), T("common.cancel"), components.ButtonOpts{Icon: lobeui.IconStop()}).Clicked() {
						a.cancelJob(j.ID)
					}
				case domain.JobCancelled:
					if components.Button(c.Key("rebuild"), T("action.buildAgain"), components.ButtonOpts{Variant: components.Primary, Icon: lobeui.IconRetry()}).Clicked() {
						a.startBuild(pr.ID, j.Targets)
					}
				}
			})
			components.Hairline(c)
			components.BuildTimeline(c, a.timelineSteps(*j))
			a.logDisclosure(c, j.ID)
		}).Role(ui.RoleRegion).Label(T("build.progress.region")).Background(p.Surface)
	}
}

func targetsWord(host config.Target, ts []config.Target) string {
	if len(ts) == 1 && ts[0].OS == host.OS && ts[0].Arch == host.Arch {
		return T("target.thisDeviceShort", "device", deviceWord(host))
	}
	if len(ts) == 1 {
		return osLabel(ts[0].OS)
	}
	return T("target.platforms", "count", len(ts))
}

func (a *App) logDisclosure(c *ui.Context, jobID string) {
	open := a.logOpen[jobID]
	components.Disclosure(c.Key("log"), &open, T("build.viewLog"), "")
	a.logOpen[jobID] = open
	if open {
		var lines []components.LogLine
		for _, l := range a.logs[jobID] {
			lines = append(lines, components.LogLine{Time: l.Time.Format("15:04:05"), Level: l.Level, Text: l.Message})
		}
		components.LogViewer(c, lines, 240)
	}
}

func (a *App) approvalCard(c *ui.Context, j *domain.BuildJob) {
	var size int64
	var src, cache string
	for _, t := range j.Targets {
		if rt := a.runtimeInfo(t); rt != nil && !rt.Cached {
			size += rt.Size
			src = rt.Source
		}
	}
	cache = a.runtimeCacheDir()
	if src == "" {
		src = "runtimes.mypack.dev"
	}
	approve, cancel := components.RuntimeDownloadCard(c, components.RuntimeDownloadInfo{
		Title: T("runtime.first.title"), Body: T("runtime.first.body"),
		SizeLabel: T("runtime.first.size"), SourceLabel: T("runtime.first.source"), CacheLbl: T("runtime.first.cache"),
		Size: bytesLabel(size), Source: src + " · " + T("runtime.first.signed"), Cache: cache,
		Approve: T("runtime.first.approve"), Cancel: T("common.cancel"), Busy: a.dlg.approving == j.ID,
	})
	if approve {
		id := j.ID
		a.dlg.approving = id
		a.do(func(ctx context.Context) error { return a.svc.ApproveJob(ctx, id, "gui") }, func(err error) {
			a.dlg.approving = ""
			a.toastErr(err)
		})
	}
	if cancel {
		a.cancelJob(j.ID)
	}
}

// runtimeCacheDir derives the cache root from any cached runtime's path
// (<cache>/runtimes/<version>/<os-arch>); the service doesn't expose it yet.
func (a *App) runtimeCacheDir() string {
	for _, rt := range a.runtimes {
		if rt.Path != "" {
			d := filepath.Dir(filepath.Dir(rt.Path))
			if h, err := os.UserHomeDir(); err == nil && strings.HasPrefix(d, h) {
				d = "~" + d[len(h):]
			}
			return d
		}
	}
	return "~/Library/Caches/mypack/runtimes"
}

func (a *App) successPanel(c *ui.Context, pr *domain.Project, j *domain.BuildJob) {
	p := design.Of(c)
	var art *domain.Artifact
	for i := range j.Artifacts {
		if j.Artifacts[i].Target.OS == a.host.OS {
			art = &j.Artifacts[i]
			break
		}
	}
	if art == nil && len(j.Artifacts) > 0 {
		art = &j.Artifacts[0]
	}
	components.Card(c).Padding(28, 24, 22, 24).Gap(18).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Children(func() {
			components.AppIcon(c, pr.IconPNG, pr.Config.App.Name, 84)
			ui.Row(c).Absolute().Right(-6).Bottom(-6).Size(28, 28).Radius(14).Background(p.Success).Border(3, p.Surface).Center().Children(func() {
				ui.Icon(c, lobeui.IconCheck()).FontSize(14).TextColor(ui.RGB(255, 255, 255))
			})
		})
		ui.Column(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, T("build.success.title")).FontSize(design.FontTitle1).FontWeight(design.WeightSemibold).TextColor(p.Text).LetterSpacing(-0.3).Role(ui.RoleHeading)
			parts := []string{pr.Config.App.Name, targetsWord(a.host, j.Targets)}
			if j.StartedAt != nil && j.FinishedAt != nil {
				parts = append(parts, duration(j.FinishedAt.Sub(*j.StartedAt)))
			}
			if art != nil {
				parts = append(parts, bytesLabel(art.Size))
			}
			components.Secondary(c, strings.Join(parts, " · "))
		})
		inst := a.installFor(pr.ID)
		ui.Row(c).Gap(8).Wrap().Justify(ui.Center).Children(func() {
			if art != nil && art.Target.OS == a.host.OS {
				label := T("action.installOpen")
				if inst != nil && inst.ArtifactID == art.ID {
					label = T("action.open")
				}
				if components.Button(c.Key("install"), label, components.ButtonOpts{Variant: components.Primary, Size: design.ControlLG,
					Icon: lobeui.IconRocket(), Loading: a.dlg.installing == art.ID}).Clicked() || c.Shortcut(0, ui.KeyEnter) {
					if inst != nil && inst.ArtifactID == art.ID {
						a.openInstall(inst.ID)
					} else {
						a.installArtifact(art.ID, false)
					}
				}
			}
			if art != nil {
				if components.Button(c.Key("reveal"), T("action.showInFinder"), components.ButtonOpts{Size: design.ControlLG, Icon: lobeui.IconFolderOpen()}).Clicked() {
					a.reveal(art.Path)
				}
			}
		})
		ui.Row(c).Gap(4).Justify(ui.Center).Children(func() {
			if components.Button(c.Key("rebuild"), T("action.rebuild"), components.ButtonOpts{Variant: components.Ghost, Icon: lobeui.IconRefresh(), Size: design.ControlSM}).Clicked() {
				a.startBuild(pr.ID, j.Targets)
			}
			if components.Button(c.Key("edit"), T("action.edit"), components.ButtonOpts{Variant: components.Ghost, Icon: lobeui.IconSliders(), Size: design.ControlSM}).Clicked() {
				a.projTab = 1
			}
			if components.Button(c.Key("log"), T("build.viewLog"), components.ButtonOpts{Variant: components.Ghost, Icon: lobeui.IconLog(), Size: design.ControlSM}).Clicked() {
				a.logOpen[j.ID] = !a.logOpen[j.ID]
			}
		})
		if art != nil {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(8, 12).Radius(design.RadiusControl).Background(p.Well).MaxWidth(560).Children(func() {
				ui.Icon(c, lobeui.IconFolder()).FontSize(13).TextColor(p.TextTertiary)
				components.Mono(c, art.Path).SingleLine().MinWidth(0).Shrink(1).Selectable()
				lobeui.CopyButton(c, art.Path)
			})
		}
		if len(j.Warnings) > 0 {
			for i, w := range j.Warnings {
				components.Note(c.Key(i), lobeui.IconInfo(), w, components.ToneWarning)
			}
		}
		if a.logOpen[j.ID] {
			ui.Column(c).FillWidth().Children(func() {
				var lines []components.LogLine
				for _, l := range a.logs[j.ID] {
					lines = append(lines, components.LogLine{Time: l.Time.Format("15:04:05"), Level: l.Level, Text: l.Message})
				}
				components.LogViewer(c, lines, 220)
			})
		}
	}).Role(ui.RoleRegion).Label(T("build.success.title"))
}

func (a *App) failurePanel(c *ui.Context, pr *domain.Project, j *domain.BuildJob) {
	e := j.Error
	if e == nil {
		e = &domain.Error{Code: "BUILD_FAILED", Message: T("build.failed.generic")}
	}
	hasPrev := false
	for _, o := range a.jobs {
		if o.ProjectID == pr.ID && o.Status == domain.JobSucceeded && o.CreatedAt.Before(j.CreatedAt) {
			hasPrev = true
			break
		}
	}
	note := T("build.failed.noteDraft")
	if hasPrev {
		note = T("build.failed.notePrev")
	}
	retry, copy, extra := components.ErrorPanel(c, components.ErrorInfo{
		What: e.Message, Why: e.Cause, Fix: e.Remediation, Code: e.Code,
		WhyLabel: T("error.why"), FixLabel: T("error.fix"), Note: note,
		Retry: T("action.retry"), Copy: T("error.copyDiagnostic"), Extra: T("build.viewLog"),
	})
	if retry {
		a.startBuild(pr.ID, j.Targets)
	}
	if copy {
		c.WriteClipboard(a.diagnostic(j))
		a.toast(T("error.copied"), "", 1)
	}
	if extra {
		a.logOpen[j.ID] = !a.logOpen[j.ID]
	}
	if a.logOpen[j.ID] {
		var lines []components.LogLine
		for _, l := range a.logs[j.ID] {
			lines = append(lines, components.LogLine{Time: l.Time.Format("15:04:05"), Level: l.Level, Text: l.Message})
		}
		components.LogViewer(c, lines, 220)
	}
}

// diagnostic is a stable, parseable, non-localized report (spec §9D.3).
func (a *App) diagnostic(j *domain.BuildJob) string {
	var b strings.Builder
	fmt.Fprintf(&b, "mypack %s diagnostic\n", Version)
	fmt.Fprintf(&b, "job: %s\nproject: %s (rev %d)\nstatus: %s\nstage: %s\nruntime: %s\n", j.ID, j.ProjectID, j.ProjectRev, j.Status, j.Stage, j.RuntimeVersion)
	for _, t := range j.Targets {
		fmt.Fprintf(&b, "target: %s/%s/%s\n", t.OS, t.Arch, t.Format)
	}
	fmt.Fprintf(&b, "host: %s\n", a.host.Key())
	if j.Error != nil {
		fmt.Fprintf(&b, "error.code: %s\nerror.message: %s\nerror.cause: %s\nerror.remediation: %s\n", j.Error.Code, j.Error.Message, j.Error.Cause, j.Error.Remediation)
	}
	fmt.Fprintf(&b, "configHash: %s\ntime: %s\n--- log ---\n", j.ConfigHash, time.Now().UTC().Format(time.RFC3339))
	for _, l := range a.logs[j.ID] {
		fmt.Fprintf(&b, "%s %-5s %-11s %s\n", l.Time.UTC().Format("15:04:05.000"), l.Level, l.Stage, l.Message)
	}
	return b.String()
}

// ---- actions --------------------------------------------------------------------

func (a *App) startBuild(projectID string, targets []config.Target) {
	a.do(func(ctx context.Context) error {
		j, err := a.svc.StartBuild(ctx, appservice.BuildRequest{ProjectID: projectID, Targets: targets, Actor: "gui"})
		if err == nil {
			a.win.Update(func() {
				a.watch[projectID] = j.ID
				a.upsertJob(j)
			})
		}
		return err
	}, func(err error) {
		if err != nil {
			a.toastErr(err)
			return
		}
		if !(a.section == secProject && a.projectID == projectID) {
			a.openProject(projectID, 0)
		}
		a.projTab = 0
	})
}

func (a *App) cancelJob(id string) {
	a.do(func(ctx context.Context) error { return a.svc.CancelBuild(ctx, id, "gui") }, a.toastErr)
}

func (a *App) installArtifact(artifactID string, replace bool) {
	a.dlg.installing = artifactID
	a.do(func(ctx context.Context) error {
		_, err := a.svc.InstallAndOpen(ctx, appservice.InstallRequest{ArtifactID: artifactID, Replace: replace, Open: true, Actor: "gui"})
		return err
	}, func(err error) {
		a.dlg.installing = ""
		if errCode(err) == "INSTALL_CONFLICT" {
			a.dlg.conflictArtifact = artifactID
			a.dlg.installConflict = true
			return
		}
		if err != nil {
			a.toastErr(err)
			return
		}
		a.toast(T("install.done"), "", 1)
	})
}

func (a *App) openInstall(id string) {
	a.do(func(ctx context.Context) error { return a.svc.OpenInstallation(ctx, id) }, a.toastErr)
}

func (a *App) reveal(path string) {
	a.do(func(ctx context.Context) error { return a.svc.Reveal(ctx, path) }, a.toastErr)
}
