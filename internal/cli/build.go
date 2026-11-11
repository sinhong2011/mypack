package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

func (a *app) cmdBuild(args []string) int {
	var asJSON bool
	fs := a.flagSet("build", &asJSON)
	target := fs.String("target", "", "os-arch[,os-arch] (e.g. darwin-arm64); default: project targets")
	format := fs.String("format", "", "output format (app, portable, dmg, zip, deb, appimage, exe)")
	output := fs.String("output", "", "output directory")
	yes := fs.Bool("yes", false, "allow downloading a missing runtime")
	fs.BoolVar(yes, "y", false, "alias for --yes")
	runBuild := fs.Bool("run-build", false, "approve running the project's own build (install + build script) for this app")
	pos, err := parse(fs, args)
	if err != nil || len(pos) != 1 {
		return a.usageErr(asJSON, "usage: mypack build <project-id|mypack.json|project-folder> [--target T] [--format F] [--output DIR] [--yes] [--run-build] [--json]")
	}
	targets, err := parseTargets(*target, *format)
	if err != nil {
		return a.usageErr(asJSON, "%v", err)
	}

	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	ctx, stop := signalContext()
	defer stop()

	projectID := pos[0]
	if looksLikeConfigFile(pos[0]) {
		p, perr := a.ensureProjectForFile(ctx, svc, pos[0])
		if perr != nil {
			return a.fail(perr, asJSON, nil)
		}
		projectID = p.ID
	}

	if st, serr := os.Stat(pos[0]); serr == nil && st.IsDir() && !strings.HasPrefix(pos[0], "prj_") {
		p, perr := a.ensureProjectForDir(ctx, svc, pos[0])
		if perr != nil {
			return a.fail(perr, asJSON, nil)
		}
		projectID = p.ID
	}

	job, err := svc.StartBuild(ctx, appservice.BuildRequest{
		ProjectID: projectID, Targets: targets, AllowDownload: *yes, AllowProjectBuild: *runBuild,
		OutputDir: *output, Actor: "cli"})
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	if job.Status == domain.JobAwaitingApproval {
		// Non-interactive policy: never block on a prompt.
		e := job.Error
		if e == nil {
			e = &domain.Error{Code: "RUNTIME_DOWNLOAD_REQUIRED", Message: "A runtime download needs approval."}
		}
		if !asJSON {
			if job.Approval != nil && job.Approval.Kind == domain.ApprovalProjectBuild {
				fmt.Fprintf(a.errw, "Building this app first runs your project's build:\n    %s\n  in %s\nRe-run with --run-build to approve it, or approve job %s in mypack.\n",
					job.Approval.Command, job.Approval.Path, job.ID)
			} else {
				fmt.Fprintf(a.errw, "Job %s is waiting for approval. Re-run with --yes or approve it in mypack.\n", job.ID)
			}
		}
		extra := map[string]any{"jobId": job.ID, "projectId": projectID, "status": "approval_required"}
		if job.Approval != nil {
			extra["approval"] = job.Approval
		}
		return a.fail(e, asJSON, extra)
	}

	unsub := func() {}
	if !asJSON {
		unsub = a.streamProgress(svc, job.ID)
	}
	final, cancelled := a.waitForJob(ctx, svc, job.ID)
	unsub()
	if cancelled {
		// Ctrl-C: ask the service to stop the build, then report it.
		_ = svc.CancelBuild(context.Background(), job.ID, "cli")
		final, _ = a.waitForJob(context.Background(), svc, job.ID)
	}
	return a.reportBuild(final, asJSON)
}

// parseTargets turns --target/--format into config targets.
func parseTargets(target, format string) ([]config.Target, error) {
	if target == "" {
		if format != "" {
			return nil, fmt.Errorf("--format requires --target")
		}
		return nil, nil
	}
	var out []config.Target
	for _, part := range strings.Split(target, ",") {
		part = strings.TrimSpace(part)
		i := strings.IndexByte(part, '-')
		if i <= 0 || i == len(part)-1 {
			return nil, fmt.Errorf("invalid --target %q (want os-arch, e.g. darwin-arm64)", part)
		}
		t := config.Target{OS: part[:i], Arch: part[i+1:], Format: format}
		if t.Format == "" {
			t.Format = config.DefaultTarget(t).Format
		}
		out = append(out, t)
	}
	return out, nil
}

