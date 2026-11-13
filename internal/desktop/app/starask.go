package app

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"
)

// Asking for a star: a toast, now and then, after a build went well; at
// most twice, and never again once the repo is starred from it or About.

// starAsk is what the asking remembers across restarts.
type starAsk struct {
	Builds int       `json:"builds,omitempty"` // successful builds the user watched
	Asked  int       `json:"asked,omitempty"`  // times the toast showed
	Last   time.Time `json:"last,omitempty"`   // when it last showed
	Done   bool      `json:"done,omitempty"`   // starred: never ask again
}

const (
	starFirstAt  = 3                   // the first ask, after this many good builds
	starSecondAt = 12                  // the second, after this many
	starGap      = 14 * 24 * time.Hour // and at least this long after the first
	starMaxAsks  = 2
)

// due reports whether a good build now earns the ask.
func (s starAsk) due(now time.Time) bool {
	switch {
	case s.Done || s.Asked >= starMaxAsks:
		return false
	case s.Asked == 0:
		return s.Builds >= starFirstAt
	default:
		return s.Builds >= starSecondAt && now.Sub(s.Last) >= starGap
	}
}

// starAfterBuild counts a good build the user watched, and asks for a
// star a moment later when it is due: after the build's own toast and
// the app opening, and only while MyPack is in front to be seen.
func (a *App) starAfterBuild() {
	a.ui.Star.Builds++
	a.ui.save()
	if !a.ui.Star.due(time.Now()) {
		return
	}
	time.AfterFunc(3*time.Second, func() {
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			if !a.win.IsFocused() || !a.ui.Star.due(time.Now()) {
				return // try again after a later build
			}
			a.ui.Star.Asked++
			a.ui.Star.Last = time.Now()
			a.ui.save()
			a.toasts = append(a.toasts, toastReq{toast: &ui.Toast{
				ID:          "star",
				Title:       T("star.toast.title"),
				Description: T("star.toast.body"),
				Type:        "star",
				Timeout:     12 * time.Second,
				Action:      T("star.toast.action"),
				OnAction:    a.starRepo,
			}})
			a.win.Invalidate()
		})
	})
}

// starRepo opens the repo to star it, and stops the asking.
func (a *App) starRepo() {
	a.ui.Star.Done = true
	a.ui.save()
	a.do(func(ctx context.Context) error { return a.svc.OpenURL(ctx, aboutRepoURL) }, a.toastErr)
}
