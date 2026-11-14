package appservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mypack.dev/mypack/internal/builder"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/installer"
	"mypack.dev/mypack/internal/metadata"
	"mypack.dev/mypack/pkg/config"
)

// ---- fakes ---------------------------------------------------------------

type fakeEngine struct {
	mu       sync.Mutex
	calls    int
	cur, max int32
	block    chan struct{} // when non-nil, Build waits on it (or ctx)
	fail     error
	dir      string
	reqs     []builder.Request
}

func (f *fakeEngine) Build(ctx context.Context, req builder.Request) ([]domain.Artifact, error) {
	f.mu.Lock()
	f.calls++
	f.reqs = append(f.reqs, req)
	f.mu.Unlock()
	n := atomic.AddInt32(&f.cur, 1)
	for {
		m := atomic.LoadInt32(&f.max)
		if n <= m || atomic.CompareAndSwapInt32(&f.max, m, n) {
			break
		}
	}
	defer atomic.AddInt32(&f.cur, -1)

	req.Stage(domain.StageResolving, -1)
	req.Log(domain.LogLine{Level: "info", Stage: domain.StageResolving, Message: "resolving"})
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.fail != nil {
		return nil, f.fail
	}
	for i := 0; i < 50; i++ { // progress storm: must be throttled
		req.Stage(domain.StageDownloading, float64(i)/50)
	}
	req.Stage(domain.StagePublishing, -1)
	var arts []domain.Artifact
	for _, t := range req.Targets {
		p := filepath.Join(f.dir, req.Config.App.Name, t.Key(), req.Config.App.Name+".app")
		_ = os.MkdirAll(p, 0o755)
		_ = os.WriteFile(filepath.Join(p, "x"), []byte("12345"), 0o644)
		arts = append(arts, domain.Artifact{Target: t, Path: p, Size: 5, SHA256: "abc", Signature: "adhoc"})
	}
	return arts, nil
}

type fakeRuntimes struct {
	mu       sync.Mutex
	cached   map[string]bool
	size     int64
	registry string
	ensured  []string
}

func (f *fakeRuntimes) SetRegistryURL(u string) { f.mu.Lock(); f.registry = u; f.mu.Unlock() }
func (f *fakeRuntimes) List(ctx context.Context) ([]domain.RuntimeInfo, error) {
	return []domain.RuntimeInfo{{Version: config.DefaultRuntimeVersion, Key: "linux-amd64"}}, nil
}
func (f *fakeRuntimes) Status(ctx context.Context, v, k string) (domain.RuntimeInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return domain.RuntimeInfo{Version: v, Key: k, Cached: f.cached[v+"/"+k], Size: f.size}, nil
}
func (f *fakeRuntimes) Ensure(ctx context.Context, v, k string, p func(a, b int64)) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cached[v+"/"+k] = true
	f.ensured = append(f.ensured, v+"/"+k)
	if p != nil {
		p(1, 2)
	}
	return "/rt", nil
}
func (f *fakeRuntimes) Verify(ctx context.Context, v, k string) error { return nil }
func (f *fakeRuntimes) Delete(v, k string) error                      { delete(f.cached, v+"/"+k); return nil }
func (f *fakeRuntimes) CacheSize() int64                              { return 7 }

type fakeInstaller struct {
	mu        sync.Mutex
	dir       string
	running   bool
	installed []string
	launched  []string
	replaced  [][2]string
	cleared   []string
	uninst    []string
	launchErr error
	info      func(p string) (installer.AppInfo, error) // optional ReadAppInfo override
}

func (f *fakeInstaller) DefaultInstallDir() string   { return filepath.Join(f.dir, "Applications") }
func (f *fakeInstaller) ProfileDir(id string) string { return filepath.Join(f.dir, "profiles", id) }
func (f *fakeInstaller) Install(ctx context.Context, art, dir string, replace bool) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	dst := filepath.Join(dir, filepath.Base(art))
	if _, err := os.Stat(dst); err == nil && !replace {
		return "", &domain.Error{Code: "INSTALL_CONFLICT", Message: "exists"}
	}
	_ = os.MkdirAll(dst, 0o755)
	f.installed = append(f.installed, dst)
	return dst, nil
}
func (f *fakeInstaller) Replace(ctx context.Context, inst, art string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replaced = append(f.replaced, [2]string{inst, art})
	b := inst + ".bak"
	_ = os.MkdirAll(b, 0o755)
	return b, nil
}
func (f *fakeInstaller) IsRunning(string) bool { return f.running }
func (f *fakeInstaller) Launch(p string) error {
	f.launched = append(f.launched, p)
	return f.launchErr
}
func (f *fakeInstaller) Reveal(string) error  { return nil }
func (f *fakeInstaller) OpenURL(string) error { return nil }
func (f *fakeInstaller) Uninstall(p string) error {
	f.uninst = append(f.uninst, p)
	return os.RemoveAll(p)
}
func (f *fakeInstaller) ClearProfile(id string) error { f.cleared = append(f.cleared, id); return nil }
func (f *fakeInstaller) ReadAppInfo(p string) (installer.AppInfo, error) {
	if f.info != nil {
		return f.info(p)
	}
	return installer.AppInfo{Name: "Ext", Identifier: "com.ext.app", Version: "2"}, nil
}

