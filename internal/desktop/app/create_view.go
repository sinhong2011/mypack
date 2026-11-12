package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

const heroIcon float32 = 128

// createView is the URL → app transformation: the address, the app's
// identity and Build action, and a stage previewing the app (the build
// plays there), beside the settings inspector.
func (a *App) createView(c *ui.Context) {
	s := &a.create
	if s.input != s.lastInput {
		s.lastEdit = time.Now()
		s.analyze()
	}
	a.maybeFetchMetadata(c)

	if !s.ready() {
		a.createStart(c)
		if s.buildRequested {
			s.buildRequested = false
		}
		return
	}

	// Building (or built): the scene plays right here in the preview.
	var bpr *domain.Project
	var bjob *domain.BuildJob
	if s.building != "" {
		bpr = a.project(s.building)
		if id, ok := a.watch[s.building]; ok {
			bjob = a.jobByID(id)
		}
	}
	a.previewInspector(c, func() {
		if s.building != "" {
			if bpr != nil && bjob != nil {
				a.createBuilding(c, bpr, bjob)
			} else {
				// The project and its job are still arriving: keep the
				// same view (identity, stage) and say it's starting.
				a.createStarting(c)
			}
			return
		}
		a.createURLField(c, 46)
		a.createPreview(c)
	}, func() {
		a.settingHits, a.settingsQ = 0, ""
		if s.adv != nil {
			a.settingsQ = strings.TrimSpace(s.adv.query)
		}
		// While its build runs the settings are locked: controls dim, labels
		// stay readable.
		locked := s.building != ""
		col := ui.Column(c).Gap(22)
		if locked {
			col.Opacity(0.8) // quieter, still readable
		}
		col.Children(func() {
			if locked {
				// Holds the pointer: the settings can't change mid-build.
				ui.Box(c.Key("lock")).Absolute().Fill().ZIndex(10).Cursor(ui.CursorDefault).AccessHidden(true)
			}
			searching := a.settingsQ != ""
			// Look & Behaviour: the common choices, with good defaults.
			ui.Column(c).Gap(14).Children(func() {
				if !searching {
					kit.SectionHeader(c, T("look.title"), "")
				}
				if a.settingHit(T("autoInstall.label")) {
					a.autoInstallSetting(c)
				}
				a.lookAndBehaviourOpts(c, a.ensureCfg(), func() {}, &s.account, true)
			})
			// Advanced Settings (collapsed; searching opens it to its matches).
			ui.Column(c).Gap(10).Children(func() {
				if !searching {
					kit.Disclosure(c.Key("adv"), &s.advOpen, T("create.advanced"), "").Margin(0, -8)
				}
				if s.advOpen || searching {
					a.ensureCfg()
					a.settingsEditor(c, s.adv, nil)
				}
			})
			a.searchEmpty(c)
			if !searching {
				a.createTargets(c)
			}
		})
	}, nil, a.createInspectorHead(c)...)
	if s.buildRequested {
		s.buildRequested = false
		a.createAndBuild()
	}
}

// createPreview is the live preview, on one centred axis: the app's icon,
// its name (rename in place) and host, the stage with a live mockup of its
// window that follows the inspector's settings (and the Dock), and the
// Build pill in a footer under it.
func (a *App) createPreview(c *ui.Context) {
	p := design.Of(c)
	s := &a.create
	loading := s.normalized != "" && s.meta == nil && s.metaFor != s.normalized
	ui.Column(c.Key("identity")).AlignItems(ui.Center).Gap(2).Margin(18, 0, 0, 0).Children(func() {
		ui.Box(c).Size(identityIcon, identityIcon).Children(func() {
			if loading && s.iconPNG == nil {
				kit.Shimmer(c, identityIcon, identityIcon, design.AppIconRadius(identityIcon))
				return
			}
			a.iconPicker(c.Key("icon:"+s.metaFor), a.createIcon(), s.displayName(), identityIcon)
		}).Margin(0, 0, 8, 0)
		a.renameField(c, loading)
		ui.Row(c).Gap(5).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ic := icons.Link
			src := config.RegistrableHost(s.normalized)
			if s.isGroup() {
				ic, src = icons.Layers, T("group.sites", "n", len(s.more)+1)
			}
			if s.local != "" {
				ic, src = icons.Folder, shortPath(s.local)
			}
			kit.Glyph(c, ic, 12, p.TextTertiary)
			ui.Text(c, src).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine().MinWidth(0).Shrink(1)
			if loading {
				ui.Text(c, "· "+T("create.fetching")).FontSize(design.FontSmall).TextColor(p.TextTertiary).SingleLine()
			}
			if s.local != "" {
				if hint, ok := projectKind(s.local); ok {
					ui.Text(c, "· "+hint).FontSize(design.FontSmall).TextColor(p.TextTertiary).SingleLine()
				}
			}
		})
		// Couldn't reach the site: a compact hint, never a banner.
		if s.meta != nil && s.meta.Fallback && s.metaFor == s.normalized {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Justify(ui.Center).Margin(4, 0, 0, 0).Children(func() {
				kit.Glyph(c, icons.Info, 12, p.Warning)
				ui.Text(c, T("create.offline")).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine().MinWidth(0).Shrink(1)
			})
		}
	})
	a.e2eCreateHook()
	a.syncCreateGroup()
	a.createSites(c)
	a.createMockupStage(c)
	// The footer: Build, centred.
	ui.Row(c.Key("footer")).Justify(ui.Center).Margin(16, 0, 0, 0).Children(func() {
		if kit.Button(c.Key("build"), a.buildCTA(), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlLG,
			Loading: s.busy, Disabled: len(s.targets) == 0, Hotkey: "mod+b", Icon: icons.Build}).Clicked() {
			s.buildRequested = true
		}
	})
}

