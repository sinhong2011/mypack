// Package app is the generic mypack runtime: it reads mypack.json and turns it
// into a native window around a webview using MyGo.
package app

import (
	"fmt"
	"html"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"mypack.dev/mypack/pkg/config"
	"mypack.dev/mypack/runtime/inject"
	"mypack.dev/mypack/runtime/navigation"
)

// App holds the running application state.
type App struct {
	cfg    *config.Config
	cfgDir string
	policy *navigation.Policy
	state  *Store

	session  *mygo.Session
	site     *Site
	home     string // URL "Home" and new windows load
	startURL string // URL the first window loads
	preload  string

	disabled map[string]bool // normalized disabled accelerators

	mu       sync.Mutex
	main     *mygo.Window
	windows  map[*mygo.Window]bool // web windows
	zoom     int
	quitting bool
	shown    bool
	creds    map[string]credEntry
	prompts  map[string]bool
	finds    map[*mygo.Window]*mygo.Window
	tray     *mygo.Tray
}

type credEntry struct {
	resp mygo.AuthResponse
	used time.Time
}

// Main is the runtime entry point.
func Main() {
	cfg, dir, err := LoadConfig()
	if err != nil {
		fatal("This app cannot start", err.Error())
		return
	}
	a, err := New(cfg, dir)
	if err != nil {
		fatal("This app cannot start", err.Error())
		return
	}
	if err := a.Run(); err != nil {
		log.Printf("mypack-runtime: %v", err)
		os.Exit(1)
	}
}

// fatal shows a native error dialog (never a blank window) and exits 1.
func fatal(title, msg string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, msg)
	mygo.App.SetName("mypack")
	mygo.App.WhenReady(func() {
		go func() {
			mygo.Dialog.Error(title, msg)
			mygo.App.Exit(1)
		}()
	})
	_ = mygo.App.Run()
	os.Exit(1)
}

// New builds an App from a validated config.
func New(cfg *config.Config, cfgDir string) (*App, error) {
	a := &App{
		cfg: cfg, cfgDir: cfgDir,
		windows:  map[*mygo.Window]bool{},
		creds:    map[string]credEntry{},
		prompts:  map[string]bool{},
		finds:    map[*mygo.Window]*mygo.Window{},
		disabled: map[string]bool{},
	}
	for _, s := range cfg.WebView.DisabledKeys {
		a.disabled[inject.Normalize(s)] = true
	}
	return a, nil
}

func (a *App) title() string {
	if t := strings.TrimSpace(a.cfg.App.Title); t != "" {
		return t
	}
	return a.cfg.App.Name
}

// Run configures MyGo and enters the event loop.
func (a *App) Run() error {
	cfg := a.cfg
	mygo.App.SetName(cfg.App.Name)
	mygo.App.SetVersion(cfg.App.Version)

	profile := defaultProfileDir(cfg.App.ProfileID)
	if err := os.MkdirAll(profile, 0o755); err != nil {
		return fmt.Errorf("cannot create the profile directory %s: %w", profile, err)
	}
	mygo.App.SetPath(mygo.PathUserData, profile)
	a.state = OpenStore(profile)

	if runtime.GOOS == "windows" && os.Getenv("WEBVIEW2_USER_DATA_FOLDER") == "" {
		// Own WebView2 profile per app, outside the bundle (APP-05).
		_ = os.Setenv("WEBVIEW2_USER_DATA_FOLDER", filepath.Join(profile, "WebView2"))
	}
	if cfg.Network.Proxy != "" {
		log.Printf("mypack-runtime: warning: network.proxy=%q is ignored; webviews follow the operating-system proxy settings (platform limitation)", redactProxy(cfg.Network.Proxy))
	}

	if !cfg.Window.MultiInstance {
		if !mygo.App.RequestSingleInstanceLock() {
			return nil // a running instance was asked to show itself
		}
		mygo.App.OnSecondInstance(func([]string, string) { a.showMain() })
	}

	switch cfg.Window.DarkMode {
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	}

	if err := a.prepareSource(); err != nil {
		return err
	}
	if !cfg.WebView.Incognito {
		a.session = mygo.NewSession(cfg.App.ProfileID)
	} else {
		a.session = mygo.NewEphemeralSession()
	}
	a.zoom = EffectiveZoom(cfg.Window.Zoom, a.state.Get())
	a.preload = a.buildPreload()

	mygo.App.OnWindowCreated(a.setupWindow)
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.mu.Lock()
		a.quitting = true
		a.mu.Unlock()
	})
	mygo.App.OnActivate(func(has bool) {
		if !has {
			a.showMain()
		}
	})
	mygo.App.SetMenu(a.buildMenu())
	mygo.App.WhenReady(a.start)
	return mygo.App.Run()
}

