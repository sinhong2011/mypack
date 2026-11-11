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
	c.Root().Background(p.Chrome)
	ui.Box(c).Fill().Children(func() {
		// Film grain over the chrome.
		ui.Box(c).Absolute().Fill().PassThrough().AccessHidden(true).Draw(func(pt *ui.Painter, r ui.Rect) {
			design.PaintGrain(pt, r, p)
		})
		ui.Row(c).Absolute().Fill().AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c)
			in := design.CanvasInset
			canvas := ui.Box(c).Grow(1).Basis(0).MinWidth(0).Margin(in, in, in, 0).Radius(design.RadiusCanvas).
				Background(p.Window).Border(0.5, p.GroupBorder).Clip()
			design.Elevate(c, canvas, design.Floating)
			canvas.Children(func() {
				a.content(c)
				// Content scrolls under the toolbar, fading into the canvas.
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(design.TitleBarH + 14).PassThrough().AccessHidden(true).
					LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
						{Color: p.Window, At: 0}, {Color: p.Window, At: 0.62}, {Color: p.Window.Alpha(0), At: 1}}})
				a.toolbar(c)
			})
		})
	})
	a.dialogs(c)
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
				ui.Text(c, "mypack").FontSize(design.FontTitle3).FontWeight(design.WeightBold).TextColor(p.Text).LetterSpacing(design.TrackTitle)
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
						AlignItems(ui.Center).Radius(design.RadiusControl).Cursor(ui.CursorPointer).Transition(design.Transition(c))
					kit.Arrive(row, i, design.StaggerList)
					if row.Hovered() && !active {
						row.Background(p.Hover)
					}
					row.Children(func() {
						if active {
							a.navPill(c)
						}
						kit.AppIconOpts(c, pr.IconPNG, pr.Config.App.Name, 18, kit.IconOpts{})
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

// navPill is the sidebar's selection: a raised pill that glides from the
// item chosen before to this one.
func (a *App) navPill(c *ui.Context) {
	p := design.Of(c)
	pill := ui.Box(c).Absolute().Fill().Radius(design.RadiusControl).ZIndex(-1).Background(p.Raised).Border(0.5, p.GroupBorder)
	design.Elevate(c, pill, design.Lifted)
	kit.Glide(pill, navGlide{})
}

func (a *App) navItem(c *ui.Context, s section, ic *ui.SVG, label, badge string) {
	p := design.Of(c)
	active := a.section == s
	row := ui.ButtonBase(c.Key(string(s))).Label(label).Row().Justify(ui.Start).Height(30).PaddingX(9).Gap(10).AlignItems(ui.Center).
		Radius(design.RadiusControl).Cursor(ui.CursorPointer).Transition(design.Transition(c)).Current(ui.CurrentPage)
	if !active {
		row.Current(ui.CurrentNone)
		if row.Hovered() {
			row.Background(p.Hover)
		}
	}
	row.Children(func() {
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
			kit.Pop(ui.Text(c.Key("badge"), badge).FontSize(design.FontCaption).FontWeight(design.WeightBold).TextColor(p.TextOnAccent).
				Padding(1, 6).Radius(8).Background(p.Accent).FontFeatures("tnum"))
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
	ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(design.TitleBarH).Padding(0, right, 0, 20).Gap(8).AlignItems(ui.Center).
		DragWindow().Role(ui.RoleToolbar).Label(T("toolbar.label")).Children(func() {
		a.toolbarTitle(c)
		ui.Spacer(c).MinWidth(8)
		a.activityIndicator(c)
		switch a.section {
		case secLibrary:
			a.libraryToolbar(c)
		case secProject:
			a.projectToolbar(c)
		}
		if a.section != secCreate {
			if kit.GlassIcon(c.Key("tb-build"), icons.Plus, T("toolbar.buildWebsite.tip"), "mod+n", true).Clicked() {
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
			if kit.GlassIcon(c.Key("back"), icons.ChevronLeft, T("nav.back"), "", false).Clicked() {
				a.nav(secLibrary)
			}
		}
		ui.Column(c).Gap(0).MinWidth(0).Padding(0, 4).Children(func() {
			ui.Text(c, title).FontSize(design.FontTitle3).FontWeight(design.WeightBold).TextColor(p.Text).SingleLine().Role(ui.RoleHeading).
				LetterSpacing(design.TrackTitle)
			if sub != "" {
				ui.Text(c, sub).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine()
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
	b := ui.ButtonBase(c.Key("activity")).Label(label).Row().Justify(ui.Start).Height(34).Padding(0, 14, 0, 10).Gap(8).AlignItems(ui.Center).
		Radius(17).Background(p.Raised).Border(0.5, p.BorderStrong).Cursor(ui.CursorPointer).Role(ui.RoleStatus)
	design.Elevate(c, b, design.Lifted)
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
	kit.PageSwap(ui.Column(c.Key(string(a.section) + a.projectID)).Absolute().Fill()).Children(func() {
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
	ui.Scroll(c).Grow(1).Basis(0).ScrollbarInsets(design.TitleBarH, 0, 0).Children(func() {
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
