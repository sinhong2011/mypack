package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/builder"
	"mypack.dev/mypack/internal/desktop/fakesvc"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/installer"
	"mypack.dev/mypack/internal/metadata"
	"mypack.dev/mypack/pkg/config"
)

// TestLibraryFollowsExternalProcess is the bug: MyPack is already open, then
// MCP creates and builds a draft and the CLI installs it. The Library must
// pick up the project, the job and the installation without a restart.
func TestLibraryFollowsExternalProcess(t *testing.T) {
	prev := externalWatchInterval
	externalWatchInterval = 15 * time.Millisecond
	t.Cleanup(func() { externalWatchInterval = prev })

	dir := t.TempDir()
	host := builder.HostTarget()
	opts := appservice.Options{
		DBPath:     filepath.Join(dir, "mypack.db"),
		OutputDir:  filepath.Join(dir, "out"),
		InstallDir: filepath.Join(dir, "Applications"),
		RoutesFile: "-",
		Host:       &host,
		Engine:     &stubEngine{dir: filepath.Join(dir, "artifacts")},
		Runtimes:   stubRuntimes{},
		Installer:  &stubInstaller{},
		Icons:      stubIcons{},
		Metadata: func(context.Context, string) (metadata.Result, error) {
			return metadata.Result{Title: "Google Translate"}, nil
		},
	}
	guiSvc, err := appservice.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	other, err := appservice.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &App{
		svc:        guiSvc,
		reloadWant: map[string]bool{},
		// The menu-bar icon would try to create a tray. Leave it off.
		settings: appservice.Settings{ShowMenuBarIcon: false},
	}
	var snap struct {
		mu       sync.Mutex
		projects []domain.Project
		installs []domain.Installation
		jobs     []domain.BuildJob
	}
	a.reloaded = func() {
		snap.mu.Lock()
		defer snap.mu.Unlock()
		snap.projects = append([]domain.Project(nil), a.projects...)
		snap.installs = append([]domain.Installation(nil), a.installs...)
		snap.jobs = append([]domain.BuildJob(nil), a.jobs...)
	}
	t.Cleanup(func() {
		a.sync.mu.Lock()
		if a.sync.pending != nil {
			a.sync.pending.Stop()
		}
		a.sync.mu.Unlock()
		a.reloadMu.Lock()
		if a.reloadT != nil {
			a.reloadT.Stop()
		}
		a.reloadMu.Unlock()
		cancel()
		guiSvc.Close()
		other.Close()
	})
	guiSvc.Subscribe(a.onEvent)
	a.watchExternalChanges(ctx)

	// Another process: MCP creates and builds, the CLI installs.
	p, err := other.CreateProject(ctx, appservice.CreateRequest{Input: "translate.google.com", Actor: "mcp:test"})
	if err != nil {
		t.Fatal(err)
	}
	job, err := other.StartBuild(ctx, appservice.BuildRequest{ProjectID: p.ID, Actor: "mcp:test"})
	if err != nil {
		t.Fatal(err)
	}
	job = waitJobStatus(t, other, job.ID, domain.JobSucceeded, domain.JobFailed)
	if job.Status != domain.JobSucceeded || len(job.Artifacts) == 0 {
		t.Fatalf("build: %+v", job.Error)
	}
	inst, err := other.InstallAndOpen(ctx, appservice.InstallRequest{ArtifactID: job.Artifacts[0].ID, Open: true, Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(4 * time.Second)
	var gotP []domain.Project
	var gotI []domain.Installation
	var gotJ []domain.BuildJob
	for time.Now().Before(deadline) {
		snap.mu.Lock()
		gotP = append([]domain.Project(nil), snap.projects...)
		gotI = append([]domain.Installation(nil), snap.installs...)
		gotJ = append([]domain.BuildJob(nil), snap.jobs...)
		snap.mu.Unlock()
		if projectNamed(gotP, "Google Translate") != nil && installNamed(gotI, inst.Name) != nil && jobSucceeded(gotJ, job.ID) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pr := projectNamed(gotP, "Google Translate"); pr == nil || pr.Origin != "mcp" {
		t.Fatalf("projects = %s, want Google Translate from mcp", projectSummary(gotP))
	}
	if in := installNamed(gotI, "Google Translate"); in == nil || in.State != domain.InstallManaged || in.ProjectID != p.ID {
		t.Fatalf("installs = %s, want managed Google Translate", installSummary(gotI))
	}
	if !jobSucceeded(gotJ, job.ID) {
		t.Fatalf("jobs = %s, want %s completed", jobSummary(gotJ), job.ID)
	}
}

func TestWatchExternalSkippedForDemoService(t *testing.T) {
	a := &App{svc: fakesvc.New()}
	// Must not panic or block: the demo service has no shared database.
	a.watchExternalChanges(context.Background())
}

func waitJobStatus(t *testing.T, svc appservice.Service, id string, want ...domain.JobStatus) domain.BuildJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last domain.BuildJob
	for time.Now().Before(deadline) {
		j, err := svc.GetJob(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		last = j
		for _, w := range want {
			if j.Status == w {
				return j
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s stuck in %s", id, last.Status)
	return last
}

func projectNamed(ps []domain.Project, name string) *domain.Project {
	for i := range ps {
		if ps[i].Config != nil && ps[i].Config.App.Name == name {
			return &ps[i]
		}
	}
	return nil
}

func installNamed(ins []domain.Installation, name string) *domain.Installation {
	for i := range ins {
		if ins[i].Name == name {
			return &ins[i]
		}
	}
	return nil
}

func jobSucceeded(js []domain.BuildJob, id string) bool {
	for _, j := range js {
		if j.ID == id && j.Status == domain.JobSucceeded {
			return true
		}
	}
	return false
}

func projectSummary(ps []domain.Project) string {
	out := ""
	for _, p := range ps {
		name := ""
		if p.Config != nil {
			name = p.Config.App.Name
		}
		if out != "" {
			out += ", "
		}
		out += name + " (" + p.Origin + ")"
	}
	if out == "" {
		return "none"
	}
	return out
}

func installSummary(ins []domain.Installation) string {
	out := ""
	for _, in := range ins {
		if out != "" {
			out += ", "
		}
		out += in.Name + " (" + string(in.State) + ")"
	}
	if out == "" {
		return "none"
	}
	return out
}

func jobSummary(js []domain.BuildJob) string {
	out := ""
	for _, j := range js {
		if out != "" {
			out += ", "
		}
		out += j.ID + " " + string(j.Status)
	}
	if out == "" {
		return "none"
	}
	return out
}

// ---- stubs so the other process can create, build and install offline ----

type stubEngine struct{ dir string }

func (f *stubEngine) Build(_ context.Context, req builder.Request) ([]domain.Artifact, error) {
	targets := req.Targets
	if len(targets) == 0 {
		targets = []config.Target{builder.HostTarget()}
	}
	p := filepath.Join(f.dir, "Google Translate.app")
	if err := os.MkdirAll(p, 0o755); err != nil {
		return nil, err
	}
	arts := make([]domain.Artifact, 0, len(targets))
	for _, tgt := range targets {
		arts = append(arts, domain.Artifact{Target: tgt, Path: p, Size: 1, Signature: "unsigned"})
	}
	return arts, nil
}

type stubRuntimes struct{}

func (stubRuntimes) SetRegistryURL(string) {}
func (stubRuntimes) List(context.Context) ([]domain.RuntimeInfo, error) {
	return nil, nil
}
func (stubRuntimes) Status(context.Context, string, string) (domain.RuntimeInfo, error) {
	return domain.RuntimeInfo{Cached: true}, nil
}
func (stubRuntimes) Ensure(context.Context, string, string, func(int64, int64)) (string, error) {
	return "runtime", nil
}
func (stubRuntimes) Verify(context.Context, string, string) error { return nil }
func (stubRuntimes) Delete(string, string) error                  { return nil }
func (stubRuntimes) CacheSize() int64                             { return 0 }

type stubInstaller struct{}

func (stubInstaller) DefaultInstallDir() string   { return "" }
func (stubInstaller) ProfileDir(id string) string { return filepath.Join("profiles", id) }
func (stubInstaller) Install(_ context.Context, art, dir string, _ bool) (string, error) {
	dst := filepath.Join(dir, filepath.Base(art))
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	return dst, nil
}
func (stubInstaller) Replace(context.Context, string, string) (string, error) { return "", nil }
func (stubInstaller) IsRunning(string) bool                                   { return false }
func (stubInstaller) Launch(string) error                                     { return nil }
func (stubInstaller) Reveal(string) error                                     { return nil }
func (stubInstaller) OpenURL(string) error                                    { return nil }
func (stubInstaller) Uninstall(string) error                                  { return nil }
func (stubInstaller) ClearProfile(string) error                               { return nil }
func (stubInstaller) ReadAppInfo(string) (installer.AppInfo, error) {
	return installer.AppInfo{}, nil
}

type stubIcons struct{}

func (stubIcons) Default(string) []byte              { return []byte("icon") }
func (stubIcons) Normalize(b []byte) ([]byte, error) { return b, nil }
func (stubIcons) IconComposerPNG(context.Context, string) ([]byte, error) {
	return []byte("icon"), nil
}
