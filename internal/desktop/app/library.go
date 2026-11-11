package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
)

// viewGlide pairs the grid/list switch indicator across frames.
type viewGlide struct{}

type libState struct {
	query       string
	view        int // 0 grid, 1 list
	viewInit    bool
	focusSearch bool
	searchH     ui.Handle
}

const (
	tileMinW  float32 = 150
	tileGap   float32 = 8
	tileIcon  float32 = 88
	listIcon  float32 = 28
	listRowH  float32 = 44
	listPadX  float32 = 14
	listColSt float32 = 150
	listColV  float32 = 96
	listColM  float32 = 128
	listColA  float32 = 36
)

// draftCount is how many projects have no installed app.
func (a *App) draftCount() int {
	n := 0
	for i := range a.projects {
		if a.installFor(a.projects[i].ID) == nil {
			n++
		}
	}
	return n
}

// libraryToolbar holds the Library's toolbar items: the view switch and
// the search field.
func (a *App) libraryToolbar(c *ui.Context) {
	p := design.Of(c)
	l := &a.lib
	if !a.loaded || len(a.projects)+len(a.installs) == 0 {
		return
	}
	ov := l.view
	grp := ui.Row(c).Height(design.Control).Padding(2).Gap(0).Radius(design.RadiusControl + 1).Background(p.Fill).Role(ui.RoleRadioGroup).Label(T("library.view.label"))
	grp.Children(func() {
		for i, ic := range []*ui.SVG{icons.Grid, icons.List} {
			label := []string{T("library.view.grid"), T("library.view.list")}[i]
			b := ui.ButtonBase(c.Key(i)).Label(label).Size(32, design.Control-4).Center().Radius(design.RadiusControl - 1).Cursor(ui.CursorPointer).
				Role(ui.RoleRadio).Checked(l.view == i)
			fg := p.TextTertiary
			if l.view == i {
				fg = p.Text
			} else if b.Hovered() {
				fg = p.TextSecondary
			}
			b.Children(func() {
				if l.view == i {
					ind := ui.Box(c).Absolute().Fill().Radius(design.RadiusControl - 1).ZIndex(-1).Background(p.Raised)
					design.Elevate(c, ind, design.Lifted)
					kit.Glide(ind, viewGlide{})
				}
				kit.Glyph(c, ic, 15, fg)
			})
			b.Tooltip(label)
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
	kit.SearchField(c.Key("search"), &l.query, T("library.search"), &l.searchH, 200)
	if l.focusSearch {
		l.focusSearch = false
		l.searchH.Focus()
	}
}

func (a *App) libraryView(c *ui.Context) {
	l := &a.lib
	if !l.viewInit && a.loaded {
		l.viewInit = true
		if a.settings.LibraryView == "list" {
			l.view = 1
		}
	}
	if !a.loaded {
		page(c, design.ContentMaxW, func() {
			ui.Row(c).Gap(tileGap).Margin(36, 0, 0, 0).Children(func() {
				for i := 0; i < 5; i++ {
					ui.Column(c.Key(i)).Grow(1).Basis(0).AlignItems(ui.Center).Gap(10).Padding(14, 8).Children(func() {
						kit.Shimmer(c, tileIcon, tileIcon, design.AppIconRadius(tileIcon))
						kit.Shimmer(c, 80, 12, 6)
						kit.Shimmer(c, 56, 10, 5)
					})
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
		shown := 0
		// Installed apps.
		var apps []domain.Installation
		for _, in := range a.installs {
			if a.filter(in.Name, a.installDomain(&in)) {
				apps = append(apps, in)
			}
		}
		var drafts []*domain.Project
		for i := range a.projects {
			pr := &a.projects[i]
			if a.installFor(pr.ID) != nil {
				continue
			}
			if a.filter(pr.Config.App.Name, domainOf(pr.Config.App.Source)) {
				drafts = append(drafts, pr)
			}
		}
		if len(a.installs) > 0 || len(drafts) == 0 {
			ui.Column(c).Gap(14).Children(func() {
				kit.SectionHeader(c, T("library.tab.myApps"), countLabel(len(apps)))
				if len(apps) == 0 && a.lib.query == "" {
					a.noApps(c)
					return
				}
				a.collection(c, func(grid bool) {
					for i := range apps {
						a.appItem(c.Key(apps[i].ID), apps[i], grid, i)
					}
				})
			})
			shown += len(apps)
		}
		if len(drafts) > 0 {
			ui.Column(c).Gap(14).Children(func() {
				kit.SectionHeader(c, T("library.tab.projects"), countLabel(len(drafts)))
				a.collection(c, func(grid bool) {
					for i, pr := range drafts {
						a.projectItem(c.Key(pr.ID), pr, grid, i)
					}
				})
			})
			shown += len(drafts)
		}
		if shown == 0 {
			kit.EmptyState(c, icons.Search, T("library.noMatches", "query", a.lib.query), "", nil)
		}
		if len(a.installs) > 0 {
			a.libraryFooter(c)
		}
	})
}

func countLabel(n int) string {
	if n == 0 {
		return ""
	}
	return T("library.count", "count", n)
}

func (a *App) libraryFooter(c *ui.Context) {
	p := design.Of(c)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(4, 4, 0, 4).Children(func() {
		kit.Glyph(c, icons.Shield, 13, p.TextTertiary)
		kit.Footnote(c, T("myapps.footer")).MinWidth(0).Shrink(1)
		ui.Spacer(c)
		if kit.Button(c.Key("imp2"), T("menu.importApp"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM, Icon: icons.Import}).Clicked() {
			a.importExistingApp()
		}
	})
}

func (a *App) noApps(c *ui.Context) {
	p := design.Of(c)
	ui.Row(c).Padding(18, 20).Gap(14).Radius(design.RadiusPanel).Background(p.Group).Border(0.5, p.GroupBorder).AlignItems(ui.Center).Children(func() {
		kit.Glyph(c, icons.Inbox, 26, p.TextTertiary)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			kit.Headline(c, T("myapps.empty.title"))
			kit.Caption(c, T("myapps.empty.body"))
		})
		if kit.Button(c.Key("imp"), T("menu.importApp"), kit.ButtonOpts{Icon: icons.Import}).Clicked() {
			a.importExistingApp()
		}
	})
}

// collection lays items out as a responsive icon grid or a Finder-style
// list.
func (a *App) collection(c *ui.Context, items func(grid bool)) {
	p := design.Of(c)
	if a.lib.view == 1 {
		ui.Column(c).Radius(design.RadiusCard).Background(p.Group).Border(0.5, p.GroupBorder).Clip().Children(func() {
			kit.TableHeader(c, []kit.TableColumn{
				{Title: T("list.col.name")}, {Title: T("list.col.status"), Width: listColSt},
				{Title: T("list.col.version"), Width: listColV}, {Title: T("list.col.modified"), Width: listColM}, {Width: listColA},
			}, listPadX)
			ui.Column(c).Children(func() { items(false) })
		})
		return
	}
	w, _ := c.Size()
	avail := min(design.ContentMaxW, w-design.SidebarWidth-64)
	cols := max(2, int((avail+tileGap)/(tileMinW+tileGap)))
	tracks := make([]ui.Track, cols)
	for i := range tracks {
		tracks[i] = ui.Fr(1)
	}
	ui.Grid(c).ColumnTracks(tracks...).GapX(tileGap).GapY(18).Margin(0, -8).Children(func() { items(true) })
}

// tileData is what a library tile or row shows.
type tileData struct {
	icon     []byte
	name     string
	domain   string
	status   string
	tone     kit.Tone
	badge    kit.BadgeKind
	busy     bool
	progress float64
	version  string
	modified string
	menu     func(m *ui.Menu)
	idx      int
}

func (a *App) tile(c *ui.Context, d tileData) (open bool) {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(d.name).Column().AlignItems(ui.Center).Justify(ui.Start).Padding(12, 10, 14, 10).Gap(0).
		Radius(design.RadiusPanel).Cursor(ui.CursorPointer).Description(d.status)
	kit.Arrive(b, d.idx, design.StaggerGrid)
	hov := b.Hovered()
	if b.Focused() {
		b.Background(p.AccentSoft)
	}
	if d.menu != nil {
		b.ContextMenu(d.menu)
	}
	b.Children(func() {
		// The shelf the icon stands on: a ledge running through the row.
		ui.Box(c).Absolute().Left(-tileGap/2-1).Right(-tileGap/2-1).Top(12+8+tileIcon-3).Height(9).PassThrough().AccessHidden(true).
			LinearGradient(ui.LinearGradient{From: p.ShelfTop, To: p.ShelfBottom, Angle: 180}).
			Shadow(0, 6, 10, -4, p.ShadowDeep).InsetShadow(0, 1, 0, 0, p.Highlight)
		holder := ui.Box(c).Size(tileIcon+16, tileIcon+16).Center().Shrink(0)
		pose := ui.Pose{}
		if hov {
			pose = ui.Pose{Y: -6, Scale: 1.05, Rotate: -1.5}
		}
		if b.Pressed() {
			pose = ui.Pose{Scale: 0.97}
		}
		holder.AnimatePose(pose, design.Spring)
		holder.Children(func() {
			if d.busy {
				kit.ProgressRing(c, tileIcon+14, 3, d.progress, p.Accent, p.Fill).Center().Children(func() {
					kit.AppIconOpts(c, d.icon, d.name, tileIcon-10, kit.IconOpts{Shadow: true})
				})
				return
			}
			ui.Box(c).Size(tileIcon, tileIcon).Children(func() {
				kit.AppIconOpts(c, d.icon, d.name, tileIcon, kit.IconOpts{Shadow: true})
				kit.IconBadge(c, d.badge, tileIcon)
			})
		})
		ui.Text(c, d.name).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine().MaxWidthPercent(100).
			TextAlign(ui.Center).Margin(6, 0, 0, 0)
		ui.Text(c, d.domain).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine().MaxWidthPercent(100).TextAlign(ui.Center).Margin(2, 0, 0, 0)
		if d.status != "" {
			fg, _ := kit.ToneColors(p, d.tone)
			if d.tone == kit.ToneNeutral {
				fg = p.TextTertiary
			}
			ui.Text(c, d.status).FontSize(design.FontCaption).FontWeight(design.WeightMedium).TextColor(fg).SingleLine().
				MaxWidthPercent(100).TextAlign(ui.Center).Margin(3, 0, 0, 0)
		}
		if d.menu != nil {
			more := ui.ButtonBase(c.Key("more")).Label(T("common.more")).Absolute().Top(8).Right(8).Size(24, 24).Radius(12).Center().
				Background(p.Raised).Border(0.5, p.BorderStrong).Cursor(ui.CursorPointer).Menu(d.menu)
			more.Children(func() { kit.Glyph(c, icons.More, 12, p.Text) })
			if !hov && !more.Focused() {
				more.Opacity(0)
			}
		}
	})
	return b.Clicked()
}

func (a *App) row(c *ui.Context, d tileData, i int) (open bool) {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(d.name).Row().Justify(ui.Start).Height(listRowH).PaddingX(listPadX).Gap(12).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Description(d.status)
	kit.TableRowBG(c, b, i, b.Hovered(), b.Focused())
	kit.Arrive(b, i, design.StaggerList)
	if d.menu != nil {
		b.ContextMenu(d.menu)
	}
	b.Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(listIcon, listIcon).Shrink(0).Children(func() {
				kit.AppIconOpts(c, d.icon, d.name, listIcon, kit.IconOpts{})
			})
			ui.Column(c).MinWidth(0).Gap(0).Children(func() {
				ui.Text(c, d.name).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine()
				ui.Text(c, d.domain).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine()
			})
		})
		ui.Row(c).Width(listColSt).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
			if d.busy {
				kit.ProgressRing(c, 14, 2, d.progress, p.Accent, p.Fill)
				ui.Text(c, d.status).FontSize(design.FontSmall).TextColor(p.AccentText).SingleLine()
				return
			}
			kit.StatusDot(c, d.status, d.tone)
		})
		ui.Text(c, d.version).FontSize(design.FontSmall).TextColor(p.TextSecondary).Width(listColV).Shrink(0).SingleLine().FontFeatures("tnum")
		ui.Text(c, d.modified).FontSize(design.FontSmall).TextColor(p.TextSecondary).Width(listColM).Shrink(0).SingleLine()
		ui.Row(c).Width(listColA).Shrink(0).Justify(ui.End).Children(func() {
			if d.menu != nil {
				kit.IconButton(c.Key("more"), icons.More, T("common.more")).Size(26, 26).Menu(d.menu)
			}
		})
	})
	return b.Clicked()
}

