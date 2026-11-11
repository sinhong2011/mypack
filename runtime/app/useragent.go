package app

import (
	"runtime"
	"strings"

	"mypack.dev/mypack/pkg/config"
)

// safariUA is a current desktop Safari user agent. WKWebView's default user
// agent omits the "Version/… Safari/…" tokens, so sites that sniff for a
// supported browser (YouTube Music, WhatsApp Web, Google Meet, …) refuse to
// load. Safari freezes the macOS version at 10_15_7, so only the Safari
// version needs to stay recent.
const safariUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.1 Safari/605.1.15"

// mobileSafariUA is iPhone Safari. iOS freezes the OS token ("18_6" since
// iOS 26), so only Version/ moves.
const mobileSafariUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.1 Mobile/15E148 Safari/604.1"

// Current stable majors for the Chromium and Gecko presets. Chromium and
// Firefox freeze the minor/patch parts of their UA, so only the major needs
// a refresh with each runtime release.
const (
	chromeMajor  = "154"
	firefoxMajor = "157"
)

// platformToken is the "(...)" part Chromium/Edge send per OS. Chromium
// reports Intel on Apple Silicon and Win64; x64 on Windows ARM.
func chromiumPlatform(goos, goarch string) string {
	switch goos {
	case "darwin":
		return "Macintosh; Intel Mac OS X 10_15_7"
	case "windows":
		return "Windows NT 10.0; Win64; x64"
	default:
		if goarch == "arm64" {
			return "X11; Linux aarch64"
		}
		return "X11; Linux x86_64"
	}
}

func firefoxPlatform(goos, goarch string) string {
	rv := "rv:" + firefoxMajor + ".0"
	switch goos {
	case "darwin":
		return "Macintosh; Intel Mac OS X 10.15; " + rv
	case "windows":
		return "Windows NT 10.0; Win64; x64; " + rv
	default:
		if goarch == "arm64" {
			return "X11; Linux aarch64; " + rv
		}
		return "X11; Linux x86_64; " + rv
	}
}

// presetUA maps a webview.userAgentPreset to the user agent string for an OS.
// "" means: keep the platform default (see userAgent).
func presetUA(preset, goos, goarch string) string {
	switch preset {
	case config.UAPresetChrome:
		return "Mozilla/5.0 (" + chromiumPlatform(goos, goarch) + ") AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + chromeMajor + ".0.0.0 Safari/537.36"
	case config.UAPresetEdge:
		return "Mozilla/5.0 (" + chromiumPlatform(goos, goarch) + ") AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + chromeMajor + ".0.0.0 Safari/537.36 Edg/" + chromeMajor + ".0.0.0"
	case config.UAPresetFirefox:
		return "Mozilla/5.0 (" + firefoxPlatform(goos, goarch) + ") Gecko/20100101 Firefox/" + firefoxMajor + ".0"
	case config.UAPresetMobileSafari:
		return mobileSafariUA
	}
	return ""
}

// resolveUserAgent applies the user-agent settings of a config for an OS.
// An explicit non-custom preset wins over a stale userAgent string; a
// non-empty userAgent with no preset is "custom" (backward compatible).
func resolveUserAgent(w config.WebViewConfig, goos, goarch string) string {
	switch p := w.EffectiveUAPreset(); p {
	case config.UAPresetCustom:
		if ua := strings.TrimSpace(w.UserAgent); ua != "" {
			return ua
		}
		return platformUA(goos)
	case config.UAPresetSafari:
		return platformUA(goos)
	default:
		if ua := presetUA(p, goos, goarch); ua != "" {
			return ua
		}
		return platformUA(goos)
	}
}

// platformUA is the default user agent: Safari's on macOS, where the
// WKWebView default is rejected by common sites. WebView2 (Windows) and
// WebKitGTK (Linux) already send full browser user agents, so they keep
// their own ("").
func platformUA(goos string) string {
	if goos == "darwin" {
		return safariUA
	}
	return ""
}

// userAgent returns the configured user agent, or the platform default.
func userAgent(configured string) string {
	if configured != "" {
		return configured
	}
	return platformUA(runtime.GOOS)
}
