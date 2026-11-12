package app

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"github.com/egoist/mygo"
	"mypack.dev/mypack/pkg/config"
)

// Content blocking (webview.blocking, runtime 1.0.7): the config becomes
// WebKit content-blocker rules, compiled once at launch, before the first
// page loads (the engine keeps the compiled list, so later launches load it
// without compiling), and applied to every page of the app.
//
// The tracker list is mypack's own short list of well-known ad and tracker
// hosts, not EasyList (whose licence would carry over to every generated
// app). Each host is blocked, with its subdomains, only when a page loads it
// from another site, so a site's own pages keep working.

// blockedHosts are common ad, tracker and analytics hosts.
var blockedHosts = []string{
	// Ad exchanges and networks
	"doubleclick.net", "googlesyndication.com", "googleadservices.com", "adservice.google.com",
	"adnxs.com", "adsrvr.org", "advertising.com", "criteo.com", "criteo.net", "taboola.com",
	"outbrain.com", "rubiconproject.com", "pubmatic.com", "openx.net", "casalemedia.com",
	"indexww.com", "amazon-adsystem.com", "adform.net", "smartadserver.com", "moatads.com",
	"yieldmo.com", "sharethrough.com", "triplelift.com", "3lift.com", "33across.com", "sovrn.com",
	"lijit.com", "gumgum.com", "media.net", "contextweb.com", "spotxchange.com", "spotx.tv",
	"teads.tv", "adcolony.com", "applovin.com", "inmobi.com", "bidswitch.net", "mathtag.com",
	"zedo.com", "revcontent.com", "mgid.com", "propellerads.com", "popads.net", "popcash.net",
	"adsterra.com", "exoclick.com", "juicyads.com", "trafficjunky.net", "zergnet.com",
	"undertone.com", "yieldlab.net", "adition.com", "smaato.net", "adroll.com",
	"perfectaudience.com", "steelhousemedia.com", "simpli.fi", "serving-sys.com",
	"flashtalking.com", "springserve.com", "tremorhub.com", "unrulymedia.com", "emxdgt.com",
	"dotomi.com", "adhigh.net", "ads.yahoo.com", "adtech.de", "smartclip.net", "yieldlove.com",
	"adkernel.com", "districtm.io", "onetag-sys.com", "rhythmone.com", "33across.com",
	// Data brokers and identity sync
	"rlcdn.com", "demdex.net", "everesttech.net", "krxd.net", "bluekai.com", "exelator.com",
	"agkn.com", "tapad.com", "liveintent.com", "liveramp.com", "id5-sync.com", "crwdcntrl.net",
	"eyeota.net", "adsymptotic.com", "omtrdc.net", "2o7.net",
	// Verification and measurement
	"adsafeprotected.com", "doubleverify.com", "scorecardresearch.com", "imrworldwide.com",
	"quantserve.com", "quantcount.com", "gemius.pl", "comscore.com",
	// Analytics and session recording
	"google-analytics.com", "googletagmanager.com", "googletagservices.com",
	"hotjar.com", "hotjar.io", "mouseflow.com", "fullstory.com", "crazyegg.com",
	"luckyorange.com", "luckyorange.net", "clarity.ms", "mixpanel.com", "segment.com", "segment.io",
	"amplitude.com", "heapanalytics.com", "kissmetrics.com", "nr-data.net", "chartbeat.com",
	"chartbeat.net", "statcounter.com", "histats.com", "mc.yandex.ru", "hm.baidu.com",
	"cxense.com", "bounceexchange.com", "bouncex.net", "hs-analytics.net",
	"munchkin.marketo.net", "pi.pardot.com", "appsflyer.com", "kochava.com",
	// Social pixels (never the sign-in SDKs, which sign-ins need)
	"bat.bing.com", "analytics.twitter.com", "static.ads-twitter.com", "analytics.tiktok.com",
	"tr.snapchat.com", "ct.pinterest.com", "px.ads.linkedin.com", "alb.reddit.com",
	"events.redditmedia.com", "q.quora.com",
	// Share widgets
	"addthis.com", "sharethis.com",
}

// blockedPaths are tracker endpoints on hosts the app may need otherwise:
// a URL-filter (WebKit regular expression) each.
var blockedPaths = []string{
	`^https?://(www\.)?facebook\.com/tr[/?]`,
	`^https?://connect\.facebook\.net/[^/]+/fbevents\.js`,
	`^https?://(www\.)?google\.com/pagead/`,
	`^https?://(www\.)?youtube\.com/api/stats/ads`,
}

