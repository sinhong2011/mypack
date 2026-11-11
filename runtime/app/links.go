package app

import (
	"log"
	"net/url"
	"runtime"
	"strings"

	"mypack.dev/mypack/runtime/navigation"
	"mypack.dev/mypack/runtime/routing"
)

// openLink opens an http(s) link that leaves this app: in the Mypack app that
// owns its host when link routing finds one, else in the default browser.
func (a *App) openLink(raw string) {
	if e, ok := a.routeTarget(raw); ok {
		name, args, err := routing.Command(runtime.GOOS, e, raw)
		if err == nil {
			log.Printf("mypack-runtime: link-routing: %s -> %s (%s %s)", hostOf(raw), e.Identifier, name, redactArgs(args, raw))
			go func() {
				if err := routing.Start(name, args...); err != nil {
					log.Printf("mypack-runtime: link-routing: cannot start %s: %v; using the browser", e.Identifier, err)
					openExternal(raw)
				}
			}()
			return
		}
		log.Printf("mypack-runtime: link-routing: %v; using the browser", err)
	}
	openExternal(raw)
}

// routeTarget returns the other installed Mypack app that owns raw's host.
func (a *App) routeTarget(raw string) (routing.Entry, bool) {
	if !a.cfg.Native.LinkRoutingEnabled() {
		return routing.Entry{}, false
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return routing.Entry{}, false
	}
	return a.routes.Lookup(u.Hostname(), a.cfg.App.Identifier)
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Host
	}
	return "?"
}

// redactArgs keeps URLs out of logs (paths and queries can be private).
func redactArgs(args []string, raw string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if a == raw {
			a = "<url:" + hostOf(raw) + ">"
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}

// firstWebURL returns the first http(s) URL among command-line arguments.
func firstWebURL(args []string) string {
	for _, a := range args {
		if isWebURL(a) {
			return a
		}
	}
	return ""
}

func isWebURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && !strings.ContainsAny(s, " \t\r\n")
}

// incomingAction decides what to do with a URL another app handed to this one:
// open it in the main window when it belongs to this app, else in the
// browser. It is never routed on to a third app (no hand-off loops).
func (a *App) incomingAction(raw string) navigation.Action {
	if !isWebURL(raw) || a.policy == nil {
		return navigation.Block
	}
	// Treated as a user click: the user followed a link in another app.
	switch a.policy.Navigate(raw, true) {
	case navigation.Internal:
		return navigation.Internal
	default:
		return navigation.External
	}
}

// handleIncomingURL receives a URL from OnOpenURL / a second instance.
func (a *App) handleIncomingURL(raw string) {
	if a.group != nil {
		a.mu.Lock()
		started := a.started
		a.mu.Unlock()
		if started && a.group.handleIncoming(raw) {
			return
		}
	}
	act := a.incomingAction(raw)
	log.Printf("mypack-runtime: incoming url host=%s -> %s", hostOf(raw), act)
	switch act {
	case navigation.Internal:
		a.mu.Lock()
		started := a.started
		if !started {
			a.startURL = raw // the first window has not been created yet
		}
		a.mu.Unlock()
		if started {
			a.navigateMain(raw)
		}
	case navigation.External:
		openExternal(raw)
	}
}

// useLaunchURL applies a URL passed on the command line at launch.
func (a *App) useLaunchURL(raw string) {
	switch a.incomingAction(raw) {
	case navigation.Internal:
		a.startURL = raw
	case navigation.External:
		openExternal(raw)
	}
}

// navigateMain loads raw in the main window and brings it to the front.
func (a *App) navigateMain(raw string) {
	a.mu.Lock()
	w := a.main
	p := a.windows[w]
	a.mu.Unlock()
	if w == nil || w.IsDestroyed() || p == nil {
		// Closed main window (macOS keeps the app alive): reopen on the URL.
		nw, np := a.openWindow(true, raw, "main", false)
		a.mu.Lock()
		a.main = nw
		a.mu.Unlock()
		a.attachMain(nw, np)
		return
	}
	if err := p.LoadURL(raw); err != nil {
		log.Printf("mypack-runtime: cannot open incoming url: %v", err)
	}
	a.showMain()
}
