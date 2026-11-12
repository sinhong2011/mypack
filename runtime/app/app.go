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
	"github.com/egoist/mygo/ui"
	"mypack.dev/mypack/pkg/config"
	"mypack.dev/mypack/runtime/inject"
	"mypack.dev/mypack/runtime/navigation"
	"mypack.dev/mypack/runtime/routing"
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

	bridge *bridgeService   // page -> Go calls: notifications, badge
	badges *badgeController // Dock / taskbar badge
	routes *routing.Table   // link routing between Mypack apps

	mu       sync.Mutex
	main     *mygo.Window
	windows  map[*mygo.Window]*mygo.Page // app windows -> their web page
	zoom     int
	quitting bool
	started  bool // the main window was created
	shown    bool
	creds    map[string]credEntry
	prompts  map[string]bool
	finds    map[*mygo.Window]*mygo.Window
	tray     *mygo.Tray
	menu     *mygo.Menu  // the application menu
	lay      layoutState // the main window's layout (layout.go)
	group    *group      // app.group: the member websites (group.go), or nil
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
		windows:  map[*mygo.Window]*mygo.Page{},
		creds:    map[string]credEntry{},
		prompts:  map[string]bool{},
		finds:    map[*mygo.Window]*mygo.Window{},
		disabled: map[string]bool{},
		routes:   &routing.Table{},
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

	// Links other Mypack apps (or the Builder) hand to this app: macOS
	// delivers them as open-URL events, Windows/Linux as arguments. Register
	// before Run to receive the URL the app was launched with.
	mygo.App.OnOpenURL(a.handleIncomingURL)
	if !cfg.Window.MultiInstance {
		if !mygo.App.RequestSingleInstanceLock() {
			return nil // a running instance was asked to show itself
		}
		mygo.App.OnSecondInstance(func(args []string, _ string) {
			if u := firstWebURL(args); u != "" {
				a.handleIncomingURL(u)
				return
			}
			a.showMain()
		})
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
	a.badges = newBadgeController(cfg.Native.BadgeMode() == "auto", func(label string) {
		log.Printf("mypack-runtime: badge: set %q", label)
		mygo.App.Dock.SetBadge(label)
		// A menu-bar app has no Dock icon: the count goes next to its icon.
		a.mu.Lock()
		t := a.tray
		a.mu.Unlock()
		if t != nil && a.menuBarMode() {
			t.SetTitle(label)
		}
	})
	a.badges.sum = cfg.IsGroup() // a group's Dock badge is its sites' total
	a.bridge = a.newBridge()
	mygo.BindAs("mypack", a.bridge)
	a.preload = a.buildPreload()
	if u := firstWebURL(os.Args[1:]); u != "" {
		a.useLaunchURL(u)
	}

	mygo.App.OnWindowCreated(a.setupWindow)
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.mu.Lock()
		a.quitting = true
		a.mu.Unlock()
		a.saveLayoutOnQuit()
	})
	mygo.App.OnActivate(func(has bool) {
		if !has {
			a.showMain()
		}
	})
	a.menu = a.buildMenu()
	mygo.App.SetMenu(a.menu)
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

	if cfg.IsGroup() {
		// A group opens its members, not Source: each website member's
		// site is internal; each local member is served at its own host.
		g, err := a.newGroup()
		if err != nil {
			return err
		}
		a.group = g
		safe = append(safe, g.safeDomains()...)
		localSchemes = append(localSchemes, g.localSchemes...)
		localOrigins = append(localOrigins, g.localOrigins...)
		a.home, a.startURL = g.members[0].home, g.members[0].home
	}
	switch {
	case cfg.IsGroup():
	case cfg.App.Source.Type == config.SourceURL:
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
	default: // a local folder or file
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
	perms := a.state.Get().Notify
	return inject.Build(inject.Options{
		JS: js, CSS: css, DragStrip: a.framelessBar(), DisabledShortcuts: a.cfg.WebView.DisabledKeys,
		WebAPIs: inject.WebAPIs{Notifications: a.cfg.Native.NotificationsMode(), Perms: perms, Badge: a.cfg.Native.BadgeMode() != "off"},
	})
}

// framelessBar: only frameless windows (decorations:false) still need the
// injected drag region; hidden/overlay use the native strip (shell.go).
func (a *App) framelessBar() bool {
	w := a.cfg.Window
	return w.Decorations != nil && !*w.Decorations
}

// pageOptions are the web page options of every app page.
func (a *App) pageOptions() mygo.PageOptions { return a.pageOptionsIn(a.session) }

