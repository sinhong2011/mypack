//go:build mypack_lite

package app

// The Lite runtime: built with -tags mypack_lite, it leaves out MyGo's UI
// toolkit (ui, glass and the GPU, text and font code under them), which is
// most of the full runtime's size. Every window is a plain native window
// around one web page. The features that need native UI are not here:
//
//   - window.titleBarStyle hidden / overlay / glass (the strip, shell.go)
//   - window.tabs (tabs.go)
//   - app.group (group.go)
//   - webview.findInPage (the find bar, dialogs.go)
//
// The Builder picks the full runtime for a config that turns any of them on
// (config.Config.RuntimeTier), so this file only keeps the package
// compiling and degrades safely if a Lite runtime meets such a config.

import (
	"errors"
	"html"
	"log"
	"strings"

	"github.com/egoist/mygo"
)

// Lite reports whether this is the Lite runtime.
const Lite = true

type group struct {
	members      []*member
	localSchemes []string
	localOrigins []string
}

type member struct {
	home string
	page *mygo.Page
}

type tabSet struct{}

func (a *App) newGroup() (*group, error) {
	return nil, errors.New("site groups need the full MyPack runtime; rebuild the app with this version of MyPack")
}

func (g *group) safeDomains() []string          { return nil }
func (g *group) content() mygo.Content          { return nil }
func (g *group) attach(*mygo.Window) *mygo.Page { return nil }
func (g *group) focusMember(*mygo.Page) bool    { return false }
func (g *group) handleIncoming(string) bool     { return false }
func (t *tabSet) focusPage(*mygo.Page) bool     { return false }
func (t *tabSet) open(string, bool)             {}
func (a *App) groupMenuItems() []*mygo.MenuItem { return nil }
func (a *App) tabMenuItems() []*mygo.MenuItem   { return nil }
func (a *App) tabSwitchItems() []*mygo.MenuItem { return nil }
func (a *App) tabsOn() bool                     { return false }
func (a *App) useStrip() bool                   { return false }
func (a *App) stripHeight() int                 { return 0 }
func (a *App) homeOf(*mygo.Page) string         { return a.home }
func (a *App) openFind(*mygo.Window) {
	log.Printf("mypack-runtime: find in page needs the full runtime")
}
func (a *App) openGroupWindow(string, bool) (*mygo.Window, *mygo.Page) { return nil, nil }

func (a *App) openTabsWindow(string, string, bool) (*mygo.Window, *mygo.Page) { return nil, nil }

func (a *App) openStripWindow(o mygo.WindowOptions, url string) (*mygo.Window, *mygo.Page) {
	return nil, nil
}

// ---- HTTP authentication -------------------------------------------------
//
// The sign-in prompt is a small local web page. Its buttons report through
// the page title (a title change is an event MyGo delivers for any page,
// with no bridge); the fields are then read with Eval. Nothing leaves the
// process: the page has no network access and is never navigated.

const (
	credTitleSubmit = "mypack:credentials:submit"
	credTitleCancel = "mypack:credentials:cancel"
)

func openCredentialsWindow(opts mygo.WindowOptions, f *credForm) *mygo.Window {
	opts.Page = mygo.PageOptions{Session: mygo.NewEphemeralSession(), DevTools: mygo.DevToolsDisabled}
	win := mygo.NewWindow(opts)
	p := win.Page()
	if p == nil {
		f.cancel()
		return win
	}
	p.OnPageTitleUpdated(func(e *mygo.TitleEvent) {
		switch e.Title {
		case credTitleSubmit:
			e.PreventDefault()
			go func() {
				v, err := p.Eval(`return {u: document.getElementById("u").value, p: document.getElementById("p").value}`)
				m, ok := v.(map[string]any)
				if err != nil || !ok {
					mygo.RunOnMain(f.cancel)
					return
				}
				u, _ := m["u"].(string)
				pw, _ := m["p"].(string)
				mygo.RunOnMain(func() { f.submit(u, pw) })
			}()
		case credTitleCancel:
			e.PreventDefault()
			f.cancel()
		}
	})
	p.LoadHTML(credentialsHTML(f), "")
	return win
}

func credentialsHTML(f *credForm) string {
	realm := ""
	if f.realm != "" {
		realm = `<p class="muted">` + html.EscapeString(f.realm) + `</p>`
	}
	focusU, focusP := " autofocus", ""
	if f.focusPass {
		focusU, focusP = "", " autofocus"
	}
	r := strings.NewReplacer(
		"{{HOST}}", html.EscapeString(f.host),
		"{{REALM}}", realm,
		"{{USER}}", html.EscapeString(f.user),
		"{{FU}}", focusU,
		"{{FP}}", focusP,
		"{{SUBMIT}}", credTitleSubmit,
		"{{CANCEL}}", credTitleCancel,
	)
	return r.Replace(credentialsTemplate)
}

const credentialsTemplate = `<!doctype html>
<html><head><meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'">
<title>Sign In</title>
<style>
:root { color-scheme: light dark; font: 13px -apple-system, system-ui, "Segoe UI", sans-serif; }
body { margin: 0; padding: 18px 20px; display: flex; flex-direction: column; gap: 8px; height: 100vh; box-sizing: border-box; }
h1 { font-size: 14px; margin: 0; }
.muted { margin: 0; opacity: .65; }
input { font: inherit; padding: 5px 7px; border-radius: 6px; border: 1px solid #8886; background: transparent; color: inherit; }
.row { margin-top: auto; display: flex; gap: 8px; justify-content: flex-end; }
button { font: inherit; padding: 4px 14px; border-radius: 6px; border: 1px solid #8886; background: transparent; color: inherit; }
button.primary { background: #0a64d8; border-color: #0a64d8; color: #fff; }
</style></head>
<body>
<h1>Sign in to {{HOST}}</h1>
{{REALM}}
<form id="f">
<div style="display:flex;flex-direction:column;gap:8px">
<input id="u" placeholder="Username" aria-label="Username" autocomplete="username" value="{{USER}}"{{FU}}>
<input id="p" type="password" placeholder="Password" aria-label="Password" autocomplete="current-password"{{FP}}>
</div>
</form>
<div class="row"><button id="c" type="button">Cancel</button><button class="primary" type="submit" form="f">Sign In</button></div>
<script>
const say = t => { document.title = ""; document.title = t; };
document.getElementById("f").addEventListener("submit", e => { e.preventDefault(); say("{{SUBMIT}}"); });
document.getElementById("c").addEventListener("click", () => say("{{CANCEL}}"));
document.addEventListener("keydown", e => { if (e.key === "Escape") say("{{CANCEL}}"); });
</script>
</body></html>`
