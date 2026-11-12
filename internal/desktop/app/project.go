package app

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
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
			page(c, func() {
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
	if a.projTab == 1 {
		a.projectSettingsView(c, pr)
		return
	}
	page(c, func() {
		// Header (the build panel shows the icon itself while it is up).
		var watched *domain.BuildJob
		if id, ok := a.watch[pr.ID]; ok {
			watched = a.jobByID(id)
		}
		if lj := a.latestJob(pr.ID); lj != nil && (lj.Status == domain.JobRunning || lj.Status == domain.JobQueued) {
			watched = lj
		}
		hero := a.projTab == 0 && watched != nil && watched.Status != domain.JobAwaitingApproval
		// The page carries a faint wash of the site's own colour.
		tint := kit.IconTint(pr.IconPNG, pr.Config.App.Name)
		ui.Box(c).Absolute().Top(-120).Left(-200).Right(-200).Height(520).PassThrough().AccessHidden(true).ZIndex(-1).
			RadialGradient(ui.RadialGradient{Center: ui.Point{X: 0.5, Y: 0.001}, Radius: 520,
				Stops: []ui.GradientStop{{Color: design.Fade(tint, 0.13), At: 0}, {Color: tint.Alpha(0), At: 1}}})
		if a.projTab == 0 && !hero {
			a.projectBento(c, pr)
			a.projectOverview(c, pr, p)
			return
		}
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
		a.projectOverview(c, pr, p)
	})
}

// projectIcon is the project's icon with a menu to change it.
func (a *App) projectIcon(c *ui.Context, pr *domain.Project, size float32) {
	p := design.Of(c)
	b := ui.ButtonBase(c.Key("icon")).Label(T("create.icon.change")).Size(size, size).Radius(design.AppIconRadius(size)).Cursor(ui.CursorPointer).Shrink(0)
	b.Menu(func(m *ui.Menu) {
		if m.Item(T("studio.open")).Chosen() {
			a.openIconStudio(pr.ID)
		}
		m.Separator()
		if m.Item(T("create.icon.choose")).Chosen() {
			id := pr.ID
			a.chooseIcon(func(b []byte) {
				a.do(func(ctx context.Context) error { _, err := a.svc.SetProjectIcon(ctx, id, b, "gui"); return err }, a.toastErr)
			})
		}
		if pr.Config.App.Source.Type == config.SourceURL && m.Item(T("icon.refresh")).Chosen() {
			a.refreshIcon(pr)
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
	kit.PillSegmented(c.Key("tabs"), &a.projTab, []kit.PillSegment{{Label: T("project.tab.overview")}, {Label: T("project.tab.settings")}},
		T("project.tabs"), tabGlide{}, true)
	busy := false
	if j := a.latestJob(pr.ID); j != nil && (j.Status == domain.JobRunning || j.Status == domain.JobQueued || j.Status == domain.JobAwaitingApproval) {
		busy = true
	}
	// Build stays in the bar on both tabs: the bar never reflows when the
	// tab changes (the tab indicator used to slide in from where the tabs
	// had been), and the page's primary action is always in one place.
	label := T("action.build")
	if pr.LastBuild != nil {
		label = T("action.rebuild")
	}
	if kit.Button(c.Key("build"), label, kit.ButtonOpts{Variant: kit.Primary, Size: design.ToolbarControl, Icon: icons.Build,
		Disabled: busy, Loading: busy, Hotkey: "mod+b"}).Clicked() {
		a.startBuild(pr.ID, nil)
		a.projTab = 0 // the build's progress shows on Overview
	}
	kit.ToolbarIcon(c.Key("more"), icons.More, T("common.more")).Menu(func(m *ui.Menu) { a.projectMenu(m, pr) })
}

// projectMenu is shared by cards, rows and the project header.
func (a *App) projectMenu(m *ui.Menu, pr *domain.Project) {
	id := pr.ID
	if label, _, run := a.projectLaunch(pr); label == T("action.open") {
		if m.Item(label).Chosen() {
			run()
		}
	}
	a.pinMenuItem(m, id)
	if m.Item(T("action.build")).Shortcut(ui.Cmd, ui.KeyB).Chosen() {
		a.startBuild(id, nil)
	}
	if m.Item(T("action.edit")).Chosen() {
		a.openProject(id, 1)
	}
	if pr.Config.App.Source.Type == config.SourceURL && m.Item(T("icon.refresh")).Chosen() {
		a.refreshIcon(pr)
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
			// Open and Show in Finder live on the hero above; the row only
			// says where the app is.
			kit.SettingIconRow(c, icons.Folder, ui.Color{}, T("app.location"), in.Path, nil)
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
		// Same eyebrow-over-box as Installed, so the page has one heading style.
		kit.Group(c, T("project.history"), func() {
			for i, hj := range hist {
				if i >= 6 {
					break
				}
				a.jobRow(c.Key(hj.ID), hj, false, i)
			}
		})
	}
}

// exportProject saves the app as a .mypack file (its icon included), to
// import on another computer.
func (a *App) exportProject(id, name string) { a.exportLibrary([]string{id}, name) }

// exportProjectConfig saves the app's mypack.json alone (for the CLI and
// version control).
func (a *App) exportProjectConfig(id, name string) {
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

// projectLaunch is the one-click action for a project: Open its installed
// app, else launch its latest build in place, else Build it.
func (a *App) projectLaunch(pr *domain.Project) (label string, ic *ui.SVG, run func()) {
	id := pr.ID
	if in := a.installFor(id); in != nil && in.State != domain.InstallMissing {
		return T("action.open"), icons.Play, func() { a.openInstall(in.ID) }
	}
	if j := a.lastSuccess(id); j != nil {
		for _, art := range j.Artifacts {
			if art.Target.OS == a.host.OS {
				artID := art.ID
				return T("action.open"), icons.Play, func() { a.openArtifact(artID) }
			}
		}
	}
	return T("action.build"), icons.Build, func() { a.startBuild(id, nil) }
}

func (a *App) openArtifact(id string) {
	a.do(func(ctx context.Context) error { return a.svc.OpenArtifact(ctx, id) }, a.toastErr)
}

// projectBento is the project's overview at a glance: a large hero tile
// (icon in its own light, the name in display type, the one-click action)
// beside small tiles for status, size, runtime and last build.
func (a *App) projectBento(c *ui.Context, pr *domain.Project) {
	p := design.Of(c)
	tint := kit.IconTint(pr.IconPNG, pr.Config.App.Name)
	st, tone, _ := a.projectStatus(pr)
	var size, runtime, built string = "—", "—", "—"
	if j := a.lastSuccess(pr.ID); j != nil {
		if len(j.Artifacts) > 0 {
			size = bytesLabel(j.Artifacts[0].Size)
		}
		runtime = j.RuntimeVersion
		built = ago(j.CreatedAt)
	}
	tile := func(key string) ui.Element {
		t := ui.Column(c.Key(key)).Padding(16, 18).Gap(6).Radius(design.RadiusPanel).Border(0.5, p.GroupBorder).MinWidth(0)
		design.Tint(t, p.CanvasTop, p.CanvasBot)
		return design.Elevate(c, t, design.Lifted)
	}
	ui.Grid(c).ColumnTracks(ui.Fr(1), ui.Fr(1), ui.Fr(1), ui.Fr(1)).Gap(12).Children(func() {
		hero := tile("hero").ColumnSpan(2).RowSpan(2).Padding(22).Gap(14).Clip().Justify(ui.SpaceBetween)
		hero.Children(func() {
			ui.Box(c).Absolute().Fill().PassThrough().AccessHidden(true).ZIndex(-1).
				LinearGradient(ui.LinearGradient{Angle: 160, From: design.Fade(tint, 0.16), To: tint.Alpha(0)})
			ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(84, 84).Children(func() {
					a.projectIcon(c, pr, 84)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).AlignItems(ui.Start).Children(func() {
					kit.LargeTitle(c, pr.Config.App.Name).SingleLine().MinWidth(0)
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						kit.Glyph(c, icons.Link, 12, p.TextTertiary)
						kit.Secondary(c, domainOf(pr.Config.App.Source)).SingleLine().MinWidth(0)
					})
				})
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				label, ic, run := a.projectLaunch(pr)
				if kit.Button(c.Key("launch"), label, kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlLG, Icon: ic}).Clicked() {
					run()
				}
				// Show in Finder: the installed app when there is one, else
				// the latest build.
				path := ""
				if in := a.installFor(pr.ID); in != nil && in.State != domain.InstallMissing {
					path = in.Path
				} else if j := a.lastSuccess(pr.ID); j != nil && len(j.Artifacts) > 0 {
					path = j.Artifacts[0].Path
				}
				if path != "" {
					if kit.Button(c.Key("reveal"), T("action.showInFinder"), kit.ButtonOpts{Size: design.ControlLG, Icon: icons.Folder}).Clicked() {
						a.reveal(path)
					}
				}
			})
		})
		stat := func(i int, key, label, value string, numeric bool, dot *ui.Color) {
			t := tile(key)
			t.Children(func() {
				kit.Eyebrow(c, label)
				// A status dot sits beside its word, read as one.
				ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
					if dot != nil {
						ui.Box(c).Size(8, 8).Radius(4).Background(*dot).Shrink(0)
					}
					v := ui.Text(c, value).FontSize(design.FontTitle1).FontWeight(design.WeightSemibold).TextColor(p.Text).
						LetterSpacing(design.TrackTitle).SingleLine().MinWidth(0)
					if numeric {
						v.FontFeatures("tnum")
					}
				})
			})
		}
		fg, _ := kit.ToneColors(p, tone)
		stat(1, "status", T("list.col.status"), st, false, &fg)
		stat(2, "size", T("builds.size"), size, true, nil)
		stat(3, "runtime", T("app.runtime"), runtime, true, nil)
		stat(4, "built", T("project.bento.built"), built, false, nil)
	})
}

