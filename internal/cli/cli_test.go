package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// fakeSvc implements the subset of Service the CLI uses.
type fakeSvc struct {
	appservice.Service
	mu       sync.Mutex
	projects []domain.Project
	jobs     map[string]domain.BuildJob
	nextJob  domain.BuildJob // job template returned by StartBuild
	builds   []appservice.BuildRequest
	updates  []appservice.UpdateRequest
	settings appservice.Settings
	runtimes []domain.RuntimeInfo
	deleted  []string
}

var host = config.Target{OS: "linux", Arch: "amd64", Format: "portable"}

func newFake() *fakeSvc {
	return &fakeSvc{jobs: map[string]domain.BuildJob{}, settings: appservice.Settings{Language: "system", LibraryView: "grid", ShowMenuBarIcon: true}}
}

func (f *fakeSvc) HostTarget() config.Target               { return host }
func (f *fakeSvc) Close() error                            { return nil }
func (f *fakeSvc) Subscribe(func(appservice.Event)) func() { return func() {} }
func (f *fakeSvc) ListProjects(context.Context) ([]domain.Project, error) {
	return f.projects, nil
}
func (f *fakeSvc) GetProject(_ context.Context, id string) (domain.Project, error) {
	for _, p := range f.projects {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Project{}, &domain.Error{Code: "NOT_FOUND", Message: "project " + id + " not found"}
}
func (f *fakeSvc) CreateProject(_ context.Context, r appservice.CreateRequest) (domain.Project, error) {
	var c *config.Config
	if strings.HasSuffix(r.Input, ".json") {
		var err error
		c, _, err = appservice.LoadConfigFile(r.Input)
		if err != nil {
			return domain.Project{}, err
		}
		c.ApplyDefaults(host)
	} else {
		name := r.Name
		if name == "" {
			name = "Site"
		}
		c = config.New(name, config.Source{Type: config.SourceURL, Value: "https://" + r.Input + "/"}, host)
	}
	p := domain.Project{ID: "prj_" + strings.Repeat("a", 8), Revision: 1, Config: c, Draft: true, Origin: "cli"}
	f.projects = append(f.projects, p)
	return p, nil
}
func (f *fakeSvc) UpdateProject(_ context.Context, r appservice.UpdateRequest) (domain.Project, error) {
	f.updates = append(f.updates, r)
	p := f.projects[0]
	p.Config, p.Revision = r.Config, p.Revision+1
	f.projects[0] = p
	return p, nil
}
func (f *fakeSvc) ExportProject(_ context.Context, id, path string) error {
	return os.WriteFile(path, []byte("{}"), 0o644)
}
func (f *fakeSvc) StartBuild(_ context.Context, r appservice.BuildRequest) (domain.BuildJob, error) {
	f.builds = append(f.builds, r)
	j := f.nextJob
	j.ID, j.ProjectID = "job_cccccccc", r.ProjectID
	f.jobs[j.ID] = j
	return j, nil
}
func (f *fakeSvc) GetJob(_ context.Context, id string) (domain.BuildJob, error) {
	return f.jobs[id], nil
}
func (f *fakeSvc) CancelBuild(context.Context, string, string) error     { return nil }
func (f *fakeSvc) Settings(context.Context) (appservice.Settings, error) { return f.settings, nil }
func (f *fakeSvc) SaveSettings(_ context.Context, s appservice.Settings) error {
	f.settings = s
	return nil
}
func (f *fakeSvc) ListRuntimes(context.Context) ([]domain.RuntimeInfo, error) { return f.runtimes, nil }
func (f *fakeSvc) DeleteRuntime(_ context.Context, v, k string) error {
	f.deleted = append(f.deleted, v+"/"+k)
	return nil
}
func (f *fakeSvc) DownloadRuntime(_ context.Context, v, k string, p func(int64, int64)) (domain.RuntimeInfo, error) {
	p(50, 100)
	return domain.RuntimeInfo{Version: v, Key: k, Cached: true, Verified: true}, nil
}
func (f *fakeSvc) VerifyRuntime(_ context.Context, v, k string) error {
	if k == "bad-key" {
		return &domain.Error{Code: "RUNTIME_VERIFY_FAILED", Message: "hash mismatch"}
	}
	return nil
}
func (f *fakeSvc) InstallAndOpen(_ context.Context, r appservice.InstallRequest) (domain.Installation, error) {
	return domain.Installation{ID: "ins_1", Name: "Site", Path: "/Apps/Site.app"}, nil
}
func (f *fakeSvc) ListInstallations(context.Context) ([]domain.Installation, error) {
	return []domain.Installation{{ID: "ins_1", Name: "Site", State: domain.InstallManaged}}, nil
}

func run(t *testing.T, f appservice.Service, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = RunWith(Deps{NewService: func() (appservice.Service, error) { return f, nil }}, args, &out, &errb)
	return code, out.String(), errb.String()
}

// oneJSON asserts stdout is exactly one JSON object and returns it.
func oneJSON(t *testing.T, stdout string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("stdout is not JSON: %q (%v)", stdout, err)
	}
	if dec.More() || strings.Count(strings.TrimSpace(stdout), "\n") != 0 {
		t.Fatalf("stdout has more than one JSON object: %q", stdout)
	}
	return m
}

