package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
)

// Window chrome: the sidebar sits on the window's textured chrome
// surface; the content lives on a raised canvas inset from the window
// edges, with the toolbar (title and actions) along its top. The same on
// every platform; only the window controls differ (traffic lights at the
// left on macOS, caption buttons on the right elsewhere).

func (a *App) view(c *ui.Context) {
	design.Apply(c)
	p := design.Of(c)
	kit.Strings.Close = T("common.close")
	a.reduced = c.Preferences().ReduceMotion
	if a.iconAt == nil {
		a.iconAt = iconRects{}
	}
	c.Root().Background(p.Chrome)
	win := ui.Box(c).Fill()
	dragging := win.FileDragOver()
	if files := win.DroppedFiles(); len(files) > 0 {
		a.create.reset(a.host)
		a.create.setInput(files[0])
		a.nav(secCreate)
	}
	win.Children(func() {
		// The chrome: one flat surface.
		ui.Box(c).Absolute().Fill().PassThrough().AccessHidden(true).Background(p.Chrome)
		ui.Row(c).Absolute().Fill().AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c)
			in := design.CanvasInset
			canvas := ui.Box(c).Grow(1).Basis(0).MinWidth(0).Margin(in, in, in, 0).Radius(design.RadiusCanvas).
				Border(1, p.GroupBorder).Clip()
			design.Tint(canvas, p.CanvasTop, p.CanvasBot)
			design.Float(c, canvas)
			// Behind a sheet or the palette the canvas blurs in, and clears after.
			blur := float32(0)
			if kit.ModalOpen() {
				blur = 6
			}
			if b := canvas.AnimateTo("blur", blur, design.SpringSoft); b > 0.05 {
				canvas.Blur(b)
			}
			canvas.Children(func() {
				a.content(c)
				// Content scrolls under the toolbar, fading into the canvas.
				// Scrolled, it firms into a bar with a hairline under it.
				k := scrollCollapse()
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(design.TitleBarH+14*(1-k)).PassThrough().AccessHidden(true).
					LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
						{Color: p.CanvasTop, At: 0}, {Color: p.CanvasTop, At: 0.62 + 0.38*k}, {Color: design.Fade(p.CanvasTop, k*0.96), At: 1}}}).
					BorderWidth(0, 0, 0.5, 0).BorderColor(design.Fade(p.Divider, k))
				a.toolbar(c)
			})
		})
	})
	if dragging {
		a.dropOverlay(c)
	}
	a.flightsView(c)
	a.dialogs(c)
	a.paletteView(c)
	kit.Toasts(c)
	for _, t := range a.toasts {
		typ := ""
		switch t.kind {
		case 1:
			typ = "success"
		case 2:
			typ = "error"
		}
		if t.action != "" {
			c.ToastAction(t.title, t.action, t.fn)
		} else {
			c.AddToast(ui.Toast{Title: t.title, Description: t.desc, Type: typ})
		}
	}
	a.toasts = nil
	if c.Shortcut(0, ui.KeyEscape) {
		a.escape()
	}
}

func (a *App) escape() {
	switch a.section {
	case secCreate:
		if a.create.busy {
			return
		}
		a.nav(secLibrary)
	case secProject:
		a.nav(secLibrary)
	}
}

// ---- sidebar --------------------------------------------------------------------

// navGlide pairs the sidebar's selection pill across items, so it glides.
type navGlide struct{}