// streamProgress prints stage changes and log lines to stderr.
func (a *app) streamProgress(svc appservice.Service, jobID string) (unsubscribe func()) {
	var last domain.Stage
	return svc.Subscribe(func(e appservice.Event) {
		if e.JobID != jobID {
			return
		}
		switch e.Kind {
		case appservice.EventJobLog:
			if e.Log != nil {
				fmt.Fprintf(a.errw, "  %s\n", e.Log.Message)
			}
		case appservice.EventJobChanged:
			if e.Job != nil && e.Job.Stage != last && e.Job.Stage != "" {
				last = e.Job.Stage
				fmt.Fprintf(a.errw, "[%s]\n", last)
			}
		}
	})
}

func terminal(s domain.JobStatus) bool {
	switch s {
	case domain.JobSucceeded, domain.JobFailed, domain.JobCancelled, domain.JobAwaitingApproval:
		return true
	}
	return false
}

// waitForJob polls until the job reaches a terminal state. cancelled is true
// when ctx ended first.
func (a *app) waitForJob(ctx context.Context, svc appservice.Service, id string) (domain.BuildJob, bool) {
	t := time.NewTicker(150 * time.Millisecond)
	defer t.Stop()
	for {
		j, err := svc.GetJob(context.Background(), id)
		if err == nil && terminal(j.Status) {
			return j, false
		}
		select {
		case <-ctx.Done():
			return j, true
		case <-t.C:
		}
	}
}

func (a *app) reportBuild(j domain.BuildJob, asJSON bool) int {
	name := ""
	if j.Snapshot != nil {
		name = j.Snapshot.App.Name
	}
	outputs := make([]map[string]any, 0, len(j.Artifacts))
	for _, art := range j.Artifacts {
		outputs = append(outputs, map[string]any{
			"artifactId": art.ID, "target": art.Target.Key(), "format": art.Target.Format, "path": art.Path,
			"size": art.Size, "sha256": art.SHA256, "signature": art.Signature,
		})
	}
	warnings := j.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	platform, arch := "", ""
	if len(j.Targets) > 0 {
		platform, arch = j.Targets[0].OS, j.Targets[0].Arch
	}
	base := map[string]any{
		"name": name, "platform": platform, "arch": arch, "projectId": j.ProjectID, "jobId": j.ID,
		"outputs": outputs, "warnings": warnings,
	}

	switch j.Status {
	case domain.JobSucceeded:
		if asJSON {
			a.okJSON(base)
		} else {
			for _, art := range j.Artifacts {
				fmt.Fprintf(a.out, "%s  %s  %d bytes  sha256:%s\n", art.Target.Key(), art.Path, art.Size, art.SHA256)
			}
			for _, w := range warnings {
				fmt.Fprintf(a.errw, "warning: %s\n", w)
			}
			if len(j.Artifacts) > 0 {
				fmt.Fprintf(a.errw, "Install with: mypack install %s --open\n", j.Artifacts[0].ID)
			}
		}
		return ExitOK
	case domain.JobCancelled:
		return a.fail(&domain.Error{Code: "BUILD_CANCELLED", Message: "The build was cancelled.",
			Remediation: "Previous build results were left untouched."}, asJSON, base)
	default:
		e := j.Error
		if e == nil {
			e = &domain.Error{Code: "BUILD_FAILED", Message: "The build failed."}
		}
		code := a.fail(e, asJSON, base)
		if code == ExitGeneric { // an unclassified failure of a build is still a build failure
			code = ExitBuildFailed
		}
		return code
	}
}

// ensureProjectForDir returns the project for a project/site folder given to
// `build`: the existing one for that folder, else a new draft.
func (a *app) ensureProjectForDir(ctx context.Context, svc appservice.Service, dir string) (domain.Project, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	projects, err := svc.ListProjects(ctx)
	if err != nil {
		return domain.Project{}, err
	}
	for _, p := range projects {
		src := p.Config.App.Source
		if src.Type == config.SourceDir && (src.Project == abs || src.Value == abs) {
			return p, nil
		}
	}
	return svc.CreateProject(ctx, appservice.CreateRequest{Input: abs, Actor: "cli"})
}