func redactProxy(p string) string {
	u, err := url.Parse(p)
	if err != nil {
		return "<invalid>"
	}
	u.User = nil
	return u.String()
}

// prepareSource resolves the start URL, starts local serving if needed and
// builds the navigation policy.
func (a *App) prepareSource() error {
	cfg := a.cfg
	nav := cfg.WebView.Navigation
	var localSchemes, localOrigins []string
	safe := append([]string(nil), nav.SafeDomains...)

	switch cfg.App.Source.Type {
	case config.SourceURL:
		u, err := config.NormalizeURL(cfg.App.Source.Value)
		if err != nil {
			return err
		}
		a.home = u
		if h := config.RegistrableHost(u); h != "" {
			safe = append(safe, h) // the source's own site is always internal
		}
		a.startURL = u
		if cfg.WebView.RestoreLastURL && !cfg.WebView.Incognito {
			a.startURL = ChooseStartURL(u, a.state.Get().LastURL, true)
		}
	default:
		root, entry, err := ResolveLocal(a.cfgDir, cfg.App.Source)
		if err != nil {
			return err
		}
		site, err := StartSite(root, entry, runtime.GOOS, os.Getenv("MYPACK_SITE_MODE"))
		if err != nil {
			return err
		}
		a.site = site
		a.home, a.startURL = site.StartURL, site.StartURL
		if site.Scheme != "" {
			localSchemes = append(localSchemes, site.Scheme)
		}
		if site.Origin != "" {
			localOrigins = append(localOrigins, site.Origin)
		}
	}
	p, err := navigation.New(safe, nav.InternalURLRegex, nav.ExternalByDefault, nav.Popups)
	if err != nil {
		return err
	}
	p.LocalSchemes, p.LocalOrigins = localSchemes, localOrigins
	a.policy = p
	return nil
}

func (a *App) buildPreload() string {
	var js, css []string
	for _, inj := range a.cfg.WebView.Injections {
		content := inj.Content
		if content == "" && inj.Path != "" { // normally inlined by the Builder
			p := inj.Path
			if !filepath.IsAbs(p) {
				p = filepath.Join(a.cfgDir, p)
			}
			if b, err := os.ReadFile(p); err == nil {
				content = string(b)
			} else {
				log.Printf("mypack-runtime: injection %s: %v", inj.Path, err)
			}
		}
		switch inj.Type {
		case "js":
			js = append(js, content)
		case "css":
			css = append(css, content)
		}
	}
	return inject.Build(inject.Options{JS: js, CSS: css, DragStrip: a.customTitleBar(), DisabledShortcuts: a.cfg.WebView.DisabledKeys})
}

func (a *App) customTitleBar() bool {
	w := a.cfg.Window
	return w.TitleBarStyle == "hidden" || w.TitleBarStyle == "overlay" || (w.Decorations != nil && !*w.Decorations)
}

// windowOptions returns the shared options of every app window.
func (a *App) windowOptions(main bool) mygo.WindowOptions {
	w := a.cfg.Window
	o := mygo.WindowOptions{
		Title:       a.title(),
		Width:       w.Width,
		Height:      w.Height,
		MinWidth:    w.MinWidth,
		MinHeight:   w.MinHeight,
		AlwaysOnTop: w.AlwaysOnTop,
		Page: mygo.PageOptions{
			Session:          a.session,
			UserAgent:        a.cfg.WebView.UserAgent,
			ZoomFactor:       float64(a.zoom) / 100,
			PreloadScript:    a.preload,
			PreloadAllFrames: false,
			SwipeNavigation:  true,
			DevTools:         mygo.DevToolsDisabled,
		},
		BackgroundColor: w.Background,
	}
	if a.cfg.WebView.DevTools {
		o.Page.DevTools = mygo.DevToolsEnabled
	}
	switch w.TitleBarStyle {
	case "hidden":
		o.TitleBarStyle = mygo.TitleBarHidden
	case "overlay":
		o.TitleBarStyle = mygo.TitleBarHiddenInset
	}
	if w.Decorations != nil && !*w.Decorations {
		o.Frameless, o.TitleBarStyle = true, ""
	}
	if main {
		o.Maximized, o.FullScreen = w.Maximized, w.Fullscreen
	}
	return o
}

