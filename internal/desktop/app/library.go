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
	iconSize    float32
	query       string
	view        int // 0 grid, 1 list
	viewInit    bool
	focusSearch bool
	searchH     ui.Handle
}

const (
	tileMinW  float32 = 150
	tileGap   float32 = 8
	listIcon  float32 = 28
	listRowH  float32 = 44
	listPadX  float32 = 14
	listColSt float32 = 150
	listColV  float32 = 96
	listColM  float32 = 128
	listColA  float32 = 132
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
	l := &a.lib
	if !a.loaded || len(a.projects)+len(a.installs) == 0 {
		return
	}
	ov := l.view
	kit.PillSegmented(c.Key("view"), &l.view, []kit.PillSegment{{Icon: icons.Grid, Label: T("library.view.grid")}, {Icon: icons.List, Label: T("library.view.list")}},
		T("library.view.label"), viewGlide{}, false)
	if ov != l.view {
		a.settings.LibraryView = []string{"grid", "list"}[l.view]
		st := a.settings
		go func() { _ = a.svc.SaveSettings(bgctx(), st) }()
	}
	kit.ToolbarSearch(c.Key("search"), kit.ToolbarSearchOpts{Placeholder: T("library.search"), Hotkey: "mod+k", Query: &l.query, Handle: &l.searchH,
		OnPalette: a.togglePalette})
	if l.focusSearch {
		l.focusSearch = false
		l.searchH.Focus()
	}
}

