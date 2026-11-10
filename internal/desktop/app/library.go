package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/components"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
	"mypack.dev/mypack/internal/domain"
)

type libState struct {
	tab         int // 0 My Apps, 1 Projects
	tabInit     bool
	query       string
	view        int // 0 grid, 1 list
	focusSearch bool
	searchH     ui.Handle
}

func (a *App) libraryView(c *ui.Context) {
	p := design.Of(c)
	l := &a.lib
	if !l.tabInit && a.loaded {
		l.tabInit = true
		l.tab = a.ui.LibraryTab
		if l.tab < 0 {
			l.tab = 1
			if len(a.installs) > 0 {
				l.tab = 0
			}
		}
		if a.settings.LibraryView == "list" {
			l.view = 1
		}
	}
	if !a.loaded {
		page(c, design.ContentMaxW, func() {
			ui.Grid(c).ColumnTracks(ui.Fr(1), ui.Fr(1), ui.Fr(1), ui.Fr(1)).Gap(16).Margin(56, 0, 0, 0).Children(func() {
				for i := 0; i < 4; i++ {
					lobeui.Skeleton(c.Key(i), 0, 156).Radius(design.RadiusCard)
				}
			})
		})
		return
	}
	if len(a.projects) == 0 && len(a.installs) == 0 {
		a.emptyLibrary(c)
		return
	}
	page(c, design.ContentMaxW, func() {
		// Sub-toolbar: tabs, search, view toggle.
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			updates := 0
			for _, in := range a.installs {
				if in.UpdateReason != "" {
					updates++
				}
			}
			myApps := T("library.tab.myApps")
			if updates > 0 {
				myApps += " · " + T("library.updates", "count", updates)
			}
			old := l.tab
			lobeui.Segmented(c.Key("tabs"), &l.tab, []string{myApps, T("library.tab.projects")}, lobeui.Middle)
			if old != l.tab {
				a.ui.LibraryTab = l.tab
				a.ui.save()
			}
			ui.Spacer(c)
			search := lobeui.SearchBar(c.Key("search"), &l.query, T("library.search"), "mod+f").Width(220)
			search.Bind(&l.searchH)
			if l.focusSearch {
				l.focusSearch = false
				l.searchH.Focus()
			}
			ov := l.view
			ui.Row(c).Gap(2).Padding(2).Radius(design.RadiusControl+1).Background(p.Hover).Children(func() {
				for i, ic := range []*ui.SVG{lobeui.IconGrid(), lobeui.IconList()} {
					i := i
					b := ui.ButtonBase(c.Key(i)).Label([]string{T("library.view.grid"), T("library.view.list")}[i]).Size(28, 26).Center().
						Radius(design.RadiusSM).Cursor(ui.CursorPointer).Selected(l.view == i)
					fg := p.TextTertiary
					if l.view == i {
						b.Background(p.Surface).Shadow(0, 1, 2, 0, p.Shadow)
						fg = p.Text
					}
					b.Children(func() { ui.Icon(c, ic).FontSize(15).TextColor(fg) })
					if b.Clicked() {
						l.view = i
					}
				}
			})
			if ov != l.view {
				a.settings.LibraryView = []string{"grid", "list"}[l.view]
				st := a.settings
				go func() { _ = a.svc.SaveSettings(bgctx(), st) }()
			}
		})
		if l.tab == 0 {
			a.myApps(c)
		} else {
			a.projectsGrid(c)
		}
	})
}

