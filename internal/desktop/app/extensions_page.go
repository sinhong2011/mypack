package app

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/catalog"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/runtime/userscript"
)

// The Extensions page: two tabs, Installed and Hub, like an app store's
// "Updates" and "Discover", with one search over the tab shown. Rows and
// cards open the Details sheet; the toolbar's Add Extension opens a sheet
// with the three ways to add one (address, file, write it).

// extTabsGlide pairs the tabs' thumb across its segments (kit.Glide).
type extTabsGlide struct{}

// extToolbar is the page's primary action, where New App is elsewhere.
func (a *App) extToolbar(c *ui.Context) {
	if a.ext.edit.open {
		a.extEditorToolbar(c)
		return
	}
	if kit.NewPill(c.Key("tb-ext"), T("ext.add.title"), "", a.toolbarW > 0 && a.toolbarW < 700).Clicked() {
		a.ext.adding = true
	}
}

func (a *App) extensionsView(c *ui.Context) {
	if !a.ext.loaded {
		a.reloadInstalledExtensions()
	}
	if a.ext.edit.open {
		a.extEditorView(c)
		return
	}
	page(c, func() {
		a.extTabsRow(c)
		body := ui.Column(c.Key(fmt.Sprint("tab", a.ext.tab))).Gap(24)
		kit.TabSwap(body, a.ext.tabDir).Children(func() {
			if a.ext.tab == 0 {
				a.extInstalledTab(c)
			} else {
				a.extHubTab(c)
			}
		})
	})
	a.extAddSheet(c)
	a.extReviewSheet(c)
	a.extDetailSheet(c)
}

// extTabsRow: the tabs, and the search of the tab shown.
func (a *App) extTabsRow(c *ui.Context) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		segs := []kit.PillSegment{
			{Label: T("ext.tab.installed", "n", len(a.ext.installed))},
			{Label: T("ext.tab.hub", "n", len(a.ext.hub))},
		}
		was := a.ext.tab
		if kit.PillSegmented(c.Key("tabs"), &a.ext.tab, segs, T("nav.extensions"), extTabsGlide{}, true) {
			a.ext.tabDir = 1
			if a.ext.tab < was {
				a.ext.tabDir = -1
			}
		}
		ui.Spacer(c)
		ph := T("ext.search.installed")
		if a.ext.tab == 1 {
			ph = T("ext.search.hub")
		}
		kit.SearchField(c.Key("q"), &a.ext.query, ph, nil, 260)
	})
}

// ---- Installed -------------------------------------------------------------------

func (a *App) extInstalledTab(c *ui.Context) {
	p := design.Of(c)
	if len(a.ext.installed) == 0 {
		kit.EmptyState(c, icons.Puzzle, T("ext.empty.title"), T("ext.empty.body"), func() {
			if kit.Button(c.Key("browse"), T("ext.empty.browse"), kit.ButtonOpts{Variant: kit.Primary}).Clicked() {
				a.ext.tab, a.ext.tabDir = 1, 1
			}
			if kit.Button(c.Key("add"), T("ext.add.title"), kit.ButtonOpts{Icon: icons.Plus}).Clicked() {
				a.ext.adding = true
			}
		})
		return
	}
	var shown []userscript.Entry
	on := 0
	for _, e := range a.ext.installed {
		if e.Enabled {
			on++
		}
		m := a.ext.metas[e.ID]
		if extMatches(a.ext.query, e.Name, e.Description, strings.Join(m.Sites(), " ")) {
			shown = append(shown, e)
		}
	}
	kit.Note(c, icons.Info, T("ext.runtimeNote"), kit.ToneNeutral)
	ui.Column(c.Key("installed")).Gap(8).Children(func() {
		kit.Eyebrow(c, T("ext.installed.count", "on", on, "n", len(a.ext.installed))).Padding(0, 2)
		if len(shown) == 0 {
			ui.Text(c, T("ext.search.none")).FontSize(design.FontBody).TextColor(p.TextSecondary).Padding(8, 2)
			return
		}
		kit.GroupBox(c).Children(func() {
			for _, e := range shown {
				a.extInstalledRow(c.Key(e.ID), e)
			}
		})
	})
}

