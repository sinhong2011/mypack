package kit

import (
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
)

// Window controls drawn by the view, for Windows and Linux, where the
// system's caption buttons (46×32, flush with the window's corner) would
// sit across the canvas's rounded edge. Ours live in the toolbar, centred
// on its line with the other pills: quiet glyphs whose hover is the same
// rounded wash as a toolbar icon's, close turning red as Windows' does.

// WindowControls are the window's state and actions.
type WindowControls struct {
	// Order is the buttons, left to right: "minimize", "maximize",
	// "close" (the desktop's order, mygo.Theme.WindowButtons).
	Order     []string
	Maximized bool
	Active    bool // the window is focused; inactive glyphs dim
	Minimize  func()
	Maximize  func() // toggles maximize and restore
	Close     func()
	// Labels for assistive tech and tooltips.
	MinimizeLabel, MaximizeLabel, RestoreLabel, CloseLabel string
}

// Caption glyphs after Windows 11's (10 pt, hairline), on a 10×10 grid.
func captionGlyph(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	glyphMinimize = captionGlyph(`<path d="M0.5 5h9"/>`)
	glyphMaximize = captionGlyph(`<rect x="0.5" y="0.5" width="9" height="9" rx="1.5"/>`)
	glyphRestore  = captionGlyph(`<rect x="0.5" y="2.5" width="7" height="7" rx="1.25"/><path d="M2.5 0.5h5.5a1.5 1.5 0 0 1 1.5 1.5v5.5"/>`)
	glyphClose    = captionGlyph(`<path d="M0.5 0.5l9 9M9.5 0.5l-9 9"/>`)
)

const (
	winButtonW = 40
	winButtonH = tb
)

// WindowButtons draws the window controls as one cluster; it reports
// nothing, as the buttons act through w's functions.
func WindowButtons(c *ui.Context, w WindowControls) {
	if len(w.Order) == 0 {
		return
	}
	ui.Row(c.Key("winbtns")).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
		for _, name := range w.Order {
			windowButton(c, name, w)
		}
	})
}

func windowButton(c *ui.Context, name string, w WindowControls) {
	p := design.Of(c)
	glyph, label, act := glyphClose, w.CloseLabel, w.Close
	switch name {
	case "minimize":
		glyph, label, act = glyphMinimize, w.MinimizeLabel, w.Minimize
	case "maximize":
		glyph, label, act = glyphMaximize, w.MaximizeLabel, w.Maximize
		if w.Maximized {
			glyph, label = glyphRestore, w.RestoreLabel
		}
	case "close":
	default:
		return
	}
	b := ui.ButtonBase(c.Key("win-"+name)).Label(label).Size(winButtonW, winButtonH).Radius(PillRadius(winButtonH)).
		Center().Shrink(0).FocusRing(false).Transition(design.Transition(c))
	if name == "maximize" {
		b.MaximizeButton() // snap layouts on hover, on Windows 11
	}
	fg := p.TextSecondary
	if !w.Active {
		fg = p.TextTertiary
	}
	hot := b.Hovered() || b.FocusVisible()
	switch {
	case name == "close" && b.Pressed():
		b.Background(p.DangerHover)
		fg = ui.Hex("#ffffff")
	case name == "close" && hot:
		b.Background(p.Danger)
		fg = ui.Hex("#ffffff")
	case b.Pressed():
		b.Background(p.Pressed)
		fg = p.Text
	case hot:
		b.Background(p.Hover)
		fg = p.Text
	}
	b.Children(func() {
		Glyph(c, glyph, 10, fg)
		FocusMark(c, b.FocusVisible(), 9)
	})
	if b.Clicked() && act != nil {
		act()
	}
}
