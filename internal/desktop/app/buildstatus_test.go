package app

import (
	"testing"
	"time"
)

// The status bar's progress never goes back, and detail updates never
// restart (or skip) the title's roll: a new title waits for the running
// roll to finish, and only the latest waiting title is shown next.
func TestBuildStatusProgressAndTitle(t *testing.T) {
	t0 := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	var st statusState
	st.advance("Getting the app engine", "1.0 of 9.8 MB · 10%", 0.30, true, t0)
	for i, f := range []float64{0.35, 0.20, 0.40, 0.05, 0.38} {
		st.advance("Getting the app engine", "detail", f, true, t0.Add(time.Duration(i+1)*200*time.Millisecond))
	}
	if st.progress != 0.40 {
		t.Fatalf("progress = %v, want the furthest 0.40", st.progress)
	}
	// Details alone never change the title or its start.
	at := st.titleAt
	for i := 0; i < 20; i++ {
		st.advance("Getting the app engine", "d"+string(rune('a'+i)), 0.4, true, t0.Add(2*time.Second+time.Duration(i)*130*time.Millisecond))
	}
	if st.title != "Getting the app engine" || st.titleAt != at {
		t.Fatalf("a detail restarted the title: %q at %v", st.title, st.titleAt)
	}
	// Two stage changes inside one roll: the second waits, then shows.
	now := t0.Add(10 * time.Second)
	st.advance("Making it an app", "Step 3 of 5", 0.6, false, now)
	if st.title != "Making it an app" {
		t.Fatalf("title = %q", st.title)
	}
	st.advance("Signing", "Step 4 of 5", 0.7, false, now.Add(100*time.Millisecond))
	if st.title != "Making it an app" || st.pending != "Signing" {
		t.Fatalf("mid-roll: title %q pending %q", st.title, st.pending)
	}
	st.advance("Checking everything", "Step 5 of 5", 0.8, false, now.Add(200*time.Millisecond))
	if st.pending != "Checking everything" {
		t.Fatalf("pending = %q, want the latest", st.pending)
	}
	st.advance("Checking everything", "Step 5 of 5", 0.8, false, now.Add(titleRoll+10*time.Millisecond))
	if st.title != "Checking everything" || st.pending != "" {
		t.Fatalf("after the roll: %q pending %q", st.title, st.pending)
	}
	// Details are throttled to 8 a second.
	st.advance("Checking everything", "x1", 0.8, false, now.Add(time.Second))
	st.advance("Checking everything", "x2", 0.8, false, now.Add(time.Second+50*time.Millisecond))
	if st.detail != "x1" {
		t.Fatalf("detail = %q, want x1 (throttled)", st.detail)
	}
}
