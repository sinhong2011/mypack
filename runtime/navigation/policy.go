// Package navigation holds the pure, testable decision logic that decides
// whether a navigation or a popup stays inside the generated app, goes to the
// default browser, is handed to the operating system (mailto:, tel:, ...) or is
// blocked (WEB-04..07).
package navigation

import (
	"golang.org/x/net/publicsuffix"
	"net/url"
	"regexp"
	"strings"
)

// Action is what the runtime does with a URL.
type Action int

const (
	// Internal keeps the URL inside the app (same window, or a new app window
	// for popups; always the same session).
	Internal Action = iota
	// External opens an http(s) URL in the default browser.
	External
	// OS hands a non-web scheme (mailto:, tel:, zoommtg:, ...) to the OS.
	OS
	// Block does nothing.
	Block
)

func (a Action) String() string {
	switch a {
	case Internal:
		return "internal"
	case External:
		return "external"
	case OS:
		return "os"
	case Block:
		return "block"
	}
	return "?"
}

// Popup modes (config webview.navigation.popups).
const (
	PopupInternal = "internal"
	PopupExternal = "external"
	PopupBlock    = "block"
)

// Policy is built once from mypack.json.
type Policy struct {
	// SafeDomains: a host equal to or a subdomain of an entry is internal.
	SafeDomains []string
	// InternalURL, when non-nil, forces matching URLs internal.
	InternalURL *regexp.Regexp
	// ExternalByDefault false keeps every http(s) URL internal.
	ExternalByDefault bool
	// Popups is internal | external | block (empty = internal).
	Popups string
	// LocalSchemes are custom schemes serving the bundled site (always internal).
	LocalSchemes []string
	// LocalOrigins are scheme://host:port origins (loopback site server)
	// that are always internal.
	LocalOrigins []string
}

// New compiles a Policy. externalByDefault nil means true.
func New(safeDomains []string, internalRegex string, externalByDefault *bool, popups string) (*Policy, error) {
	p := &Policy{ExternalByDefault: externalByDefault == nil || *externalByDefault, Popups: popups}
	for _, d := range safeDomains {
		d = strings.ToLower(strings.TrimSpace(d))
		d = strings.TrimPrefix(d, "*.")
		if d != "" {
			p.SafeDomains = append(p.SafeDomains, d)
			// The whole site counts, not just the starting host: a site's
			// sign-in and app hosts are siblings (mail.proton.me →
			// account.proton.me). The public-suffix list keeps this safe for
			// shared hosts (user.github.io stays user.github.io).
			if site, err := publicsuffix.EffectiveTLDPlusOne(d); err == nil && site != d {
				p.SafeDomains = append(p.SafeDomains, site)
			}
		}
	}
	if internalRegex != "" {
		re, err := regexp.Compile(internalRegex)
		if err != nil {
			return nil, err
		}
		p.InternalURL = re
	}
	if p.Popups == "" {
		p.Popups = PopupInternal
	}
	return p, nil
}

