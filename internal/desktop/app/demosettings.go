package app

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
)

// demoPrefs is this built app's Settings overlay. It is not MyPack's
// Settings. DownloadDir starts from the project or MyPack default and
// can be changed here. Active and Split are a group window's rail
// (which site shows, and which sits beside it); Split is -1 when the
// window is not split. SplitPct is the first pane's share (0–100).
// SplitVert is stacked (top/bottom) rather than side by side.
type demoPrefs struct {
	Title          string
	AlwaysOnTop    bool
	HideOnClose    bool
	GlobalShortcut string
	DownloadDir    string
	Active         int
	Split          int
	SplitPct       float32
	SplitVert      bool
}

func (a *App) prefsFor(d demoLaunch) *demoPrefs {
	if a.demoPrefs == nil {
		a.demoPrefs = map[string]*demoPrefs{}
	}
	p := a.demoPrefs[d.key]
	if p == nil {
		p = &demoPrefs{Split: -1, SplitPct: 50}
		if d.cfg != nil {
			p.Title = d.cfg.App.Title
			p.AlwaysOnTop = d.cfg.Window.AlwaysOnTop
			p.HideOnClose = d.cfg.Window.HideOnClose
			p.GlobalShortcut = d.cfg.Native.GlobalShortcut
			p.DownloadDir = strings.TrimSpace(d.cfg.Network.DownloadDirectory)
		}
		if p.DownloadDir == "" {
			p.DownloadDir = strings.TrimSpace(a.settings.DownloadDir)
		}
		if d.key != "" {
			a.demoPrefs[d.key] = p
		}
	}
	return p
}

func (a *App) demoDownloadDir(d demoLaunch) string {
	if p := a.demoPrefs[d.key]; p != nil {
		return strings.TrimSpace(p.DownloadDir)
	}
	if d.cfg != nil && strings.TrimSpace(d.cfg.Network.DownloadDirectory) != "" {
		return d.cfg.Network.DownloadDirectory
	}
	return strings.TrimSpace(a.settings.DownloadDir)
}

func (a *App) demoAppMenu(d demoLaunch) *mygo.Menu {
	settings := func(*mygo.MenuItem, *mygo.Window) { a.openDemoSettings(d) }
	openBrowser := func(*mygo.MenuItem, *mygo.Window) { a.tryOpenInBrowser(a.demoBrowserURL(d)) }
	name := d.name
	if name == "" {
		name = "App"
	}
	appItems := []*mygo.MenuItem{
		{Label: T("menu.about"), Click: settings},
		mygo.Separator(),
		{Label: T("menu.settings"), Accelerator: "CmdOrCtrl+,", Click: settings},
		mygo.Separator(),
		{Label: T("common.close"), Accelerator: "CmdOrCtrl+W", Click: func(_ *mygo.MenuItem, w *mygo.Window) {
			if w != nil {
				w.Close()
			}
		}},
	}
	file := []*mygo.MenuItem{
		{Label: T("menu.settings"), Click: settings},
		mygo.Separator(),
		{Label: T("demo.open.browser"), Click: openBrowser},
		mygo.Separator(),
		{Role: mygo.RoleClose},
	}
	if runtime.GOOS == "darwin" {
		file = file[2:] // Settings lives in the app menu
	}
	items := []*mygo.MenuItem{
		{Label: name, Submenu: appItems},
		{Label: T("menu.file"), Submenu: file},
		{Role: mygo.RoleEditMenu},
	}
	if d.isGroup() {
		items = append(items, a.demoGroupViewMenu(d))
	}
	items = append(items, &mygo.MenuItem{Role: mygo.RoleWindowMenu})
	return mygo.NewMenu(items)
}

func (a *App) openDemoSettings(d demoLaunch) {
	if a.demoSet == nil {
		a.demoSet = map[string]*mygo.Window{}
	}
	if w := a.demoSet[d.key]; w != nil && !w.IsDestroyed() {
		w.Show()
		w.Focus()
		return
	}
	state := d
	title := T("demo.settings.title", "name", d.name)
	h := 560
	if d.isGroup() {
		h = 680
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:             title,
		Width:             640,
		Height:            h,
		MinWidth:          480,
		MinHeight:         400,
		TitleBarStyle:     mygo.TitleBarHidden,
		HideWindowButtons: ownWindowButtons,
		BackgroundColor:   "light-dark(#efefeb, #141415)",
		Content:           ui.View(func(c *ui.Context) { a.demoSettingsView(c, state) }),
	})
	setWindowIcon(win, d.icon)
	menu := a.demoAppMenu(d)
	win.SetMenu(menu)
	win.OnFocus(func() { mygo.App.SetMenu(menu) })
	if d.key != "" {
		key := d.key
		a.demoSet[key] = win
		win.OnClose(func(*mygo.CloseEvent) { delete(a.demoSet, key) })
	}
}

