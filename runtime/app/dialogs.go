package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// ---- HTTP authentication (network.basicAuthUser) -------------------------
//
// MyGo's auth handler must answer synchronously and cannot show a dialog from
// the main thread, so an unknown challenge is cancelled, a small native
// credentials window is shown, and the page is reloaded once the user submits.
// Credentials are kept in memory only (never in mypack.json, never on disk).

func (a *App) onAuth(w *mygo.Window, req mygo.AuthRequest) mygo.AuthResponse {
	key := fmt.Sprintf("%t|%s|%d|%s|%s", req.IsProxy, strings.ToLower(req.Host), req.Port, req.Realm, req.Scheme)
	a.mu.Lock()
	defer a.mu.Unlock()
	if c, ok := a.creds[key]; ok {
		if time.Since(c.used) > 5*time.Second {
			c.used = time.Now()
			a.creds[key] = c
			return c.resp
		}
		delete(a.creds, key) // asked again right away: the server refused them
	}
	if !a.prompts[key] {
		a.prompts[key] = true
		go a.promptCredentials(w, req, key)
	}
	return mygo.AuthResponse{}
}

type credForm struct {
	host, realm string
	user, pass  string
	focusPass   bool
	submit      func(user, pass string)
	cancel      func()
}

func (f *credForm) view(c *ui.Context) {
	ui.Column(c).Fill().Padding(20).Gap(10).Children(func() {
		ui.Text(c, "Sign in to "+f.host).Bold()
		if f.realm != "" {
			ui.Text(c, f.realm).TextColor(c.Theme().TextMuted)
		}
		u := ui.TextInput(c, &f.user).Label("Username").Placeholder("Username")
		p := ui.TextInput(c, &f.pass).Label("Password").Placeholder("Password").Password()
		if f.focusPass {
			p.AutoFocus()
		} else {
			u.AutoFocus()
		}
		ui.Spacer(c)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "Cancel").Clicked() || c.Shortcut(0, ui.KeyEscape) {
				f.cancel()
			}
			if ui.PrimaryButton(c, "Sign In").Clicked() || u.Submitted() || p.Submitted() {
				f.submit(f.user, f.pass)
			}
		})
	})
}

func (a *App) promptCredentials(parent *mygo.Window, req mygo.AuthRequest, key string) {
	host := req.Host
	if req.IsProxy {
		host = "proxy " + host
	}
	var win *mygo.Window
	done := func() {
		a.mu.Lock()
		delete(a.prompts, key)
		a.mu.Unlock()
		if win != nil {
			go win.Close()
		}
	}
	f := &credForm{host: host, realm: req.Realm, user: a.cfg.Network.BasicAuthUser, focusPass: a.cfg.Network.BasicAuthUser != ""}
	f.cancel = done
	f.submit = func(u, p string) {
		a.mu.Lock()
		a.creds[key] = credEntry{resp: mygo.AuthResponse{Username: u, Password: p}}
		a.mu.Unlock()
		done()
		if parent != nil && !parent.IsDestroyed() {
			if pg := a.pageOf(parent); pg != nil {
				pg.Reload()
			}
		}
	}
	opts := mygo.WindowOptions{Title: "Sign In", Width: 380, Height: 240, DisableResize: true, Content: ui.View(f.view)}
	if parent != nil && !parent.IsDestroyed() {
		opts.Parent, opts.Modal = parent, true
	}
	win = mygo.NewWindow(opts)
	win.OnClosed(func() {
		a.mu.Lock()
		delete(a.prompts, key)
		a.mu.Unlock()
	})
}

// ---- Find in page (webview.findInPage) ------------------------------------

type findBar struct {
	page    *mygo.Page
	win     *mygo.Window
	query   string
	last    string
	matches int
	active  int
	case_   bool
	close   func()
}

func (f *findBar) run(next, backward bool) {
	q, mc := f.query, f.case_
	go func() {
		if q == "" {
			f.page.StopFindInPage()
			f.win.Update(func() { f.matches, f.active = 0, 0 })
			return
		}
		res, err := f.page.FindInPage(q, mygo.FindOptions{MatchCase: mc, Backward: backward, FindNext: next})
		if err != nil {
			return
		}
		f.win.Update(func() { f.matches, f.active = res.Matches, res.Active })
	}()
}

func (f *findBar) view(c *ui.Context) {
	ui.Row(c).Fill().Padding(10).Gap(8).Children(func() {
		in := ui.TextInput(c, &f.query).Label("Find").Placeholder("Find in page").AutoFocus().Grow(1)
		if in.Changed() {
			f.run(false, false)
		}
		if in.Submitted() {
			f.run(true, false)
		}
		switch {
		case f.query == "":
		case f.matches == 0:
			ui.Text(c, "No matches").TextColor(c.Theme().TextMuted)
		default:
			ui.Textf(c, "%d / %d", f.active, f.matches).TextColor(c.Theme().TextMuted)
		}
		if ui.Button(c, "Prev").Clicked() {
			f.run(true, true)
		}
		if ui.Button(c, "Next").Clicked() {
			f.run(true, false)
		}
		if ui.Button(c, "Done").Clicked() || c.Shortcut(0, ui.KeyEscape) {
			f.close()
		}
	})
}

func (a *App) openFind(win *mygo.Window) {
	w, p := a.pageFor(win)
	if p == nil {
		return
	}
	a.mu.Lock()
	if ex := a.finds[w]; ex != nil && !ex.IsDestroyed() {
		a.mu.Unlock()
		ex.Show()
		ex.Focus()
		return
	}
	a.mu.Unlock()
	f := &findBar{page: p}
	f.close = func() {
		p.StopFindInPage()
		a.mu.Lock()
		fw := a.finds[w]
		delete(a.finds, w)
		a.mu.Unlock()
		if fw != nil {
			go fw.Close()
		}
	}
	b := w.Bounds()
	fw := mygo.NewWindow(mygo.WindowOptions{
		Title: "Find", Width: 460, Height: 56, DisableResize: true, Parent: w,
		X: b.X + max(0, b.Width-480), Y: b.Y + 60,
		Content: ui.View(f.view),
	})
	f.win = fw
	a.mu.Lock()
	a.finds[w] = fw
	a.mu.Unlock()
	fw.OnClosed(func() {
		p.StopFindInPage()
		a.mu.Lock()
		if a.finds[w] == fw {
			delete(a.finds, w)
		}
		a.mu.Unlock()
	})
}
