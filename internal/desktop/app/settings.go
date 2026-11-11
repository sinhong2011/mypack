package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/i18n"
)

type settingsState struct {
	agentTab    int
	scrollAbout bool
	registry    string
	regInit     bool

	licKey  string
	licErr  string
	licBusy bool
}

func (a *App) saveSettings() {
	st := a.settings
	go func() {
		if err := a.svc.SaveSettings(bgctx(), st); err != nil {
			a.win.Update(func() { a.toastErr(err) })
		}
	}()
}

func (a *App) settingsView(c *ui.Context) {
	p := design.Of(c)
	s := &a.set
	page(c, 680, func() {
		// Header: the app, as System Settings' account card.
		ui.Row(c).Gap(16).Padding(16, 18).Radius(design.RadiusPanel).Background(p.Group).Border(0.5, p.GroupBorder).AlignItems(ui.Center).Children(func() {
			kit.BrandMark(c, 60)
			ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, "Mypack").FontSize(design.FontTitle1).FontWeight(design.WeightBold).TextColor(p.Text)
				kit.Secondary(c, T("settings.about.tagline"))
				kit.Caption(c, T("settings.about.version", "v", Version)).TextColor(p.TextTertiary)
			})
		})
		// General.
		kit.Group(c, T("settings.general"), func() {
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

		a.licenseGroup(c)

		// Appearance.
		kit.Group(c, T("settings.appearance"), func() {
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

		// Builds.
		kit.Group(c, T("settings.builds"), func() {
			dir := func(key, label, desc string, v *string, tint ui.Color) {
				kit.SettingIconRow(c.Key(key), icons.Folder, tint, label, desc, func() {
					kit.Mono(c, shortPath(*v)).SingleLine().MaxWidth(220)
					if kit.Button(c.Key("ch"), T("common.choose"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
						go func() {
							paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Directory: true, CreateDirectories: true, DefaultPath: *v})
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

		// Agent Connections (spec §12.2): honest, copyable setup; no fake status.
		kit.Group(c, T("settings.agents"), func() {
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

		// Advanced.
		kit.Group(c, T("settings.advanced"), func() {
			if !s.regInit {
				s.regInit, s.registry = true, a.settings.RegistryURL
			}
			kit.SettingBlock(c, T("settings.registry"), T("settings.registry.desc"), func() {
				ui.Row(c).Gap(8).Children(func() {
					kit.TextField(c.Key("reg"), &s.registry, T("settings.registry"), kit.FieldOpts{Placeholder: "https://…/registry.json", Mono: true}).Grow(1)
					if kit.Button(c.Key("regsave"), T("common.save"), kit.ButtonOpts{Size: design.ControlSM, Disabled: s.registry == a.settings.RegistryURL}).Clicked() {
						a.settings.RegistryURL = s.registry
						a.saveSettings()
						a.toast(T("settings.registry.saved"), "", 1)
					}
				})
			})
		})

		// About.
		kit.Group(c, T("settings.about"), func() {
			kit.SettingBlock(c, T("settings.licenses"), T("settings.licenses.desc"), func() {
				ui.Text(c, licenseNotice).Font(kit.MonoFont).FontSize(10.5).TextColor(p.TextSecondary).LineHeight(1.5).Selectable().
					Padding(10, 12).Radius(design.RadiusControl).Background(p.Well).Font(kit.MonoFont)
			})
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

const licenseNotice = `MyGo — github.com/egoist/mygo — MIT License
Lucide icons — lucide.dev — ISC License (portions MIT, Feather)
modernc.org/sqlite — BSD-3-Clause
golang.org/x/* — BSD-3-Clause
Full license texts ship in mypack.app/Contents/Resources/licenses.`

// licenseGroup is Settings › License (spec §9E): edition, free-app usage,
// offline activation and deactivation. No prices are shown.
func (a *App) licenseGroup(c *ui.Context) {
	p := design.Of(c)
	s, lic := &a.set, a.license
	kit.Group(c, T("settings.license"), func() {
		pro := lic.Valid
		note := T("license.freeNote", "limit", appservice.FreeManagedApps)
		label, tone := T("license.free"), kit.ToneNeutral
		if pro {
			note, label, tone = T("license.proNote"), T("license.pro"), kit.ToneAccent
		}
		kit.SettingIconRow(c, icons.Key, ui.Color{}, T("license.edition"), note, func() { kit.StatusPill(c, label, tone) })
		count := T("license.managed.unlimited", "used", lic.ManagedApps)
		if !pro {
			count = T("license.managed.count", "used", lic.ManagedApps, "limit", appservice.FreeManagedApps)
		}
		kit.SettingRow(c.Key("managed"), T("license.managed"), "", func() {
			ui.Text(c, count).FontSize(design.FontBody).TextColor(p.TextSecondary)
		})
		if pro {
			who := T("license.id")
			if lic.Email != "" {
				who = T("license.licensedTo", "email", lic.Email)
			}
			kit.SettingRow(c.Key("who"), who, lic.LicenseID, func() {
				if kit.Button(c.Key("deact"), T("license.deactivate"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
					a.dlg.deactivateLicense = licenseConfirm{open: true}
				}
			})
			return
		}
		if lic.Error != "" {
			ui.Column(c).Padding(12, 14, 0, 14).Children(func() {
				kit.Note(c, icons.Warning, T("license.savedInvalid"), kit.ToneWarning)
			})
		}
		kit.SettingBlock(c, T("license.key.label"), T("license.key.desc"), func() {
			ui.Row(c).Gap(8).Children(func() {
				kit.TextField(c.Key("lickey"), &s.licKey, T("license.key.label"), kit.FieldOpts{Placeholder: T("license.key.placeholder"), Invalid: s.licErr != "", Mono: true}).Grow(1)
				if kit.Button(c.Key("licgo"), T("license.activate"), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlSM, Loading: s.licBusy}).Clicked() {
					a.activateLicense()
				}
			})
			if s.licErr != "" {
				ui.Text(c, s.licErr).FontSize(design.FontSmall).TextColor(p.Danger).LineHeight(1.4)
			}
		})
	})
}

func (a *App) activateLicense() {
	s := &a.set
	key := strings.TrimSpace(s.licKey)
	if key == "" {
		s.licErr = T("license.empty")
		return
	}
	s.licBusy, s.licErr = true, ""
	a.do(func(ctx context.Context) error {
		_, err := a.svc.ActivateLicense(ctx, key, "gui")
		return err
	}, func(err error) {
		s.licBusy = false
		switch {
		case errCode(err) == appservice.CodeLicenseInvalid:
			s.licErr = T("license.invalid")
		case err != nil:
			s.licErr = friendly(err)
		default:
			s.licKey = ""
			a.reload("license")
			a.toast(T("license.activated"), "", 1)
		}
	})
}