func (a *App) emptyLibrary(c *ui.Context) {
	p := design.Of(c)
	ui.Column(c).Grow(1).Center().Padding(0, 32, 60, 32).Children(func() {
		ui.Column(c).Width(560).MaxWidthPercent(100).Gap(18).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).AlignItems(ui.Center).Gap(10).Children(func() {
				components.BrandMark(c, 72).Margin(0, 0, 6, 0)
				ui.Text(c, T("library.empty.title")).FontSize(design.FontDisplay).FontWeight(design.WeightSemibold).
					TextColor(p.Text).LetterSpacing(-0.5).TextAlign(ui.Center).Role(ui.RoleHeading)
				ui.Text(c, T("library.empty.body")).FontSize(design.FontBodyLG).TextColor(p.TextSecondary).TextAlign(ui.Center)
			})
			s := &a.create
			r := components.URLHero(c.Key("hero"), &s.input, components.URLHeroOpts{
				Placeholder: T("create.url.placeholder"), Label: T("create.url.label"), AutoFocus: true,
				Action: T("library.empty.cta"), ActionOff: strings.TrimSpace(s.input) == "",
				OnPaste: func(text string) bool {
					if t := strings.TrimSpace(text); t != "" {
						s.setInput(t)
						a.nav(secCreate)
						return true
					}
					return false
				},
			})
			if len(r.Dropped) > 0 {
				s.setInput(r.Dropped[0])
				a.nav(secCreate)
			}
			if (r.Action || r.Submitted) && strings.TrimSpace(s.input) != "" {
				s.setInput(s.input)
				a.nav(secCreate)
			}
			ui.Row(c).Justify(ui.Center).Gap(6).Children(func() {
				components.Caption(c, T("create.drop.hint"))
				if components.Button(c.Key("choose"), T("create.chooseFile"), components.ButtonOpts{Variant: components.Plain, Size: 18}).FontSize(design.FontSmall).Clicked() {
					a.nav(secCreate)
					a.chooseLocalSource()
				}
			})
			ui.Column(c).Margin(14, 0, 0, 0).Children(func() { a.createSuggestions(c) })
		})
	})
}

func (a *App) filter(name, domain string) bool {
	q := strings.ToLower(strings.TrimSpace(a.lib.query))
	return q == "" || strings.Contains(strings.ToLower(name), q) || strings.Contains(strings.ToLower(domain), q)
}

func (a *App) grid(c *ui.Context, body func()) {
	if a.lib.view == 1 {
		ui.Column(c).Gap(2).Children(body)
		return
	}
	ui.Grid(c).ColumnTracks(ui.Fr(1), ui.Fr(1), ui.Fr(1), ui.Fr(1)).Gap(14).Children(body)
}

func (a *App) projectsGrid(c *ui.Context) {
	n := 0
	a.grid(c, func() {
		for i := range a.projects {
			pr := &a.projects[i]
			dom := domainOf(pr.Config.App.Source)
			if !a.filter(pr.Config.App.Name, dom) {
				continue
			}
			n++
			st, tone, busy := a.projectStatus(pr)
			meta := T("library.meta.edited", "when", ago(pr.UpdatedAt))
			if pr.LastBuild != nil {
				meta = T("library.meta.built", "when", ago(*pr.LastBuild))
			}
			prc := *pr
			d := components.AppCardData{Icon: pr.IconPNG, Name: pr.Config.App.Name, Domain: dom, Meta: meta, Status: st, Tone: tone, Busy: busy,
				Primary: T("action.build"), MoreText: T("common.more"), Menu: func(m *ui.Menu) { a.projectMenu(m, &prc) }}
			var r components.AppCardResult
			if a.lib.view == 1 {
				r = components.AppRow(c.Key(pr.ID), d)
			} else {
				r = components.AppCard(c.Key(pr.ID), d)
			}
			if r.Primary {
				a.startBuild(pr.ID, nil)
			} else if r.Open {
				a.openProject(pr.ID, 0)
			}
		}
	})
	if n == 0 {
		a.noMatches(c, len(a.projects) == 0)
	}
}

func (a *App) installStatus(in *domain.Installation) (string, components.Tone) {
	switch {
	case in.State == domain.InstallExternal:
		return T("status.readOnly"), components.ToneNeutral
	case in.State == domain.InstallMissing:
		return T("status.missing"), components.ToneWarning
	case in.State == domain.InstallAttention:
		return T("status.attention"), components.ToneDanger
	case in.UpdateReason != "":
		return T("status.update"), components.ToneAccent
	}
	return T("status.installed"), components.ToneSuccess
}

