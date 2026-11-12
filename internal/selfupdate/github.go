package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Repo is where Mypack's releases are published.
const Repo = "sinhong2011/mypack"

// Release is a published Mypack release.
type Release struct {
	Version    string    // "1.2.3", without the tag's "v"
	Tag        string    // "v1.2.3"
	Notes      string    // the release body, Markdown
	URL        string    // the release page
	Prerelease bool      // marked a prerelease, or has a prerelease version
	Published  time.Time // when it was published
}

// Checker asks GitHub for the newest release.
type Checker struct {
	Repo    string       // owner/name; empty = Repo
	BaseURL string       // the API; empty = https://api.github.com
	Client  *http.Client // nil = a client with a 20 s timeout
	// UserAgent identifies the app to the API (it requires one).
	UserAgent string
}

// tagRE is a release tag of the app: vX.Y.Z or vX.Y.Z-pre.
var tagRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

// ErrRateLimited is returned when GitHub refuses the request for now.
var ErrRateLimited = errors.New("GitHub's rate limit was reached; try again later")

// Latest returns the newest release: the highest vX.Y.Z that is neither a
// draft nor, unless prereleases is set, a prerelease. It returns nil, nil
// when there is none.
func (c Checker) Latest(ctx context.Context, prereleases bool) (*Release, error) {
	repo, base := c.Repo, strings.TrimSuffix(c.BaseURL, "/")
	if repo == "" {
		repo = Repo
	}
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/repos/"+repo+"/releases?per_page=30", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	ua := c.UserAgent
	if ua == "" {
		ua = "Mypack"
	}
	req.Header.Set("User-Agent", ua)
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0",
		resp.StatusCode == http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var list []ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&list); err != nil {
		return nil, fmt.Errorf("reading GitHub's releases: %w", err)
	}
	return pickLatest(list, prereleases), nil
}

// pickLatest chooses the highest eligible release.
func pickLatest(list []ghRelease, prereleases bool) *Release {
	var best *Release
	var bestV Version
	for _, r := range list {
		if r.Draft || !tagRE.MatchString(r.TagName) {
			continue
		}
		v, ok := ParseVersion(r.TagName)
		if !ok {
			continue
		}
		pre := r.Prerelease || v.IsPrerelease()
		if pre && !prereleases {
			continue
		}
		if best != nil && v.Cmp(bestV) <= 0 {
			continue
		}
		bestV = v
		best = &Release{Version: v.String(), Tag: r.TagName, Notes: r.Body, URL: r.HTMLURL, Prerelease: pre, Published: r.PublishedAt}
	}
	return best
}

// ReleaseURL is the page of a tag's release.
func ReleaseURL(tag string) string {
	return "https://github.com/" + Repo + "/releases/tag/" + tag
}

// Summary shortens Markdown release notes to at most maxLines plain lines:
// headings, blank lines, HTML comments and code blocks are dropped,
// blockquote markers are removed,
// list markers become "• ", and **bold**/`code` markers are removed.
func Summary(notes string, maxLines int) []string {
	var out []string
	inComment, inFence := false, false
	for _, line := range strings.Split(strings.ReplaceAll(notes, "\r\n", "\n"), "\n") {
		l := strings.TrimSpace(line)
		for strings.HasPrefix(l, ">") { // blockquotes read as plain text
			l = strings.TrimSpace(l[1:])
		}
		if strings.HasPrefix(l, "```") || strings.HasPrefix(l, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue // code is for the release page, not a summary
		}
		if inComment {
			if strings.Contains(l, "-->") {
				inComment = false
			}
			continue
		}
		if strings.HasPrefix(l, "<!--") {
			inComment = !strings.Contains(l, "-->")
			continue
		}
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "---") ||
			strings.HasPrefix(strings.ToLower(l), "**full changelog**") {
			continue
		}
		for _, m := range []string{"- ", "* ", "+ "} {
			if strings.HasPrefix(l, m) {
				l = "• " + strings.TrimSpace(l[len(m):])
				break
			}
		}
		l = strings.NewReplacer("**", "", "__", "", "`", "").Replace(l)
		l = mdLinkRE.ReplaceAllString(l, "$1")
		if strings.TrimSpace(strings.TrimPrefix(l, "•")) == "" {
			continue
		}
		out = append(out, l)
		if len(out) == maxLines {
			break
		}
	}
	return out
}

var mdLinkRE = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
