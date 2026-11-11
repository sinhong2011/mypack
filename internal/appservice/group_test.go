package appservice

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"mypack.dev/mypack/internal/domain"
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
	if _, err := e.svc.CreateGroup(ctx, GroupRequest{Name: "X", Sites: sites(ids[0], ids[0])}); err == nil {
		t.Fatal("the same website twice made a group")
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
	if len(m) != 3 || m[0].URL != "https://discord.example.com/" || m[1].ProjectID != p.ID || m[2].ProjectID != "" {
		t.Fatalf("members (a repeated address is dropped): %+v", m)
	}
	if m[0].Name == "" || m[0].ID == "" {
		t.Fatalf("an address member needs a name and id: %+v", m[0])
	}
	if _, err := e.svc.CreateGroup(ctx, GroupRequest{Name: "X", Sites: []GroupSite{{URL: "not a url"}, {URL: "a.example.com"}}}); err == nil {
		t.Fatal("a bad address was accepted")
	}
}

func TestGroupQuota(t *testing.T) {
	e, priv := licEnv(t)
	t.Setenv("MYPACK_LICENSE", "")
	ctx := context.Background()
	var ids []string
	for _, n := range []string{"Slack", "Gmail"} {
		p, err := e.svc.CreateProject(ctx, CreateRequest{Input: strings.ToLower(n) + ".example.com", Name: n, Actor: "cli"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
	}
	if _, err := e.svc.CreateGroup(ctx, GroupRequest{Name: "One", Sites: sites(ids...)}); err != nil {
		t.Fatalf("the free edition's first group: %v", err)
	}
	st, _ := e.svc.License(ctx)
	if st.Groups != 1 || st.FreeGroups != FreeGroups {
		t.Fatalf("status: %+v", st)
	}
	_, err := e.svc.CreateGroup(ctx, GroupRequest{Name: "Two", Sites: sites(ids...)})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != CodeLicenseLimit {
		t.Fatalf("a second free group: %v", err)
	}
	if _, err := e.svc.ActivateLicense(ctx, token(t, priv), "cli"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateGroup(ctx, GroupRequest{Name: "Two", Sites: sites(ids...)}); err != nil {
		t.Fatalf("pro has unlimited groups: %v", err)
	}
}
