package app

import (
	"context"
	"fmt"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/i18n"
)

type settingsState struct {
	agentTab int
	registry string
	regInit  bool

	pane settingsPane // the category on show
	scrl ui.ScrollState
}

// settingsPane is one category of Settings, listed in its sidebar.
type settingsPane string

const (
	paneGeneral    settingsPane = "general"
	paneAppearance settingsPane = "appearance"
	paneSync       settingsPane = "sync"
	paneBuilds     settingsPane = "builds"
	paneAgents     settingsPane = "agents"
	paneAdvanced   settingsPane = "advanced"
	paneAbout      settingsPane = "about"
)

type paneInfo struct {
	id    settingsPane
	glyph *ui.SVG
	title string // message ID
}

// settingsPanes lists the categories in sidebar order.
func settingsPanes() []paneInfo {
	return []paneInfo{
		{paneGeneral, icons.Sliders, "settings.general"},
		{paneAppearance, icons.Appearance, "settings.appearance"},
		{paneSync, icons.Refresh, "settings.sync"},
		{paneBuilds, icons.Build, "settings.builds"},
		{paneAgents, icons.Agent, "settings.agents"},
		{paneAdvanced, icons.Code, "settings.advanced"},
		{paneAbout, icons.About, "settings.about"},
	}
}

func paneIndex(id settingsPane) int {
	for i, p := range settingsPanes() {
		if p.id == id {
			return i
		}
	}
	return -1
}

// curPane is the category on show (General until one is picked).
func (a *App) curPane() settingsPane {
	if paneIndex(a.set.pane) < 0 {
		return paneGeneral
	}
	return a.set.pane
}

// openSettings opens the Settings window (or brings it forward), on pane
// when given.
func (a *App) openSettings(pane settingsPane) {
	if pane != "" {
		a.selectPane(pane)
	}
	if a.setWin == nil {
		a.setWin = mygo.NewWindow(mygo.WindowOptions{
			Title:                T("nav.settings"),
			Width:                860,
			Height:               620,
			MinWidth:             680,
			MinHeight:            460,
			TitleBarStyle:        mygo.TitleBarHidden,
			TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
			BackgroundColor:      "light-dark(#efefeb, #141415)",
			StateKey:             "settings",
			Content:              ui.View(a.settingsWindowView),
		})
		// Closing hides it, so it opens again where it was.
		a.setWin.OnClose(func(e *mygo.CloseEvent) {
			if !a.quitting {
				e.PreventDefault()
				a.setWin.Hide()
			}
		})
	}
	a.setWin.SetTitle(T("nav.settings"))
	a.setWin.Show()
	a.setWin.Restore()
	a.setWin.Focus()
	a.setWin.Invalidate()
}

// redrawSettings redraws the Settings window, when it is open.
func (a *App) redrawSettings() {
	if a.setWin != nil {
		a.setWin.Invalidate()
	}
}

// dialogParent is the window a dialog opened from Settings belongs to.
func (a *App) dialogParent() *mygo.Window {
	if a.setWin != nil && a.setWin.IsVisible() {
		return a.setWin
	}
	return a.win
}

func (a *App) selectPane(id settingsPane) {
	if id == a.curPane() {
		a.set.pane = id
		return
	}
	a.set.scrl = ui.ScrollState{}
	a.set.pane = id
	a.ui.SettingsPane = string(id)
	a.ui.save()
	a.redrawSettings()
}

// settingsWindowView is the Settings window: the categories in a sidebar on
// the window's chrome, the chosen one on the raised canvas beside it, as
// the main window lays out its sections.
func (a *App) settingsWindowView(c *ui.Context) {
	design.Apply(c)
	p := design.Of(c)
	c.Root().Background(p.Chrome)
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.settingsSidebar(c)
		in := design.CanvasInset
		canvas := ui.Box(c).Grow(1).Basis(0).MinWidth(0).Margin(in, in, in, 0).Radius(design.RadiusCanvas).
			Border(1, p.GroupBorder).Clip()
		design.Tint(canvas, p.CanvasTop, p.CanvasBot)
		design.Float(c, canvas)
		canvas.Children(func() {
			// Panes switch at once, as a settings window's categories do.
			ui.Column(c.Key("pane/" + string(a.curPane()))).Absolute().Fill().Children(func() {
				a.settingsView(c)
			})
			// The pane's title, over the content that scrolls under it.
			ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(design.TitleBarH).Padding(0, 20).AlignItems(ui.Center).DragWindow().
				LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
					{Color: p.CanvasTop, At: 0}, {Color: p.CanvasTop, At: 0.7}, {Color: design.Fade(p.CanvasTop, 0), At: 1}}}).
				Children(func() {
					ui.Text(c, T(settingsPanes()[paneIndex(a.curPane())].title)).FontSize(design.FontTitle3).FontWeight(design.WeightBold).
						TextColor(p.Text).SingleLine().Role(ui.RoleHeading).LetterSpacing(design.TrackTitle)
				})
		})
	})
	kit.Toasts(c)
	// Toasts raised from Settings show here while it is in front.
	if a.setWin != nil && a.setWin.IsFocused() {
		a.drainToasts(c)
	}
	if c.Shortcut(0, ui.KeyEscape) && !kit.ModalOpen() {
		a.setWin.Close()
	}
}

