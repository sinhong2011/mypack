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
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
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
func (a *App) timelineSteps(j domain.BuildJob) []kit.Step {
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
	out := make([]kit.Step, len(visibleSteps))
	for i, v := range visibleSteps {
		st := kit.Step{Label: T(v.id), Progress: -1}
		switch {
		case j.Status == domain.JobAwaitingApproval:
			st.State = kit.StepPending
			if i == 0 {
				st.State = kit.StepDone
			}
		case i < cur:
			st.State = kit.StepDone
		case i == cur && j.Status == domain.JobFailed:
			st.State = kit.StepFailed
		case i == cur && (j.Status == domain.JobRunning || j.Status == domain.JobQueued):
			st.State = kit.StepActive
			if j.Progress >= 0 {
				st.Progress = j.Progress
			}
		case j.Status == domain.JobSucceeded:
			st.State = kit.StepDone
		}
		if i == 1 {
			if (st.State == kit.StepDone || j.Status == domain.JobSucceeded) && !downloaded && cur > 1 {
				st.State = kit.StepSkipped
				st.Detail = T("step.download.cached")
			}
			if st.State == kit.StepActive && j.Progress >= 0 {
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
		ui.Column(c).Gap(18).AlignItems(ui.Center).Margin(12, 0, 0, 0).Children(func() {
			running := j.Status == domain.JobRunning || j.Status == domain.JobQueued
			a.sceneFor(c, pr, j, 300)
			ui.Column(c).AlignItems(ui.Center).Gap(4).Children(func() {
				title := T("build.progress.title", "device", targetsWord(a.host, j.Targets))
				sub := T("build.progress.sub")
				if j.Status == domain.JobCancelled {
					title, sub = T("build.cancelled.title"), T("build.cancelled.sub")
				}
				kit.PageTitle(c, title).TextAlign(ui.Center)
				if running {
					// The stage name rolls in as it changes.
					ui.Box(c).Height(20).MinWidth(240).Clip().Children(func() {
						kit.Roll(ui.Text(c.Key(T(visibleStepLabel(j.Stage))), stageLabel(*j)).FontSize(design.FontBody).FontWeight(design.WeightSemibold).
							TextColor(p.AccentText).Role(ui.RoleStatus).SingleLine().Absolute().Left(0).Right(0).TextAlign(ui.Center))
					})
				}
				kit.Caption(c, sub).TextAlign(ui.Center).MaxWidth(420)
			})
			switch j.Status {
			case domain.JobRunning, domain.JobQueued:
				if kit.Button(c.Key("cancel"), T("common.cancel"), kit.ButtonOpts{}).Clicked() {
					a.cancelJob(j.ID)
				}
			case domain.JobCancelled:
				if kit.Button(c.Key("rebuild"), T("action.buildAgain"), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlLG, Icon: icons.Retry}).Clicked() {
					a.startBuild(pr.ID, j.Targets)
				}
			}
			ui.Column(c).FillWidth().Gap(10).Margin(6, 0, 0, 0).Children(func() {
				kit.BuildTimeline(c, a.timelineSteps(*j))
				a.logDisclosure(c, j.ID)
			})
		}).Role(ui.RoleRegion).Label(T("build.progress.region"))
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
	kit.Disclosure(c.Key("log"), &open, T("build.viewLog"), "").Margin(0, -8)
	a.logOpen[jobID] = open
	if open {
		a.logView(c, jobID, 240)
	}
}

func (a *App) logView(c *ui.Context, jobID string, h float32) {
	var lines []kit.LogLine
	for _, l := range a.logs[jobID] {
		lines = append(lines, kit.LogLine{Time: l.Time.Format("15:04:05"), Level: l.Level, Text: l.Message})
	}
	kit.LogViewer(c, lines, h)
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
	approve, cancel := kit.RuntimeDownloadCard(c, kit.RuntimeDownloadInfo{
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
	ui.Column(c).Padding(24, 0, 8, 0).Gap(0).AlignItems(ui.Center).Children(func() {
		a.sceneFor(c, pr, j, 280)
		ui.Column(c).AlignItems(ui.Center).Gap(6).Margin(6, 0, 0, 0).Children(func() {
			kit.LargeTitle(c, T("build.success.title")).TextAlign(ui.Center)
			parts := []string{pr.Config.App.Name, targetsWord(a.host, j.Targets)}
			if j.StartedAt != nil && j.FinishedAt != nil {
				parts = append(parts, duration(j.FinishedAt.Sub(*j.StartedAt)))
			}
			if art != nil {
				parts = append(parts, bytesLabel(art.Size))
			}
			kit.Secondary(c, strings.Join(parts, " · ")).TextAlign(ui.Center)
		})
		inst := a.installFor(pr.ID)
		ui.Row(c).Gap(10).Wrap().Justify(ui.Center).Margin(24, 0, 0, 0).Children(func() {
			if art != nil && art.Target.OS == a.host.OS {
				label := T("action.installOpen")
				if inst != nil && inst.ArtifactID == art.ID {
					label = T("action.open")
				}
				if kit.Button(c.Key("install"), label, kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlXL,
					Icon: icons.Rocket, Loading: a.dlg.installing == art.ID}).MinWidth(200).Clicked() || c.Shortcut(0, ui.KeyEnter) {
					if inst != nil && inst.ArtifactID == art.ID {
						a.openInstall(inst.ID)
					} else {
						a.installArtifact(art.ID, false)
					}
				}
			}
			if art != nil {
				if kit.Button(c.Key("reveal"), T("action.showInFinder"), kit.ButtonOpts{Size: design.ControlXL, Icon: icons.Folder}).Clicked() {
					a.reveal(art.Path)
				}
			}
		})
		ui.Row(c).Gap(4).Justify(ui.Center).Margin(14, 0, 0, 0).Children(func() {
			if kit.Button(c.Key("rebuild"), T("action.rebuild"), kit.ButtonOpts{Variant: kit.Ghost, Icon: icons.Refresh, Size: design.ControlSM}).Clicked() {
				a.startBuild(pr.ID, j.Targets)
			}
			if kit.Button(c.Key("edit"), T("action.edit"), kit.ButtonOpts{Variant: kit.Ghost, Icon: icons.Sliders, Size: design.ControlSM}).Clicked() {
				a.projTab = 1
			}
			if kit.Button(c.Key("log"), T("build.viewLog"), kit.ButtonOpts{Variant: kit.Ghost, Icon: icons.Log, Size: design.ControlSM}).Clicked() {
				a.logOpen[j.ID] = !a.logOpen[j.ID]
			}
		})
		if art != nil {
			design.Well(c, ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(4, 4, 4, 12).Radius(design.RadiusCard).MaxWidth(560).Margin(16, 0, 0, 0)).Children(func() {
				kit.Glyph(c, icons.Folder, 13, p.TextTertiary)
				kit.Mono(c, art.Path).SingleLine().MinWidth(0).Shrink(1).Selectable()
				if kit.CopyButton(c.Key("copy"), art.Path, T("action.copyPath")) {
					a.toast(T("common.copied"), "", 1)
				}
			})
		}
		if len(j.Warnings) > 0 {
			ui.Column(c).FillWidth().Gap(8).Margin(16, 0, 0, 0).Children(func() {
				for i, w := range j.Warnings {
					kit.Note(c.Key(i), icons.Info, w, kit.ToneWarning)
				}
			})
		}
		if a.logOpen[j.ID] {
			ui.Column(c).FillWidth().Margin(16, 0, 0, 0).Children(func() { a.logView(c, j.ID, 220) })
		}
		a.nextSteps(c, pr)
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
	a.sceneFor(c, pr, j, 220)
	retry, copy, extra := kit.ErrorPanel(c, kit.ErrorInfo{
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
		a.logView(c, j.ID, 220)
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
		if errCode(err) == appservice.CodeLicenseLimit {
			a.dlg.licenseLimit = true
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

// sceneMark is when a job's scene entered its current stage.
type sceneMark struct {
	stage int
	at    time.Time
}

// sceneStage maps a job to its scene stage.
func sceneStage(j domain.BuildJob) int {
	switch j.Status {
	case domain.JobSucceeded:
		return kit.SceneDone
	case domain.JobAwaitingApproval, domain.JobQueued:
		return kit.SceneValidate
	}
	return min(stepIndex(j.Stage), kit.SceneVerify)
}

// sceneFor draws the build scene for job j, remembering when it entered
// each stage so its motion starts there.
func (a *App) sceneFor(c *ui.Context, pr *domain.Project, j *domain.BuildJob, h float32) {
	st := sceneStage(*j)
	key := j.ID
	if j.Status == domain.JobFailed || j.Status == domain.JobCancelled {
		key += "/" + string(j.Status)
	}
	m, ok := a.scenes[key]
	if !ok || m.stage != st {
		at := time.Now()
		if !ok && j.Status == domain.JobSucceeded && a.successAt[j.ID].IsZero() {
			at = time.Time{} // an old success: show it settled
		}
		m = sceneMark{stage: st, at: at}
		a.scenes[key] = m
	}
	host := domainOf(pr.Config.App.Source)
	// The icon flies in from Create onto the card; once risen, it is
	// where the icon flies home from when the page is left.
	slot := func(e ui.Element, stage int) {
		if stage == kit.SceneValidate {
			a.flyTo(c, cardKey(pr.ID), e)
		} else {
			a.iconAt[iconKey(pr.ID)] = e.Bounds()
		}
	}
	native := func() {
		kit.BuildScene(c, kit.SceneOpts{IconSlot: slot, Stage: st, StageAt: m.at, Progress: j.Progress, Failed: j.Status == domain.JobFailed,
			Cancelled: j.Status == domain.JobCancelled, Icon: pr.IconPNG, Name: pr.Config.App.Name, Host: host,
			Tint: kit.IconTint(pr.IconPNG, pr.Config.App.Name), Height: h})
	}
	native()
}

// nextSteps fills the space under a success with what to do next, in
// pressed-in wells.
func (a *App) nextSteps(c *ui.Context, pr *domain.Project) {
	p := design.Of(c)
	dockTitle, dockBody := T("next.dock.title"), T("next.dock.body")
	if a.host.OS != "darwin" {
		dockTitle, dockBody = T("next.taskbar.title"), T("next.taskbar.body")
	}
	steps := []struct {
		ic          *ui.SVG
		title, body string
		run         func()
	}{
		{icons.Pin, dockTitle, dockBody, nil},
		{icons.Person, T("next.account.title"), T("next.account.body"), func() {
			src := pr.Config.App.Source.Value
			a.create.reset(a.host)
			a.create.setInput(src)
			a.nav(secCreate)
		}},
		{icons.Sliders, T("next.custom.title"), T("next.custom.body"), func() { a.projTab = 1 }},
	}
	ui.Column(c).FillWidth().Gap(10).Margin(28, 0, 0, 0).Children(func() {
		kit.Eyebrow(c, T("next.title")).AlignSelf(ui.Center)
		ui.Row(c).Gap(10).AlignItems(ui.Stretch).Children(func() {
			for i, st := range steps {
				st := st
				card := ui.ButtonBase(c.Key(i)).Label(st.title).Column().Gap(6).Padding(14, 14).Grow(1).Basis(0).MinWidth(0).
					Radius(design.RadiusCard).AlignItems(ui.Start)
				design.Well(c, card)
				kit.Arrive(card, i+2, design.StaggerGrid)
				if st.run != nil {
					card.Cursor(ui.CursorPointer)
					if card.Hovered() {
						card.Background(p.Hover.Over(p.Sunken))
					}
				}
				card.Children(func() {
					kit.SymbolTile(c, st.ic, p.Accent, 28)
					kit.Headline(c, st.title).Margin(4, 0, 0, 0)
					kit.Caption(c, st.body).LineHeight(1.4)
				})
				if card.Clicked() && st.run != nil {
					st.run()
				}
			}
		})
	})
}
