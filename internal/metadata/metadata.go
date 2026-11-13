// Package metadata fetches a website's title, icon and theme color safely
// (timeouts, redirect cap, size limits, http/https only). Spec §3 Source.
//
// Privacy: Fetch only contacts the website itself. FetchIconFrom asks a
// third-party favicon service (Google or DuckDuckGo) and is only called
// when the user picks one; it sends just the site's host name.
package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/pkg/config"
)

const (
	totalBudget   = 8 * time.Second
	maxRedirects  = 5
	maxHTMLBytes  = 2 << 20
	maxIconBytes  = 1 << 20
	maxManifest   = 256 << 10
	maxIconTries  = 6
	userAgent     = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15"
	iconOutputMax = 512
)

// Result is what we learned about a website.
type Result struct {
	URL         string // normalized final URL after redirects
	Title       string // cleaned suggested app name (site name, not "Home | Foo")
	Description string
	IconURL     string
	IconPNG     []byte // best icon, decoded and re-encoded as square PNG (≤512px), nil if none
	ThemeColor  string // "#rrggbb" or ""
	Fallback    bool   // true when the fetch failed and values are guesses from the URL
	Warning     string // human-readable reason when Fallback
}

// Fetch never returns an error for network problems: it returns a Result
// with Fallback=true (title guessed from the host). It returns an error only
// for invalid input URLs (or when ctx itself is cancelled). Total budget ~8s;
// HTML read capped at 2 MiB, icons at 1 MiB; at most 5 redirects.
func Fetch(ctx context.Context, rawURL string) (Result, error) {
	norm, err := config.NormalizeURL(rawURL)
	if err != nil {
		return Result{}, err
	}
	return fetch(ctx, norm, newClient())
}

func newClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 6 * time.Second,
			MaxIdleConns:          4,
			IdleConnTimeout:       5 * time.Second,
			DisableKeepAlives:     true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			if s := req.URL.Scheme; s != "http" && s != "https" {
				return fmt.Errorf("redirect to unsupported scheme %q", s)
			}
			return nil
		},
	}
}

func fallback(norm, warning string) Result {
	r := Result{URL: norm, Fallback: true, Warning: warning}
	if u, err := url.Parse(norm); err == nil {
		r.Title = GuessTitleFromHost(u.Hostname())
	}
	return r
}

func fetch(parent context.Context, norm string, client *http.Client) (Result, error) {
	ctx, cancel := context.WithTimeout(parent, totalBudget)
	defer cancel()

	body, final, status, err := get(ctx, client, norm, maxHTMLBytes, "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	if err != nil && body == nil {
		if parent.Err() != nil {
			return Result{}, parent.Err()
		}
		return fallback(norm, "Could not reach the website: "+shortErr(err)), nil
	}
	res := Result{URL: norm}
	if final != nil {
		if n, e := config.NormalizeURL(final.String()); e == nil {
			res.URL = n
		}
	}
	base, _ := url.Parse(res.URL)
	if status >= 400 {
		res = fallback(res.URL, fmt.Sprintf("The website answered with HTTP %d", status))
		// Still try the conventional favicon location, best effort.
		if p, iu := bestIcon(ctx, client, []iconCand{{u: base.ResolveReference(&url.URL{Path: "/favicon.ico"}).String(), size: 16}}); p != nil {
			res.IconPNG, res.IconURL = p, iu
		}
		return res, nil
	}

	doc := parseHTML(body)
	// Web app manifest
	var man *manifest
	if doc.manifestHref != "" {
		if mu := resolve(base, doc.manifestHref); mu != "" {
			if mb, mfinal, _, merr := get(ctx, client, mu, maxManifest, "application/manifest+json,application/json,*/*;q=0.5"); merr == nil || mb != nil {
				if mf, ok := parseManifest(mb, mfinal); ok {
					man = mf
				}
			}
		}
	}

	res.Title = chooseTitle(doc, man, base.Hostname())
	res.Description = clean(firstNonEmpty(doc.meta["description"], doc.meta["og:description"], doc.meta["twitter:description"]), 300)
	res.ThemeColor = normalizeColor(doc.themeColor)
	if res.ThemeColor == "" && man != nil {
		res.ThemeColor = normalizeColor(man.ThemeColor)
	}

	cands := iconCandidates(doc, man, base)
	if p, iu := bestIcon(ctx, client, cands); p != nil {
		res.IconPNG, res.IconURL = p, iu
	}
	if parent.Err() != nil {
		return Result{}, parent.Err()
	}
	return res, nil
}

func shortErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	s := err.Error()
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

// get fetches a URL, reading at most limit bytes. It returns the body even
// when it was truncated by the limit.
func get(ctx context.Context, client *http.Client, u string, limit int64, accept string) ([]byte, *url.URL, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil && len(b) == 0 {
		return nil, resp.Request.URL, resp.StatusCode, err
	}
	if b == nil {
		b = []byte{}
	}
	return b, resp.Request.URL, resp.StatusCode, nil
}

func resolve(base *url.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(strings.ToLower(ref), "data:") {
		return ""
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	abs := base.ResolveReference(r)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return ""
	}
	return abs.String()
}

// ---- HTML parsing ----

type linkTag struct {
	rel, href, sizes, typ string
}

type pageInfo struct {
	title        string
	meta         map[string]string // name/property → content (first wins)
	themeColor   string
	themeMedia   string
	links        []linkTag
	manifestHref string
	baseHref     string
}

func parseHTML(b []byte) *pageInfo {
	p := &pageInfo{meta: map[string]string{}}
	root, err := html.Parse(bytes.NewReader(b))
	if err != nil {
		return p
	}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if p.title == "" && n.FirstChild != nil {
					p.title = textOf(n)
				}
			case "meta":
				name := strings.ToLower(attr(n, "name"))
				if name == "" {
					name = strings.ToLower(attr(n, "property"))
				}
				content := attr(n, "content")
				if name == "" || content == "" {
					break
				}
				if name == "theme-color" {
					// first one wins, unless it was media-conditioned and this one is not
					media := attr(n, "media")
					if p.themeColor == "" || (p.themeMedia != "" && media == "") {
						p.themeColor, p.themeMedia = content, media
					}
					break
				}
				if _, ok := p.meta[name]; !ok {
					p.meta[name] = content
				}
			case "link":
				rel := strings.ToLower(strings.TrimSpace(attr(n, "rel")))
				href := attr(n, "href")
				if href == "" {
					break
				}
				if hasToken(rel, "manifest") && p.manifestHref == "" {
					p.manifestHref = href
				}
				p.links = append(p.links, linkTag{rel: rel, href: href, sizes: attr(n, "sizes"), typ: strings.ToLower(attr(n, "type"))})
			case "base":
				if p.baseHref == "" {
					p.baseHref = attr(n, "href")
				}
			case "body":
				return // everything we need lives in <head>
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return p
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	}
	return sb.String()
}

func hasToken(s, tok string) bool {
	for _, f := range strings.Fields(s) {
		if f == tok {
			return true
		}
	}
	return false
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// ---- manifest ----

type manifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose"`
}

type manifest struct {
	Name       string         `json:"name"`
	ShortName  string         `json:"short_name"`
	ThemeColor string         `json:"theme_color"`
	Icons      []manifestIcon `json:"icons"`
	base       *url.URL
}

func parseManifest(b []byte, final *url.URL) (*manifest, bool) {
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, false
	}
	m.base = final
	return &m, true
}

// ---- title ----

var titleSeparators = []string{" | ", " - ", " — ", " – ", " · ", " • ", " :: ", " « ", " » "}

func chooseTitle(doc *pageInfo, man *manifest, host string) string {
	var cands []string
	cands = append(cands, doc.meta["og:site_name"], doc.meta["application-name"], doc.meta["apple-mobile-web-app-title"])
	if man != nil {
		cands = append(cands, man.ShortName, man.Name)
	}
	for _, c := range cands {
		if t := clean(c, 64); t != "" && len([]rune(t)) <= 40 {
			return t
		}
	}
	if t := cleanTitleTag(doc.title, host); t != "" {
		return t
	}
	if t := cleanTitleTag(doc.meta["og:title"], host); t != "" {
		return t
	}
	return GuessTitleFromHost(host)
}

