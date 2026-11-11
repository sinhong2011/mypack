package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// fakeSvc implements only what the MCP tools touch; any other method panics
// (nil embedded interface), proving the server stays inside its tool surface.
type fakeSvc struct {
	appservice.Service
	mu       sync.Mutex
	proj     domain.Project
	activity []domain.Activity
	started  []appservice.BuildRequest
	updates  []appservice.UpdateRequest
	created  []appservice.CreateRequest
	status   domain.JobStatus
	// projectBuild makes awaiting-approval jobs a project-build approval.
	projectBuild bool
}

func newFake() *fakeSvc {
	host := config.Target{OS: "linux", Arch: "amd64", Format: "portable"}
	c := config.New("Demo", config.Source{Type: config.SourceURL, Value: "https://example.com/"}, host)
	return &fakeSvc{
		proj:   domain.Project{ID: "prj_aaaaaaaa", Revision: 3, Config: c, Draft: true, Origin: "gui"},
		status: domain.JobQueued,
	}
}

func (f *fakeSvc) HostTarget() config.Target {
	return config.Target{OS: "linux", Arch: "amd64", Format: "portable"}
}
func (f *fakeSvc) ListProjects(context.Context) ([]domain.Project, error) {
	return []domain.Project{f.proj}, nil
}
func (f *fakeSvc) GetProject(_ context.Context, id string) (domain.Project, error) {
	if id != f.proj.ID {
		return domain.Project{}, &domain.Error{Code: "NOT_FOUND", Message: "project not found"}
	}
	return f.proj, nil
}
func (f *fakeSvc) CreateProject(_ context.Context, r appservice.CreateRequest) (domain.Project, error) {
	f.mu.Lock()
	f.created = append(f.created, r)
	f.mu.Unlock()
	return f.proj, nil
}
func (f *fakeSvc) UpdateProject(_ context.Context, r appservice.UpdateRequest) (domain.Project, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, r)
	p := f.proj
	p.Config = r.Config
	p.Revision++
	f.proj = p
	return p, nil
}
func (f *fakeSvc) StartBuild(_ context.Context, r appservice.BuildRequest) (domain.BuildJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, r)
	j := domain.BuildJob{ID: "job_bbbbbbbb", ProjectID: r.ProjectID, Status: f.status, Snapshot: f.proj.Config}
	if f.status == domain.JobAwaitingApproval {
		j.Error = &domain.Error{Code: "RUNTIME_DOWNLOAD_REQUIRED", Message: "Needs runtime 1.0.0 (12 MB)."}
		if f.projectBuild {
			j.Error = &domain.Error{Code: "PROJECT_BUILD_REQUIRED", Message: "needs build"}
			j.Approval = &domain.ApprovalRequest{Kind: domain.ApprovalProjectBuild, Command: "npm run build", Path: "/p/app"}
		}
	}
	return j, nil
}
func (f *fakeSvc) GetJob(context.Context, string) (domain.BuildJob, error) {
	return domain.BuildJob{ID: "job_bbbbbbbb", Status: domain.JobSucceeded, Snapshot: f.proj.Config,
		Artifacts: []domain.Artifact{{ID: "art_1", Path: "/out/Demo"}}}, nil
}
func (f *fakeSvc) JobLog(context.Context, string) ([]domain.LogLine, error) {
	var l []domain.LogLine
	for i := 0; i < 30; i++ {
		l = append(l, domain.LogLine{Level: "info", Message: strings.Repeat("x", i%3+1)})
	}
	return l, nil
}
func (f *fakeSvc) CancelBuild(context.Context, string, string) error { return nil }
func (f *fakeSvc) ListJobs(context.Context, string, int) ([]domain.BuildJob, error) {
	return []domain.BuildJob{{Artifacts: []domain.Artifact{{ID: "art_2"}}}, {Artifacts: []domain.Artifact{{ID: "art_1"}}}}, nil
}
func (f *fakeSvc) ListRuntimes(context.Context) ([]domain.RuntimeInfo, error) {
	return []domain.RuntimeInfo{{Version: "1.0.0", Key: "linux-amd64", Path: "/secret/path"}}, nil
}
func (f *fakeSvc) Preflight(context.Context, string, []config.Target) (appservice.Preflight, error) {
	return appservice.Preflight{DownloadBytes: 5}, nil
}
func (f *fakeSvc) RecordActivity(_ context.Context, a domain.Activity) error {
	f.mu.Lock()
	f.activity = append(f.activity, a)
	f.mu.Unlock()
	return nil
}

