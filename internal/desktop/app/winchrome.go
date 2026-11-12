package app

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/kit"
)

// ownWindowButtons tells that the view draws the window controls itself:
// everywhere but macOS, whose traffic lights sit in the sidebar's strip.
// The system's caption buttons on Windows fill the window's top-right
// corner, across the canvas's rounded edge and off the toolbar's line;
// ours sit in the toolbar with its other pills (kit.WindowButtons).
var ownWindowButtons = runtime.GOOS != "darwin"

// winOrder is the desktop's order of the buttons, read once.
var winOrder []string

func windowOrder() []string {
	if winOrder == nil {
		b := mygo.Theme.WindowButtons()
		winOrder = append(append([]string{}, b.Start...), b.End...)
		if winOrder == nil {
			winOrder = []string{} // the desktop shows none
		}
	}
	return winOrder
}

// watchWindowChrome redraws w when its controls change look: maximized or
// restored, focused or not.
func watchWindowChrome(w *mygo.Window) {
	if !ownWindowButtons {
		return
	}
	redraw := func() { w.Invalidate() }
	w.OnMaximize(redraw)
	w.OnUnmaximize(redraw)
	w.OnFocus(redraw)
	w.OnBlur(redraw)
}

// windowButtons draws w's controls at the end of a toolbar row.
func windowButtons(c *ui.Context, w *mygo.Window) {
	if !ownWindowButtons || w == nil {
		return
	}
	kit.WindowButtons(c, kit.WindowControls{
		Order:         windowOrder(),
		Maximized:     w.IsMaximized(),
		Active:        w.IsFocused(),
		Minimize:      w.Minimize,
		Maximize:      w.ToggleMaximize,
		Close:         w.Close,
		MinimizeLabel: T("window.minimize"),
		MaximizeLabel: T("window.maximize"),
		RestoreLabel:  T("window.restore"),
		CloseLabel:    T("common.close"),
	})
}
