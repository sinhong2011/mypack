package design

import (
	"github.com/egoist/mygo/ui"
)

// Surfaces. mypack is flat and modern: flat fills, 1 px hairlines, colour
// tints and motion. No grain, no bevels, no inner highlights, no pressed-in
// wells. The only shadows are one soft ambient level for things that float
// (menus, sheets, ⌘K, toasts; Elevate) and a very soft one under app icons.

// Tint fills e flat with the middle of top and bot (surfaces used to carry
// a vertical tint; they are flat now).
func Tint(e ui.Element, top, bot ui.Color) ui.Element {
	return e.Background(top.Mix(bot, 0.5))
}

// Well is a quiet inset surface (logs, code, path chips, tracks): a flat
// subtle fill and a hairline.
func Well(c *ui.Context, e ui.Element) ui.Element {
	p := Of(c)
	return e.Background(p.Sunken).Border(1, p.Border)
}

// RaisedControl is kept for callers; controls are flat, so it adds nothing.
func RaisedControl(c *ui.Context, e ui.Element) ui.Element { return e }

// Float sets the canvas apart from the chrome with a hairline only.
func Float(c *ui.Context, e ui.Element) ui.Element { return e }
