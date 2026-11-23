package userscript

import (
	"fmt"
	"regexp"
	"strings"
)

// Rule is one URL rule as a regular expression that both Go (RE2) and
// JavaScript accept, so the runtime's page-side dispatcher and Go tests agree.
type Rule struct {
	Source     string `json:"s"`
	IgnoreCase bool   `json:"i,omitempty"`
}

// Test reports whether rawURL matches r (Go side; the page uses RegExp).
func (r Rule) Test(rawURL string) bool {
	src := r.Source
	if r.IgnoreCase {
		src = "(?i)" + src
	}
	re, err := regexp.Compile(src)
	return err == nil && re.MatchString(rawURL)
}

// Rules turns @match/@include and @exclude/@exclude-match into rules. A
// script with neither @match nor @include runs on every page, as in
// Violentmonkey.
func (m Meta) Rules() (include, exclude []Rule, err error) {
	for _, p := range m.Match {
		r, err := MatchPattern(p)
		if err != nil {
			return nil, nil, err
		}
		include = append(include, r)
	}
	for _, p := range m.Include {
		include = append(include, Glob(p))
	}
	for _, p := range m.ExcludeMatch {
		r, err := MatchPattern(p)
		if err != nil {
			return nil, nil, err
		}
		exclude = append(exclude, r)
	}
	for _, p := range m.Exclude {
		exclude = append(exclude, Glob(p))
	}
	if len(m.Match) == 0 && len(m.Include) == 0 {
		include = []Rule{{Source: "^"}}
	}
	return include, exclude, nil
}

// Matches reports whether rawURL is matched by include and not by exclude.
func Matches(include, exclude []Rule, rawURL string) bool {
	in := false
	for _, r := range include {
		if r.Test(rawURL) {
			in = true
			break
		}
	}
	if !in {
		return false
	}
	for _, r := range exclude {
		if r.Test(rawURL) {
			return false
		}
	}
	return true
}

var matchRe = regexp.MustCompile(`^(\*|https?|file|ftp|wss?)://([^/]*)(/.*)$`)

// MatchPattern converts a @match pattern (Chrome match pattern syntax).
func MatchPattern(p string) (Rule, error) {
	p = strings.TrimSpace(p)
	if p == "<all_urls>" || p == "*://*/*" {
		return Rule{Source: `^(?:https?|file|ftp)://`}, nil
	}
	g := matchRe.FindStringSubmatch(p)
	if g == nil {
		return Rule{}, fmt.Errorf("@match %q is not a match pattern (scheme://host/path)", p)
	}
	scheme, host, path := g[1], g[2], g[3]
	var b strings.Builder
	b.WriteString("^")
	if scheme == "*" {
		b.WriteString("https?")
	} else {
		b.WriteString(regexp.QuoteMeta(scheme))
	}
	b.WriteString("://")
	switch {
	case scheme == "file":
	case host == "*":
		b.WriteString(`[^/]+`)
	case strings.HasPrefix(host, "*."):
		rest := host[2:]
		if rest == "" || strings.Contains(rest, "*") {
			return Rule{}, fmt.Errorf("@match %q has an invalid host", p)
		}
		b.WriteString(`(?:[^/]+\.)?` + regexp.QuoteMeta(rest) + `(?::\d+)?`)
	default:
		if host == "" || strings.Contains(host, "*") {
			return Rule{}, fmt.Errorf("@match %q has an invalid host", p)
		}
		b.WriteString(regexp.QuoteMeta(host) + `(?::\d+)?`)
	}
	b.WriteString(globBody(path))
	b.WriteString("$")
	return Rule{Source: b.String()}, nil
}

var regexRule = regexp.MustCompile(`^/(.+)/([a-z]*)$`)

// Glob converts an @include/@exclude value: a glob with * wildcards, or a
// /regular expression/ (optionally with the i flag).
func Glob(p string) Rule {
	p = strings.TrimSpace(p)
	if g := regexRule.FindStringSubmatch(p); g != nil {
		return Rule{Source: g[1], IgnoreCase: strings.Contains(g[2], "i")}
	}
	if p == "*" {
		return Rule{Source: "^"}
	}
	return Rule{Source: "^" + globBody(p) + "$", IgnoreCase: true}
}

func globBody(p string) string {
	parts := strings.Split(p, "*")
	for i, s := range parts {
		parts[i] = regexp.QuoteMeta(s)
	}
	return strings.Join(parts, ".*")
}

// Sites are the hosts a script's @match/@include name, for display
// ("x.com", "*.example.com"); "all sites" when it runs everywhere.
func (m Meta) Sites() []string {
	var out []string
	seen := map[string]bool{}
	add := func(h string) {
		if h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	for _, p := range append(append([]string{}, m.Match...), m.Include...) {
		p = strings.TrimSpace(p)
		if p == "<all_urls>" || p == "*" || p == "*://*/*" {
			return []string{"*"}
		}
		if i := strings.Index(p, "://"); i >= 0 {
			h := p[i+3:]
			if j := strings.IndexAny(h, "/?#"); j >= 0 {
				h = h[:j]
			}
			if h == "*" {
				return []string{"*"}
			}
			add(h)
		}
	}
	if len(m.Match) == 0 && len(m.Include) == 0 {
		return []string{"*"}
	}
	return out
}