func (a *App) sidebar(c *ui.Context) {
	p := design.Of(c)
	bar := c.TitleBar()
	ui.Column(c).Width(design.SidebarWidth).Shrink(0).Padding(0, 10, 12, 10).Children(func() {
		// The window controls' strip (traffic lights on macOS), with the
		// brand where there are none.
		ui.Row(c).Height(design.TitleBarH).Shrink(0).DragWindow().AlignItems(ui.Center).Padding(0, 0, 0, max(6, bar.Left)).Gap(8).Children(func() {
			if bar.Left == 0 {
				kit.BrandMark(c, 22)
				ui.Text(c, "Mypack").FontSize(design.FontTitle3).FontWeight(design.WeightBold).TextColor(p.Text).LetterSpacing(design.TrackTitle)
			}
		})
		ui.Column(c).Gap(1).Role(ui.RoleNavigation).Label(T("nav.label")).Children(func() {
			updates := 0
			for _, in := range a.installs {
				if in.UpdateReason != "" {
					updates++
				}
			}
			badge := ""
			if updates > 0 {
				badge = fmt.Sprint(updates)
			}
			a.navItem(c, secLibrary, icons.Library, T("nav.library"), badge)
			a.navItem(c, secDiscover, icons.Discover, T("nav.discover"), "")
			running := ""
			if n := len(a.activeJobs()); n > 0 {
				running = "•"
			}
			a.navItem(c, secBuilds, icons.Builds, T("nav.builds"), running)
		})
		// Recent projects.
		if len(a.projects) > 0 {
			kit.Eyebrow(c, T("nav.recent")).Padding(20, 10, 8, 10)
			ui.Column(c).Gap(1).Children(func() {
				for i, pr := range a.projects {
					if i >= 6 {
						break
					}
					pr := pr
					active := a.section == secProject && a.projectID == pr.ID
					row := ui.ButtonBase(c.Key(pr.ID)).Label(pr.Config.App.Name).Row().Justify(ui.Start).Height(30).PaddingX(8).Gap(9).
						AlignItems(ui.Center).Cursor(ui.CursorPointer)
					kit.NavRow(c, row, active)
					kbd := row.FocusVisible()
					kit.Arrive(row, i, design.StaggerList)
					row.Children(func() {
						kit.FocusMark(c, kbd, 10)
						if active {
							a.navPill(c)
						}
						ic := kit.AppIconOpts(c, pr.IconPNG, pr.Config.App.Name, 18, kit.IconOpts{})
						if row.Clicked() {
							a.flyFrom(iconKey(pr.ID), ic.Bounds(), pr.IconPNG, pr.Config.App.Name)
						}
						fg := p.TextSecondary
						if active {
							fg = p.Text
						}
						ui.Text(c, pr.Config.App.Name).FontSize(design.FontBody).TextColor(fg).SingleLine().MinWidth(0).Grow(1)
						if j := a.latestJob(pr.ID); j != nil && (j.Status == domain.JobRunning || j.Status == domain.JobQueued) {
							kit.Spinner(c, 12, p.Accent)
						}
					})
					if row.Clicked() {
						a.openProject(pr.ID, 0)
					}
				}
			})
		}
		ui.Spacer(c)
		ui.Column(c).Gap(1).Children(func() {
			a.navItem(c, secRuntimes, icons.Runtimes, T("nav.runtimes"), "")
			a.navItem(c, secSettings, icons.Settings, T("nav.settings"), "")
		})
	})
}

// navPill is the sidebar's selection (kit.NavPill), gliding between items.
func (a *App) navPill(c *ui.Context) { kit.NavPill(c, navGlide{}) }

func (a *App) navItem(c *ui.Context, s section, ic *ui.SVG, label, badge string) {
	p := design.Of(c)
	active := a.section == s
	row := ui.ButtonBase(c.Key(string(s))).Label(label).Row().Justify(ui.Start).Height(30).PaddingX(9).Gap(10).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Current(ui.CurrentPage)
	kit.NavRow(c, row, active)
	kbd := row.FocusVisible()
	if !active {
		row.Current(ui.CurrentNone)
	}
	row.Children(func() {
		kit.FocusMark(c, kbd, 10)
		if active {
			a.navPill(c)
		}
		col, tc := p.TextSecondary, p.TextSecondary
		if active {
			col, tc = p.Accent, p.Text
		}
		kit.Glyph(c, ic, 16, col)
		w := design.WeightMedium
		if active {
			w = design.WeightSemibold
		}
		ui.Text(c, label).FontSize(design.FontBody).FontWeight(w).TextColor(tc).SingleLine().Grow(1).MinWidth(0)
		switch badge {
		case "":
		case "•":
			kit.Spinner(c, 12, p.Accent)
		default:
			kit.NavCount(c, badge, s == secLibrary) // Library counts updates
		}
	})
	if row.Clicked() {
		a.nav(s)
	}
}

// ---- toolbar --------------------------------------------------------------------

