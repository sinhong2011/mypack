package appservice

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mypack.dev/mypack/pkg/config"
)

func TestLibraryExportImport(t *testing.T) {
	a, b := newEnv(t), newEnv(t)
	ctx := context.Background()
	p, err := a.svc.CreateProject(ctx, CreateRequest{Input: "notes.example.com", Name: "Notes", Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := p.Config.Clone()
	cfg.App.Icon.Style = &config.IconStyle{Glyph: config.GlyphText, Text: "N", Background: config.BackgroundGlass}
	cfg.Window.Tabs = true
	if p, err = a.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: p.Revision, Config: cfg, Actor: "cli"}); err != nil {
		t.Fatal(err)
	}
	base, _ := a.svc.ProjectIconImage(ctx, p.ID)

	file := filepath.Join(t.TempDir(), "lib.mypack")
	if n, err := a.svc.ExportLibrary(ctx, nil, file); err != nil || n != 1 {
		t.Fatalf("export: %d %v", n, err)
	}
	res, err := b.svc.ImportLibrary(ctx, file, ImportOptions{Actor: "cli"})
	if err != nil || len(res.Added) != 1 {
		t.Fatalf("import: %+v %v", res, err)
	}
	q, _ := b.svc.GetProject(ctx, res.Added[0])
	if q.Config.App.Identifier != p.Config.App.Identifier || q.Config.App.ProfileID != p.Config.App.ProfileID {
		t.Fatal("the identity did not travel")
	}
	if !q.Config.Window.Tabs || q.Config.App.Icon.Style == nil || !bytes.Equal(q.IconPNG, p.IconPNG) {
		t.Fatal("settings or icon did not travel")
	}
	if b2, _ := b.svc.ProjectIconImage(ctx, q.ID); !bytes.Equal(b2, base) {
		t.Fatal("the image beneath the style did not travel")
	}
	// Again: skipped, unless replacing.
	if res, _ = b.svc.ImportLibrary(ctx, file, ImportOptions{}); len(res.Skipped) != 1 || len(res.Added) != 0 {
		t.Fatalf("second import: %+v", res)
	}
	if res, _ = b.svc.ImportLibrary(ctx, file, ImportOptions{Replace: true}); len(res.Updated) != 1 || res.Updated[0] != q.ID {
		t.Fatalf("replace: %+v", res)
	}
	if _, err := b.svc.ImportLibrary(ctx, filepath.Join(t.TempDir(), "x.mypack"), ImportOptions{}); err == nil {
		t.Fatal("a missing file imports")
	}
	// The same document named .json is a Library file too; a config is not.
	asJSON := filepath.Join(t.TempDir(), "lib.json")
	raw, _ := os.ReadFile(file)
	os.WriteFile(asJSON, raw, 0o644)
	if !IsLibraryFile(asJSON) || !IsLibraryFile(file) {
		t.Fatal("a Library file is not recognised")
	}
	if res, err := b.svc.ImportLibrary(ctx, asJSON, ImportOptions{}); err != nil || len(res.Skipped) != 1 {
		t.Fatalf("import .json: %+v %v", res, err)
	}
	cfgFile := filepath.Join(t.TempDir(), "mypack.json")
	os.WriteFile(cfgFile, []byte(`{"schemaVersion":1,"app":{"name":"X"}}`), 0o644)
	if IsLibraryFile(cfgFile) || IsLibraryFile(filepath.Join(t.TempDir(), "missing.json")) {
		t.Fatal("a config or missing .json counts as a Library file")
	}
	bad := filepath.Join(t.TempDir(), "bad.mypack")
	os.WriteFile(bad, []byte(`{"format":"other"}`), 0o644)
	if _, err := b.svc.ImportLibrary(ctx, bad, ImportOptions{}); err == nil {
		t.Fatal("a foreign file imports")
	}
}

