package navigation

import (
	"net/url"
	"testing"
)

func bptr(b bool) *bool { return &b }

func mk(t *testing.T, safe []string, re string, ext *bool, popups string) *Policy {
	t.Helper()
	p, err := New(safe, re, ext, popups)
	if err != nil {
		t.Fatal(err)
	}
	p.LocalSchemes = []string{"mypack"}
	p.LocalOrigins = []string{"http://127.0.0.1:5555"}
	return p
}

func TestNavigate(t *testing.T) {
	base := mk(t, []string{"example.com"}, `^https://cdn\.partner\.io/app/`, nil, "")
	open := mk(t, []string{"example.com"}, "", bptr(false), "")
	cases := []struct {
		name string
		p    *Policy
		url  string
		user bool
		want Action
	}{
		{"same host", base, "https://example.com/a", true, Internal},
		{"subdomain", base, "https://app.example.com/a", true, Internal},
		{"lookalike suffix", base, "https://badexample.com/", true, External},
		{"prefix lookalike", base, "https://example.com.evil.net/", true, External},
		{"other site", base, "https://news.ycombinator.com/", true, External},
		{"regex internal", base, "https://cdn.partner.io/app/x", true, Internal},
		{"regex miss", base, "https://cdn.partner.io/other", true, External},
		{"google oauth", base, "https://accounts.google.com/o/oauth2/v2/auth?x=1", true, Internal},
		{"apple", base, "https://appleid.apple.com/auth/authorize", true, Internal},
		{"microsoft", base, "https://login.microsoftonline.com/common/oauth2", true, Internal},
		{"github login", base, "https://github.com/login/oauth/authorize", true, Internal},
		{"github repo", base, "https://github.com/egoist/mygo", true, External},
		{"auth0 tenant", base, "https://foo.us.auth0.com/authorize", true, Internal},
		{"okta tenant", base, "https://acme.okta.com/oauth2/v1/authorize", true, Internal},
		{"externalByDefault=false", open, "https://anything.org/x", true, Internal},
		{"mailto user", base, "mailto:a@b.c", true, OS},
		{"mailto script", base, "mailto:a@b.c", false, Block},
		{"tel", base, "tel:+1555", true, OS},
		{"zoommtg", base, "zoommtg://zoom.us/join?confno=1", true, OS},
		{"file blocked", base, "file:///etc/passwd", true, Block},
		{"javascript", base, "javascript:void(0)", true, Internal},
		{"about:blank", base, "about:blank", false, Internal},
		{"local scheme", base, "mypack://localhost/index.html", true, Internal},
		{"local origin", base, "http://127.0.0.1:5555/tok/x", true, Internal},
		{"other loopback port", base, "http://127.0.0.1:9999/", true, External},
		{"garbage", base, "::::", true, Block},
		{"host case", base, "https://EXAMPLE.com/", true, Internal},
	}
	for _, c := range cases {
		if got := c.p.Navigate(c.url, c.user); got != c.want {
			t.Errorf("%s: Navigate(%q) = %v, want %v", c.name, c.url, got, c.want)
		}
	}
}

func TestPopup(t *testing.T) {
	cases := []struct {
		name   string
		popups string
		url    string
		user   bool
		want   Action
	}{
		{"internal same site", "internal", "https://example.com/p", true, Internal},
		{"internal other site", "internal", "https://other.org/", true, External},
		{"internal oauth", "internal", "https://accounts.google.com/x", true, Internal},
		{"scripted popup", "internal", "https://example.com/p", false, Block},
		{"external same site", "external", "https://example.com/p", true, External},
		{"external oauth stays", "external", "https://accounts.google.com/x", true, Internal},
		{"block", "block", "https://example.com/p", true, Block},
		{"block keeps oauth", "block", "https://accounts.google.com/x", true, Internal},
		{"block keeps microsoft", "block", "https://login.microsoftonline.com/common/oauth2/v2.0/authorize", true, Internal},
		{"block keeps apple", "block", "https://appleid.apple.com/auth/authorize?client_id=x", true, Internal},
		{"block keeps github", "block", "https://github.com/login/oauth/authorize?client_id=x", true, Internal},
		{"scripted google popup (after await)", "internal", "https://accounts.google.com/o/oauth2/v2/auth", false, Internal},
		{"scripted microsoft popup", "internal", "https://login.microsoftonline.com/common/oauth2/v2.0/authorize", false, Internal},
		{"scripted apple popup", "internal", "https://appleid.apple.com/auth/authorize", false, Internal},
		{"scripted github popup", "internal", "https://github.com/login/oauth/authorize", false, Internal},
		{"scripted google ccTLD popup", "internal", "https://accounts.google.co.uk/signin", false, Internal},
		{"firebase handler popup", "internal", "https://myproj.firebaseapp.com/__/auth/handler?apiKey=x", true, Internal},
		{"firebase custom domain popup", "internal", "https://auth.acme.dev/__/auth/handler", false, Internal},
		{"scripted unrelated popup", "internal", "https://ads.example.net/", false, Block},
		{"scripted github repo popup", "internal", "https://github.com/egoist/mygo", false, Block},
		{"external keeps scripted oauth", "external", "https://accounts.google.com/x", false, Internal},
		{"external other", "external", "https://other.org/", true, External},
		{"about:blank internal", "internal", "about:blank", true, Internal},
		{"about:blank block", "block", "about:blank", true, Block},
		{"scripted javascript popup", "internal", "javascript:void(0)", false, Block},
		{"blank internal", "internal", "", true, Internal},
		{"blank block", "block", "", true, Block},
		{"mailto popup", "block", "mailto:x@y.z", true, OS},
		{"default empty mode", "", "https://example.com/p", true, Internal},
	}
	for _, c := range cases {
		p := mk(t, []string{"example.com"}, "", nil, c.popups)
		if got := p.Popup(c.url, c.user); got != c.want {
			t.Errorf("%s: Popup(%q) = %v, want %v", c.name, c.url, got, c.want)
		}
	}
}

