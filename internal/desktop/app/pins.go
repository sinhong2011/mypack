package app

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/domain"
)

// Pinned apps sit in the sidebar, one click from any page. A pin is keyed
// by the project (so a draft stays pinned once it is installed), or by
// the installation for an app with no project (an imported one).

// installPinKey is the pin key of an installed app.
func installPinKey(in *domain.Installation) string {
	if in.ProjectID != "" {
		return in.ProjectID
	}
	return in.ID
}

func (u *uiState) isPinned(key string) bool { return slices.Contains(u.Pinned, key) }

// togglePin pins key at the end of the list, or unpins it.
func (u *uiState) togglePin(key string) {
	if i := slices.Index(u.Pinned, key); i >= 0 {
		u.Pinned = slices.Delete(u.Pinned, i, i+1)
	} else {
		u.Pinned = append(u.Pinned, key)
	}
	u.save()
}

// pinMenuItem is the Pin to Sidebar / Unpin item of an app's menus.
func (a *App) pinMenuItem(m *ui.Menu, key string) {
	label := T("action.pin")
	if a.ui.isPinned(key) {
		label = T("action.unpin")
	}
	if m.Item(label).Chosen() {
		a.ui.togglePin(key)
		a.syncTray() // its Pinned list
	}
}

// pinnedItem is one pinned app as the sidebar shows it: installed, or a
// project not installed yet.
type pinnedItem struct {
	key     string
	install *domain.Installation
	project *domain.Project
}

func (p pinnedItem) name() string {
	if p.install != nil {
		return p.install.Name
	}
	return p.project.Config.App.Name
}

func (p pinnedItem) icon() []byte {
	if p.install != nil {
		return p.install.IconPNG
	}
	return p.project.IconPNG
}

// pinnedItems is the pinned apps in pin order; pins whose app is gone are
// skipped (and kept, should it come back from sync).
func (a *App) pinnedItems() []pinnedItem {
	var out []pinnedItem
	for _, key := range a.ui.Pinned {
		it := pinnedItem{key: key}
		for i := range a.installs {
			if installPinKey(&a.installs[i]) == key {
				it.install = &a.installs[i]
				break
			}
		}
		if it.install == nil {
			it.project = a.project(key)
		} else if it.install.ProjectID != "" {
			it.project = a.project(it.install.ProjectID)
		}
		if it.install != nil || it.project != nil {
			out = append(out, it)
		}
	}
	return out
}

// sidebarPinned draws the sidebar's Pinned list: a click opens the app
// (or, not installed yet, its project); right-click has its full menu.
func (a *App) sidebarPinned(c *ui.Context, items []pinnedItem) {
	p := design.Of(c)
	kit.Eyebrow(c, T("nav.pinned")).Padding(20, 10, 8, 10)
	ui.Column(c).Gap(1).Role(ui.RoleList).Label(T("nav.pinned")).Children(func() {
		for _, it := range items {
			it := it
			active := it.install == nil && a.section == secProject && a.projectID == it.key
			row := ui.ButtonBase(c.Key("pin-" + it.key)).Label(it.name()).Row().Justify(ui.Start).Height(30).PaddingX(8).Gap(9).
				AlignItems(ui.Center).Cursor(ui.CursorPointer)
			kit.NavRow(c, row, active)
			if it.install != nil {
				in := *it.install
				row.ContextMenu(func(m *ui.Menu) { a.appMenu(m, in) })
				st, _ := a.installStatus(&in)
				row.Description(st)
			} else {
				pr := *it.project
				row.ContextMenu(func(m *ui.Menu) { a.projectMenu(m, &pr) })
			}
			kbd := row.FocusVisible()
			row.Children(func() {
				kit.FocusMark(c, kbd, 10)
				if active {
					a.navPill(c)
				}
				kit.AppIconOpts(c, it.icon(), it.name(), 18, kit.IconOpts{})
				fg := p.TextSecondary
				if active || row.Hovered() {
					fg = p.Text
				}
				ui.Text(c, it.name()).FontSize(design.FontBody).TextColor(fg).SingleLine().MinWidth(0).Grow(1)
				switch {
				case it.project != nil && a.building(it.project.ID):
					kit.BuildingSpinner(c, 12, p.Accent)
				case it.install != nil:
					// A dot when the app needs something: an update, or
					// it went missing.
					if _, tone := a.installStatus(it.install); tone == kit.ToneAccent || tone == kit.ToneWarning || tone == kit.ToneDanger {
						fg, _ := kit.ToneColors(p, tone)
						ui.Box(c).Size(6, 6).Radius(3).Background(fg).Shrink(0).AccessHidden(true)
					}
				}
			})
			if row.Clicked() {
				switch {
				case it.install != nil && it.install.State == domain.InstallMissing:
					a.dlg.details = detailsDialog{open: true, id: it.install.ID}
				case it.install != nil:
					a.openInstall(it.install.ID)
				default:
					a.openProject(it.project.ID, 0)
				}
			}
		}
	})
}

// building reports whether a build of project id is queued or running.
func (a *App) building(id string) bool {
	j := a.latestJob(id)
	return j != nil && (j.Status == domain.JobRunning || j.Status == domain.JobQueued)
}
