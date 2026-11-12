package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/egoist/mygo"
)

// bridgeService is the Go side of the web API shims (runtime/inject/webapi.go).
// It is bound as service "mypack" and called by the page through
// window.mygo.call("mypack.<Method>", ...). Every method first checks that the
// calling page belongs to the app (navigation policy): identity-provider
// popups and unrelated sites that ended up in a window may not notify.
type bridgeService struct {
	mode  string // notifications: allow | ask | deny
	badge bool

	// seams (set by newBridge; tests replace them)
	caller func(ctx context.Context) (page *mygo.Page, rawURL string)
	isApp  func(rawURL string) bool
	perms  permStore
	show   func(opts mygo.NotificationOptions, onClick func()) (closeFn func(), err error)
	prompt func(page *mygo.Page, origin string) bool
	focus  func(page *mygo.Page)
	click  func(page *mygo.Page, jsID string)
	badges *badgeController
	now    func() time.Time

	mu    sync.Mutex
	times []time.Time // delivery times in the rate-limit window
	live  map[string]*liveNote
	order []string
	asked map[string]bool // origins with a prompt in flight
}

type liveNote struct {
	page  *mygo.Page
	jsID  string
	close func()
}

// permStore remembers per-origin answers of "ask" mode (profile state).
type permStore interface {
	Get(origin string) string // "granted" | "denied" | ""
	Set(origin, state string)
}

// Rate limit: at most notifyBurst notifications per notifyWindow, per app.
const (
	notifyBurst  = 10
	notifyWindow = time.Minute
	maxLive      = 200
	maxTitle     = 200
	maxBody      = 1000
	maxIconBytes = 400 << 10
)

// NotifyRequest is what the page's Notification constructor posts.
type NotifyRequest struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Tag    string `json:"tag"`
	Icon   string `json:"icon"` // data: URL (the shim inlines remote icons), or ""
	Silent bool   `json:"silent"`
}

// Notify shows a native notification: "shown", "denied" (permission), "limited"
// (rate limit), "unsupported" or "blocked" (calling page not part of the app).
func (b *bridgeService) Notify(ctx context.Context, r NotifyRequest) (string, error) {
	page, raw := b.caller(ctx)
	origin := originOf(raw)
	if !b.isApp(raw) || origin == "" {
		log.Printf("mypack-runtime: notify: blocked, %q is not part of the app", origin)
		return "blocked", nil
	}
	switch b.state(origin) {
	case "granted":
	default:
		log.Printf("mypack-runtime: notify: denied for %s (mode %s)", origin, b.mode)
		return "denied", nil
	}
	if !b.allow() {
		log.Printf("mypack-runtime: notify: rate limited (%d per %s)", notifyBurst, notifyWindow)
		return "limited", nil
	}
	title := cleanText(r.Title, maxTitle)
	body := cleanText(r.Body, maxBody)
	if title == "" {
		title = body
		body = ""
	}
	nativeID := "mypack-" + shortHash(origin+"|"+r.ID)
	if r.Tag != "" {
		nativeID = "mypack-" + shortHash(origin+"|tag|"+r.Tag)
	}
	opts := mygo.NotificationOptions{ID: nativeID, Title: title, Body: body, Silent: r.Silent, Image: decodeDataImage(r.Icon)}
	ln := &liveNote{page: page, jsID: r.ID}
	onClick := func() {
		log.Printf("mypack-runtime: notify: clicked %s", nativeID)
		b.focus(page)
		b.click(page, r.ID)
	}
	b.remember(nativeID, ln)
	closeFn, err := b.show(opts, onClick)
	if err != nil {
		b.forget(nativeID)
		if err == mygo.ErrNotificationsDenied {
			log.Printf("mypack-runtime: notify: the system denied notifications for this app")
			return "denied", nil
		}
		return "unsupported", nil
	}
	ln.close = closeFn
	log.Printf("mypack-runtime: notify: shown id=%s origin=%s titleLen=%d bodyLen=%d", nativeID, origin, len([]rune(title)), len([]rune(body)))
	return "shown", nil
}

// CloseNotification removes a notification the page closed.
func (b *bridgeService) CloseNotification(ctx context.Context, jsID string) {
	b.mu.Lock()
	var found *liveNote
	for id, ln := range b.live {
		if ln.jsID == jsID {
			found = ln
			delete(b.live, id)
			break
		}
	}
	b.mu.Unlock()
	if found != nil && found.close != nil {
		found.close()
	}
}

// Permission returns the notification permission for the calling origin.
func (b *bridgeService) Permission(ctx context.Context) string {
	_, raw := b.caller(ctx)
	origin := originOf(raw)
	if !b.isApp(raw) || origin == "" {
		return "denied"
	}
	return b.state(origin)
}

// RequestPermission is Notification.requestPermission(): in "ask" mode a
// native prompt appears once per origin and the answer is remembered.
func (b *bridgeService) RequestPermission(ctx context.Context) string {
	page, raw := b.caller(ctx)
	origin := originOf(raw)
	if !b.isApp(raw) || origin == "" {
		return "denied"
	}
	if s := b.state(origin); s != "default" || b.mode != "ask" {
		return s
	}
	b.mu.Lock()
	if b.asked[origin] {
		b.mu.Unlock()
		return "default" // a prompt for this origin is already on screen
	}
	b.asked[origin] = true
	b.mu.Unlock()
	allowed := b.prompt(page, origin)
	b.mu.Lock()
	delete(b.asked, origin)
	b.mu.Unlock()
	if allowed {
		b.perms.Set(origin, "granted")
		return "granted"
	}
	b.perms.Set(origin, "denied")
	return "denied"
}

