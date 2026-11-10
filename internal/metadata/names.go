package metadata

import (
	"net"
	"strings"
	"unicode"
)

// knownNames keeps the canonical casing of popular sites, keyed by the
// registrable label ("github") or the full host ("mail.google.com").
var knownNames = map[string]string{
	"github": "GitHub", "gitlab": "GitLab", "bitbucket": "Bitbucket", "youtube": "YouTube",
	"chatgpt": "ChatGPT", "openai": "OpenAI", "claude": "Claude", "anthropic": "Anthropic",
	"notion": "Notion", "figma": "Figma", "slack": "Slack", "discord": "Discord",
	"whatsapp": "WhatsApp", "x": "X", "twitter": "X", "gmail": "Gmail", "linear": "Linear",
	"spotify": "Spotify", "twitch": "Twitch", "reddit": "Reddit", "linkedin": "LinkedIn",
	"trello": "Trello", "vercel": "Vercel", "netlify": "Netlify", "facebook": "Facebook",
	"instagram": "Instagram", "telegram": "Telegram", "netflix": "Netflix", "airtable": "Airtable",
	"asana": "Asana", "zoom": "Zoom", "dropbox": "Dropbox", "canva": "Canva", "miro": "Miro",
	"stackoverflow": "Stack Overflow", "medium": "Medium", "wikipedia": "Wikipedia",
	"tiktok": "TikTok", "pinterest": "Pinterest", "deepseek": "DeepSeek", "perplexity": "Perplexity",
	"bilibili": "Bilibili", "zhihu": "Zhihu", "google": "Google", "microsoft": "Microsoft",
	"outlook": "Outlook", "amazon": "Amazon", "apple": "Apple", "icloud": "iCloud",
	"jira": "Jira", "atlassian": "Atlassian", "clickup": "ClickUp", "evernote": "Evernote",
	"todoist": "Todoist", "soundcloud": "SoundCloud", "duckduckgo": "DuckDuckGo", "gemini": "Gemini",

	"mail.google.com":     "Gmail",
	"calendar.google.com": "Google Calendar",
	"docs.google.com":     "Google Docs",
	"drive.google.com":    "Google Drive",
	"maps.google.com":     "Google Maps",
	"meet.google.com":     "Google Meet",
	"music.youtube.com":   "YouTube Music",
	"chat.openai.com":     "ChatGPT",
	"web.whatsapp.com":    "WhatsApp",
	"web.telegram.org":    "Telegram",
	"app.slack.com":       "Slack",
	"outlook.live.com":    "Outlook",
	"outlook.office.com":  "Outlook",
}

var twoPartTLDs = map[string]bool{"co": true, "com": true, "org": true, "net": true, "gov": true, "edu": true, "ac": true}

// brandLabel returns the second-level label of a host ("www.github.com" → "github").
func brandLabel(host string) string {
	host = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "www.")
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return host
	}
	n := len(parts)
	if n >= 3 && len(parts[n-1]) == 2 && twoPartTLDs[parts[n-2]] {
		return parts[n-3]
	}
	return parts[n-2]
}

func brandKey(host string) string { return normKey(brandLabel(host)) }

func knownName(host string) string {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	if n, ok := knownNames[host]; ok {
		return n
	}
	return knownNames[brandLabel(host)]
}

// GuessTitleFromHost suggests an app name from a host name alone:
// "github.com" → "GitHub", "my-notes.example.org" → "Example".
func GuessTitleFromHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return "App"
	}
	if net.ParseIP(host) != nil {
		return host
	}
	if n := knownName(host); n != "" {
		return n
	}
	label := brandLabel(host)
	if label == "" {
		return host
	}
	words := strings.FieldsFunc(label, func(r rune) bool { return r == '-' || r == '_' })
	for i, w := range words {
		rs := []rune(w)
		rs[0] = unicode.ToUpper(rs[0])
		words[i] = string(rs)
	}
	return strings.Join(words, " ")
}