func (a *App) myApps(c *ui.Context) {
	p := design.Of(c)
	n := 0
	a.grid(c, func() {
		for i := range a.installs {
			in := a.installs[i]
			dom := in.SourceURL
			if pr := a.project(in.ProjectID); pr != nil {
				dom = domainOf(pr.Config.App.Source)
			} else if dom != "" {
				dom = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(dom, "https://"), "http://"), "/")
			}
			if !a.filter(in.Name, dom) {
				continue
			}
			n++
			st, tone := a.installStatus(&in)
			meta := T("myapps.version", "v", in.Version)
			if in.State == domain.InstallExternal {
				meta = shortPath(in.Path)
			}
			primary := T("action.open")
			if in.State == domain.InstallMissing {
				primary = T("action.locate")
			}
			d := components.AppCardData{Icon: in.IconPNG, Name: in.Name, Domain: dom, Meta: meta, Status: st, Tone: tone,
				Primary: primary, MoreText: T("common.more"), Menu: func(m *ui.Menu) { a.appMenu(m, in) }}
			var r components.AppCardResult
			if a.lib.view == 1 {
				r = components.AppRow(c.Key(in.ID), d)
			} else {
				r = components.AppCard(c.Key(in.ID), d)
			}
			if r.Primary {
				if in.State == domain.InstallMissing {
					a.dlg.details = detailsDialog{open: true, id: in.ID}
				} else {
					a.openInstall(in.ID)
				}
			} else if r.Open {
				a.dlg.details = detailsDialog{open: true, id: in.ID}
			}
		}
	})
	if n == 0 {
		if len(a.installs) == 0 {
			components.EmptyState(c, lobeui.IconInbox(), T("myapps.empty.title"), T("myapps.empty.body"), func() {
				if components.Button(c.Key("imp"), T("menu.importApp"), components.ButtonOpts{Icon: lobeui.IconImport()}).Clicked() {
					a.importExistingApp()
				}
			})
		} else {
			a.noMatches(c, false)
		}
	}
	if len(a.installs) > 0 {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lobeui.IconInfo()).FontSize(13).TextColor(p.TextTertiary)
			components.Caption(c, T("myapps.footer"))
			ui.Spacer(c)
			if components.Button(c.Key("imp2"), T("menu.importApp"), components.ButtonOpts{Variant: components.Ghost, Size: design.ControlSM, Icon: lobeui.IconImport()}).Clicked() {
				a.importExistingApp()
			}
		})
	}
}

func (a *App) noMatches(c *ui.Context, empty bool) {
	if empty {
		components.EmptyState(c, lobeui.IconPackage(), T("projects.empty.title"), T("projects.empty.body"), func() {
			if components.Button(c.Key("new"), T("toolbar.buildWebsite"), components.ButtonOpts{Variant: components.Primary, Icon: lobeui.IconPlus()}).Clicked() {
				a.newApp()
			}
		})
		return
	}
	components.EmptyState(c, lobeui.IconSearch(), T("library.noMatches", "query", a.lib.query), "", nil)
}

// appMenu is the My Apps context menu.
func (a *App) appMenu(m *ui.Menu, in domain.Installation) {
	managed := in.State != domain.InstallExternal
	if in.State == domain.InstallMissing {
		if m.Item(T("action.locate")).Chosen() {
			a.dlg.details = detailsDialog{open: true, id: in.ID}
		}
	} else if m.Item(T("action.open")).Chosen() {
		a.openInstall(in.ID)
	}
	if managed && in.UpdateReason != "" {
		if m.Item(T("action.reviewUpdate")).Chosen() {
			a.dlg.openUpdate(&in)
		}
	}
	if managed && in.ProjectID != "" {
		if m.Item(T("action.edit")).Chosen() {
			a.openProject(in.ProjectID, 1)
		}
		if m.Item(T("action.repair")).Chosen() {
			a.dlg.repair = installDialog{open: true, id: in.ID}
		}
	}
	if !managed && m.Item(T("action.createCopy")).Chosen() {
		a.create.reset(a.host)
		a.create.setInput(in.SourceURL)
		a.create.name, a.create.nameEdited = in.Name, true
		a.nav(secCreate)
	}
	if m.Item(T("action.reveal")).Disabled(in.State == domain.InstallMissing).Chosen() {
		a.reveal(in.Path)
	}
	if m.Item(T("action.details")).Chosen() {
		a.dlg.details = detailsDialog{open: true, id: in.ID}
	}
	if managed {
		m.Separator()
		if m.Item(T("action.uninstall")).Danger().Chosen() {
			a.dlg.uninstall = uninstallDialog{open: true, id: in.ID}
		}
	}
}
