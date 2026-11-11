package appservice

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"mypack.dev/mypack/internal/domain"
)

// fakePM is a stand-in package manager. It logs every invocation (with the
// env it was given) and behaves according to $FAKE_MODE.
const fakePM = `#!/bin/sh
echo "$(basename "$0") $* | cwd=$(basename "$PWD") CI=$CI NODE_ENV=$NODE_ENV" >> "$FAKE_LOG"
case "$1" in
  ci|install)
    mkdir -p node_modules
    echo "installing deps"
    ;;
  run)
    case "$FAKE_MODE" in
      fail) echo "compiling..."; echo "ERROR: boom in main.tsx" >&2; exit 3 ;;
      nooutput) echo "built, but into nowhere" ;;
      hang) echo started; sleep 300 & echo $! > "$FAKE_PID"; wait ;;
      *) mkdir -p dist; echo '<!doctype html><title>built</title>' > dist/index.html; echo "built ok" ;;
    esac
    ;;
esac
`

type projEnv struct {
	*env
	proj string // project folder
	log  string // fake pm invocation log
	bin  string
}

func skipWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake package manager")
	}
}

// newProjEnv makes a service plus an unbuilt Vite project whose package
// manager ("npm") is the fake script found through PATH.
func newProjEnv(t *testing.T) *projEnv {
	t.Helper()
	skipWindows(t)
	e := newEnv(t)
	root := t.TempDir()
	pe := &projEnv{env: e, proj: filepath.Join(root, "my-app"), log: filepath.Join(root, "pm.log"), bin: filepath.Join(root, "bin")}
	writeFiles(t, pe.proj, map[string]string{
		"package.json": `{"name":"my-app","scripts":{"build":"vite build","generate":"x"},"devDependencies":{"vite":"^6"}}`,
		"index.html":   `<script type="module" src="/src/main.tsx"></script>`,
		"src/main.tsx": "console.log(1)",
	})
	if err := os.MkdirAll(pe.bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pe.bin, "npm"), []byte(fakePM), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pe.bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_LOG", pe.log)
	t.Setenv("FAKE_PID", filepath.Join(root, "pid"))
	t.Setenv("FAKE_MODE", "")
	old := extraBinDirs
	extraBinDirs = func() []string { return nil }
	t.Cleanup(func() { extraBinDirs = old })
	return pe
}

