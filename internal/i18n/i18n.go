// Package i18n provides mypack's seven UI locales (spec §9D, ADR-031):
// en, zh-Hant, zh-Hans, yue-Hant-HK, es, fr, ja.
//
// Catalogs are flat JSON objects in catalogs/<locale>.json mapping stable
// message IDs ("create.title") to ICU-lite strings with named placeholders
// ("Build for This {device}"). Plurals use "{count, plural, one {# app} other {# apps}}".
// Message IDs, never display text, are used as keys anywhere machine-facing.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

//go:embed catalogs/*.json
var catalogFS embed.FS

// Locale describes one supported UI language.
type Locale struct {
	ID       string // BCP 47
	Autonym  string // self-name shown in the language picker
	English  string
}

// Supported lists the seven locales in picker order.
var Supported = []Locale{
	{"en", "English", "English"},
	{"zh-Hant", "繁體中文", "Traditional Chinese"},
	{"zh-Hans", "简体中文", "Simplified Chinese"},
	{"yue-Hant-HK", "粵語（香港）", "Cantonese (Hong Kong)"},
	{"es", "Español", "Spanish"},
	{"fr", "Français", "French"},
	{"ja", "日本語", "Japanese"},
}

var (
	mu       sync.RWMutex
	catalogs = map[string]map[string]string{}
	current  atomic.Value // string
	// MissingHook, when set (dev/CI), is called for missing IDs.
	MissingHook func(locale, id string)
)

func init() {
	for _, l := range Supported {
		b, err := catalogFS.ReadFile("catalogs/" + l.ID + ".json")
		if err != nil {
			continue
		}
		m := map[string]string{}
		if err := json.Unmarshal(b, &m); err == nil {
			catalogs[l.ID] = m
		}
	}
	current.Store("en")
}

// Resolve maps an OS/browser locale tag to a supported locale (exact →
// script/region family → en). zh-HK/zh-TW/zh-MO → zh-Hant (never silently
// Cantonese); zh-CN/zh-SG → zh-Hans; yue* → yue-Hant-HK.
func Resolve(tag string) string {
	t := strings.ReplaceAll(strings.TrimSpace(tag), "_", "-")
	if i := strings.IndexAny(t, ".@"); i >= 0 {
		t = t[:i]
	}
	low := strings.ToLower(t)
	for _, l := range Supported {
		if strings.ToLower(l.ID) == low {
			return l.ID
		}
	}
	switch {
	case strings.HasPrefix(low, "yue"):
		return "yue-Hant-HK"
	case strings.HasPrefix(low, "zh"):
		if strings.Contains(low, "hans") || strings.HasSuffix(low, "-cn") || strings.HasSuffix(low, "-sg") || low == "zh" {
			return "zh-Hans"
		}
		return "zh-Hant"
	}
	base := low
	if i := strings.Index(base, "-"); i > 0 {
		base = base[:i]
	}
	for _, l := range Supported {
		if l.ID == base {
			return l.ID
		}
	}
	return "en"
}

// SystemLocale best-effort reads the OS preferred language.
func SystemLocale() string {
	for _, k := range []string{"MYPACK_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" && v != "C" && v != "POSIX" {
			return v
		}
	}
	return systemLocale()
}

// SetLocale sets the active locale ("system" resolves the OS language).
func SetLocale(id string) {
	if id == "" || id == "system" {
		id = SystemLocale()
	}
	current.Store(Resolve(id))
}

// Current returns the active locale ID.
func Current() string { return current.Load().(string) }

// T translates id in the active locale. args are name/value pairs:
// T("build.cta", "device", "Mac").
func T(id string, args ...any) string { return TL(Current(), id, args...) }

// TL translates id in a specific locale.
func TL(locale, id string, args ...any) string {
	mu.RLock()
	s, ok := catalogs[locale][id]
	if !ok {
		if MissingHook != nil && locale != "en" {
			MissingHook(locale, id)
		}
		s, ok = catalogs["en"][id]
	}
	mu.RUnlock()
	if !ok {
		if MissingHook != nil {
			MissingHook("en", id)
		}
		return id
	}
	if len(args) == 0 {
		return s
	}
	vals := map[string]any{}
	for i := 0; i+1 < len(args); i += 2 {
		vals[fmt.Sprint(args[i])] = args[i+1]
	}
	return format(locale, s, vals)
}

// Has reports whether id exists in locale's catalog.
func Has(locale, id string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := catalogs[locale][id]
	return ok
}

// IDs returns all English message IDs.
func IDs() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(catalogs["en"]))
	for k := range catalogs["en"] {
		out = append(out, k)
	}
	return out
}

// format substitutes {name} and {name, plural, one {...} other {...}}.
func format(locale, s string, vals map[string]any) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '{' {
			b.WriteByte(s[i])
			i++
			continue
		}
		end := matchBrace(s, i)
		if end < 0 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(expand(locale, s[i+1:end], vals))
		i = end + 1
	}
	return b.String()
}

func matchBrace(s string, i int) int {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

func expand(locale, body string, vals map[string]any) string {
	parts := strings.SplitN(body, ",", 3)
	name := strings.TrimSpace(parts[0])
	v, ok := vals[name]
	if len(parts) < 3 || strings.TrimSpace(parts[1]) != "plural" {
		if !ok {
			return "{" + body + "}"
		}
		return fmt.Sprint(v)
	}
	n, _ := strconv.ParseFloat(fmt.Sprint(v), 64)
	forms := parseForms(parts[2])
	cat := pluralCategory(locale, n)
	f, ok := forms["="+strconv.FormatFloat(n, 'f', -1, 64)]
	if !ok {
		f, ok = forms[cat]
	}
	if !ok {
		f = forms["other"]
	}
	f = strings.ReplaceAll(f, "#", strconv.FormatFloat(n, 'f', -1, 64))
	return format(locale, f, vals)
}

func parseForms(s string) map[string]string {
	out := map[string]string{}
	s = strings.TrimSpace(s)
	for len(s) > 0 {
		i := strings.IndexByte(s, '{')
		if i < 0 {
			break
		}
		key := strings.TrimSpace(s[:i])
		end := matchBrace(s, i)
		if end < 0 {
			break
		}
		out[key] = s[i+1 : end]
		s = strings.TrimSpace(s[end+1:])
	}
	return out
}

// pluralCategory implements CLDR cardinal rules for our locales.
func pluralCategory(locale string, n float64) string {
	switch locale {
	case "zh-Hant", "zh-Hans", "yue-Hant-HK", "ja":
		return "other"
	case "fr":
		if n >= 0 && n < 2 {
			return "one"
		}
		return "other"
	default: // en, es
		if n == 1 {
			return "one"
		}
		return "other"
	}
}
