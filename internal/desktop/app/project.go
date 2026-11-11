package app

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
)

// projectStatus maps a project to its library status.
func (a *App) projectStatus(pr *domain.Project) (string, kit.Tone, bool) {
	if j := a.latestJob(pr.ID); j != nil {
		switch j.Status {
		case domain.JobRunning, domain.JobQueued:
			return stageLabel(*j), kit.ToneAccent, true
		case domain.JobAwaitingApproval:
			return T("stage.awaitingApproval"), kit.ToneWarning, false
		case domain.JobFailed:
			if a.installFor(pr.ID) == nil {
				return T("status.failed"), kit.ToneDanger, false
			}
		}
	}
	if in := a.installFor(pr.ID); in != nil {
		switch {
		case in.State == domain.InstallMissing:
			return T("status.missing"), kit.ToneWarning, false
		case in.UpdateReason != "":
			return T("status.update"), kit.ToneAccent, false
		}
		return T("status.installed"), kit.ToneSuccess, false
	}
	if pr.LastBuild != nil {
		return T("status.built"), kit.ToneNeutral, false
	}
	return T("status.draft"), kit.ToneNeutral, false
}

func (a *App) projectView(c *ui.Context) {
	pr := a.project(a.projectID)
	if pr == nil {
		if a.loaded {
			page(c, design.ContentNarrow, func() {
				kit.EmptyState(c, icons.Inbox, T("project.missing.title"), T("project.missing.body"), func() {
					if kit.Button(c, T("nav.library"), kit.ButtonOpts{}).Clicked() {
						a.nav(secLibrary)
					}
				})
			})
		}
		return
	}
	p := design.Of(c)
	page(c, 760, func() {
		// Header (the build panel shows the icon itself while it is up).
		var watched *domain.BuildJob
		if id, ok := a.watch[pr.ID]; ok {
			watched = a.jobByID(id)
		}
		if lj := a.latestJob(pr.ID); lj != nil && (lj.Status == domain.JobRunning || lj.Status == domain.JobQueued) {
			watched = lj
		}
		hero := a.projTab == 0 && watched != nil && watched.Status != domain.JobAwaitingApproval
		ui.Row(c).Gap(20).Hidden(hero).AlignItems(ui.Center).Children(func() {
			a.projectIcon(c, pr, 80)
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				kit.LargeTitle(c, pr.Config.App.Name).SingleLine().MinWidth(0)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					kit.Glyph(c, icons.Link, 12, p.TextTertiary)
					kit.Secondary(c, domainOf(pr.Config.App.Source)).SingleLine().MinWidth(0)
					if pr.LastBuild != nil {
						ui.Text(c, "· "+T("project.lastBuilt", "when", ago(*pr.LastBuild))).FontSize(design.FontBody).TextColor(p.TextTertiary).SingleLine()
					}
				})
				st, tone, busy := a.projectStatus(pr)
				if !busy {
					kit.StatusPill(c, st, tone).Margin(4, 0, 0, 0)
				}
			})
		})
		if a.projTab == 1 {
			ed := a.editorFor(pr)
			ui.Row(c).Children(func() {
				ui.Spacer(c)
				a.saveIndicator(c, ed)
			}).Margin(-12, 0, -12, 0)
			a.conflictBanner(c, pr, ed)
			ui.Column(c).Gap(14).Children(func() {
				kit.SectionHeader(c, T("settings.general"), "")
				a.lookAndBehaviour(c, ed.cfg, ed.changed, nil)
			})
			ui.Column(c).Gap(14).Children(func() {
				kit.SectionHeader(c, T("create.advanced"), "")
				a.settingsEditor(c, ed, pr)
			})
			return
		}
		a.projectOverview(c, pr, p)
	})
}

