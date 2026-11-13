package app

import (
	"context"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
)

// LoginItem is this MyPack's entry in the system's login items, for
// appservice.Options.LoginItem: MyGo's login item (SMAppService, or a launch
// agent before macOS 13), the user's Run key on Windows and an XDG autostart
// entry on Linux. Call it only once the GUI runs (app.Run).
func LoginItem() appservice.LoginItem { return mygoLogin{} }

type mygoLogin struct{}

func (mygoLogin) Enabled() bool            { return mygo.App.OpenAtLogin() }
func (mygoLogin) SetEnabled(on bool) error { return mygo.App.SetOpenAtLogin(on) }

// loginState is the Open at login switch: what the system said last.
type loginState struct {
	known bool // read at least once; the row is hidden until then
	on    bool
	busy  bool // a change is on its way to the system
}

// refreshLogin reads whether the system opens MyPack at login. Settings
// reads it as it opens and comes forward, so a change made in System
// Settings or Task Manager shows.
func (a *App) refreshLogin() {
	var on bool
	a.do(func(ctx context.Context) (err error) {
		on, err = a.svc.OpenAtLogin(ctx)
		return err
	}, func(err error) {
		if a.set.login.busy {
			return // the change in flight reads it again
		}
		a.set.login.known, a.set.login.on = err == nil, on
		a.redrawSettings()
	})
}

// setOpenAtLogin asks the system for the change, then shows what it did.
func (a *App) setOpenAtLogin(on bool) {
	l := &a.set.login
	l.on, l.busy = on, true
	a.do(func(ctx context.Context) error {
		return a.svc.SetOpenAtLogin(ctx, on, "gui")
	}, func(err error) {
		l.busy = false
		if err != nil {
			a.toast(T("settings.openAtLogin.failed"), errInfo(err).Remediation, 2)
		}
		a.refreshLogin()
	})
	a.redrawSettings()
}

// openAtLoginRow is the switch in Settings › General.
func (a *App) openAtLoginRow(c *ui.Context) {
	l := &a.set.login
	if !l.known {
		return
	}
	kit.SettingIconRow(c, icons.Power, ui.Color{}, T("settings.openAtLogin"), T("settings.openAtLogin.desc"), func() {
		v := l.on
		kit.Switch(c.Key("login"), &v, T("settings.openAtLogin"))
		if v != l.on && !l.busy {
			a.setOpenAtLogin(v)
		}
	})
}
