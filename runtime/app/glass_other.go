//go:build !darwin

package app

import "github.com/egoist/mygo"

// Only macOS 26 and later have the system's glass that samples a web page;
// elsewhere the glass title bar falls back to the Seamless strip.
func nativeGlassAvailable() bool { return false }

func setWebViewTopInset(*mygo.Window, float64) {}