// cleanTitleTag extracts the brand-ish part of a page title.
func cleanTitleTag(title, host string) string {
	title = collapse(title)
	if title == "" {
		return ""
	}
	parts := []string{title}
	for _, sep := range titleSeparators {
		if strings.Contains(title, sep) {
			parts = nil
			for _, p := range strings.Split(title, sep) {
				if p = strings.TrimSpace(p); p != "" {
					parts = append(parts, p)
				}
			}
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	brand := brandKey(host)
	best, bestScore := -1, 0
	for i, p := range parts {
		if s := matchScore(normKey(p), brand); s > bestScore {
			best, bestScore = i, s
		}
	}
	var pick string
	switch {
	case best >= 0:
		pick = parts[best]
	case len(parts) == 1:
		pick = parts[0]
	default:
		last := parts[len(parts)-1]
		if len([]rune(last)) <= 30 {
			pick = last
		} else {
			pick = parts[0]
		}
	}
	if k := knownName(host); k != "" && normKey(pick) == normKey(k) {
		pick = k
	}
	return clean(pick, 64)
}

func matchScore(part, brand string) int {
	if part == "" || brand == "" {
		return 0
	}
	switch {
	case part == brand:
		return 100
	case strings.Contains(part, brand):
		return 60
	case len(part) >= 3 && strings.Contains(brand, part):
		return 40
	}
	return 0
}

func normKey(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127 {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// clean collapses whitespace, strips characters illegal in app names and
// truncates to max runes.
func clean(s string, max int) string {
	s = collapse(s)
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		s = strings.TrimSpace(string(r[:max]))
	}
	return s
}

// ---- icons ----

type iconCand struct {
	u      string
	size   int // declared (or guessed) size; 0 = unknown
	source int // tie-breaker: lower is preferred
	png    bool
}

func parseSizes(s string) int {
	best := 0
	for _, f := range strings.Fields(strings.ToLower(s)) {
		if f == "any" {
			if best < 512 {
				best = 512
			}
			continue
		}
		w, _, ok := strings.Cut(f, "x")
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(w); err == nil && n > best {
			best = n
		}
	}
	return best
}

func iconCandidates(doc *pageInfo, man *manifest, base *url.URL) []iconCand {
	var out []iconCand
	seen := map[string]bool{}
	add := func(c iconCand) {
		if c.u == "" || seen[c.u] {
			return
		}
		low := strings.ToLower(c.u)
		if i := strings.IndexAny(low, "?#"); i >= 0 {
			low = low[:i]
		}
		if strings.HasSuffix(low, ".svg") || strings.HasSuffix(low, ".svgz") {
			return // no SVG rasterizer: skip
		}
		seen[c.u] = true
		c.png = strings.HasSuffix(low, ".png")
		out = append(out, c)
	}
	linkBase := base
	if doc.baseHref != "" {
		if bu, err := url.Parse(doc.baseHref); err == nil {
			linkBase = base.ResolveReference(bu)
		}
	}
	for _, l := range doc.links {
		if hasToken(l.rel, "mask-icon") || l.typ == "image/svg+xml" {
			continue
		}
		touch := hasToken(l.rel, "apple-touch-icon") || hasToken(l.rel, "apple-touch-icon-precomposed")
		if !touch && !hasToken(l.rel, "icon") && !hasToken(l.rel, "shortcut") {
			continue
		}
		size := parseSizes(l.sizes)
		src := 2
		if touch {
			src = 0
			if size == 0 {
				size = 180
			}
		} else if size == 0 {
			size = 32
		}
		add(iconCand{u: resolve(linkBase, l.href), size: size, source: src})
	}
	if man != nil {
		for _, mi := range man.Icons {
			if mi.Type == "image/svg+xml" {
				continue
			}
			size := parseSizes(mi.Sizes)
			if size == 0 {
				size = 64
			}
			if strings.Contains(strings.ToLower(mi.Purpose), "maskable") && !strings.Contains(strings.ToLower(mi.Purpose), "any") {
				size = size * 9 / 10 // maskable art has extra safe-zone padding
			}
			add(iconCand{u: resolve(man.base, mi.Src), size: size, source: 1})
		}
	}
	// conventional locations as last resorts
	add(iconCand{u: resolve(base, "/apple-touch-icon.png"), size: 120, source: 3})
	add(iconCand{u: resolve(base, "/favicon.ico"), size: 16, source: 4})

	score := func(c iconCand) int {
		s := c.size
		if s > 512 {
			s = 512
		}
		if c.size >= 180 {
			s += 1000 // "prefer largest PNG >= 180px"
		}
		return s
	}
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := score(out[i]), score(out[j])
		if si != sj {
			return si > sj
		}
		if out[i].png != out[j].png {
			return out[i].png
		}
		return out[i].source < out[j].source
	})
	return out
}

// bestIcon tries candidates in order and returns the largest real icon:
// declared sizes are often wrong, so it stops early only once an icon is
// actually ≥ 180px, otherwise it keeps the biggest one it decoded.
func bestIcon(ctx context.Context, client *http.Client, cands []iconCand) ([]byte, string) {
	var bestRaw []byte
	bestURL, bestSize := "", 0
	tries := 0
	for _, c := range cands {
		if tries >= maxIconTries || ctx.Err() != nil {
			break
		}
		tries++
		b, _, status, err := get(ctx, client, c.u, maxIconBytes, "image/png,image/*;q=0.8,*/*;q=0.5")
		if err != nil || status >= 400 || len(b) == 0 {
			continue
		}
		img, err := icons.Decode(b)
		if err != nil {
			continue
		}
		size := min(img.Bounds().Dx(), img.Bounds().Dy())
		if size > bestSize {
			bestRaw, bestURL, bestSize = b, c.u, size
		}
		if size >= 180 {
			break
		}
	}
	if bestRaw == nil {
		return nil, ""
	}
	norm, err := icons.Normalize(bestRaw)
	if err != nil {
		return nil, ""
	}
	out, err := icons.Resize(norm, iconOutputMax)
	if err != nil {
		return nil, ""
	}
	return out, bestURL
}

// ---- colors ----

func normalizeColor(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "#") {
		h := s[1:]
		switch len(h) {
		case 3, 4:
			var sb strings.Builder
			for _, c := range h[:3] {
				sb.WriteRune(c)
				sb.WriteRune(c)
			}
			h = sb.String()
		case 6:
		case 8:
			h = h[:6]
		default:
			return ""
		}
		if _, err := strconv.ParseUint(h, 16, 32); err != nil {
			return ""
		}
		return "#" + h
	}
	if strings.HasPrefix(s, "rgb") {
		open, close := strings.Index(s, "("), strings.LastIndex(s, ")")
		if open < 0 || close < open {
			return ""
		}
		fields := strings.FieldsFunc(s[open+1:close], func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
		if len(fields) < 3 {
			return ""
		}
		var v [3]int
		for i := 0; i < 3; i++ {
			f := fields[i]
			pct := strings.HasSuffix(f, "%")
			n, err := strconv.ParseFloat(strings.TrimSuffix(f, "%"), 64)
			if err != nil {
				return ""
			}
			if pct {
				n = n * 255 / 100
			}
			if n < 0 {
				n = 0
			}
			if n > 255 {
				n = 255
			}
			v[i] = int(n + 0.5)
		}
		return fmt.Sprintf("#%02x%02x%02x", v[0], v[1], v[2])
	}
	if h, ok := namedColors[s]; ok {
		return h
	}
	return ""
}

var namedColors = map[string]string{
	"white": "#ffffff", "black": "#000000", "red": "#ff0000", "blue": "#0000ff", "green": "#008000",
	"orange": "#ffa500", "purple": "#800080", "yellow": "#ffff00", "gray": "#808080", "grey": "#808080",
}
