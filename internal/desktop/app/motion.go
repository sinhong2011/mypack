package app

import "github.com/egoist/mygo/ui"

// Screen-level motion: the page scroll, reset on each page change, that
// collapses the toolbar.

// ---- page travel ----------------------------------------------------------------------

// pageScroll is the current page's scroll (reset on every page change).
var pageScroll ui.ScrollState

// travel resets the page scroll when the page changes to s.
func (a *App) travel(s section) {
	if s == a.section {
		return
	}
	pageScroll = ui.ScrollState{}
}

// scrollCollapse is how far the toolbar has collapsed: 0 at the top of
// the page, 1 once 44 pt have scrolled under it.
func scrollCollapse() float32 {
	return min(max(pageScroll.Y/44, 0), 1)
}
