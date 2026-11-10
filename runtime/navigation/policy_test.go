package navigation

import "testing"

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
		{"block oauth", "block", "https://accounts.google.com/x", true, Block},
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
