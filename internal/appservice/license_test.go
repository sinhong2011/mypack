package appservice

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/installer"
	"mypack.dev/mypack/internal/licensing"
)

func licEnv(t *testing.T) (*env, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	e := newEnv(t, func(o *Options) {
		o.LicenseDir = filepath.Join(t.TempDir(), "lic")
		o.LicenseKeys = []ed25519.PublicKey{pub}
	})
	return e, priv
}

func token(t *testing.T, priv ed25519.PrivateKey) string {
	t.Helper()
	tok, err := licensing.Sign(priv, licensing.Payload{LicenseID: "lic_t", Edition: "pro", Email: "me@example.com",
		IssuedAt: time.Now(), Scope: []string{"builder"}, MaxDevices: 3})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

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

func TestFreeQuotaAndPro(t *testing.T) {
	e, priv := licEnv(t)
	ctx := context.Background()

	st, _ := e.svc.License(ctx)
	if st.Edition != "free" || st.Valid || st.FreeLimit != 2 || st.ManagedApps != 0 {
		t.Fatalf("initial %+v", st)
	}
	var first, second domain.Installation
	var err error
	if first, err = installNew(t, e, "Alpha"); err != nil {
		t.Fatal(err)
	}
	if second, err = installNew(t, e, "Beta"); err != nil {
		t.Fatal(err)
	}
	// Building is free; installing a third managed app is what is limited.
	_, err = installNew(t, e, "Gamma")
	var de *domain.Error
	if code(err) != CodeLicenseLimit {
		t.Fatalf("third install: %v", err)
	}
	de = err.(*domain.Error)
	if de.Remediation == "" {
		t.Error("LICENSE_LIMIT needs remediation")
	}
	if st, _ = e.svc.License(ctx); st.ManagedApps != 2 {
		t.Errorf("managed = %d", st.ManagedApps)
	}

	// Existing apps stay fully manageable at the limit: re-install of an
	// existing project (update path), repair and open.
	if err := e.svc.OpenInstallation(ctx, first.ID); err != nil {
		t.Errorf("open: %v", err)
	}
	rj, err := e.svc.RepairInstallation(ctx, first.ID, "cli")
	if err != nil {
		t.Fatalf("repair at limit: %v", err)
	}
	waitJob(t, e.svc, rj.ID, domain.JobSucceeded, domain.JobFailed)
	art := buildAndWait(t, e, mustProject(t, e, second.ProjectID)).Artifacts[0]
	if _, err := e.svc.InstallAndOpen(ctx, InstallRequest{ArtifactID: art.ID}); err != nil {
		t.Errorf("re-install existing at limit: %v", err)
	}

	// Activate Pro: unlimited.
	if _, err := e.svc.ActivateLicense(ctx, "garbage", "gui"); code(err) != CodeLicenseInvalid {
		t.Fatalf("garbage: %v", err)
	}
	if _, err := e.svc.ActivateLicense(ctx, "  ", "gui"); code(err) != CodeLicenseInvalid {
		t.Fatalf("blank: %v", err)
	}
	st, err = e.svc.ActivateLicense(ctx, token(t, priv), "gui")
	if err != nil || st.Edition != "pro" || !st.Valid || st.Email != "me@example.com" || st.LicenseID != "lic_t" || st.FreeLimit != 0 {
		t.Fatalf("activate: %+v %v", st, err)
	}
	if _, err := installNew(t, e, "Gamma"); err != nil {
		t.Fatalf("pro third install: %v", err)
	}
	// Persisted: a second "process" sees it without the network.
	e2, _ := New(e.opts)
	defer e2.Close()
	if st, _ := e2.License(ctx); st.Edition != "pro" {
		t.Errorf("not persisted: %+v", st)
	}

	// Deactivate: back to Free; the three apps stay installed and usable.
	if err := e.svc.DeactivateLicense(ctx, "gui"); err != nil {
		t.Fatal(err)
	}
	st, _ = e.svc.License(ctx)
	if st.Edition != "free" || st.ManagedApps != 3 {
		t.Errorf("after deactivate %+v", st)
	}
	if err := e.svc.OpenInstallation(ctx, first.ID); err != nil {
		t.Errorf("open after deactivate: %v", err)
	}
	// The activity log never contains the token.
	acts, _ := e.svc.Activity(ctx, 100)
	for _, a := range acts {
		if strings.Contains(a.Target, ".") && strings.HasPrefix(a.Action, "license") {
			t.Errorf("token leaked into activity: %+v", a)
		}
	}
}

func mustProject(t *testing.T, e *env, id string) domain.Project {
	t.Helper()
	p, err := e.svc.GetProject(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInvalidStoredLicenseFallsBackToFree(t *testing.T) {
	e, priv := licEnv(t)
	ctx := context.Background()
	if _, err := e.svc.ActivateLicense(ctx, token(t, priv), "gui"); err != nil {
		t.Fatal(err)
	}
	// Tamper with the stored file.
	f := filepath.Join(e.opts.LicenseDir, licensing.FileName)
	st := licensing.Store{Dir: e.opts.LicenseDir}
	tok, _ := st.Load()
	_ = st.Save(tok[:len(tok)-4] + "AAAA")
	s, _ := e.svc.License(ctx)
	if s.Edition != "free" || s.Valid || s.Error == "" {
		t.Fatalf("%+v", s)
	}
	if fi, err := os.Stat(f); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("license file perms: %v %v", fi, err)
	}
}

func TestRelocateInstallation(t *testing.T) {
	e, _ := licEnv(t)
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
	e, _ := licEnv(t)
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