// createMockupStage is the stage with the live mockup of the app.
func (a *App) createMockupStage(c *ui.Context) {
	s := &a.create
	cfg := a.ensureCfg()
	a.stage(c, 0, func(h float32) {
		a.mockup(c, c.Key("mockup"), cfg, a.createIcon(), s.displayName(), h, false)
	})
}

// mockup is the live window mockup for a config; launched springs it
// open (a finished build's result).
func (a *App) mockup(c *ui.Context, key *ui.Context, cfg *config.Config, icon []byte, name string, h float32, launched bool) {
	p := design.Of(c)
	w := cfg.Window
	dark := w.DarkMode == "dark" || ((w.DarkMode == "" || w.DarkMode == "system") && p.Dark)
	tint := kit.IconTint(icon, name)
	rail := a.railOf(cfg)
	if len(rail) > 1 { // a group's page shows its first site
		tint = kit.IconTint(rail[0].Icon, rail[0].Name)
	}
	kit.AppMockup(key, kit.MockupOpts{Icon: icon, Name: name,
		Tint: tint, Seamless: w.TitleBarStyle == "overlay" || w.TitleBarStyle == "hidden",
		Dark: dark, W: w.Width, H: w.Height, Fill: w.Maximized, Tray: cfg.Native.Tray, Height: h, Launched: launched,
		Mode: w.EffectiveMode(), Edge: w.EffectiveSidebarEdge(), Rail: rail})
}

// railOf is the rail of a group config: its sites with their Library
// icons (pasted sites draw their initial).
func (a *App) railOf(cfg *config.Config) []kit.MockupSite {
	if !cfg.IsGroup() {
		return nil
	}
	var out []kit.MockupSite
	for _, m := range cfg.App.Group.Members {
		site := kit.MockupSite{Name: m.Name}
		if pr := a.project(m.ProjectID); pr != nil {
			site.Icon = pr.IconPNG
		} else if png, ok := a.siteIcons[m.URL]; ok {
			site.Icon = png
		}
		out = append(out, site)
	}
	return out
}

// autoInstallSetting is the inspector's first row: Install & Open when
// done, with the one-time download as its quiet sub-line until cached.
func (a *App) autoInstallSetting(c *ui.Context) {
	s := &a.create
	if !s.autoSet {
		s.autoInstall, s.autoSet = a.ui.autoInstallDefault(), true
	}
	kit.GroupBox(c).Children(func() {
		kit.SettingIconRow(c.Key("auto"), nil, ui.Color{}, T("autoInstall.label"), a.createDownloadHint(), func() {
			kit.Switch(c.Key("sw"), &s.autoInstall, T("autoInstall.label"))
		})
	})
}

// identityIcon is the preview's icon size.
const identityIcon float32 = 88

// stage is the preview's large panel, filling the column's remaining
// height: one surface step off the canvas, radius 16, no border. The
// footer under it keeps its room.
// footerRoom is the height kept under the stage for Create's footer.
const footerRoom = 16 + 56 + 8 + 16 + 12

