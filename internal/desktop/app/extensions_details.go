package app

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/catalog"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/runtime/userscript"
)

// The details sheet: everything about one extension, and where it is
// managed (on/off, which apps it runs in, update, remove). It opens from an
// installed row or a hub card; a hub entry that is not installed shows what
// the hub says about it and Install.

type extDetail struct {
	open          bool
	id            string // installed entry id ("" when not installed)
	hubURL        string // hub entry URL ("" when not from the hub)
	confirmRemove bool
	pickApps      bool // "Selected apps" is chosen, even before an app is ticked
}

func (a *App) openExtDetail(id, hubURL string) {
	a.ext.detail = extDetail{open: true, id: id, hubURL: hubURL}
	if e := a.installedByID(id); e != nil && len(e.Apps) > 0 {
		a.ext.detail.pickApps = true
	}
}

func (a *App) installedByID(id string) *userscript.Entry {
	if id == "" {
		return nil
	}
	for i := range a.ext.installed {
		if a.ext.installed[i].ID == id {
			return &a.ext.installed[i]
		}
	}
	return nil
}

func (a *App) hubByURL(u string) *catalog.Script {
	for i := range a.ext.hub {
		if a.ext.hub[i].URL == u {
			return &a.ext.hub[i]
		}
	}
	return nil
}

// hubFor finds the hub entry an installed script came from.
func (a *App) hubFor(e userscript.Entry) *catalog.Script {
	for i, s := range a.ext.hub {
		if s.URL == e.SourceURL || strings.EqualFold(s.Name, e.Name) {
			return &a.ext.hub[i]
		}
	}
	return nil
}

// extApp is an app the scope picker offers.
type extApp struct {
	id, name string
	icon     []byte
}

// scopeApps lists the Library's apps by identifier, plus identifiers in
// the scope that are no longer in the Library (so they can be unticked).
func (a *App) scopeApps(scope []string) []extApp {
	var out []extApp
	seen := map[string]bool{}
	for _, pr := range a.projects {
		id := pr.Config.App.Identifier
		if id == "" || seen[strings.ToLower(id)] {
			continue
		}
		seen[strings.ToLower(id)] = true
		out = append(out, extApp{id: id, name: pr.Config.App.Name, icon: pr.IconPNG})
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].name) < strings.ToLower(out[j].name) })
	for _, id := range scope {
		if !seen[strings.ToLower(id)] {
			out = append(out, extApp{id: id, name: id})
		}
	}
	return out
}