// pageOptionsIn are the page options with session s (a group member's).
func (a *App) pageOptionsIn(s *mygo.Session) mygo.PageOptions {
	p := mygo.PageOptions{
		Session:          s,
		UserAgent:        resolveUserAgent(a.cfg.WebView, runtime.GOOS, runtime.GOARCH),
		ZoomFactor:       float64(a.zoom) / 100,
		PreloadScript:    a.preload,
		PreloadAllFrames: false, // third-party iframes never get the native bridge
		// Pages call the bound "mypack" service from any host of the app
		// (subdomains cannot be listed up front). Every method re-checks the
		// calling page against the navigation policy.
		TrustedOrigins:  []string{"*"},
		SwipeNavigation: true,
		DevTools:        mygo.DevToolsDisabled,
	}
	if a.cfg.WebView.DevTools {
		p.DevTools = mygo.DevToolsEnabled
	}
	// A chat or mail app hidden in the tray must keep running to notify.
	if a.cfg.Native.NotificationsMode() != "deny" && (a.cfg.Native.Tray || a.cfg.Window.HideOnClose) {
		p.BackgroundThrottling = mygo.ThrottlingOff
	}
	return p
}

// windowOptions returns the shared options of every app window. With strip
// the window shows native UI (strip + web view, see shell.go) instead of a
// page; popups opened by pages pass strip=false and get a regular title bar.
func (a *App) windowOptions(main, strip bool) mygo.WindowOptions {
	w := a.cfg.Window
	o := mygo.WindowOptions{
		Title:           a.title(),
		Width:           w.Width,
		Height:          w.Height,
		MinWidth:        w.MinWidth,
		MinHeight:       w.MinHeight,
		AlwaysOnTop:     w.AlwaysOnTop,
		BackgroundColor: w.Background,
	}
	if !strip {
		o.Page = a.pageOptions()
	}
	if strip {
		h := a.stripHeight()
		if w.TitleBarStyle == "overlay" {
			o.TitleBarStyle = mygo.TitleBarHiddenInset
			// Traffic lights (14px) vertically centered in the strip.
			o.TrafficLightPosition = &mygo.Point{X: 18, Y: (h - 14) / 2}
		} else {
			o.TitleBarStyle = mygo.TitleBarHidden
			o.TrafficLightPosition = &mygo.Point{X: 13, Y: (h - 14) / 2}
		}
		o.TitleBarHeight = 32
	}
	if w.Decorations != nil && !*w.Decorations {
		o.Frameless, o.TitleBarStyle = true, ""
	}
	if main {
		o.Maximized, o.FullScreen = w.Maximized, w.Fullscreen
	}
	return o
}

// openWindow creates an app window loading url and returns it with its web
// page. Main thread.
func (a *App) openWindow(main bool, url, stateKey string, hidden bool) (*mygo.Window, *mygo.Page) {
	if main && a.group != nil {
		return a.openGroupWindow(stateKey, hidden)
	}
	strip := a.useStrip()
	o := a.windowOptions(main, strip)
	o.Hidden, o.StateKey = hidden, stateKey
	if !strip {
		o.URL = url
		w := safeNewWindow(o)
		return w, w.Page()
	}
	s := a.newShell()
	o.Content = ui.View(s.viewFn)
	w := safeNewWindow(o)
	s.win = w
	p, err := s.attach(url)
	if err != nil {
		log.Printf("mypack-runtime: web view: %v", err)
		return w, nil
	}
	a.registerPage(w, p)
	return w, p
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
	a.mu.Lock()
	a.started = true
	startURL := a.startURL
	a.mu.Unlock()
	a.e2eAppHook()
	if a.menuBarMode() {
		if a.startMenuBar(startURL) {
			a.registerGlobalShortcut()
			return
		}
		// No icon could be made: be a normal app, with a Dock icon.
		log.Printf("mypack-runtime: menu-bar icon unavailable; showing a window instead")
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyRegular)
	}
	w, p := a.openWindow(true, startURL, "main", true)
	a.mu.Lock()
	a.main = w
	a.mu.Unlock()
	a.attachMain(w, p)

	a.applyStartLayout(w)

	startHidden := a.cfg.Native.StartToTray
	if a.cfg.Native.Tray && !a.menuBarMode() {
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
	a.registerGlobalShortcut()
}

func (a *App) registerGlobalShortcut() {
	if acc := strings.TrimSpace(a.cfg.Native.GlobalShortcut); acc != "" {
		if err := mygo.GlobalShortcut.Register(acc, a.toggleMain); err != nil {
			log.Printf("mypack-runtime: global shortcut %q: %v", acc, err)
		}
	}
}

// attachMain wires what only the main window does.
func (a *App) attachMain(w *mygo.Window, p *mygo.Page) {
	a.attachMainPage(w, p)
	w.OnClose(func(e *mygo.CloseEvent) {
		a.mu.Lock()
		quitting := a.quitting
		a.mu.Unlock()
		if a.hidesOnClose() && !quitting {
			e.PreventDefault()
			w.Hide()
		}
	})
}