func (a *App) stage(c *ui.Context, cap float32, body func(h float32)) {
	p := design.Of(c)
	st := ui.Column(c.Key("stage")).FillWidth().Radius(16).Background(p.Group).Margin(16, 0, 0, 0).Clip()
	// Fill the rest of the column's viewport, less the footer, but never
	// less than 300 pt. The footer's room is the same in every state (the
	// Build pill, the status block, the result bar and its caption), so
	// the stage never jumps as one gives way to the next.
	h := float32(300)
	if b, v := st.Bounds(), a.previewBox; b.H > 0 && v.H > 0 {
		h = max(h, v.Y+v.H-b.Y-28-footerRoom)
	}
	if cap > 0 {
		h = min(h, cap)
	}
	st.Height(h)
	st.Children(func() { body(h) })
}

// createStarting holds Create's view for the moment between pressing Build
// and the project and its job arriving: the same identity and stage, and
// "Starting…" in the footer.
func (a *App) createStarting(c *ui.Context) {
	p := design.Of(c)
	s := &a.create
	a.createIdentity(c, a.createIcon(), s.displayName(), "")
	a.createMockupStage(c)
	ui.Row(c.Key("footer")).Gap(8).AlignItems(ui.Center).Justify(ui.Center).Margin(16, 0, 0, 0).Children(func() {
		kit.Spinner(c, 14, p.TextTertiary)
		ui.Text(c, T("create.starting")).FontSize(design.FontBody).TextColor(p.TextSecondary)
	})
}

// createIdentity is the centred icon, name and host of an app that is
// building or built.
func (a *App) createIdentity(c *ui.Context, icon []byte, name, host string) {
	p := design.Of(c)
	ui.Column(c.Key("identity")).AlignItems(ui.Center).Gap(2).Margin(18, 0, 0, 0).Children(func() {
		kit.AppIconOpts(c, icon, name, identityIcon, kit.IconOpts{Shadow: true}).Margin(0, 0, 8, 0)
		kit.PageTitle(c, name).SingleLine()
		if host != "" {
			ui.Text(c, host).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine()
		}
	})
}

// createBuilding is Create while its build runs and after: the same
// centred identity; the build plays in the stage; the footer holds Cancel,
// then the result and its links.
func (a *App) createBuilding(c *ui.Context, pr *domain.Project, j *domain.BuildJob) {
	si := a.successOf(pr, j)
	a.createIdentity(c, pr.IconPNG, pr.Config.App.Name, addressLabel(pr.Config.App.Source, pr.Config.App.Name))
	// A failure keeps the stage short, so the failed step and its fix
	// sit in view under it.
	var cap float32
	if j.Status == domain.JobFailed {
		cap = 220
	}
	a.stage(c, cap, func(h float32) {
		switch j.Status {
		case domain.JobAwaitingApproval:
			ui.Column(c).Padding(4, 20, 20, 20).Children(func() { a.approvalCard(c, j) })
		case domain.JobSucceeded:
			// The result is the app, launched: its window springs open.
			a.mockup(c, c.Key("launched"), pr.Config, pr.IconPNG, pr.Config.App.Name, h, true)
		case domain.JobFailed:
			// The app as it would be; the step list below says why not.
			a.mockup(c, c.Key("mockup"), pr.Config, pr.IconPNG, pr.Config.App.Name, h, false)
		default:
			a.sceneFor(c, pr, j, h)
		}
	})
	ui.Column(c.Key("footer")).AlignItems(ui.Center).Gap(8).Margin(16, 0, 0, 0).Children(func() {
		a.buildFooter(c, pr, j)
	})
	switch j.Status {
	case domain.JobSucceeded:
		a.successExtras(c, j, si)
	}
}

// createStart is Create before an address: a centered, inviting field.
func (a *App) createStart(c *ui.Context) {
	p := design.Of(c)
	ui.Scroll(c).Grow(1).Basis(0).Children(func() {
		ui.Column(c).FillWidth().MinHeightPercent(100).Center().Padding(design.TitleBarH+24, 40, 48, 40).Children(func() {
			ui.Column(c).Width(600).MaxWidthPercent(100).Gap(0).AlignItems(ui.Stretch).Children(func() {
				ui.Text(c, T("create.subtitle")).FontSize(design.FontBody).TextColor(p.TextSecondary).Margin(0, 2, 10, 2)
				a.createURLField(c, 52)
				ui.Column(c).Margin(40, 0, 0, 0).Children(func() { a.createSuggestions(c) })
			})
		})
	})
}