func (a *App) libraryView(c *ui.Context) {
	l := &a.lib
	// Few apps get bigger icons, so the shelves never look empty.
	l.iconSize = 76
	if len(a.installs)+a.draftCount() <= 8 {
		l.iconSize = 88
	}
	if !l.viewInit && a.loaded {
		l.viewInit = true
		if a.settings.LibraryView == "list" {
			l.view = 1
		}
	}
	if !a.loaded {
		page(c, func() {
			ui.Row(c).Gap(tileGap).Margin(36, 0, 0, 0).Children(func() {
				for i := 0; i < 5; i++ {
					ui.Column(c.Key(i)).Grow(1).Basis(0).AlignItems(ui.Center).Gap(10).Padding(14, 8).Children(func() {
						kit.Shimmer(c, 88, 88, design.AppIconRadius(88))
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
	page(c, func() {
		shown := 0
		first := true
		// The first section's header carries Import Existing App…, at its
		// right, level with the title.
		header := func(title, count string) {
			ui.Row(c).AlignItems(ui.Center).Gap(8).MinHeight(32).Children(func() {
				kit.SectionHeader(c, title, count).Grow(1)
				if first {
					if len(a.groupCandidates()) >= 2 && kit.Button(c.Key("grp2"), T("menu.newGroup"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM, Icon: icons.Layers}).Clicked() {
						a.newGroup()
					}
					if kit.Button(c.Key("imp2"), T("menu.importApp"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM, Icon: icons.Import}).Clicked() {
						a.importExistingApp()
					}
				}
			})
			first = false
		}
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
				header(T("library.tab.myApps"), countLabel(len(apps)))
				if len(apps) == 0 && a.lib.query == "" {
					a.noApps(c)
					return
				}
				a.collection(c, len(apps), true, func(i int, grid bool) {
					a.appItem(c.Key(apps[i].ID), apps[i], grid, i)
				})
			})
			shown += len(apps)
		}
		if len(drafts) > 0 {
			ui.Column(c).Gap(14).Children(func() {
				header(T("library.tab.projects"), countLabel(len(drafts)))
				a.collection(c, len(drafts), len(a.installs) == 0, func(i int, grid bool) {
					a.projectItem(c.Key(drafts[i].ID), drafts[i], grid, i)
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

// collection lays n items out as rows of icons standing on shelves (or a
// Finder-style list). With ghost, empty places in the last row invite
// another website.
func (a *App) collection(c *ui.Context, n int, ghost bool, item func(i int, grid bool)) {
	if a.lib.view == 1 {
		design.Well(c, ui.Column(c).Radius(design.RadiusCard).Clip()).Children(func() {
			kit.TableHeader(c, []kit.TableColumn{
				{Title: T("list.col.name")}, {Title: T("list.col.status"), Width: listColSt},
				{Title: T("list.col.version"), Width: listColV}, {Title: T("list.col.modified"), Width: listColM}, {Width: listColA},
			}, listPadX)
			ui.Column(c).Children(func() {
				for i := 0; i < n; i++ {
					item(i, false)
				}
			})
		})
		return
	}
	size := a.lib.iconSize
	w, _ := c.Size()
	avail := w - design.SidebarWidth - 64
	minW := size + 56
	cols := max(2, int((avail+tileGap)/(minW+tileGap)))
	rows := (n + cols - 1) / cols
	if rows == 0 {
		rows = 1
	}
	ui.Column(c).Gap(10).Margin(0, -6).Children(func() {
		for r := 0; r < rows; r++ {
			ui.Box(c.Key(r)).Children(func() {

				ui.Row(c).Gap(tileGap).AlignItems(ui.Start).Children(func() {
					for k := 0; k < cols; k++ {
						i := r*cols + k
						cell := ui.Column(c.Key(k)).Grow(1).Basis(0).MinWidth(0)
						cell.Children(func() {
							switch {
							case i < n:
								item(i, true)
							case ghost && i == n:
								a.ghostTile(c, size)
							}
						})
					}
				})
			})
		}
	})
}

// ghostTile is a dashed placeholder at the end of the grid that adds a website.
func (a *App) ghostTile(c *ui.Context, size float32) {
	p := design.Of(c)
	b := ui.ButtonBase(c.Key("ghost")).Label(T("library.ghost")).Column().AlignItems(ui.Center).Padding(14, 6, 12, 6).Cursor(ui.CursorPointer)
	hov := b.Hovered()
	b.Children(func() {
		box := ui.Box(c).Size(size, size).Center().Radius(design.AppIconRadius(size))
		if hov {
			box.Background(p.AccentSoft)
		}
		col := design.Fade(p.TextTertiary, 0.55)
		if hov {
			col = p.AccentText
		}
		r := design.AppIconRadius(size)
		box.Draw(func(pt *ui.Painter, rr ui.Rect) {
			pt.StrokeDashed(rr, col, r, 1.5)
		})
		box.Children(func() { kit.Glyph(c, icons.Plus, size*0.26, col) })
		kit.Press(box, false, b.Pressed(), 0)
		ui.Text(c, T("library.ghost")).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(col).SingleLine().
			MaxWidthPercent(100).TextAlign(ui.Center).Margin(12, 0, 0, 0)
	})
	if b.Clicked() {
		a.newApp()
	}
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
	pid      string // the project, for the icon that flies to its page
	// The one-click action (Open the app, or Build a draft).
	action     string
	actionIcon *ui.SVG
	run        func()
}

// tile is an app floating on the canvas: its icon over a soft glow in its
// own colour, the name and one compact status line. Hover blooms the
// glow and shows its quick actions; keyboard focus
// rings it in the accent.
func (a *App) tile(c *ui.Context, d tileData) (open bool) {
	p := design.Of(c)
	size := a.lib.iconSize
	b := ui.ButtonBase(c).Label(d.name).Column().AlignItems(ui.Center).Justify(ui.Start).Padding(14, 6, 12, 6).Gap(0).
		Radius(design.RadiusPanel).Cursor(ui.CursorPointer).Description(d.status)
	kit.Arrive(b)
	b.FocusRing(false)
	hov, focused := b.Hovered(), b.FocusVisible()
	if d.menu != nil {
		b.ContextMenu(d.menu)
	}
	if d.run != nil && (b.DoubleClicked() || b.Shortcut(0, ui.KeyEnter)) {
		d.run()
	}
	b.Children(func() {
		holder := ui.Box(c).Size(size, size).Shrink(0)
		pose := ui.Pose{}
		if b.Pressed() {
			pose = ui.Pose{Scale: 0.97}
		}
		holder.AnimatePose(pose, design.Spring)
		holder.Children(func() {
			if d.busy {
				kit.ProgressRing(c, size+12, 3, d.progress, p.Accent, p.Fill).Absolute().Top(-6).Left(-6).Center().Children(func() {
					kit.AppIconOpts(c, d.icon, d.name, size-12, kit.IconOpts{Shadow: true})
				})
				return
			}
			kit.AppIconOpts(c, d.icon, d.name, size, kit.IconOpts{Shadow: true})
			if focused {
				// Keyboard focus: an underline under the icon, no ring.
				ui.Box(c.Key("focus")).Absolute().Left(size * 0.3).Right(size * 0.3).Bottom(-6).Height(1.5).Radius(0.75).
					Background(p.Accent).PassThrough().AccessHidden(true)
			}
			kit.IconBadge(c, d.badge, size)
			if d.run != nil && (hov || focused) {
				a.launchButton(c, d, size)
			}
			if d.menu != nil {
				// Built every frame, only shown on hover or focus: the menu
				// hands its chosen item back to this button in the frame
				// after, when the pointer is over the menu, not the tile.
				// A button built only on hover would drop every choice.
				more := ui.ButtonBase(c.Key("more")).Label(T("common.more")).Absolute().Top(-6).Left(-6).Size(24, 24).Radius(12).Center().
					Background(p.Overlay).Border(0.5, p.BorderStrong).Cursor(ui.CursorPointer).Menu(d.menu)
				if !hov && !focused && !more.Focused() {
					more.Opacity(0)
				}
				design.Elevate(c, more, design.Floating)
				more.Children(func() { kit.Glyph(c, icons.More, 13, p.Text) })
			}
		})
		nm := ui.Text(c, d.name).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine().MaxWidthPercent(100).
			TextAlign(ui.Center).Margin(12, 0, 0, 0)
		_ = nm
		// One compact line: a status dot and word when it matters, else
		// the site.
		ui.Row(c).Gap(5).AlignItems(ui.Center).Justify(ui.Center).MaxWidthPercent(100).Margin(2, 0, 0, 0).Children(func() {
			if d.status != "" {
				fg, _ := kit.ToneColors(p, d.tone)
				if d.tone == kit.ToneNeutral {
					fg = p.TextTertiary
				}
				ui.Box(c).Size(6, 6).Radius(3).Background(fg).Shrink(0)
				ui.Text(c, d.status).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine().MinWidth(0)
				return
			}
			ui.Text(c, d.domain).FontSize(design.FontCaption).TextColor(p.TextTertiary).SingleLine().MinWidth(0)
		})
	})
	if b.Clicked() {
		return true
	}
	return false
}

func (a *App) row(c *ui.Context, d tileData, i int) (open bool) {
	p := design.Of(c)
	b := ui.ButtonBase(c).Label(d.name).Row().Justify(ui.Start).Height(listRowH).PaddingX(listPadX).Gap(12).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Description(d.status)
	kit.TableRowBG(c, b, i, b.Hovered(), b.Focused())
	kit.Arrive(b)
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
		ui.Row(c).Width(listColA).Shrink(0).Justify(ui.End).Gap(4).AlignItems(ui.Center).Children(func() {
			if d.run != nil {
				if kit.Button(c.Key("open"), d.action, kit.ButtonOpts{Size: design.ControlSM, Icon: d.actionIcon}).Clicked() {
					d.run()
				}
			}
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
				// No hero: one plain line, then the field.
				ui.Column(c).Gap(4).Children(func() {
					kit.PageTitle(c, T("library.empty.title"))
					ui.Text(c, T("library.empty.body")).FontSize(design.FontBody).TextColor(p.TextSecondary)
				})
				s := &a.create
				ui.Box(c).Height(16)
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
				ui.Row(c).Justify(ui.Start).Gap(6).Margin(12, 0, 0, 0).Children(func() {
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
		version: pr.Config.App.Version, modified: meta, menu: func(m *ui.Menu) { a.projectMenu(m, &prc) }, idx: i, pid: pr.ID}
	if busy {
		d.progress = -1
		if j := a.latestJob(pr.ID); j != nil {
			d.progress = j.Progress
		}
	}
	if tone == kit.ToneDanger {
		d.badge = kit.BadgeFailed
	}
	if !busy {
		d.action, d.actionIcon, d.run = a.projectLaunch(pr)
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
	_ = d
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
		version: in.Version, modified: ago(in.LastManagedAt), menu: func(m *ui.Menu) { a.appMenu(m, in) }, idx: i, pid: in.ProjectID}
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
	if !d.busy {
		if in.State == domain.InstallMissing {
			id := in.ID
			d.action, d.actionIcon, d.run = T("action.locate"), icons.Folder, func() { a.dlg.details = detailsDialog{open: true, id: id} }
		} else {
			id := in.ID
			d.action, d.actionIcon, d.run = T("action.open"), icons.Play, func() { a.openInstall(id) }
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

// launchButton is the round one-click action on a hovered or focused tile.
func (a *App) launchButton(c *ui.Context, d tileData, size float32) {
	p := design.Of(c)
	s := max(30, size*0.3)
	b := ui.ButtonBase(c.Key("launch")).Label(d.action+" "+d.name).Absolute().Right(-s*0.25).Bottom(-s*0.2).Size(s, s).Radius(s / 2).Center().
		Cursor(ui.CursorPointer)
	base := p.Accent
	if b.Hovered() {
		base = p.AccentHover
	}
	b.Background(base).Border(2, p.CanvasTop)
	kit.Press(b, false, b.Pressed(), 0)
	b.Children(func() { kit.Glyph(c, d.actionIcon, s*0.44, p.TextOnAccent) })
	b.Tooltip(d.action)
	if b.Clicked() {
		d.run()
	}
}