func (a *App) emptyLibrary(c *ui.Context) {
	p := design.Of(c)
	ui.Scroll(c).Grow(1).Basis(0).Children(func() {
		ui.Column(c).FillWidth().MinHeightPercent(100).Center().Padding(design.TitleBarH+24, 40, 48, 40).Children(func() {
			ui.Column(c).Width(600).MaxWidthPercent(100).Gap(0).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).AlignItems(ui.Center).Gap(10).Children(func() {
					ui.Box(c).Children(func() {
						ui.Box(c).Absolute().Left(-160).Top(-90).Right(-160).Bottom(-90).PassThrough().AccessHidden(true).
							Draw(func(pt *ui.Painter, r ui.Rect) { design.PaintDots(pt, r, p, true) })
						logo := kit.BrandMark(c, 128)
						entered := ui.Local(logo, "in", func() bool { return false })
						pose := ui.Pose{}
						if !*entered {
							pose, *entered = ui.Pose{Y: 10, Scale: 0.9, Fade: 1}, true
							c.Invalidate()
						}
						logo.AnimatePose(pose, design.SpringPop)
					}).Margin(0, 0, 18, 0)
					kit.LargeTitle(c, T("library.empty.title")).TextAlign(ui.Center)
					ui.Text(c, T("library.empty.body")).FontSize(design.FontTitle3).TextColor(p.TextSecondary).TextAlign(ui.Center)
				})
				s := &a.create
				ui.Box(c).Height(28)
				r := kit.URLField(c.Key("hero"), &s.input, kit.URLFieldOpts{
					Placeholder: T("create.url.placeholder"), Label: T("create.url.label"), AutoFocus: true,
					Action: T("library.empty.cta"), ActionOff: strings.TrimSpace(s.input) == "", Paste: T("create.paste"),
					OnPaste: func(text string) bool {
						if t := strings.TrimSpace(text); t != "" {
							s.setInput(t)
							a.nav(secCreate)
							return true
						}
						return false
					},
				})
				if r.Paste {
					a.pasteIntoCreate(c)
				}
				if len(r.Dropped) > 0 {
					s.setInput(r.Dropped[0])
					a.nav(secCreate)
				}
				if (r.Action || r.Submitted) && strings.TrimSpace(s.input) != "" {
					s.setInput(s.input)
					a.nav(secCreate)
				}
				ui.Row(c).Justify(ui.Center).Gap(6).Margin(12, 0, 0, 0).Children(func() {
					kit.Footnote(c, T("create.drop.hint"))
					if kit.Button(c.Key("choose"), T("create.chooseFile"), kit.ButtonOpts{Variant: kit.Plain, Size: 18}).FontSize(design.FontSmall).Clicked() {
						a.nav(secCreate)
						a.chooseLocalSource()
					}
				})
				ui.Column(c).Margin(36, 0, 0, 0).Children(func() { a.createSuggestions(c) })
			})
		})
	})
}