func (a *App) setExtApps(id string, apps []string) {
	if _, err := a.extStore().SetApps(id, apps); err != nil {
		a.toastErr(err)
	}
	a.reloadInstalledExtensions()
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// grantLines says in words what a script may use.
func grantLines(m userscript.Meta) []string {
	g := m.Grants()
	var can []string
	if g["values"] {
		can = append(can, T("ext.grant.values"))
	}
	if g["setclipboard"] {
		can = append(can, T("ext.grant.clipboard"))
	}
	if g["xmlhttprequest"] {
		to := strings.Join(m.Connect, ", ")
		if to == "" {
			to = T("ext.grant.self")
		}
		if len(m.Connect) == 1 && m.Connect[0] == "*" {
			to = T("ext.sites.all")
		}
		can = append(can, T("ext.grant.requests", "hosts", to))
	}
	if len(can) == 0 {
		can = append(can, T("ext.grant.none"))
	}
	return can
}

func (a *App) extDetailSheet(c *ui.Context) {
	d := &a.ext.detail
	if !d.open {
		return
	}
	e := a.installedByID(d.id)
	hub := a.hubByURL(d.hubURL)
	if e != nil && hub == nil {
		hub = a.hubFor(*e)
	}
	if e == nil && hub == nil {
		d.open = false // removed meanwhile
		return
	}
	var m userscript.Meta
	if e != nil {
		m = a.ext.metas[e.ID]
	}
	name, desc := m.Name, m.Description
	if name == "" && e != nil {
		name = e.Name
	}
	if hub != nil {
		if name == "" {
			name = hub.Name
		}
		if desc == "" {
			desc = hub.Description
		}
	}
	p := design.Of(c)
	kit.Sheet(c.Key("ext-detail"), &d.open, 560, func() {
		// Header: icon, name, version · author, status.
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			kit.Placeholder(c, name, 48)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, name).FontSize(design.FontTitle3).FontWeight(design.WeightBold).TextColor(p.Text).Role(ui.RoleHeading).
					LetterSpacing(design.TrackTitle).SingleLine()
				var sub []string
				if m.Version != "" {
					sub = append(sub, T("ext.detail.version", "v", m.Version))
				}
				author := m.Author
				if author == "" && hub != nil {
					author = hub.Author
				}
				if author != "" {
					sub = append(sub, author)
				}
				if len(sub) > 0 {
					ui.Text(c, strings.Join(sub, " · ")).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine()
				}
			})
			// Whether it is on is the switch below; no pill repeats it.
		})
		// What it is and where it comes from stay in view, above the
		// scrolling part (the sheet's focus scrolls to its first switch).
		if desc != "" {
			kit.SheetBody(c, desc)
		}
		if e != nil && e.Edited {
			kit.Note(c, icons.Edit, T("ext.detail.edited"), kit.ToneAccent)
		} else if hub != nil && (e == nil || a.extVerified(*e)) {
			kit.Note(c, icons.Shield, T("ext.review.reviewed"), kit.ToneNeutral)
		} else if hub != nil {
			kit.Note(c, icons.Refresh, T("ext.detail.behindHub"), kit.ToneAccent)
		} else if e != nil {
			kit.Note(c, icons.Warning, T("ext.detail.unreviewed"), kit.ToneWarning)
		}
		ui.Scroll(c).MaxHeight(440).Children(func() {
			ui.Column(c).Gap(14).Padding(0, 2, 2, 2).Children(func() {
				if e != nil {
					a.extManageGroup(c, e)
				}
				a.extFactsGroup(c, e, hub, m)
			})
		})
		// Footer actions.
		ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
			if e != nil {
				if d.confirmRemove {
					ui.Text(c, T("ext.detail.removeAsk")).FontSize(design.FontSmall).TextColor(p.TextSecondary).Grow(1).MinWidth(0)
					if kit.Button(c.Key("rm-no"), T("common.cancel"), kit.ButtonOpts{}).Clicked() {
						d.confirmRemove = false
					}
					if kit.Button(c.Key("rm-yes"), T("ext.remove"), kit.ButtonOpts{Variant: kit.Danger}).Clicked() {
						d.open = false
						a.removeExtension(*e)
					}
					return
				}
				if kit.Button(c.Key("rm"), T("ext.remove"), kit.ButtonOpts{Icon: icons.Trash, Danger: true}).Clicked() {
					d.confirmRemove = true
				}
				ui.Spacer(c)
				if e.SourceURL != "" || hub != nil {
					if kit.Button(c.Key("upd"), T("ext.detail.checkUpdate"), kit.ButtonOpts{Icon: icons.Refresh, Loading: a.extBusy(e.ID)}).Clicked() {
						d.open = false
						a.updateExtension(*e)
					}
				}
				if kit.Button(c.Key("done"), T("ext.detail.done"), kit.ButtonOpts{Variant: kit.Primary}).MinWidth(84).Clicked() {
					d.open = false
				}
				return
			}
			ui.Spacer(c)
			if kit.Button(c.Key("close"), T("common.close"), kit.ButtonOpts{}).Clicked() {
				d.open = false
			}
			if kit.Button(c.Key("get"), T("ext.install"), kit.ButtonOpts{Variant: kit.Primary, Loading: a.extBusy(hub.URL)}).MinWidth(84).Clicked() {
				d.open = false
				a.fetchForReview(hub.URL, hub.URL, hub.SHA256, true)
			}
		})
	})
}

// extManageGroup: on/off and the apps it runs in.
func (a *App) extManageGroup(c *ui.Context, e *userscript.Entry) {
	d := &a.ext.detail
	kit.Group(c, T("ext.detail.manage"), func() {
		kit.SettingRow(c, T("ext.detail.enabled"), T("ext.detail.enabled.desc"), func() {
			on := e.Enabled
			kit.Switch(c.Key("on"), &on, T("ext.enabled", "name", e.Name))
			if on != e.Enabled {
				a.setExtEnabled(e.ID, on)
			}
		})
		kit.SettingBlock(c, T("ext.detail.apps"), T("ext.detail.apps.desc"), func() {
			sel := 0
			if d.pickApps || len(e.Apps) > 0 {
				sel = 1
			}
			old := sel
			kit.Segmented(c.Key("scope"), &sel, []string{T("ext.scope.all"), T("ext.detail.apps.some")}, true)
			if sel != old {
				if sel == 0 {
					d.pickApps = false
					a.setExtApps(e.ID, nil)
				} else {
					d.pickApps = true
				}
			}
			if sel == 0 {
				return
			}
			apps := a.scopeApps(e.Apps)
			if len(apps) == 0 {
				kit.Caption(c, T("ext.detail.apps.none"))
				return
			}
			if len(e.Apps) == 0 {
				kit.Caption(c, T("ext.detail.apps.pick"))
			}
			ui.Column(c.Key("apps")).Gap(6).Margin(6, 0, 0, 0).Children(func() {
				for _, ap := range apps {
					ap := ap
					ui.Row(c.Key(ap.id)).Gap(8).AlignItems(ui.Center).Children(func() {
						checked := containsFold(e.Apps, ap.id)
						was := checked
						kit.CheckboxLead(c.Key("cb"), &checked, ap.name, func() {
							kit.AppIconOpts(c, ap.icon, ap.name, 18, kit.IconOpts{})
						}).Grow(1).MinWidth(0)
						ui.Text(c, ap.id).FontSize(design.FontCaption).TextColor(design.Of(c).TextTertiary).SingleLine()
						if checked != was {
							var next []string
							for _, x := range e.Apps {
								if !strings.EqualFold(x, ap.id) {
									next = append(next, x)
								}
							}
							if checked {
								next = append(next, ap.id)
							}
							if len(next) == 0 {
								// Unticking the last app would mean "all apps";
								// keep the choice open instead.
								d.pickApps = true
							}
							a.setExtApps(e.ID, next)
						}
					})
				}
			})
		})
	})
}

