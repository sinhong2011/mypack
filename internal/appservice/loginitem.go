package appservice

import (
	"context"
	"strconv"

	"mypack.dev/mypack/internal/domain"
)

// LoginItem starts Mypack when the user logs in: a login item on macOS, a
// Run entry of the user on Windows, an XDG autostart entry on Linux. It
// reads and writes the system's own record, never a stored flag, so it
// shows what the system will do, including changes made in System Settings
// or Task Manager. The GUI gives the service one (Options.LoginItem); the
// CLI and the MCP server have none, as only the GUI is a login item.
type LoginItem interface {
	// Enabled reports whether the system starts Mypack at login.
	Enabled() bool
	// SetEnabled asks the system to start Mypack at login, or not.
	SetEnabled(on bool) error
}

// CodeOpenAtLogin: the system did not take the open-at-login change.
const CodeOpenAtLogin = "OPEN_AT_LOGIN_FAILED"

// CodeUnsupported: this front end or platform cannot do what was asked.
const CodeUnsupported = "UNSUPPORTED"

// OpenAtLogin implements Service.
func (s *Impl) OpenAtLogin(ctx context.Context) (bool, error) {
	if s.login == nil {
		return false, errNoLoginItem()
	}
	return s.login.Enabled(), nil
}

// SetOpenAtLogin implements Service. It reads the system's record back, so
// a change the system holds for approval (macOS Login Items) is reported
// as an error rather than shown as done.
func (s *Impl) SetOpenAtLogin(ctx context.Context, on bool, actor string) (err error) {
	defer func() { s.audit(ctx, actor, "settings.openAtLogin", strconv.FormatBool(on), err) }()
	if s.login == nil {
		return errNoLoginItem()
	}
	if err := s.login.SetEnabled(on); err != nil {
		return &domain.Error{Code: CodeOpenAtLogin, Message: loginFailedMsg(on), Cause: err.Error(), Remediation: loginRemedy}
	}
	if s.login.Enabled() != on {
		return newErr(CodeOpenAtLogin, loginFailedMsg(on), loginRemedy)
	}
	s.emit(Event{Kind: EventSettingsChanged})
	return nil
}

const loginRemedy = "Check the login items in your system settings (macOS: System Settings › General › Login Items)."

func loginFailedMsg(on bool) string {
	if on {
		return "Mypack could not be set to open at login."
	}
	return "Mypack could not be stopped from opening at login."
}

func errNoLoginItem() error {
	return newErr(CodeUnsupported, "Open at login is set in the Mypack app's Settings.", "")
}