// hostMatches reports host == domain or host is a subdomain of domain.
func hostMatches(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

type oauthRule struct {
	host       string // exact host, or suffix when suffix is true
	suffix     bool
	pathPrefix string
}

// oauthRules are identity providers that must stay inside the app, otherwise
// "Sign in with ..." flows cannot complete (they need the app's cookie jar).
var oauthRules = []oauthRule{
	{host: "accounts.google.com"},
	{host: "appleid.apple.com"},
	{host: "idmsa.apple.com"},
	{host: "login.microsoftonline.com"},
	{host: "login.live.com"},
	{host: "login.windows.net"},
	{host: "login.microsoft.com"},
	{host: "login.microsoftonline.us"},
	{host: "account.live.com"},
	{host: "b2clogin.com", suffix: true},
	{host: "accounts.youtube.com"},
	{host: "github.com", pathPrefix: "/login/oauth"},
	{host: "gitlab.com", pathPrefix: "/oauth"},
	{host: "gitlab.com", pathPrefix: "/users/sign_in"},
	{host: "www.linkedin.com", pathPrefix: "/oauth"},
	{host: "discord.com", pathPrefix: "/oauth2"},
	{host: "accounts.spotify.com"},
	{host: "login.yahoo.com"},
	{host: "api.login.yahoo.com"},
	{host: "id.twitch.tv"},
	{host: "bitbucket.org", pathPrefix: "/site/oauth2"},
	{host: "onelogin.com", suffix: true},
	{host: "duosecurity.com", suffix: true},
	{host: "pingone.com", suffix: true},
	{host: "github.com", pathPrefix: "/login"},
	{host: "github.com", pathPrefix: "/sessions"},
	{host: "auth0.com", suffix: true},
	{host: "okta.com", suffix: true},
	{host: "oktapreview.com", suffix: true},
	{host: "amazoncognito.com", suffix: true},
	{host: "facebook.com", suffix: true, pathPrefix: "/dialog/oauth"},
	{host: "facebook.com", suffix: true, pathPrefix: "/v"}, // /v19.0/dialog/oauth
	{host: "api.twitter.com", pathPrefix: "/oauth"},
	{host: "x.com", pathPrefix: "/i/oauth2"},
	{host: "id.atlassian.com"},
	{host: "slack.com", suffix: true, pathPrefix: "/oauth"},
}

// IsOAuth reports whether u is a well-known identity-provider login URL.
func IsOAuth(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	// Firebase / Identity Platform sign-in handlers live on the project's
	// own (or custom) domain under /__/auth/, opened with signInWithPopup.
	if strings.HasPrefix(u.Path, "/__/auth/") {
		return true
	}
	// Country domains of Google's sign-in: accounts.google.co.uk, ...
	if tail, ok := strings.CutPrefix(host, "accounts.google."); ok {
		if suffix, icann := publicsuffix.PublicSuffix(tail); icann && suffix == tail {
			return true
		}
	}
	for _, r := range oauthRules {
		ok := host == r.host
		if r.suffix {
			ok = hostMatches(host, r.host)
		}
		if !ok {
			continue
		}
		if r.pathPrefix == "" {
			return true
		}
		if strings.HasPrefix(u.Path, r.pathPrefix) && (r.pathPrefix != "/v" || strings.Contains(u.Path, "/dialog/oauth")) {
			return true
		}
	}
	return false
}

func (p *Policy) isLocal(u *url.URL) bool {
	for _, s := range p.LocalSchemes {
		if u.Scheme == s {
			return true
		}
	}
	origin := u.Scheme + "://" + strings.ToLower(u.Host)
	for _, o := range p.LocalOrigins {
		if origin == o {
			return true
		}
	}
	return false
}

// webInternal decides for an http(s) URL whether it belongs inside the app.
func (p *Policy) webInternal(u *url.URL, raw string) bool {
	if p.isLocal(u) {
		return true
	}
	host := strings.ToLower(u.Hostname())
	if IsOAuth(u) {
		return true
	}
	for _, d := range p.SafeDomains {
		if hostMatches(host, d) {
			return true
		}
	}
	if p.InternalURL != nil && p.InternalURL.MatchString(raw) {
		return true
	}
	if !p.ExternalByDefault {
		return true
	}
	return false
}

// Navigate decides a top-level navigation (OnWillNavigate).
// userInitiated is whether a gesture caused it: non-web schemes are only
// handed to the OS for gestures (a page cannot spam the OS with launches).
func (p *Policy) Navigate(raw string, userInitiated bool) Action {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" {
		return Block
	}
	u.Scheme = strings.ToLower(u.Scheme)
	switch u.Scheme {
	case "http", "https", "ws", "wss":
		if u.Host == "" {
			return Block
		}
		if p.isLocal(u) || p.webInternal(u, raw) {
			return Internal
		}
		// Redirects the user did not click (server redirects, sign-in
		// hand-offs such as notion.so → notion.com, SSO providers) stay in
		// the app, as they would in a browser tab. Only links the user
		// actually follows to another site leave for the default browser.
		if !userInitiated {
			return Internal
		}
		return External
	case "about", "blob", "data", "javascript":
		return Internal // the page's own documents
	case "file", "vbscript", "chrome", "chrome-extension", "view-source":
		return Block
	}
	if p.isLocal(u) {
		return Internal
	}
	if userInitiated {
		return OS
	}
	return Block
}

// Popup decides window.open() / target=_blank.
//
// Sign-in popups are special: an identity provider's window must stay inside
// the app (same session, window.opener kept, closes itself after the
// redirect), whatever the popup mode, and even when the page opens it from
// a promise or timer (so the browser's user-gesture flag is already gone,
// as with "await fetch(...); window.open(authUrl)").
func (p *Policy) Popup(raw string, userInitiated bool) Action {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Block
	}
	scheme := strings.ToLower(u.Scheme)
	isWeb := scheme == "http" || scheme == "https"
	if isWeb && u.Host != "" && IsOAuth(u) {
		return Internal
	}
	if !userInitiated {
		return Block
	}
	if scheme == "" {
		// window.open("") : blank popup that scripts fill in later.
		if p.Popups == PopupInternal {
			return Internal
		}
		return Block
	}
	base := p.Navigate(raw, userInitiated)
	if !isWeb && scheme != "about" {
		return base // OS / Block / local scheme
	}
	if scheme == "about" {
		if p.Popups == PopupInternal {
			return Internal
		}
		return Block
	}
	switch p.Popups {
	case PopupBlock:
		return Block
	case PopupExternal:
		if p.isLocal(u) {
			return Internal
		}
		return External
	default:
		return base // Internal when the URL belongs to the app, else External
	}
}

// IsAppURL reports whether a page at raw belongs to the app itself (its
// start site, safe domains, internal-URL regex, local site), as opposed to
// an identity provider or a third party. Only such pages may use the
// native bridge (notifications, badge).
func (p *Policy) IsAppURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return p.isLocal(u)
	}
	if u.Host == "" {
		return false
	}
	if p.isLocal(u) {
		return true
	}
	host := strings.ToLower(u.Hostname())
	for _, d := range p.SafeDomains {
		if hostMatches(host, d) {
			return true
		}
	}
	if p.InternalURL != nil && p.InternalURL.MatchString(raw) {
		return true
	}
	return !p.ExternalByDefault
}