// projectIcon is the project's icon with a menu to change it.
func (a *App) projectIcon(c *ui.Context, pr *domain.Project, size float32) {
	p := design.Of(c)
	b := ui.ButtonBase(c.Key("icon")).Label(T("create.icon.change")).Size(size, size).Radius(design.AppIconRadius(size)).Cursor(ui.CursorPointer).Shrink(0)
	b.Menu(func(m *ui.Menu) {
		if m.Item(T("create.icon.choose")).Chosen() {
			id := pr.ID
			a.chooseIcon(func(b []byte) {
				a.do(func(ctx context.Context) error { _, err := a.svc.SetProjectIcon(ctx, id, b, "gui"); return err }, a.toastErr)
			})
		}
		if m.Item(T("create.icon.reset")).Disabled(pr.Config.App.Icon.Source != "file").Chosen() {
			id := pr.ID
			a.do(func(ctx context.Context) error { _, err := a.svc.SetProjectIcon(ctx, id, nil, "gui"); return err }, a.toastErr)
		}
	})
	hov := b.Hovered()
	b.Children(func() {
		kit.AppIconOpts(c, pr.IconPNG, pr.Config.App.Name, size, kit.IconOpts{Shadow: true})
		editBadge(c, p, size, hov)
	})
	b.Tooltip(T("create.icon.change"))
}

// tabGlide pairs the Overview/Settings indicator across frames.
type tabGlide struct{}

// projectToolbar holds the project's toolbar items: the Overview/Settings
// switch, Build and More.
func (a *App) projectToolbar(c *ui.Context) {
	pr := a.project(a.projectID)
	if pr == nil {
		return
	}
	p := design.Of(c)
	grp := ui.Row(c).Height(design.Control).Padding(2).Gap(0).Radius(design.RadiusControl + 1).Background(p.Fill).Role(ui.RoleTabList).Label(T("project.tabs"))
	grp.Children(func() {
		for i, label := range []string{T("project.tab.overview"), T("project.tab.settings")} {
			b := ui.ButtonBase(c.Key(i)).Label(label).Height(design.Control - 4).PaddingX(12).Center().Radius(design.RadiusControl - 1).Cursor(ui.CursorPointer).
				Role(ui.RoleTab).Selected(a.projTab == i)
			fg := p.TextSecondary
			if a.projTab == i {
				fg = p.Text
			}
			b.Children(func() {
				if a.projTab == i {
					ind := ui.Box(c).Absolute().Fill().Radius(design.RadiusControl - 1).ZIndex(-1).Background(p.Raised)
					design.Elevate(c, ind, design.Lifted)
					kit.Glide(ind, tabGlide{})
				}
				ui.Text(c, label).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(fg).SingleLine()
			})
			if b.Clicked() {
				a.projTab = i
			}
		}
	})
	busy := false
	if j := a.latestJob(pr.ID); j != nil && (j.Status == domain.JobRunning || j.Status == domain.JobQueued || j.Status == domain.JobAwaitingApproval) {
		busy = true
	}
	if kit.Button(c.Key("build"), T("action.build"), kit.ButtonOpts{Variant: kit.Primary, Icon: icons.Build,
		Disabled: busy, Hotkey: "mod+b"}).Clicked() {
		a.startBuild(pr.ID, nil)
	}
	kit.GlassIcon(c.Key("more"), icons.More, T("common.more"), "", false).Menu(func(m *ui.Menu) { a.projectMenu(m, pr) })
}

// projectMenu is shared by cards, rows and the project header.
func (a *App) projectMenu(m *ui.Menu, pr *domain.Project) {
	id := pr.ID
	if in := a.installFor(id); in != nil {
		if m.Item(T("action.open")).Chosen() {
			a.openInstall(in.ID)
		}
	}
	if m.Item(T("action.build")).Shortcut(ui.Cmd, ui.KeyB).Chosen() {
		a.startBuild(id, nil)
	}
	if m.Item(T("action.edit")).Chosen() {
		a.openProject(id, 1)
	}
	m.Separator()
	if m.Item(T("action.duplicate")).Chosen() {
		a.do(func(ctx context.Context) error { _, err := a.svc.DuplicateProject(ctx, id, "gui"); return err }, a.toastErr)
	}
	if m.Item(T("action.export")).Chosen() {
		a.exportProject(id, pr.Config.App.Name)
	}
	if j := a.lastSuccess(id); j != nil && len(j.Artifacts) > 0 {
		path := j.Artifacts[0].Path
		if m.Item(T("action.showInFinder")).Chosen() {
			a.reveal(path)
		}
	}
	m.Separator()
	if m.Item(T("action.trash")).Danger().Chosen() {
		a.trashProject(id, pr.Config.App.Name)
	}
}

