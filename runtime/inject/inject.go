// Package inject builds the preload script the runtime installs in every page:
// user JS injections, user CSS (kept alive), a drag strip for hidden title
// bars, and page-level disabled shortcuts.
package inject

import (
	"encoding/json"
	"sort"
	"strings"
)

// Options feeds Build.
type Options struct {
	JS  []string // user scripts, concatenated in order
	CSS []string // user stylesheets, concatenated in order
	// DragStrip adds a window-drag region at the top of every page (hidden /
	// overlay / frameless title bars). Height is in CSS px (0 = use
	// --mygo-titlebar-height, falling back to 28).
	DragStrip bool
	// DisabledShortcuts are accelerators such as "CmdOrCtrl+R" that the page
	// must not receive nor act on.
	DisabledShortcuts []string
	// Web APIs the platform webview lacks (Notification, Badging API).
	WebAPIs WebAPIs
	// Media reports what the page plays, for the media keys (MediaScript).
	Media bool
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	// Keep the script safe inside inline contexts.
	r := strings.NewReplacer("<", `<`, ">", `>`, " ", ` `, " ", ` `)
	return r.Replace(string(b))
}

// Build returns the preload script ("" when there is nothing to inject).
func Build(o Options) string {
	var b strings.Builder
	// The shims go first so user scripts see the finished APIs.
	if shim := WebAPIsScript(o.WebAPIs); shim != "" {
		b.WriteString("try {\n")
		b.WriteString(shim)
		b.WriteString("\n} catch (e) { console.error('[mypack] web API shim failed:', e); }\n")
	}
	if o.Media {
		b.WriteString("try {\n")
		b.WriteString(MediaScript())
		b.WriteString("\n} catch (e) { console.error('[mypack] media shim failed:', e); }\n")
	}
	for _, js := range o.JS {
		if strings.TrimSpace(js) == "" {
			continue
		}
		// Each user script runs in its own try block so one failure does not
		// stop the others.
		b.WriteString("try {\n")
		b.WriteString(js)
		b.WriteString("\n} catch (e) { console.error('[mypack] injected script failed:', e); }\n")
	}
	css := strings.TrimSpace(strings.Join(o.CSS, "\n"))
	if css != "" {
		b.WriteString(cssScript(css))
	}
	if o.DragStrip {
		b.WriteString(dragScript)
	}
	if specs := shortcutSpecs(o.DisabledShortcuts); specs != "" {
		b.WriteString(shortcutScript(specs))
	}
	return b.String()
}

func cssScript(css string) string {
	return `(function () {
  var CSS = ` + jsString(css) + `;
  function ensure() {
    var root = document.head || document.documentElement;
    if (!root) return;
    var el = document.getElementById('__mypack_css');
    if (!el) {
      el = document.createElement('style');
      el.id = '__mypack_css';
      el.textContent = CSS;
      root.appendChild(el);
    } else if (!el.isConnected) {
      root.appendChild(el);
    }
  }
  function start() {
    ensure();
    // Keep the style present when single-page apps replace <head>.
    try {
      new MutationObserver(function () {
        if (!document.getElementById('__mypack_css')) ensure();
      }).observe(document.documentElement, { childList: true, subtree: true });
    } catch (e) {}
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
  ensure();
})();
`
}

const dragScript = `(function () {
  if (window.top !== window) return;
  function add() {
    if (document.getElementById('__mypack_drag')) return;
    var s = document.createElement('div');
    s.id = '__mypack_drag';
    s.setAttribute('aria-hidden', 'true');
    s.style.cssText = 'position:fixed;top:0;left:var(--mygo-titlebar-inset-left,0px);right:var(--mygo-titlebar-inset-right,0px);' +
      'height:min(var(--mygo-titlebar-height,28px),28px);z-index:2147483647;background:transparent;' +
      '--app-region:drag;app-region:drag;-webkit-app-region:drag;-webkit-user-select:none;user-select:none;';
    (document.body || document.documentElement).appendChild(s);
  }
  function start() {
    add();
    try { new MutationObserver(add).observe(document.documentElement, { childList: true }); } catch (e) {}
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
})();
`

type shortcut struct {
	CmdOrCtrl bool   `json:"cmdOrCtrl,omitempty"`
	Cmd       bool   `json:"cmd,omitempty"`
	Ctrl      bool   `json:"ctrl,omitempty"`
	Alt       bool   `json:"alt,omitempty"`
	Shift     bool   `json:"shift,omitempty"`
	Key       string `json:"key"`
}

func parse(acc string) (shortcut, bool) {
	var s shortcut
	parts := strings.Split(strings.TrimSpace(acc), "+")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" && i == len(parts)-1 && i > 0 { // trailing "+" => the plus key
			s.Key = "+"
			continue
		}
		switch strings.ToLower(p) {
		case "cmdorctrl", "commandorcontrol":
			s.CmdOrCtrl = true
		case "cmd", "command", "super", "meta", "win":
			s.Cmd = true
		case "ctrl", "control":
			s.Ctrl = true
		case "alt", "option", "altgr":
			s.Alt = true
		case "shift":
			s.Shift = true
		default:
			if s.Key != "" {
				return s, false
			}
			s.Key = strings.ToLower(p)
			switch s.Key {
			case "plus":
				s.Key = "+"
			case "esc":
				s.Key = "escape"
			case "return":
				s.Key = "enter"
			case "del":
				s.Key = "delete"
			case "space":
				s.Key = " "
			}
		}
	}
	return s, s.Key != ""
}

// Normalize returns a canonical form of an accelerator for comparisons
// ("cmdorctrl+shift+r"), so menu accelerators can be matched against
// webview.disabledShortcuts regardless of case, order and aliases.
func Normalize(acc string) string {
	s, ok := parse(acc)
	if !ok {
		return strings.ToLower(strings.TrimSpace(acc))
	}
	var mods []string
	if s.CmdOrCtrl {
		mods = append(mods, "cmdorctrl")
	}
	if s.Cmd {
		mods = append(mods, "cmd")
	}
	if s.Ctrl {
		mods = append(mods, "ctrl")
	}
	if s.Alt {
		mods = append(mods, "alt")
	}
	if s.Shift {
		mods = append(mods, "shift")
	}
	sort.Strings(mods)
	return strings.Join(append(mods, s.Key), "+")
}

func shortcutSpecs(list []string) string {
	var out []shortcut
	for _, a := range list {
		if s, ok := parse(a); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return ""
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func shortcutScript(specs string) string {
	return `(function () {
  var SPECS = ` + specs + `;
  var mac = /Mac|iPhone|iPad/.test(navigator.platform || '');
  function match(e, s) {
    var k = (e.key || '').toLowerCase();
    if (k !== s.key && (e.code || '').toLowerCase() !== 'key' + s.key) return false;
    var meta = !!s.cmd || (!!s.cmdOrCtrl && mac);
    var ctrl = !!s.ctrl || (!!s.cmdOrCtrl && !mac);
    return e.metaKey === meta && e.ctrlKey === ctrl && e.altKey === !!s.alt && e.shiftKey === !!s.shift;
  }
  window.addEventListener('keydown', function (e) {
    for (var i = 0; i < SPECS.length; i++) {
      if (match(e, SPECS[i])) { e.preventDefault(); e.stopImmediatePropagation(); return; }
    }
  }, true);
})();
`
}
