package appservice

import (
	"bytes"
	"context"
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
