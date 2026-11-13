package app

import (
	"fmt"
	"strings"

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
	a.dialogs(c)
	a.paletteView(c)
	kit.Toasts(c)
	a.drainToasts(c)
	if a.setWin != nil && a.setWin.IsVisible() {
		a.setWin.Invalidate() // it shows the same state
	}
	if c.Shortcut(0, ui.KeyEscape) {
		a.escape()
	}
}

// drainToasts shows the queued toasts in the window drawing c.
func (a *App) drainToasts(c *ui.Context) {
	for _, t := range a.toasts {
		typ := ""
		switch t.kind {
		case 1:
			typ = "success"
		case 2:
			typ = "error"
		}
		if t.toast != nil {
			c.AddToast(*t.toast)
		} else if t.action != "" {
			c.ToastAction(t.title, t.action, t.fn)
		} else {
			c.AddToast(ui.Toast{Title: t.title, Description: t.desc, Type: typ})
		}
	}
	a.toasts = nil
}

func (a *App) escape() {
	switch a.section {
	case secCreate:
		if a.create.busy || a.create.building != "" {
			return // Esc cancels the build there (buildStatus)
		}
		a.nav(secLibrary)
	case secProject:
		a.nav(secLibrary)
	case secLibrary:
		// Esc clears the search, then the filter.
		if a.lib.query != "" {
			a.lib.query = ""
		} else {
			a.lib.filter = filterAll
		}
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
				ui.Text(c, "MyPack").FontSize(design.FontTitle3).FontWeight(design.WeightBold).TextColor(p.Text).LetterSpacing(design.TrackTitle)
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
			// Builds running: a spinner, and their count when more than one.
			running := ""
			if n := len(a.activeJobs()); n == 1 {
				running = "•"
			} else if n > 1 {
				running = "•" + fmt.Sprint(n)
			}
			a.navItem(c, secBuilds, icons.Builds, T("nav.builds"), running)
		})
		// Pinned apps, then recent projects (those not pinned).
		pinned := a.pinnedItems()
		if len(pinned) > 0 {
			a.sidebarPinned(c, pinned)
		}
		// Recent gives room to the pins: 6 rows, down to 3.
		var recent []domain.Project
		limit := max(3, 6-len(pinned))
		for _, pr := range a.projects {
			if len(recent) < limit && !a.ui.isPinned(pr.ID) {
				recent = append(recent, pr)
			}
		}
		if len(recent) > 0 {
			kit.Eyebrow(c, T("nav.recent")).Padding(20, 10, 8, 10)
			ui.Column(c).Gap(1).Children(func() {
				for _, pr := range recent {
					pr := pr
					active := a.section == secProject && a.projectID == pr.ID
					row := ui.ButtonBase(c.Key(pr.ID)).Label(pr.Config.App.Name).Row().Justify(ui.Start).Height(30).PaddingX(8).Gap(9).
						AlignItems(ui.Center).Cursor(ui.CursorPointer)
					kit.NavRow(c, row, active)
					kbd := row.FocusVisible()
					row.Children(func() {
						kit.FocusMark(c, kbd, 10)
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
							kit.BuildingSpinner(c, 12, p.Accent)
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
			if a.sideRow(c, "settings", icons.Settings, T("nav.settings"), a.updateBadge(), true, false, nil) {
				if a.updateBadge() != "" {
					a.openSettings(paneUpdates) // where the waiting update is
				} else {
					a.openSettings("")
				}
			}
		})
	})
}

// navPill is the sidebar's selection (kit.NavPill), gliding between items.
func (a *App) navPill(c *ui.Context) { kit.NavPill(c, navGlide{}) }

func (a *App) navItem(c *ui.Context, s section, ic *ui.SVG, label, badge string) {
	if a.sideRow(c, string(s), ic, label, badge, s == secLibrary, a.section == s, nil) {
		a.nav(s)
	}
}

// sideRow is one sidebar row, the selection pill gliding between rows that
// share glide (navGlide when nil); it reports a click. A badge of "•"
// (and a count after it) is a spinner; accent marks a count to act on.
func (a *App) sideRow(c *ui.Context, key string, ic *ui.SVG, label, badge string, accent, active bool, glide any) bool {
	p := design.Of(c)
	if glide == nil {
		glide = navGlide{}
	}
	row := ui.ButtonBase(c.Key(key)).Label(label).Row().Justify(ui.Start).Height(30).PaddingX(9).Gap(10).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Current(ui.CurrentPage)
	kit.NavRow(c, row, active)
	kbd := row.FocusVisible()
	if !active {
		row.Current(ui.CurrentNone)
	}
	row.Children(func() {
		kit.FocusMark(c, kbd, 10)
		if active {
			kit.NavPill(c, glide)
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
		switch {
		case badge == "":
		case strings.HasPrefix(badge, "•"):
			kit.BuildingSpinner(c, 12, p.Accent)
			if n := strings.TrimPrefix(badge, "•"); n != "" {
				kit.NavCount(c, n, false)
			}
		default:
			kit.NavCount(c, badge, accent)
		}
	})
	return row.Clicked()
}

// ---- toolbar --------------------------------------------------------------------

func (a *App) toolbar(c *ui.Context) {
	row := ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(design.TitleBarH).Padding(0, toolbarEnd(c), 0, 20).Gap(8).AlignItems(ui.Center).
		DragWindow().Role(ui.RoleToolbar).Label(T("toolbar.label"))
	a.toolbarW = row.Bounds().W
	row.Children(func() {
		a.toolbarTitle(c)
		ui.Spacer(c).MinWidth(8)
		switch a.section {
		case secLibrary:
			a.libraryToolbar(c)
		case secProject:
			a.projectToolbar(c)
		case secDiscover:
			a.discoverToolbar(c)
		}
		switch a.section {
		case secLibrary, secProject, secCreate, secGallery:
		default:
			// Elsewhere the search pill opens the palette.
			kit.ToolbarSearch(c.Key("search"), kit.ToolbarSearchOpts{Placeholder: T("library.search"), Hotkey: "mod+k", OnPalette: a.togglePalette})
		}
		switch {
		case a.section == secCreate && a.create.building == "":
			// Over the inspector: its settings search and Build, so both
			// stay in reach however far the settings scroll.
			if a.create.ready() && !a.inspectorNarrow() {
				a.createToolbar(c)
			}
		case a.section == secProject || a.section == secCreate:
			// The project's own Build is the primary here.
			// The project's Build is the primary here, so New App is a
			// quiet button, but still says what it does.
			if kit.Button(c.Key("tb-build"), T("menu.newApp"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ToolbarControl, Icon: icons.Plus}).Clicked() {
				a.newApp()
			}
		default:
			if kit.NewPill(c.Key("tb-build"), T("menu.newApp"), "mod+n", a.toolbarW > 0 && a.toolbarW < 700).Clicked() {
				a.newApp()
			}
		}
		windowButtons(c, a.win)
	})
}

// toolbarEnd is a toolbar's right padding: clear of the system's window
// controls where they overlap the canvas, and a pill's breadth from the
// canvas edge where the view draws its own (as from the top).
func toolbarEnd(c *ui.Context) float32 {
	if ownWindowButtons {
		return (design.TitleBarH - design.ToolbarControl) / 2
	}
	return max(c.TitleBar().Right-design.CanvasInset, 0) + 14
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
		sub = T("group.sites", "n", len(templates))
	case secBuilds:
		title = T("nav.builds")
		sub = T("builds.count", "count", len(a.jobs))
	case secRuntimes:
		title = T("nav.runtimes")
	case secCreate:
		title = T("create.toolbar")
	case secProject:
		// The page shows the app's name and site large; the bar keeps only Back.
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).MinWidth(0).Shrink(1).Children(func() {
		if a.section == secProject || a.section == secCreate {
			if kit.ToolbarIcon(c.Key("back"), icons.ChevronLeft, T("nav.back")).Clicked() {
				a.nav(secLibrary)
			}
		}
		if title == "" {
			return
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

// ---- content --------------------------------------------------------------------

func (a *App) content(c *ui.Context) {
	ui.Box(c.Key("pages")).Absolute().Fill().Children(func() { a.pageView(c) })
}

func (a *App) pageView(c *ui.Context) {
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
// toolbar and scrolls beneath it. Every page uses the canvas's full width,
// inset by the same gutter as the toolbar's title, so no page leaves empty
// bands at its sides.
func page(c *ui.Context, body func()) {
	ui.Scroll(c).Grow(1).Basis(0).ScrollbarInsets(design.TitleBarH, 0, 0).TrackScroll(&pageScroll).Children(func() {
		ui.Column(c).FillWidth().Gap(24).Padding(design.TitleBarH+16, design.PageGutter, 48, design.PageGutter).Children(body)
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
		ui.Row(c.Key("dropicon")).Size(64, 64).Radius(18).Center().Background(p.AccentSoft).Children(func() {
			kit.Glyph(c, icons.Import, 30, p.Accent)
		})
		ui.Text(c, T("drop.title")).FontSize(design.FontDisplay).FontWeight(design.WeightBold).TextColor(p.Text).LetterSpacing(design.TrackDisplay)
		kit.Secondary(c, T("drop.body"))
	})
}
