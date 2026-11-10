package storage

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

func open(t *testing.T) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sub", "mypack.db")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, p
}

func newProject(t *testing.T, s *Store) domain.Project {
	t.Helper()
	c := config.New("Demo", config.Source{Type: config.SourceURL, Value: "https://example.com/"},
		config.Target{OS: "linux", Arch: "amd64"})
	p, err := s.InsertProject(context.Background(), domain.Project{Config: c, Draft: true, Origin: "cli", IconPNG: []byte{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNewID(t *testing.T) {
	re := regexp.MustCompile(`^prj_[a-z2-7]{8}$`)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id := NewID("prj")
		if !re.MatchString(id) || seen[id] {
			t.Fatalf("bad or duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestProjectRevisionConflict(t *testing.T) {
	ctx := context.Background()
	s, _ := open(t)
	p := newProject(t, s)
	if p.Revision != 1 || p.ID == "" {
		t.Fatalf("project = %+v", p)
	}
	got, _ := s.GetProject(ctx, p.ID)
	if string(got.IconPNG) != "\x01\x02" || got.Config.App.Name != "Demo" || !got.Draft {
		t.Fatalf("roundtrip = %+v", got)
	}
	got.Config.App.Name = "Renamed"
	up, err := s.UpdateProject(ctx, got, 1)
	if err != nil || up.Revision != 2 || up.Config.App.Name != "Renamed" {
		t.Fatalf("update: %v %+v", err, up)
	}
	_, err = s.UpdateProject(ctx, got, 1) // stale
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != "REVISION_CONFLICT" {
		t.Fatalf("want REVISION_CONFLICT, got %v", err)
	}
	got.ID = "prj_missing"
	if _, err := s.UpdateProject(ctx, got, 1); !IsNotFound(err) {
		t.Fatalf("want NOT_FOUND, got %v", err)
	}
}

func TestTrashAndBuilt(t *testing.T) {
	ctx := context.Background()
	s, _ := open(t)
	p := newProject(t, s)
	now := time.Now()
	if err := s.SetTrashed(ctx, p.ID, &now); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.ListProjects(ctx, false); len(l) != 0 {
		t.Fatal("trashed project listed")
	}
	if l, _ := s.ListProjects(ctx, true); len(l) != 1 {
		t.Fatal("trashed project missing from full list")
	}
	_ = s.SetTrashed(ctx, p.ID, nil)
	if err := s.MarkBuilt(ctx, p.ID, now); err != nil {
		t.Fatal(err)
	}
	g, _ := s.GetProject(ctx, p.ID)
	if g.Draft || g.LastBuild == nil || g.Revision != 1 {
		t.Fatalf("after build: %+v", g)
	}
}

func TestJobsLogsArtifacts(t *testing.T) {
	ctx := context.Background()
	s, _ := open(t)
	p := newProject(t, s)
	j, err := s.InsertJob(ctx, domain.BuildJob{
		ProjectID: p.ID, ProjectRev: 1, Snapshot: p.Config, ConfigHash: "h",
		Targets: p.Config.Build.Targets, Status: domain.JobQueued, Stage: domain.StageQueued, Progress: -1, Actor: "cli",
	}, JobMeta{Kind: "build", OutputDir: "/out"})
	if err != nil {
		t.Fatal(err)
	}
	j.Status = domain.JobFailed
	j.Error = &domain.Error{Code: "BUILD_FAILED", Message: "x"}
	j.Warnings = []string{"w"}
	if err := s.SaveJobState(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendLogs(ctx, j.ID, []domain.LogLine{{Time: time.Now(), Level: "info", Stage: domain.StageResolving, Message: "a"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendLogs(ctx, j.ID, []domain.LogLine{{Time: time.Now(), Level: "warn", Message: "b"}, {Time: time.Now(), Level: "error", Message: "c"}}); err != nil {
		t.Fatal(err)
	}
	lines, _ := s.JobLog(ctx, j.ID)
	if len(lines) != 3 || lines[0].Message != "a" || lines[2].Message != "c" {
		t.Fatalf("log = %+v", lines)
	}
	a, err := s.InsertArtifact(ctx, domain.Artifact{JobID: j.ID, ProjectID: p.ID, Target: p.Config.Build.Targets[0], Path: "/out/x", Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetJob(ctx, j.ID)
	if err != nil || got.Status != domain.JobFailed || got.Error.Code != "BUILD_FAILED" ||
		len(got.Warnings) != 1 || len(got.Artifacts) != 1 || got.Artifacts[0].ID != a.ID || got.Snapshot.App.Name != "Demo" {
		t.Fatalf("job = %v %+v", err, got)
	}
	meta, _ := s.GetJobMeta(ctx, j.ID)
	if meta.OutputDir != "/out" {
		t.Fatalf("meta = %+v", meta)
	}
	if l, _ := s.ListJobs(ctx, p.ID, 10); len(l) != 1 || len(l[0].Artifacts) != 1 {
		t.Fatalf("list jobs = %+v", l)
	}
	if _, err := s.GetJob(ctx, "job_nope"); !IsNotFound(err) {
		t.Fatal("want not found")
	}
}

func TestInterruptedJobsRecovered(t *testing.T) {
	ctx := context.Background()
	s, path := open(t)
	p := newProject(t, s)
	mk := func(st domain.JobStatus) string {
		j, err := s.InsertJob(ctx, domain.BuildJob{ProjectID: p.ID, Snapshot: p.Config, Status: st}, JobMeta{})
		if err != nil {
			t.Fatal(err)
		}
		return j.ID
	}
	q, r, done := mk(domain.JobQueued), mk(domain.JobRunning), mk(domain.JobSucceeded)
	wait := mk(domain.JobAwaitingApproval)
	s.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	for _, id := range []string{q, r} {
		j, _ := s2.GetJob(ctx, id)
		if j.Status != domain.JobFailed || j.Error == nil || j.Error.Code != "INTERRUPTED" || j.FinishedAt == nil {
			t.Errorf("%s not recovered: %+v", id, j)
		}
	}
	if j, _ := s2.GetJob(ctx, done); j.Status != domain.JobSucceeded {
		t.Error("completed job touched")
	}
	if j, _ := s2.GetJob(ctx, wait); j.Status != domain.JobAwaitingApproval {
		t.Error("awaiting job touched")
	}
}

func TestInstallationsActivitySettings(t *testing.T) {
	ctx := context.Background()
	s, _ := open(t)
	in, err := s.SaveInstallation(ctx, domain.Installation{Name: "B", Path: "/a/B.app", State: domain.InstallManaged, IconPNG: []byte{9}})
	if err != nil || in.ID == "" {
		t.Fatal(err)
	}
	_, _ = s.SaveInstallation(ctx, domain.Installation{Name: "a", Path: "/a/a.app", State: domain.InstallExternal})
	l, _ := s.ListInstallations(ctx)
	if len(l) != 2 || l[0].Name != "a" {
		t.Fatalf("list = %+v", l)
	}
	in.State = domain.InstallMissing
	_, _ = s.SaveInstallation(ctx, in)
	g, _ := s.GetInstallation(ctx, in.ID)
	if g.State != domain.InstallMissing || len(g.IconPNG) != 1 {
		t.Fatalf("got %+v", g)
	}
	_ = s.DeleteInstallation(ctx, in.ID)
	if _, err := s.GetInstallation(ctx, in.ID); !IsNotFound(err) {
		t.Fatal("not deleted")
	}

	for i := 0; i < 3; i++ {
		_ = s.AddActivity(ctx, domain.Activity{Actor: "cli", Action: "x", Result: "ok"})
	}
	if a, _ := s.ListActivity(ctx, 2); len(a) != 2 {
		t.Fatalf("activity = %+v", a)
	}

	var v struct{ A int }
	if ok, _ := s.GetSetting(ctx, "k", &v); ok {
		t.Fatal("unexpected setting")
	}
	_ = s.SetSetting(ctx, "k", struct{ A int }{3})
	if ok, _ := s.GetSetting(ctx, "k", &v); !ok || v.A != 3 {
		t.Fatalf("setting = %+v", v)
	}
	if err := s.SetSetting(ctx, ChangeSeqKey, 1); err == nil {
		t.Fatal("change_seq must be reserved")
	}
}

func TestExternalChangeDetection(t *testing.T) {
	ctx := context.Background()
	a, path := open(t)
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if ch, _ := a.ExternalChange(ctx); ch {
		t.Fatal("fresh store reports change")
	}
	newProject(t, a)
	if ch, _ := a.ExternalChange(ctx); ch {
		t.Fatal("own write reported as external")
	}
	newProject(t, b)
	if ch, _ := a.ExternalChange(ctx); !ch {
		t.Fatal("foreign write not detected")
	}
	if ch, _ := a.ExternalChange(ctx); ch {
		t.Fatal("change reported twice")
	}
	// Interleaved foreign write followed by our own write.
	newProject(t, b)
	newProject(t, a)
	if ch, _ := a.ExternalChange(ctx); !ch {
		t.Fatal("interleaved foreign write missed")
	}
}

func TestConcurrentWriters(t *testing.T) {
	ctx := context.Background()
	s, _ := open(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := config.New("X", config.Source{Type: config.SourceURL, Value: "https://x.com/"}, config.Target{OS: "linux", Arch: "amd64"})
			if _, err := s.InsertProject(ctx, domain.Project{Config: c}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if l, _ := s.ListProjects(ctx, false); len(l) != 20 {
		t.Fatalf("got %d projects", len(l))
	}
	if seq, _ := s.ChangeSeq(ctx); seq != 20 {
		t.Fatalf("seq = %d", seq)
	}
}