func (a *App) demoSettingsView(c *ui.Context, d demoLaunch) {
	p := design.Of(c)
	design.Apply(c)
	pref := a.prefsFor(d)
	ui.Column(c).Fill().Background(p.CanvasBot).Children(func() {
		ui.Row(c).Padding(16, 20).Gap(12).AlignItems(ui.Center).Children(func() {
			kit.AppIconOpts(c, d.icon, d.name, 36, kit.IconOpts{Shadow: true})
			ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
				kit.Headline(c, T("demo.settings.title", "name", d.name))
				kit.Caption(c, T("demo.open.body"))
			})
			windowButtons(c, a.demoSet[d.key])
		})
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Padding(8, 20, 28, 20).Gap(16).Children(func() {
				kit.Group(c, T("settings.general"), func() {
					kit.SettingRow(c, T("set.title"), T("set.title.desc"), func() {
						in := kit.TextField(c.Key("title"), &pref.Title, T("set.title"), kit.FieldOpts{Placeholder: d.name})
						in.Width(220)
						if in.Changed() {
							a.applyDemoWindow(d)
						}
					})
					kit.SettingRow(c, T("set.alwaysOnTop"), "", func() {
						old := pref.AlwaysOnTop
						kit.Switch(c.Key("aot"), &pref.AlwaysOnTop, T("set.alwaysOnTop"))
						if pref.AlwaysOnTop != old {
							a.applyDemoWindow(d)
						}
					})
					kit.SettingRow(c, T("set.hideOnClose"), T("set.hideOnClose.desc"), func() {
						kit.Switch(c.Key("hoc"), &pref.HideOnClose, T("set.hideOnClose"))
					})
					kit.SettingRow(c, T("set.shortcut"), T("set.shortcut.desc"), func() {
						in := kit.TextField(c.Key("gs"), &pref.GlobalShortcut, T("set.shortcut"), kit.FieldOpts{Placeholder: "CmdOrCtrl+Shift+Space"})
						in.Width(220)
					})
				})
				if d.isGroup() {
					kit.GroupWith(c, T("sites.title"), T("demo.sites.rebuild"), func() {
						for _, m := range d.members() {
							kit.SettingRow(c.Key("site-"+m.ID), m.Name, memberHome(m), nil)
						}
					})
				}
				kit.Group(c, T("set.downloads"), func() {
					kit.SettingRow(c, T("set.downloads"), T("demo.downloads.baked"), func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
							in := kit.TextField(c.Key("dl"), &pref.DownloadDir, T("set.downloads"), kit.FieldOpts{Placeholder: "~/Downloads"})
							in.Width(200)
							if kit.Button(c.Key("dlc"), T("common.choose"), kit.ButtonOpts{}).Clicked() {
								go func() {
									paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
										Parent: a.demoSet[d.key], Directory: true, CreateDirectories: true,
										DefaultPath: pref.DownloadDir,
									})
									if err == nil && len(paths) > 0 {
										if w := a.demoSet[d.key]; w != nil && !w.IsDestroyed() {
											w.Update(func() { pref.DownloadDir = paths[0] })
										}
									}
								}()
							}
						})
					})
					if path := strings.TrimSpace(pref.DownloadDir); path == "" {
						kit.Caption(c, T("demo.downloads.empty"))
					} else {
						kit.Caption(c, T("set.downloads.using", "path", shortPath(path)))
					}
				})
			})
		})
	})
}

func (a *App) applyDemoWindow(d demoLaunch) {
	w := a.demoApps[d.key]
	if w == nil || w.IsDestroyed() {
		return
	}
	p := a.prefsFor(d)
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = d.name
	}
	w.SetTitle(title)
	w.SetAlwaysOnTop(p.AlwaysOnTop)
}
