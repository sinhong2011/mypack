package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/pkg/config"
)

const heroIcon float32 = 128

// createView is the URL → app transformation: a large glass address
// field; as the site is looked up its icon shimmers, then pops into a real
// macOS app icon above an editable name, sitting in a little Dock.
func (a *App) createView(c *ui.Context) {
	p := design.Of(c)
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

	page(c, 640, func() {
		a.createURLField(c, 46)

		// The hero: icon, name, source and the Dock preview.
		ui.Column(c).AlignItems(ui.Center).Gap(0).Padding(28, 24, 24, 24).Radius(28).Children(func() {
			ui.Box(c).Absolute().Fill().PassThrough().AccessHidden(true).Draw(func(pt *ui.Painter, r ui.Rect) { design.PaintDots(pt, r, p, true) })
			loading := s.normalized != "" && s.meta == nil && s.metaFor != s.normalized
			ui.Box(c).Size(heroIcon, heroIcon).Children(func() {
				if loading && s.iconPNG == nil {
					kit.Shimmer(c, heroIcon, heroIcon, design.AppIconRadius(heroIcon))
					return
				}
				icon := a.iconPicker(c.Key("icon:"+s.metaFor), a.createIcon(), s.displayName(), heroIcon)
				entered := ui.Local(icon, "in", func() bool { return false })
				pose := ui.Pose{}
				if !*entered {
					pose, *entered = ui.Pose{Scale: 0.82, Fade: 1}, true
					c.Invalidate()
				}
				icon.AnimatePose(pose, design.SpringPop)
			})
			a.renameField(c, loading)
			ui.Row(c).Gap(5).AlignItems(ui.Center).Margin(2, 0, 0, 0).Children(func() {
				ic := icons.Link
				src := config.RegistrableHost(s.normalized)
				if s.local != "" {
					ic, src = icons.Folder, shortPath(s.local)
				}
				kit.Glyph(c, ic, 12, p.TextTertiary)
				ui.Text(c, src).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine().MinWidth(0)
				if loading {
					ui.Text(c, "· "+T("create.fetching")).FontSize(design.FontSmall).TextColor(p.TextTertiary).SingleLine()
				}
			})
			a.dockPreview(c, loading)
		})
		if s.meta != nil && s.meta.Fallback && s.metaFor == s.normalized {
			kit.Note(c, icons.Info, T("create.offline"), kit.ToneWarning)
		}

		// Look & Behaviour: the common choices, with good defaults.
		ui.Column(c).Gap(14).Children(func() {
			kit.SectionHeader(c, T("look.title"), "")
			a.lookAndBehaviour(c, a.ensureCfg(), func() {}, &s.account)
		})

		a.createTargets(c)

		// Advanced Settings (collapsed): an inspector-style grouped form.
		ui.Column(c).Gap(10).Children(func() {
			kit.Disclosure(c.Key("adv"), &s.advOpen, T("create.advanced"), T("create.advanced.sub")).Margin(0, -8)
			if s.advOpen {
				a.ensureCfg()
				a.settingsEditor(c, s.adv, nil)
			}
		})

		// The primary call to action.
		ui.Column(c).AlignItems(ui.Center).Gap(10).Margin(4, 0, 0, 0).Children(func() {
			label := a.buildCTA()
			if kit.Button(c.Key("build"), label, kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlXL,
				Loading: s.busy, Disabled: len(s.targets) == 0, Hotkey: "mod+b", Icon: icons.Build}).MinWidth(260).Clicked() {
				s.buildRequested = true
			}
			ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
				if pf := a.createDownloadHint(); pf != "" {
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						kit.Glyph(c, icons.Download, 12, p.TextTertiary)
						kit.Footnote(c, pf)
					})
				}
				if kit.Button(c.Key("cancel"), T("common.cancel"), kit.ButtonOpts{Variant: kit.Plain, Size: 20}).FontSize(design.FontSmall).Clicked() {
					a.nav(secLibrary)
				}
			})
		})
	})
	if s.buildRequested {
		s.buildRequested = false
		a.createAndBuild()
	}
}

// createStart is Create before an address: a centered, inviting field.
func (a *App) createStart(c *ui.Context) {
	p := design.Of(c)
	ui.Scroll(c).Grow(1).Basis(0).Children(func() {
		ui.Column(c).FillWidth().MinHeightPercent(100).Center().Padding(design.TitleBarH+24, 40, 48, 40).Children(func() {
			ui.Column(c).Width(600).MaxWidthPercent(100).Gap(0).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).AlignItems(ui.Center).Gap(10).Margin(0, 0, 26, 0).Children(func() {
					kit.BrandMark(c, 72).Margin(0, 0, 8, 0)
					kit.LargeTitle(c, T("create.title")).TextAlign(ui.Center)
					ui.Text(c, T("create.subtitle")).FontSize(design.FontTitle3).TextColor(p.TextSecondary).TextAlign(ui.Center)
				})
				a.createURLField(c, 52)
				ui.Column(c).Margin(40, 0, 0, 0).Children(func() { a.createSuggestions(c) })
			})
		})
	})
}