func okJob(status domain.JobStatus) domain.BuildJob {
	c := config.New("Site", config.Source{Type: config.SourceURL, Value: "https://site.com/"}, host)
	return domain.BuildJob{Status: status, Snapshot: c, Targets: []config.Target{{OS: "darwin", Arch: "arm64", Format: "app"}}}
}

func TestVersionAndUsage(t *testing.T) {
	code, out, _ := run(t, newFake(), "version", "--json")
	if m := oneJSON(t, out); code != 0 || m["ok"] != true || m["version"] != Version {
		t.Errorf("version: %d %v", code, m)
	}
	if code, out, _ := run(t, newFake(), "version"); code != 0 || !strings.HasPrefix(out, "mypack ") {
		t.Errorf("human version: %d %q", code, out)
	}
	if code, _, errs := run(t, newFake()); code != ExitUsage || !strings.Contains(errs, "Usage") {
		t.Errorf("no args: %d", code)
	}
	if code, _, _ := run(t, newFake(), "frobnicate"); code != ExitUsage {
		t.Errorf("unknown command: %d", code)
	}
	if code, out, _ := run(t, newFake(), "help"); code != 0 || !strings.Contains(out, "mypack build") {
		t.Errorf("help: %d", code)
	}
	code, out, _ = run(t, newFake(), "build", "--json")
	if m := oneJSON(t, out); code != ExitUsage || m["ok"] != false || m["error"].(map[string]any)["code"] != "USAGE" {
		t.Errorf("usage json: %d %v", code, m)
	}
	if code, _, _ := run(t, newFake(), "build", "x", "--bogus"); code != ExitUsage {
		t.Errorf("bad flag: %d", code)
	}
}

func TestCreate(t *testing.T) {
	f := newFake()
	code, out, errs := run(t, f, "create", "github.com", "--name", "GitHub", "--json")
	m := oneJSON(t, out)
	p := m["project"].(map[string]any)
	if code != 0 || m["ok"] != true || p["name"] != "GitHub" || p["id"] != "prj_aaaaaaaa" || p["revision"].(float64) != 1 {
		t.Fatalf("create: %d %v", code, m)
	}
	if errs != "" {
		t.Errorf("--json must keep stderr quiet, got %q", errs)
	}
	code, out, errs = run(t, f, "create", "github.com")
	if code != 0 || !strings.Contains(out, "prj_aaaaaaaa") || !strings.Contains(errs, "mypack build") {
		t.Errorf("human create: %d %q %q", code, out, errs)
	}
}

