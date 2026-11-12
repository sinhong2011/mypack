package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/selfupdate"
)

// Software update (Settings → Updates, "Check for Updates…" in the app
// menu). Mypack asks GitHub for the newest release; a copy that can
// replace itself downloads and installs it with MyGo's signed updater, and
// one a package manager installed shows that manager's command instead.

type updPhase int

const (
	updIdle        updPhase = iota // not checked this session
	updChecking                    // asking GitHub
	updCurrent                     // up to date
	updAvailable                   // a newer release exists
	updDownloading                 // downloading and installing it
	updReady                       // installed; runs after a restart
	updFailed                      // the check or the install failed
)

type updateState struct {
	phase    updPhase
	rel      *selfupdate.Release // the newer release, when one was found
	progress float64             // the download's fraction; < 0 unknown
	err      string
	checked  time.Time
	src      selfupdate.Source
	canSelf  bool // MyGo's updater can replace this copy
	cancel   context.CancelFunc
}

// errNoUpdatePackage: GitHub has a newer release, but no signed update for
// this platform (yet), so the app cannot install it itself.
var errNoUpdatePackage = errors.New("no update package for this platform")

// missingUpdateManifest reports a 404 on this platform's update-*.json, as
// when the macOS installer job failed to upload (v0.1.0-beta.4).
func missingUpdateManifest(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "404") && strings.Contains(s, "update-")
}

// startUpdates works out how this copy updates and schedules the
// automatic checks: one shortly after launch, then hourly ticks that check
// when the last check is a day old.
func (a *App) startUpdates() {
	mygo.Updater.SetPrereleases(a.settings.UpdatePrereleases)
	a.upd.src = selfupdate.DetectSource()
	a.upd.canSelf = !a.upd.src.Managed() && mygo.Updater.Enabled()
	time.AfterFunc(8*time.Second, func() { mygo.RunOnMain(a.autoCheckUpdates) })
	go func() {
		for range time.Tick(time.Hour) {
			mygo.RunOnMain(a.autoCheckUpdates)
		}
	}()
}

// autoCheckUpdates checks when automatic checks are on and one is due.
// Development and demo builds never check on their own.
func (a *App) autoCheckUpdates() {
	if !a.settings.CheckUpdates || isFake() || strings.HasSuffix(Version, "-dev") {
		return
	}
	switch a.upd.phase {
	case updChecking, updDownloading, updReady:
		return
	}
	if !selfupdate.Due(a.ui.LastUpdateCheck, time.Now(), selfupdate.CheckInterval) {
		return
	}
	a.checkForUpdates(false)
}

// checkForUpdatesMenu is "Check for Updates…": Settings, and a check.
func (a *App) checkForUpdatesMenu() {
	a.openSettings(paneUpdates)
	a.checkForUpdates(true)
}

// checkForUpdates asks GitHub for the newest release. manual checks come
// from the user, who is looking at the result in Settings.
func (a *App) checkForUpdates(manual bool) {
	switch a.upd.phase {
	case updChecking, updDownloading, updReady:
		return
	}
	a.upd.phase, a.upd.err = updChecking, ""
	pre := a.settings.UpdatePrereleases
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		rel, err := selfupdate.Checker{UserAgent: "Mypack/" + Version}.Latest(ctx, pre)
		mygo.RunOnMain(func() {
			a.finishCheck(rel, err, manual)
			a.redrawUpdates()
		})
	}()
	a.redrawUpdates()
}

func (a *App) finishCheck(rel *selfupdate.Release, err error, manual bool) {
	now := time.Now()
	a.upd.checked = now
	if err != nil {
		a.upd.phase, a.upd.err = updFailed, err.Error()
		return
	}
	a.ui.LastUpdateCheck = now
	a.ui.save()
	if rel == nil || !selfupdate.IsNewer(rel.Version, Version) {
		a.upd.phase, a.upd.rel = updCurrent, nil
		return
	}
	a.upd.phase, a.upd.rel = updAvailable, rel
	// A check of its own tells of each new version once, in a toast,
	// unless Settings, which shows it, is in front.
	if !manual && a.ui.UpdateNotified != rel.Version && !(a.curPane() == paneUpdates && a.setWin != nil && a.setWin.IsVisible()) {
		a.ui.UpdateNotified = rel.Version
		a.ui.save()
		a.toastAction(T("update.toast", "version", rel.Version), T("update.toast.view"), func() {
			a.openSettings(paneUpdates)
		})
	}
}