// settingsGlide pairs the Settings sidebar's selection pill across rows.
type settingsGlide struct{}

// settingsSidebar lists the Settings window's categories.
func (a *App) settingsSidebar(c *ui.Context) {
	p := design.Of(c)
	bar := c.TitleBar()
	ui.Column(c.Key("settings-sidebar")).Width(design.SidebarWidth).Shrink(0).Padding(0, 10, 12, 10).Children(func() {
		ui.Row(c).Height(design.TitleBarH).Shrink(0).DragWindow().AlignItems(ui.Center).Padding(0, 0, 0, max(6, bar.Left)).Gap(8).Children(func() {
			if bar.Left == 0 {
				kit.BrandMark(c, 22)
				ui.Text(c, "Mypack").FontSize(design.FontTitle3).FontWeight(design.WeightBold).TextColor(p.Text).LetterSpacing(design.TrackTitle)
			}
		})
		kit.Eyebrow(c, T("nav.settings")).Padding(4, 10, 8, 10)
		cur := a.curPane()
		ui.Column(c).Gap(1).Role(ui.RoleNavigation).Label(T("nav.settings")).Children(func() {
			for _, pi := range settingsPanes() {
				if a.sideRow(c, "pane-"+string(pi.id), pi.glyph, T(pi.title), "", false, pi.id == cur, settingsGlide{}) {
					a.selectPane(pi.id)
				}
			}
		})
	})
}

func (a *App) saveSettings() {
	st := a.settings
	go func() {
		if err := a.svc.SaveSettings(bgctx(), st); err != nil {
			a.win.Update(func() { a.toastErr(err) })
		}
	}()
	a.redrawSettings()
}

func (a *App) settingsView(c *ui.Context) {
	ui.Scroll(c).Grow(1).Basis(0).ScrollbarInsets(design.TitleBarH, 0, 0).TrackScroll(&a.set.scrl).Children(func() {
		// A readable measure, left-aligned, as System Settings' panes.
		ui.Column(c).FillWidth().MaxWidth(760).Gap(24).Padding(design.TitleBarH+8, design.PageGutter, 40, design.PageGutter).Children(func() {
			switch a.curPane() {
			case paneGeneral:
				a.settingsGeneral(c)
			case paneAppearance:
				a.settingsAppearance(c)
			case paneSync:
				a.syncGroup(c)
			case paneBuilds:
				a.settingsBuilds(c)
			case paneAgents:
				a.settingsAgents(c)
			case paneAdvanced:
				a.settingsAdvanced(c)
			case paneAbout:
				a.settingsAbout(c)
			}
		})
	})
}

func (a *App) settingsGeneral(c *ui.Context) {
	kit.Group(c, "", func() {
		kit.SettingIconRow(c, icons.Globe, ui.Color{}, T("settings.language"), T("settings.language.desc"), func() {
			opts := []kit.Option{{Value: "system", Label: T("settings.language.system")}}
			for _, l := range i18n.Supported {
				opts = append(opts, kit.Option{Value: l.ID, Label: l.Autonym})
			}
			v := a.settings.Language
			if v == "" {
				v = "system"
			}
			old := v
			kit.PopUp(c.Key("lang"), &v, opts, T("settings.language")).Width(200)
			if v != old {
				a.settings.Language = v
				i18n.SetLocale(v)
				a.installMenu()
				a.syncTray()
				a.saveSettings()
			}
		})
		kit.SettingIconRow(c, icons.Bell, ui.Color{}, T("settings.menubar"), T("settings.menubar.desc"), func() {
			v := a.settings.ShowMenuBarIcon
			kit.Switch(c.Key("mb"), &v, T("settings.menubar"))
			if v != a.settings.ShowMenuBarIcon {
				a.settings.ShowMenuBarIcon = v
				a.syncTray()
				a.saveSettings()
			}
		})
		kit.SettingIconRow(c, icons.Download, ui.Color{}, T("settings.updates"), T("settings.updates.desc"), func() {
			v := a.settings.CheckUpdates
			kit.Switch(c.Key("cu"), &v, T("settings.updates"))
			if v != a.settings.CheckUpdates {
				a.settings.CheckUpdates = v
				a.saveSettings()
			}
		})
	})
}