func (a *App) lastSuccess(projectID string) *domain.BuildJob {
	for i := range a.jobs {
		if a.jobs[i].ProjectID == projectID && a.jobs[i].Status == domain.JobSucceeded {
			return &a.jobs[i]
		}
	}
	return nil
}

func (a *App) trashProject(id, name string) {
	a.do(func(ctx context.Context) error { return a.svc.TrashProject(ctx, id, "gui") }, func(err error) {
		if err != nil {
			a.toastErr(err)
			return
		}
		if a.section == secProject && a.projectID == id {
			a.nav(secLibrary)
		}
		a.toastAction(T("trash.done", "name", name), T("action.undo"), func() {
			a.do(func(ctx context.Context) error { return a.svc.RestoreProject(ctx, id, "gui") }, a.toastErr)
		})
	})
}

func (a *App) projectOverview(c *ui.Context, pr *domain.Project, p *design.Palette) {
	// Build panel for the watched (or running) job.
	var j *domain.BuildJob
	if id, ok := a.watch[pr.ID]; ok {
		j = a.jobByID(id)
	}
	if lj := a.latestJob(pr.ID); lj != nil && (lj.Status == domain.JobRunning || lj.Status == domain.JobQueued || lj.Status == domain.JobAwaitingApproval) {
		j = lj
	}
	if j != nil {
		a.buildPanel(c, pr, j)
	}

	// Installation.
	if in := a.installFor(pr.ID); in != nil {
		kit.Group(c, T("project.installed"), func() {
			kit.SettingIconRow(c, icons.Folder, ui.Color{}, T("app.location"), in.Path, func() {
				if kit.Button(c.Key("reveal"), T("action.reveal"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
					a.reveal(in.Path)
				}
				if kit.Button(c.Key("open"), T("action.open"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
					a.openInstall(in.ID)
				}
			})
			if in.UpdateReason != "" {
				kit.SettingIconRow(c, icons.ArrowUp, p.Accent, T("myapps.update.title"), in.UpdateReason, func() {
					if kit.Button(c.Key("upd"), T("action.reviewUpdate"), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlSM}).Clicked() {
						a.dlg.openUpdate(in)
					}
				})
			}
		})
	}

	// History.
	var hist []domain.BuildJob
	for _, hj := range a.jobs {
		if hj.ProjectID == pr.ID && (j == nil || hj.ID != j.ID) {
			hist = append(hist, hj)
		}
	}
	if j == nil && len(hist) == 0 {
		kit.Card(c).Padding(22).Gap(12).Children(func() {
			ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
				kit.SymbolTile(c, icons.Build, p.Accent, 44)
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					kit.Heading(c, T("project.firstBuild.title"))
					kit.Secondary(c, T("project.firstBuild.body"))
				})
				if kit.Button(c.Key("b"), T("create.cta.thisDevice", "device", deviceWord(a.host)), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlLG}).Clicked() {
					a.startBuild(pr.ID, nil)
				}
			})
		})
	}
	if len(hist) > 0 {
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, T("project.history")).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).Padding(0, 4).Role(ui.RoleHeading)
			kit.GroupBox(c).Children(func() {
				for i, hj := range hist {
					if i >= 6 {
						break
					}
					a.jobRow(c.Key(hj.ID), hj, false, i)
				}
			})
		})
	}
}

func (a *App) exportProject(id, name string) {
	go func() {
		path, err := mygoSave(a, strings.ReplaceAll(name, "/", "-")+".mypack.json")
		if err != nil || path == "" {
			return
		}
		a.do(func(ctx context.Context) error { return a.svc.ExportProject(ctx, id, path) }, func(err error) {
			if err != nil {
				a.toastErr(err)
				return
			}
			a.toast(T("export.done"), path, 1)
		})
	}()
}
