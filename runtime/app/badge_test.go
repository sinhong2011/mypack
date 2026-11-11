package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo"
)

func TestParseTitleBadge(t *testing.T) {
	cases := []struct {
		title string
		n     int
		dot   bool
	}{
		{"(3) Inbox", 3, false},
		{"(12) WhatsApp", 12, false},
		{"[7] Slack | general", 7, false},
		{"Inbox (12)", 12, false},
		{"Inbox (3) - me@example.com - Gmail", 3, false},
		{"(99+) Discord | #general", 99, false},
		{"(0) Inbox", 0, false},
		{"• Slack", 0, true},
		{"* general - Acme - Slack", 0, true},
		{"● Messages", 0, true},
		{"Inbox", 0, false},
		{"(Draft) Inbox", 0, false},
		{"(2024) Annual report", 0, false}, // a year
		{"Report (2024)", 0, false},
		{"2 new messages", 0, false},
		{"", 0, false},
		{"(3)", 0, false},
		{"Chapter 3 (draft)", 0, false},
	}
	for _, c := range cases {
		n, dot := ParseTitleBadge(c.title)
		if n != c.n || dot != c.dot {
			t.Errorf("ParseTitleBadge(%q) = %d,%v want %d,%v", c.title, n, dot, c.n, c.dot)
		}
	}
}

func TestBadgeLabel(t *testing.T) {
	if badgeLabel(0, false) != "" || badgeLabel(5, false) != "5" || badgeLabel(0, true) != "•" || badgeLabel(5, true) != "5" || badgeLabel(1500, false) != "999+" {
		t.Fatal("labels wrong")
	}
}

// fakeClock drives the controller's throttle deterministically.
type fakeClock struct {
	now    time.Time
	fire   func()
	armed  bool
	labels []string
}

func newTestBadges(auto bool) (*badgeController, *fakeClock) {
	fc := &fakeClock{now: time.Unix(1000, 0)}
	c := newBadgeController(auto, func(l string) { fc.labels = append(fc.labels, l) })
	c.now = func() time.Time { return fc.now }
	c.after = func(d time.Duration, f func()) func() {
		fc.armed, fc.fire = true, f
		return func() { fc.armed = false }
	}
	return c, fc
}

func TestBadgeController(t *testing.T) {
	p1, p2 := new(mygo.Page), new(mygo.Page)
	c, fc := newTestBadges(true)

	c.setTitle(p1, "(3) Inbox")
	if len(fc.labels) != 1 || fc.labels[0] != "3" {
		t.Fatalf("labels = %v", fc.labels)
	}
	// Burst of changes inside the gap: only the last one is applied, later.
	fc.now = fc.now.Add(50 * time.Millisecond)
	c.setTitle(p1, "(4) Inbox")
	c.setTitle(p1, "(5) Inbox")
	c.setTitle(p1, "(6) Inbox")
	if len(fc.labels) != 1 || !fc.armed {
		t.Fatalf("expected a pending trailing update, labels = %v armed=%v", fc.labels, fc.armed)
	}
	fc.now = fc.now.Add(time.Second)
	fc.fire()
	if len(fc.labels) != 2 || fc.labels[1] != "6" {
		t.Fatalf("labels = %v", fc.labels)
	}
	// Unchanged label: nothing is set.
	fc.now = fc.now.Add(time.Second)
	c.setTitle(p1, "(6) Another title")
	if len(fc.labels) != 2 {
		t.Fatalf("redundant native call: %v", fc.labels)
	}
	// The Badging API beats the title of the same page; clearing it hands back.
	fc.now = fc.now.Add(time.Second)
	c.setAPI(p1, 9)
	if fc.labels[len(fc.labels)-1] != "9" {
		t.Fatalf("api: %v", fc.labels)
	}
	fc.now = fc.now.Add(time.Second)
	c.setAPI(p1, 0)
	if fc.labels[len(fc.labels)-1] != "6" {
		t.Fatalf("title again after clearAppBadge: %v", fc.labels)
	}
	// Largest count across windows; closing a window drops its share.
	fc.now = fc.now.Add(time.Second)
	c.setTitle(p2, "(20) Chat")
	if fc.labels[len(fc.labels)-1] != "20" {
		t.Fatalf("max across windows: %v", fc.labels)
	}
	fc.now = fc.now.Add(time.Second)
	c.drop(p2)
	if fc.labels[len(fc.labels)-1] != "6" {
		t.Fatalf("drop: %v", fc.labels)
	}
	// Dot only when nothing has a count; zero clears.
	fc.now = fc.now.Add(time.Second)
	c.setTitle(p1, "• Slack")
	if fc.labels[len(fc.labels)-1] != "•" {
		t.Fatalf("dot: %v", fc.labels)
	}
	fc.now = fc.now.Add(time.Second)
	c.setTitle(p1, "Slack")
	if fc.labels[len(fc.labels)-1] != "" {
		t.Fatalf("clear: %v", fc.labels)
	}
}

func TestBadgeAutoOff(t *testing.T) {
	c, fc := newTestBadges(false) // native.badge = off: no title heuristic
	c.setTitle(new(mygo.Page), "(3) Inbox")
	if len(fc.labels) != 0 {
		t.Fatalf("title heuristic must be disabled: %v", fc.labels)
	}
}