// extInstalledRow: the whole row opens Details; the switch turns it on or
// off where it is.
func (a *App) extInstalledRow(c *ui.Context, e userscript.Entry) {
	p := design.Of(c)
	m := a.ext.metas[e.ID]
	row := ui.ButtonBase(c).Label(e.Name).Row().Justify(ui.Start).Padding(12, 14).Gap(12).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Transition(design.Transition(c))
	if row.Hovered() {
		row.Background(p.Fill)
	}
	row.Children(func() {
		kit.Placeholder(c, e.Name, 38)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				name := ui.Text(c, e.Name).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
				if !e.Enabled {
					name.TextColor(p.TextSecondary)
				}
				if e.Version != "" {
					ui.Text(c, e.Version).FontSize(design.FontCaption).TextColor(p.TextTertiary)
				}
				switch {
				case e.Edited:
					kit.StatusPill(c, T("ext.editor.editedPill"), kit.ToneAccent)
				case a.hubFor(e) != nil:
					kit.StatusPill(c, T("ext.badge.reviewed"), kit.ToneSuccess)
				}
			})
			if e.Description != "" {
				ui.Text(c, e.Description).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine()
			}
			scope := T("ext.scope.all")
			if len(e.Apps) > 0 {
				scope = T("ext.scope.some", "n", len(e.Apps))
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				kit.Glyph(c, icons.Globe, 12, p.TextTertiary)
				ui.Text(c, sitesLabel(m.Sites())+"  ·  "+scope).FontSize(design.FontCaption).TextColor(p.TextTertiary).SingleLine()
			})
		})
		if a.extBusy(e.ID) {
			kit.Spinner(c, 14, p.TextSecondary)
		}
		on := e.Enabled
		kit.Switch(c.Key("on"), &on, T("ext.enabled", "name", e.Name))
		if on != e.Enabled {
			a.setExtEnabled(e.ID, on)
		}
		kit.Glyph(c, icons.ChevronRt, 14, p.TextTertiary)
	})
	if row.Clicked() {
		a.openExtDetail(e.ID, "")
	}
}

// ---- Hub -------------------------------------------------------------------------

func (a *App) extHubTab(c *ui.Context) {
	p := design.Of(c)
	kit.Secondary(c, T("ext.hub.body")).LineHeight(1.45)
	var shown []catalog.Script
	for _, s := range a.ext.hub {
		if extMatches(a.ext.query, s.Name, s.Description, s.Author, strings.Join(s.Sites, " ")) {
			shown = append(shown, s)
		}
	}
	if len(shown) == 0 {
		ui.Text(c, T("ext.search.none")).FontSize(design.FontBody).TextColor(p.TextSecondary).Padding(8, 2)
		return
	}
	ui.Row(c.Key("cards")).Gap(14).Wrap().AlignItems(ui.Stretch).Children(func() {
		for _, s := range shown {
			a.extHubCard(c.Key(s.URL), s)
		}
	})
	// The way to everything else.
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(4, 2).Children(func() {
		kit.Glyph(c, icons.Info, 14, p.TextTertiary)
		ui.Text(c, T("ext.hub.more")).FontSize(design.FontSmall).TextColor(p.TextSecondary)
		if kit.Button(c.Key("more"), T("ext.add.title"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM, Icon: icons.Plus}).Clicked() {
			a.ext.adding = true
		}
	})
}