func (pe *projEnv) create(t *testing.T) domain.Project {
	t.Helper()
	p, err := pe.svc.CreateProject(context.Background(), CreateRequest{Input: pe.proj, Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (pe *projEnv) calls(t *testing.T) []string {
	b, err := os.ReadFile(pe.log)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func TestCreateProjectUnbuiltSetsBuildRecipe(t *testing.T) {
	pe := newProjEnv(t)
	p := pe.create(t)
	src := p.Config.App.Source
	if src.Type != "dir" || src.Project != pe.proj || src.Value != filepath.Join(pe.proj, "dist") || src.Entry != "index.html" {
		t.Fatalf("source = %+v", src)
	}
	b := src.Build
	if b == nil || b.PackageManager != "npm" || b.Script != "build" || b.OutDir != "dist" || b.Framework != "Vite" {
		t.Fatalf("build = %+v", b)
	}
	if p.Config.App.Name != "My App" {
		t.Errorf("name = %q", p.Config.App.Name)
	}
	if len(pe.calls(t)) != 0 {
		t.Error("creating a project ran something")
	}
}

func TestProjectBuildApprovalGatingAndOrdering(t *testing.T) {
	pe := newProjEnv(t)
	ctx := context.Background()
	p := pe.create(t)

	job, err := pe.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, Actor: "gui"})
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != domain.JobAwaitingApproval || job.Error == nil || job.Error.Code != CodeProjectBuildRequired {
		t.Fatalf("job = %+v", job)
	}
	ap := job.Approval
	if ap == nil || ap.Kind != domain.ApprovalProjectBuild || ap.Command != "npm install && npm run build" || ap.Path != pe.proj {
		t.Fatalf("approval = %+v", ap)
	}
	if !strings.Contains(job.Error.Cause, "npm run build") {
		t.Errorf("cause = %q", job.Error.Cause)
	}
	if g, _ := pe.svc.GetJob(ctx, job.ID); g.Approval == nil || g.Approval.Command != ap.Command {
		t.Errorf("GetJob lost approval: %+v", g.Approval)
	}
	if len(pe.calls(t)) != 0 || pe.eng.calls != 0 {
		t.Fatal("ran before approval")
	}

	// MCP can never approve, by either route.
	if err := pe.svc.ApproveJob(ctx, job.ID, "mcp:agent"); code(err) != CodeApprovalRequired {
		t.Fatalf("mcp approve: %v", err)
	}
	j2, _ := pe.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, Actor: "mcp:agent", AllowProjectBuild: true})
	if j2.Status != domain.JobAwaitingApproval || len(pe.calls(t)) != 0 {
		t.Fatalf("mcp start with allow: %+v", j2)
	}

	var (
		evMu      sync.Mutex
		stages    []domain.Stage
		logStages []domain.Stage
	)
	pe.svc.Subscribe(func(ev Event) {
		evMu.Lock()
		defer evMu.Unlock()
		if ev.Kind == EventJobChanged && ev.Job != nil && ev.JobID == job.ID {
			if n := len(stages); n == 0 || stages[n-1] != ev.Job.Stage {
				stages = append(stages, ev.Job.Stage)
			}
		}
		if ev.Kind == EventJobLog && ev.JobID == job.ID && ev.Log != nil {
			logStages = append(logStages, ev.Log.Stage)
		}
	})
	if err := pe.svc.ApproveJob(ctx, job.ID, "gui"); err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, pe.svc, job.ID, domain.JobSucceeded, domain.JobFailed)
	if done.Status != domain.JobSucceeded {
		t.Fatalf("done = %+v", done.Error)
	}
	calls := pe.calls(t)
	if len(calls) != 2 || !strings.HasPrefix(calls[0], "npm install |") || !strings.HasPrefix(calls[1], "npm run build |") {
		t.Fatalf("calls = %q", calls)
	}
	if !strings.Contains(calls[0], "cwd=my-app") || !strings.HasSuffix(calls[0], "CI=1 NODE_ENV=") {
		t.Errorf("install env = %q", calls[0])
	}
	if !strings.HasSuffix(calls[1], "CI=1 NODE_ENV=production") {
		t.Errorf("build env = %q", calls[1])
	}
	// The job log carries the process output in the preparing stage.
	log, _ := pe.svc.JobLog(ctx, job.ID)
	var sawBuilt bool
	for _, l := range log {
		if l.Message == "built ok" && l.Stage == domain.StagePreparing {
			sawBuilt = true
		}
	}
	if !sawBuilt {
		t.Errorf("log lacks preparing output: %+v", log)
	}
	evMu.Lock()
	stages, logStages = append([]domain.Stage(nil), stages...), append([]domain.Stage(nil), logStages...)
	evMu.Unlock()
	idx := func(st domain.Stage) int {
		for i, s := range stages {
			if s == st {
				return i
			}
		}
		return -1
	}
	if idx(domain.StagePreparing) < 0 || idx(domain.StagePreparing) > idx(domain.StageResolving) {
		t.Errorf("stages = %v", stages)
	}
	if len(logStages) == 0 {
		t.Error("no live log lines")
	}
	// The engine got the built folder and no local project path / recipe.
	req := pe.eng.reqs[0]
	if req.Config.App.Source.Value != filepath.Join(pe.proj, "dist") || req.Config.App.Source.Project != "" || req.Config.App.Source.Build != nil {
		t.Errorf("engine source = %+v", req.Config.App.Source)
	}

	// Built and fresh: no approval, no second build.
	j3, _ := pe.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, Actor: "mcp:agent"})
	if j3.Status == domain.JobAwaitingApproval {
		t.Fatalf("fresh output asked again: %+v", j3.Error)
	}
	waitJob(t, pe.svc, j3.ID, domain.JobSucceeded)
	if len(pe.calls(t)) != 2 {
		t.Errorf("rebuilt fresh output: %q", pe.calls(t))
	}

	// A newer source file triggers a rebuild; the same fingerprint stays approved
	// and node_modules now exists, so only the build step runs.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(pe.proj, "src", "main.tsx"), future, future); err != nil {
		t.Fatal(err)
	}
	j4, _ := pe.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, Actor: "gui"})
	if j4.Status == domain.JobAwaitingApproval {
		t.Fatalf("approved fingerprint asked again: %+v", j4.Error)
	}
	waitJob(t, pe.svc, j4.ID, domain.JobSucceeded)
	if c := pe.calls(t); len(c) != 3 || !strings.HasPrefix(c[2], "npm run build") {
		t.Errorf("calls = %q", c)
	}
}

