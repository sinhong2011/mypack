//go:build !mypack_lite

package app

import (
	"math"
	"testing"
)

func near32(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

func TestGlanceFrameEnds(t *testing.T) {
	from := glanceRect{100, 500, 80, 20}
	to := glanceCard(1200, 800, 0, 0.8)
	if got := glanceFrame(from, to, 0, 800); got != from {
		t.Fatalf("at 0 the card is the link: %+v", got)
	}
	got := glanceFrame(from, to, 1, 800)
	if !near32(got.X, to.X) || !near32(got.Y, to.Y) || !near32(got.W, to.W) || !near32(got.H, to.H) {
		t.Fatalf("at 1 the card rests: %+v, want %+v", got, to)
	}
}

// The centre leaves the straight line by Zen's arc: at most 30 points, 40%
// of the distance, toward the side with more room.
func TestGlanceFrameArc(t *testing.T) {
	to := glanceCard(1200, 800, 0, 0.8)
	ex, ey := to.center()
	// A link high up: more room below, so the arc bends down.
	from := glanceRect{100, 40, 80, 20}
	sx, sy := from.center()
	mid := glanceFrame(from, to, 0.5, 800)
	mx, my := mid.center()
	if !near32(mx, (sx+ex)/2) {
		t.Fatalf("x goes straight: %v", mx)
	}
	if off := my - (sy+ey)/2; !near32(off, glanceArcMax) {
		t.Fatalf("the arc's height mid-way is %v, want %v down", off, float32(glanceArcMax))
	}
	// A link low down bends up.
	from = glanceRect{100, 760, 80, 20}
	_, sy = from.center()
	_, my = glanceFrame(from, to, 0.5, 800).center()
	if off := my - (sy+ey)/2; off >= 0 {
		t.Fatalf("a link low down arcs up, got %v", off)
	}
	// A short way arcs little: 40% of the distance.
	near := glanceRect{to.X + to.W/2 - 40 + 10, to.Y + to.H/2 - 10, 80, 20}
	nx, ny := near.center()
	_, my = glanceFrame(near, to, 0.5, 800).center()
	d := float32(math.Hypot(float64(ex-nx), float64(ey-ny)))
	if off := my - (ny+ey)/2; math.Abs(float64(off)) > float64(d*glanceArcRatio)+0.01 {
		t.Fatalf("arc %v exceeds 40%% of %v", off, d)
	}
}

func TestGlanceCard(t *testing.T) {
	c := glanceCard(1200, 800, 0, 0.8)
	if !near32(c.W, 960) || !near32(c.X, 120) || !near32(c.Y, glanceMargin) || !near32(c.H, 800-2*glanceMargin) {
		t.Fatalf("card in 1200x800: %+v", c)
	}
	c = glanceCard(1200, 800, 38, 0.8)
	if !near32(c.Y, 38+glanceMargin) {
		t.Fatalf("the card starts under a strip over the page: %+v", c)
	}
	c = glanceCard(300, 600, 0, 0.8)
	if c.W <= 0 || c.X < 0 || c.X+c.W > 300 {
		t.Fatalf("a narrow window keeps the card inside: %+v", c)
	}
}

func TestGlanceOut(t *testing.T) {
	if glanceOut(0) != 0 || glanceOut(1) != 1 || glanceOut(0.25) < 0.8 {
		t.Fatalf("ease-out 1-(1-t)^6: %v %v %v", glanceOut(0), glanceOut(0.25), glanceOut(1))
	}
}

func TestGlanceCardWidth(t *testing.T) {
	if c := glanceCard(1200, 800, 0, 0.6); !near32(c.W, 720) || !near32(c.X, 240) {
		t.Fatalf("60%% card: %+v", c)
	}
	if c := glanceCard(1200, 800, 0, 0.95); !near32(c.W, 1140) {
		t.Fatalf("95%% card: %+v", c)
	}
}

func TestGlanceMotion(t *testing.T) {
	if m := motionOf("smooth"); m.d.Milliseconds() != 350 || m.bounce != 0.2 || m.off {
		t.Fatalf("smooth is Zen's: %+v", m)
	}
	if motionOf("quick").d >= motionOf("smooth").d || motionOf("relaxed").d <= motionOf("smooth").d {
		t.Fatal("quick is shorter and relaxed longer than smooth")
	}
	if !motionOf("off").off {
		t.Fatal("off has no motion")
	}
}
