package appservice

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mypack.dev/mypack/pkg/config"
)

func TestCreateGroup(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	var ids []string
	for _, n := range []string{"Slack", "Gmail", "Linear"} {
		p, err := e.svc.CreateProject(ctx, CreateRequest{Input: strings.ToLower(n) + ".example.com", Name: n, Actor: "cli"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
	}
	g, err := e.svc.CreateGroup(ctx, GroupRequest{Name: "Work", Sites: sites(ids...), Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	c := g.Config
	if !c.IsGroup() || len(c.App.Group.Members) != 3 || c.App.Name != "Work" {
		t.Fatalf("group config: %+v", c.App)
	}
	m := c.App.Group.Members
	if m[0].ID != "slack" || m[1].ID != "gmail" || m[0].ProjectID != ids[0] || c.App.Source.Value != m[0].URL {
		t.Fatalf("members: %+v", m)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mm := range m {
		if mm.Icon != "" {
			if _, err := os.Stat(mm.Icon); err != nil {
				t.Fatalf("member icon %s: %v", mm.Icon, err)
			}
		}
	}
	if len(g.IconPNG) == 0 {
		t.Fatal("group icon missing")
	}
	// A group cannot join a group; one website is not a group.
	if _, err := e.svc.CreateGroup(ctx, GroupRequest{Name: "X", Sites: sites(g.ID, ids[0])}); err == nil {
		t.Fatal("a group joined a group")
	}
	// The same website twice is two accounts of it, each its own member.
	two, err := e.svc.CreateGroup(ctx, GroupRequest{Name: "Two Slacks", Sites: sites(ids[0], ids[0])})
	if err != nil {
		t.Fatal(err)
	}
	if tm := two.Config.App.Group.Members; len(tm) != 2 || tm[0].ID == tm[1].ID || tm[1].Name != "Slack 2" {
		t.Fatalf("two accounts of one site: %+v", tm)
	} else if err := two.Config.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateGroup(ctx, GroupRequest{Name: " ", Sites: sites(ids...)}); err == nil {
		t.Fatal("an unnamed group was made")
	}
}

func sites(ids ...string) []GroupSite {
	var out []GroupSite
	for _, id := range ids {
		out = append(out, GroupSite{ProjectID: id})
	}
	return out
}

func TestCreateGroupFromAddresses(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p, err := e.svc.CreateProject(ctx, CreateRequest{Input: "slack.example.com", Name: "Slack", Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := e.svc.CreateGroup(ctx, GroupRequest{Name: "Chats", Sites: []GroupSite{
		{URL: "discord.example.com"}, {ProjectID: p.ID}, {URL: "https://discord.example.com"}, {URL: "telegram.example.com"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	m := g.Config.App.Group.Members
	// A repeated address is another account of the site: its own member
	// (and session), numbered.
	if len(m) != 4 || m[0].URL != "https://discord.example.com/" || m[1].ProjectID != p.ID ||
		m[2].URL != m[0].URL || m[2].ID == m[0].ID || m[2].Name != m[0].Name+" 2" || m[3].ProjectID != "" {
		t.Fatalf("members: %+v", m)
	}
	if m[0].Name == "" || m[0].ID == "" {
		t.Fatalf("an address member needs a name and id: %+v", m[0])
	}
	acc, err := e.svc.CreateGroup(ctx, GroupRequest{Name: "Accounts", Sites: []GroupSite{
		{URL: "chat.example.com", Name: "Work"}, {URL: "chat.example.com", Name: "Home"}, {URL: "chat.example.com"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	am := acc.Config.App.Group.Members
	if len(am) != 3 || am[0].Name != "Work" || am[1].Name != "Home" || am[0].ID == am[1].ID || am[1].ID == am[2].ID {
		t.Fatalf("accounts of one site: %+v", am)
	}
	if _, err := e.svc.CreateGroup(ctx, GroupRequest{Name: "X", Sites: []GroupSite{{URL: "not a url"}, {URL: "a.example.com"}}}); err == nil {
		t.Fatal("a bad address was accepted")
	}
}

func TestCreateGroupWithLocalSites(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	root := t.TempDir()
	docs := filepath.Join(root, "Docs")
	os.MkdirAll(docs, 0o755)
	os.WriteFile(filepath.Join(docs, "index.html"), []byte("docs"), 0o644)
	board := filepath.Join(root, "board.html")
	os.WriteFile(board, []byte("board"), 0o644)
	g, err := e.svc.CreateGroup(ctx, GroupRequest{Name: "Local", Sites: []GroupSite{{Path: docs}, {Path: board}, {URL: "chat.example.com"}}})
	if err != nil {
		t.Fatal(err)
	}
	m := g.Config.App.Group.Members
	if !m[0].IsLocal() || m[0].Local != docs || m[0].Name != "Docs" || m[1].Local != board || m[1].Name != "board" || m[2].IsLocal() {
		t.Fatalf("members: %+v", m)
	}
	if g.Config.App.Source.Type != config.SourceURL || g.Config.App.Source.Value != m[2].URL {
		t.Fatalf("a group's source is its first website: %+v", g.Config.App.Source)
	}
	if err := g.Config.Validate(); err != nil {
		t.Fatal(err)
	}
	// An unbuilt JavaScript project can't join; a folder without index.html neither.
	proj := filepath.Join(root, "proj")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "package.json"), []byte(`{"name":"proj","scripts":{"build":"vite build"},"devDependencies":{"vite":"5"}}`), 0o644)
	empty := filepath.Join(root, "empty")
	os.MkdirAll(empty, 0o755)
	for _, bad := range []string{proj, empty, filepath.Join(root, "nope")} {
		e2 := newEnv(t)
		if _, err := e2.svc.CreateGroup(ctx, GroupRequest{Name: "X", Sites: []GroupSite{{Path: bad}, {URL: "a.example.com"}}}); err == nil {
			t.Fatalf("%s joined a group", bad)
		}
	}
}