func (a *App) settingsAppearance(c *ui.Context) {
	kit.Group(c, "", func() {
		kit.SettingIconRow(c, icons.Appearance, ui.Color{}, T("settings.theme"), "", func() {
			idx := map[string]int{"system": 0, "light": 1, "dark": 2}[a.settings.Appearance]
			old := idx
			kit.Segmented(c.Key("ap"), &idx, []string{T("appearance.system"), T("appearance.light"), T("appearance.dark")}, true).Label(T("settings.theme"))
			if idx != old {
				a.settings.Appearance = []string{"system", "light", "dark"}[idx]
				applyAppearance(a.settings.Appearance)
				a.saveSettings()
			}
		})
	})
}

func (a *App) settingsBuilds(c *ui.Context) {
	kit.Group(c, "", func() {
		dir := func(key, label, desc string, v *string, tint ui.Color) {
			kit.SettingIconRow(c.Key(key), icons.Folder, tint, label, desc, func() {
				kit.Mono(c, shortPath(*v)).SingleLine().MaxWidth(220)
				if kit.Button(c.Key("ch"), T("common.choose"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
					go func() {
						paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.dialogParent(), Directory: true, CreateDirectories: true, DefaultPath: *v})
						if err == nil && len(paths) > 0 {
							a.win.Update(func() { *v = paths[0]; a.saveSettings() })
						}
					}()
				}
			})
		}
		on := a.ui.autoInstallDefault()
		kit.SettingIconRow(c.Key("autoinst"), icons.Play, ui.Color{}, T("autoInstall.default"), T("autoInstall.default.desc"), func() {
			old := on
			kit.Switch(c.Key("sw"), &on, T("autoInstall.default"))
			if on != old {
				a.ui.AutoInstallDefault = &on
				a.ui.save()
			}
		})
		hudOn := a.ui.ShowHUD == nil || *a.ui.ShowHUD
		kit.SettingIconRow(c.Key("hud"), icons.Monitor, ui.Color{}, T("hud.setting"), T("hud.setting.desc"), func() {
			old := hudOn
			kit.Switch(c.Key("sw-hud"), &hudOn, T("hud.setting"))
			if hudOn != old {
				a.ui.ShowHUD = &hudOn
				a.ui.save()
				a.syncHUD()
			}
		})
		dir("out", T("settings.outputDir"), T("settings.outputDir.desc"), &a.settings.OutputDir, ui.Color{})
		dir("inst", T("settings.installDir"), T("settings.installDir.desc"), &a.settings.InstallDir, ui.Color{})
	})
}