func (a *App) toolbar(c *ui.Context) {
	bar := c.TitleBar()
	right := max(bar.Right-design.CanvasInset, 0) + 14
	row := ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(design.TitleBarH).Padding(0, right, 0, 20).Gap(8).AlignItems(ui.Center).
		DragWindow().Role(ui.RoleToolbar).Label(T("toolbar.label"))
	a.toolbarW = row.Bounds().W
	row.Children(func() {
		a.toolbarTitle(c)
		ui.Spacer(c).MinWidth(8)
		a.activityIndicator(c)
		switch a.section {
		case secLibrary:
			a.libraryToolbar(c)
		case secProject:
			a.projectToolbar(c)
		}
		switch a.section {
		case secLibrary, secProject, secCreate, secGallery:
		default:
			// Elsewhere the search pill opens the palette.
			kit.ToolbarSearch(c.Key("search"), kit.ToolbarSearchOpts{Placeholder: T("library.search"), Hotkey: "mod+k", OnPalette: a.togglePalette})
		}
		switch {
		case a.section == secCreate && a.create.building == "":
		case a.section == secProject || a.section == secCreate:
			// The project's own Build is the primary here.
			if kit.ToolbarIcon(c.Key("tb-build"), icons.Plus, T("menu.newApp")).Clicked() {
				a.newApp()
			}
		default:
			if kit.NewPill(c.Key("tb-build"), T("menu.newApp"), "mod+n", a.toolbarW > 0 && a.toolbarW < 700).Clicked() {
				a.newApp()
			}
		}
	})
}

func (a *App) toolbarTitle(c *ui.Context) {
	p := design.Of(c)
	title, sub := "", ""
	switch a.section {
	case secLibrary:
		title = T("nav.library")
		if n := len(a.installs) + a.draftCount(); a.loaded && n > 0 {
			sub = T("library.subtitle", "count", n)
		}
	case secDiscover:
		title = T("nav.discover")
	case secBuilds:
		title = T("nav.builds")
		sub = T("builds.count", "count", len(a.jobs))
	case secRuntimes:
		title = T("nav.runtimes")
	case secSettings:
		title = T("nav.settings")
	case secCreate:
		title = T("create.toolbar")
	case secProject:
		if pr := a.project(a.projectID); pr != nil {
			title = pr.Config.App.Name
			sub = domainOf(pr.Config.App.Source)
		}
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).MinWidth(0).Shrink(1).Children(func() {
		if a.section == secProject || a.section == secCreate {
			if kit.ToolbarIcon(c.Key("back"), icons.ChevronLeft, T("nav.back")).Clicked() {
				a.nav(secLibrary)
			}
		}
		ui.Column(c).Gap(0).MinWidth(0).Padding(0, 4).Children(func() {
			ui.Text(c, title).FontSize(design.FontTitle3).FontWeight(design.WeightBold).TextColor(p.Text).SingleLine().Role(ui.RoleHeading).
				LetterSpacing(design.TrackTitle)
			if sub != "" {
				// The subtitle folds away as the page scrolls under the bar.
				k := scrollCollapse()
				ui.Box(c).Height(15 * (1 - k)).Clip().Opacity(1 - k).Children(func() {
					ui.Text(c, sub).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine()
				})
			}
		})
	})
}

// activityIndicator shows only while work is running (spec §9, §12.2).
func (a *App) activityIndicator(c *ui.Context) {
	p := design.Of(c)
	active := a.activeJobs()
	if len(active) == 0 {
		return
	}
	j := active[0]
	if a.section == secProject && a.projectID == j.ProjectID && len(active) == 1 {
		return
	}
	label := j.Snapshot.App.Name + " · " + stageLabel(j)
	if len(active) > 1 {
		label = T("activity.many", "count", len(active))
	}
	b := ui.ButtonBase(c.Key("activity")).Label(label).Row().Justify(ui.Start).Height(design.ToolbarControl).Padding(0, 14, 0, 10).Gap(8).AlignItems(ui.Center).
		Radius(kit.PillRadius(design.ToolbarControl)).Background(p.Raised).Border(1, p.BorderStrong).Cursor(ui.CursorPointer).Role(ui.RoleStatus)
	kit.Pop(b)
	b.Children(func() {
		if j.Status == domain.JobAwaitingApproval {
			kit.Glyph(c, icons.Download, 15, p.Warning)
		} else if j.Progress >= 0 {
			kit.ProgressRing(c, 16, 2.5, j.Progress, p.Accent, p.Fill)
		} else {
			kit.Spinner(c, 15, p.Accent)
		}
		ui.Text(c, label).FontSize(design.FontSmall).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine().MaxWidth(240)
	})
	if b.Clicked() {
		if len(active) > 1 {
			a.nav(secBuilds)
		} else {
			a.openProject(j.ProjectID, 0)
		}
	}
}

