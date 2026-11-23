package appservice

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"mypack.dev/mypack/pkg/config"
)

func TestIconStyleRoundTrip(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p, err := e.svc.CreateProject(ctx, CreateRequest{Input: "notes.example.com", Name: "Notes", Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	orig := p.IconPNG

	cfg := p.Config.Clone()
	cfg.App.Icon.Style = &config.IconStyle{Glyph: config.GlyphText, Text: "N", Background: config.BackgroundGlass, Badge: "Work"}
	p, err = e.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: p.Revision, Config: cfg, Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(p.IconPNG, orig) {
		t.Fatal("the Library icon is not styled")
	}
	base, err := e.svc.ProjectIconImage(ctx, p.ID)
	if err != nil || !bytes.Equal(base, orig) {
		t.Fatal("the image beneath the style was not kept")
	}
	// Builds get the image beneath (the Builder draws the style itself).
	if got := e.svc.iconBase(p); !bytes.Equal(got, orig) {
		t.Fatal("builds would draw the style twice")
	}

	// A new style is drawn over the original image, not over the old style.
	styled := p.IconPNG
	cfg = p.Config.Clone()
	cfg.App.Icon.Style.Badge = "Home"
	p, err = e.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: p.Revision, Config: cfg, Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(p.IconPNG, styled) {
		t.Fatal("changing the badge did not redraw the icon")
	}

	// Duplicates keep the image beneath.
	d, err := e.svc.DuplicateProject(ctx, p.ID, "cli")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := e.svc.ProjectIconImage(ctx, d.ID); !bytes.Equal(b, orig) {
		t.Fatal("the duplicate lost the image beneath its style")
	}

	// Removing the style brings the original back.
	cfg = p.Config.Clone()
	cfg.App.Icon.Style = nil
	p, err = e.svc.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: p.Revision, Config: cfg, Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(p.IconPNG, orig) {
		t.Fatal("removing the style did not restore the icon")
	}
}

func TestSetProjectIconDocument(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.project(t)
	doc := filepath.Join(t.TempDir(), "Brand.icon")
	os.MkdirAll(doc, 0o755)
	if _, err := e.svc.SetProjectIconDocument(ctx, p.ID, doc, "gui"); err == nil {
		t.Fatal("a .icon folder without icon.json must be refused")
	}
	os.WriteFile(filepath.Join(doc, "icon.json"), []byte("{}"), 0o644)
	got, err := e.svc.SetProjectIconDocument(ctx, p.ID, doc, "gui")
	if err != nil {
		t.Fatal(err)
	}
	if ic := got.Config.App.Icon; ic.Source != "file" || ic.Path != doc || string(got.IconPNG) != "iconcomposer:Brand.icon" {
		t.Fatalf("icon %+v %q", ic, got.IconPNG)
	}
	// A new picture replaces the document, so builds stop compiling it.
	got, err = e.svc.SetProjectIcon(ctx, p.ID, []byte("img"), "gui")
	if err != nil {
		t.Fatal(err)
	}
	if ic := got.Config.App.Icon; ic.Path != "" || ic.Source != "default" {
		t.Fatalf("after a new picture: %+v", ic)
	}
}
