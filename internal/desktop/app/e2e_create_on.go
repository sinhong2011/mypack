//go:build e2e

package app

import (
	"os"
	"strings"
)

// Test builds only: MYPACK_E2E_CREATE_SITES="linear.app,prj_0009" adds
// those sites to Create once its first site is ready (native popup menus
// cannot be driven by the automation CLI).
func (a *App) e2eCreateHook() {
	v := os.Getenv("MYPACK_E2E_CREATE_SITES")
	s := &a.create
	if v == "" || a.e2eSeeded || !s.ready() || s.normalized == "" {
		return
	}
	a.e2eSeeded = true
	for _, x := range strings.Split(v, ",") {
		if pr := a.project(x); pr != nil {
			s.more = append(s.more, createSite{url: pr.Config.App.Source.Value, projectID: pr.ID, name: pr.Config.App.Name})
		} else {
			a.addCreateURL(x)
		}
	}
	a.syncCreateGroup()
}