func TestBuildSuccessJSONContract(t *testing.T) {
	f := newFake()
	f.projects = []domain.Project{{ID: "prj_11111111", Revision: 1, Config: okJob("").Snapshot}}
	j := okJob(domain.JobSucceeded)
	j.Artifacts = []domain.Artifact{{ID: "art_1", Target: j.Targets[0], Path: "/out/Site.app", Size: 42, SHA256: "ff", Signature: "adhoc"}}
	j.Warnings = []string{"unsigned"}
	f.nextJob = j

	code, out, errs := run(t, f, "build", "prj_11111111", "--target", "darwin-arm64", "--format", "app", "--output", "/o", "--yes", "--json")
	m := oneJSON(t, out)
	if code != 0 || m["ok"] != true || m["name"] != "Site" || m["platform"] != "darwin" || m["arch"] != "arm64" {
		t.Fatalf("build: %d %v", code, m)
	}
	o := m["outputs"].([]any)[0].(map[string]any)
	if o["path"] != "/out/Site.app" || o["format"] != "app" || o["size"].(float64) != 42 || o["sha256"] != "ff" || o["artifactId"] != "art_1" {
		t.Errorf("outputs = %v", o)
	}
	if w := m["warnings"].([]any); len(w) != 1 {
		t.Errorf("warnings = %v", w)
	}
	if errs != "" {
		t.Errorf("stderr should be quiet in --json mode: %q", errs)
	}
	b := f.builds[0]
	if !b.AllowDownload || b.OutputDir != "/o" || b.Actor != "cli" || len(b.Targets) != 1 ||
		b.Targets[0] != (config.Target{OS: "darwin", Arch: "arm64", Format: "app"}) {
		t.Errorf("request = %+v", b)
	}

	code, out, errs = run(t, f, "build", "prj_11111111")
	if code != 0 || !strings.Contains(out, "/out/Site.app") || !strings.Contains(errs, "mypack install art_1") {
		t.Errorf("human build: %d %q %q", code, out, errs)
	}
}

func TestBuildNeedsApprovalExit6(t *testing.T) {
	f := newFake()
	j := okJob(domain.JobAwaitingApproval)
	j.Error = &domain.Error{Code: "RUNTIME_DOWNLOAD_REQUIRED", Message: "needs runtime", Remediation: "use --yes"}
	f.nextJob = j
	code, out, _ := run(t, f, "build", "prj_11111111", "--json")
	m := oneJSON(t, out)
	e := m["error"].(map[string]any)
	if code != ExitApproval || m["ok"] != false || e["code"] != "RUNTIME_DOWNLOAD_REQUIRED" || e["remediation"] != "use --yes" || m["jobId"] != "job_cccccccc" {
		t.Fatalf("approval: %d %v", code, m)
	}
	if f.builds[0].AllowDownload {
		t.Error("download allowed without --yes")
	}
}

func TestBuildFailureAndCancel(t *testing.T) {
	f := newFake()
	j := okJob(domain.JobFailed)
	j.Error = &domain.Error{Code: "SIGNING_FAILED", Message: "codesign failed", Remediation: "install Xcode tools"}
	f.nextJob = j
	code, out, _ := run(t, f, "build", "prj_1", "--json")
	if m := oneJSON(t, out); code != ExitBuildFailed || m["error"].(map[string]any)["code"] != "SIGNING_FAILED" {
		t.Errorf("failed: %d %v", code, m)
	}
	j.Error = &domain.Error{Code: "RUNTIME_VERIFY_FAILED", Message: "bad hash"}
	f.nextJob = j
	if code, _, _ := run(t, f, "build", "prj_1", "--json"); code != ExitRuntime {
		t.Errorf("runtime failure: %d", code)
	}
	f.nextJob = okJob(domain.JobCancelled)
	if code, _, _ := run(t, f, "build", "prj_1", "--json"); code != ExitCancelled {
		t.Errorf("cancelled: %d", code)
	}
}