type fakeIcons struct{}

func (fakeIcons) Default(name string) []byte         { return []byte("default:" + name) }
func (fakeIcons) Normalize(b []byte) ([]byte, error) { return append([]byte("norm:"), b...), nil }
func (fakeIcons) IconComposerPNG(_ context.Context, p string) ([]byte, error) {
	return []byte("iconcomposer:" + filepath.Base(p)), nil
}

type env struct {
	svc  *Impl
	eng  *fakeEngine
	rt   *fakeRuntimes
	ins  *fakeInstaller
	dir  string
	opts Options // options used for svc; reuse to open a second "process"
}

func newEnv(t *testing.T, mut ...func(*Options)) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{
		eng: &fakeEngine{dir: filepath.Join(dir, "out")},
		rt:  &fakeRuntimes{cached: map[string]bool{config.DefaultRuntimeVersion + "/linux-amd64": true}, size: 1 << 20},
		ins: &fakeInstaller{dir: dir},
		dir: dir,
	}
	host := config.Target{OS: "linux", Arch: "amd64", Format: "portable"}
	opts := Options{
		DBPath: filepath.Join(dir, "db", "mypack.db"), OutputDir: filepath.Join(dir, "out"),
		Engine: e.eng, Runtimes: e.rt, Installer: e.ins, Icons: fakeIcons{}, Host: &host,
		Metadata: func(ctx context.Context, u string) (metadata.Result, error) {
			return metadata.Result{URL: u, Title: "Fetched: Title/Bad", IconURL: u + "icon.png", IconPNG: []byte("PNG")}, nil
		},
		Capabilities: func(h, t config.Target) builder.Capability {
			return builder.Capability{Supported: t.OS == "linux", Level: "portable", Signing: "unsigned", Reason: "only linux"}
		},
	}
	for _, m := range mut {
		m(&opts)
	}
	e.opts = opts
	svc, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	e.svc = svc
	return e
}

