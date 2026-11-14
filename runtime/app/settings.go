package app

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"mypack.dev/mypack/pkg/config"
	"mypack.dev/mypack/runtime/inject"
	"mypack.dev/mypack/runtime/navigation"
)

const (
	settingsTitleSave    = "mypack:settings:save"
	settingsTitleFolder  = "mypack:settings:folder"
	settingsTitleReset   = "mypack:settings:reset"
	settingsTitleProfile = "mypack:settings:profile"
	settingsTitleReveal  = "mypack:settings:downloads"
)

// settingsForm is the JSON the Settings page posts. Names match mypack.json.
type settingsForm struct {
	Title   string               `json:"title"`
	Window  config.WindowConfig  `json:"window"`
	WebView settingsWebViewForm  `json:"webview"`
	Network config.NetworkConfig `json:"network"`
	Native  settingsNativeForm   `json:"native"`
}

type settingsWebViewForm struct {
	UserAgent         string                  `json:"userAgent"`
	UserAgentPreset   string                  `json:"userAgentPreset"`
	Incognito         bool                    `json:"incognito"`
	RestoreLastURL    bool                    `json:"restoreLastUrl"`
	FindInPage        bool                    `json:"findInPage"`
	DevTools          bool                    `json:"devTools"`
	Navigation        config.NavigationConfig `json:"navigation"`
	CSS               string                  `json:"css"`
	JS                string                  `json:"js"`
	Blocking          *config.BlockingConfig  `json:"blocking"`
	ExternalByDefault bool                    `json:"externalByDefault"`
}

type settingsNativeForm struct {
	Tray           bool   `json:"tray"`
	StartToTray    bool   `json:"startToTray"`
	GlobalShortcut string `json:"globalShortcut"`
	Camera         bool   `json:"camera"`
	Microphone     bool   `json:"microphone"`
	DragDrop       bool   `json:"dragDrop"`
	Notifications  string `json:"notifications"`
	Badge          string `json:"badge"`
	LinkRouting    bool   `json:"linkRouting"`
}

type settingsBoot struct {
	settingsForm
	Name        string `json:"name"`
	Version     string `json:"version"`
	Identifier  string `json:"identifier"`
	ProfileID   string `json:"profileId"`
	ProfileDir  string `json:"profileDir"`
	DownloadDir string `json:"downloadDir"`
	Lite        bool   `json:"lite"`
	MenuBar     bool   `json:"menuBar"`
	RestartHint string `json:"restartHint"`
}

func (a *App) openSettings() {
	a.mu.Lock()
	if a.setWin != nil && !a.setWin.IsDestroyed() {
		w := a.setWin
		a.mu.Unlock()
		w.Show()
		w.Focus()
		return
	}
	a.openingSettings = true
	a.mu.Unlock()

	opts := mygo.WindowOptions{
		Title:     "Settings",
		Width:     760,
		Height:    640,
		MinWidth:  560,
		MinHeight: 480,
		Page: mygo.PageOptions{
			Session:  mygo.NewEphemeralSession(),
			DevTools: mygo.DevToolsDisabled,
		},
	}
	win := safeNewWindow(opts)
	p := win.Page()
	a.mu.Lock()
	a.openingSettings = false
	a.setWin = win
	a.mu.Unlock()
	if p == nil {
		return
	}
	p.OnPageTitleUpdated(func(e *mygo.TitleEvent) {
		switch e.Title {
		case settingsTitleSave, settingsTitleFolder, settingsTitleReset, settingsTitleProfile, settingsTitleReveal:
			e.PreventDefault()
		}
		switch e.Title {
		case settingsTitleSave:
			go a.settingsSave(p)
		case settingsTitleFolder:
			go a.settingsChooseFolder(p)
		case settingsTitleReset:
			mygo.RunOnMain(func() { a.resetSettings(p) })
		case settingsTitleProfile:
			go a.revealProfile()
		case settingsTitleReveal:
			go a.revealDownloads()
		}
	})
	win.OnClosed(func() {
		a.mu.Lock()
		if a.setWin == win {
			a.setWin = nil
		}
		a.mu.Unlock()
	})
	p.LoadHTML(settingsHTML(a.settingsBoot("")), "")
	win.Show()
	win.Focus()
}

