package appservice

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"mypack.dev/mypack/runtime/routing"
)

func routesFile(e *env) string { return filepath.Join(e.dir, "db", "routes.json") }

func waitRoutes(t *testing.T, e *env, want func([]routing.Entry) bool) []routing.Entry {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		es, _ := routing.Read(routesFile(e))
		if want(es) {
			return es
		}
		if time.Now().After(deadline) {
			t.Fatalf("routes never matched; last: %+v", es)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRoutesFollowInstallations(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.project(t)
	job := buildAndWait(t, e, p)

	// Service start writes an (empty) table.
	waitRoutes(t, e, func(es []routing.Entry) bool { return len(es) == 0 })

	inst, err := e.svc.InstallAndOpen(ctx, InstallRequest{ArtifactID: job.Artifacts[0].ID, Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	es := waitRoutes(t, e, func(es []routing.Entry) bool { return len(es) == 1 })
	got := es[0]
	if got.Identifier != p.Config.App.Identifier || got.BundlePath != inst.Path || got.SourceHost != "github.com" {
		t.Fatalf("entry = %+v", got)
	}
	var hasGH bool
	for _, h := range got.Hosts {
		hasGH = hasGH || h == "github.com"
	}
	if !hasGH {
		t.Fatalf("hosts = %v", got.Hosts)
	}

	if err := e.svc.Uninstall(ctx, inst.ID, false, "gui"); err != nil {
		t.Fatal(err)
	}
	waitRoutes(t, e, func(es []routing.Entry) bool { return len(es) == 0 })
}

func TestRoutesOptOutAndDisabled(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.project(t)
	off := false
	c := p.Config.Clone()
	c.Native.LinkRouting = &off
	if _, err := e.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: p.Revision, Config: c}); err != nil {
		t.Fatal(err)
	}
	job := buildAndWait(t, e, p)
	if _, err := e.svc.InstallAndOpen(ctx, InstallRequest{ArtifactID: job.Artifacts[0].ID}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SyncRoutes(ctx); err != nil {
		t.Fatal(err)
	}
	if es, _ := routing.Read(routesFile(e)); len(es) != 0 {
		t.Fatalf("an app with linkRouting=false must not be a target: %+v", es)
	}

	e2 := newEnv(t, func(o *Options) { o.RoutesFile = "-" })
	if e2.svc.routesPath() != "" || e2.svc.SyncRoutes(ctx) != nil {
		t.Fatal("RoutesFile \"-\" must disable writing")
	}
}