// client drives a Server over in-memory pipes.
type client struct {
	t   *testing.T
	in  *io.PipeWriter
	out *bufio.Reader
	id  int
}

func startServer(t *testing.T, svc appservice.Service) *client {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := NewServer(svc, ServerInfo{Name: "mypack", Version: "test"})
	done := make(chan struct{})
	go func() {
		_ = srv.Serve(context.Background(), inR, outW)
		outW.Close()
		close(done)
	}()
	t.Cleanup(func() {
		inW.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("server did not stop on EOF")
		}
	})
	return &client{t: t, in: inW, out: bufio.NewReader(outR)}
}

func (c *client) send(v any) {
	b, _ := json.Marshal(v)
	if _, err := c.in.Write(append(b, '\n')); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) recv() map[string]any {
	c.t.Helper()
	ch := make(chan string, 1)
	go func() { l, _ := c.out.ReadString('\n'); ch <- l }()
	select {
	case l := <-ch:
		if !strings.HasSuffix(l, "\n") || strings.Count(l, "\n") != 1 {
			c.t.Fatalf("not newline-delimited: %q", l)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			c.t.Fatalf("bad JSON %q: %v", l, err)
		}
		return m
	case <-time.After(3 * time.Second):
		c.t.Fatal("timeout waiting for response")
	}
	return nil
}

func (c *client) call(method string, params any) map[string]any {
	c.t.Helper()
	c.id++
	c.send(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	m := c.recv()
	if int(m["id"].(float64)) != c.id {
		c.t.Fatalf("id mismatch: %v", m)
	}
	return m
}

func (c *client) tool(name string, args any) (result map[string]any) {
	c.t.Helper()
	m := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if m["error"] != nil {
		c.t.Fatalf("protocol error: %v", m["error"])
	}
	return m["result"].(map[string]any)
}

func structured(r map[string]any) map[string]any { return r["structuredContent"].(map[string]any) }

func (c *client) init(proto, name string) map[string]any {
	m := c.call("initialize", map[string]any{"protocolVersion": proto, "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": name, "version": "1"}})
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return m["result"].(map[string]any)
}

func TestInitializeNegotiation(t *testing.T) {
	for proto, want := range map[string]string{
		"2025-06-18": "2025-06-18", "2025-03-26": "2025-03-26", "2024-11-05": "2024-11-05", "1999-01-01": "2025-06-18",
	} {
		c := startServer(t, newFake())
		r := c.init(proto, "x")
		if r["protocolVersion"] != want {
			t.Errorf("client %s -> %v, want %s", proto, r["protocolVersion"], want)
		}
		caps := r["capabilities"].(map[string]any)
		if _, ok := caps["tools"]; !ok {
			t.Errorf("no tools capability: %v", caps)
		}
		if r["serverInfo"].(map[string]any)["name"] != "mypack" {
			t.Errorf("serverInfo = %v", r["serverInfo"])
		}
	}
}