// projectSettingsView is the project's Settings tab: the app previewed on
// the left (icon, name, site, Dock), its settings in the inspector on the
// right, saved as they change.
func (a *App) projectSettingsView(c *ui.Context, pr *domain.Project) {
	p := design.Of(c)
	ed := a.editorFor(pr)
	a.previewInspector(c, func() {
		ui.Column(c).AlignItems(ui.Center).Gap(0).Padding(32, 0, 24, 0).Children(func() {
			a.projectIcon(c, pr, heroIcon)
			kit.PageTitle(c, ed.cfg.App.Name).Margin(16, 0, 2, 0).SingleLine()
			ui.Row(c).Height(20).Gap(5).AlignItems(ui.Center).Children(func() {
				kit.Glyph(c, icons.Link, 12, p.TextTertiary)
				ui.Text(c, domainOf(pr.Config.App.Source)).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine()
			})
			a.saveIndicator(c, ed).Margin(10, 0, 0, 0)
			// Build is in the bar above; under the preview, Open the
			// installed app to try a change.
			if in := a.installFor(pr.ID); in != nil && in.State != domain.InstallMissing {
				id := in.ID
				ui.Row(c).Gap(6).AlignItems(ui.Center).Margin(18, 0, 0, 0).Children(func() {
					if kit.Button(c.Key("links"), T("action.open"), kit.ButtonOpts{Size: design.ControlLG, Icon: icons.Play}).Clicked() {
						a.openInstall(id)
					}
				})
			}
			a.appDockPreview(c, pr.IconPNG, ed.cfg.App.Name)
		})
	}, func() {
		a.settingHits, a.settingsQ = 0, strings.TrimSpace(ed.query)
		searching := a.settingsQ != ""
		a.conflictBanner(c, pr, ed)
		ui.Column(c).Gap(14).Children(func() {
			if !searching {
				kit.SectionHeader(c, T("settings.general"), "")
			}
			if a.settingHit(T("autoInstall.label"), T("autoInstall.desc")) {
				kit.GroupBox(c).Children(func() {
					on := a.ui.autoInstall(pr.ID)
					kit.SettingIconRow(c.Key("autoinst"), icons.Play, ui.Color{}, T("autoInstall.label"), T("autoInstall.desc"), func() {
						old := on
						kit.Switch(c.Key("sw"), &on, T("autoInstall.label"))
						if on != old {
							a.ui.setAutoInstall(pr.ID, on)
						}
					})
				})
			}
			if ed.cfg.IsGroup() {
				a.sitesCard(c, ed.cfg, ed.changed)
			}
			a.lookAndBehaviourOpts(c, ed.cfg, ed.changed, nil, true)
		})
		ui.Column(c).Gap(14).Children(func() {
			if !searching {
				kit.SectionHeader(c, T("create.advanced"), "")
			}
			a.settingsEditor(c, ed, pr)
		})
		a.searchEmpty(c)
	}, nil, func() {
		a.settingsSearch(c, ed)
	})
}

// refreshIcon fetches a URL project's icon from its website again, through
// today's icon pipeline, and sets it (Refresh Icon from Website).
func (a *App) refreshIcon(pr *domain.Project) {
	id, src, name := pr.ID, pr.Config.App.Source.Value, pr.Config.App.Name
	a.do(func(ctx context.Context) error {
		m, err := a.svc.FetchMetadata(ctx, src)
		if err != nil {
			return err
		}
		if len(m.IconPNG) == 0 || m.Fallback {
			return &domain.Error{Code: "ICON_NOT_FOUND", Message: T("icon.refresh.none", "name", name)}
		}
		_, err = a.svc.SetProjectIcon(ctx, id, m.IconPNG, "gui")
		return err
	}, func(err error) {
		if err != nil {
			a.toastErr(err)
			return
		}
		a.toast(T("icon.refresh.done", "name", name), "", 1)
	})
}
