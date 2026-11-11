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

// timelineSteps converts a job into honest timeline steps; a project
// that is built first gets a "Build project" step ahead of the rest.
func (a *App) timelineSteps(j domain.BuildJob) []kit.Step {
	steps := a.pipelineSteps(j)
	if !jobBuildsProject(j) {
		return steps
	}
	prep := kit.Step{Label: T("step.prepare"), Progress: -1}
	switch {
	case j.Status == domain.JobAwaitingApproval && j.Approval != nil && j.Approval.Kind == "project_build":
		prep.State = kit.StepPending
	case j.Stage == domain.StagePreparing && j.Status == domain.JobFailed:
		prep.State = kit.StepFailed
	case j.Stage == domain.StagePreparing && (j.Status == domain.JobRunning || j.Status == domain.JobQueued):
		prep.State = kit.StepActive
		for i := range steps {
			steps[i].State = kit.StepPending
		}
	default:
		prep.State = kit.StepDone
	}
	return append([]kit.Step{prep}, steps...)
}

// jobBuildsProject reports whether the job's source is a JS project the
// Builder builds first.
func jobBuildsProject(j domain.BuildJob) bool {
	return j.Snapshot.App.Source.Build != nil || j.Stage == domain.StagePreparing
}

func (a *App) pipelineSteps(j domain.BuildJob) []kit.Step {
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
		ui.Column(c).FillWidth().Gap(0).Margin(8, 0, 0, 0).Children(func() {
			running := j.Status == domain.JobRunning || j.Status == domain.JobQueued
			a.sceneFor(c, pr, j, 280)
			// Aligned to the content edge: what is happening, in one line.
			ui.Column(c).Gap(4).Margin(8, 0, 0, 0).Children(func() {
				title := T("build.progress.title", "device", targetsWord(a.host, j.Targets))
				if j.Status == domain.JobCancelled {
					title = T("build.cancelled.title")
				}
				kit.PageTitle(c, title).SingleLine()
				if running && j.Stage == domain.StagePreparing {
					// The project's own build: its latest output, line by line.
					last := ""
					if ls := a.logs[j.ID]; len(ls) > 0 {
						last = ls[len(ls)-1].Message
					}
					ui.Text(c, T("step.prepare.active")).FontSize(design.FontBody).TextColor(p.TextSecondary).SingleLine()
					if last != "" {
						kit.Mono(c, last).SingleLine().TextColor(p.TextTertiary).Role(ui.RoleStatus).Margin(2, 0, 0, 0)
					}
				} else if running {
					// The stage name rolls in as it changes.
					ui.Box(c).Height(20).FillWidth().Clip().Children(func() {
						kit.Roll(ui.Text(c.Key(T(visibleStepLabel(j.Stage))), stageLabel(*j)).FontSize(design.FontBody).
							TextColor(p.TextSecondary).Role(ui.RoleStatus).SingleLine().FontFeatures("tnum").Absolute().Left(0).Right(0))
					})
				} else {
					ui.Text(c, T("build.cancelled.sub")).FontSize(design.FontBody).TextColor(p.TextSecondary)
				}
			})
			switch j.Status {
			case domain.JobRunning, domain.JobQueued:
				id := j.ID
				kit.Links(c.Key("links"), []kit.Link{{Label: T("common.cancel"), Run: func() { a.cancelJob(id) }}}).Margin(10, 0, 0, -6)
			case domain.JobCancelled:
				ui.Row(c).Margin(16, 0, 0, 0).Children(func() {
					if kit.Button(c.Key("rebuild"), T("action.buildAgain"), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlLG}).Clicked() {
						a.startBuild(pr.ID, j.Targets)
					}
				})
			}
			ui.Column(c).FillWidth().Gap(10).Margin(20, 0, 0, 0).Children(func() {
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
	if j.Approval != nil && j.Approval.Kind == "project_build" {
		a.projectBuildApproval(c, j)
		return
	}
	a.runtimeApproval(c, j)
}

// projectBuildApproval asks, once per project, to run the project's own
// build: one sentence, the exact command, where it runs, one primary
// action and a quiet Cancel.
func (a *App) projectBuildApproval(c *ui.Context, j *domain.BuildJob) {
	p := design.Of(c)
	ap := j.Approval
	name := j.Snapshot.App.Name
	ui.Column(c).FillWidth().Gap(0).Margin(24, 0, 0, 0).Children(func() {
		kit.PageTitle(c, T("projectBuild.title", "name", name))
		ui.Text(c, nonEmpty(ap.Detail, T("projectBuild.body"))).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.4).Margin(4, 0, 0, 0)
		// The exact command, as it will run.
		ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(16, 0, 0, 0).Padding(8, 8, 8, 12).Radius(design.RadiusField).
			Background(p.Sunken).Border(1, p.Border).Children(func() {
			kit.Mono(c, ap.Command).Selectable().Grow(1).MinWidth(0)
			if kit.CopyButton(c.Key("copy"), ap.Command, T("action.copy")) {
				a.toast(T("common.copied"), "", 1)
			}
		})
		if ap.Path != "" {
			path := ap.Path
			ui.Row(c).Gap(4).AlignItems(ui.Center).Margin(8, 0, 0, 0).Children(func() {
				ui.Text(c, T("projectBuild.in", "path", shortPath(path))).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine().MinWidth(0).Shrink(1)
				kit.Links(c.Key("reveal"), []kit.Link{{Label: T("action.showInFinder"), Run: func() { a.reveal(path) }}})
			})
			if _, err := os.Stat(filepath.Join(path, "node_modules")); err != nil {
				ui.Text(c, T("projectBuild.depsFirst")).FontSize(design.FontSmall).TextColor(p.TextSecondary).Margin(2, 0, 0, 0)
			}
		}
		id := j.ID
		ui.Row(c).Gap(6).AlignItems(ui.Center).Margin(18, 0, 0, 0).Children(func() {
			if kit.Button(c.Key("run"), T("projectBuild.run"), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlLG,
				Loading: a.dlg.approving == id}).Clicked() {
				a.approveJob(id)
			}
			kit.Links(c.Key("cancel"), []kit.Link{{Label: T("common.cancel"), Run: func() { a.cancelJob(id) }}})
		})
	})
}

// approveJob approves a waiting job and resumes it.
func (a *App) approveJob(id string) {
	a.dlg.approving = id
	a.do(func(ctx context.Context) error { return a.svc.ApproveJob(ctx, id, "gui") }, func(err error) {
		a.dlg.approving = ""
		a.toastErr(err)
	})
}

// runtimeApproval asks to download a runtime (approval kind
// "runtime_download").
func (a *App) runtimeApproval(c *ui.Context, j *domain.BuildJob) {
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
	body := T("runtime.first.body")
	if j.Approval != nil && j.Approval.Detail != "" {
		body = j.Approval.Detail
	}
	approve, cancel := kit.RuntimeDownloadCard(c, kit.RuntimeDownloadInfo{
		Title: T("runtime.first.title"), Body: body,
		SizeLabel: T("runtime.first.size"), SourceLabel: T("runtime.first.source"), CacheLbl: T("runtime.first.cache"),
		Size: bytesLabel(size), Source: src + " · " + T("runtime.first.signed"), Cache: cache,
		Approve: T("runtime.first.approve"), Cancel: T("common.cancel"), Busy: a.dlg.approving == j.ID,
	})
	if approve {
		a.approveJob(j.ID)
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
	art := a.hostArtifact(*j)
	if art == nil && len(j.Artifacts) > 0 {
		art = &j.Artifacts[0]
	}
	inst := a.installFor(pr.ID)
	installed := art != nil && inst != nil && inst.ArtifactID == art.ID
	opened, auto := a.autoOpen[j.ID]
	installing := art != nil && a.dlg.installing == art.ID
	ui.Column(c).FillWidth().Padding(8, 0, 8, 0).Gap(0).Children(func() {
		// The scene's last frame is the hero: the app settled in the Dock.
		a.sceneFor(c, pr, j, 260)
		// Then, aligned to the content edge: its name and one quiet line.
		ui.Column(c).Gap(4).Margin(8, 0, 0, 0).Children(func() {
			kit.PageTitle(c, pr.Config.App.Name).SingleLine()
			var parts []string
			switch {
			case installing:
				parts = append(parts, T("autoInstall.installing"))
			case installed:
				parts = append(parts, installedWhere(a.host))
				if auto && !opened.IsZero() {
					parts = append(parts, T("success.openedAgo", "when", agoShort(opened)))
				}
			default:
				parts = append(parts, T("success.builtFor", "device", targetsWord(a.host, j.Targets)))
			}
			if art != nil {
				parts = append(parts, bytesLabel(art.Size))
			}
			if j.StartedAt != nil && j.FinishedAt != nil {
				parts = append(parts, duration(j.FinishedAt.Sub(*j.StartedAt)))
			}
			ui.Text(c, strings.Join(parts, " · ")).FontSize(design.FontBody).TextColor(p.TextSecondary).FontFeatures("tnum").SingleLine()
		})
		// One primary action, only when there is something to do.
		if art != nil && art.Target.OS == a.host.OS && !installed {
			ui.Row(c).Margin(16, 0, 0, 0).Children(func() {
				if kit.Button(c.Key("install"), T("action.installOpen"), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlLG,
					Loading: installing}).Clicked() || c.Shortcut(0, ui.KeyEnter) {
					a.installArtifact(art.ID, false)
				}
			})
		}
		// Everything else is a quiet row of links.
		var links []kit.Link
		if installed {
			label := T("action.open")
			if auto {
				label = T("success.openAgain")
			}
			id := inst.ID
			links = append(links, kit.Link{Label: label, Run: func() { a.openInstall(id) }})
		}
		if art != nil {
			path := art.Path
			links = append(links, kit.Link{Label: T("action.showInFinder"), Icon: icons.Folder, Run: func() { a.reveal(path) }})
		}
		if a.section == secCreate {
			id := pr.ID
			links = append([]kit.Link{{Label: T("create.viewProject"), Run: func() {
				a.create.reset(a.host)
				a.openProject(id, 0)
			}}}, links...)
		}
		links = append(links,
			kit.Link{Label: T("action.rebuild"), Run: func() { a.startBuild(pr.ID, j.Targets) }},
			kit.Link{Label: T("build.viewLog"), Run: func() { a.logOpen[j.ID] = !a.logOpen[j.ID] }},
			kit.Link{Label: T("success.addAccount"), Run: func() {
				src := pr.Config.App.Source.Value
				a.create.reset(a.host)
				a.create.setInput(src)
				a.nav(secCreate)
			}})
		kit.Links(c.Key("links"), links).Margin(installedMargin(installed), 0, 0, -6)
		if len(j.Warnings) > 0 {
			ui.Column(c).FillWidth().Gap(8).Margin(16, 0, 0, 0).Children(func() {
				for i, w := range j.Warnings {
					kit.Note(c.Key(i), icons.Info, w, kit.ToneWarning)
				}
			})
		}
		if a.logOpen[j.ID] {
			ui.Column(c).FillWidth().Margin(16, 0, 0, 0).Children(func() {
				if art != nil {
					kit.Mono(c, art.Path).SingleLine().Selectable().Margin(0, 0, 8, 0)
				}
				a.logView(c, j.ID, 220)
			})
		}
	}).Role(ui.RoleRegion).Label(T("build.success.title"))
}

func installedMargin(installed bool) float32 {
	if installed {
		return 12
	}
	return 10
}

// installedWhere says where an installed app now lives.
func installedWhere(host config.Target) string {
	if host.OS == "darwin" {
		return T("success.installedApplications")
	}
	return T("success.installed")
}

// agoShort is "just now" within a minute, else ago().
func agoShort(t time.Time) string {
	if time.Since(t) < time.Minute {
		return T("time.justNow")
	}
	return ago(t)
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
	why := e.Cause
	if e.Code == "PROJECT_BUILD_FAILED" {
		why = "" // its last lines show below, as code
	}
	retry, copy, extra := kit.ErrorPanel(c, kit.ErrorInfo{
		What: e.Message, Why: why, Fix: e.Remediation, Code: e.Code,
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
	switch e.Code {
	case "NODE_NOT_FOUND":
		kit.Links(c.Key("node"), []kit.Link{{Label: T("projectBuild.getNode"), Run: func() {
			a.do(func(ctx context.Context) error { return a.svc.OpenURL(ctx, "https://nodejs.org") }, a.toastErr)
		}}}).Margin(8, 0, 0, -6)
	case "PROJECT_BUILD_FAILED":
		if strings.TrimSpace(e.Cause) != "" {
			p := design.Of(c)
			ui.Column(c).FillWidth().Margin(12, 0, 0, 0).Padding(10, 12).Radius(design.RadiusField).Background(p.Sunken).Border(1, p.Border).Children(func() {
				kit.Mono(c, strings.TrimRight(e.Cause, "\n")).Selectable()
			})
		}
	}
	if a.logOpen[j.ID] {
		a.logView(c, j.ID, 220)
	}
}

// diagnostic is a stable, parseable, non-localized report (spec §9D.3).
func (a *App) diagnostic(j *domain.BuildJob) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Mypack %s diagnostic\n", Version)
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

// hostArtifact is the job's artifact for this device, if it made one.
func (a *App) hostArtifact(j domain.BuildJob) *domain.Artifact {
	for i := range j.Artifacts {
		if t := j.Artifacts[i].Target; t.OS == a.host.OS && t.Arch == a.host.Arch {
			return &j.Artifacts[i]
		}
	}
	for i := range j.Artifacts {
		if j.Artifacts[i].Target.OS == a.host.OS {
			return &j.Artifacts[i]
		}
	}
	return nil
}

// autoInstall installs and opens a successful build at once when its
// project asks for it ("Install & Open when done"). It replaces only an
// installation mypack manages; anything else asks first (the conflict
// dialog), and a running app is left alone with a toast.
func (a *App) autoInstall(j domain.BuildJob) {
	if !a.ui.autoInstall(j.ProjectID) {
		return
	}
	art := a.hostArtifact(j)
	if art == nil {
		return
	}
	replace := false
	if inst := a.installFor(j.ProjectID); inst != nil {
		if inst.ArtifactID == art.ID {
			return
		}
		replace = inst.State == domain.InstallManaged || inst.State == domain.InstallMissing || inst.State == domain.InstallAttention
	}
	if a.autoOpen == nil {
		a.autoOpen = map[string]time.Time{}
	}
	a.autoOpen[j.ID] = time.Time{}
	jobID, name := j.ID, j.Snapshot.App.Name
	a.installArtifactThen(art.ID, replace, name, func(ok bool) {
		if ok {
			a.autoOpen[jobID] = time.Now()
		} else {
			delete(a.autoOpen, jobID)
		}
	})
}

func (a *App) installArtifact(artifactID string, replace bool) {
	a.installArtifactThen(artifactID, replace, "", nil)
}

// installArtifactThen installs and opens an artifact; done, if set, hears
// whether it worked.
func (a *App) installArtifactThen(artifactID string, replace bool, name string, done func(ok bool)) {
	a.dlg.installing = artifactID
	a.do(func(ctx context.Context) error {
		_, err := a.svc.InstallAndOpen(ctx, appservice.InstallRequest{ArtifactID: artifactID, Replace: replace, Open: true, Actor: "gui"})
		return err
	}, func(err error) {
		a.dlg.installing = ""
		if done != nil {
			done(err == nil)
		}
		if errCode(err) == appservice.CodeAppRunning {
			n := name
			if n == "" {
				n = T("app.generic")
			}
			a.toast(T("install.quitToUpdate", "name", n), "", 2)
			return
		}
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
		// The success screen shows the result itself; elsewhere, say it.
		if a.section != secProject && a.section != secCreate {
			a.toast(T("install.done"), "", 1)
		}
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
	host := addressLabel(pr.Config.App.Source, pr.Config.App.Name)
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