func TestPingAndErrors(t *testing.T) {
	c := startServer(t, newFake())
	c.init("2025-06-18", "x")
	if m := c.call("ping", nil); m["error"] != nil {
		t.Errorf("ping: %v", m)
	}
	if m := c.call("nope/method", nil); int(m["error"].(map[string]any)["code"].(float64)) != -32601 {
		t.Errorf("unknown method: %v", m)
	}
	if m := c.call("tools/call", map[string]any{"name": "rm_rf"}); int(m["error"].(map[string]any)["code"].(float64)) != -32602 {
		t.Errorf("unknown tool: %v", m)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/unknown"}) // silently ignored
	if _, err := c.in.Write([]byte("{not json\n")); err != nil {
		t.Fatal(err)
	}
	m := c.recv()
	if int(m["error"].(map[string]any)["code"].(float64)) != -32700 || m["id"] != nil {
		t.Errorf("parse error: %v", m)
	}
	if m := c.call("ping", nil); m["error"] != nil { // still alive
		t.Errorf("after parse error: %v", m)
	}
}

func TestToolsList(t *testing.T) {
	c := startServer(t, newFake())
	c.init("2025-06-18", "x")
	m := c.call("tools/list", nil)
	tools := m["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, x := range tools {
		tl := x.(map[string]any)
		names[tl["name"].(string)] = true
		sch := tl["inputSchema"].(map[string]any)
		if sch["type"] != "object" || tl["description"] == "" {
			t.Errorf("bad tool %v", tl["name"])
		}
	}
	for _, n := range []string{"project_list", "project_get", "project_create", "project_update", "project_validate",
		"runtime_status", "build_preflight", "build_start", "build_status", "build_cancel", "artifact_list"} {
		if !names[n] {
			t.Errorf("missing tool %s", n)
		}
	}
	for n := range names {
		if strings.Contains(n, "shell") || strings.Contains(n, "file") || strings.Contains(n, "exec") {
			t.Errorf("forbidden tool %s", n)
		}
	}
}

func TestProjectToolsAndAudit(t *testing.T) {
	f := newFake()
	c := startServer(t, f)
	c.init("2025-06-18", "claude-code")

	r := c.tool("project_list", map[string]any{})
	ps := structured(r)["projects"].([]any)
	if len(ps) != 1 || ps[0].(map[string]any)["id"] != "prj_aaaaaaaa" {
		t.Errorf("list = %v", ps)
	}
	// content mirrors structuredContent as JSON text.
	txt := r["content"].([]any)[0].(map[string]any)
	var back map[string]any
	if txt["type"] != "text" || json.Unmarshal([]byte(txt["text"].(string)), &back) != nil || back["projects"] == nil {
		t.Errorf("content = %v", txt)
	}

	r = c.tool("project_get", map[string]any{"id": "prj_aaaaaaaa"})
	if p := structured(r)["project"].(map[string]any); p["revision"].(float64) != 3 {
		t.Errorf("get = %v", p)
	}
	r = c.tool("project_get", map[string]any{"id": "prj_nope"})
	if r["isError"] != true || structured(r)["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Errorf("not found = %v", r)
	}

	// create: URL only, actor propagated.
	r = c.tool("project_create", map[string]any{"url": "github.com", "name": "GH"})
	if r["isError"] == true || f.created[0].Input != "https://github.com/" || f.created[0].Actor != "mcp:claude-code" {
		t.Errorf("create: %v %+v", r, f.created)
	}
	for _, bad := range []string{"/etc/passwd", "file:///etc/passwd", "~/secrets", "javascript:alert(1)"} {
		r = c.tool("project_create", map[string]any{"url": bad})
		if r["isError"] != true {
			t.Errorf("create accepted %q", bad)
		}
	}
	if len(f.created) != 1 {
		t.Errorf("service saw %d creates", len(f.created))
	}

	// Every call audited with the client actor.
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.activity) < 6 {
		t.Fatalf("activity = %+v", f.activity)
	}
	for _, a := range f.activity {
		if a.Actor != "mcp:claude-code" || !strings.HasPrefix(a.Action, "mcp.") {
			t.Errorf("bad activity %+v", a)
		}
	}
	if f.activity[0].Action != "mcp.project_list" || f.activity[0].Result != "ok" {
		t.Errorf("first activity %+v", f.activity[0])
	}
	if f.activity[2].Result != "error: NOT_FOUND" {
		t.Errorf("error activity %+v", f.activity[2])
	}
}

