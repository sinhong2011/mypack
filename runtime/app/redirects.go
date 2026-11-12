package app

import (
	"sync"
	"time"
)

// redirectWindow bounds how long after a user's click a further
// "user-initiated" navigation still counts as that click's redirect.
const redirectWindow = 10 * time.Second

// redirectTracker tells a page's server redirects apart from the user's
// clicks. WebView2 (Windows) reports every hop of a redirect chain with
// the IsUserInitiated flag of the click that started it, so a click on
// notion.so/login that the server sends on to notion.com/login looks
// like the user following a link off-site, and would open in the default
// browser. macOS WebKit reports the redirect as not user-initiated.
//
// A user-initiated navigation the app lets through starts a chain; a
// navigation that arrives before the page commits (OnDidNavigate) or
// fails is a redirect of it, not a new click.
type redirectTracker struct {
	mu      sync.Mutex
	pending bool
	since   time.Time
	now     func() time.Time
}

func newRedirectTracker() *redirectTracker { return &redirectTracker{now: time.Now} }

// userInitiated returns whether a navigation reported with gesture
// really comes from the user, rather than a redirect of their last click.
func (r *redirectTracker) userInitiated(gesture bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !gesture {
		return false
	}
	return !(r.pending && r.now().Sub(r.since) < redirectWindow)
}

// allowed records that a navigation (user-initiated or not) was let
// through: a user-initiated one starts a chain, one continuing a chain
// keeps it open.
func (r *redirectTracker) allowed(gesture bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if gesture && !r.pending {
		r.pending, r.since = true, r.now()
	}
}

// settled ends the chain: the page committed, failed, or the navigation
// was cancelled.
func (r *redirectTracker) settled() {
	r.mu.Lock()
	r.pending = false
	r.mu.Unlock()
}