func TestProjectBuildFingerprintChangeAsksAgain(t *testing.T) {
	pe := newProjEnv(t)
	ctx := context.Background()
	p := pe.create(t)
	j1, _ := pe.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, Actor: "cli", AllowProjectBuild: true})
	if j1.Status == domain.JobAwaitingApproval {
		t.Fatalf("--run-build still asked: %+v", j1.Error)
	}
	waitJob(t, pe.svc, j1.ID, domain.JobSucceeded)

	// Same command, but the script changes: new fingerprint, new approval, and
	// (never built under it) a build is needed again.
	cfg := p.Config.Clone()
	cfg.App.Source.Build.Script = "generate"
	p, err := pe.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: p.Revision, Config: cfg, Actor: "gui"})
	if err != nil {
		t.Fatal(err)
	}
	j2, _ := pe.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, Actor: "gui"})
	if j2.Status != domain.JobAwaitingApproval || j2.Approval == nil || !strings.HasSuffix(j2.Approval.Command, "npm run generate") {
		t.Fatalf("script change not re-asked: %+v", j2)
	}
	_ = pe.svc.CancelBuild(ctx, j2.ID, "gui")

	// Moving the project folder is a new fingerprint too.
	cfg2 := p.Config.Clone()
	cfg2.App.Source.Build.Script = "build"
	other := filepath.Join(filepath.Dir(pe.proj), "moved")
	writeFiles(t, other, map[string]string{"package.json": `{"scripts":{"build":"x"}}`})
	cfg2.App.Source.Project, cfg2.App.Source.Value = other, filepath.Join(other, "dist")
	p, err = pe.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: p.Revision, Config: cfg2, Actor: "gui"})
	if err != nil {
		t.Fatal(err)
	}
	j3, _ := pe.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, Actor: "gui"})
	if j3.Status != domain.JobAwaitingApproval || j3.Approval.Path != other {
		t.Fatalf("path change not re-asked: %+v", j3)
	}
}

func TestProjectBuildFailureSurfacesLogs(t *testing.T) {
	pe := newProjEnv(t)
	ctx := context.Background()
	t.Setenv("FAKE_MODE", "fail")
	p := pe.create(t)
	job, _ := pe.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, Actor: "cli", AllowProjectBuild: true})
	done := waitJob(t, pe.svc, job.ID, domain.JobFailed, domain.JobSucceeded)
	if done.Status != domain.JobFailed || done.Error == nil || done.Error.Code != CodeProjectBuildFailed {
		t.Fatalf("done = %+v", done)
	}
	if !strings.Contains(done.Error.Cause, "ERROR: boom in main.tsx") || !strings.Contains(done.Error.Cause, "compiling...") ||
		!strings.Contains(done.Error.Message, "status 3") {
		t.Errorf("error = %+v", done.Error)
	}
	if pe.eng.calls != 0 {
		t.Error("engine ran after a failed project build")
	}
	// A failed build must not mark the project as built.
	if st := pe.svc.projectState(ctx, p.ID); st.Built != "" {
		t.Errorf("state = %+v", st)
	}
}

func TestProjectBuildOutputMissing(t *testing.T) {
	pe := newProjEnv(t)
	t.Setenv("FAKE_MODE", "nooutput")
	p := pe.create(t)
	job, _ := pe.svc.StartBuild(context.Background(), BuildRequest{ProjectID: p.ID, Actor: "cli", AllowProjectBuild: true})
	done := waitJob(t, pe.svc, job.ID, domain.JobFailed, domain.JobSucceeded)
	if done.Status != domain.JobFailed || done.Error == nil || done.Error.Code != CodeProjectOutputMissing ||
		!strings.Contains(done.Error.Message, "dist/index.html") {
		t.Fatalf("done = %+v", done.Error)
	}
}

func TestProjectBuildNodeNotFound(t *testing.T) {
	pe := newProjEnv(t)
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	p := pe.create(t)
	job, _ := pe.svc.StartBuild(context.Background(), BuildRequest{ProjectID: p.ID, Actor: "cli", AllowProjectBuild: true})
	done := waitJob(t, pe.svc, job.ID, domain.JobFailed, domain.JobSucceeded)
	if done.Status != domain.JobFailed || done.Error == nil || done.Error.Code != CodeNodeNotFound ||
		!strings.Contains(done.Error.Remediation, "nodejs.org") || !strings.Contains(done.Error.Remediation, "brew install node") {
		t.Fatalf("done = %+v", done.Error)
	}
}

