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

// jobBuildsProject reports whether the job's source is a JS project the
// Builder builds first.
func jobBuildsProject(j domain.BuildJob) bool {
	return j.Snapshot.App.Source.Build != nil || j.Stage == domain.StagePreparing
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
			if j.Status == domain.JobCancelled {
				ui.Column(c).Gap(4).Margin(8, 0, 0, 0).Children(func() {
					kit.PageTitle(c, T("build.cancelled.title")).SingleLine()
					ui.Text(c, T("build.cancelled.sub")).FontSize(design.FontBody).TextColor(p.TextSecondary)
				})
				ui.Row(c).Margin(16, 0, 0, 0).Children(func() {
					if kit.Button(c.Key("rebuild"), T("action.buildAgain"), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlLG}).Clicked() {
						a.startBuild(pr.ID, j.Targets)
					}
				})
			} else {
				ui.Column(c).FillWidth().AlignItems(ui.Center).Margin(12, 0, 0, 0).Children(func() { a.buildSteps(c, pr, j, false, false) })
			}
			_ = running
			ui.Column(c).FillWidth().Gap(10).Margin(16, 0, 0, 0).Children(func() {
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
				if kit.Button(c.Key("reveal"), T("action.showInFinder"), kit.ButtonOpts{Variant: kit.Ghost, Size: 26}).FontWeight(design.WeightRegular).Clicked() {
					a.reveal(path)
				}
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
			if kit.Button(c.Key("cancel"), T("common.cancel"), kit.ButtonOpts{Variant: kit.Ghost, Size: 26}).FontWeight(design.WeightRegular).Clicked() {
				a.cancelJob(id)
			}
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

// successInfo is what a finished build offers, for the success views.
type successInfo struct {
	art                         *domain.Artifact
	inst                        *domain.Installation
	installed, auto, installing bool
	opened                      time.Time
}

func (a *App) successOf(pr *domain.Project, j *domain.BuildJob) successInfo {
	si := successInfo{art: a.hostArtifact(*j)}
	if si.art == nil && len(j.Artifacts) > 0 {
		si.art = &j.Artifacts[0]
	}
	si.inst = a.installFor(pr.ID)
	si.installed = si.art != nil && si.inst != nil && si.inst.ArtifactID == si.art.ID
	si.opened, si.auto = a.autoOpen[j.ID]
	si.installing = si.art != nil && a.dlg.installing == si.art.ID
	return si
}

// successExtras are a finished build's warnings and, when asked, its log.
func (a *App) successExtras(c *ui.Context, j *domain.BuildJob, si successInfo) {
	if len(j.Warnings) > 0 {
		ui.Column(c).FillWidth().Gap(8).Margin(16, 0, 0, 0).Children(func() {
			for i, w := range j.Warnings {
				kit.Note(c.Key(i), icons.Info, w, kit.ToneWarning)
			}
		})
	}
	if a.logOpen[j.ID] {
		ui.Column(c).FillWidth().Margin(16, 0, 0, 0).Children(func() {
			if si.art != nil {
				kit.Mono(c, si.art.Path).SingleLine().Selectable().Margin(0, 0, 8, 0)
			}
			a.logView(c, j.ID, 220)
		})
	}
}

func (a *App) successPanel(c *ui.Context, pr *domain.Project, j *domain.BuildJob) {
	si := a.successOf(pr, j)
	ui.Column(c).FillWidth().Padding(8, 0, 8, 0).Gap(16).Children(func() {
		// The result is the app, launched (its window springs open); under
		// it the steps, then the result bar.
		a.mockup(c, c.Key("launched"), pr.Config, pr.IconPNG, pr.Config.App.Name, 260, true)
		ui.Column(c).FillWidth().AlignItems(ui.Center).Children(func() { a.buildFooter(c, pr, j) })
		a.successExtras(c, j, si)
	}).Role(ui.RoleRegion).Label(T("build.success.title"))
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

// failurePanel is a failed build on its project page: the scene (it shook),
// the result bar in its failed state, then why and what to do.
func (a *App) failurePanel(c *ui.Context, pr *domain.Project, j *domain.BuildJob) {
	ui.Column(c).FillWidth().Padding(8, 0, 8, 0).Gap(16).Children(func() {
		a.mockup(c, c.Key("mockup"), pr.Config, pr.IconPNG, pr.Config.App.Name, 220, false)
		ui.Column(c).FillWidth().AlignItems(ui.Center).Children(func() { a.buildFooter(c, pr, j) })
	}).Role(ui.RoleRegion).Label(T("build.failed.generic"))
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
	native := func() {
		kit.BuildScene(c, kit.SceneOpts{Stage: st, StageAt: m.at, Progress: a.statusProgress(*j), Failed: j.Status == domain.JobFailed,
			Cancelled: j.Status == domain.JobCancelled, Icon: pr.IconPNG, Name: pr.Config.App.Name, Host: host,
			Tint: kit.IconTint(pr.IconPNG, pr.Config.App.Name), Height: h})
	}
	native()
}