func (a *App) createURLField(c *ui.Context, h float32) {
	p := design.Of(c)
	s := &a.create
	ui.Column(c).Gap(10).Children(func() {
		r := kit.URLField(c.Key("url"), &s.input, kit.URLFieldOpts{
			Placeholder: T("create.url.placeholder"), Label: T("create.url.label"), Height: h,
			Loading: s.fetchingFor != "" && s.fetchingFor == s.normalized, Invalid: s.invalidMsg != "" && s.input != "",
			AutoFocus: true, Handle: &s.urlHandle, Paste: T("create.paste"),
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
		ui.Row(c).Gap(6).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			switch {
			case s.invalidMsg != "" && s.input != "" && time.Since(s.lastEdit) > 700*time.Millisecond:
				ui.Text(c, s.invalidMsg).FontSize(design.FontSmall).TextColor(p.Warning).TextAlign(ui.Center)
			case r.DragOver:
				ui.Text(c, T("create.drop.release")).FontSize(design.FontSmall).TextColor(p.AccentText)
			case s.ready():
			default:
				kit.Footnote(c, T("create.drop.hint"))
				if kit.Button(c.Key("choose"), T("create.chooseFile"), kit.ButtonOpts{Variant: kit.Plain, Size: 18}).
					FontSize(design.FontSmall).Clicked() {
					a.chooseLocalSource()
				}
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
		kit.Shimmer(c, 160, 22, 8).Margin(18, 0, 8, 0)
		return
	}
	in := ui.TextInputBase(c.Key("name"), &shown).FontSize(design.FontTitle1).FontWeight(design.WeightBold).
		Placeholder(placeholder).PlaceholderColor(p.TextTertiary).TextColor(p.Text).FocusRing(false).Label(T("create.name.label")).
		TextAlign(ui.Center).Width(320).MaxWidthPercent(100).Height(34).Padding(0, 12).Radius(10).Margin(14, 0, 2, 0).
		Transition(design.Transition(c)).Tooltip(T("create.name.tip"))
	if in.Focused() {
		in.Background(p.FieldFill).Border(1, p.Accent.Alpha(0.7)).Shadow(0, 0, 0, 3, p.Accent.Alpha(0.16))
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
	p := design.Of(c)
	s := &a.create
	const sz float32 = 40
	ui.Column(c).AlignItems(ui.Center).Gap(8).Margin(24, 0, 0, 0).AccessHidden(true).Children(func() {
		dock := ui.Row(c).Height(64).Padding(0, 12).Gap(10).AlignItems(ui.End).Radius(18).Background(p.Overlay).Border(0.5, p.BorderStrong)
		design.Elevate(c, dock, design.Floating)
		dock.Children(func() {
			for i := 0; i < 6; i++ {
				if i == 3 {
					ui.Column(c).AlignItems(ui.Center).Gap(3).Padding(0, 0, 3, 0).Children(func() {
						big := sz * 1.3
						holder := ui.Box(c).Size(big, big)
						holder.Children(func() {
							if loading && s.iconPNG == nil {
								kit.Shimmer(c, big, big, design.AppIconRadius(big))
							} else {
								kit.AppIconOpts(c, a.createIcon(), s.displayName(), big, kit.IconOpts{Shadow: true})
							}
						})
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
		if m.Item(T("create.icon.choose")).Chosen() {
			a.chooseIcon(func(bb []byte) { s.iconPNG, s.iconCustom = bb, true })
		}
		if m.Item(T("create.icon.reset")).Disabled(!s.iconCustom).Chosen() {
			s.iconPNG, s.iconCustom = nil, false
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
		Background(p.Raised).Border(0.5, p.BorderStrong).Shadow(0, 2, 6, 0, p.Shadow).PassThrough().AccessHidden(true)
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
	ui.Column(c).Gap(10).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			kit.Overline(c, T("create.target.label"))
			ui.Spacer(c)
			if kit.Button(c.Key("other"), T("create.otherPlatforms"), kit.ButtonOpts{Variant: kit.Plain, Size: 22,
				Trailing: map[bool]*ui.SVG{true: icons.ChevronDown, false: icons.ChevronRt}[s.otherOpen]}).FontSize(design.FontSmall).Clicked() {
				s.otherOpen = !s.otherOpen
			}
		})
		host := config.DefaultTarget(a.host)
		ui.Grid(c).ColumnTracks(ui.Fr(1), ui.Fr(1)).Gap(8).Children(func() {
			_, on := s.targets[host.Key()]
			if kit.TargetChip(c.Key("host"), icons.Laptop, thisDevice(a.host), T("target.recommended"), on, false).Clicked() {
				s.toggleTarget(host)
			}
			if s.otherOpen {
				for _, t := range otherTargets(a.host) {
					t := t
					_, on := s.targets[t.Key()]
					unsupported := t.OS == "darwin" && a.host.OS != "darwin"
					sub := archLabel(t.OS, t.Arch)
					if unsupported {
						sub = T("target.needsMac")
					}
					ic := icons.Monitor
					if t.OS == "darwin" {
						ic = icons.Laptop
					}
					if kit.TargetChip(c.Key(t.Key()), ic, osLabel(t.OS), sub, on, unsupported).Clicked() {
						s.toggleTarget(t)
					}
				}
			}
		})
		if s.otherOpen {
			ui.Text(c, T("create.otherPlatforms.note")).FontSize(design.FontCaption).TextColor(p.TextSecondary).LineHeight(1.4).Padding(0, 4)
		}
	})
}