func TestProjectBuildCancelKillsProcess(t *testing.T) {
	pe := newProjEnv(t)
	ctx := context.Background()
	t.Setenv("FAKE_MODE", "hang")
	// Skip the install step so the hanging process is the build.
	if err := os.MkdirAll(filepath.Join(pe.proj, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := pe.create(t)
	job, _ := pe.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, Actor: "cli", AllowProjectBuild: true})
	pidFile := os.Getenv("FAKE_PID")
	var pid int
	for i := 0; i < 300 && pid == 0; i++ {
		if b, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("fake build never started")
	}
	if err := pe.svc.CancelBuild(ctx, job.ID, "gui"); err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, pe.svc, job.ID, domain.JobCancelled, domain.JobFailed)
	if done.Status != domain.JobCancelled {
		t.Fatalf("done = %+v", done)
	}
	alive := true
	for i := 0; i < 300 && alive; i++ {
		alive = processAlive(pid)
		if alive {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if alive {
		t.Errorf("child process %d survived cancellation", pid)
	}
	if pe.eng.calls != 0 {
		t.Error("engine ran after cancel")
	}
}

func TestFindToolSearchesExtraDirs(t *testing.T) {
	skipWindows(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pnpm"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := extraBinDirs
	extraBinDirs = func() []string { return []string{dir} }
	defer func() { extraBinDirs = old }()
	t.Setenv("PATH", t.TempDir())
	exe, d, err := findTool("pnpm")
	if err != nil || d != dir || exe != filepath.Join(dir, "pnpm") {
		t.Fatalf("exe=%q dir=%q err=%v", exe, d, err)
	}
	if _, _, err := findTool("yarn"); err == nil {
		t.Error("found a tool that is not there")
	}
	// nvm: newest version wins.
	home := t.TempDir()
	for _, v := range []string{"v18.20.1", "v22.3.0", "v20.11.0"} {
		_ = os.MkdirAll(filepath.Join(home, ".nvm", "versions", "node", v, "bin"), 0o755)
	}
	if got := newestNvmBin(home); !strings.Contains(got, "v22.3.0") {
		t.Errorf("newest nvm = %q", got)
	}
}

// Real node + npm end to end: create → approve → build runs the project's own
// script → packages dist.
func TestProjectBuildSmokeRealNpm(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not installed")
	}
	e := newEnv(t)
	proj := filepath.Join(t.TempDir(), "tiny-site")
	writeFiles(t, proj, map[string]string{
		"package.json": `{"name":"tiny-site","scripts":{"build":"node build.js"}}`,
		"build.js": `const fs=require('fs');fs.mkdirSync('dist',{recursive:true});` +
			`fs.writeFileSync('dist/index.html','<!doctype html><title>tiny</title>'+process.env.NODE_ENV);`,
	})
	ctx := context.Background()
	p, err := e.svc.CreateProject(ctx, CreateRequest{Input: proj, Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	job, err := e.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, Actor: "cli"})
	if err != nil || job.Status != domain.JobAwaitingApproval {
		t.Fatalf("job=%+v err=%v", job, err)
	}
	if err := e.svc.ApproveJob(ctx, job.ID, "cli"); err != nil {
		t.Fatal(err)
	}
	done := func() domain.BuildJob {
		deadline := time.Now().Add(2 * time.Minute)
		for time.Now().Before(deadline) {
			j, _ := e.svc.GetJob(ctx, job.ID)
			if j.Status == domain.JobSucceeded || j.Status == domain.JobFailed {
				return j
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("timed out")
		return domain.BuildJob{}
	}()
	if done.Status != domain.JobSucceeded {
		t.Fatalf("done = %+v", done.Error)
	}
	b, err := os.ReadFile(filepath.Join(proj, "dist", "index.html"))
	if err != nil || !strings.Contains(string(b), "production") {
		t.Fatalf("dist = %q err=%v", b, err)
	}
	if got := e.eng.reqs[0].Config.App.Source.Value; got != filepath.Join(proj, "dist") {
		t.Errorf("packaged %q", got)
	}
}
