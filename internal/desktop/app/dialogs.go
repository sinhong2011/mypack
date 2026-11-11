package app

import (
	"context"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

type dialogs struct {
	approving        string
	installing       string
	installConflict  bool
	conflictArtifact string

	identity  identityDialog
	reset     resetDialog
	uninstall uninstallDialog
	update    installDialog
	repair    installDialog
	details   detailsDialog
	group     groupDialog

	licenseLimit      bool // free-edition limit explainer (spec §9E.6)
	groupLimit        bool // free edition: one group app
	deactivateLicense licenseConfirm
}

type licenseConfirm struct {
	open, busy bool
}

type identityDialog struct {
	open       bool
	ed         *editorState
	newID      string
	newProfile bool
	fromSave   bool
}

type resetDialog struct {
	open bool
	ed   *editorState
}

type uninstallDialog struct {
	open      bool
	id        string
	clearData bool
	busy      bool
	err       *domain.Error
}

type installDialog struct {
	open bool
	id   string
	busy bool
	err  *domain.Error // the last attempt failed: said in the sheet
}

type detailsDialog struct {
	open bool
	id   string
}

func (d *dialogs) openUpdate(in *domain.Installation) {
	d.update = installDialog{open: true, id: in.ID}
}

func bgctx() context.Context { return context.Background() }

// mygoSave asks for a save path (call off the main thread).
func mygoSave(a *App, name string) (string, error) {
	return mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, DefaultPath: name,
		Filters: []mygo.FileFilter{{Name: "mypack.json", Extensions: []string{"json"}}}})
}

func (a *App) install(id string) *domain.Installation {
	for i := range a.installs {
		if a.installs[i].ID == id {
			return &a.installs[i]
		}
	}
	return nil
}