func (a *App) settingsBoot(restartHint string) settingsBoot {
	cfg := a.cfg
	ext := cfg.WebView.Navigation.ExternalByDefault == nil || *cfg.WebView.Navigation.ExternalByDefault
	b := settingsBoot{
		Name:        cfg.App.Name,
		Version:     cfg.App.Version,
		Identifier:  cfg.App.Identifier,
		ProfileID:   cfg.App.ProfileID,
		ProfileDir:  a.prefsDir,
		DownloadDir: a.downloadDir(),
		Lite:        Lite,
		MenuBar:     a.menuBarMode(),
		RestartHint: restartHint,
		settingsForm: settingsForm{
			Title:   cfg.App.Title,
			Window:  cfg.Window,
			Network: cfg.Network,
			WebView: settingsWebViewForm{
				UserAgent:         cfg.WebView.UserAgent,
				UserAgentPreset:   cfg.WebView.EffectiveUAPreset(),
				Incognito:         cfg.WebView.Incognito,
				RestoreLastURL:    cfg.WebView.RestoreLastURL,
				FindInPage:        cfg.WebView.FindInPage,
				DevTools:          cfg.WebView.DevTools,
				Navigation:        cfg.WebView.Navigation,
				CSS:               inlineInjection(cfg, "css"),
				JS:                inlineInjection(cfg, "js"),
				Blocking:          cfg.WebView.Blocking,
				ExternalByDefault: ext,
			},
			Native: settingsNativeForm{
				Tray:           cfg.Native.Tray,
				StartToTray:    cfg.Native.StartToTray,
				GlobalShortcut: cfg.Native.GlobalShortcut,
				Camera:         cfg.Native.Camera,
				Microphone:     cfg.Native.Microphone,
				DragDrop:       cfg.Native.DragDrop,
				Notifications:  cfg.Native.NotificationsMode(),
				Badge:          cfg.Native.BadgeMode(),
				LinkRouting:    cfg.Native.LinkRoutingEnabled(),
			},
		},
	}
	if b.WebView.Blocking == nil {
		b.WebView.Blocking = &config.BlockingConfig{}
	}
	return b
}

func (a *App) settingsSave(p *mygo.Page) {
	v, err := p.Eval(`return JSON.stringify(window.MP.collect())`)
	if err != nil {
		log.Printf("mypack-runtime: settings: %v", err)
		return
	}
	raw, _ := v.(string)
	var f settingsForm
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		a.settingsError(p, "Those settings could not be read.")
		return
	}
	next := a.cfg.Clone()
	if err := applySettingsForm(next, f); err != nil {
		a.settingsError(p, err.Error())
		return
	}
	mygo.RunOnMain(func() { a.commitSettings(p, next) })
}

func (a *App) commitSettings(p *mygo.Page, next *config.Config) {
	reload := settingsNeedReload(a.cfg, next)
	restart := settingsNeedRestart(a.cfg, next)
	copyUserPrefs(a.cfg, next)
	if err := saveUserPrefs(a.bundled, a.cfg, a.prefsDir); err != nil {
		log.Printf("mypack-runtime: settings: save: %v", err)
		a.settingsError(p, "The settings could not be saved.")
		return
	}
	a.applyLiveSettings(reload)
	hint := ""
	if restart {
		hint = "Some changes apply the next time you open this app."
	}
	a.settingsReloadPage(p, hint)
}

func (a *App) resetSettings(p *mygo.Page) {
	if a.bundled == nil {
		return
	}
	next := a.bundled.Clone()
	reload := settingsNeedReload(a.cfg, next)
	copyUserPrefs(a.cfg, next)
	_ = saveUserPrefs(a.bundled, a.cfg, a.prefsDir)
	a.applyLiveSettings(reload)
	a.settingsReloadPage(p, "Settings restored to the way this app was built.")
}

func (a *App) settingsReloadPage(p *mygo.Page, hint string) {
	if p == nil {
		return
	}
	p.LoadHTML(settingsHTML(a.settingsBoot(hint)), "")
}

func (a *App) settingsError(p *mygo.Page, msg string) {
	js, _ := jsonString(msg)
	_, _ = p.Eval(`window.MP && window.MP.error(` + js + `)`)
}

func (a *App) settingsChooseFolder(p *mygo.Page) {
	parent := a.setWin
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: parent, Directory: true, CreateDirectories: true,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	js, _ := jsonString(paths[0])
	mygo.RunOnMain(func() {
		if _, err := p.Eval(`window.MP.setDownloadDir(` + js + `)`); err != nil {
			log.Printf("mypack-runtime: settings: folder: %v", err)
		}
	})
}