// pasteIntoCreate reads an http(s) URL from the clipboard into Create.
func (a *App) pasteIntoCreate(c *ui.Context) {
	s := strings.TrimSpace(c.ReadClipboard())
	if s == "" {
		return
	}
	a.create.setInput(s)
	a.nav(secCreate)
}

func (a *App) filter(name, domain string) bool {
	q := strings.ToLower(strings.TrimSpace(a.lib.query))
	return q == "" || strings.Contains(strings.ToLower(name), q) || strings.Contains(strings.ToLower(domain), q)
}

func (a *App) installDomain(in *domain.Installation) string {
	dom := in.SourceURL
	if pr := a.project(in.ProjectID); pr != nil {
		return domainOf(pr.Config.App.Source)
	}
	if dom != "" {
		dom = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(dom, "https://"), "http://"), "/")
		dom = strings.TrimPrefix(dom, "www.")
	}
	return dom
}

func (a *App) projectItem(c *ui.Context, pr *domain.Project, grid bool, i int) {
	st, tone, busy := a.projectStatus(pr)
	meta := ago(pr.UpdatedAt)
	if pr.LastBuild != nil {
		meta = ago(*pr.LastBuild)
	}
	prc := *pr
	d := tileData{icon: pr.IconPNG, name: pr.Config.App.Name, domain: domainOf(pr.Config.App.Source), status: st, tone: tone, busy: busy,
		version: pr.Config.App.Version, modified: meta, menu: func(m *ui.Menu) { a.projectMenu(m, &prc) }, idx: i}
	if busy {
		d.progress = -1
		if j := a.latestJob(pr.ID); j != nil {
			d.progress = j.Progress
		}
	}
	if tone == kit.ToneDanger {
		d.badge = kit.BadgeFailed
	}
	var open bool
	if grid {
		open = a.tile(c, d)
	} else {
		open = a.row(c, d, i)
	}
	if open {
		a.openProject(pr.ID, 0)
	}
}