func TestBuildFromFileReusesProject(t *testing.T) {
	f := newFake()
	f.nextJob = okJob(domain.JobSucceeded)
	d := t.TempDir()
	cfg := config.New("Site", config.Source{Type: config.SourceURL, Value: "https://site.com/"}, host)
	cfg.App.Identifier, cfg.App.ProfileID = "com.site.app", "com.site.app"
	path := filepath.Join(d, "mypack.json")
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	code, _, _ := run(t, f, "build", path, "--json")
	if code != 0 || len(f.projects) != 1 || f.builds[0].ProjectID != f.projects[0].ID {
		t.Fatalf("first build: %d projects=%d builds=%+v", code, len(f.projects), f.builds)
	}
	// Same file again: no duplicate project, no update.
	f.projects[0].Origin = "cli"
	code, _, _ = run(t, f, "build", path, "--json")
	if code != 0 || len(f.projects) != 1 || len(f.updates) != 0 {
		t.Fatalf("second build: projects=%d updates=%d", len(f.projects), len(f.updates))
	}
	// Edited file: the transient project follows it.
	cfg.Window.Width = 1111
	_ = cfg.Save(path)
	code, _, _ = run(t, f, "build", path, "--json")
	if code != 0 || len(f.updates) != 1 || f.updates[0].Config.Window.Width != 1111 || f.updates[0].Actor != "cli" {
		t.Fatalf("third build: updates=%+v", f.updates)
	}
	// Invalid file => exit 3 with issues, nothing built.
	cfg.Window.Zoom = 9999
	_ = cfg.Save(path)
	n := len(f.builds)
	code, out, _ := run(t, f, "build", path, "--json")
	m := oneJSON(t, out)
	if code != ExitValidation || m["error"].(map[string]any)["code"] != "INVALID_CONFIG" || m["issues"] == nil || len(f.builds) != n {
		t.Errorf("invalid file: %d %v", code, m)
	}
}

func TestProjectCommands(t *testing.T) {
	f := newFake()
	f.projects = []domain.Project{{ID: "prj_11111111", Revision: 2, Draft: true, Origin: "gui",
		Config: config.New("Site", config.Source{Type: config.SourceURL, Value: "https://site.com/"}, host)}}
	code, out, _ := run(t, f, "project", "list", "--json")
	m := oneJSON(t, out)
	if ps := m["projects"].([]any); code != 0 || len(ps) != 1 || ps[0].(map[string]any)["config"] != nil {
		t.Errorf("list: %d %v", code, m)
	}
	code, out, _ = run(t, f, "project", "list")
	if code != 0 || !strings.Contains(out, "prj_11111111") || !strings.Contains(out, "draft") {
		t.Errorf("human list: %q", out)
	}
	code, out, _ = run(t, f, "project", "get", "prj_11111111", "--json")
	if m := oneJSON(t, out); code != 0 || m["project"].(map[string]any)["config"] == nil {
		t.Errorf("get: %v", m)
	}
	code, out, _ = run(t, f, "project", "get", "prj_nope", "--json")
	if m := oneJSON(t, out); code != ExitGeneric || m["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Errorf("get missing: %d %v", code, m)
	}
	// validate a stored project
	if code, out, _ := run(t, f, "project", "validate", "prj_11111111", "--json"); code != 0 || oneJSON(t, out)["valid"] != true {
		t.Errorf("validate id: %d %q", code, out)
	}
	// validate a file
	d := t.TempDir()
	bad := filepath.Join(d, "bad.json")
	_ = os.WriteFile(bad, []byte(`{"schemaVersion":1,"app":{"name":"x"}}`), 0o644)
	code, out, _ = run(t, f, "project", "validate", bad, "--json")
	if m := oneJSON(t, out); code != ExitValidation || m["valid"] != false || m["issues"] == nil {
		t.Errorf("validate bad: %d %v", code, m)
	}
	good := filepath.Join(d, "good.json")
	_ = config.New("G", config.Source{Type: config.SourceURL, Value: "https://g.com/"}, host).Save(good)
	if code, _, _ := run(t, f, "project", "validate", good); code != 0 {
		t.Errorf("validate good: %d", code)
	}
	exp := filepath.Join(d, "out.json")
	if code, _, _ := run(t, f, "project", "export", "prj_11111111", exp); code != 0 {
		t.Errorf("export: %d", code)
	}
	if _, err := os.Stat(exp); err != nil {
		t.Errorf("export file: %v", err)
	}
	if code, _, _ := run(t, f, "project"); code != ExitUsage {
		t.Errorf("project without sub: %d", code)
	}
}

