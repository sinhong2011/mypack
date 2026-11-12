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

// Arrive lets e leave by folding its own space away (a removed row,
// tile or card), so its neighbours close the gap instead of jumping. It
// comes in at once: content showing up needs no announcement.
func Arrive(e ui.Element) ui.Element {
	return e.Transition(ui.ElementTransition{Animation: &design.SpringSoft,
		Exit: &ui.Motion{Scale: 0.96, Collapse: true}})
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

// PageSwap is the change between pages (key the element by the page): the
// new page fades in quickly and the old one is gone at once, so nothing
// slides and two pages never overlap.
func PageSwap(e ui.Element) ui.Element {
	fade := design.Tween(design.MotionFast, design.EaseStd)
	return e.Transition(ui.ElementTransition{Animation: &fade, Enter: &ui.Motion{}})
}

// modalAt is when a sheet, palette or popover was last open.
var modalAt time.Time

// MarkModal notes that a modal surface is open this frame; the canvas
// behind it blurs in (ModalOpen).
func MarkModal() { modalAt = time.Now() }

var typingAt time.Time

// MarkTyping records that a text field has the keyboard focus.
func MarkTyping() { typingAt = time.Now() }

// Typing reports whether a text field had the focus a moment ago: Return
// then belongs to the field, and window-wide Return shortcuts (Open,
// Retry) must leave it alone.
func Typing() bool { return time.Since(typingAt) < 150*time.Millisecond }

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