func (a *App) installStatus(in *domain.Installation) (string, kit.Tone) {
	switch {
	case in.State == domain.InstallExternal:
		return T("status.readOnly"), kit.ToneNeutral
	case in.State == domain.InstallMissing:
		return T("status.missing"), kit.ToneWarning
	case in.State == domain.InstallAttention:
		return T("status.attention"), kit.ToneDanger
	case in.UpdateReason != "":
		return T("status.update"), kit.ToneAccent
	}
	return T("status.installed"), kit.ToneSuccess
}

func (a *App) appItem(c *ui.Context, in domain.Installation, grid bool, i int) {
	st, tone := a.installStatus(&in)
	d := tileData{icon: in.IconPNG, name: in.Name, domain: a.installDomain(&in), status: st, tone: tone,
		version: in.Version, modified: ago(in.LastManagedAt), menu: func(m *ui.Menu) { a.appMenu(m, in) }, idx: i}
	switch {
	case in.State == domain.InstallExternal:
		d.badge = kit.BadgeLocked
	case in.State == domain.InstallMissing:
		d.badge = kit.BadgeWarning
	case in.State == domain.InstallAttention:
		d.badge = kit.BadgeFailed
	case in.UpdateReason != "":
		d.badge = kit.BadgeUpdate
	}
	if in.ProjectID != "" {
		if j := a.latestJob(in.ProjectID); j != nil && (j.Status == domain.JobRunning || j.Status == domain.JobQueued) {
			d.busy, d.progress, d.status, d.tone = true, j.Progress, stageLabel(*j), kit.ToneAccent
		}
	}
	if grid && tone == kit.ToneSuccess && !d.busy {
		d.status = "" // installed and healthy: the tile stays quiet
	}
	var open bool
	if grid {
		open = a.tile(c, d)
	} else {
		open = a.row(c, d, i)
	}
	if open {
		a.dlg.details = detailsDialog{open: true, id: in.ID}
	}
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
	if in.State == domain.InstallMissing && m.Item(T("locate.remove")).Chosen() {
		a.forgetApp(in.ID, in.Name)
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
