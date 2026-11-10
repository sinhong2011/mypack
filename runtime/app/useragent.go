package app

import "runtime"

// safariUA is a current desktop Safari user agent. WKWebView's default user
// agent omits the "Version/… Safari/…" tokens, so sites that sniff for a
// supported browser (YouTube Music, WhatsApp Web, Google Meet, …) refuse to
// load. Safari freezes the macOS version at 10_15_7, so only the Safari
// version needs to stay recent.
const safariUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.1 Safari/605.1.15"

// userAgent returns the configured user agent, or a browser-equivalent
// default where the platform webview's own is not accepted by common sites.
// WebView2 (Windows) and WebKitGTK (Linux) already send full browser user
// agents, so they keep their defaults.
func userAgent(configured string) string {
	if configured != "" {
		return configured
	}
	if runtime.GOOS == "darwin" {
		return safariUA
	}
	return ""
}
