package app

import (
	"testing"
	"time"
)

// The HUD shows only while Mypack's window is not in front, for a running
// or just-ended build; coming back hides it at once and counts as having
// seen a result; a result waits about 6 s; the setting turns it off.
func TestHUDVisibility(t *testing.T) {
	t0 := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	var m hudMem
	step := func(in hudIn, want bool, what string) {
		t.Helper()
		var got bool
		m, got = hudNext(m, in)
		if got != want {
			t.Fatalf("%s: show = %v, want %v (mem %+v)", what, got, want, m)
		}
	}
	run := func(front bool, at time.Duration) hudIn {
		return hudIn{enabled: true, front: front, job: "j1", running: true, now: t0.Add(at)}
	}
	ended := func(front bool, how string, at time.Duration) hudIn {
		return hudIn{enabled: true, front: front, job: "j1", ended: how, now: t0.Add(at)}
	}

	step(hudIn{enabled: true, front: false, now: t0}, false, "no build")
	step(run(true, 0), false, "building, window in front")
	step(run(false, time.Second), true, "building, window behind")
	step(run(true, 2*time.Second), false, "back to the window mid-build")
	step(run(false, 3*time.Second), true, "away again: shows at once")
	step(ended(false, "succeeded", 4*time.Second), true, "finished while away")
	step(ended(false, "succeeded", 8*time.Second), true, "result waits")
	step(ended(false, "succeeded", 11*time.Second), false, "result expires after ~6 s")

	m = hudMem{}
	step(run(false, 0), true, "second build, away")
	step(ended(false, "failed", time.Second), true, "failed while away")
	step(ended(true, "failed", 2*time.Second), false, "returning sees the failure")
	step(ended(false, "failed", 3*time.Second), false, "and it stays seen")

	m = hudMem{}
	step(ended(false, "cancelled", 0), false, "a cancelled build shows nothing")

	m = hudMem{}
	off := run(false, 0)
	off.enabled = false
	step(off, false, "setting off: menu bar only")
	step(hudIn{enabled: true, front: false, job: "j2", running: true, now: t0}, true, "a new build starts afresh")
}
