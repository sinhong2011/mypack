package app

import "testing"

func TestCompactRect(t *testing.T) {
	work := Bounds{X: 0, Y: 25, W: 1440, H: 875}
	// First time: bottom-right corner, inside the margin.
	got := compactRect(nil, work, 400, 640)
	want := Bounds{X: 1440 - 400 - compactMargin, Y: 25 + 875 - 640 - compactMargin, W: 400, H: 640}
	if got != want {
		t.Fatalf("first = %+v, want %+v", got, want)
	}
	// Where the user left it.
	saved := &Bounds{X: 100, Y: 100, W: 360, H: 500}
	if got := compactRect(saved, work, 400, 640); got != *saved {
		t.Fatalf("saved = %+v", got)
	}
	// Half off screen: clamped back in.
	if got := compactRect(&Bounds{X: 1200, Y: 100, W: 400, H: 500}, work, 400, 640); got.X != 1040 || got.W != 400 {
		t.Fatalf("clamped = %+v", got)
	}
	// On a display that is gone: back to the corner.
	if got := compactRect(&Bounds{X: 3000, Y: 100, W: 400, H: 500}, work, 400, 640); got != want {
		t.Fatalf("gone display = %+v", got)
	}
	// Too small to be useful: back to the corner.
	if got := compactRect(&Bounds{X: 10, Y: 30, W: 100, H: 100}, work, 400, 640); got != want {
		t.Fatalf("tiny = %+v", got)
	}
	// Panel taller than a small work area: shrunk to fit.
	small := Bounds{X: 0, Y: 0, W: 800, H: 500}
	if got := compactRect(nil, small, 400, 640); got.H != 500 || got.Y != 0 {
		t.Fatalf("small = %+v", got)
	}
	// No work area known.
	if got := compactRect(nil, Bounds{}, 400, 640); got.W != 400 || got.H != 640 {
		t.Fatalf("no work area = %+v", got)
	}
}

func TestSidebarRect(t *testing.T) {
	work := Bounds{X: 0, Y: 25, W: 1440, H: 875}
	if got, want := sidebarRect(work, "right", 400), (Bounds{X: 1040, Y: 25, W: 400, H: 875}); got != want {
		t.Fatalf("right = %+v", got)
	}
	if got, want := sidebarRect(work, "left", 400), (Bounds{X: 0, Y: 25, W: 400, H: 875}); got != want {
		t.Fatalf("left = %+v", got)
	}
	if got := sidebarRect(work, "right", 100); got.W != minPanelSide || got.X != 1440-minPanelSide {
		t.Fatalf("too narrow = %+v", got)
	}
	if got := sidebarRect(work, "right", 5000); got.W != 1440-minPageBesides {
		t.Fatalf("too wide = %+v", got)
	}
	// A second display to the left of the primary.
	left := Bounds{X: -1920, Y: 0, W: 1920, H: 1050}
	if got := sidebarRect(left, "right", 420); got.X != -420 || got.H != 1050 {
		t.Fatalf("second display = %+v", got)
	}
}
