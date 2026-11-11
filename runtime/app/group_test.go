package app

import (
	"testing"

	"github.com/egoist/mygo"

	"mypack.dev/mypack/pkg/config"
)

func testGroup(t *testing.T, urls ...string) *group {
	t.Helper()
	cfg := config.New("G", config.Source{Type: config.SourceURL, Value: urls[0]}, config.Target{OS: "darwin", Arch: "arm64"})
	cfg.App.Group = &config.GroupConfig{}
	for i, u := range urls {
		cfg.App.Group.Members = append(cfg.App.Group.Members, config.GroupMember{ID: string(rune('a' + i)), Name: u, URL: u})
	}
	a, _ := New(cfg, t.TempDir())
	a.state = OpenStore(t.TempDir())
	return a.newGroup()
}

func TestGroupOwnerOf(t *testing.T) {
	g := testGroup(t, "https://app.slack.com", "https://mail.google.com", "https://calendar.google.com", "https://linear.app")
	slack, gmail, cal, linear := g.members[0], g.members[1], g.members[2], g.members[3]
	for _, tc := range []struct {
		raw  string
		from *groupMember
		want *groupMember
	}{
		{"https://linear.app/team/issue/1", slack, linear}, // another member's site
		{"https://mail.google.com/mail/u/0", slack, gmail}, // exact host
		{"https://calendar.google.com/r", gmail, cal},      // same domain, other member's exact host
		{"https://docs.google.com/x", slack, nil},          // google.com is ambiguous (two members)
		{"https://accounts.google.com/signin", gmail, nil}, // its own domain: stays (sign-in)
		{"https://files.slack.com/f", slack, nil},          // its own site
		{"https://example.com", slack, nil},                // nobody's
		{"mailto:x@y.z", slack, nil},                       // not web
		{"https://linear.app/x", nil, linear},              // a link handed to the app
	} {
		if got := g.ownerOf(tc.raw, tc.from); got != tc.want {
			name := func(m *groupMember) string {
				if m == nil {
					return "nil"
				}
				return m.cfg.URL
			}
			t.Errorf("ownerOf(%s from %s) = %s, want %s", tc.raw, name(tc.from), name(got), name(tc.want))
		}
	}
	if d := g.safeDomains(); len(d) != 4 || d[0] != "slack.com" || d[1] != "google.com" {
		t.Fatalf("safe domains = %v", d)
	}
}

func TestGroupSwitchAndSplit(t *testing.T) {
	g := testGroup(t, "https://a.example", "https://b.example", "https://c.example")
	g.show(1, false)
	if g.active != 1 || g.prev != 0 {
		t.Fatalf("show: active=%d prev=%d", g.active, g.prev)
	}
	g.toggleSplit() // beside the one shown before
	if g.split != 0 {
		t.Fatalf("split = %d", g.split)
	}
	g.show(0, false) // picking the split one swaps sides
	if g.active != 0 || g.split != 1 {
		t.Fatalf("swap: active=%d split=%d", g.active, g.split)
	}
	g.splitWith(2)
	g.toggleSplit()
	if g.split != -1 {
		t.Fatal("toggle closes the split")
	}
	g.step(-1)
	if g.active != 2 {
		t.Fatalf("step back wraps: %d", g.active)
	}
}

func TestBadgeSum(t *testing.T) {
	var got string
	c := newBadgeController(true, func(l string) { got = l })
	c.sum, c.minGap = true, 0
	p1, p2 := new(mygo.Page), new(mygo.Page)
	c.setTitle(p1, "(3) Inbox")
	c.setTitle(p2, "Chat (4)")
	if got != "7" || c.pageLabel(p1) != "3" || c.pageLabel(p2) != "4" {
		t.Fatalf("sum = %q, labels %q %q", got, c.pageLabel(p1), c.pageLabel(p2))
	}
}