// safeNewWindow tolerates an invalid background color (NewWindow panics).
func safeNewWindow(o mygo.WindowOptions) (w *mygo.Window) {
	defer func() {
		if r := recover(); r != nil {
			if o.BackgroundColor == "" {
				panic(r)
			}
			log.Printf("mypack-runtime: window background %q rejected: %v", o.BackgroundColor, r)
			o.BackgroundColor = ""
			w = mygo.NewWindow(o)
		}
	}()
	return mygo.NewWindow(o)
}

func (a *App) start() {
	o := a.windowOptions(true)
	o.URL = a.startURL
	o.Hidden = true
	o.StateKey = "main"
	w := safeNewWindow(o)
	a.mu.Lock()
	a.main = w
	a.mu.Unlock()
	a.attachMain(w)

	startHidden := a.cfg.Native.StartToTray
	if a.cfg.Native.Tray {
		if !a.createTray() {
			startHidden = false // never leave the user with no way in
		}
	}
	reveal := func() {
		a.mu.Lock()
		if a.shown {
			a.mu.Unlock()
			return
		}
		a.shown = true
		a.mu.Unlock()
		if !startHidden {
			w.Show()
		}
	}
	w.OnReadyToShow(reveal)
	// A page that never paints (offline, blocked) must still show a window.
	time.AfterFunc(6*time.Second, func() { mygo.RunOnMain(reveal) })

	if acc := strings.TrimSpace(a.cfg.Native.GlobalShortcut); acc != "" {
		if err := mygo.GlobalShortcut.Register(acc, a.toggleMain); err != nil {
			log.Printf("mypack-runtime: global shortcut %q: %v", acc, err)
		}
	}
}

// attachMain wires what only the main window does.
func (a *App) attachMain(w *mygo.Window) {
	p := w.Page()
	title := a.title()
	p.OnPageTitleUpdated(func(e *mygo.TitleEvent) { e.PreventDefault(); w.SetTitle(title) })
	if a.cfg.WebView.RestoreLastURL && !a.cfg.WebView.Incognito && a.cfg.App.Source.Type == config.SourceURL {
		p.OnDidNavigate(func(u string) {
			if strings.HasPrefix(u, "http") && SameOrigin(a.home, u) {
				a.state.Update(func(s *State) { s.LastURL = u })
			}
		})
	}
	w.OnClose(func(e *mygo.CloseEvent) {
		a.mu.Lock()
		quitting := a.quitting
		a.mu.Unlock()
		if a.cfg.Window.HideOnClose && !quitting {
			e.PreventDefault()
			w.Hide()
		}
	})
}

// setupWindow runs for every window MyGo creates (main, popups, new windows).
func (a *App) setupWindow(w *mygo.Window) {
	p := w.Page()
	if p == nil {
		return // native UI helper window
	}
	a.mu.Lock()
	a.windows[w] = true
	a.mu.Unlock()
	w.OnClosed(func() {
		a.mu.Lock()
		delete(a.windows, w)
		if a.main == w {
			a.main = nil
		}
		f := a.finds[w]
		delete(a.finds, w)
		a.mu.Unlock()
		if f != nil {
			f.Destroy()
		}
	})

	p.OnWillNavigate(a.onWillNavigate)
	p.SetWindowOpenHandler(a.onWindowOpen)
	p.SetPermissionHandler(a.onPermission)
	p.SetAuthHandler(func(req mygo.AuthRequest) mygo.AuthResponse { return a.onAuth(w, req) })
	p.OnWillDownload(a.onWillDownload)
	p.OnDownloadDone(a.onDownloadDone)
	if a.cfg.WebView.IgnoreCertErrs {
		log.Printf("mypack-runtime: warning: ignoreCertificateErrors is on; TLS certificate errors are ignored")
		p.SetCertificateErrorHandler(func(mygo.CertificateError) bool { return true })
	}
	p.OnDidFailLoad(func(e *mygo.LoadError) { a.onLoadFailed(p, e) })
}