func TestImportPake(t *testing.T) {
	f := newFake()
	d := t.TempDir()
	p := filepath.Join(d, "pake.json")
	_ = os.WriteFile(p, []byte(`{"windows":[{"url":"https://weread.qq.com","resizable":false}]}`), 0o644)
	code, out, _ := run(t, f, "import-pake", p, "--json")
	m := oneJSON(t, out)
	if code != 0 || len(m["warnings"].([]any)) != 1 {
		t.Fatalf("import: %d %v", code, m)
	}
	code, _, errs := run(t, f, "import-pake", p)
	if code != 0 || !strings.Contains(errs, "warning: resizable") {
		t.Errorf("human import: %d %q", code, errs)
	}
	notPake := filepath.Join(d, "x.json")
	_ = os.WriteFile(notPake, []byte(`{"schemaVersion":1,"app":{}}`), 0o644)
	if code, _, _ := run(t, f, "import-pake", notPake, "--json"); code != ExitValidation {
		t.Errorf("non-pake: %d", code)
	}
}

func TestRuntimeCommands(t *testing.T) {
	f := newFake()
	f.runtimes = []domain.RuntimeInfo{
		{Version: config.DefaultRuntimeVersion, Key: "linux-amd64", Cached: true, Verified: true},
		{Version: "0.9.0", Key: "linux-amd64", Cached: true},
		{Version: config.DefaultRuntimeVersion, Key: "darwin-arm64"},
	}
	f.projects = []domain.Project{{ID: "p", Config: func() *config.Config {
		c := config.New("S", config.Source{Type: config.SourceURL, Value: "https://s.com/"}, host)
		return c
	}()}}
	if code, out, _ := run(t, f, "runtime", "list"); code != 0 || !strings.Contains(out, "not downloaded") || !strings.Contains(out, "ready") {
		t.Errorf("list: %q", out)
	}
	code, out, _ := run(t, f, "runtime", "list", "--json")
	if m := oneJSON(t, out); code != 0 || len(m["runtimes"].([]any)) != 3 {
		t.Errorf("list json: %v", m)
	}
	code, out, errs := run(t, f, "runtime", "download", "darwin-arm64", "--version", "1.2.3")
	if code != 0 || !strings.Contains(out, "1.2.3 darwin-arm64 ready") || !strings.Contains(errs, "50%") {
		t.Errorf("download: %d %q %q", code, out, errs)
	}
	code, out, _ = run(t, f, "runtime", "verify", "--json")
	if m := oneJSON(t, out); code != 0 || len(m["results"].([]any)) != 2 {
		t.Errorf("verify: %d %v", code, m)
	}
	code, out, _ = run(t, f, "runtime", "verify", "bad-key", "--json")
	if m := oneJSON(t, out); code != ExitRuntime || m["ok"] != false {
		t.Errorf("verify bad: %d %v", code, m)
	}
	// clean keeps what the project pins (1.0.0/linux-amd64) and removes the rest.
	if code, _, _ := run(t, f, "runtime", "clean"); code != 0 || len(f.deleted) != 1 || f.deleted[0] != "0.9.0/linux-amd64" {
		t.Errorf("clean: %d %v", code, f.deleted)
	}
	f.deleted = nil
	run(t, f, "runtime", "clean", "--all")
	if len(f.deleted) != 2 {
		t.Errorf("clean --all: %v", f.deleted)
	}
}