func (e *env) project(t *testing.T) domain.Project {
	t.Helper()
	p, err := e.svc.CreateProject(context.Background(), CreateRequest{Input: "github.com", Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func waitJob(t *testing.T, svc Service, id string, want ...domain.JobStatus) domain.BuildJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, err := svc.GetJob(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range want {
			if j.Status == w {
				return j
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	j, _ := svc.GetJob(context.Background(), id)
	t.Fatalf("job %s stuck in %s (want %v)", id, j.Status, want)
	return j
}

func code(err error) string {
	var de *domain.Error
	if errors.As(err, &de) {
		return de.Code
	}
	return ""
}

// ---- projects --------------------------------------------------------------

func TestCreateProjectURL(t *testing.T) {
	e := newEnv(t)
	p, err := e.svc.CreateProject(context.Background(), CreateRequest{Input: "github.com", Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	c := p.Config
	if c.App.Source.Type != config.SourceURL || c.App.Source.Value != "https://github.com/" {
		t.Errorf("source = %+v", c.App.Source)
	}
	if c.App.Name != "Fetched Title Bad" { // unsafe chars replaced
		t.Errorf("name = %q", c.App.Name)
	}
	if string(p.IconPNG) != "PNG" || c.App.Icon.Source != "auto" {
		t.Errorf("icon = %q / %s", p.IconPNG, c.App.Icon.Source)
	}
	if !p.Draft || p.Origin != "cli" || p.Revision != 1 {
		t.Errorf("project = %+v", p)
	}
	if len(c.Build.Targets) != 1 || c.Build.Targets[0].OS != "linux" {
		t.Errorf("targets = %+v", c.Build.Targets)
	}
	// Second project for the same site gets a distinct identity.
	p2, _ := e.svc.CreateProject(context.Background(), CreateRequest{Input: "https://github.com", Name: "Other", Actor: "gui"})
	if p2.Config.App.Identifier == c.App.Identifier || p2.Config.App.ProfileID == c.App.ProfileID {
		t.Errorf("identity collision: %s", p2.Config.App.Identifier)
	}
	if p2.Config.App.Name != "Other" {
		t.Errorf("name override lost")
	}
	acts, _ := e.svc.Activity(context.Background(), 10)
	if len(acts) == 0 || acts[len(acts)-1].Action != "project.create" {
		t.Errorf("activity = %+v", acts)
	}
}

func TestCreateProjectFallbackIcon(t *testing.T) {
	e := newEnv(t, func(o *Options) {
		o.Metadata = func(ctx context.Context, u string) (metadata.Result, error) {
			return metadata.Result{URL: u, Title: "Foo", Fallback: true}, nil
		}
	})
	p := e.project(t)
	if string(p.IconPNG) != "default:Foo" || p.Config.App.Icon.Source != "default" {
		t.Errorf("icon = %q %s", p.IconPNG, p.Config.App.Icon.Source)
	}
}

func TestCreateProjectLocal(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d := t.TempDir()

	// directory without html
	if _, err := e.svc.CreateProject(ctx, CreateRequest{Input: d}); code(err) != CodeInvalidInput {
		t.Errorf("empty dir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(d, "a.html"), []byte("x"), 0o644)
	p, err := e.svc.CreateProject(ctx, CreateRequest{Input: d})
	if err != nil || p.Config.App.Source.Type != config.SourceDir || p.Config.App.Source.Entry != "a.html" {
		t.Fatalf("dir: %v %+v", err, p.Config.App.Source)
	}
	_ = os.WriteFile(filepath.Join(d, "index.html"), []byte("x"), 0o644)
	p, _ = e.svc.CreateProject(ctx, CreateRequest{Input: d})
	if p.Config.App.Source.Entry != "index.html" {
		t.Errorf("entry = %s", p.Config.App.Source.Entry)
	}

	f := filepath.Join(d, "page.html")
	_ = os.WriteFile(f, []byte("x"), 0o644)
	p, err = e.svc.CreateProject(ctx, CreateRequest{Input: f})
	if err != nil || p.Config.App.Source.Type != config.SourceFile || p.Config.App.Name != "page" {
		t.Fatalf("file: %v %+v", err, p.Config.App)
	}
	if _, err := e.svc.CreateProject(ctx, CreateRequest{Input: filepath.Join(d, "nope.html")}); code(err) != CodeInvalidInput {
		t.Errorf("missing file: %v", err)
	}
	if _, err := e.svc.CreateProject(ctx, CreateRequest{Input: "not a domain"}); code(err) != CodeInvalidInput {
		t.Errorf("bad domain: %v", err)
	}
	if _, err := e.svc.CreateProject(ctx, CreateRequest{Input: ""}); code(err) != CodeInvalidInput {
		t.Errorf("empty: %v", err)
	}
}

func TestCreateProjectFromConfigFiles(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d := t.TempDir()

	cfg := config.New("MyApp", config.Source{Type: config.SourceURL, Value: "https://example.org/"}, e.svc.HostTarget())
	cfg.App.Identifier, cfg.App.ProfileID = "com.me.app", "com.me.profile"
	cfg.App.Icon.Source = "default"
	jf := filepath.Join(d, "mypack.json")
	if err := cfg.Save(jf); err != nil {
		t.Fatal(err)
	}
	p, err := e.svc.CreateProject(ctx, CreateRequest{Input: jf, Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Config.App.Identifier != "com.me.app" || p.Config.App.ProfileID != "com.me.profile" {
		t.Errorf("identity not preserved: %+v", p.Config.App)
	}

	bad := filepath.Join(d, "bad.json")
	_ = os.WriteFile(bad, []byte(`{"schemaVersion":1,"app":{"name":"x","bogus":1}}`), 0o644)
	_, err = e.svc.CreateProject(ctx, CreateRequest{Input: bad})
	if code(err) != CodeInvalidConfig || len(ConfigIssues(err)) == 0 {
		t.Errorf("bad config: %v", err)
	}

	pake := filepath.Join(d, "pake.json")
	_ = os.WriteFile(pake, []byte(`{"windows":[{"url":"https://weread.qq.com","width":900}]}`), 0o644)
	p, err = e.svc.CreateProject(ctx, CreateRequest{Input: pake})
	if err != nil || p.Origin != "import" || p.Config.Window.Width != 900 || p.Config.App.Name != "Weread" {
		t.Fatalf("pake: %v %+v", err, p.Config.App)
	}
}

func TestUpdateProjectRules(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.project(t)

	// Plain edit works and bumps the revision.
	c := p.Config.Clone()
	c.Window.Width = 1000
	up, err := e.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: 1, Config: c, Actor: "gui"})
	if err != nil || up.Revision != 2 || up.Config.Window.Width != 1000 {
		t.Fatalf("update: %v %+v", err, up)
	}
	// Stale revision.
	_, err = e.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: 1, Config: c})
	if code(err) != CodeRevisionConflict {
		t.Errorf("want conflict, got %v", err)
	}
	// Identity change needs confirmation.
	c2 := up.Config.Clone()
	c2.App.Identifier = "com.other.id"
	_, err = e.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: 2, Config: c2})
	if code(err) != CodeIdentityChange {
		t.Errorf("want identity error, got %v", err)
	}
	c2.App.Identifier = up.Config.App.Identifier
	c2.App.ProfileID = "com.other.profile"
	if _, err = e.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: 2, Config: c2}); code(err) != CodeIdentityChange {
		t.Errorf("profile change: %v", err)
	}
	up2, err := e.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: 2, Config: c2, ConfirmIdentityChange: true})
	if err != nil || up2.Config.App.ProfileID != "com.other.profile" {
		t.Errorf("confirmed change: %v", err)
	}
	// Empty identity in the request keeps the stored one.
	c3 := up2.Config.Clone()
	c3.App.Identifier, c3.App.ProfileID = "", ""
	up3, err := e.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: 3, Config: c3})
	if err != nil || up3.Config.App.ProfileID != "com.other.profile" {
		t.Errorf("empty identity: %v %+v", err, up3.Config.App)
	}
	// Invalid config carries issues.
	c4 := up3.Config.Clone()
	c4.Window.Zoom = 5000
	_, err = e.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: 4, Config: c4})
	is := ConfigIssues(err)
	if code(err) != CodeInvalidConfig || len(is) == 0 || is[0].Field != "window.zoom" {
		t.Errorf("invalid: %v %+v", err, is)
	}
}