func TestBadRegex(t *testing.T) {
	if _, err := New(nil, "(", nil, ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestWildcardDomain(t *testing.T) {
	p := mk(t, []string{"*.example.com"}, "", nil, "")
	if p.Navigate("https://a.example.com/", true) != Internal {
		t.Fatal("wildcard should match subdomain")
	}
}

func TestSiteAndRedirects(t *testing.T) {
	p, err := New([]string{"mail.proton.me"}, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		url  string
		user bool
		want Action
	}{
		{"https://account.proton.me/login", false, Internal}, // sibling host, redirect
		{"https://account.proton.me/login", true, Internal},  // sibling host, clicked
		{"https://proton.me/support", true, Internal},        // same site
		{"https://example.com/", true, External},             // clicked link elsewhere
		{"https://www.notion.com/login", false, Internal},    // unclicked redirect off-site
	}
	for _, c := range cases {
		if got := p.Navigate(c.url, c.user); got != c.want {
			t.Errorf("Navigate(%q, user=%v) = %v, want %v", c.url, c.user, got, c.want)
		}
	}
	// Shared hosting: one user's pages do not make the whole suffix internal.
	g, _ := New([]string{"alice.github.io"}, "", nil, "")
	if got := g.Navigate("https://bob.github.io/", true); got != External {
		t.Errorf("bob.github.io = %v, want External", got)
	}
}

func TestIsAppURL(t *testing.T) {
	p := mk(t, []string{"example.com"}, `^https://cdn\.partner\.io/app/`, nil, "")
	cases := []struct {
		url  string
		want bool
	}{
		{"https://example.com/", true},
		{"https://mail.example.com/x", true},
		{"https://evilexample.com/", false},
		{"https://accounts.google.com/", false}, // identity provider: internal, but not the app
		{"https://cdn.partner.io/app/x", true},
		{"https://cdn.partner.io/other", false},
		{"https://ads.third.party/", false},
		{"mypack://localhost/index.html", true},
		{"http://127.0.0.1:5555/index.html", true},
		{"http://127.0.0.1:6666/", false},
		{"javascript:alert(1)", false},
		{"about:blank", false},
	}
	for _, c := range cases {
		if got := p.IsAppURL(c.url); got != c.want {
			t.Errorf("IsAppURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
	open := mk(t, []string{"example.com"}, "", bptr(false), "")
	if !open.IsAppURL("https://anything.org/") {
		t.Error("externalByDefault=false: every http(s) page belongs to the app")
	}
}

func TestOAuthRules(t *testing.T) {
	yes := []string{
		"https://accounts.google.com/o/oauth2/v2/auth",
		"https://accounts.google.de/signin",
		"https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		"https://contoso.b2clogin.com/contoso.onmicrosoft.com/b2c_1_signin/oauth2/v2.0/authorize",
		"https://appleid.apple.com/auth/authorize",
		"https://idmsa.apple.com/appleauth/auth/signin",
		"https://github.com/login/oauth/authorize",
		"https://github.com/sessions/two-factor",
		"https://app.firebaseapp.com/__/auth/handler",
		"https://discord.com/oauth2/authorize?client_id=1",
	}
	no := []string{
		"https://github.com/egoist/mygo",
		"https://accounts.google.evil.com.attacker.io/x",
		"https://accounts.google.evil.com/signin",
		"https://example.com/",
		"https://discord.com/channels/1/2",
	}
	for _, s := range yes {
		u, _ := url.Parse(s)
		if !IsOAuth(u) {
			t.Errorf("IsOAuth(%q) = false", s)
		}
	}
	for _, s := range no {
		u, _ := url.Parse(s)
		if IsOAuth(u) {
			t.Errorf("IsOAuth(%q) = true", s)
		}
	}
}
