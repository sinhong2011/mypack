package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

type memPerms struct{ m map[string]string }

func (p *memPerms) Get(o string) string { return p.m[o] }
func (p *memPerms) Set(o, s string) {
	if p.m == nil {
		p.m = map[string]string{}
	}
	p.m[o] = s
}

type shown struct {
	opts    mygo.NotificationOptions
	onClick func()
}

type harness struct {
	b       *bridgeService
	url     string
	shown   []shown
	focused int
	clicked []string
	prompts int
	allow   bool
	showErr error
	closed  []string
	perms   *memPerms
	now     time.Time
	badges  []string
}

func newHarness(mode string) *harness {
	h := &harness{url: "https://mail.example.com/inbox", allow: true, perms: &memPerms{}, now: time.Unix(5000, 0)}
	var mu sync.Mutex
	h.b = &bridgeService{
		mode: mode, badge: true, perms: h.perms, asked: map[string]bool{},
		now:    func() time.Time { return h.now },
		caller: func(context.Context) (*mygo.Page, string) { return nil, h.url },
		isApp: func(raw string) bool {
			return raw == "https://mail.example.com/inbox" || raw == "https://app.example.com/"
		},
		show: func(o mygo.NotificationOptions, onClick func()) (func(), error) {
			mu.Lock()
			defer mu.Unlock()
			if h.showErr != nil {
				return nil, h.showErr
			}
			h.shown = append(h.shown, shown{o, onClick})
			return func() { h.closed = append(h.closed, o.ID) }, nil
		},
		prompt: func(*mygo.Page, string) bool { h.prompts++; return h.allow },
		focus:  func(*mygo.Page) { h.focused++ },
		click:  func(_ *mygo.Page, id string) { h.clicked = append(h.clicked, id) },
	}
	h.b.badges = newBadgeController(true, func(l string) { h.badges = append(h.badges, l) })
	return h
}

func req(id, title string) NotifyRequest { return NotifyRequest{ID: id, Title: title, Body: "b"} }

func TestNotifyAllow(t *testing.T) {
	h := newHarness("allow")
	ctx := context.Background()
	if r, _ := h.b.Notify(ctx, NotifyRequest{ID: "n1", Title: "Hello\x00 world", Body: "line\x07", Tag: "chat", Silent: true}); r != "shown" {
		t.Fatalf("Notify = %q", r)
	}
	s := h.shown[0]
	if s.opts.Title != "Hello world" || s.opts.Body != "line" || !s.opts.Silent || s.opts.ID == "" {
		t.Fatalf("native options: %+v", s.opts)
	}
	// Click focuses the window and dispatches 'click' to the JS notification.
	s.onClick()
	if h.focused != 1 || len(h.clicked) != 1 || h.clicked[0] != "n1" {
		t.Fatalf("click: focused=%d clicked=%v", h.focused, h.clicked)
	}
	// Same tag => same native ID (replaces); different tag => different.
	h.b.Notify(ctx, NotifyRequest{ID: "n2", Title: "again", Tag: "chat"})
	h.b.Notify(ctx, NotifyRequest{ID: "n3", Title: "other", Tag: "mail"})
	if h.shown[1].opts.ID != s.opts.ID || h.shown[2].opts.ID == s.opts.ID {
		t.Fatalf("tag ids: %q %q %q", s.opts.ID, h.shown[1].opts.ID, h.shown[2].opts.ID)
	}
	// An empty title falls back to the body.
	h.b.Notify(ctx, NotifyRequest{ID: "n4", Body: "only body"})
	if h.shown[3].opts.Title != "only body" || h.shown[3].opts.Body != "" {
		t.Fatalf("%+v", h.shown[3].opts)
	}
	// Close removes the native notification.
	h.b.CloseNotification(ctx, "n3")
	if len(h.closed) != 1 || h.closed[0] != h.shown[2].opts.ID {
		t.Fatalf("closed = %v", h.closed)
	}
}

func TestNotifyOriginsAndModes(t *testing.T) {
	ctx := context.Background()
	h := newHarness("allow")
	h.url = "https://ads.thirdparty.net/frame" // not part of the app
	if r, _ := h.b.Notify(ctx, req("x", "spam")); r != "blocked" || len(h.shown) != 0 {
		t.Fatalf("third party: %q shown=%d", r, len(h.shown))
	}
	if h.b.Permission(ctx) != "denied" || h.b.RequestPermission(ctx) != "denied" {
		t.Fatal("third parties see denied")
	}
	h.url = "about:blank"
	if r, _ := h.b.Notify(ctx, req("x", "t")); r != "blocked" {
		t.Fatalf("about:blank: %q", r)
	}

	d := newHarness("deny")
	if r, _ := d.b.Notify(ctx, req("x", "t")); r != "denied" || len(d.shown) != 0 {
		t.Fatalf("deny: %q", r)
	}
	if d.b.Permission(ctx) != "denied" {
		t.Fatal("deny permission")
	}

	// The system refusing (macOS permission) is reported as denied.
	e := newHarness("allow")
	e.showErr = mygo.ErrNotificationsDenied
	if r, _ := e.b.Notify(ctx, req("x", "t")); r != "denied" {
		t.Fatalf("system denied: %q", r)
	}
	e.showErr = errors.New("boom")
	if r, _ := e.b.Notify(ctx, req("x", "t")); r != "unsupported" {
		t.Fatalf("error: %q", r)
	}
}