func (a *App) onLoadFailed(p *mygo.Page, e *mygo.LoadError) {
	if e.Code == -999 || !(strings.HasPrefix(e.URL, "http") || strings.HasPrefix(e.URL, siteScheme+":")) {
		return // cancelled, or not a page we can retry
	}
	target := html.EscapeString(e.URL)
	js, _ := jsonString(e.URL)
	p.LoadHTML(`<!doctype html><meta charset="utf-8"><meta name="color-scheme" content="light dark">
<style>body{font:15px system-ui,sans-serif;display:grid;place-items:center;height:100vh;margin:0;text-align:center}
div{max-width:30em;padding:2em}h1{font-size:1.3em}code{word-break:break-all;opacity:.7}button{font:inherit;padding:.5em 1.2em;margin-top:1em}</style>
<div><h1>This page could not be loaded</h1><p>`+html.EscapeString(e.Description)+`</p><p><code>`+target+`</code></p>
<button onclick="location.href=`+html.EscapeString(js)+`">Try again</button></div>`, "")
}

func (a *App) onWillNavigate(e *mygo.NavigateEvent) {
	switch a.policy.Navigate(e.URL, e.UserInitiated) {
	case navigation.Internal:
	case navigation.External, navigation.OS:
		e.PreventDefault()
		openExternal(e.URL)
	default:
		e.PreventDefault()
	}
}

func (a *App) onWindowOpen(req mygo.WindowOpenRequest) *mygo.WindowOptions {
	switch a.policy.Popup(req.URL, req.UserInitiated) {
	case navigation.Internal:
		o := a.windowOptions(false)
		o.Title = a.title()
		if req.Width > 0 {
			o.Width, o.Height = req.Width, req.Height
		}
		return &o
	case navigation.External, navigation.OS:
		openExternal(req.URL)
	}
	return nil
}

func openExternal(u string) {
	go func() {
		if err := mygo.Shell.OpenExternal(u); err != nil {
			log.Printf("mypack-runtime: cannot open %s: %v", u, err)
		}
	}()
}

// onPermission: camera/microphone only when the app was built with them;
// notifications are allowed (chat/mail apps); everything else is denied.
func (a *App) onPermission(req mygo.PermissionRequest) bool {
	for _, perm := range req.Permissions {
		ok := false
		switch perm {
		case mygo.PermissionCamera:
			ok = a.cfg.Native.Camera
		case mygo.PermissionMicrophone:
			ok = a.cfg.Native.Microphone
		case mygo.PermissionNotifications:
			ok = true
		}
		if !ok {
			return false
		}
	}
	return len(req.Permissions) > 0
}

func (a *App) showMain() {
	a.mu.Lock()
	w := a.main
	a.mu.Unlock()
	if w == nil || w.IsDestroyed() {
		if a.policy == nil {
			return
		}
		o := a.windowOptions(true)
		o.URL, o.StateKey = a.home, "main"
		w = safeNewWindow(o)
		a.mu.Lock()
		a.main = w
		a.mu.Unlock()
		a.attachMain(w)
		return
	}
	if w.IsMinimized() {
		w.Restore()
	}
	w.Show()
	w.Focus()
}

func (a *App) toggleMain() {
	a.mu.Lock()
	w := a.main
	a.mu.Unlock()
	if w != nil && !w.IsDestroyed() && w.IsVisible() && w.IsFocused() {
		w.Hide()
		return
	}
	a.showMain()
}

func (a *App) newWindow() {
	o := a.windowOptions(false)
	o.URL = a.home
	safeNewWindow(o)
}

// pageFor returns the web page of win, else of the main/any web window.
func (a *App) pageFor(win *mygo.Window) (*mygo.Window, *mygo.Page) {
	if win != nil {
		if p := win.Page(); p != nil {
			return win, p
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.main != nil && a.main.Page() != nil {
		return a.main, a.main.Page()
	}
	for w := range a.windows {
		if p := w.Page(); p != nil {
			return w, p
		}
	}
	return nil, nil
}

func (a *App) setZoom(delta int, reset bool) {
	base := a.cfg.Window.Zoom
	if base == 0 {
		base = 100
	}
	a.mu.Lock()
	if reset {
		a.zoom = clampZoom(base)
	} else {
		a.zoom = clampZoom(a.zoom + delta)
	}
	z := a.zoom
	ws := make([]*mygo.Window, 0, len(a.windows))
	for w := range a.windows {
		ws = append(ws, w)
	}
	a.mu.Unlock()
	a.state.Update(func(s *State) { s.Zoom, s.ZoomBase = z, base })
	for _, w := range ws {
		if p := w.Page(); p != nil {
			p.SetZoomFactor(float64(z) / 100)
		}
	}
}
