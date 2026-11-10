package app

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/components"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
	"mypack.dev/mypack/internal/domain"
)

// projectStatus maps a project to its library status.
func (a *App) projectStatus(pr *domain.Project) (string, components.Tone, bool) {
	if j := a.latestJob(pr.ID); j != nil {
		switch j.Status {
		case domain.JobRunning, domain.JobQueued:
			return stageLabel(*j), components.ToneAccent, true
		case domain.JobAwaitingApproval:
			return T("stage.awaitingApproval"), components.ToneWarning, false
		case domain.JobFailed:
			if a.installFor(pr.ID) == nil {
				return T("status.failed"), components.ToneDanger, false
			}
		}
	}
	if in := a.installFor(pr.ID); in != nil {
		switch {
		case in.State == domain.InstallMissing:
			return T("status.missing"), components.ToneWarning, false
		case in.UpdateReason != "":
			return T("status.update"), components.ToneAccent, false
		}
		return T("status.installed"), components.ToneSuccess, false
	}
	if pr.LastBuild != nil {
		return T("status.built"), components.ToneNeutral, false
	}
	return T("status.draft"), components.ToneNeutral, false
}

func (a *App) projectView(c *ui.Context) {
	pr := a.project(a.projectID)
	if pr == nil {
		if a.loaded {
			page(c, design.ContentNarrow, func() {
				components.EmptyState(c, lobeui.IconInbox(), T("project.missing.title"), T("project.missing.body"), func() {
					if components.Button(c, T("nav.library"), components.ButtonOpts{}).Clicked() {
						a.nav(secLibrary)
					}
				})
			})
		}
		return
	}
	p := design.Of(c)
	page(c, 760, func() {
		// Header.
		ui.Row(c).Gap(16).AlignItems(ui.Center).Margin(12, 0, 0, 0).Children(func() {
			components.IconPicker(c.Key("icon"), pr.IconPNG, pr.Config.App.Name, 56, T("create.icon.change"), func(m *ui.Menu) {
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
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					components.PageTitle(c, pr.Config.App.Name).SingleLine().MinWidth(0)
					st, tone, busy := a.projectStatus(pr)
					if !busy {
						components.StatusPill(c, st, tone)
					}
				})
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					components.Secondary(c, domainOf(pr.Config.App.Source)).SingleLine().MinWidth(0)
					if pr.LastBuild != nil {
						components.Caption(c, "· "+T("project.lastBuilt", "when", ago(*pr.LastBuild)))
					}
				})
			})
			a.projectActions(c, pr)
		})
		// Tabs.
		ui.Row(c).Children(func() {
			lobeui.Segmented(c.Key("tabs"), &a.projTab, []string{T("project.tab.overview"), T("project.tab.settings")}, lobeui.Middle)
			ui.Spacer(c)
			if a.projTab == 1 {
				a.saveIndicator(c, a.editorFor(pr))
			}
		})
		if a.projTab == 1 {
			ed := a.editorFor(pr)
			a.conflictBanner(c, pr, ed)
			a.settingsEditor(c, ed, pr)
			return
		}
		a.projectOverview(c, pr, p)
	})
}

func (a *App) projectActions(c *ui.Context, pr *domain.Project) {
	busy := false
	if j := a.latestJob(pr.ID); j != nil && (j.Status == domain.JobRunning || j.Status == domain.JobQueued || j.Status == domain.JobAwaitingApproval) {
		busy = true
	}
	ui.Row(c).Gap(8).Shrink(0).Children(func() {
		if components.Button(c.Key("build"), T("action.build"), components.ButtonOpts{Variant: components.Primary, Icon: lobeui.IconPlay(),
			Disabled: busy, Hotkey: "mod+b"}).Clicked() {
			a.startBuild(pr.ID, nil)
		}
		components.IconButton(c.Key("more"), lobeui.IconMore(), T("common.more")).Size(32, 32).Menu(func(m *ui.Menu) { a.projectMenu(m, pr) })
	})
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
		components.Group(c, T("project.installed"), func() {
			components.SettingRow(c, T("app.location"), in.Path, func() {
				if components.Button(c.Key("reveal"), T("action.reveal"), components.ButtonOpts{Size: design.ControlSM}).Clicked() {
					a.reveal(in.Path)
				}
				if components.Button(c.Key("open"), T("action.open"), components.ButtonOpts{Size: design.ControlSM}).Clicked() {
					a.openInstall(in.ID)
				}
			})
			if in.UpdateReason != "" {
				components.SettingRow(c, T("myapps.update.title"), in.UpdateReason, func() {
					if components.Button(c.Key("upd"), T("action.reviewUpdate"), components.ButtonOpts{Variant: components.Primary, Size: design.ControlSM}).Clicked() {
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
		components.Card(c).Padding(20).Gap(12).Children(func() {
			ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
				components.IconBadge(c, lobeui.IconPlay(), components.ToneAccent, 36)
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					components.Heading(c, T("project.firstBuild.title"))
					components.Secondary(c, T("project.firstBuild.body"))
				})
				if components.Button(c.Key("b"), T("create.cta.thisDevice", "device", deviceWord(a.host)), components.ButtonOpts{Variant: components.Primary}).Clicked() {
					a.startBuild(pr.ID, nil)
				}
			})
		})
	}
	if len(hist) > 0 {
		ui.Column(c).Gap(8).Children(func() {
			components.Heading(c, T("project.history"))
			ui.Column(c).Background(p.Surface).Border(1, p.Border).Radius(design.RadiusCard-2).Dividers(1, p.Divider).Children(func() {
				for i, hj := range hist {
					if i >= 6 {
						break
					}
					a.jobRow(c.Key(hj.ID), hj, false)
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
