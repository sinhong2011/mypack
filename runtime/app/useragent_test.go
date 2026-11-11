package app

import (
	"runtime"
	"strings"
	"testing"

	"mypack.dev/mypack/pkg/config"
)

func TestUserAgent(t *testing.T) {
	if got := userAgent("Custom/1.0"); got != "Custom/1.0" {
		t.Fatalf("configured UA not kept: %q", got)
	}
	got := userAgent("")
	if runtime.GOOS == "darwin" && (!strings.Contains(got, "Version/") || !strings.Contains(got, "Safari/")) {
		t.Fatalf("darwin default must look like Safari: %q", got)
	}
}

func TestResolveUserAgent(t *testing.T) {
	wv := func(preset, ua string) config.WebViewConfig {
		return config.WebViewConfig{UserAgentPreset: preset, UserAgent: ua}
	}
	// Backward compatibility: a bare userAgent is custom, nothing set is the default.
	if got := resolveUserAgent(wv("", "Legacy/1"), "windows", "amd64"); got != "Legacy/1" {
		t.Fatalf("legacy custom: %q", got)
	}
	if got := resolveUserAgent(wv("", ""), "darwin", "arm64"); got != safariUA {
		t.Fatalf("default darwin: %q", got)
	}
	if got := resolveUserAgent(wv("", ""), "windows", "amd64"); got != "" {
		t.Fatalf("default windows keeps WebView2's: %q", got)
	}
	if got := resolveUserAgent(wv("safari", "Stale/1"), "darwin", "arm64"); got != safariUA {
		t.Fatalf("explicit safari preset wins: %q", got)
	}
	if got := resolveUserAgent(wv("custom", "Mine/2"), "linux", "amd64"); got != "Mine/2" {
		t.Fatalf("custom: %q", got)
	}
	if got := resolveUserAgent(wv("custom", ""), "linux", "amd64"); got != "" {
		t.Fatalf("custom without a string falls back: %q", got)
	}

	want := map[string]map[string]string{
		"chrome": {
			"darwin":  "(Macintosh; Intel Mac OS X 10_15_7)",
			"windows": "(Windows NT 10.0; Win64; x64)",
			"linux":   "(X11; Linux x86_64)",
		},
		"edge": {
			"darwin":  "(Macintosh; Intel Mac OS X 10_15_7)",
			"windows": "(Windows NT 10.0; Win64; x64)",
			"linux":   "(X11; Linux x86_64)",
		},
		"firefox": {
			"darwin":  "(Macintosh; Intel Mac OS X 10.15; rv:",
			"windows": "(Windows NT 10.0; Win64; x64; rv:",
			"linux":   "(X11; Linux x86_64; rv:",
		},
	}
	for preset, per := range want {
		for goos, frag := range per {
			ua := resolveUserAgent(wv(preset, ""), goos, "amd64")
			if !strings.HasPrefix(ua, "Mozilla/5.0 "+frag) {
				t.Errorf("%s/%s: %q", preset, goos, ua)
			}
			switch preset {
			case "chrome":
				if !strings.Contains(ua, " Chrome/"+chromeMajor+".0.0.0 Safari/537.36") || strings.Contains(ua, "Edg/") {
					t.Errorf("chrome/%s: %q", goos, ua)
				}
			case "edge":
				if !strings.HasSuffix(ua, " Edg/"+chromeMajor+".0.0.0") {
					t.Errorf("edge/%s: %q", goos, ua)
				}
			case "firefox":
				if !strings.HasSuffix(ua, "Gecko/20100101 Firefox/"+firefoxMajor+".0") || strings.Contains(ua, "Chrome") {
					t.Errorf("firefox/%s: %q", goos, ua)
				}
			}
		}
	}
	if ua := resolveUserAgent(wv("chrome", ""), "linux", "arm64"); !strings.Contains(ua, "Linux aarch64") {
		t.Errorf("linux arm64: %q", ua)
	}
	m := resolveUserAgent(wv("mobile-safari", ""), "darwin", "arm64")
	if !strings.Contains(m, "iPhone") || !strings.Contains(m, "Mobile/") || !strings.HasSuffix(m, "Safari/604.1") {
		t.Errorf("mobile safari: %q", m)
	}
	if resolveUserAgent(wv("mobile-safari", ""), "windows", "amd64") != m {
		t.Error("mobile-safari is the same everywhere")
	}
}