// installUpdate downloads and installs the update with MyGo's updater,
// which checks its signature; it runs after a restart.
func (a *App) installUpdate() {
	if a.upd.phase != updAvailable && a.upd.phase != updFailed {
		return
	}
	a.upd.phase, a.upd.progress, a.upd.err = updDownloading, -1, ""
	ctx, cancel := context.WithCancel(context.Background())
	a.upd.cancel = cancel
	go func() {
		up, err := mygo.Updater.Check(ctx)
		if err == nil && up == nil {
			err = errNoUpdatePackage
		}
		if missingUpdateManifest(err) {
			err = errNoUpdatePackage
		}
		if err == nil {
			last := -1
			err = up.Install(ctx, func(done, total int64) {
				if total <= 0 {
					return
				}
				pct := int(done * 100 / total)
				if pct == last {
					return
				}
				last = pct
				mygo.RunOnMain(func() {
					if a.upd.phase == updDownloading {
						a.upd.progress = float64(pct) / 100
						a.redrawUpdates()
					}
				})
			})
		}
		mygo.RunOnMain(func() {
			canceled := ctx.Err() != nil
			cancel()
			a.upd.cancel = nil
			switch {
			case canceled:
				a.upd.phase = updAvailable
			case errors.Is(err, errNoUpdatePackage):
				a.upd.phase, a.upd.err = updFailed, T("update.err.noPackage")
			case err != nil:
				a.upd.phase, a.upd.err = updFailed, err.Error()
			default:
				a.upd.phase = updReady
			}
			a.redrawUpdates()
		})
	}()
}

func (a *App) cancelUpdate() {
	if a.upd.cancel != nil {
		a.upd.cancel()
	}
}

// openRelease opens the release's page, or the releases list.
func (a *App) openRelease() {
	u := "https://github.com/" + selfupdate.Repo + "/releases/latest"
	if a.upd.rel != nil && a.upd.rel.URL != "" {
		u = a.upd.rel.URL
	}
	a.do(func(ctx context.Context) error { return a.svc.OpenURL(ctx, u) }, a.toastErr)
}

// managerName is how the UI names the package manager.
func managerName(m selfupdate.Manager) string {
	if m == selfupdate.ManagerLinuxPackage {
		return T("update.manager.linux")
	}
	return string(m)
}

// ---- Settings → Updates ---------------------------------------------------------

func (a *App) updatesGroup(c *ui.Context) {
	u := &a.upd
	kit.Group(c, "", func() { // the pane is titled Updates
		a.updateStatus(c)
		if u.phase == updAvailable || (u.phase == updFailed && u.rel != nil) || u.phase == updDownloading {
			a.updateDetails(c)
		}
		kit.SettingIconRow(c.Key("auto"), icons.Refresh, ui.Color{}, T("settings.updates"), T("settings.updates.desc"), func() {
			v := a.settings.CheckUpdates
			kit.Switch(c.Key("cu"), &v, T("settings.updates"))
			if v != a.settings.CheckUpdates {
				a.settings.CheckUpdates = v
				a.saveSettings()
			}
		})
		kit.SettingIconRow(c.Key("pre"), icons.Layers, ui.Color{}, T("update.prereleases"), T("update.prereleases.desc"), func() {
			v := a.settings.UpdatePrereleases
			kit.Switch(c.Key("pre-sw"), &v, T("update.prereleases"))
			if v != a.settings.UpdatePrereleases {
				a.settings.UpdatePrereleases = v
				mygo.Updater.SetPrereleases(v)
				a.saveSettings()
				if u.phase == updCurrent || u.phase == updAvailable || u.phase == updFailed {
					a.checkForUpdates(true)
				}
			}
		})
	})
}

// updateStatus is the group's first row: where updating stands, and the
// one action that moves it on.
func (a *App) updateStatus(c *ui.Context) {
	p := design.Of(c)
	u := &a.upd
	var title, desc string
	tone := kit.ToneNeutral
	switch u.phase {
	case updIdle:
		title, desc = T("update.idle"), T("update.current.desc", "version", Version)
	case updChecking:
		title, desc = T("update.checking"), T("update.current.desc", "version", Version)
	case updCurrent:
		title, desc, tone = T("update.current"), T("update.checked", "version", Version, "time", u.checked.Format("15:04")), kit.ToneSuccess
	case updAvailable:
		title, desc, tone = T("update.available", "version", u.rel.Version), T("update.available.desc", "version", Version), kit.ToneAccent
		if u.src.Managed() {
			desc = T("update.managed.desc", "manager", managerName(u.src.Manager))
		}
	case updDownloading:
		title, desc, tone = T("update.downloading", "version", u.rel.Version), T("update.downloading.desc"), kit.ToneAccent
	case updReady:
		title, desc, tone = T("update.ready", "version", u.rel.Version), T("update.ready.desc"), kit.ToneSuccess
	case updFailed:
		title, desc, tone = T("update.failed"), u.err, kit.ToneDanger
	}
	ui.Row(c.Key("status")).MinHeight(design.Row+4).Padding(10, 14).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Size(20, 20).Center().Shrink(0).Margin(1, 0, 0, 0).Children(func() {
			if u.phase == updChecking {
				kit.Spinner(c, 14, p.TextSecondary)
				return
			}
			col := p.TextSecondary
			if tone != kit.ToneNeutral {
				col, _ = kit.ToneColors(p, tone)
			}
			ic := icons.Update
			switch tone {
			case kit.ToneSuccess:
				ic = icons.Success
			case kit.ToneDanger:
				ic = icons.Alert
			}
			kit.Glyph(c, ic, 16, col)
		})
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
			kit.Roll(ui.Text(c, title).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text))
			if desc != "" {
				ui.Text(c, desc).FontSize(design.FontSmall).TextColor(p.TextSecondary).LineHeight(1.35).Selectable()
			}
		})
		ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() { a.updateActions(c) })
	}).Role(ui.RoleStatus)
}