func TestTrashDuplicateExportIcon(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.project(t)
	d, err := e.svc.DuplicateProject(ctx, p.ID, "gui")
	if err != nil || d.Config.App.Identifier == p.Config.App.Identifier || !strings.HasSuffix(d.Config.App.Name, "copy") {
		t.Fatalf("dup: %v %+v", err, d.Config.App)
	}
	if err := e.svc.TrashProject(ctx, d.ID, "gui"); err != nil {
		t.Fatal(err)
	}
	if l, _ := e.svc.ListProjects(ctx); len(l) != 1 {
		t.Errorf("list = %d", len(l))
	}
	if err := e.svc.RestoreProject(ctx, d.ID, "gui"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "x.json")
	if err := e.svc.ExportProject(ctx, p.ID, out); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(out); err != nil {
		t.Errorf("exported file does not parse: %v", err)
	}
	ip, err := e.svc.SetProjectIcon(ctx, p.ID, []byte("img"), "gui")
	if err != nil || string(ip.IconPNG) != "norm:img" {
		t.Errorf("icon: %v %q", err, ip.IconPNG)
	}
}

// ---- builds ----------------------------------------------------------------

func TestBuildSuccess(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.project(t)

	var events []Event
	var mu sync.Mutex
	unsub := e.svc.Subscribe(func(ev Event) { mu.Lock(); events = append(events, ev); mu.Unlock() })
	defer unsub()

	job, err := e.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if job.Status == domain.JobAwaitingApproval || job.ConfigHash == "" || job.Snapshot == nil || job.ProjectRev != 1 {
		t.Fatalf("job = %+v", job)
	}
	done := waitJob(t, e.svc, job.ID, domain.JobSucceeded, domain.JobFailed)
	if done.Status != domain.JobSucceeded {
		t.Fatalf("job failed: %+v", done.Error)
	}
	// Re-read from the DB once the live state is gone.
	time.Sleep(50 * time.Millisecond)
	stored, _ := e.svc.GetJob(ctx, job.ID)
	if len(stored.Artifacts) != 1 || stored.Artifacts[0].ID == "" || stored.Artifacts[0].JobID != job.ID ||
		stored.Artifacts[0].ProjectID != p.ID || stored.Stage != domain.StageCompleted || stored.StartedAt == nil {
		t.Fatalf("stored = %+v", stored)
	}
	pp, _ := e.svc.GetProject(ctx, p.ID)
	if pp.Draft || pp.LastBuild == nil || pp.Revision != 1 {
		t.Errorf("project after build: draft=%v last=%v rev=%d", pp.Draft, pp.LastBuild, pp.Revision)
	}
	lines, _ := e.svc.JobLog(ctx, job.ID)
	if len(lines) < 2 {
		t.Errorf("log = %+v", lines)
	}
	l, _ := e.svc.ListJobs(ctx, p.ID, 5)
	if len(l) != 1 || len(l[0].Artifacts) != 1 {
		t.Errorf("list jobs = %+v", l)
	}

	// Event throttling: 50 progress callbacks must not become 50 events.
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	var jobEvents, logEvents int
	for _, ev := range events {
		switch ev.Kind {
		case EventJobChanged:
			jobEvents++
		case EventJobLog:
			logEvents++
		}
	}
	mu.Unlock()
	if jobEvents > 20 || logEvents == 0 {
		t.Errorf("job events = %d, log events = %d", jobEvents, logEvents)
	}
	if e.eng.reqs[0].OutputDir != filepath.Join(e.dir, "out") || e.eng.reqs[0].AllowDownload {
		t.Errorf("request = %+v", e.eng.reqs[0])
	}
}

func TestBuildFailureKeepsError(t *testing.T) {
	e := newEnv(t)
	e.eng.fail = &domain.Error{Code: "SIGNING_FAILED", Message: "nope"}
	p := e.project(t)
	job, _ := e.svc.StartBuild(context.Background(), BuildRequest{ProjectID: p.ID})
	done := waitJob(t, e.svc, job.ID, domain.JobFailed, domain.JobSucceeded)
	if done.Status != domain.JobFailed || done.Error == nil || done.Error.Code != "SIGNING_FAILED" {
		t.Fatalf("done = %+v", done)
	}
	if pp, _ := e.svc.GetProject(context.Background(), p.ID); !pp.Draft {
		t.Error("failed build cleared draft flag")
	}
}