// SetBadge is navigator.setAppBadge / clearAppBadge: n > 0 count, 0 clear,
// n < 0 a flag without number.
func (b *bridgeService) SetBadge(ctx context.Context, n int) {
	if !b.badge {
		return
	}
	page, raw := b.caller(ctx)
	if !b.isApp(raw) {
		return
	}
	b.badges.setAPI(page, n)
}

// state is the effective permission of origin: "granted" | "denied" | "default".
func (b *bridgeService) state(origin string) string {
	switch b.mode {
	case "allow":
		return "granted"
	case "ask":
		if s := b.perms.Get(origin); s == "granted" || s == "denied" {
			return s
		}
		return "default"
	}
	return "denied"
}

func (b *bridgeService) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	cut := now.Add(-notifyWindow)
	keep := b.times[:0]
	for _, t := range b.times {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	b.times = keep
	if len(b.times) >= notifyBurst {
		return false
	}
	b.times = append(b.times, now)
	return true
}

func (b *bridgeService) remember(id string, ln *liveNote) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.live == nil {
		b.live = map[string]*liveNote{}
	}
	if _, ok := b.live[id]; !ok {
		b.order = append(b.order, id)
	}
	b.live[id] = ln
	for len(b.order) > maxLive {
		delete(b.live, b.order[0])
		b.order = b.order[1:]
	}
}

func (b *bridgeService) forget(id string) {
	b.mu.Lock()
	delete(b.live, id)
	b.mu.Unlock()
}

// ---- helpers -----------------------------------------------------------------

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != siteScheme) {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

// cleanText drops control characters and bounds the length (runes).
func cleanText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if rs := []rune(s); len(rs) > max {
		s = string(rs[:max-1]) + "…"
	}
	return s
}

// decodeDataImage returns the bytes of a PNG/JPEG data: URL, else nil.
func decodeDataImage(u string) []byte {
	const maxB64 = maxIconBytes * 4 / 3
	if !strings.HasPrefix(u, "data:image/") || len(u) > maxB64+64 {
		return nil
	}
	head, data, ok := strings.Cut(u, ",")
	if !ok || !strings.HasSuffix(head, ";base64") {
		return nil
	}
	if !strings.HasPrefix(head, "data:image/png") && !strings.HasPrefix(head, "data:image/jpeg") && !strings.HasPrefix(head, "data:image/jpg") {
		return nil
	}
	raw, err := decodeBase64(data)
	if err != nil || len(raw) > maxIconBytes {
		return nil
	}
	return raw
}

// ---- production wiring --------------------------------------------------------

// profilePerms stores the "ask" answers in the profile's state.json.
type profilePerms struct{ st *Store }

func (p profilePerms) Get(origin string) string { return p.st.Get().Notify[origin] }
func (p profilePerms) Set(origin, state string) {
	p.st.Update(func(s *State) {
		if s.Notify == nil {
			s.Notify = map[string]string{}
		}
		s.Notify[origin] = state
	})
}

func (a *App) newBridge() *bridgeService {
	n := a.cfg.Native
	b := &bridgeService{
		mode:   n.NotificationsMode(),
		badge:  n.BadgeMode() != "off",
		perms:  profilePerms{a.state},
		now:    time.Now,
		asked:  map[string]bool{},
		badges: a.badges,
	}
	b.caller = func(ctx context.Context) (*mygo.Page, string) {
		p := mygo.CallerPage(ctx)
		if p == nil {
			return nil, ""
		}
		return p, p.URL()
	}
	b.isApp = func(raw string) bool { return a.policy != nil && a.policy.IsAppURL(raw) }
	b.show = func(opts mygo.NotificationOptions, onClick func()) (func(), error) {
		if !mygo.NotificationsSupported() {
			return nil, fmt.Errorf("notifications are not supported here")
		}
		note := mygo.NewNotification(opts)
		note.OnClick(onClick)
		if err := note.Show(); err != nil {
			return nil, err
		}
		return note.Close, nil
	}
	b.prompt = func(page *mygo.Page, origin string) bool {
		var parent *mygo.Window
		if w, _ := a.windowOfPage(page); w != nil {
			parent = w
		}
		host := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent:        parent,
			Type:          mygo.MessageQuestion,
			Title:         a.title(),
			Message:       fmt.Sprintf("Allow %s to show notifications?", host),
			Detail:        "You can change this later in the app's settings in Mypack.",
			Buttons:       []string{"Allow", "Don't Allow"},
			DefaultButton: 0,
			CancelButton:  1,
		})
		return err == nil && res.Button == 0
	}
	b.focus = func(page *mygo.Page) { a.focusPage(page) }
	b.click = func(page *mygo.Page, jsID string) {
		if page == nil {
			return
		}
		js, _ := jsonString(jsID)
		if _, err := page.Eval("window.__mypackNotify&&window.__mypackNotify.click(" + js + ")"); err != nil {
			log.Printf("mypack-runtime: notify: click dispatch: %v", err)
		}
	}
	return b
}

// windowOfPage finds the app window showing page p.
func (a *App) windowOfPage(p *mygo.Page) (*mygo.Window, *mygo.Page) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for w, pg := range a.windows {
		if pg == p {
			return w, pg
		}
	}
	return nil, nil
}

// focusPage brings the window showing p to the front (the main window when
// the page is gone, e.g. a click after a restart).
func (a *App) focusPage(p *mygo.Page) {
	a.mu.Lock()
	tabs := a.tabs
	a.mu.Unlock()
	if tabs != nil && tabs.focusPage(p) {
		return
	}
	if a.group != nil && a.group.focusMember(p) {
		return
	}
	w, _ := a.windowOfPage(p)
	if w == nil || w.IsDestroyed() {
		a.showMain()
		return
	}
	mygo.RunOnMain(func() {
		if w.IsMinimized() {
			w.Restore()
		}
		w.Show()
		w.Focus()
	})
}