// updateActions is the status row's buttons: one primary at most.
func (a *App) updateActions(c *ui.Context) {
	u := &a.upd
	sm := design.ControlSM
	switch u.phase {
	case updIdle, updCurrent:
		if kit.Button(c.Key("check"), T("update.check"), kit.ButtonOpts{Size: sm}).Clicked() {
			a.checkForUpdates(true)
		}
	case updChecking:
		kit.Button(c.Key("check"), T("update.check"), kit.ButtonOpts{Size: sm, Loading: true, Disabled: true})
	case updAvailable:
		switch {
		case u.canSelf:
			if kit.Button(c.Key("install"), T("update.install"), kit.ButtonOpts{Variant: kit.Primary, Size: sm, Icon: icons.Download}).Clicked() {
				a.installUpdate()
			}
		case !u.src.Managed():
			// A copy that cannot replace itself (no signed update for
			// it, or no write access where it is): the download page.
			if kit.Button(c.Key("dl"), T("update.download"), kit.ButtonOpts{Variant: kit.Primary, Size: sm, Trailing: icons.External}).Clicked() {
				a.openRelease()
			}
		}
	case updDownloading:
		if kit.Button(c.Key("cancel"), T("common.cancel"), kit.ButtonOpts{Size: sm}).Clicked() {
			a.cancelUpdate()
		}
	case updReady:
		if kit.Button(c.Key("restart"), T("update.restart"), kit.ButtonOpts{Variant: kit.Primary, Size: sm}).Clicked() {
			mygo.App.Relaunch()
		}
	case updFailed:
		if u.rel != nil && u.err == T("update.err.noPackage") {
			if kit.Button(c.Key("dl"), T("update.download"), kit.ButtonOpts{Variant: kit.Primary, Size: sm, Trailing: icons.External}).Clicked() {
				a.openRelease()
			}
			return
		}
		if u.rel != nil && u.canSelf {
			if kit.Button(c.Key("retry"), T("update.retry"), kit.ButtonOpts{Size: sm, Icon: icons.Retry}).Clicked() {
				a.installUpdate()
			}
			return
		}
		if kit.Button(c.Key("retry"), T("update.retry"), kit.ButtonOpts{Size: sm, Icon: icons.Retry}).Clicked() {
			a.upd.phase = updIdle
			a.checkForUpdates(true)
		}
	}
}

// updateDetails is the found release: progress, what is new, and for a
// package manager's copy the command that updates it.
func (a *App) updateDetails(c *ui.Context) {
	p := design.Of(c)
	u := &a.upd
	kit.Reveal(ui.Column(c.Key("details")).Padding(12, 14).Gap(10).Children(func() {
		if u.phase == updDownloading {
			kit.ProgressBar(c, u.progress).Label(T("update.downloading", "version", u.rel.Version))
		}
		if u.rel == nil {
			return
		}
		if lines := selfupdate.Summary(u.rel.Notes, 5); len(lines) > 0 {
			ui.Column(c).Gap(3).Children(func() {
				for i, l := range lines {
					ui.Text(c.Key(i), l).FontSize(design.FontSmall).TextColor(p.TextSecondary).LineHeight(1.4).Selectable()
				}
			})
		}
		if u.src.Managed() && u.phase == updAvailable {
			if cmd := u.src.Command; cmd != "" {
				ui.Column(c).Gap(6).Children(func() {
					kit.Caption(c, T("update.managed.command", "manager", managerName(u.src.Manager)))
					ui.Row(c).Gap(8).Padding(8, 12).Radius(design.RadiusCard).Background(p.Well).Border(0.5, p.GroupBorder).AlignItems(ui.Center).Children(func() {
						ui.Text(c, cmd).Font(kit.MonoFont).FontSize(11.5).TextColor(p.Text).Grow(1).MinWidth(0).Selectable()
						if kit.CopyButton(c.Key("cp"), cmd, T("common.copy")) {
							a.toast(T("common.copied"), "", 1)
						}
					})
				})
			}
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if kit.Button(c.Key("notes"), T("update.notes"), kit.ButtonOpts{Variant: kit.Plain, Size: design.ControlSM, Trailing: icons.External}).Clicked() {
				a.openRelease()
			}
			if u.rel.Prerelease {
				kit.StatusPill(c, T("update.prerelease"), kit.ToneWarning)
			}
		})
	}))
}

// updateBadge is the sidebar's Settings count: 1 while an update waits.
func (a *App) updateBadge() string {
	switch a.upd.phase {
	case updAvailable, updReady:
		return "1"
	}
	return ""
}

// redrawUpdates redraws where updating shows: the main window's sidebar
// badge and Settings' Updates pane.
func (a *App) redrawUpdates() {
	if a.win != nil {
		a.win.Invalidate()
	}
	a.redrawSettings()
}