// cookieBannerSelectors are the banners and overlays of common consent
// tools.
var cookieBannerSelectors = []string{
	"#onetrust-consent-sdk", "#onetrust-banner-sdk", "#CybotCookiebotDialog",
	"#CybotCookiebotDialogBodyUnderlay", ".qc-cmp2-container", "#qc-cmp2-container",
	"#truste-consent-track", ".truste_overlay", ".truste_box_overlay", "#didomi-host",
	"#usercentrics-root", ".osano-cm-window", "#cmplz-cookiebanner-container", ".cky-consent-container",
	".cky-overlay", "#termly-code-snippet-support", "#iubenda-cs-banner", "div[id^=\"sp_message_container\"]",
	".klaro .cookie-notice", "#cookie-law-info-bar", "#CookieConsent", ".cc-window.cc-banner",
	"#gdpr-cookie-message", "#cookie-notice", ".cookie-banner", "#cookie-banner", "#cookiebanner",
	".fc-consent-root", "#hs-eu-cookie-confirmation", "#BorlabsCookieBox", "#moove_gdpr_cookie_info_bar",
	"#cookiescript_injected", "#ccc", ".evidon-banner", "#_evidon_banner", "#axeptio_overlay",
}

type blockRule struct {
	Trigger blockTrigger `json:"trigger"`
	Action  blockAction  `json:"action"`
}

type blockTrigger struct {
	URLFilter string   `json:"url-filter"`
	LoadType  []string `json:"load-type,omitempty"`
	IfDomain  []string `json:"if-domain,omitempty"`
}

type blockAction struct {
	Type     string `json:"type"`
	Selector string `json:"selector,omitempty"`
}

// blockingRules turns b into WebKit content-blocker JSON ("" when it blocks
// nothing). Rules run in order: trackers, banners, then the allow list
// undoes both on its sites, and last the app's own hide list applies
// everywhere.
func blockingRules(b *config.BlockingConfig) string {
	if !b.Active() {
		return ""
	}
	var rules []blockRule
	if b.Trackers {
		seen := map[string]bool{}
		for _, h := range blockedHosts {
			if seen[h] {
				continue
			}
			seen[h] = true
			rules = append(rules, blockRule{
				Trigger: blockTrigger{URLFilter: `^[a-z]+://([^/:]+\.)?` + regexp.QuoteMeta(h) + `[:/]`, LoadType: []string{"third-party"}},
				Action:  blockAction{Type: "block"},
			})
		}
		for _, f := range blockedPaths {
			rules = append(rules, blockRule{Trigger: blockTrigger{URLFilter: f, LoadType: []string{"third-party"}}, Action: blockAction{Type: "block"}})
		}
	}
	if b.CookieBanners {
		rules = append(rules, blockRule{Trigger: blockTrigger{URLFilter: ".*"},
			Action: blockAction{Type: "css-display-none", Selector: strings.Join(cookieBannerSelectors, ", ")}})
	}
	if len(rules) > 0 && len(b.Allow) > 0 {
		var doms []string
		for _, d := range b.Allow {
			if d = config.NormalizeAllowDomain(d); d != "" {
				doms = append(doms, "*"+d)
			}
		}
		if len(doms) > 0 {
			rules = append(rules, blockRule{Trigger: blockTrigger{URLFilter: ".*", IfDomain: doms}, Action: blockAction{Type: "ignore-previous-rules"}})
		}
	}
	var hide []string
	for _, s := range b.Hide {
		if s = strings.TrimSpace(s); s != "" && !strings.ContainsAny(s, "{}") {
			hide = append(hide, s)
		}
	}
	if len(hide) > 0 {
		rules = append(rules, blockRule{Trigger: blockTrigger{URLFilter: ".*"},
			Action: blockAction{Type: "css-display-none", Selector: strings.Join(hide, ", ")}})
	}
	if len(rules) == 0 {
		return ""
	}
	out, err := json.Marshal(rules)
	if err != nil {
		return ""
	}
	return string(out)
}

// compileBlocking compiles the app's rules (main thread, before the first
// page; the app keeps handling events while it waits). A list the engine
// refuses is logged and the app runs without blocking.
func (a *App) compileBlocking() {
	rules := blockingRules(a.cfg.WebView.Blocking)
	if rules == "" {
		return
	}
	cr, err := mygo.CompileContentRules("mypack-blocking", rules)
	if err != nil {
		log.Printf("mypack-runtime: content blocking: %v", err)
		return
	}
	a.mu.Lock()
	a.blocking = cr
	a.mu.Unlock()
}
