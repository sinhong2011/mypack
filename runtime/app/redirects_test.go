package app

import (
	"testing"
	"time"
)

func TestRedirectTracker(t *testing.T) {
	clock := time.Unix(0, 0)
	r := newRedirectTracker()
	r.now = func() time.Time { return clock }

	// A click is the user's.
	if !r.userInitiated(true) {
		t.Fatal("first click: want user-initiated")
	}
	r.allowed(true)
	// WebView2 reports the server's redirect (notion.so → notion.com)
	// as user-initiated too: it is not.
	if r.userInitiated(true) {
		t.Fatal("redirect before commit: want not user-initiated")
	}
	r.allowed(true)
	if r.userInitiated(true) {
		t.Fatal("second redirect hop: want not user-initiated")
	}
	// The page commits: the next click is the user's again.
	r.settled()
	if !r.userInitiated(true) {
		t.Fatal("click after commit: want user-initiated")
	}
	r.allowed(true)
	// A stuck chain does not swallow clicks for long.
	clock = clock.Add(redirectWindow + time.Second)
	if !r.userInitiated(true) {
		t.Fatal("click after the window: want user-initiated")
	}
	// Script navigations stay script navigations.
	r.settled()
	if r.userInitiated(false) {
		t.Fatal("script navigation: want not user-initiated")
	}
	r.allowed(false)
	if !r.userInitiated(true) {
		t.Fatal("script navigation must not start a chain")
	}
}