// attachMainPage wires what the main page does, in a window or the
// menu-bar panel: a fixed title and the last-URL memory.
func (a *App) attachMainPage(w *mygo.Window, p *mygo.Page) {
	title := a.title()
	if p != nil {
		p.OnPageTitleUpdated(func(e *mygo.TitleEvent) { e.PreventDefault(); w.SetTitle(title) })
	}
	if p != nil && a.cfg.WebView.RestoreLastURL && !a.cfg.WebView.Incognito && a.cfg.App.Source.Type == config.SourceURL {
		p.OnDidNavigate(func(u string) {
			if strings.HasPrefix(u, "http") && SameOrigin(a.home, u) {
				a.state.Update(func(s *State) { s.LastURL = u })
			}
		})
	}
}

// setupWindow runs for every window MyGo creates (main, popups, new windows).
// Windows showing native UI (the strip layout) register their page from
// openWindow instead.
func (a *App) setupWindow(w *mygo.Window) {
	if p := w.Page(); p != nil {
		a.registerPage(w, p)
	}
}

// registerPage wires every per-page feature of window w showing page p.
func (a *App) registerPage(w *mygo.Window, p *mygo.Page) {
	a.mu.Lock()
	if _, dup := a.windows[w]; dup {
		a.mu.Unlock()
		return
	}
	a.windows[w] = p
	a.mu.Unlock()
	a.trackClose(w, p)
	a.wirePage(w, p, a.onWillNavigate, a.onWindowOpen)
}

// trackClose forgets window w (showing page p) as it closes.
func (a *App) trackClose(w *mygo.Window, p *mygo.Page) {
	w.OnClosed(func() {
		a.badges.drop(p)
		a.mu.Lock()
		delete(a.windows, w)
		if a.main == w {
			a.main = nil
			a.lay.cur = ""
		}
		f := a.finds[w]
		delete(a.finds, w)
		a.mu.Unlock()
		if f != nil {
			f.Destroy()
		}
	})
}

// wirePage wires every per-page feature of page p in window w; nav and
// open decide its navigations and popups.
func (a *App) wirePage(w *mygo.Window, p *mygo.Page, nav func(*mygo.NavigateEvent), open func(mygo.WindowOpenRequest) *mygo.WindowOptions) {
	p.OnPageTitleUpdated(func(e *mygo.TitleEvent) { a.badges.setTitle(p, e.Title) })
	p.OnWillNavigate(nav)
	p.SetWindowOpenHandler(open)
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
	case navigation.External:
		e.PreventDefault()
		a.openLink(e.URL)
	case navigation.OS:
		e.PreventDefault()
		openExternal(e.URL)
	default:
		e.PreventDefault()
	}
}

func (a *App) onWindowOpen(req mygo.WindowOpenRequest) *mygo.WindowOptions {
	switch a.policy.Popup(req.URL, req.UserInitiated) {
	case navigation.Internal:
		o := a.windowOptions(false, false) // popups keep window.opener: regular title bar
		o.Title = a.title()
		if req.Width > 0 {
			o.Width, o.Height = req.Width, req.Height
		}
		return &o
	case navigation.External:
		a.openLink(req.URL)
	case navigation.OS:
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
// notifications unless native.notifications is "deny"; everything else is denied.
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
	if a.menuBarMode() {
		if w := a.panel(); w != nil {
			if !w.IsVisible() {
				w.Show()
			}
			w.Focus()
			return
		}
	}
	a.mu.Lock()
	w := a.main
	a.mu.Unlock()
	if w == nil || w.IsDestroyed() {
		if a.policy == nil {
			return
		}
		w, p := a.openWindow(true, a.home, "main", false)
		a.mu.Lock()
		a.main = w
		a.mu.Unlock()
		a.attachMain(w, p)
		a.applyStartLayout(w)
		return
	}
	if w.IsMinimized() {
		w.Restore()
	}
	w.Show()
	w.Focus()
}

func (a *App) toggleMain() {
	if a.menuBarMode() && a.panel() != nil {
		a.togglePanel()
		return
	}
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
	a.openWindow(false, a.home, "", false)
}

// pageFor returns the web page of win, else of the main/any web window.
func (a *App) pageFor(win *mygo.Window) (*mygo.Window, *mygo.Page) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if p := a.windows[win]; p != nil {
		return win, p
	}
	if p := a.windows[a.main]; p != nil {
		return a.main, p
	}
	for w, p := range a.windows {
		if p != nil {
			return w, p
		}
	}
	return nil, nil
}

// pageOf returns the web page shown by window w, or nil.
func (a *App) pageOf(w *mygo.Window) *mygo.Page {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.windows[w]
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
	ps := make([]*mygo.Page, 0, len(a.windows))
	for _, p := range a.windows {
		if p != nil {
			ps = append(ps, p)
		}
	}
	a.mu.Unlock()
	if a.group != nil { // every site of a group, not only the one shown
		for _, m := range a.group.members {
			if m.page != nil {
				ps = append(ps, m.page)
			}
		}
	}
	a.state.Update(func(s *State) { s.Zoom, s.ZoomBase = z, base })
	for _, p := range ps {
		p.SetZoomFactor(float64(z) / 100)
	}
}