// extFactsGroup: where it runs, what it may do, where it came from.
func (a *App) extFactsGroup(c *ui.Context, e *userscript.Entry, hub *catalog.Script, m userscript.Meta) {
	var rows [][2]string
	sites := hub != nil && e == nil
	if e != nil {
		rows = append(rows, [2]string{T("ext.review.runsOn"), sitesLabel(m.Sites())})
		if len(m.Exclude)+len(m.ExcludeMatch) > 0 {
			rows = append(rows, [2]string{T("ext.detail.except"), strings.Join(append(append([]string{}, m.Exclude...), m.ExcludeMatch...), "\n")})
		}
		rows = append(rows, [2]string{T("ext.review.can"), strings.Join(grantLines(m), "\n")})
		if m.RunAt != "" {
			rows = append(rows, [2]string{T("ext.detail.runAt"), runAtLabel(m.RunAt)})
		}
	} else if sites {
		rows = append(rows, [2]string{T("ext.review.runsOn"), strings.Join(hub.Sites, ", ")})
		rows = append(rows, [2]string{T("ext.review.can"), T("ext.detail.canOnInstall")})
	}
	if e != nil && len(e.Requires) > 0 {
		rows = append(rows, [2]string{T("ext.review.requires"), strings.Join(m.Require, "\n")})
	}
	if e != nil && len(e.Resources) > 0 {
		var rs []string
		for _, r := range e.Resources {
			rs = append(rs, r.Name+" ("+r.MIME+")")
		}
		rows = append(rows, [2]string{T("ext.detail.resources"), strings.Join(rs, "\n")})
	}
	src := ""
	switch {
	case e != nil && e.SourceURL != "":
		src = e.SourceURL
	case e != nil:
		src = T("ext.detail.fromFile")
	case hub != nil:
		src = hub.URL
	}
	if src != "" {
		rows = append(rows, [2]string{T("ext.review.source"), src})
	}
	if hub != nil {
		rows = append(rows, [2]string{T("ext.detail.sha"), hub.SHA256})
	}
	if e != nil {
		rows = append(rows, [2]string{T("ext.detail.installed"), e.Installed.Local().Format("2006-01-02 15:04")})
		if !e.Updated.Equal(e.Installed) {
			rows = append(rows, [2]string{T("ext.detail.updated"), e.Updated.Local().Format("2006-01-02 15:04")})
		}
		rows = append(rows, [2]string{T("ext.detail.id"), e.ID})
	}
	kit.Group(c, T("ext.detail.about"), func() {
		kit.KeyValues(c, rows, map[int]bool{})
	})
	// Links: homepage, and the script on disk.
	ui.Row(c).Gap(8).Wrap().Children(func() {
		home := m.Homepage
		if home == "" && hub != nil {
			home = hub.Homepage
		}
		if home != "" {
			if kit.Button(c.Key("home"), T("ext.detail.homepage"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM, Icon: icons.External}).Clicked() {
				a.do(func(ctx context.Context) error { return a.svc.OpenURL(ctx, home) }, a.toastErr)
			}
		}
		if e != nil {
			if kit.Button(c.Key("code"), T("ext.detail.editCode"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM, Icon: icons.Code}).Clicked() {
				a.openExtEditor(e.ID)
			}
			if kit.Button(c.Key("reveal"), T("ext.detail.reveal"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM, Icon: icons.Folder}).Clicked() {
				a.reveal(filepath.Join(userscript.Dir(), e.ID, "script.user.js"))
			}
		}
	})
}

func runAtLabel(s string) string {
	switch s {
	case "document-start":
		return T("ext.runAt.start")
	case "document-end":
		return T("ext.runAt.end")
	}
	return T("ext.runAt.idle")
}

// extMatches reports whether an extension matches the page's search.
func extMatches(q string, fields ...string) bool {
	q = strings.TrimSpace(strings.ToLower(q))
	if q == "" {
		return true
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}
