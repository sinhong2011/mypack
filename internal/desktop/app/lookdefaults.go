package app

import "mypack.dev/mypack/pkg/config"

// lookDefaults are the common Look & Behaviour choices last made on Create,
// remembered in ui-state.json so the next new app starts with them: the
// title bar, appearance and window size, and the behaviour rows that are
// a matter of taste rather than of the site (signed in, links,
// notifications, ad blocking). How the app shows (window, menu bar…),
// tabs, tray and badge stay per site.
type lookDefaults struct {
	TitleBarStyle string `json:"titleBarStyle,omitempty"`
	DarkMode      string `json:"darkMode,omitempty"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	Maximized     bool   `json:"maximized,omitempty"`
	Incognito     bool   `json:"incognito,omitempty"`
	LinksInApp    bool   `json:"linksInApp,omitempty"`
	Notifications string `json:"notifications,omitempty"`
	Blocking      string `json:"blocking,omitempty"`
}

// captureLook records cfg's common choices.
func captureLook(cfg *config.Config) *lookDefaults {
	w := &cfg.Window
	ext := cfg.WebView.Navigation.ExternalByDefault
	d := &lookDefaults{
		TitleBarStyle: w.TitleBarStyle, DarkMode: w.DarkMode,
		Width: w.Width, Height: w.Height, Maximized: w.Maximized,
		Incognito:     cfg.WebView.Incognito,
		LinksInApp:    ext != nil && !*ext,
		Notifications: cfg.Native.NotificationsMode(),
		Blocking:      blockingMode(cfg),
	}
	if d.Blocking == blockCustom {
		d.Blocking = "" // custom lists belong to their site
	}
	return d
}

// apply seeds a new app's cfg with the remembered choices.
func (d *lookDefaults) apply(cfg *config.Config) {
	if d == nil {
		return
	}
	w := &cfg.Window
	if d.TitleBarStyle != "" {
		w.TitleBarStyle = d.TitleBarStyle
	}
	if d.DarkMode != "" {
		w.DarkMode = d.DarkMode
	}
	if d.Width > 0 && d.Height > 0 {
		w.Width, w.Height = d.Width, d.Height
	}
	w.Maximized = d.Maximized
	cfg.WebView.Incognito = d.Incognito
	if d.LinksInApp {
		f := false
		cfg.WebView.Navigation.ExternalByDefault = &f
	}
	if d.Notifications != "" {
		cfg.Native.Notifications = d.Notifications
	}
	if d.Blocking != "" {
		setBlockingMode(cfg, d.Blocking)
	}
}

// rememberLook saves the new app's current choices as the next defaults.
func (a *App) rememberLook() {
	if cfg := a.create.cfg; cfg != nil {
		a.ui.LookDefaults = captureLook(cfg)
		a.ui.save()
	}
}