func TestInstallAppsSettings(t *testing.T) {
	f := newFake()
	code, out, _ := run(t, f, "install", "art_1", "--open", "--json")
	if m := oneJSON(t, out); code != 0 || m["installation"] == nil {
		t.Errorf("install: %d %v", code, m)
	}
	code, out, _ = run(t, f, "apps", "list", "--json")
	if m := oneJSON(t, out); code != 0 || len(m["apps"].([]any)) != 1 {
		t.Errorf("apps: %v", m)
	}
	if code, out, _ := run(t, f, "apps", "list"); code != 0 || !strings.Contains(out, "Site") {
		t.Errorf("human apps: %q", out)
	}
	code, out, _ = run(t, f, "settings", "get", "language")
	if code != 0 || strings.TrimSpace(out) != "system" {
		t.Errorf("get: %q", out)
	}
	if code, _, _ := run(t, f, "settings", "set", "libraryView", "list"); code != 0 || f.settings.LibraryView != "list" {
		t.Errorf("set: %+v", f.settings)
	}
	if code, _, _ := run(t, f, "settings", "set", "showMenuBarIcon", "false"); code != 0 || f.settings.ShowMenuBarIcon {
		t.Errorf("set bool: %+v", f.settings)
	}
	if code, _, _ := run(t, f, "settings", "set", "showMenuBarIcon", "maybe"); code != ExitValidation {
		t.Errorf("bad bool: %d", code)
	}
	if code, _, _ := run(t, f, "settings", "get", "nope", "--json"); code != ExitValidation {
		t.Errorf("unknown key: %d", code)
	}
	code, out, _ = run(t, f, "settings", "get", "--json")
	if m := oneJSON(t, out); code != 0 || m["settings"].(map[string]any)["libraryView"] != "list" {
		t.Errorf("get all: %v", m)
	}
}

