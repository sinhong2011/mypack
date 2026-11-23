package app

import (
	"log"

	"github.com/egoist/mygo"

	"mypack.dev/mypack/internal/desktop/assets"
)

// brandWindowIcon is MyPack's dock / taskbar artwork (Linux, Windows).
func brandWindowIcon() []byte { return assets.Brand256 }

// setBrandAppIcon is the application default: Linux gtk_window_set_default_icon
// (the dock tile) and the Windows default window icon. Call it before the
// first window. Unpackaged `go run` / ad-hoc binaries have none, so Linux
// otherwise shows an empty tile. macOS uses the bundled icns instead.
func setBrandAppIcon() {
	png := brandWindowIcon()
	if len(png) == 0 {
		return
	}
	if err := mygo.App.Dock.SetIcon(png); err != nil {
		log.Printf("mypack: dock icon: %v", err)
	}
}

// setWindowIcon puts png on the window's title bar and taskbar button
// (Linux, Windows). macOS windows show no icon of their own.
func setWindowIcon(w *mygo.Window, png []byte) {
	if w == nil || len(png) == 0 {
		return
	}
	if err := w.SetIcon(png); err != nil {
		log.Printf("mypack: window icon: %v", err)
	}
}
