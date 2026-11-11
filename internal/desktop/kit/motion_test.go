package kit

import (
	"math"
	"testing"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestSegAndClamp(t *testing.T) {
	cases := []struct{ t, a, b, want float64 }{
		{-1, 0, 1, 0}, {0.5, 0, 1, 0.5}, {2, 0, 1, 1}, {0.75, 0.5, 1, 0.5}, {1, 1, 1, 1}, {0.9, 1, 1, 0},
	}
	for _, c := range cases {
		if got := Seg(c.t, c.a, c.b); !near(got, c.want, 1e-9) {
			t.Errorf("Seg(%v,%v,%v) = %v, want %v", c.t, c.a, c.b, got, c.want)
		}
	}
}

func TestEasingsHitTheirEnds(t *testing.T) {
	for name, f := range map[string]func(float64) float64{
		"outCubic": EaseOutCubic, "inOutCubic": EaseInOutCubic, "outBack": func(x float64) float64 { return EaseOutBack(x, 1) },
	} {
		if !near(f(0), 0, 1e-9) || !near(f(1), 1, 1e-9) {
			t.Errorf("%s: f(0)=%v f(1)=%v, want 0 and 1", name, f(0), f(1))
		}
	}
	// Back overshoots past 1 on the way.
	over := 0.0
	for x := 0.0; x <= 1; x += 0.01 {
		over = math.Max(over, EaseOutBack(x, 1))
	}
	if over <= 1 {
		t.Errorf("EaseOutBack never overshoots (max %v)", over)
	}
}

func TestSpringSettles(t *testing.T) {
	for _, bounce := range []float64{0, 0.2, 0.4} {
		if v := SpringAt(0, 0.5, bounce); v != 0 {
			t.Errorf("bounce %v: starts at %v", bounce, v)
		}
		if v := SpringAt(2.5, 0.5, bounce); !near(v, 1, 0.01) {
			t.Errorf("bounce %v: at 5 durations it is %v, want ≈1", bounce, v)
		}
	}
	// A critically damped spring never overshoots; a bouncy one does.
	maxC, maxB := 0.0, 0.0
	for x := 0.0; x < 2; x += 0.005 {
		maxC = math.Max(maxC, SpringAt(x, 0.5, 0))
		maxB = math.Max(maxB, SpringAt(x, 0.5, 0.4))
	}
	if maxC > 1+1e-6 || maxB <= 1.01 {
		t.Errorf("overshoot: critical %v (want ≤1), bouncy %v (want >1)", maxC, maxB)
	}
}

func TestShakeDecaysToRest(t *testing.T) {
	if Shake(-0.1) != 0 || Shake(0.5) != 0 {
		t.Error("shake moves outside its 0.45 s window")
	}
	if math.Abs(Shake(0.03)) <= math.Abs(Shake(0.4)) {
		t.Error("shake does not decay")
	}
}