func TestBuildApprovalFlow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.rt.cached = map[string]bool{} // runtime missing
	p := e.project(t)

	var approvals int32
	e.svc.Subscribe(func(ev Event) {
		if ev.Kind == EventApprovalNeeded {
			atomic.AddInt32(&approvals, 1)
		}
	})
	job, err := e.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, Actor: "mcp:test"})
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != domain.JobAwaitingApproval || job.Error == nil || job.Error.Code != CodeDownloadRequired {
		t.Fatalf("job = %+v", job)
	}
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&approvals) != 1 || e.eng.calls != 0 {
		t.Fatalf("approvals=%d engine calls=%d", approvals, e.eng.calls)
	}
	if err := e.svc.ApproveJob(ctx, job.ID, "gui"); err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, e.svc, job.ID, domain.JobSucceeded, domain.JobFailed)
	if done.Status != domain.JobSucceeded || !e.eng.reqs[0].AllowDownload {
		t.Fatalf("done = %+v allow=%v", done, e.eng.reqs[0].AllowDownload)
	}
	if err := e.svc.ApproveJob(ctx, job.ID, "gui"); code(err) != "INVALID_STATE" {
		t.Errorf("double approve: %v", err)
	}

	// With AllowDownload up front there is no approval step.
	e.rt.cached = map[string]bool{}
	j2, _ := e.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, AllowDownload: true})
	if j2.Status == domain.JobAwaitingApproval {
		t.Error("AllowDownload ignored")
	}
	waitJob(t, e.svc, j2.ID, domain.JobSucceeded)

	// Cancelling a parked job works without a worker.
	e.rt.cached = map[string]bool{}
	j3, _ := e.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID})
	if err := e.svc.CancelBuild(ctx, j3.ID, "gui"); err != nil {
		t.Fatal(err)
	}
	if g, _ := e.svc.GetJob(ctx, j3.ID); g.Status != domain.JobCancelled {
		t.Errorf("status = %s", g.Status)
	}
}

func TestBuildUnsupportedTarget(t *testing.T) {
	e := newEnv(t)
	p := e.project(t)
	_, err := e.svc.StartBuild(context.Background(), BuildRequest{ProjectID: p.ID,
		Targets: []config.Target{{OS: "darwin", Arch: "arm64", Format: "app"}}})
	if code(err) != CodeTargetUnsupp {
		t.Errorf("err = %v", err)
	}
	pf, err := e.svc.Preflight(context.Background(), p.ID, []config.Target{{OS: "darwin", Arch: "arm64", Format: "app"}})
	if err != nil || len(pf.Targets) != 1 || pf.Targets[0].Supported {
		t.Errorf("preflight = %+v %v", pf, err)
	}
}

func TestCancelRunningBuild(t *testing.T) {
	e := newEnv(t)
	e.eng.block = make(chan struct{})
	ctx := context.Background()
	p := e.project(t)
	job, _ := e.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID})
	waitJob(t, e.svc, job.ID, domain.JobRunning)
	if err := e.svc.CancelBuild(ctx, job.ID, "gui"); err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, e.svc, job.ID, domain.JobCancelled, domain.JobFailed)
	if done.Status != domain.JobCancelled || done.Error.Code != CodeCancelled {
		t.Fatalf("done = %+v", done)
	}
	if pp, _ := e.svc.GetProject(ctx, p.ID); !pp.Draft {
		t.Error("cancelled build marked project built")
	}
}

func TestBoundedConcurrency(t *testing.T) {
	e := newEnv(t)
	e.eng.block = make(chan struct{})
	ctx := context.Background()
	p := e.project(t)
	var ids []string
	for i := 0; i < 5; i++ {
		j, err := e.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, j.ID)
	}
	time.Sleep(200 * time.Millisecond)
	if m := atomic.LoadInt32(&e.eng.max); m != 2 {
		t.Fatalf("max concurrent builds = %d, want 2", m)
	}
	queued := 0
	for _, id := range ids {
		if j, _ := e.svc.GetJob(ctx, id); j.Status == domain.JobQueued {
			queued++
		}
	}
	if queued != 3 {
		t.Errorf("queued = %d, want 3", queued)
	}
	close(e.eng.block)
	for _, id := range ids {
		waitJob(t, e.svc, id, domain.JobSucceeded)
	}
	if m := atomic.LoadInt32(&e.eng.max); m != 2 {
		t.Errorf("max = %d after drain", m)
	}
}