// settingsAgents (spec §12.2): honest, copyable setup; no fake status.
func (a *App) settingsAgents(c *ui.Context) {
	p := design.Of(c)
	s := &a.set
	kit.Group(c, "", func() {
		ui.Column(c).Padding(14).Gap(12).Children(func() {
			ui.Text(c, T("settings.agents.desc")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
			kit.Segmented(c.Key("agent"), &s.agentTab, []string{"Claude Code", "Codex", T("settings.agents.json")}, true).Label(T("settings.agents"))
			snippet := agentSnippet(s.agentTab)
			ui.Row(c).Gap(8).Padding(10, 12).Radius(design.RadiusCard).Background(p.Well).Border(0.5, p.GroupBorder).AlignItems(ui.Start).Children(func() {
				ui.Text(c, snippet).Font(kit.MonoFont).FontSize(11.5).TextColor(p.Text).Grow(1).MinWidth(0).Selectable().LineHeight(1.5)
				if kit.CopyButton(c.Key("cps"), snippet, T("common.copy")) {
					a.toast(T("common.copied"), "", 1)
				}
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				kit.Glyph(c, icons.Shield, 13, p.TextTertiary)
				kit.Caption(c, T("settings.agents.note"))
			})
		})
	})
}

func (a *App) settingsAdvanced(c *ui.Context) {
	s := &a.set
	kit.Group(c, "", func() {
		if !s.regInit {
			s.regInit, s.registry = true, a.settings.RegistryURL
		}
		kit.SettingBlock(c, T("settings.registry"), T("settings.registry.desc"), func() {
			ui.Row(c).Gap(8).Children(func() {
				kit.TextField(c.Key("reg"), &s.registry, T("settings.registry"), kit.FieldOpts{Placeholder: "https://…/registry.json", Mono: true}).Grow(1)
				if kit.Button(c.Key("regsave"), T("common.save"), kit.ButtonOpts{Disabled: s.registry == a.settings.RegistryURL}).Clicked() {
					a.settings.RegistryURL = s.registry
					a.saveSettings()
					a.toast(T("settings.registry.saved"), "", 1)
				}
			})
		})
	})
}

// The author shown in About, and where About's links go.
const (
	aboutAuthor    = "sinhong2011"
	aboutAuthorURL = "https://github.com/sinhong2011"
	aboutRepoURL   = "https://github.com/sinhong2011/mypack"
)

func (a *App) settingsAbout(c *ui.Context) {
	p := design.Of(c)
	open := func(url string) {
		a.do(func(ctx context.Context) error { return a.svc.OpenURL(ctx, url) }, a.toastErr)
	}
	// Header: the app, centred, as a Mac app's About window.
	ui.Column(c).Gap(4).Padding(26, 18, 22, 18).Radius(design.RadiusPanel).Background(p.Group).Border(0.5, p.GroupBorder).
		AlignItems(ui.Center).Children(func() {
		kit.BrandMark(c, 76).Margin(0, 0, 10, 0)
		ui.Text(c, "Mypack").FontSize(design.FontTitle1).FontWeight(design.WeightBold).TextColor(p.Text).LetterSpacing(design.TrackTitle)
		kit.Secondary(c, T("settings.about.tagline")).TextAlign(ui.Center)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(8, 0, 0, 0).Children(func() {
			ui.Row(c).Padding(2, 8).Radius(999).Background(p.Fill).Children(func() {
				ui.Text(c, T("settings.about.version", "v", Version)).FontSize(design.FontCaption).FontWeight(design.WeightMedium).
					TextColor(p.TextSecondary).Selectable()
			})
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				kit.Caption(c, T("settings.about.by")).TextColor(p.TextTertiary)
				by := ui.ButtonBase(c.Key("author")).Label(aboutAuthor).Cursor(ui.CursorPointer).Tooltip(aboutAuthorURL)
				by.Children(func() {
					col := p.AccentText
					if by.Hovered() {
						col = p.Accent
					}
					kit.Caption(c, aboutAuthor).TextColor(col).FontWeight(design.WeightMedium)
				})
				if by.Clicked() {
					open(aboutAuthorURL)
				}
			})
		})
	})
	// The ask: a star helps others find it.
	kit.Group(c, "", func() {
		kit.SettingIconRow(c, icons.Star, ui.Color{}, T("settings.about.star"), T("settings.about.star.desc"), func() {
			if kit.Button(c.Key("star"), T("settings.about.star.action"), kit.ButtonOpts{Variant: kit.Primary, Icon: icons.Star, Tooltip: aboutRepoURL}).Clicked() {
				a.starRepo()
			}
		})
	})
	kit.Group(c, T("settings.about.links"), func() {
		link := func(key string, ic *ui.SVG, label, desc, url string) {
			kit.SettingIconRow(c, ic, ui.Color{}, label, desc, func() {
				b := kit.Button(c.Key(key), "", kit.ButtonOpts{Variant: kit.Ghost, Icon: icons.External, Tooltip: url})
				b.Label(label)
				if b.Clicked() {
					open(url)
				}
			})
		}
		link("src", icons.Code, T("settings.about.source"), "github.com/sinhong2011/mypack", aboutRepoURL)
		link("issue", icons.Message, T("settings.about.issue"), T("settings.about.issue.desc"), aboutRepoURL+"/issues/new/choose")
		link("notes", icons.Update, T("settings.about.releases"), T("settings.about.releases.desc"), aboutRepoURL+"/releases")
	})
	kit.Group(c, "", func() {
		kit.SettingBlock(c, T("settings.licenses"), T("settings.licenses.desc"), func() {
			ui.Text(c, licenseNotice).Font(kit.MonoFont).FontSize(10.5).TextColor(p.TextSecondary).LineHeight(1.5).Selectable().
				Padding(10, 12).Radius(design.RadiusControl).Background(p.Well)
		})
	})
}

func agentSnippet(tab int) string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "/Applications/mypack.app/Contents/MacOS/mypack"
	}
	switch tab {
	case 0:
		return fmt.Sprintf("claude mcp add mypack -- %q mcp serve", exe)
	case 1:
		return fmt.Sprintf("# ~/.codex/config.toml\n[mcp_servers.mypack]\ncommand = %q\nargs = [\"mcp\", \"serve\"]", exe)
	}
	return fmt.Sprintf("{\n  \"mcpServers\": {\n    \"mypack\": {\n      \"command\": %q,\n      \"args\": [\"mcp\", \"serve\"]\n    }\n  }\n}", exe)
}

const licenseNotice = `Mypack — Apache License 2.0
MyGo — github.com/egoist/mygo — MIT License (shader: Apache-2.0)
Lucide icons — lucide.dev — ISC License (portions MIT, Feather)
modernc.org/sqlite — BSD-3-Clause
golang.org/x/* — BSD-3-Clause
Full license texts ship in mypack.app/Contents/Resources/licenses.`
