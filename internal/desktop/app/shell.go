package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/components"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
	"mypack.dev/mypack/internal/domain"
)

func (a *App) view(c *ui.Context) {
	design.Apply(c)
	p := design.Of(c)
	root := ui.Row(c).Fill().AlignItems(ui.Stretch)
	if !c.Vibrancy() {
		root.Background(p.Sidebar)
	}
	root.Children(func() {
		a.sidebar(c)
		main := ui.Column(c).Grow(1).Basis(0).MinWidth(0).Background(p.Bg).BorderWidth(0, 0, 0, 1).BorderColor(p.Border)
		main.Children(func() {
			a.toolbar(c)
			a.content(c)
		})
	})
	lobeui.ToastHost(c)
	a.dialogs(c)
	for _, t := range a.toasts {
		if t.action != "" {
			lobeui.ToastAction(c, t.title, t.action, t.fn)
		} else {
			lobeui.Toast(c, t.title, t.desc, lobeui.ToastKind(t.kind))
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

func (a *App) sidebar(c *ui.Context) {
	p := design.Of(c)
	ui.Column(c).Width(design.SidebarWidth).Shrink(0).Padding(0, 10, 12, 10).Children(func() {
		// Title-bar strip (traffic lights) + brand.
		ui.Row(c).Height(design.TitleBarH).Shrink(0).DragWindow()
		ui.Row(c).Gap(9).AlignItems(ui.Center).Padding(0, 8, 14, 8).Children(func() {
			components.BrandMark(c, 22)
			ui.Text(c, "mypack").FontSize(design.FontTitle3).FontWeight(design.WeightSemibold).TextColor(p.Text).LetterSpacing(-0.2)
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
			a.navItem(c, secLibrary, lobeui.IconLibrary(), T("nav.library"), badge, "mod+1")
			a.navItem(c, secDiscover, lobeui.IconDiscover(), T("nav.discover"), "", "mod+2")
			running := ""
			if n := len(a.activeJobs()); n > 0 {
				running = "•"
			}
			a.navItem(c, secBuilds, lobeui.IconBuilds(), T("nav.builds"), running, "mod+3")
		})
		// Recent projects.
		if len(a.projects) > 0 {
			ui.Text(c, T("nav.recent")).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).
				TextColor(p.TextTertiary).Padding(18, 10, 6, 10)
			ui.Column(c).Gap(1).Children(func() {
				for i, pr := range a.projects {
					if i >= 5 {
						break
					}
					pr := pr
					active := a.section == secProject && a.projectID == pr.ID
					row := ui.ButtonBase(c.Key(pr.ID)).Label(pr.Config.App.Name).Row().Justify(ui.Start).Height(28).PaddingX(10).Gap(9).
						AlignItems(ui.Center).Radius(design.RadiusControl).Cursor(ui.CursorPointer).Transition(design.Transition(c))
					if active {
						row.Background(p.Selected)
					} else if row.Hovered() {
						row.Background(p.Hover)
					}
					row.Children(func() {
						components.AppIcon(c, pr.IconPNG, pr.Config.App.Name, 16)
						fg := p.TextSecondary
						if active {
							fg = p.Text
						}
						ui.Text(c, pr.Config.App.Name).FontSize(design.FontBody).TextColor(fg).SingleLine().MinWidth(0)
						if j := a.latestJob(pr.ID); j != nil && (j.Status == domain.JobRunning || j.Status == domain.JobQueued) {
							ui.Spacer(c)
							components.Spinner(c, 11, p.TextTertiary)
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
			a.navItem(c, secRuntimes, lobeui.IconRuntimes(), T("nav.runtimes"), "", "mod+4")
			a.navItem(c, secSettings, lobeui.IconSettings(), T("nav.settings"), "", "mod+,")
		})
	})
}

func (a *App) navItem(c *ui.Context, s section, icon *ui.SVG, label, badge, hotkey string) {
	p := design.Of(c)
	active := a.section == s
	row := ui.ButtonBase(c.Key(string(s))).Label(label).Row().Justify(ui.Start).Height(30).PaddingX(10).Gap(10).AlignItems(ui.Center).
		Radius(design.RadiusControl).Cursor(ui.CursorPointer).Transition(design.Transition(c)).Current(ui.CurrentPage)
	if !active {
		row.Current(ui.CurrentNone)
	}
	if active {
		row.Background(p.Selected)
	} else if row.Hovered() {
		row.Background(p.Hover)
	}
	row.Children(func() {
		col, tc := p.TextSecondary, p.TextSecondary
		if active {
			col, tc = p.AccentText, p.Text
		}
		ui.Icon(c, icon).FontSize(16).TextColor(col)
		ui.Text(c, label).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(tc).SingleLine()
		ui.Spacer(c)
		switch badge {
		case "":
		case "•":
			components.Spinner(c, 12, p.AccentText)
		default:
			ui.Text(c, badge).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).TextColor(p.AccentText).
				Padding(1, 6).Radius(8).Background(p.AccentSoft)
		}
	})
	if row.Clicked() {
		a.nav(s)
	}
}

// ---- toolbar --------------------------------------------------------------------

func (a *App) toolbar(c *ui.Context) {
	p := design.Of(c)
	bar := c.TitleBar()
	ui.Row(c).Height(design.TitleBarH).Shrink(0).Padding(0, bar.Right+20, 0, 24).Gap(10).AlignItems(ui.Center).DragWindow().Children(func() {
		a.toolbarTitle(c, p)
		ui.Spacer(c)
		a.activityIndicator(c)
		if a.section != secCreate {
			if components.Button(c.Key("tb-build"), T("toolbar.buildWebsite"), components.ButtonOpts{
				Variant: components.Primary, Icon: lobeui.IconPlus(), Hotkey: "mod+n", Tooltip: T("toolbar.buildWebsite.tip"),
			}).Clicked() {
				a.newApp()
			}
		}
	})
}

func (a *App) toolbarTitle(c *ui.Context, p *design.Palette) {
	title := ""
	switch a.section {
	case secLibrary:
		title = T("nav.library")
	case secDiscover:
		title = T("nav.discover")
	case secBuilds:
		title = T("nav.builds")
	case secRuntimes:
		title = T("nav.runtimes")
	case secSettings:
		title = T("nav.settings")
	case secCreate:
		title = T("create.toolbar")
	}
	if a.section == secProject || a.section == secCreate {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			back := ui.ButtonBase(c.Key("crumb")).Label(T("nav.library")).Padding(3, 6).Radius(design.RadiusSM).Cursor(ui.CursorPointer)
			if back.Hovered() {
				back.Background(p.Hover)
			}
			back.Children(func() {
				ui.Text(c, T("nav.library")).FontSize(design.FontTitle3).FontWeight(design.WeightMedium).TextColor(p.TextSecondary)
			})
			if back.Clicked() {
				a.nav(secLibrary)
			}
			ui.Icon(c, lobeui.IconChevronRt()).FontSize(13).TextColor(p.TextTertiary)
			name := title
			if a.section == secProject {
				if pr := a.project(a.projectID); pr != nil {
					name = pr.Config.App.Name
				}
			}
			ui.Text(c, name).FontSize(design.FontTitle3).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
		})
		return
	}
	ui.Text(c, title).FontSize(design.FontTitle3).FontWeight(design.WeightSemibold).TextColor(p.Text).Role(ui.RoleHeading)
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
	b := ui.ButtonBase(c.Key("activity")).Label(label).Row().Justify(ui.Start).Height(28).PaddingX(10).Gap(7).AlignItems(ui.Center).
		Radius(14).Background(p.Hover).Cursor(ui.CursorPointer).Transition(design.Transition(c)).Role(ui.RoleStatus)
	if b.Hovered() {
		b.Background(p.Pressed)
	}
	b.Children(func() {
		if j.Status == domain.JobAwaitingApproval {
			ui.Icon(c, lobeui.IconDownload()).FontSize(13).TextColor(p.Warning)
		} else {
			components.Spinner(c, 13, p.AccentText)
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
	ui.Column(c.Key(string(a.section)+a.projectID)).Grow(1).Basis(0).MinHeight(0).Children(func() {
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
		}
	})
}

// page is the standard scrolling content column.
func page(c *ui.Context, maxW float32, body func()) {
	ui.Scroll(c).Grow(1).Basis(0).Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(8, 32, 40, 32).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(maxW).Gap(20).Children(body)
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