func TestProjectUpdate(t *testing.T) {
	f := newFake()
	c := startServer(t, f)
	c.init("2025-06-18", "agent")

	// dry run
	r := c.tool("project_update", map[string]any{"id": f.proj.ID, "revision": 3, "dryRun": true,
		"patch": map[string]any{"window": map[string]any{"width": 1400}}})
	s := structured(r)
	ch := s["changes"].([]any)
	if r["isError"] == true || s["dryRun"] != true || s["valid"] != true || len(ch) != 1 ||
		ch[0].(map[string]any)["path"] != "window.width" || len(f.updates) != 0 {
		t.Fatalf("dry run: %v", r)
	}
	// dry run reports invalid
	r = c.tool("project_update", map[string]any{"id": f.proj.ID, "revision": 3, "dryRun": true,
		"patch": map[string]any{"window": map[string]any{"zoom": 9000}}})
	if s := structured(r); s["valid"] != false || s["issues"] == nil {
		t.Errorf("invalid dry run: %v", s)
	}
	// real update
	r = c.tool("project_update", map[string]any{"id": f.proj.ID, "revision": 3,
		"patch": map[string]any{"window": map[string]any{"width": 1400, "height": 900}}})
	s = structured(r)
	if r["isError"] == true || len(f.updates) != 1 || f.updates[0].ExpectedRevision != 3 ||
		f.updates[0].ConfirmIdentityChange || f.updates[0].Actor != "mcp:agent" ||
		f.updates[0].Config.Window.Width != 1400 || s["revision"].(float64) != 4 || len(s["changes"].([]any)) != 2 {
		t.Fatalf("update: %v %+v", r, f.updates)
	}
	// stale revision
	r = c.tool("project_update", map[string]any{"id": f.proj.ID, "revision": 3,
		"patch": map[string]any{"window": map[string]any{"width": 1000}}})
	if r["isError"] != true || structured(r)["error"].(map[string]any)["code"] != "REVISION_CONFLICT" {
		t.Errorf("stale: %v", r)
	}
	// protected fields
	for name, patch := range map[string]map[string]any{
		"identifier": {"app": map[string]any{"identifier": "com.evil.app"}},
		"js":         {"webview": map[string]any{"injections": []any{map[string]any{"type": "js", "content": "alert(1)"}}}},
		"certs":      {"webview": map[string]any{"ignoreCertificateErrors": true}},
		"source":     {"app": map[string]any{"source": map[string]any{"type": "file", "value": "/etc/passwd"}}},
		"proxy":      {"network": map[string]any{"proxy": "http://evil:1"}},
	} {
		r = c.tool("project_update", map[string]any{"id": f.proj.ID, "revision": 4, "patch": patch})
		if r["isError"] != true || structured(r)["error"].(map[string]any)["code"] != "APPROVAL_REQUIRED" {
			t.Errorf("%s not blocked: %v", name, r)
		}
	}
	// unknown field
	r = c.tool("project_update", map[string]any{"id": f.proj.ID, "revision": 4, "patch": map[string]any{"bogus": 1}})
	if r["isError"] != true {
		t.Errorf("unknown field accepted")
	}
	if len(f.updates) != 1 {
		t.Errorf("blocked updates reached the service: %d", len(f.updates))
	}
}

func TestProjectValidate(t *testing.T) {
	c := startServer(t, newFake())
	c.init("2025-06-18", "x")
	s := structured(c.tool("project_validate", map[string]any{"id": "prj_aaaaaaaa"}))
	if s["valid"] != true {
		t.Errorf("stored: %v", s)
	}
	s = structured(c.tool("project_validate", map[string]any{"config": map[string]any{
		"schemaVersion": 1, "app": map[string]any{"name": "", "source": map[string]any{"type": "url", "value": "https://a.com"}}}}))
	if s["valid"] != false || len(s["issues"].([]any)) == 0 {
		t.Errorf("config: %v", s)
	}
	if r := c.tool("project_validate", map[string]any{}); r["isError"] != true {
		t.Errorf("needs id or config")
	}
}