// ---- content --------------------------------------------------------------------

func (a *App) content(c *ui.Context) {
	dir := a.navDir
	if dir == 0 {
		dir = 1
	}
	ui.Box(c.Key("pages")).Absolute().Fill().Children(func() { a.pageView(c, dir) })
}

func (a *App) pageView(c *ui.Context, dir float32) {
	kit.PageSwap(ui.Column(c.Key(string(a.section)+a.projectID)).Absolute().Fill(), dir).Children(func() {
		switch a.section {
		case secLibrary:
			a.libraryView(c)
		case secDiscover:
			a.discoverView(c)
		case secBuilds:
			a.buildsView(c)
		case secRuntimes:
			a.runtimesView(c)
		case secSettings:
			a.settingsView(c)
		case secCreate:
			a.createView(c)
		case secProject:
			a.projectView(c)
		case secGallery:
			a.galleryView(c)
		}
	})
}

// page is the standard scrolling content column; it starts under the
// toolbar and scrolls beneath it.
func page(c *ui.Context, maxW float32, body func()) {
	ui.Scroll(c).Grow(1).Basis(0).ScrollbarInsets(design.TitleBarH, 0, 0).TrackScroll(&pageScroll).Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(design.TitleBarH+16, 32, 48, 32).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(maxW).Gap(24).Children(body)
		})
	})
}

// stageLabel describes a job's current state in a few words.
func stageLabel(j domain.BuildJob) string {
	switch j.Status {
	case domain.JobAwaitingApproval:
		return T("stage.awaitingApproval")
	case domain.JobQueued:
		return T("stage.queued")
	case domain.JobSucceeded:
		return T("status.built")
	}
	if j.Stage == domain.StagePreparing && j.Status == domain.JobRunning {
		return T("step.prepare.active")
	}
	switch j.Status {
	case domain.JobFailed:
		return T("status.failed")
	case domain.JobCancelled:
		return T("status.cancelled")
	}
	s := T(visibleStepLabel(j.Stage))
	if j.Progress >= 0 && j.Stage == domain.StageDownloading {
		s += fmt.Sprintf(" %d%%", int(j.Progress*100))
	}
	return s
}

// dropOverlay is the whole-window drop state while a file is dragged over
// mypack: drop it anywhere to make an app of it.
func (a *App) dropOverlay(c *ui.Context) {
	p := design.Of(c)
	fade := design.Tween(design.MotionFast, design.EaseStd)
	ui.Column(c.Key("drop")).Absolute().Fill().Center().Gap(10).PassThrough().Background(design.Fade(p.Window, 0.86)).
		Transition(ui.ElementTransition{Animation: &fade, Enter: &ui.Motion{}, Exit: &ui.Motion{}}).Children(func() {
		ui.Box(c).Absolute().Top(14).Left(14).Right(14).Bottom(14).PassThrough().AccessHidden(true).Draw(func(pt *ui.Painter, r ui.Rect) {
			pt.StrokeDashed(r, p.Accent, design.RadiusCanvas+4, 2)
		})
		kit.Pop(ui.Row(c.Key("dropicon")).Size(64, 64).Radius(18).Center().Background(p.AccentSoft)).Children(func() {
			kit.Glyph(c, icons.Import, 30, p.Accent)
		})
		ui.Text(c, T("drop.title")).FontSize(design.FontDisplay).FontWeight(design.WeightBold).TextColor(p.Text).LetterSpacing(design.TrackDisplay)
		kit.Secondary(c, T("drop.body"))
	})
}