func (a *App) createURLField(c *ui.Context, h float32) {
	p := design.Of(c)
	s := &a.create
	ui.Column(c).Gap(10).FillWidth().MaxWidth(640).Children(func() {
		r := kit.URLField(c.Key("url"), &s.input, kit.URLFieldOpts{
			Placeholder: T("create.url.placeholder"), Label: T("create.url.label"), Height: h,
			Loading: s.fetchingFor != "" && s.fetchingFor == s.normalized, Invalid: s.invalidMsg != "" && s.input != "",
			AutoFocus: true, Handle: &s.urlHandle, Paste: T("create.paste"), Open: T("create.open"),
			OnPaste: func(text string) bool {
				t := strings.TrimSpace(text)
				if t == "" {
					return false
				}
				// Paste replaces the field and fetches at once (auto-detect).
				s.setInput(t)
				return true
			},
		})
		if s.focus {
			s.focus = false
			s.urlHandle.Focus()
		}
		if r.Paste {
			if t := strings.TrimSpace(c.ReadClipboard()); t != "" {
				s.setInput(t)
			}
		}
		if len(r.Dropped) > 0 {
			s.setInput(r.Dropped[0])
		}
		if r.Submitted && s.ready() {
			s.buildRequested = true
		}
		if r.Open {
			a.openSource()
		}
		// One quiet line under the field, starting at its text edge.
		ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(0, 0, 0, 48).Children(func() {
			switch {
			case s.invalidMsg != "" && s.input != "" && time.Since(s.lastEdit) > 700*time.Millisecond:
				ui.Text(c, s.invalidMsg).FontSize(design.FontSmall).TextColor(p.Warning).SingleLine()
			case r.DragOver:
				ui.Text(c, T("create.drop.release")).FontSize(design.FontSmall).TextColor(p.AccentText).SingleLine()
			case s.ready():
			default:
				ui.Text(c, T("create.drop.hint")).FontSize(design.FontSmall).TextColor(p.TextTertiary).SingleLine().MinWidth(0).Shrink(1)
			}
		})
	})
}

// renameField edits the app name in place, as Finder renames: plain
// centered text that becomes a field on hover and focus.
func (a *App) renameField(c *ui.Context, loading bool) {
	p := design.Of(c)
	s := &a.create
	placeholder := s.displayName()
	if placeholder == "" {
		placeholder = T("create.name.placeholder")
	}
	shown := s.name
	if !s.nameEdited {
		shown = s.displayName()
	}
	if loading && shown == "" {
		kit.Shimmer(c, 160, 20, 6).Margin(4, 0, 4, 0)
		return
	}
	in := ui.TextInputBase(c.Key("name"), &shown).FontSize(design.FontTitle2).FontWeight(design.WeightSemibold).
		Placeholder(placeholder).PlaceholderColor(p.TextTertiary).TextColor(p.Text).FocusRing(false).Label(T("create.name.label")).
		TextAlign(ui.Center).Width(320).MaxWidthPercent(100).Height(30).Padding(0, 8).Radius(8).
		Transition(design.Transition(c)).Tooltip(T("create.name.tip"))
	if in.Focused() {
		kit.FieldStyle(c, in, true, false)
	} else if in.Hovered() {
		in.Background(p.Hover)
	}
	if in.Changed() {
		s.name = shown
		s.nameEdited = strings.TrimSpace(shown) != ""
	}
}

// dockIcons are neutral stand-ins for the apps around ours in the Dock.
var dockIcons = []struct {
	ic   *ui.SVG
	tint ui.Color
}{
	{icons.Folder, design.Paper[9]},
	{icons.Mail, design.Paper[9]},
	{icons.Note, design.Paper[9]},
	{icons.Photo, design.Paper[9]},
	{icons.Music, design.Paper[9]},
	{icons.Calendar, design.Paper[9]},
}

// dockPreview shows the new app sitting in a Dock, magnified, with the
// running dot under it.
func (a *App) dockPreview(c *ui.Context, loading bool) {
	s := &a.create
	a.dockWith(c, func(big float32) {
		if loading && s.iconPNG == nil {
			kit.Shimmer(c, big, big, design.AppIconRadius(big))
		} else {
			kit.AppIconOpts(c, a.createIcon(), s.displayName(), big, kit.IconOpts{Shadow: true})
		}
	})
}

