package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
)

// Screen-level motion: which way a page change travels, the app icon
// that flies between screens, and the scroll that collapses the toolbar.

// ---- shared icon flights -------------------------------------------------------------

// A flight carries an app's icon from where it was clicked (a Library
// tile, a sidebar row, a ⌘K row, Create's preview, the project hero) to
// where it lands on the next screen, over everything, on a spring. The
// source notes its window rect as it is chosen (flyFrom); the
// destination, once laid out, takes it (flyTo) and stays hidden until the
// icon arrives.
type flight struct {
	png        []byte
	name       string
	from, to   ui.Rect
	noted      time.Time // when the source was chosen
	start      time.Time // when the destination was found
	toRadius   float32
	fromRadius float32
}

// iconRects remembers where each app's icon showed last (window DIPs),
// by flight key, for sources that are left by a page change.
type iconRects map[string]ui.Rect

// flyFrom starts a flight to key from r, the icon's rect on screen.
func (a *App) flyFrom(key string, r ui.Rect, png []byte, name string) {
	if r.W <= 0 || a.reduced {
		return
	}
	if a.flights == nil {
		a.flights = map[string]*flight{}
	}
	a.flights[key] = &flight{png: png, name: name, from: r, noted: time.Now(), fromRadius: design.AppIconRadius(r.W)}
}

// flyTo makes e (an icon) the destination of a flight to key: hidden
// while the icon flies in, it shows once it lands.
func (a *App) flyTo(c *ui.Context, key string, e ui.Element) ui.Element {
	f := a.flights[key]
	if f == nil {
		return e
	}
	r := e.Bounds()
	if r.W <= 0 {
		// Laid out next frame.
		if time.Since(f.noted) > design.Scaled(800*time.Millisecond) {
			delete(a.flights, key)
			return e
		}
		c.Invalidate()
		return e.Opacity(0)
	}
	f.to, f.toRadius = r, design.AppIconRadius(r.W)
	if f.start.IsZero() {
		f.start = time.Now()
	}
	return e.Opacity(0)
}

// flightsView draws the icons on their way, over the window.
func (a *App) flightsView(c *ui.Context) {
	for key, f := range a.flights {
		if f.start.IsZero() {
			continue
		}
		t := time.Since(f.start).Seconds() * float64(design.Speed())
		const dur = 0.5
		if t >= dur*1.25 {
			delete(a.flights, key)
			c.Invalidate()
			continue
		}
		k := float32(kit.SpringAt(t, dur, 0.14))
		lerp := func(x, y float32) float32 { return x + (y-x)*k }
		r := ui.Rect{X: lerp(f.from.X, f.to.X), Y: lerp(f.from.Y, f.to.Y), W: lerp(f.from.W, f.to.W), H: lerp(f.from.H, f.to.H)}
		// It arcs a little on its way, as a thing thrown does.
		r.Y -= 24 * float32(4*float64(k)*(1-float64(k)))
		ui.Box(c.Key("flight:"+key)).Absolute().Left(r.X).Top(r.Y).Size(r.W, r.H).PassThrough().AccessHidden(true).ZIndex(50).
			Children(func() { kit.AppIconOpts(c, f.png, f.name, r.W, kit.IconOpts{Shadow: true}) })
		c.Invalidate()
	}
}

// Flight keys: an app's icon in general, and the card of its build scene.
func iconKey(projectID string) string { return "icon:" + projectID }
func cardKey(projectID string) string { return "card:" + projectID }

// ---- page travel ----------------------------------------------------------------------

// pageScroll is the current page's scroll (reset on every page change).
var pageScroll ui.ScrollState

// sectionRank orders sections for the direction of travel.
func sectionRank(s section) int {
	switch s {
	case secLibrary:
		return 0
	case secDiscover:
		return 1
	case secBuilds:
		return 2
	case secCreate:
		return 5
	case secProject:
		return 6
	case secRuntimes:
		return 8
	case secSettings:
		return 9
	}
	return 10
}

// travel sets the direction of the page change from a.section to s, and
// sends the project hero's icon home when leaving a project.
func (a *App) travel(s section) {
	if s == a.section {
		return
	}
	if sectionRank(s) >= sectionRank(a.section) {
		a.navDir = 1
	} else {
		a.navDir = -1
	}
	pageScroll = ui.ScrollState{}
	if a.section == secProject && a.projectID != "" {
		if pr := a.project(a.projectID); pr != nil {
			a.flyFrom(iconKey(pr.ID), a.iconAt[iconKey(pr.ID)], pr.IconPNG, pr.Config.App.Name)
		}
	}
}

// scrollCollapse is how far the toolbar has collapsed: 0 at the top of
// the page, 1 once 44 pt have scrolled under it.
func scrollCollapse() float32 {
	return min(max(pageScroll.Y/44, 0), 1)
}