func TestMCPServeOverStdio(t *testing.T) {
	f := newFake()
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"t"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n")
	var out, errb bytes.Buffer
	code := RunWith(Deps{Stdin: in, NewService: func() (appservice.Service, error) { return f, nil }},
		[]string{"mcp", "serve", "--transport", "stdio"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 { // stdout carries protocol messages only
		t.Fatalf("stdout = %q", out.String())
	}
	for _, l := range lines {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) != nil || m["jsonrpc"] != "2.0" {
			t.Errorf("non-protocol line on stdout: %q", l)
		}
	}
	if code := RunWith(Deps{NewService: func() (appservice.Service, error) { return f, nil }}, []string{"mcp", "serve", "--transport", "http"}, &out, &errb); code != ExitUsage {
		t.Errorf("http transport: %d", code)
	}
}

func TestExitCodeMapping(t *testing.T) {
	for code, want := range map[string]int{
		"INVALID_CONFIG": 3, "REVISION_CONFLICT": 3, "RUNTIME_DOWNLOAD_REQUIRED": 6, "RUNTIME_NOT_FOUND": 4,
		"RUNTIME_DOWNLOAD_FAILED": 4, "BUILD_CANCELLED": 130, "BUILD_FAILED": 5, "SIGNING_FAILED": 5, "NOT_FOUND": 1,
		"INSTALL_CONFLICT": 1, "TARGET_UNSUPPORTED": 3,
	} {
		if got := exitCodeFor(&domain.Error{Code: code}); got != want {
			t.Errorf("%s -> %d, want %d", code, got, want)
		}
	}
}

func TestServiceOpenFailure(t *testing.T) {
	var out, errb bytes.Buffer
	code := RunWith(Deps{NewService: func() (appservice.Service, error) { return nil, os.ErrPermission }},
		[]string{"project", "list", "--json"}, &out, &errb)
	if m := oneJSON(t, out.String()); code != ExitGeneric || m["ok"] != false {
		t.Errorf("open failure: %d %v", code, m)
	}
}

// licSvc adds license + relocate behaviour to the fake.
type licSvc struct {
	*fakeSvc
	edition string
	located [2]string
}

func (l *licSvc) License(context.Context) (appservice.LicenseStatus, error) {
	if l.edition == "pro" {
		return appservice.LicenseStatus{Edition: "pro", Valid: true, Email: "a@b.c", LicenseID: "lic_1"}, nil
	}
	return appservice.LicenseStatus{Edition: "free", ManagedApps: 1, FreeLimit: 2}, nil
}
func (l *licSvc) ActivateLicense(_ context.Context, tok, _ string) (appservice.LicenseStatus, error) {
	if tok != "good" {
		return appservice.LicenseStatus{}, &domain.Error{Code: "LICENSE_INVALID", Message: "bad", Remediation: "fix"}
	}
	l.edition = "pro"
	return l.License(context.Background())
}
func (l *licSvc) DeactivateLicense(context.Context, string) error { l.edition = ""; return nil }
func (l *licSvc) RelocateInstallation(_ context.Context, id, p, _ string) (domain.Installation, error) {
	if id == "other" {
		return domain.Installation{}, &domain.Error{Code: "NOT_SAME_APP", Message: "no"}
	}
	l.located = [2]string{id, p}
	return domain.Installation{ID: id, Name: "Site", Path: p}, nil
}

func TestLicenseAndLocate(t *testing.T) {
	l := &licSvc{fakeSvc: newFake()}
	code, out, _ := run(t, l, "license", "status", "--json")
	m := oneJSON(t, out)
	if code != 0 || m["license"].(map[string]any)["edition"] != "free" {
		t.Fatalf("status: %d %v", code, m)
	}
	if code, out, _ := run(t, l, "license", "status"); code != 0 || !strings.Contains(out, "1 of 2") {
		t.Errorf("human status: %q", out)
	}
	code, out, _ = run(t, l, "license", "activate", "bad", "--json")
	m = oneJSON(t, out)
	if code != ExitValidation || m["error"].(map[string]any)["code"] != "LICENSE_INVALID" {
		t.Errorf("bad activate: %d %v", code, m)
	}
	code, out, _ = run(t, l, "license", "activate", "good", "--json")
	if m = oneJSON(t, out); code != 0 || m["license"].(map[string]any)["edition"] != "pro" {
		t.Errorf("activate: %d %v", code, m)
	}
	if code, out, _ := run(t, l, "license", "status"); code != 0 || !strings.Contains(out, "Pro") {
		t.Errorf("pro status: %q", out)
	}
	if code, _, _ := run(t, l, "license", "deactivate"); code != 0 || l.edition != "" {
		t.Errorf("deactivate: %d", code)
	}
	if code, _, _ := run(t, l, "license"); code != ExitUsage {
		t.Errorf("usage: %d", code)
	}

	if code, _, _ := run(t, l, "apps", "locate", "inst_1", "/x/Site.app"); code != 0 || l.located != [2]string{"inst_1", "/x/Site.app"} {
		t.Errorf("locate: %v", l.located)
	}
	code, out, _ = run(t, l, "apps", "locate", "other", "/y.app", "--json")
	if m = oneJSON(t, out); code != ExitValidation || m["error"].(map[string]any)["code"] != "NOT_SAME_APP" {
		t.Errorf("locate mismatch: %d %v", code, m)
	}
	if code, _, _ := run(t, l, "apps", "locate", "only-one"); code != ExitUsage {
		t.Errorf("locate usage: %d", code)
	}
}
