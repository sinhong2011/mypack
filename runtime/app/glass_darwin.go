package app

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// The glass title bar (titleBarStyle "glass") lays a native strip of the
// system's Liquid Glass over the web view, so the page scrolls under it, as
// Safari's toolbar does. Two things need AppKit directly: whether the
// system has that glass (NSGlassEffectView, macOS 26), as the glass
// plugin asks, and the web view's top inset, so that the top of the page
// and its fixed headers start under the bar rather than behind it. WebKit
// calls that inset obscuredContentInsets (macOS 26); MyGo does not expose
// the WKWebView, so it is found under the window's content view.

var (
	glassOnce sync.Once
	glassOK   bool

	msgOnce   sync.Once
	msgInsets func(objc.ID, objc.SEL, nsEdgeInsets)

	selContentView    = objc.RegisterName("contentView")
	selSubviews       = objc.RegisterName("subviews")
	selCount          = objc.RegisterName("count")
	selObjectAtIndex  = objc.RegisterName("objectAtIndex:")
	selIsKindOfClass  = objc.RegisterName("isKindOfClass:")
	selResponds       = objc.RegisterName("respondsToSelector:")
	selObscuredInsets = objc.RegisterName("setObscuredContentInsets:")
)

// nsEdgeInsets is NSEdgeInsets: top, left, bottom, right in points.
type nsEdgeInsets struct{ Top, Left, Bottom, Right float64 }

// nativeGlassAvailable reports whether the system draws Liquid Glass, as
// the glass plugin's native material asks (NSGlassEffectView exists).
func nativeGlassAvailable() bool {
	glassOnce.Do(func() { glassOK = objc.GetClass("NSGlassEffectView") != 0 })
	return glassOK
}

// setWebViewTopInset sets the top obscured content inset of every web view
// in win to top points. Main thread.
func setWebViewTopInset(win *mygo.Window, top float64) {
	if win == nil {
		return
	}
	h := win.NativeHandle()
	if h == 0 {
		return
	}
	msgOnce.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			return
		}
		fn, err := purego.Dlsym(lib, "objc_msgSend")
		if err != nil {
			return
		}
		purego.RegisterFunc(&msgInsets, fn)
	})
	if msgInsets == nil {
		return
	}
	wk := objc.ID(objc.GetClass("WKWebView"))
	if wk == 0 {
		return
	}
	var walk func(v objc.ID, depth int)
	walk = func(v objc.ID, depth int) {
		if v == 0 || depth > 8 {
			return
		}
		if objc.Send[bool](v, selIsKindOfClass, wk) {
			if objc.Send[bool](v, selResponds, selObscuredInsets) {
				msgInsets(v, selObscuredInsets, nsEdgeInsets{Top: top})
			}
			return // a web view's own subviews are WebKit's
		}
		subs := v.Send(selSubviews)
		n := objc.Send[uint](subs, selCount)
		for i := uint(0); i < n; i++ {
			walk(subs.Send(selObjectAtIndex, i), depth+1)
		}
	}
	walk(objc.ID(h).Send(selContentView), 0)
}
