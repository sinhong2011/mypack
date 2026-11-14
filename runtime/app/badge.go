package app

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// Title heuristics (native.badge "auto"): many web apps show their unread
// count in document.title. Recognized shapes:
//
//	"(3) Inbox"            leading count        (WhatsApp, Discord, Slack, X ...)
//	"[3] Inbox"            leading count in brackets
//	"Inbox (12)"           trailing count
//	"Inbox (3) - me@x.com - Gmail"   count after the first label
//	"• Slack", "* general - Slack"   leading dot: unread, no number
//
// Favicon-only badges are out of scope.
var (
	reLeadCount  = regexp.MustCompile(`^\s*[(\[](\d{1,6})(\+?)[)\]]\s*\S`)
	reTrailCount = regexp.MustCompile(`\s[(\[](\d{1,6})(\+?)[)\]]\s*$`)
	reMidCount   = regexp.MustCompile(`^[^()\[\]\-–—|]{1,40}?\s\((\d{1,6})(\+?)\)\s+[-–—|·]\s`)
	reLeadDot    = regexp.MustCompile(`^\s*[•●◉*]\s+\S`)
)

// ParseTitleBadge extracts an unread count (or an unread dot) from a page
// title. count is 0 and dot false when the title carries no badge.
func ParseTitleBadge(title string) (count int, dot bool) {
	title = strings.TrimSpace(title)
	if title == "" {
		return 0, false
	}
	for _, re := range []*regexp.Regexp{reLeadCount, reTrailCount, reMidCount} {
		if m := re.FindStringSubmatch(title); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil || (n >= 1900 && n <= 2100 && len(m[1]) == 4 && m[2] == "") {
				return 0, false // a year, not an unread count
			}
			return n, false
		}
	}
	if reLeadDot.MatchString(title) {
		return 0, true
	}
	return 0, false
}

// badgeLabel renders the Dock/taskbar text: "" clears.
func badgeLabel(count int, dot bool) string {
	switch {
	case count > 999:
		return "999+"
	case count > 0:
		return strconv.Itoa(count)
	case dot:
		return "•"
	}
	return ""
}

type badgeVal struct {
	n   int
	dot bool
}

// badgeController merges the badge values of all app windows (Badging API
// first, then the title heuristic), and applies the result to the native
// badge no more often than minGap.
type badgeController struct {
	auto bool // title heuristic enabled (native.badge "auto")
	sum  bool // add the pages' counts (a group's sites) instead of taking the largest
	set  func(label string)
	// changed, when set, hears that a page's value changed (a group's rail).
	changed func()
	minGap  time.Duration
	now     func() time.Time
	after   func(d time.Duration, f func()) (stop func())

	mu    sync.Mutex
	api   map[*mygo.Page]badgeVal
	title map[*mygo.Page]badgeVal
	shown string
	last  time.Time
	stop  func()
}

func (c *badgeController) setAuto(on bool) {
	c.auto = on
	if !on {
		c.mu.Lock()
		c.applyLocked("")
		c.mu.Unlock()
		return
	}
	c.refresh()
}

func newBadgeController(auto bool, set func(string)) *badgeController {
	return &badgeController{
		auto: auto, set: set, minGap: 400 * time.Millisecond, now: time.Now,
		after: func(d time.Duration, f func()) func() {
			t := time.AfterFunc(d, f)
			return func() { t.Stop() }
		},
		api: map[*mygo.Page]badgeVal{}, title: map[*mygo.Page]badgeVal{},
	}
}

// setAPI records navigator.setAppBadge: n > 0 count, 0 clear, n < 0 flag.
func (c *badgeController) setAPI(p *mygo.Page, n int) {
	c.mu.Lock()
	switch {
	case n == 0:
		delete(c.api, p)
	case n < 0:
		c.api[p] = badgeVal{dot: true}
	default:
		c.api[p] = badgeVal{n: n}
	}
	c.mu.Unlock()
	c.refresh()
}

// setTitle feeds the title heuristic (ignored for pages that use the API).
func (c *badgeController) setTitle(p *mygo.Page, title string) {
	if !c.auto {
		return
	}
	n, dot := ParseTitleBadge(title)
	c.mu.Lock()
	if n == 0 && !dot {
		delete(c.title, p)
	} else {
		c.title[p] = badgeVal{n: n, dot: dot}
	}
	c.mu.Unlock()
	c.refresh()
}

// drop forgets a closed window's page.
func (c *badgeController) drop(p *mygo.Page) {
	c.mu.Lock()
	delete(c.api, p)
	delete(c.title, p)
	c.mu.Unlock()
	c.refresh()
}

// pageLabel is the badge of one page: "3", "•" or "".
func (c *badgeController) pageLabel(p *mygo.Page) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.api[p]
	if !ok {
		v = c.title[p]
	}
	return badgeLabel(v.n, v.dot)
}

// compute merges the pages (caller holds mu): the largest count wins (the
// sum in a group); a flag shows only when no page has a count.
func (c *badgeController) compute() string {
	pages := map[*mygo.Page]badgeVal{}
	for p, v := range c.title {
		pages[p] = v
	}
	for p, v := range c.api { // the API beats the title of the same page
		pages[p] = v
	}
	count, dot := 0, false
	for _, v := range pages {
		if c.sum {
			count += v.n
		} else if v.n > count {
			count = v.n
		}
		dot = dot || v.dot
	}
	return badgeLabel(count, dot)
}

// refresh applies the merged badge now, or after the throttle gap.
func (c *badgeController) refresh() {
	if c.changed != nil {
		c.changed()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	label := c.compute()
	if label == c.shown {
		if c.stop != nil { // a stale trailing update is pointless now
			c.stop()
			c.stop = nil
		}
		return
	}
	wait := c.minGap - c.now().Sub(c.last)
	if wait <= 0 {
		c.applyLocked(label)
		return
	}
	if c.stop != nil {
		return // the trailing update recomputes
	}
	c.stop = c.after(wait, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.stop = nil
		if l := c.compute(); l != c.shown {
			c.applyLocked(l)
		}
	})
}

func (c *badgeController) applyLocked(label string) {
	c.shown, c.last = label, c.now()
	c.set(label)
}
