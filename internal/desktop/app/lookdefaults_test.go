package app

import (
	"testing"

	"mypack.dev/mypack/pkg/config"
)

func TestLookDefaultsRoundTrip(t *testing.T) {
	host := config.Target{OS: "darwin", Arch: "arm64"}
	src := config.Source{Type: config.SourceURL, Value: "https://example.com"}
	a := config.New("A", src, host)
	a.Window.TitleBarStyle, a.Window.DarkMode = "overlay", "dark"
	a.Window.Width, a.Window.Height = 1440, 900
	a.WebView.Incognito = true
	f := false
	a.WebView.Navigation.ExternalByDefault = &f
	a.Native.Notifications = "ask"
	setBlockingMode(a, blockAll)
	a.Window.Mode = config.ModeMenuBar // per site: not carried over

	b := config.New("B", src, host)
	captureLook(a).apply(b)
	w := b.Window
	if w.TitleBarStyle != "overlay" || w.DarkMode != "dark" || w.Width != 1440 || w.Height != 900 {
		t.Fatalf("window = %+v", w)
	}
	if !b.WebView.Incognito || b.WebView.Navigation.ExternalByDefault == nil || *b.WebView.Navigation.ExternalByDefault {
		t.Fatal("behaviour not carried over")
	}
	if b.Native.Notifications != "ask" || blockingMode(b) != blockAll {
		t.Fatal("notifications/blocking not carried over")
	}
	if w.Mode != "" {
		t.Fatal("mode should stay per site")
	}

	var none *lookDefaults
	c := config.New("C", src, host)
	none.apply(c) // no saved choices: untouched
	if c.Window.TitleBarStyle != "default" {
		t.Fatal("nil defaults changed cfg")
	}
}