// appDockPreview is the Dock with an app's icon in it.
func (a *App) appDockPreview(c *ui.Context, png []byte, name string) {
	a.dockWith(c, func(big float32) { kit.AppIconOpts(c, png, name, big, kit.IconOpts{Shadow: true}) })
}

// dockWith draws the little Dock with icon() in its middle slot.
func (a *App) dockWith(c *ui.Context, icon func(big float32)) {
	p := design.Of(c)
	const sz float32 = 40
	ui.Column(c).AlignItems(ui.Start).Gap(8).Margin(24, 0, 0, 0).AccessHidden(true).Children(func() {
		dock := ui.Row(c).Height(64).Padding(0, 12).Gap(10).AlignItems(ui.End).Radius(18).Background(p.Overlay).Border(0.5, p.BorderStrong)
		design.Elevate(c, dock, design.Floating)
		dock.Children(func() {
			for i := 0; i < 6; i++ {
				if i == 3 {
					ui.Column(c).AlignItems(ui.Center).Gap(3).Padding(0, 0, 3, 0).Children(func() {
						big := sz * 1.3
						holder := ui.Box(c).Size(big, big)
						holder.Children(func() { icon(big) })
						ui.Box(c).Size(4, 4).Radius(2).Background(p.Text.Alpha(0.6))
					})
					continue
				}
				d := dockIcons[i%len(dockIcons)]
				ui.Box(c).Margin(0, 0, 12, 0).Children(func() {
					kit.IconShadow(c, kit.SymbolTile(c, d.ic, d.tint, sz).Radius(design.AppIconRadius(sz)), sz*0.6)
				})
			}
		})
		ui.Text(c, T("create.dock.caption")).FontSize(design.FontCaption).TextColor(p.TextTertiary)
	})
}

// iconPicker shows an app icon with a menu: choose image, reset.
func (a *App) iconPicker(c *ui.Context, png []byte, name string, size float32) ui.Element {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(T("create.icon.change")).Size(size, size).Radius(design.AppIconRadius(size)).Cursor(ui.CursorPointer).Shrink(0)
	s := &a.create
	b.Menu(func(m *ui.Menu) {
		if m.Item(T("studio.open")).Chosen() {
			a.openIconStudio("")
		}
		m.Separator()
		if m.Item(T("create.icon.choose")).Chosen() {
			a.chooseIcon(func(bb []byte) { s.iconPNG, s.iconCustom = bb, true })
		}
		if m.Item(T("create.icon.reset")).Disabled(!s.iconCustom && s.iconStyle == nil).Chosen() {
			s.iconPNG, s.iconCustom, s.iconStyle = nil, false, nil
		}
	})
	hov := b.Hovered()
	b.Children(func() {
		kit.AppIconOpts(c, png, name, size, kit.IconOpts{Shadow: true})
		editBadge(c, p, size, hov)
	})
	b.Tooltip(T("create.icon.change"))
	return b
}

// editBadge is the pencil that appears on an icon while hovered.
func editBadge(c *ui.Context, p *design.Palette, size float32, show bool) {
	s := max(22, size*0.22)
	e := ui.Row(c).Absolute().Right(-s*0.25).Bottom(-s*0.25).Size(s, s).Radius(s/2).Center().
		Background(p.Raised).Border(1, p.BorderStrong).PassThrough().AccessHidden(true)
	e.Children(func() { kit.Glyph(c, icons.Edit, s*0.45, p.Text) })
	op := float32(0)
	if show {
		op = 1
	}
	e.Opacity(e.AnimateTo("op", op, design.Spring))
}

func (a *App) createSuggestions(c *ui.Context) {
	p := design.Of(c)
	ui.Column(c).Gap(12).Children(func() {
		ui.Text(c, T("create.try")).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.TextSecondary).
			TextAlign(ui.Center).AlignSelf(ui.Center)
		ui.Grid(c).ColumnTracks(ui.Fr(1), ui.Fr(1), ui.Fr(1)).Gap(8).Children(func() {
			for i, t := range templates {
				if i >= 6 {
					break
				}
				t := t
				chip := ui.ButtonBase(c.Key(t.url)).Label(t.name).Row().Justify(ui.Start).Height(48).Padding(0, 12, 0, 8).Gap(10).AlignItems(ui.Center).
					Radius(design.RadiusCard+2).Background(p.Group).Border(0.5, p.GroupBorder).Cursor(ui.CursorPointer).Transition(design.Transition(c))
				if chip.Hovered() {
					chip.Background(p.Hover)
				}
				kit.Lift(chip, chip.Hovered(), chip.Pressed())
				chip.Children(func() {
					a.presetIconView(c, t, 32, false)
					ui.Column(c).MinWidth(0).Children(func() {
						ui.Text(c, t.name).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine()
						ui.Text(c, hostOf(t.url)).FontSize(design.FontCaption).TextColor(p.TextTertiary).SingleLine()
					})
				})
				if chip.Clicked() {
					a.create.setInput(t.url)
					if a.section != secCreate {
						a.nav(secCreate)
					}
				}
			}
		})
	})
}