func (a *App) dialogs(c *ui.Context) {
	p := design.Of(c)
	d := &a.dlg
	a.groupSheet(c)

	// Identity change (spec §9C.2: explain and confirm).
	if d.identity.open {
		id := &d.identity
		kit.Sheet(c.Key("dlg-identity"), &id.open, 480, func() {
			kit.SheetHeader(c, T("identity.title"), nil)
			ui.Column(c).Gap(12).Children(func() {
				ui.Text(c, T("identity.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				kit.Note(c, icons.Warning, T("identity.warning"), kit.ToneWarning)
				in := kit.TextField(c.Key("newid"), &id.newID, T("set.identifier"), kit.FieldOpts{Placeholder: "app.mypack.example", Mono: true})
				_ = in
				kit.Checkbox(c.Key("np"), &id.newProfile, T("identity.newProfile"))
				ok, cancel := kit.SheetFooter(c, T("common.cancel"), T("identity.confirm"), kit.Danger, false, id.newID == "" || id.newID == id.ed.cfg.App.Identifier && !id.fromSave)
				if cancel {
					id.open = false
					if id.fromSave {
						id.ed.cfg.App.Identifier = a.project(id.ed.projectID).Config.App.Identifier
						id.ed.dirty = false
					}
				}
				if ok {
					id.ed.cfg.App.Identifier = id.newID
					if id.newProfile {
						id.ed.cfg.App.ProfileID = id.newID
					}
					id.open = false
					a.saveEditor(id.ed, true, id.ed.rev)
				}
			})
		})
	}

	// Reset to recommended defaults.
	if d.reset.open {
		r := &d.reset
		kit.Sheet(c.Key("dlg-reset"), &r.open, 440, func() {
			kit.SheetHeader(c, T("reset.title"), nil)
			ui.Column(c).Gap(12).Children(func() {
				ui.Text(c, T("reset.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				ok, cancel := kit.SheetFooter(c, T("common.cancel"), T("reset.confirm"), kit.Primary, false, false)
				if cancel {
					r.open = false
				}
				if ok {
					old := r.ed.cfg
					n := config.New(old.App.Name, old.App.Source, a.host)
					n.App.Identifier, n.App.ProfileID, n.App.Icon, n.App.Version = old.App.Identifier, old.App.ProfileID, old.App.Icon, old.App.Version
					n.Build.Targets = old.Build.Targets
					*r.ed.cfg = *n
					r.ed.inited = false
					r.ed.changed()
					r.open = false
					a.toast(T("reset.done"), "", 1)
				}
			})
		})
	}

	// Uninstall (website data unchecked by default, irreversible warning).
	if d.uninstall.open {
		u := &d.uninstall
		in := a.install(u.id)
		if in == nil {
			u.open = false
		} else {
			kit.Sheet(c.Key("dlg-uninstall"), &u.open, 460, func() {
				kit.SheetHeader(c, T("uninstall.title", "name", in.Name), nil)
				ui.Column(c).Gap(12).Children(func() {
					ui.Text(c, T("uninstall.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
					ui.Column(c).Gap(6).Padding(12, 14).Radius(design.RadiusCard).Background(p.Group).Border(0.5, p.GroupBorder).Children(func() {
						kit.Checkbox(c.Key("cd"), &u.clearData, T("uninstall.clearData"))
						if u.clearData {
							ui.Text(c, T("uninstall.clearData.warning")).FontSize(design.FontSmall).TextColor(p.Danger).LineHeight(1.4)
						} else {
							kit.Caption(c, T("uninstall.keepData"))
						}
					})
					if u.err != nil {
						a.failureBody(c, u.err, "", a.fixesFor(u.err, "", nil))
					}
					ok, cancel := kit.SheetFooter(c, T("common.cancel"), T("action.uninstall"), kit.Danger, u.busy, false)
					if cancel {
						u.open = false
					}
					if ok {
						u.busy = true
						id, clear, name := u.id, u.clearData, in.Name
						a.do(func(ctx context.Context) error { return a.svc.Uninstall(ctx, id, clear, "gui") }, func(err error) {
							u.busy = false
							if err != nil {
								// Said in the sheet, which stays open to try again.
								u.err = errInfo(err)
								return
							}
							u.open, u.err = false, nil
							a.toast(T("uninstall.done", "name", name), "", 1)
						})
					}
				})
			})
		}
	}

	// Update review ("What's changing").
	if d.update.open {
		u := &d.update
		in := a.install(u.id)
		if in == nil {
			u.open = false
		} else {
			kit.Sheet(c.Key("dlg-update"), &u.open, 480, func() {
				kit.SheetHeader(c, T("update.title", "name", in.Name), nil)
				ui.Column(c).Gap(12).Children(func() {
					ui.Text(c, T("update.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
					kit.Overline(c, T("update.changing"))
					ui.Column(c).Gap(8).Padding(12, 14).Radius(design.RadiusCard).Background(p.Group).Border(0.5, p.GroupBorder).Children(func() {
						row := func(icon *ui.SVG, text string) {
							ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
								kit.Glyph(c, icon, 14, p.TextSecondary).Margin(1, 0, 0, 0)
								ui.Text(c, text).FontSize(design.FontBody).TextColor(p.Text).Grow(1).MinWidth(0).LineHeight(1.4)
							})
						}
						row(icons.Update, in.UpdateReason)
						row(icons.Shield, T("update.profileKept"))
						row(icons.Retry, T("update.rollback"))
					})
					if u.err != nil {
						a.failureBody(c, u.err, "", a.fixesFor(u.err, in.ProjectID, nil))
					}
					ok, cancel := kit.SheetFooter(c, T("action.later"), T("action.update"), kit.Primary, u.busy, false)
					if cancel {
						u.open = false
					}
					if ok {
						u.busy = true
						id, pid := in.ID, in.ProjectID
						a.do(func(ctx context.Context) error {
							j, err := a.svc.UpdateInstallation(ctx, id, "gui")
							if err == nil {
								a.win.Update(func() { a.watch[pid] = j.ID; a.upsertJob(j) })
							}
							return err
						}, func(err error) {
							u.busy = false
							if err != nil {
								// Said in the sheet, which stays open to try again.
								u.err = errInfo(err)
								return
							}
							u.open, u.err = false, nil
							if pid != "" {
								a.openProject(pid, 0)
							}
						})
					}
				})
			})
		}
	}

	// Repair review.
	if d.repair.open {
		u := &d.repair
		in := a.install(u.id)
		if in == nil {
			u.open = false
		} else {
			kit.Sheet(c.Key("dlg-repair"), &u.open, 460, func() {
				kit.SheetHeader(c, T("repair.title", "name", in.Name), nil)
				ui.Column(c).Gap(12).Children(func() {
					ui.Text(c, T("repair.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
					if u.err != nil {
						a.failureBody(c, u.err, "", a.fixesFor(u.err, in.ProjectID, nil))
					}
					ok, cancel := kit.SheetFooter(c, T("common.cancel"), T("action.repair"), kit.Primary, u.busy, false)
					if cancel {
						u.open = false
					}
					if ok {
						u.busy = true
						id, pid := in.ID, in.ProjectID
						a.do(func(ctx context.Context) error {
							j, err := a.svc.RepairInstallation(ctx, id, "gui")
							if err == nil {
								a.win.Update(func() { a.watch[pid] = j.ID; a.upsertJob(j) })
							}
							return err
						}, func(err error) {
							u.busy = false
							if err != nil {
								// Said in the sheet, which stays open to try again.
								u.err = errInfo(err)
								return
							}
							u.open, u.err = false, nil
							a.openProject(pid, 0)
						})
					}
				})
			})
		}
	}

	// Installed app details.
	if d.details.open {
		u := &d.details
		in := a.install(u.id)
		if in == nil {
			u.open = false
		} else {
			a.detailsModal(c, u, in)
		}
	}

	// Free-edition limit: calm and factual, no countdowns, no price (spec §9E.6).
	if d.licenseLimit {
		limit := a.license.FreeLimit
		if limit == 0 {
			limit = appservice.FreeManagedApps
		}
		kit.Sheet(c.Key("dlg-limit"), &d.licenseLimit, 460, func() {
			kit.SheetHeader(c, T("license.limit.title"), nil)
			ui.Column(c).Gap(12).Children(func() {
				ui.Text(c, T("license.limit.body", "limit", limit)).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				ui.Text(c, T("license.limit.options")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				ui.Row(c).Gap(8).Justify(ui.End).Margin(8, 0, 0, 0).Children(func() {
					if kit.Button(c.Key("later"), T("license.limit.later"), kit.ButtonOpts{Variant: kit.Ghost}).Clicked() {
						d.licenseLimit = false
					}
					if kit.Button(c.Key("manage"), T("license.limit.manage"), kit.ButtonOpts{}).Clicked() {
						d.licenseLimit = false
						a.nav(secLibrary)
					}
					if kit.Button(c.Key("enter"), T("license.limit.enter"), kit.ButtonOpts{Variant: kit.Primary}).Clicked() {
						d.licenseLimit = false
						a.nav(secSettings)
					}
				})
			})
		})
	}

	// Free edition: one group app; Pro makes as many as you like.
	if d.groupLimit {
		kit.Sheet(c.Key("dlg-glimit"), &d.groupLimit, 460, func() {
			kit.SheetHeader(c, T("group.limit.title"), nil)
			ui.Column(c).Gap(12).Children(func() {
				ui.Text(c, T("group.limit.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				ui.Row(c).Gap(8).Justify(ui.End).Margin(8, 0, 0, 0).Children(func() {
					if kit.Button(c.Key("later"), T("license.limit.later"), kit.ButtonOpts{Variant: kit.Ghost}).Clicked() {
						d.groupLimit = false
					}
					if kit.Button(c.Key("enter"), T("license.limit.enter"), kit.ButtonOpts{Variant: kit.Primary}).Clicked() {
						d.groupLimit = false
						a.nav(secSettings)
					}
				})
			})
		})
	}

	// Deactivate license.
	if d.deactivateLicense.open {
		u := &d.deactivateLicense
		kit.Sheet(c.Key("dlg-deact"), &u.open, 440, func() {
			kit.SheetHeader(c, T("license.deactivate.title"), nil)
			ui.Column(c).Gap(12).Children(func() {
				ui.Text(c, T("license.deactivate.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				ok, cancel := kit.SheetFooter(c, T("common.cancel"), T("license.deactivate"), kit.Primary, u.busy, false)
				if cancel {
					u.open = false
				}
				if ok {
					u.busy = true
					a.do(func(ctx context.Context) error { return a.svc.DeactivateLicense(ctx, "gui") }, func(err error) {
						u.busy, u.open = false, false
						if err != nil {
							a.toastErr(err)
							return
						}
						a.reload("license")
						a.toast(T("license.deactivated"), "", 1)
					})
				}
			})
		})
	}

	// Install conflict.
	if d.installConflict {
		kit.Sheet(c.Key("dlg-conflict"), &d.installConflict, 440, func() {
			kit.SheetHeader(c, T("installConflict.title"), nil)
			ui.Column(c).Gap(12).Children(func() {
				ui.Text(c, T("installConflict.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				ok, cancel := kit.SheetFooter(c, T("common.cancel"), T("installConflict.replace"), kit.Danger, false, false)
				if cancel {
					d.installConflict = false
				}
				if ok {
					d.installConflict = false
					a.installArtifact(d.conflictArtifact, true)
				}
			})
		})
	}
}

func (a *App) detailsModal(c *ui.Context, u *detailsDialog, in *domain.Installation) {
	kit.Sheet(c.Key("dlg-details"), &u.open, 520, func() {
		ui.Column(c).Gap(16).Children(func() {
			ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(72, 72).Children(func() {
					kit.AppIconOpts(c, in.IconPNG, in.Name, 72, kit.IconOpts{Shadow: true})
				})
				ui.Column(c).Gap(4).Grow(1).MinWidth(0).Children(func() {
					kit.PageTitle(c, in.Name).SingleLine()
					kit.Secondary(c, a.installDomain(in)).SingleLine()
					st, tone := a.installStatus(in)
					kit.StatusPill(c, st, tone).Margin(2, 0, 0, 0)
				})
			})
			switch in.State {
			case domain.InstallMissing:
				kit.Note(c, icons.Warning, T("myapps.missing"), kit.ToneWarning)
				kit.Caption(c, T("locate.hint"))
			case domain.InstallExternal:
				kit.Note(c, icons.Lock, T("myapps.readOnly"), kit.ToneNeutral)
			}
			if in.UpdateReason != "" {
				kit.Note(c, icons.Update, T("myapps.updateAvailable"), kit.ToneAccent)
			}
			rows := [][2]string{{T("app.version"), in.Version}}
			mono := map[int]bool{}
			if in.RuntimeVersion != "" {
				rows = append(rows, [2]string{T("app.runtime"), in.RuntimeVersion})
			}
			mono[len(rows)] = true
			rows = append(rows, [2]string{T("app.location"), in.Path})
			mono[len(rows)] = true
			rows = append(rows, [2]string{T("app.identifier"), in.Identifier})
			if in.ProfileDir != "" {
				mono[len(rows)] = true
				rows = append(rows, [2]string{T("app.profile"), in.ProfileDir})
			}
			rows = append(rows, [2]string{T("app.lastManaged"), ago(in.LastManagedAt)})
			kit.KeyValues(c, rows, mono)
			ui.Row(c).Gap(8).Wrap().Justify(ui.End).Children(func() {
				managed := in.State != domain.InstallExternal
				if !managed && in.SourceURL != "" && kit.Button(c.Key("copy"), T("action.createCopy"), kit.ButtonOpts{}).Clicked() {
					u.open = false
					a.create.reset(a.host)
					a.create.setInput(in.SourceURL)
					a.nav(secCreate)
				}
				if in.State == domain.InstallMissing && kit.Button(c.Key("forget"), T("locate.remove"), kit.ButtonOpts{Variant: kit.Ghost}).Clicked() {
					u.open = false
					a.forgetApp(in.ID, in.Name)
				}
				if managed && in.ProjectID != "" && kit.Button(c.Key("repair"), T("action.repair"), kit.ButtonOpts{}).Clicked() {
					u.open = false
					a.dlg.repair = installDialog{open: true, id: in.ID}
				}
				if in.State != domain.InstallMissing && kit.Button(c.Key("reveal"), T("action.reveal"), kit.ButtonOpts{}).Clicked() {
					a.reveal(in.Path)
				}
				if managed && in.UpdateReason != "" && kit.Button(c.Key("upd"), T("action.reviewUpdate"), kit.ButtonOpts{}).Clicked() {
					u.open = false
					a.dlg.openUpdate(in)
				}
				if in.State == domain.InstallMissing {
					if in.ProjectID != "" && kit.Button(c.Key("rebuild"), T("action.rebuild"), kit.ButtonOpts{}).Clicked() {
						u.open = false
						a.startBuild(in.ProjectID, nil)
					}
					if kit.Button(c.Key("locate"), T("action.locate"), kit.ButtonOpts{Variant: kit.Primary, Icon: icons.Folder}).Clicked() {
						a.locateApp(in.ID)
					}
				} else if kit.Button(c.Key("open"), T("action.open"), kit.ButtonOpts{Variant: kit.Primary, Icon: icons.Rocket}).MinWidth(84).Clicked() {
					a.openInstall(in.ID)
				}
			})
			if in.State != domain.InstallExternal {
				kit.Hairline(c)
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					kit.Caption(c, T("uninstall.keepData")).Grow(1).MinWidth(0)
					if kit.Button(c.Key("un"), T("action.uninstall"), kit.ButtonOpts{Icon: icons.Trash, Danger: true}).Clicked() {
						u.open = false
						a.dlg.uninstall = uninstallDialog{open: true, id: in.ID}
					}
				})
			}
		})
	})
}

// locateApp asks where a moved app went and reconnects the library entry to
// it. The service refuses a different app (NOT_SAME_APP).
func (a *App) locateApp(id string) {
	in := a.install(id)
	if in == nil {
		return
	}
	name, start := in.Name, filepath.Dir(in.Path)
	if _, err := os.Stat(start); err != nil {
		start = a.settings.InstallDir
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: T("locate.title"), DefaultPath: start,
			Filters: []mygo.FileFilter{{Name: T("import.filter"), Extensions: []string{"app", "exe", "AppImage"}}}})
		if err != nil || len(paths) == 0 {
			return
		}
		a.relocate(id, name, paths[0])
	}()
}

// relocate reconnects installation id to path (main-thread safe).
func (a *App) relocate(id, name, path string) {
	a.do(func(ctx context.Context) error {
		_, err := a.svc.RelocateInstallation(ctx, id, path, "gui")
		return err
	}, func(err error) {
		switch {
		case errCode(err) == appservice.CodeNotSameApp:
			a.toast(T("locate.notSame", "name", name), errInfo(err).Remediation, 2)
		case err != nil:
			a.toastErr(err)
		default:
			a.toast(T("locate.done", "name", name), "", 1)
		}
	})
}

// forgetApp drops the library entry only; files and website data stay.
func (a *App) forgetApp(id, name string) {
	a.do(func(ctx context.Context) error { return a.svc.ForgetInstallation(ctx, id, "gui") }, func(err error) {
		if err != nil {
			a.toastErr(err)
			return
		}
		a.toast(T("locate.removed", "name", name), "", 1)
	})
}
