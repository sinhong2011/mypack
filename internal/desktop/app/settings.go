package app

import (
	"fmt"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/components"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
	"mypack.dev/mypack/internal/i18n"
)

type settingsState struct {
	agentTab    int
	scrollAbout bool
	registry    string
	regInit     bool
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
		// General.
		components.Group(c, T("settings.general"), func() {
			components.SettingRow(c, T("settings.language"), T("settings.language.desc"), func() {
				opts := []lobeui.Option{{Value: "system", Label: T("settings.language.system")}}
				for _, l := range i18n.Supported {
					opts = append(opts, lobeui.Option{Value: l.ID, Label: l.Autonym})
				}
				v := a.settings.Language
				if v == "" {
					v = "system"
				}
				old := v
				lobeui.Select(c.Key("lang"), &v, opts, "", false).Width(200)
				if v != old {
					a.settings.Language = v
					i18n.SetLocale(v)
					a.installMenu()
					a.syncTray()
					a.saveSettings()
				}
			})
			components.SettingRow(c, T("settings.menubar"), T("settings.menubar.desc"), func() {
				v := a.settings.ShowMenuBarIcon
				lobeui.Switch(c.Key("mb"), &v, T("settings.menubar"))
				if v != a.settings.ShowMenuBarIcon {
					a.settings.ShowMenuBarIcon = v
					a.syncTray()
					a.saveSettings()
				}
			})
			components.SettingRow(c, T("settings.updates"), T("settings.updates.desc"), func() {
				v := a.settings.CheckUpdates
				lobeui.Switch(c.Key("cu"), &v, T("settings.updates"))
				if v != a.settings.CheckUpdates {
					a.settings.CheckUpdates = v
					a.saveSettings()
				}
			})
		})

		// Appearance.
		components.Group(c, T("settings.appearance"), func() {
			components.SettingRow(c, T("settings.theme"), "", func() {
				idx := map[string]int{"system": 0, "light": 1, "dark": 2}[a.settings.Appearance]
				old := idx
				lobeui.Segmented(c.Key("ap"), &idx, []string{T("appearance.system"), T("appearance.light"), T("appearance.dark")}, lobeui.Small)
				if idx != old {
					a.settings.Appearance = []string{"system", "light", "dark"}[idx]
					applyAppearance(a.settings.Appearance)
					a.saveSettings()
				}
			})
		})

		// Builds.
		components.Group(c, T("settings.builds"), func() {
			dir := func(key, label, desc string, v *string) {
				components.SettingRow(c.Key(key), label, desc, func() {
					components.Mono(c, shortPath(*v)).SingleLine().MaxWidth(220)
					if components.Button(c.Key("ch"), T("common.choose"), components.ButtonOpts{Size: design.ControlSM}).Clicked() {
						go func() {
							paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Directory: true, CreateDirectories: true, DefaultPath: *v})
							if err == nil && len(paths) > 0 {
								a.win.Update(func() { *v = paths[0]; a.saveSettings() })
							}
						}()
					}
				})
			}
			dir("out", T("settings.outputDir"), T("settings.outputDir.desc"), &a.settings.OutputDir)
			dir("inst", T("settings.installDir"), T("settings.installDir.desc"), &a.settings.InstallDir)
		})

		// Agent Connections (spec §12.2): honest, copyable setup; no fake status.
		components.Group(c, T("settings.agents"), func() {
			ui.Column(c).Padding(14).Gap(12).Children(func() {
				ui.Text(c, T("settings.agents.desc")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				lobeui.Segmented(c.Key("agent"), &s.agentTab, []string{"Claude Code", "Codex", T("settings.agents.json")}, lobeui.Small)
				snippet := agentSnippet(s.agentTab)
				ui.Row(c).Gap(8).Padding(10, 12).Radius(design.RadiusControl+1).Background(p.Well).Border(1, p.Border).AlignItems(ui.Start).Children(func() {
					ui.Text(c, snippet).Font("Menlo").FontSize(11.5).TextColor(p.Text).Grow(1).MinWidth(0).Selectable().LineHeight(1.5)
					lobeui.CopyButton(c, snippet)
				})
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, lobeui.IconShield()).FontSize(13).TextColor(p.TextTertiary)
					components.Caption(c, T("settings.agents.note"))
				})
			})
		})

		// Advanced.
		components.Group(c, T("settings.advanced"), func() {
			if !s.regInit {
				s.regInit, s.registry = true, a.settings.RegistryURL
			}
			components.SettingBlock(c, T("settings.registry"), T("settings.registry.desc"), func() {
				ui.Row(c).Gap(8).Children(func() {
					lobeui.Input(c.Key("reg"), &s.registry, lobeui.InputOpts{Size: lobeui.Small, Placeholder: "https://…/registry.json"}).Font("Menlo")
					if components.Button(c.Key("regsave"), T("common.save"), components.ButtonOpts{Size: design.ControlSM, Disabled: s.registry == a.settings.RegistryURL}).Clicked() {
						a.settings.RegistryURL = s.registry
						a.saveSettings()
						a.toast(T("settings.registry.saved"), "", 1)
					}
				})
			})
		})

		// About.
		components.Group(c, T("settings.about"), func() {
			ui.Row(c).Padding(16).Gap(14).AlignItems(ui.Center).Children(func() {
				components.BrandMark(c, 48)
				ui.Column(c).Gap(2).Grow(1).Children(func() {
					ui.Text(c, "mypack").FontSize(design.FontTitle3).FontWeight(design.WeightSemibold).TextColor(p.Text)
					components.Caption(c, T("settings.about.version", "v", Version))
					components.Caption(c, T("settings.about.tagline"))
				})
			})
			components.SettingBlock(c, T("settings.licenses"), T("settings.licenses.desc"), func() {
				ui.Text(c, licenseNotice).Font("Menlo").FontSize(10.5).TextColor(p.TextSecondary).LineHeight(1.5).Selectable().
					Padding(10, 12).Radius(design.RadiusControl).Background(p.Well)
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
Lobe MyGo — github.com/lobehub/lobe-mygo — MIT License
Lucide icons — lucide.dev — ISC License
modernc.org/sqlite — BSD-3-Clause
golang.org/x/* — BSD-3-Clause
Full license texts ship in mypack.app/Contents/Resources/licenses.`