func (a *App) revealProfile() {
	d := a.prefsDir
	if d == "" {
		return
	}
	_ = os.MkdirAll(d, 0o755)
	mygo.Shell.ShowItemInFolder(d)
}

func copyUserPrefs(dst, src *config.Config) {
	dst.App.Title = src.App.Title
	dst.Window = src.Window
	dst.WebView = src.WebView
	dst.Network = src.Network
	dst.Native = src.Native
}

func applySettingsForm(cfg *config.Config, f settingsForm) error {
	if cfg == nil {
		return fmt.Errorf("no configuration")
	}
	w := f.Window
	if w.Width < 320 || w.Width > 7680 || w.Height < 240 || w.Height > 4320 {
		return fmt.Errorf("Window size must be between 320×240 and 7680×4320.")
	}
	if w.MinWidth < 0 || w.MinWidth > 7680 || w.MinHeight < 0 || w.MinHeight > 4320 {
		return fmt.Errorf("Minimum size is out of range.")
	}
	if w.Zoom != 0 && (w.Zoom < 30 || w.Zoom > 300) {
		return fmt.Errorf("Zoom must be between 30%% and 300%%.")
	}
	switch w.DarkMode {
	case "", "system", "light", "dark":
	default:
		return fmt.Errorf("Appearance is not valid.")
	}
	switch w.TitleBarStyle {
	case "", "default", "hidden", "overlay", "glass":
	default:
		return fmt.Errorf("Title bar style is not valid.")
	}
	switch w.EffectiveMode() {
	case config.ModeWindow, config.ModeCompact, config.ModeSidebar, config.ModeMenuBar:
	default:
		return fmt.Errorf("Window mode is not valid.")
	}
	switch w.SidebarEdge {
	case "", config.EdgeLeft, config.EdgeRight:
	default:
		return fmt.Errorf("Sidebar edge is not valid.")
	}
	nav := f.WebView.Navigation
	if r := strings.TrimSpace(nav.InternalURLRegex); r != "" {
		if _, err := regexp.Compile(r); err != nil {
			return fmt.Errorf("The internal URL pattern is not a valid regular expression.")
		}
	}
	switch nav.Popups {
	case "", "internal", "external", "block":
	default:
		return fmt.Errorf("Popup behaviour is not valid.")
	}
	preset := f.WebView.UserAgentPreset
	switch preset {
	case "", config.UAPresetSafari, config.UAPresetChrome, config.UAPresetEdge,
		config.UAPresetFirefox, config.UAPresetMobileSafari, config.UAPresetCustom:
	default:
		return fmt.Errorf("Browser identity is not valid.")
	}
	switch f.Native.Notifications {
	case "", "allow", "ask", "deny":
	default:
		return fmt.Errorf("Notifications setting is not valid.")
	}
	switch f.Native.Badge {
	case "", "auto", "off":
	default:
		return fmt.Errorf("Unread badge setting is not valid.")
	}

	cfg.App.Title = strings.TrimSpace(f.Title)
	cfg.Window.Width, cfg.Window.Height = w.Width, w.Height
	cfg.Window.MinWidth, cfg.Window.MinHeight = w.MinWidth, w.MinHeight
	cfg.Window.Zoom = w.Zoom
	cfg.Window.AlwaysOnTop = w.AlwaysOnTop
	cfg.Window.Fullscreen = w.Fullscreen
	cfg.Window.HideOnClose = w.HideOnClose
	cfg.Window.MultiWindow = w.MultiWindow
	cfg.Window.DarkMode = w.DarkMode
	cfg.Window.TitleBarStyle = w.TitleBarStyle
	if w.Mode == config.ModeWindow {
		cfg.Window.Mode = ""
	} else {
		cfg.Window.Mode = w.Mode
	}
	cfg.Window.Tabs = w.Tabs
	cfg.Window.SidebarEdge = w.SidebarEdge
	cfg.Window.SidebarAutoHide = w.SidebarAutoHide
	cfg.Window.PanelWidth, cfg.Window.PanelHeight = w.PanelWidth, w.PanelHeight

	wv := &cfg.WebView
	wv.UserAgentPreset = preset
	if preset != config.UAPresetCustom {
		wv.UserAgent = ""
	} else {
		wv.UserAgent = strings.TrimSpace(f.WebView.UserAgent)
	}
	wv.Incognito = f.WebView.Incognito
	wv.RestoreLastURL = f.WebView.RestoreLastURL
	wv.FindInPage = f.WebView.FindInPage
	wv.DevTools = f.WebView.DevTools
	wv.Navigation.Popups = nav.Popups
	wv.Navigation.InternalURLRegex = strings.TrimSpace(nav.InternalURLRegex)
	wv.Navigation.SafeDomains = cleanDomains(nav.SafeDomains)
	if f.WebView.ExternalByDefault {
		wv.Navigation.ExternalByDefault = nil
	} else {
		off := false
		wv.Navigation.ExternalByDefault = &off
	}
	setInlineInjection(cfg, "css", f.WebView.CSS)
	setInlineInjection(cfg, "js", f.WebView.JS)
	wv.Blocking = tidyBlocking(f.WebView.Blocking)

	cfg.Network.DownloadDirectory = strings.TrimSpace(f.Network.DownloadDirectory)
	cfg.Network.Proxy = strings.TrimSpace(f.Network.Proxy)
	cfg.Network.BasicAuthUser = strings.TrimSpace(f.Network.BasicAuthUser)

	n := &cfg.Native
	n.Tray = f.Native.Tray
	n.StartToTray = f.Native.StartToTray && f.Native.Tray
	n.GlobalShortcut = strings.TrimSpace(f.Native.GlobalShortcut)
	n.Camera = f.Native.Camera
	n.Microphone = f.Native.Microphone
	n.DragDrop = f.Native.DragDrop
	if f.Native.Notifications == "allow" {
		n.Notifications = ""
	} else {
		n.Notifications = f.Native.Notifications
	}
	if f.Native.Badge == "auto" {
		n.Badge = ""
	} else {
		n.Badge = f.Native.Badge
	}
	if f.Native.LinkRouting {
		n.LinkRouting = nil
	} else {
		off := false
		n.LinkRouting = &off
	}
	return nil
}