func (a *App) createTargets(c *ui.Context) {
	p := design.Of(c)
	s := &a.create
	// The same card and rows as every other setting: one switch per
	// platform, this Mac first; the others fold under a quiet last row.
	ui.Column(c).Gap(14).Children(func() {
		kit.SectionHeader(c, T("create.target.label"), "")
		kit.GroupBox(c).Children(func() {
			row := func(t config.Target, ic *ui.SVG, label, sub string, disabled bool) {
				_, on := s.targets[t.Key()]
				r := ui.Column(c.Key(t.Key()))
				if disabled {
					r.Opacity(0.5)
				}
				r.Children(func() {
					kit.SettingIconRow(c, ic, ui.Color{}, label, sub, func() {
						v := on
						kit.Switch(c.Key("sw"), &v, label)
						if v != on && !disabled {
							s.toggleTarget(t)
						}
					})
				})
			}
			host := config.DefaultTarget(a.host)
			row(host, icons.Laptop, thisDevice(a.host), "", false)
			if s.otherOpen {
				for _, t := range otherTargets(a.host) {
					unsupported := t.OS == "darwin" && a.host.OS != "darwin"
					sub := archLabel(t.OS, t.Arch)
					if unsupported {
						sub = T("target.needsMac")
					}
					ic := icons.Monitor
					if t.OS == "darwin" {
						ic = icons.Laptop
					}
					row(t, ic, osLabel(t.OS), sub, unsupported)
				}
			}
			more := ui.ButtonBase(c.Key("other")).Label(T("create.otherPlatforms")).Row().MinHeight(40).Padding(8, 14).Gap(12).
				AlignItems(ui.Center).Cursor(ui.CursorPointer).Role(ui.RoleDisclosure).Expanded(s.otherOpen).FocusRing(false).
				Transition(design.Transition(c))
			if more.Hovered() || more.FocusVisible() {
				more.Background(p.Hover)
			}
			more.Children(func() {
				kit.FocusMark(c, more.FocusVisible(), 0)
				ui.Box(c).Size(20, 20).Shrink(0)
				ui.Text(c, T("create.otherPlatforms")).FontSize(design.FontBody).TextColor(p.TextSecondary).Grow(1)
				ch := kit.Glyph(c, icons.ChevronRt, 14, p.TextTertiary)
				turn := float32(0)
				if s.otherOpen {
					turn = 90
				}
				ch.Rotate(ch.AnimateTo("turn", turn, design.Spring))
			})
			if more.Clicked() {
				s.otherOpen = !s.otherOpen
			}
		})
		if s.otherOpen {
			ui.Text(c, T("create.otherPlatforms.note")).FontSize(design.FontCaption).TextColor(p.TextSecondary).LineHeight(1.4).Padding(0, 2)
		}
	})
}

// createInspectorHead is the inspector's pinned search, only when the
// window is too narrow for the toolbar to hold it (createToolbar).
func (a *App) createInspectorHead(c *ui.Context) []func() {
	if !a.inspectorNarrow() {
		return nil
	}
	return []func(){func() {
		a.ensureCfg()
		a.settingsSearch(c, a.create.adv)
	}}
}

// createToolbar is the toolbar's part over the inspector in Create: the
// settings search, the width of the inspector's content, and Build.
func (a *App) createToolbar(c *ui.Context) {
	s := &a.create
	a.ensureCfg()
	ui.Row(c.Key("create-tools")).Width(a.inspectorWidth() - 26).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Grow(1).Basis(0).MinWidth(0).Children(func() { a.settingsSearch(c, s.adv) })
		if kit.Button(c.Key("tb-create-build"), a.buildCTA(), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlSM,
			Loading: s.busy, Disabled: len(s.targets) == 0, Icon: icons.Build}).Clicked() {
			s.buildRequested = true
		}
	})
}