func TestRestartRecoversInterruptedJobs(t *testing.T) {
	e := newEnv(t)
	e.eng.block = make(chan struct{})
	ctx := context.Background()
	p := e.project(t)
	job, _ := e.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID})
	waitJob(t, e.svc, job.ID, domain.JobRunning)
	// Wait until the database has the job's last write before the build
	// blocks, its first stage: one saved after the second service has
	// marked the job interrupted would undo that, and a crashed process
	// writes nothing more.
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		if j, _ := e.svc.store.GetJob(ctx, job.ID); j.Stage == domain.StageResolving {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the build never reached its first stage")
		}
	}
	// Simulate a crash: open the same DB from a second service while the first "died".
	e2, err := New(e.opts)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	j, _ := e2.GetJob(ctx, job.ID)
	if j.Status != domain.JobFailed || j.Error.Code != "INTERRUPTED" {
		t.Fatalf("job = %+v", j)
	}
	close(e.eng.block)
}

// ---- installs --------------------------------------------------------------

func buildAndWait(t *testing.T, e *env, p domain.Project) domain.BuildJob {
	t.Helper()
	j, err := e.svc.StartBuild(context.Background(), BuildRequest{ProjectID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	waitJob(t, e.svc, j.ID, domain.JobSucceeded, domain.JobFailed)
	time.Sleep(50 * time.Millisecond)
	done, _ := e.svc.GetJob(context.Background(), j.ID)
	if done.Status != domain.JobSucceeded {
		t.Fatalf("build failed: %+v", done.Error)
	}
	return done
}

func TestInstallUpdateRepairUninstall(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.project(t)
	job := buildAndWait(t, e, p)
	art := job.Artifacts[0]

	inst, err := e.svc.InstallAndOpen(ctx, InstallRequest{ArtifactID: art.ID, Open: true, Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if inst.State != domain.InstallManaged || inst.ProjectID != p.ID || inst.ArtifactID != art.ID ||
		!strings.HasPrefix(inst.Path, filepath.Join(e.dir, "Applications")) || len(e.ins.launched) != 1 ||
		inst.ProfileDir != filepath.Join(e.dir, "profiles", p.Config.App.ProfileID) {
		t.Fatalf("inst = %+v", inst)
	}
	list, _ := e.svc.ListInstallations(ctx)
	if len(list) != 1 || list[0].UpdateReason != "" {
		t.Fatalf("list = %+v", list)
	}

	// Changing the project makes an update available.
	c := p.Config.Clone()
	c.Window.Width = 999
	if _, err := e.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: 1, Config: c}); err != nil {
		t.Fatal(err)
	}
	list, _ = e.svc.ListInstallations(ctx)
	if list[0].UpdateReason == "" {
		t.Fatal("no update reason after config change")
	}

	// App running => APP_RUNNING with remediation naming the app.
	e.ins.running = true
	_, err = e.svc.UpdateInstallation(ctx, inst.ID, "gui")
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != CodeAppRunning || !strings.HasPrefix(de.Remediation, "Quit ") {
		t.Fatalf("running: %v", err)
	}
	e.ins.running = false

	uj, err := e.svc.UpdateInstallation(ctx, inst.ID, "gui")
	if err != nil {
		t.Fatal(err)
	}
	waitJob(t, e.svc, uj.ID, domain.JobSucceeded, domain.JobFailed)
	time.Sleep(50 * time.Millisecond)
	if g, _ := e.svc.GetJob(ctx, uj.ID); g.Status != domain.JobSucceeded {
		t.Fatalf("update job: %+v", g.Error)
	}
	if len(e.ins.replaced) != 1 || e.ins.replaced[0][0] != inst.Path {
		t.Fatalf("replaced = %+v", e.ins.replaced)
	}
	if _, err := os.Stat(inst.Path + ".bak"); err == nil {
		t.Error("backup kept although the app launched fine")
	}
	list, _ = e.svc.ListInstallations(ctx)
	if list[0].UpdateReason != "" || list[0].ArtifactID == art.ID || list[0].State != domain.InstallManaged {
		t.Fatalf("after update: %+v", list[0])
	}

	// Repair rebuilds from the pinned runtime and snapshot.
	rj, err := e.svc.RepairInstallation(ctx, inst.ID, "gui")
	if err != nil {
		t.Fatal(err)
	}
	waitJob(t, e.svc, rj.ID, domain.JobSucceeded, domain.JobFailed)
	if len(e.ins.replaced) != 2 {
		t.Fatalf("repair replaced = %d", len(e.ins.replaced))
	}

	// Launch failure keeps the backup and warns.
	e.ins.launchErr = errors.New("boom")
	uj2, _ := e.svc.UpdateInstallation(ctx, inst.ID, "gui")
	waitJob(t, e.svc, uj2.ID, domain.JobSucceeded, domain.JobFailed)
	time.Sleep(50 * time.Millisecond)
	g, _ := e.svc.GetJob(ctx, uj2.ID)
	if g.Status != domain.JobSucceeded || len(g.Warnings) == 0 {
		t.Errorf("launch failure: %+v", g)
	}
	if _, err := os.Stat(inst.Path + ".bak"); err != nil {
		t.Error("backup removed after failed launch")
	}

	// Missing detection.
	_ = os.RemoveAll(inst.Path)
	list, _ = e.svc.ListInstallations(ctx)
	if list[0].State != domain.InstallMissing {
		t.Errorf("state = %s", list[0].State)
	}

	// Uninstall with data.
	if err := e.svc.Uninstall(ctx, inst.ID, true, "gui"); err != nil {
		t.Fatal(err)
	}
	if len(e.ins.cleared) != 1 || e.ins.cleared[0] != p.Config.App.ProfileID {
		t.Errorf("cleared = %v", e.ins.cleared)
	}
	if l, _ := e.svc.ListInstallations(ctx); len(l) != 0 {
		t.Errorf("still listed: %+v", l)
	}
}

func TestInstallConflictAndExternal(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.project(t)
	art := buildAndWait(t, e, p).Artifacts[0]

	// A foreign app already occupies the destination.
	dst := filepath.Join(e.ins.DefaultInstallDir(), filepath.Base(art.Path))
	_ = os.MkdirAll(dst, 0o755)
	_, err := e.svc.InstallAndOpen(ctx, InstallRequest{ArtifactID: art.ID})
	if code(err) != "INSTALL_CONFLICT" {
		t.Fatalf("want conflict, got %v", err)
	}
	if _, err := e.svc.InstallAndOpen(ctx, InstallRequest{ArtifactID: art.ID, Replace: true}); err != nil {
		t.Fatal(err)
	}

	ext, err := e.svc.ImportExistingApp(ctx, t.TempDir(), "gui")
	if err != nil || ext.State != domain.InstallExternal || ext.Name != "Ext" {
		t.Fatalf("import: %v %+v", err, ext)
	}
	if _, err := e.svc.UpdateInstallation(ctx, ext.ID, "gui"); code(err) != CodeNotManaged {
		t.Errorf("update external: %v", err)
	}
	if err := e.svc.Uninstall(ctx, ext.ID, false, "gui"); code(err) != CodeNotManaged {
		t.Errorf("uninstall external: %v", err)
	}
}

// ---- runtimes, settings, storage, events ------------------------------------

func TestRuntimesAndStorage(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.rt.cached = map[string]bool{}
	var prog int32
	info, err := e.svc.DownloadRuntime(ctx, config.DefaultRuntimeVersion, "linux-amd64", func(a, b int64) { atomic.AddInt32(&prog, 1) })
	if err != nil || !info.Cached || prog != 1 {
		t.Fatalf("download: %v %+v %d", err, info, prog)
	}
	if err := e.svc.DeleteRuntime(ctx, config.DefaultRuntimeVersion, "linux-amd64"); err != nil {
		t.Fatal(err)
	}
	if l, err := e.svc.ListRuntimes(ctx); err != nil || len(l) != 1 {
		t.Fatal(err)
	}
	e.rt.cached[config.DefaultRuntimeVersion+"/linux-amd64"] = true
	p := e.project(t)
	buildAndWait(t, e, p)
	_ = os.MkdirAll(e.ins.ProfileDir(p.Config.App.ProfileID), 0o755)
	_ = os.WriteFile(filepath.Join(e.ins.ProfileDir(p.Config.App.ProfileID), "f"), []byte("123"), 0o644)
	st, err := e.svc.StorageBreakdown(ctx)
	if err != nil || st.RuntimeCache != 7 || st.Artifacts != 5 || st.Profiles != 3 {
		t.Fatalf("storage = %+v %v", st, err)
	}
}

func TestSettings(t *testing.T) {
	t.Setenv("MYPACK_REGISTRY", "")
	e := newEnv(t, func(o *Options) { o.OutputDir = "" })
	ctx := context.Background()
	s, err := e.svc.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if s.Language != "system" || s.Appearance != "system" || s.LibraryView != "grid" || !s.ShowMenuBarIcon ||
		s.OutputDir != filepath.Join(home, "mypack", "Builds") || s.InstallDir != e.ins.DefaultInstallDir() ||
		s.RegistryURL != DefaultRegistryURL {
		t.Errorf("defaults = %+v", s)
	}
	s.Appearance, s.ShowMenuBarIcon, s.RegistryURL = "dark", false, "file:///tmp/registry.json"
	if err := e.svc.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	g, _ := e.svc.Settings(ctx)
	if g.Appearance != "dark" || g.ShowMenuBarIcon || e.rt.registry != "file:///tmp/registry.json" {
		t.Errorf("saved = %+v registry=%q", g, e.rt.registry)
	}
	s.Appearance = "purple"
	if err := e.svc.SaveSettings(ctx, s); code(err) != CodeInvalidInput {
		t.Errorf("bad appearance: %v", err)
	}
	s.Appearance, s.RegistryURL = "dark", "ftp://x"
	if err := e.svc.SaveSettings(ctx, s); code(err) != CodeInvalidInput {
		t.Errorf("bad registry: %v", err)
	}
}

func TestEventBusDropsOldest(t *testing.T) {
	b := newBus()
	defer b.close()
	block := make(chan struct{})
	var got []string
	var mu sync.Mutex
	b.subscribe(func(e Event) {
		<-block
		mu.Lock()
		got = append(got, e.JobID)
		mu.Unlock()
	})
	for i := 0; i < subscriberBuffer*3; i++ { // must never block
		b.publish(Event{Kind: EventJobChanged, JobID: string(rune('a' + i%26))})
	}
	b.publish(Event{Kind: EventJobChanged, JobID: "LAST"})
	close(block)
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 || len(got) > subscriberBuffer+1 || got[len(got)-1] != "LAST" {
		t.Fatalf("got %d events, last %q", len(got), got[len(got)-1])
	}
}

func TestUnsubscribe(t *testing.T) {
	b := newBus()
	var n int32
	un := b.subscribe(func(Event) { atomic.AddInt32(&n, 1) })
	b.publish(Event{})
	time.Sleep(30 * time.Millisecond)
	un()
	un() // idempotent
	b.publish(Event{})
	time.Sleep(30 * time.Millisecond)
	if atomic.LoadInt32(&n) != 1 {
		t.Fatalf("n = %d", n)
	}
}

func TestWatchExternal(t *testing.T) {
	// The open GUI starts this poll itself (app.watchExternalChanges).
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var seen int32
	e.svc.Subscribe(func(ev Event) {
		if ev.Kind == EventProjectChanged {
			atomic.AddInt32(&seen, 1)
		}
	})
	go e.svc.WatchExternal(ctx, 20*time.Millisecond)

	// Own writes are not "external".
	e.project(t)
	time.Sleep(150 * time.Millisecond)
	own := atomic.LoadInt32(&seen)

	// A second service (another process) on the same DB writes.
	other, err := New(e.opts)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.CreateProject(context.Background(), CreateRequest{Input: "zzz.com"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&seen) == own && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&seen) == own {
		t.Fatal("external change not signalled")
	}
}

func TestLoadConfigFileResolvesPaths(t *testing.T) {
	d := t.TempDir()
	_ = os.WriteFile(filepath.Join(d, "mypack.json"), []byte(`{"schemaVersion":1,"app":{"name":"L","identifier":"com.a.b","profileId":"com.a.b",
	"version":"1.0.0","source":{"type":"dir","value":"site"},"icon":{"source":"file","path":"i.png"}},"runtime":{"version":"1.0.0"},
	"window":{"width":800,"height":600},"webview":{"navigation":{}},"build":{"targets":[]}}`), 0o644)
	c, _, err := LoadConfigFile(filepath.Join(d, "mypack.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.App.Source.Value != filepath.Join(d, "site") || c.App.Icon.Path != filepath.Join(d, "i.png") {
		t.Errorf("paths = %q %q", c.App.Source.Value, c.App.Icon.Path)
	}
}

func TestOpenArtifactLaunchesInPlace(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.project(t)
	job, err := e.svc.StartBuild(ctx, BuildRequest{ProjectID: p.ID, Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, e.svc, job.ID, domain.JobSucceeded, domain.JobFailed)
	if done.Status != domain.JobSucceeded || len(done.Artifacts) == 0 {
		t.Fatalf("build: %+v", done)
	}
	art := done.Artifacts[0]
	_ = os.MkdirAll(art.Path, 0o755)
	if err := e.svc.OpenArtifact(ctx, art.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(e.ins.launched); n == 0 || e.ins.launched[n-1] != art.Path {
		t.Fatalf("launched %v, want %s", e.ins.launched, art.Path)
	}
	if err := e.svc.OpenArtifact(ctx, "art_missing"); err == nil {
		t.Fatal("opening an unknown artifact succeeded")
	}
	_ = os.RemoveAll(art.Path)
	if err := e.svc.OpenArtifact(ctx, art.ID); err == nil {
		t.Fatal("opening a removed artifact succeeded")
	}
}

func TestFetchIconFrom(t *testing.T) {
	var got []string
	e := newEnv(t, func(o *Options) {
		o.IconFrom = func(ctx context.Context, u string, p metadata.IconProvider) ([]byte, error) {
			got = append(got, string(p)+" "+u)
			if p == metadata.IconGoogle {
				return []byte("PNG"), nil
			}
			return nil, metadata.ErrNoProviderIcon
		}
	})
	ctx := context.Background()
	if b, err := e.svc.FetchIconFrom(ctx, "acme.example", metadata.IconGoogle); err != nil || string(b) != "PNG" {
		t.Fatalf("google: %q %v", b, err)
	}
	var de *domain.Error
	if _, err := e.svc.FetchIconFrom(ctx, "acme.example", metadata.IconDuckDuckGo); !errors.As(err, &de) || de.Code != CodeIconNotFound {
		t.Fatalf("duckduckgo: %v", err)
	}
	if _, err := e.svc.FetchIconFrom(ctx, "", metadata.IconGoogle); !errors.As(err, &de) || de.Code != CodeInvalidInput {
		t.Fatalf("empty: %v", err)
	}
	if len(got) != 2 || got[0] != "google https://acme.example/" {
		t.Fatalf("calls %q", got)
	}
}