func cleanDomains(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, d := range in {
		d = strings.ToLower(strings.TrimSpace(d))
		d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
		d = strings.TrimSuffix(d, "/")
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

func tidyBlocking(b *config.BlockingConfig) *config.BlockingConfig {
	if b == nil {
		return nil
	}
	var allow, hide []string
	seen := map[string]bool{}
	for _, d := range b.Allow {
		if n := config.NormalizeAllowDomain(d); n != "" && !seen[n] {
			seen[n] = true
			allow = append(allow, n)
		}
	}
	for _, s := range b.Hide {
		s = strings.TrimSpace(s)
		if s == "" || strings.ContainsAny(s, "{}") {
			continue
		}
		hide = append(hide, s)
	}
	b.Allow, b.Hide = allow, hide
	if !b.Active() {
		return nil
	}
	return b
}

func inlineInjection(cfg *config.Config, typ string) string {
	for _, in := range cfg.WebView.Injections {
		if in.Type == typ && in.Path == "" {
			return in.Content
		}
	}
	return ""
}

func setInlineInjection(cfg *config.Config, typ, content string) {
	var out []config.Injection
	for _, in := range cfg.WebView.Injections {
		if in.Type == typ && in.Path == "" {
			continue
		}
		out = append(out, in)
	}
	if strings.TrimSpace(content) != "" {
		out = append(out, config.Injection{Type: typ, Content: content})
	}
	cfg.WebView.Injections = out
}

func settingsNeedReload(a, b *config.Config) bool {
	if a.WebView.UserAgent != b.WebView.UserAgent || a.WebView.UserAgentPreset != b.WebView.UserAgentPreset {
		return true
	}
	if a.WebView.Incognito != b.WebView.Incognito {
		return true
	}
	if inlineInjection(a, "css") != inlineInjection(b, "css") || inlineInjection(a, "js") != inlineInjection(b, "js") {
		return true
	}
	if blockingKey(a.WebView.Blocking) != blockingKey(b.WebView.Blocking) {
		return true
	}
	if a.Native.NotificationsMode() != b.Native.NotificationsMode() || a.Native.BadgeMode() != b.Native.BadgeMode() {
		return true
	}
	return a.Native.DragDrop != b.Native.DragDrop
}

func settingsNeedRestart(a, b *config.Config) bool {
	return a.WebView.Incognito != b.WebView.Incognito ||
		a.Window.TitleBarStyle != b.Window.TitleBarStyle ||
		a.Window.Tabs != b.Window.Tabs ||
		a.Window.EffectiveMode() != b.Window.EffectiveMode() ||
		a.Native.Camera != b.Native.Camera ||
		a.Native.Microphone != b.Native.Microphone
}

func blockingKey(b *config.BlockingConfig) string {
	raw, _ := json.Marshal(b)
	return string(raw)
}

func (a *App) applyLiveSettings(reload bool) {
	cfg := a.cfg
	switch cfg.Window.DarkMode {
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
	a.disabled = map[string]bool{}
	for _, s := range cfg.WebView.DisabledKeys {
		a.disabled[inject.Normalize(s)] = true
	}
	a.preload = a.buildPreload()
	if err := a.rebuildPolicy(); err != nil {
		log.Printf("mypack-runtime: settings: navigation: %v", err)
	}
	a.compileBlocking()
	if a.bridge != nil {
		a.bridge.mode = cfg.Native.NotificationsMode()
		a.bridge.badge = cfg.Native.BadgeMode() != "off"
	}
	if a.badges != nil {
		a.badges.setAuto(cfg.Native.BadgeMode() == "auto")
	}
	a.setZoom(0, true)
	title := a.title()
	ua := resolveUserAgent(cfg.WebView, runtime.GOOS, runtime.GOARCH)
	lay := a.layout()
	forceTop := lay == layoutCompact || lay == layoutSidebar
	a.mu.Lock()
	main := a.main
	wins := make([]*mygo.Window, 0, len(a.windows)+1)
	pages := make([]*mygo.Page, 0, len(a.windows))
	if main != nil {
		wins = append(wins, main)
	}
	for w, p := range a.windows {
		wins = append(wins, w)
		if p != nil {
			pages = append(pages, p)
		}
	}
	a.mu.Unlock()
	seen := map[*mygo.Window]bool{}
	for _, w := range wins {
		if w == nil || w.IsDestroyed() || seen[w] {
			continue
		}
		seen[w] = true
		w.SetTitle(title)
		if w == main && forceTop {
			w.SetAlwaysOnTop(true)
		} else {
			w.SetAlwaysOnTop(cfg.Window.AlwaysOnTop)
		}
		w.SetMinimumSize(cfg.Window.MinWidth, cfg.Window.MinHeight)
	}
	for _, p := range pages {
		p.SetUserAgent(ua)
		p.SetZoomFactor(float64(a.zoom) / 100)
	}
	a.rebindGlobalShortcut()
	a.syncTray()
	a.installAppMenu()
	if reload {
		for _, p := range pages {
			p.Reload()
		}
	}
}

func (a *App) rebuildPolicy() error {
	cfg := a.cfg
	nav := cfg.WebView.Navigation
	safe := append([]string(nil), nav.SafeDomains...)
	if cfg.App.Source.Type == config.SourceURL {
		if u, err := config.NormalizeURL(cfg.App.Source.Value); err == nil {
			if h := config.RegistrableHost(u); h != "" {
				safe = append(safe, h)
			}
		}
	}
	var localSchemes, localOrigins []string
	if a.policy != nil {
		localSchemes, localOrigins = a.policy.LocalSchemes, a.policy.LocalOrigins
	}
	if a.group != nil {
		safe = append(safe, a.group.safeDomains()...)
	}
	p, err := navigation.New(safe, nav.InternalURLRegex, nav.ExternalByDefault, nav.Popups)
	if err != nil {
		return err
	}
	p.LocalSchemes, p.LocalOrigins = localSchemes, localOrigins
	a.policy = p
	return nil
}

func (a *App) rebindGlobalShortcut() {
	mygo.GlobalShortcut.UnregisterAll()
	a.registerGlobalShortcut()
}

func (a *App) syncTray() {
	if a.menuBarMode() {
		return
	}
	a.mu.Lock()
	t := a.tray
	a.mu.Unlock()
	want := a.cfg.Native.Tray
	if want && t == nil {
		a.createTray()
		return
	}
	if !want && t != nil {
		t.Destroy()
		a.mu.Lock()
		a.tray = nil
		a.mu.Unlock()
	} else if t != nil {
		t.SetMenu(a.trayMenu())
	}
}
