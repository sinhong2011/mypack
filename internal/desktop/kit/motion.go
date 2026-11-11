package kit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
)

// The motion engine: one place for how things arrive, leave, react and
// travel, so every component and screen moves alike. Springs and
// durations come from design's motion tokens; MyGo's animations already
// jump to their targets under Reduce Motion, and Reduced lets painted
// scenes do the same.

// Reduced reports whether the user asked for reduced motion.
func Reduced(c *ui.Context) bool { return c.Preferences().ReduceMotion }

// Stagger returns anim delayed for the i-th item of a group (reading
// order), each step after the one before, capped so long lists don't
// wait.
func Stagger(anim ui.Animation, i int, step time.Duration) ui.Animation {
	i = min(i, 12)
	anim.Delay = design.Scaled(time.Duration(i) * step)
	return anim
}

// Arrive makes e come in from a little below, fading up, and leave the
// same way: list rows, cards, sections. i staggers it among its siblings.
func Arrive(e ui.Element, i int, step time.Duration) ui.Element {
	a := Stagger(design.SpringSoft, i, step)
	return e.Transition(ui.ElementTransition{Animation: &a,
		Enter: &ui.Motion{Y: 10}, Exit: &ui.Motion{Scale: 0.92, Collapse: true}})
}

// Roll makes a changing label roll: the new text comes up from below as
// the old one leaves upward (put it in a clipped slot, keyed by its text).
func Roll(e ui.Element) ui.Element {
	return e.Transition(ui.ElementTransition{Animation: &design.SpringSoft,
		Enter: &ui.Motion{YPercent: 100}, Exit: &ui.Motion{YPercent: -100}})
}

// Pop makes e grow in from slightly smaller, as surfaces that open from
// a point do (menus, popovers, badges).
func Pop(e ui.Element) ui.Element {
	return e.Transition(ui.ElementTransition{Animation: &design.Spring,
		Enter: &ui.Motion{Scale: 0.92}, Exit: &ui.Motion{Scale: 0.96}})
}

// Reveal makes e open and close its own height, its content fading, as
// disclosures and inserted rows do.
func Reveal(e ui.Element) ui.Element {
	return e.Transition(ui.ElementTransition{Animation: &design.SpringSoft,
		Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true}})
}

// PageSwap moves between pages (key the element by the page): the page
// going fades, slides 12 pt against the way of travel and settles back to
// .985; the one coming slides in from the way of travel. dir is +1 going
// deeper or down the sidebar, -1 coming back. An interruptible spring of
// about 280 ms; the page's blocks stagger in after it (Block).
func PageSwap(e ui.Element, dir float32) ui.Element {
	return e.Transition(ui.ElementTransition{Animation: &design.SpringPage,
		Enter: &ui.Motion{X: 28 * dir}, Exit: &ui.Motion{X: -12 * dir, Scale: 0.985}})
}

// Block staggers the i-th block of a page in after the page itself:
// header first, then each content block, rising 10 pt.
func Block(e ui.Element, i int) ui.Element {
	a := Stagger(design.SpringPage, i+1, design.StaggerBlock)
	return e.Transition(ui.ElementTransition{Animation: &a, Enter: &ui.Motion{Y: 10}})
}

// modalAt is when a sheet, palette or popover was last open.
var modalAt time.Time

// MarkModal notes that a modal surface is open this frame; the canvas
// behind it blurs in (ModalOpen).
func MarkModal() { modalAt = time.Now() }

// ModalOpen reports whether a modal surface was open a moment ago.
func ModalOpen() bool { return time.Since(modalAt) < 120*time.Millisecond }

// Press gives an element the shared hover/press feedback: a lift of lift
// DIPs while hovered and a slight sink while pressed.
func Press(e ui.Element, hovered, pressed bool, lift float32) {
	pose := ui.Pose{}
	switch {
	case pressed:
		pose.Scale = 0.98
	case hovered:
		pose.Y = -lift
	}
	e.AnimatePose(pose, design.Spring)
}

// Lift raises a card while hovered and sinks it while pressed.
func Lift(e ui.Element, hovered, pressed bool) { Press(e, hovered, pressed, 2) }

// Glide is a shared-element match: an indicator built inside whichever
// item is current, with the same Glide, slides from the old item to the
// new one instead of jumping.
func Glide(e ui.Element, group any) ui.Element {
	return e.Transition(ui.ElementTransition{Animation: &design.Spring, Match: group, Colors: true})
}

// ---- Pure motion math (scenes, tests) ----------------------------------------

// Clamp01 limits t to 0..1.
func Clamp01(t float64) float64 { return math.Max(0, math.Min(1, t)) }

// Seg returns how far t is through the segment a..b, as 0..1.
func Seg(t, a, b float64) float64 {
	if b <= a {
		if t >= b {
			return 1
		}
		return 0
	}
	return Clamp01((t - a) / (b - a))
}

// Lerp mixes a and b by t.
func Lerp(a, b, t float64) float64 { return a + (b-a)*t }

// EaseOutCubic decelerates.
func EaseOutCubic(t float64) float64 { t = Clamp01(t); return 1 - math.Pow(1-t, 3) }

// EaseInOutCubic accelerates then decelerates.
func EaseInOutCubic(t float64) float64 {
	t = Clamp01(t)
	if t < 0.5 {
		return 4 * t * t * t
	}
	return 1 - math.Pow(-2*t+2, 3)/2
}

// EaseOutBack overshoots a little before settling (bounce 0..1).
func EaseOutBack(t, bounce float64) float64 {
	t = Clamp01(t)
	c1 := 1.70158 * bounce
	c3 := c1 + 1
	return 1 + c3*math.Pow(t-1, 3) + c1*math.Pow(t-1, 2)
}

// SpringAt is the position of an underdamped spring released from 0
// toward 1 after t seconds, settling in about dur seconds with the given
// bounce (0 = critically damped), as SwiftUI's perceptual springs.
func SpringAt(t, dur, bounce float64) float64 {
	if t <= 0 {
		return 0
	}
	if dur <= 0 {
		return 1
	}
	omega := 2 * math.Pi / dur
	if bounce <= 0 {
		// Critically damped.
		return 1 - (1+omega*t)*math.Exp(-omega*t)
	}
	zeta := 1 - bounce
	wd := omega * math.Sqrt(1-zeta*zeta)
	e := math.Exp(-zeta * omega * t)
	return 1 - e*(math.Cos(wd*t)+zeta*omega/wd*math.Sin(wd*t))
}

// Shake is a decaying horizontal shake for invalid input, t in seconds
// since it started, in DIPs.
func Shake(t float64) float64 {
	if t < 0 || t > 0.45 {
		return 0
	}
	return 7 * math.Sin(t*2*math.Pi*7) * (1 - t/0.45)
}