func TestNotifyAskMode(t *testing.T) {
	ctx := context.Background()
	h := newHarness("ask")
	if h.b.Permission(ctx) != "default" {
		t.Fatal("ask starts as default")
	}
	// Notifying without permission does nothing.
	if r, _ := h.b.Notify(ctx, req("a", "t")); r != "denied" || len(h.shown) != 0 {
		t.Fatalf("before grant: %q", r)
	}
	if got := h.b.RequestPermission(ctx); got != "granted" || h.prompts != 1 {
		t.Fatalf("request: %q prompts=%d", got, h.prompts)
	}
	// Remembered: no second prompt, notifications flow.
	if got := h.b.RequestPermission(ctx); got != "granted" || h.prompts != 1 {
		t.Fatalf("second request prompted: %q prompts=%d", got, h.prompts)
	}
	if h.perms.m["https://mail.example.com"] != "granted" {
		t.Fatalf("not persisted per origin: %v", h.perms.m)
	}
	if r, _ := h.b.Notify(ctx, req("a", "t")); r != "shown" {
		t.Fatalf("after grant: %q", r)
	}
	// A different origin asks again; a refusal sticks and never prompts again.
	h.url = "https://app.example.com/"
	h.allow = false
	if got := h.b.RequestPermission(ctx); got != "denied" || h.prompts != 2 {
		t.Fatalf("second origin: %q prompts=%d", got, h.prompts)
	}
	if got := h.b.RequestPermission(ctx); got != "denied" || h.prompts != 2 {
		t.Fatalf("remembered denial: %q prompts=%d", got, h.prompts)
	}
	if r, _ := h.b.Notify(ctx, req("b", "t")); r != "denied" {
		t.Fatalf("denied origin notified: %q", r)
	}
	// allow/deny modes never prompt.
	a := newHarness("allow")
	if a.b.RequestPermission(ctx) != "granted" || a.prompts != 0 {
		t.Fatal("allow mode prompted")
	}
}

func TestNotifyRateLimit(t *testing.T) {
	h := newHarness("allow")
	ctx := context.Background()
	for i := 0; i < notifyBurst; i++ {
		if r, _ := h.b.Notify(ctx, req("n", "t")); r != "shown" {
			t.Fatalf("#%d: %q", i, r)
		}
	}
	if r, _ := h.b.Notify(ctx, req("n", "t")); r != "limited" {
		t.Fatalf("over the limit: %q", r)
	}
	h.now = h.now.Add(notifyWindow + time.Second)
	if r, _ := h.b.Notify(ctx, req("n", "t")); r != "shown" {
		t.Fatalf("after the window: %q", r)
	}
}

func TestSetBadgeFromAPI(t *testing.T) {
	h := newHarness("allow")
	ctx := context.Background()
	h.b.SetBadge(ctx, 5)
	if len(h.badges) != 1 || h.badges[0] != "5" {
		t.Fatalf("badges = %v", h.badges)
	}
	h.url = "https://ads.thirdparty.net/"
	h.b.SetBadge(ctx, 77) // third parties cannot badge
	if len(h.badges) != 1 {
		t.Fatalf("third-party badge: %v", h.badges)
	}
	off := newHarness("allow")
	off.b.badge = false
	off.b.SetBadge(ctx, 3)
	if len(off.badges) != 0 {
		t.Fatalf("badge off: %v", off.badges)
	}
}

func TestDecodeDataImage(t *testing.T) {
	png := "data:image/png;base64,iVBORw0KGgo="
	if len(decodeDataImage(png)) == 0 {
		t.Error("png data URL should decode")
	}
	for _, bad := range []string{"", "https://x/y.png", "data:image/svg+xml;base64,PHN2Zz4=", "data:image/png,rawtext", "data:text/html;base64,PGI+"} {
		if decodeDataImage(bad) != nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestCleanText(t *testing.T) {
	if got := cleanText("  a\x00b‮ c\n", 10); got != "ab‮ c" && got != "ab c" {
		t.Logf("cleanText = %q", got)
	}
	long := make([]rune, 500)
	for i := range long {
		long[i] = 'é'
	}
	if got := cleanText(string(long), 50); len([]rune(got)) != 50 {
		t.Fatalf("len = %d", len([]rune(got)))
	}
}
