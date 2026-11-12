package appservice

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/installer"
)

// installNew creates a distinct project, builds it and installs it.
func installNew(t *testing.T, e *env, name string) (domain.Installation, error) {
	t.Helper()
	ctx := context.Background()
	p, err := e.svc.CreateProject(ctx, CreateRequest{Input: strings.ToLower(name) + ".example.com", Name: name, Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	art := buildAndWait(t, e, p).Artifacts[0]
	return e.svc.InstallAndOpen(ctx, InstallRequest{ArtifactID: art.ID, Actor: "cli"})
}

func mustProject(t *testing.T, e *env, id string) domain.Project {
	t.Helper()
	p, err := e.svc.GetProject(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRelocateInstallation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in, err := installNew(t, e, "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	ident := in.Identifier
	moved := filepath.Join(t.TempDir(), "Moved.app")
	_ = os.MkdirAll(moved, 0o755)
	_ = os.RemoveAll(in.Path)
	if l, _ := e.svc.ListInstallations(ctx); l[0].State != domain.InstallMissing {
		t.Fatalf("want missing: %+v", l[0])
	}

	// Wrong identifier: never silently adopt a different app.
	e.ins.info = func(string) (installer.AppInfo, error) {
		return installer.AppInfo{Name: "Other", Identifier: "com.other", Version: "9", Managed: true}, nil
	}
	if _, err := e.svc.RelocateInstallation(ctx, in.ID, moved, "gui"); code(err) != CodeNotSameApp {
		t.Fatalf("different app: %v", err)
	}
	// Right identifier but not created by mypack.
	e.ins.info = func(string) (installer.AppInfo, error) {
		return installer.AppInfo{Name: "Alpha", Identifier: ident, Version: "1"}, nil
	}
	if _, err := e.svc.RelocateInstallation(ctx, in.ID, moved, "gui"); code(err) != CodeNotSameApp {
		t.Fatalf("not managed: %v", err)
	}
	if got, _ := e.svc.ListInstallations(ctx); got[0].Path != in.Path || got[0].State != domain.InstallMissing {
		t.Fatalf("failed relocate must not change the record: %+v", got[0])
	}
	// Not an app at all.
	e.ins.info = func(string) (installer.AppInfo, error) {
		return installer.AppInfo{}, &domain.Error{Code: "NOT_FOUND", Message: "nope"}
	}
	if _, err := e.svc.RelocateInstallation(ctx, in.ID, moved, "gui"); code(err) != "NOT_FOUND" {
		t.Fatalf("missing: %v", err)
	}
	// Match.
	e.ins.info = func(string) (installer.AppInfo, error) {
		return installer.AppInfo{Name: "Alpha", Identifier: ident, Version: "1.2", Managed: true}, nil
	}
	got, err := e.svc.RelocateInstallation(ctx, in.ID, moved, "gui")
	if err != nil || got.Path != moved || got.State != domain.InstallManaged || got.Version != "1.2" {
		t.Fatalf("relocate: %v %+v", err, got)
	}
	if err := e.svc.OpenInstallation(ctx, in.ID); err != nil {
		t.Errorf("open after relocate: %v", err)
	}
	if _, err := e.svc.RelocateInstallation(ctx, "nope", moved, "gui"); code(err) != CodeNotFound {
		t.Errorf("unknown id: %v", err)
	}
}

func TestForgetInstallationKeepsFiles(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in, err := installNew(t, e, "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ForgetInstallation(ctx, in.ID, "gui"); err != nil {
		t.Fatal(err)
	}
	if l, _ := e.svc.ListInstallations(ctx); len(l) != 0 {
		t.Errorf("still listed: %+v", l)
	}
	if _, err := os.Stat(in.Path); err != nil {
		t.Errorf("files must be untouched: %v", err)
	}
	if len(e.ins.uninst) != 0 || len(e.ins.cleared) != 0 {
		t.Error("forget must not uninstall or clear data")
	}
	if err := e.svc.ForgetInstallation(ctx, in.ID, "gui"); code(err) != CodeNotFound {
		t.Errorf("twice: %v", err)
	}
}
