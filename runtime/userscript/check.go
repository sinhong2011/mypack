package userscript

import (
	"fmt"
	"strings"
)

// Problem is something wrong with a script's header, on a line (0-based)
// of the source, for an editor to mark.
type Problem struct {
	Line    int
	Message string
	Warning bool // the script still installs
}

// Check finds the header's problems: no header, no @name, a @match that is
// not a match pattern, an unknown @run-at, a request grant without
// @connect. Errors stop Install; warnings do not.
func Check(src string) []Problem {
	lines := strings.Split(src, "\n")
	open, close := -1, -1
	for i, l := range lines {
		t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "//"))
		if !strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		if open < 0 && t == "==UserScript==" {
			open = i
		} else if open >= 0 && t == "==/UserScript==" {
			close = i
			break
		}
	}
	if open < 0 {
		return []Problem{{Line: 0, Message: "No // ==UserScript== header: start the file with one."}}
	}
	if close < 0 {
		return []Problem{{Line: open, Message: "The header is not closed: add // ==/UserScript== after it."}}
	}
	var out []Problem
	name, xhr, connect := -1, -1, false
	for i := open + 1; i < close; i++ {
		g := metaLine.FindStringSubmatch(strings.TrimSpace(lines[i]))
		if g == nil {
			continue
		}
		key, val := g[1], strings.TrimSpace(g[2])
		switch key {
		case "name":
			if val != "" {
				name = i
			}
		case "match", "exclude-match":
			if _, err := MatchPattern(val); err != nil {
				out = append(out, Problem{Line: i, Message: err.Error()})
			}
		case "run-at":
			switch val {
			case "document-start", "document-end", "document-idle", "document-body":
			default:
				out = append(out, Problem{Line: i, Message: fmt.Sprintf("Unknown @run-at %q: it runs at document-idle.", val), Warning: true})
			}
		case "grant":
			n := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(val), "gm_"), "gm."))
			if n == "xmlhttprequest" {
				xhr = i
			}
		case "connect":
			connect = true
		}
	}
	if name < 0 {
		out = append(out, Problem{Line: open, Message: "The header has no @name."})
	}
	if xhr >= 0 && !connect {
		out = append(out, Problem{Line: xhr, Message: "No @connect: requests can only reach the page's own site.", Warning: true})
	}
	return out
}

// HasErrors reports whether any problem stops an install.
func HasErrors(ps []Problem) bool {
	for _, p := range ps {
		if !p.Warning {
			return true
		}
	}
	return false
}