func TestBuildTools(t *testing.T) {
	f := newFake()
	c := startServer(t, f)
	c.init("2025-06-18", "agent")

	s := structured(c.tool("runtime_status", map[string]any{}))
	rt := s["runtimes"].([]any)[0].(map[string]any)
	if rt["path"] != nil {
		t.Errorf("runtime path leaked: %v", rt)
	}
	s = structured(c.tool("build_preflight", map[string]any{"projectId": f.proj.ID}))
	if s["needsApproval"] != true {
		t.Errorf("preflight = %v", s)
	}

	// Normal start: downloads never allowed from MCP, even if the model asks.
	r := c.tool("build_start", map[string]any{"projectId": f.proj.ID, "targets": []any{map[string]any{"os": "linux", "arch": "amd64"}}})
	s = structured(r)
	if r["isError"] == true || s["jobId"] != "job_bbbbbbbb" || s["status"] != "queued" {
		t.Fatalf("start = %v", r)
	}
	if req := f.started[0]; req.AllowDownload || req.Actor != "mcp:agent" || req.Targets[0].Format != "portable" {
		t.Errorf("request = %+v", req)
	}
	if _, has := s["job"].(map[string]any)["snapshot"]; has {
		t.Error("snapshot leaked into job view")
	}
	if r := c.tool("build_start", map[string]any{"projectId": f.proj.ID, "allowRuntimeDownload": true}); r["isError"] != true {
		t.Error("unknown argument accepted")
	}

	// Approval required is a normal (non-error) structured result.
	f.status = domain.JobAwaitingApproval
	r = c.tool("build_start", map[string]any{"projectId": f.proj.ID})
	s = structured(r)
	if r["isError"] == true || s["status"] != "approval_required" || !strings.HasPrefix(s["message"].(string), "approval_required:") {
		t.Errorf("approval = %v", r)
	}

	s = structured(c.tool("build_status", map[string]any{"jobId": "job_bbbbbbbb", "logTail": 5}))
	if len(s["log"].([]any)) != 5 || s["status"] != "completed" {
		t.Errorf("status = %v", s)
	}
	s = structured(c.tool("build_cancel", map[string]any{"jobId": "job_bbbbbbbb"}))
	if s["cancelRequested"] != true {
		t.Errorf("cancel = %v", s)
	}
	s = structured(c.tool("artifact_list", map[string]any{"projectId": f.proj.ID}))
	if a := s["artifacts"].([]any); len(a) != 2 || a[0].(map[string]any)["id"] != "art_2" {
		t.Errorf("artifacts = %v", a)
	}
	if r := c.tool("build_status", map[string]any{}); r["isError"] != true {
		t.Error("missing jobId accepted")
	}
}

func TestConcurrentRequestsAndCancellation(t *testing.T) {
	c := startServer(t, newFake())
	c.init("2025-06-18", "x")
	for i := 1; i <= 20; i++ {
		c.send(map[string]any{"jsonrpc": "2.0", "id": 1000 + i, "method": "ping"})
	}
	seen := map[int]bool{}
	for i := 0; i < 20; i++ {
		seen[int(c.recv()["id"].(float64))] = true
	}
	if len(seen) != 20 {
		t.Errorf("got %d distinct responses", len(seen))
	}
}

func TestBuildStartProjectBuildNeedsHumanApproval(t *testing.T) {
	f := newFake()
	f.status, f.projectBuild = domain.JobAwaitingApproval, true
	c := startServer(t, f)
	c.init("2025-06-18", "agent")
	r := c.tool("build_start", map[string]any{"projectId": f.proj.ID})
	s := structured(r)
	msg, _ := s["message"].(string)
	if r["isError"] == true || s["status"] != "approval_required" || !strings.Contains(msg, "npm run build") || !strings.Contains(msg, "/p/app") {
		t.Fatalf("start = %v", r)
	}
	if f.started[0].AllowProjectBuild {
		t.Error("MCP asked for project build approval")
	}
	// An agent cannot rewrite the build recipe through project_update either.
	r = c.tool("project_update", map[string]any{"id": f.proj.ID, "revision": 3,
		"patch": map[string]any{"app": map[string]any{"source": map[string]any{"build": map[string]any{"script": "evil"}}}}})
	if r["isError"] != true {
		t.Errorf("project_update accepted a build change: %v", r)
	}
}
