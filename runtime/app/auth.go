package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
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
	opts := mygo.WindowOptions{Title: "Sign In", Width: 380, Height: 240, DisableResize: true}
	if parent != nil && !parent.IsDestroyed() {
		opts.Parent, opts.Modal = parent, true
	}
	win = openCredentialsWindow(opts, f)
	win.OnClosed(func() {
		a.mu.Lock()
		delete(a.prompts, key)
		a.mu.Unlock()
	})
}
