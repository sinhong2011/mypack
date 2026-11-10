package companion

import (
	"net"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ParseWebURL validates a URL coming from the browser: http(s) only, bounded
// length, a real host, no credentials, no control characters. It returns the
// parsed URL and the normalized host.
func ParseWebURL(raw string) (*url.URL, string, bool) {
	if raw == "" || len(raw) > MaxURLLen {
		return nil, "", false
	}
	for _, r := range raw {
		if r < 0x21 || r == 0x7f { // controls and spaces
			return nil, "", false
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, "", false
	}
	if u.User != nil || u.Opaque != "" {
		return nil, "", false
	}
	h := NormalizeHost(u.Hostname())
	if h == "" {
		return nil, "", false
	}
	return u, h, true
}

// NormalizeHost lowercases, strips a trailing dot and a leading "www.".
// IP literals and "localhost" are kept as-is (no www stripping issue).
func NormalizeHost(h string) string {
	h = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(h), "."))
	if h == "" || net.ParseIP(h) != nil {
		return h
	}
	return strings.TrimPrefix(h, "www.")
}

// hostMatches implements the origin mapping: exact host equality with the
// source host or a configured safe domain, or a subdomain of a configured
// safe domain. Never a bare suffix match on the source host and never a
// display-name match.
func hostMatches(tabHost, sourceHost string, safeDomains []string) bool {
	if tabHost == "" {
		return false
	}
	if sourceHost != "" && tabHost == sourceHost {
		return true
	}
	if net.ParseIP(tabHost) != nil {
		for _, d := range safeDomains {
			if NormalizeHost(d) == tabHost {
				return true
			}
		}
		return false
	}
	for _, d := range safeDomains {
		d = NormalizeHost(d)
		if d == "" || net.ParseIP(d) != nil {
			continue
		}
		if tabHost == d || strings.HasSuffix(tabHost, "."+d) {
			return true
		}
	}
	return false
}

// SanitizeTitle strips control/format characters, collapses whitespace and
// truncates to MaxTitleRunes. The result is plain text, never HTML.
func SanitizeTitle(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = true
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '<' || r == '>':
			// dropped
		default:
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		}
	}
	out := b.String()
	if utf8.RuneCountInString(out) > MaxTitleRunes {
		out = string([]rune(out)[:MaxTitleRunes])
	}
	return strings.TrimSpace(out)
}
