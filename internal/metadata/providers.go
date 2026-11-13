package metadata

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/pkg/config"
)

// IconProvider is a third-party favicon service. Fetch never uses one:
// FetchIconFrom is only called when the user picks a provider, for sites
// whose own icon can't be fetched.
type IconProvider string

const (
	IconGoogle     IconProvider = "google"
	IconDuckDuckGo IconProvider = "duckduckgo"
)

// Name is the provider as shown to people.
func (p IconProvider) Name() string {
	switch p {
	case IconGoogle:
		return "Google"
	case IconDuckDuckGo:
		return "DuckDuckGo"
	}
	return string(p)
}

// providerBases are where each provider is asked; tests point them at a
// local server.
var providerBases = map[IconProvider]string{
	IconGoogle:     "https://www.google.com/s2/favicons",
	IconDuckDuckGo: "https://icons.duckduckgo.com/ip3/",
}

// ErrNoProviderIcon means the provider had no icon for the site.
var ErrNoProviderIcon = errors.New("the icon service has no icon for this site")

func providerURL(p IconProvider, host string) (string, error) {
	base, ok := providerBases[p]
	if !ok {
		return "", fmt.Errorf("unknown icon provider %q", p)
	}
	switch p {
	case IconGoogle:
		return base + "?" + url.Values{"domain": {host}, "sz": {"256"}}.Encode(), nil
	default:
		return base + url.PathEscape(host) + ".ico", nil
	}
}

// FetchIconFrom asks provider for rawURL's icon: the site's own host first,
// then its registrable domain. The icon is decoded and re-encoded as a
// square PNG (≤512px), like Fetch's. Only the host name is sent to the
// provider. It returns ErrNoProviderIcon when the provider has none.
func FetchIconFrom(ctx context.Context, rawURL string, p IconProvider) ([]byte, error) {
	norm, err := config.NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(norm)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("no host in %q", rawURL)
	}
	hosts := []string{u.Hostname()}
	if r := config.RegistrableHost(norm); r != "" && !strings.EqualFold(r, hosts[0]) {
		hosts = append(hosts, r)
	}
	ctx, cancel := context.WithTimeout(ctx, totalBudget)
	defer cancel()
	client := newClient()
	for _, h := range hosts {
		pu, err := providerURL(p, h)
		if err != nil {
			return nil, err
		}
		b, _, status, err := get(ctx, client, pu, maxIconBytes, "image/png,image/*;q=0.8,*/*;q=0.5")
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Google answers an unknown site with 404 and a generic globe.
		if err != nil || status >= 400 || len(b) == 0 {
			continue
		}
		norm, err := icons.Normalize(b)
		if err != nil {
			continue
		}
		return icons.Resize(norm, iconOutputMax)
	}
	return nil, ErrNoProviderIcon
}
