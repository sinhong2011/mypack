package app

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"mypack.dev/mypack/pkg/config"
)

func TestBlockingRules(t *testing.T) {
	if blockingRules(nil) != "" || blockingRules(&config.BlockingConfig{Allow: []string{"a.com"}}) != "" {
		t.Fatal("nothing to block makes no rules")
	}
	b := &config.BlockingConfig{Trackers: true, CookieBanners: true, Allow: []string{"bank.example"}, Hide: []string{".promo", "#ad-rail"}}
	var rules []blockRule
	if err := json.Unmarshal([]byte(blockingRules(b)), &rules); err != nil {
		t.Fatal(err)
	}
	n := len(rules)
	if n < 100 {
		t.Fatalf("only %d rules", n)
	}
	// Order: trackers, banners, the allow list's undo, then the app's hide list.
	if rules[n-1].Action.Type != "css-display-none" || rules[n-1].Action.Selector != ".promo, #ad-rail" {
		t.Fatalf("last rule = %+v", rules[n-1])
	}
	if rules[n-2].Action.Type != "ignore-previous-rules" || rules[n-2].Trigger.IfDomain[0] != "*bank.example" {
		t.Fatalf("allow rule = %+v", rules[n-2])
	}
	if rules[n-3].Action.Type != "css-display-none" || !strings.Contains(rules[n-3].Action.Selector, "#onetrust-consent-sdk") {
		t.Fatalf("banner rule = %+v", rules[n-3])
	}
	// Tracker rules: third-party only, and they match the host and its
	// subdomains but not look-alikes.
	var dc *regexp.Regexp
	for _, r := range rules {
		if r.Action.Type != "block" {
			continue
		}
		if len(r.Trigger.LoadType) != 1 || r.Trigger.LoadType[0] != "third-party" {
			t.Fatalf("not third-party only: %+v", r)
		}
		re, err := regexp.Compile("(?i)" + r.Trigger.URLFilter)
		if err != nil {
			t.Fatalf("%s: %v", r.Trigger.URLFilter, err)
		}
		if strings.Contains(r.Trigger.URLFilter, `doubleclick\.net`) {
			dc = re
		}
	}
	for u, want := range map[string]bool{
		"https://doubleclick.net/x":             true,
		"https://ad.doubleclick.net/ddm/x":      true,
		"https://stats.g.doubleclick.net:443/":  true,
		"https://notdoubleclick.net/":           false,
		"https://doubleclick.net.example.com/":  false,
		"https://example.com/?doubleclick.net/": false,
	} {
		if dc.MatchString(u) != want {
			t.Errorf("%s: blocked = %v", u, !want)
		}
	}
}

func TestBlockingOnlyHide(t *testing.T) {
	var rules []blockRule
	if err := json.Unmarshal([]byte(blockingRules(&config.BlockingConfig{Hide: []string{".x"}, Allow: []string{"a.com"}})), &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Action.Selector != ".x" {
		t.Fatalf("rules = %+v", rules)
	}
}
