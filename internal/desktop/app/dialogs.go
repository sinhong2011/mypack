package app

import (
	"context"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/components"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
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

	licenseLimit      bool // free-edition limit explainer (spec §9E.6)
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
}

type installDialog struct {
	open bool
	id   string
	busy bool
}

type detailsDialog struct {
	open bool
	id   string
}

func (d *dialogs) openUpdate(in *domain.Installation) { d.update = installDialog{open: true, id: in.ID} }

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

// footer draws a dialog footer with Cancel + a primary/danger action.
func footer(c *ui.Context, cancel, action string, v components.Variant, busy, disabled bool) (ok, dismissed bool) {
	ui.Row(c).Gap(8).Justify(ui.End).Margin(8, 0, 0, 0).Children(func() {
		dismissed = components.Button(c.Key("cancel"), cancel, components.ButtonOpts{}).Clicked()
		ok = components.Button(c.Key("ok"), action, components.ButtonOpts{Variant: v, Loading: busy, Disabled: disabled}).Clicked()
	})
	return
}

func (a *App) dialogs(c *ui.Context) {
	p := design.Of(c)
	d := &a.dlg

	// Identity change (spec §9C.2: explain and confirm).
	if d.identity.open {
		id := &d.identity
		lobeui.Modal(c.Key("dlg-identity"), &id.open, T("identity.title"), 480, func() {
			ui.Column(c).Gap(12).Children(func() {
				ui.Text(c, T("identity.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				components.Note(c, lobeui.IconWarning(), T("identity.warning"), components.ToneWarning)
				in := lobeui.Input(c.Key("newid"), &id.newID, lobeui.InputOpts{Placeholder: "app.mypack.example"}).Font("Menlo")
				_ = in
				lobeui.Checkbox(c.Key("np"), &id.newProfile, T("identity.newProfile"))
				ok, cancel := footer(c, T("common.cancel"), T("identity.confirm"), components.Danger, false, id.newID == "" || id.newID == id.ed.cfg.App.Identifier && !id.fromSave)
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
		lobeui.Modal(c.Key("dlg-reset"), &r.open, T("reset.title"), 440, func() {
			ui.Column(c).Gap(12).Children(func() {
				ui.Text(c, T("reset.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				ok, cancel := footer(c, T("common.cancel"), T("reset.confirm"), components.Primary, false, false)
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
			lobeui.Modal(c.Key("dlg-uninstall"), &u.open, T("uninstall.title", "name", in.Name), 460, func() {
				ui.Column(c).Gap(12).Children(func() {
					ui.Text(c, T("uninstall.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
					ui.Column(c).Gap(6).Padding(12).Radius(design.RadiusControl + 1).Background(p.Well).Children(func() {
						lobeui.Checkbox(c.Key("cd"), &u.clearData, T("uninstall.clearData"))
						if u.clearData {
							ui.Text(c, T("uninstall.clearData.warning")).FontSize(design.FontSmall).TextColor(p.Danger).LineHeight(1.4)
						} else {
							components.Caption(c, T("uninstall.keepData"))
						}
					})
					ok, cancel := footer(c, T("common.cancel"), T("action.uninstall"), components.Danger, u.busy, false)
					if cancel {
						u.open = false
					}
					if ok {
						u.busy = true
						id, clear, name := u.id, u.clearData, in.Name
						a.do(func(ctx context.Context) error { return a.svc.Uninstall(ctx, id, clear, "gui") }, func(err error) {
							u.busy, u.open = false, false
							if err != nil {
								a.toastErr(err)
								return
							}
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
			lobeui.Modal(c.Key("dlg-update"), &u.open, T("update.title", "name", in.Name), 480, func() {
				ui.Column(c).Gap(12).Children(func() {
					ui.Text(c, T("update.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
					components.Overline(c, T("update.changing"))
					ui.Column(c).Gap(8).Padding(12).Radius(design.RadiusControl + 1).Background(p.Well).Children(func() {
						row := func(icon *ui.SVG, text string) {
							ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
								ui.Icon(c, icon).FontSize(14).TextColor(p.TextSecondary).Margin(1, 0, 0, 0)
								ui.Text(c, text).FontSize(design.FontBody).TextColor(p.Text).Grow(1).MinWidth(0).LineHeight(1.4)
							})
						}
						row(lobeui.IconUpdate(), in.UpdateReason)
						row(lobeui.IconShield(), T("update.profileKept"))
						row(lobeui.IconRetry(), T("update.rollback"))
					})
					ok, cancel := footer(c, T("action.later"), T("action.update"), components.Primary, u.busy, false)
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
							u.busy, u.open = false, false
							if err != nil {
								a.toastErr(err)
								return
							}
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
			lobeui.Modal(c.Key("dlg-repair"), &u.open, T("repair.title", "name", in.Name), 460, func() {
				ui.Column(c).Gap(12).Children(func() {
					ui.Text(c, T("repair.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
					ok, cancel := footer(c, T("common.cancel"), T("action.repair"), components.Primary, u.busy, false)
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
							u.busy, u.open = false, false
							if err != nil {
								a.toastErr(err)
								return
							}
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
		lobeui.Modal(c.Key("dlg-limit"), &d.licenseLimit, T("license.limit.title"), 460, func() {
			ui.Column(c).Gap(12).Children(func() {
				ui.Text(c, T("license.limit.body", "limit", limit)).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				ui.Text(c, T("license.limit.options")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				ui.Row(c).Gap(8).Justify(ui.End).Margin(8, 0, 0, 0).Children(func() {
					if components.Button(c.Key("later"), T("license.limit.later"), components.ButtonOpts{Variant: components.Ghost}).Clicked() {
						d.licenseLimit = false
					}
					if components.Button(c.Key("manage"), T("license.limit.manage"), components.ButtonOpts{}).Clicked() {
						d.licenseLimit = false
						a.lib.tab, a.ui.LibraryTab = 0, 0
						a.nav(secLibrary)
					}
					if components.Button(c.Key("enter"), T("license.limit.enter"), components.ButtonOpts{Variant: components.Primary}).Clicked() {
						d.licenseLimit = false
						a.nav(secSettings)
					}
				})
			})
		})
	}

	// Deactivate license.
	if d.deactivateLicense.open {
		u := &d.deactivateLicense
		lobeui.Modal(c.Key("dlg-deact"), &u.open, T("license.deactivate.title"), 440, func() {
			ui.Column(c).Gap(12).Children(func() {
				ui.Text(c, T("license.deactivate.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				ok, cancel := footer(c, T("common.cancel"), T("license.deactivate"), components.Primary, u.busy, false)
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
		lobeui.Modal(c.Key("dlg-conflict"), &d.installConflict, T("installConflict.title"), 440, func() {
			ui.Column(c).Gap(12).Children(func() {
				ui.Text(c, T("installConflict.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
				ok, cancel := footer(c, T("common.cancel"), T("installConflict.replace"), components.Danger, false, false)
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
	p := design.Of(c)
	lobeui.Modal(c.Key("dlg-details"), &u.open, "", 520, func() {
		ui.Column(c).Gap(16).Children(func() {
			ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
				components.AppIcon(c, in.IconPNG, in.Name, 56)
				ui.Column(c).Gap(4).Grow(1).MinWidth(0).Children(func() {
					components.Heading(c, in.Name).FontSize(design.FontTitle2)
					st, tone := a.installStatus(in)
					components.StatusPill(c, st, tone)
				})
			})
			switch in.State {
			case domain.InstallMissing:
				components.Note(c, lobeui.IconWarning(), T("myapps.missing"), components.ToneWarning)
				components.Caption(c, T("locate.hint"))
			case domain.InstallExternal:
				components.Note(c, lobeui.IconLock(), T("myapps.readOnly"), components.ToneNeutral)
			}
			if in.UpdateReason != "" {
				components.Note(c, lobeui.IconUpdate(), T("myapps.updateAvailable"), components.ToneAccent)
			}
			ui.Grid(c).ColumnTracks(ui.FitContent(), ui.Fr(1)).GapX(16).GapY(8).Children(func() {
				kv := func(k, v string, mono bool) {
					components.Caption(c, k)
					if mono {
						components.Mono(c, v).MinWidth(0).Selectable()
					} else {
						ui.Text(c, v).FontSize(design.FontSmall).TextColor(p.Text).MinWidth(0)
					}
				}
				kv(T("app.version"), in.Version, false)
				if in.RuntimeVersion != "" {
					kv(T("app.runtime"), in.RuntimeVersion, false)
				}
				kv(T("app.location"), in.Path, true)
				kv(T("app.identifier"), in.Identifier, true)
				if in.ProfileDir != "" {
					kv(T("app.profile"), in.ProfileDir, true)
				}
				kv(T("app.lastManaged"), ago(in.LastManagedAt), false)
			})
			ui.Row(c).Gap(8).Wrap().Children(func() {
				managed := in.State != domain.InstallExternal
				if in.State == domain.InstallMissing {
					if components.Button(c.Key("locate"), T("action.locate"), components.ButtonOpts{Variant: components.Primary, Icon: lobeui.IconFolderOpen()}).Clicked() {
						a.locateApp(in.ID)
					}
					if in.ProjectID != "" && components.Button(c.Key("rebuild"), T("action.rebuild"), components.ButtonOpts{}).Clicked() {
						u.open = false
						a.startBuild(in.ProjectID, nil)
					}
				} else if components.Button(c.Key("open"), T("action.open"), components.ButtonOpts{Variant: components.Primary, Icon: lobeui.IconRocket()}).Clicked() {
					a.openInstall(in.ID)
				}
				if managed && in.UpdateReason != "" && components.Button(c.Key("upd"), T("action.reviewUpdate"), components.ButtonOpts{}).Clicked() {
					u.open = false
					a.dlg.openUpdate(in)
				}
				if managed && in.ProjectID != "" && components.Button(c.Key("repair"), T("action.repair"), components.ButtonOpts{}).Clicked() {
					u.open = false
					a.dlg.repair = installDialog{open: true, id: in.ID}
				}
				if in.State == domain.InstallMissing && components.Button(c.Key("forget"), T("locate.remove"), components.ButtonOpts{Variant: components.Ghost}).Clicked() {
					u.open = false
					a.forgetApp(in.ID, in.Name)
				}
				if in.State != domain.InstallMissing && components.Button(c.Key("reveal"), T("action.reveal"), components.ButtonOpts{}).Clicked() {
					a.reveal(in.Path)
				}
				if !managed && in.SourceURL != "" && components.Button(c.Key("copy"), T("action.createCopy"), components.ButtonOpts{}).Clicked() {
					u.open = false
					a.create.reset(a.host)
					a.create.setInput(in.SourceURL)
					a.nav(secCreate)
				}
			})
			if in.State != domain.InstallExternal {
				components.Hairline(c)
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					components.Caption(c, T("uninstall.keepData")).Grow(1).MinWidth(0)
					if components.Button(c.Key("un"), T("action.uninstall"), components.ButtonOpts{Variant: components.Ghost, Icon: lobeui.IconTrash()}).TextColor(p.Danger).Clicked() {
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