func TestLibraryGroupIcons(t *testing.T) {
	a, b := newEnv(t), newEnv(t)
	ctx := context.Background()
	var ids []string
	for _, n := range []string{"Slack", "Gmail"} {
		p, _ := a.svc.CreateProject(ctx, CreateRequest{Input: strings.ToLower(n) + ".example.com", Name: n, Actor: "cli"})
		ids = append(ids, p.ID)
	}
	g, err := a.svc.CreateGroup(ctx, GroupRequest{Name: "Work", Sites: sites(ids...)})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "g.mypack")
	if _, err := a.svc.ExportLibrary(ctx, []string{g.ID}, file); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file); bytes.Contains(b, []byte(a.dir)) {
		t.Fatal("the file holds paths of the exporting computer")
	}
	res, err := b.svc.ImportLibrary(ctx, file, ImportOptions{})
	if err != nil || len(res.Added) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	q, _ := b.svc.GetProject(ctx, res.Added[0])
	for _, m := range q.Config.App.Group.Members {
		if m.Icon == "" || !strings.HasPrefix(m.Icon, b.dir) {
			t.Fatalf("member %s icon = %q", m.ID, m.Icon)
		}
	}
}

func TestLibrarySync(t *testing.T) {
	folder := t.TempDir()
	a, b := newEnv(t), newEnv(t)
	ctx := context.Background()
	for _, e := range []*env{a, b} {
		set, _ := e.svc.Settings(ctx)
		set.SyncFolder = folder
		if err := e.svc.SaveSettings(ctx, set); err != nil {
			t.Fatal(err)
		}
	}
	p, _ := a.svc.CreateProject(ctx, CreateRequest{Input: "notes.example.com", Name: "Notes", Actor: "cli"})
	r, err := a.svc.SyncLibrary(ctx)
	if err != nil || r.Pushed != 1 {
		t.Fatalf("first push: %+v %v", r, err)
	}
	if r, _ = b.svc.SyncLibrary(ctx); r.Pulled != 1 {
		t.Fatalf("pull on b: %+v", r)
	}
	// In step: nothing moves either way.
	for _, e := range []*env{a, b, a} {
		if r, _ = e.svc.SyncLibrary(ctx); r.Pushed+r.Pulled+r.Trashed != 0 || len(r.Errors) > 0 {
			t.Fatalf("idle sync moved: %+v", r)
		}
	}
	// An edit on b reaches a.
	byID, _ := b.svc.projectsByIdentifier(ctx)
	q := byID[p.Config.App.Identifier]
	cfg := q.Config.Clone()
	cfg.App.Name = "Notes Work"
	time.Sleep(1100 * time.Millisecond) // UpdatedAt has second precision in files
	if _, err := b.svc.UpdateProject(ctx, UpdateRequest{ID: q.ID, ExpectedRevision: q.Revision, Config: cfg, Actor: "cli"}); err != nil {
		t.Fatal(err)
	}
	if r, _ = b.svc.SyncLibrary(ctx); r.Pushed != 1 {
		t.Fatalf("push edit: %+v", r)
	}
	if r, _ = a.svc.SyncLibrary(ctx); r.Pulled != 1 {
		t.Fatalf("pull edit: %+v", r)
	}
	if got, _ := a.svc.GetProject(ctx, p.ID); got.Config.App.Name != "Notes Work" {
		t.Fatalf("a has %q", got.Config.App.Name)
	}
	// Trash on a: b moves it to the Trash too; restoring on b brings it back.
	if err := a.svc.TrashProject(ctx, p.ID, "cli"); err != nil {
		t.Fatal(err)
	}
	if r, _ = a.svc.SyncLibrary(ctx); r.Pushed != 1 {
		t.Fatalf("push tombstone: %+v", r)
	}
	if r, _ = b.svc.SyncLibrary(ctx); r.Trashed != 1 {
		t.Fatalf("trash on b: %+v", r)
	}
	if got, _ := b.svc.GetProject(ctx, q.ID); got.TrashedAt == nil {
		t.Fatal("b did not trash it")
	}
	time.Sleep(1100 * time.Millisecond)
	b.svc.RestoreProject(ctx, q.ID, "cli")
	b.svc.SyncLibrary(ctx)
	a.svc.SyncLibrary(ctx)
	if got, _ := a.svc.GetProject(ctx, p.ID); got.TrashedAt != nil {
		t.Fatal("restoring on b did not restore on a")
	}
	if st, _ := a.svc.SyncStatus(ctx); st == nil || st.Apps != 1 {
		t.Fatalf("status: %+v", st)
	}
}