// extHubCard: the card opens Details; its button installs, or says it is.
func (a *App) extHubCard(c *ui.Context, s catalog.Script) {
	p := design.Of(c)
	inst := a.hubInstalled(s)
	card := ui.ButtonBase(c).Label(s.Name).Column().AlignItems(ui.Stretch).Justify(ui.Start).Width(320).Padding(18).Gap(12).Radius(design.RadiusPanel).
		Background(p.Raised).Border(0.5, p.GroupBorder).Cursor(ui.CursorPointer).Transition(design.Transition(c))
	design.Elevate(c, card, design.Lifted)
	kit.Lift(card, card.Hovered(), card.Pressed())
	card.Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			kit.Placeholder(c, s.Name, 46)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, s.Name).FontSize(design.FontBodyLG).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					kit.Glyph(c, icons.Shield, 12, p.Success)
					by := s.Author
					if by == "" {
						by = "MyPack"
					}
					ui.Text(c, T("ext.badge.reviewedBy", "author", by)).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine()
				})
			})
		})
		ui.Text(c, s.Description).FontSize(design.FontSmall).TextColor(p.TextSecondary).LineHeight(1.45).MaxLines(3).Grow(1)
		ui.Row(c).Gap(6).Wrap().Children(func() {
			for _, h := range s.Sites {
				ui.Row(c.Key(h)).Height(20).PaddingX(8).Radius(10).Background(p.Fill).AlignItems(ui.Center).Children(func() {
					ui.Text(c, h).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine()
				})
			}
		})
		kit.Hairline(c)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if inst != nil {
				kit.Glyph(c, icons.Check, 14, p.Success)
				state := T("ext.installed.pill")
				if !inst.Enabled {
					state = T("ext.installed.off")
				}
				ui.Text(c, state).FontSize(design.FontSmall).TextColor(p.TextSecondary).Grow(1)
				if kit.Button(c.Key("manage"), T("ext.manage"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
					a.openExtDetail(inst.ID, s.URL)
				}
				return
			}
			ui.Text(c, T("ext.hub.free")).FontSize(design.FontSmall).TextColor(p.TextTertiary).Grow(1)
			if kit.Button(c.Key("get"), T("ext.install"), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlSM,
				Loading: a.extBusy(s.URL)}).Clicked() {
				a.fetchForReview(s.URL, s.URL, s.SHA256, true)
			}
		})
	})
	if card.Clicked() {
		id := ""
		if inst != nil {
			id = inst.ID
		}
		a.openExtDetail(id, s.URL)
	}
}

// ---- Add Extension ---------------------------------------------------------------

// extAddSheet: from an address, from a file, or write a new one.
func (a *App) extAddSheet(c *ui.Context) {
	if !a.ext.adding {
		return
	}
	p := design.Of(c)
	kit.Sheet(c.Key("ext-add"), &a.ext.adding, 520, func() {
		kit.SheetHeader(c, T("ext.add.title"), func() {
			ui.Box(c).Size(36, 36).Radius(10).Center().Background(p.AccentSoft).Children(func() {
				kit.Glyph(c, icons.Puzzle, 18, p.Accent)
			})
		})
		kit.SheetBody(c, T("ext.add.body"))
		// From an address.
		kit.Group(c, T("ext.add.fromAddress"), func() {
			ui.Column(c).Padding(12, 14).Gap(8).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					kit.TextField(c.Key("addr"), &a.ext.addr, T("ext.add.url"), kit.FieldOpts{Placeholder: "https://greasyfork.org/…/script.user.js"}).Grow(1)
					u, err := url.Parse(strings.TrimSpace(a.ext.addr))
					valid := err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
					if kit.Button(c.Key("go"), T("ext.add.fetch"), kit.ButtonOpts{Variant: kit.Primary, Disabled: !valid, Loading: a.ext.addrBusy}).Clicked() ||
						(valid && !a.ext.addrBusy && c.Shortcut(0, ui.KeyEnter)) {
						a.ext.addrBusy = true
						a.fetchForReview("addr", strings.TrimSpace(a.ext.addr), "", false)
					}
				})
				kit.Caption(c, T("ext.add.fromAddress.desc"))
			})
		})
		// From a file, or written here.
		kit.Group(c, T("ext.add.other"), func() {
			kit.SettingIconRow(c.Key("file"), icons.File, p.Accent, T("ext.add.file.title"), T("ext.add.file.desc"), func() {
				if kit.Button(c.Key("file"), T("ext.add.file"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
					a.ext.adding = false
					a.chooseScriptFile()
				}
			})
			kit.SettingIconRow(c.Key("new"), icons.Code, p.Accent, T("ext.add.new.title"), T("ext.add.new.desc"), func() {
				if kit.Button(c.Key("new"), T("ext.add.new"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
					a.ext.adding = false
					a.newExtension()
				}
			})
		})
		kit.Note(c, icons.Warning, T("ext.add.trust"), kit.ToneWarning)
		ui.Row(c).Justify(ui.End).Children(func() {
			if kit.Button(c.Key("close"), T("common.close"), kit.ButtonOpts{}).MinWidth(84).Clicked() {
				a.ext.adding = false
			}
		})
	})
}
