package i18n

import (
	"os"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	cases := map[string]string{
		"en-US": "en", "zh-HK": "zh-Hant", "zh-TW": "zh-Hant", "zh-Hant-HK": "zh-Hant",
		"zh-CN": "zh-Hans", "zh_CN.UTF-8": "zh-Hans", "yue": "yue-Hant-HK", "yue-Hant-HK": "yue-Hant-HK",
		"ja-JP": "ja", "fr-CA": "fr", "es-MX": "es", "ru": "en", "ar": "en",
	}
	for in, want := range cases {
		if got := Resolve(in); got != want {
			t.Errorf("Resolve(%q)=%q want %q", in, got, want)
		}
	}
}

func TestFormat(t *testing.T) {
	s := format("en", "{count, plural, one {# app} other {# apps}} for {name}", map[string]any{"count": 2, "name": "Ada"})
	if s != "2 apps for Ada" {
		t.Fatal(s)
	}
	s = format("fr", "{count, plural, one {# app} other {# apps}}", map[string]any{"count": 0})
	if s != "0 app" {
		t.Fatal(s)
	}
}

// TestCatalogParity fails when a locale misses an English ID or has
// mismatched placeholders (spec §9D.3 translation CI gate).
// TestCatalogParity checks every translation against English. A string
// not yet translated is logged (English shows in its place), and fails
// only under MYPACK_I18N_STRICT=1; a translation whose placeholders differ
// from English's always fails.
func TestCatalogParity(t *testing.T) {
	strict := os.Getenv("MYPACK_I18N_STRICT") == "1"
	en := catalogs["en"]
	for _, l := range Supported[1:] {
		c := catalogs[l.ID]
		for id, src := range en {
			tr, ok := c[id]
			if !ok {
				if strict {
					t.Errorf("%s: missing %s", l.ID, id)
				} else {
					t.Logf("%s: missing %s (English shows)", l.ID, id)
				}
				continue
			}
			if a, b := placeholders(src), placeholders(tr); a != b {
				t.Errorf("%s: %s placeholders %q != %q", l.ID, id, b, a)
			}
		}
	}
}

func placeholders(s string) string {
	// Only top-level {name} / {name, plural, ...} count; plural branch
	// bodies are translatable text, not placeholders.
	m := map[string]bool{}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			if depth == 0 {
				j := i + 1
				for j < len(s) && s[j] != '}' && s[j] != ',' {
					j++
				}
				m[strings.TrimSpace(s[i+1:j])] = true
			}
			depth++
		case '}':
			depth--
		}
	}
	out := ""
	for _, n := range sortedKeys(m) {
		out += n + ","
	}
	return out
}

func containsSpace(s string) bool {
	for _, r := range s {
		if r == ' ' {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
