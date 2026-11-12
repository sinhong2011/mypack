package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// New Group: one app made of several website apps of the Library, shown
// side by side in a rail, each with its own logins (app.group).

type groupDialog struct {
	open   bool
	name   string
	picked []string // "p:<project id>" or "u:<address>", in rail order
	urls   []string // addresses pasted into the sheet (stay listed when unticked)
	locals []string // folders and HTML files chosen in the sheet
	addr   string   // the address field
	busy   bool
	err    string
}

// addURL adds a pasted address (normalized) and ticks it.
func (d *groupDialog) addURL(raw string) bool {
	u, err := config.NormalizeURL(strings.TrimSpace(raw))
	if err != nil {
		d.err = T("group.badURL")
		return false
	}
	if !slices.Contains(d.urls, u) {
		d.urls = append(d.urls, u)
	}
	if !d.has("u:"+u) && len(d.picked) < config.MaxGroupMembers {
		d.picked = append(d.picked, "u:"+u)
	}
	d.addr, d.err = "", ""
	return true
}

func (d *groupDialog) has(id string) bool {
	for _, p := range d.picked {
		if p == id {
			return true
		}
	}
	return false
}

func (d *groupDialog) toggle(id string) {
	for i, p := range d.picked {
		if p == id {
			d.picked = append(d.picked[:i], d.picked[i+1:]...)
			return
		}
	}
	d.picked = append(d.picked, id)
}

// newGroup opens the New Group sheet.
func (a *App) newGroup() { a.newGroupWith("") }

// newGroupWith opens the New Group sheet with an address already in it
// (from New App).
func (a *App) newGroupWith(addr string) {
	a.showWindow()
	a.dlg.group = groupDialog{open: true, name: T("group.defaultName")}
	if strings.TrimSpace(addr) != "" {
		a.dlg.group.addURL(addr)
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

// groupCandidates are the Library's website apps that can join a group.
func (a *App) groupCandidates() []*domain.Project {
	var out []*domain.Project
	for i := range a.projects {
		pr := &a.projects[i]
		if pr.Config != nil && !pr.Config.IsGroup() && pr.TrashedAt == nil {
			out = append(out, pr)
		}
	}
	return out
}

func (a *App) groupSheet(c *ui.Context) {
	d := &a.dlg.group
	if !d.open {
		return
	}
	p := design.Of(c)
	kit.Sheet(c.Key("dlg-group"), &d.open, 500, func() {
		kit.SheetHeader(c, T("group.title"), nil)
		ui.Column(c).Gap(12).Children(func() {
			ui.Text(c, T("group.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
			kit.TextField(c.Key("g-name"), &d.name, T("group.name"), kit.FieldOpts{Placeholder: T("group.defaultName")})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				f := kit.TextField(c.Key("g-addr"), &d.addr, T("group.addr"), kit.FieldOpts{Placeholder: T("group.addr")}).Grow(1)
				if f.Submitted() && strings.TrimSpace(d.addr) != "" {
					d.addURL(d.addr)
				}
				if kit.Button(c.Key("g-addr-add"), T("group.addr.add"), kit.ButtonOpts{Disabled: strings.TrimSpace(d.addr) == ""}).Clicked() {
					d.addURL(d.addr)
				}
				folder := kit.Button(c.Key("g-local"), "", kit.ButtonOpts{Icon: icons.Folder, Tooltip: T("create.addSite.local")}).Label(T("create.addSite.local"))
				folder.Menu(func(m *ui.Menu) {
					add := func(path string) {
						if !slices.Contains(d.locals, path) {
							d.locals = append(d.locals, path)
						}
						if !d.has("l:"+path) && len(d.picked) < config.MaxGroupMembers {
							d.picked = append(d.picked, "l:"+path)
						}
					}
					if m.Item(T("create.addSite.folder")).Chosen() {
						a.pickLocalSite(true, add)
					}
					if m.Item(T("create.addSite.file")).Chosen() {
						a.pickLocalSite(false, add)
					}
				})
			})
			cands := a.groupCandidates()
			ui.Scroll(c).MaxHeight(300).Children(func() {
				ui.Column(c).Gap(2).Padding(0, 10, 0, 0).Children(func() { // room for the scroll bar
					for _, l := range d.locals {
						key := "l:" + l
						on := d.has(key)
						l := l
						clicked := pickRow(c.Key("g-"+key), localName(l), shortPath(l), on, indexOf(d.picked, key)+1, func() {
							kit.AppIcon(c, a.letterIcon(localName(l)), localName(l), 28)
						})
						if clicked && !d.busy && (on || len(d.picked) < config.MaxGroupMembers) {
							d.toggle(key)
						}
					}
					for _, u := range d.urls {
						key := "u:" + u
						on := d.has(key)
						clicked := pickRow(c.Key("g-"+key), hostLabel(u), T("group.pasted"), on, indexOf(d.picked, key)+1, func() {
							ui.Row(c).Size(28, 28).Center().Shrink(0).Radius(7).Background(p.Hover).Children(func() { kit.Glyph(c, icons.Globe, 15, p.TextSecondary) })
						})
						if clicked && !d.busy && (on || len(d.picked) < config.MaxGroupMembers) {
							d.toggle(key)
						}
					}
					for _, pr := range cands {
						pr := pr
						key := "p:" + pr.ID
						on := d.has(key)
						clicked := pickRow(c.Key("g-"+pr.ID), pr.Config.App.Name, addressLabel(pr.Config.App.Source, pr.Config.App.Name), on,
							indexOf(d.picked, key)+1, func() { kit.AppIcon(c, pr.IconPNG, pr.Config.App.Name, 28) })
						if clicked && !d.busy {
							if on || len(d.picked) < config.MaxGroupMembers {
								d.toggle(key)
								d.err = ""
							}
						}
					}
				})
			})
			if len(d.picked) > 0 {
				ui.Text(c, T("group.order")).FontSize(design.FontCaption).TextColor(p.TextSecondary)
			}
			if d.err != "" {
				ui.Text(c, d.err).FontSize(design.FontCaption).TextColor(p.Danger)
			}
			ok, cancel := kit.SheetFooter(c, T("common.cancel"), T("group.create"), kit.Primary, d.busy,
				len(d.picked) < config.MinGroupMembers || strings.TrimSpace(d.name) == "")
			if cancel {
				d.open = false
			}
			if ok {
				a.createGroup()
			}
		})
	})
}

// pickRow is a row of the New Group list: the site, and on the right an
// empty ring, or once picked a filled accent circle with its place in the
// rail. Rows only tint on hover, so a long pick stays calm to read.
func pickRow(c *ui.Context, title, sub string, on bool, order int, icon func()) bool {
	p := design.Of(c)
	row := ui.ButtonBase(c).Row().Justify(ui.Start).Gap(10).AlignItems(ui.Center).Padding(6, 10, 6, 8).Radius(design.RadiusControl).
		Role(ui.RoleCheckBox).Label(title).Checked(on).Cursor(ui.CursorPointer).Transition(design.Transition(c))
	if row.Hovered() {
		row.Background(p.Hover)
	}
	row.Children(func() {
		icon()
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
			ui.Text(c, sub).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine()
		})
		mark := ui.Row(c).Size(22, 22).Radius(11).Center().Shrink(0)
		if on {
			mark.Background(p.Accent).Children(func() {
				ui.Text(c, itoa(order)).FontSize(11.5).FontWeight(design.WeightSemibold).TextColor(p.TextOnAccent).FontFeatures("tnum")
			})
			kit.Pop(mark)
		} else {
			mark.Border(1.5, p.BorderStrong)
		}
	})
	return row.Clicked()
}

// createGroup makes the group project and opens its settings.
func (a *App) createGroup() {
	d := &a.dlg.group
	d.busy, d.err = true, ""
	req := appservice.GroupRequest{Name: d.name, Actor: "gui"}
	for _, k := range d.picked {
		if id, ok := strings.CutPrefix(k, "p:"); ok {
			req.Sites = append(req.Sites, appservice.GroupSite{ProjectID: id})
		} else if u, ok := strings.CutPrefix(k, "u:"); ok {
			req.Sites = append(req.Sites, appservice.GroupSite{URL: u})
		} else if l, ok := strings.CutPrefix(k, "l:"); ok {
			req.Sites = append(req.Sites, appservice.GroupSite{Path: l})
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		pr, err := a.svc.CreateGroup(ctx, req)
		a.win.Update(func() {
			d.busy = false
			if err != nil {
				var de *domain.Error
				if errors.As(err, &de) {
					d.err = de.Message
				} else {
					d.err = err.Error()
				}
				return
			}
			d.open = false
			a.reload("projects")
			a.openProject(pr.ID, 1)
			a.toast(T("group.created"), pr.Config.App.Name, 1)
		})
	}()
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}

// sitesCard is the Sites card at the top of a group's settings: its
// websites in rail order, each with its icon; hovering a row shows the
// buttons to move or remove it; the last row adds one from the Library.
func (a *App) sitesCard(c *ui.Context, cfg *config.Config, changed func()) {
	g := cfg.App.Group
	if g == nil || !a.settingHit(T("sites.title"), T("sites.add")) {
		return
	}
	p := design.Of(c)
	ui.Column(c.Key("sites")).Gap(8).Children(func() {
		ui.Text(c, T("sites.title")).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).Padding(0, 2).Role(ui.RoleHeading)
		kit.GroupBox(c).Children(func() {
			ui.Column(c).Dividers(0.5, p.Divider).Children(func() {
				for i := range g.Members {
					a.siteRow(c.Key("site-"+g.Members[i].ID), cfg, i, changed)
				}
				if len(g.Members) < config.MaxGroupMembers {
					a.addSiteRow(c.Key("site-add"), cfg, changed)
				}
			})
		})
	})
}

func (a *App) siteRow(c *ui.Context, cfg *config.Config, i int, changed func()) {
	p := design.Of(c)
	g := cfg.App.Group
	m := g.Members[i]
	var icon []byte
	if pr := a.project(m.ProjectID); pr != nil {
		icon = pr.IconPNG
	} else if m.IsLocal() {
		icon = a.letterIcon(m.Name)
	}
	row := ui.Row(c).MinHeight(design.Row+4).Padding(8, 14).Gap(12).AlignItems(ui.Center)
	show := row.Hovered() || row.FocusWithin()
	row.Children(func() {
		kit.AppIcon(c, icon, m.Name, 28)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, m.Name).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine()
			ui.Text(c, memberLabel(m)).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine()
		})
		ui.Row(c).Gap(0).Shrink(0).Opacity(map[bool]float32{true: 1, false: 0}[show]).Children(func() {
			if i > 0 && kit.IconButton(c.Key("up"), icons.ArrowUp, T("sites.up")).Clicked() {
				g.Members[i-1], g.Members[i] = g.Members[i], g.Members[i-1]
				syncGroupSource(cfg)
				changed()
			}
			if i < len(g.Members)-1 && kit.IconButton(c.Key("down"), icons.ChevronDown, T("sites.down")).Clicked() {
				g.Members[i+1], g.Members[i] = g.Members[i], g.Members[i+1]
				syncGroupSource(cfg)
				changed()
			}
			if len(g.Members) > config.MinGroupMembers && kit.IconButton(c.Key("rm"), icons.Close, T("sites.remove")).Clicked() {
				g.Members = append(g.Members[:i:i], g.Members[i+1:]...)
				syncGroupSource(cfg)
				changed()
			}
		})
	})
}

func (a *App) addSiteRow(c *ui.Context, cfg *config.Config, changed func()) {
	p := design.Of(c)
	g := cfg.App.Group
	in := map[string]bool{}
	taken := map[string]bool{}
	for _, m := range g.Members {
		in[m.URL], in[m.Local], taken[m.ID] = true, true, true
	}
	var cands []*domain.Project
	for _, pr := range a.groupCandidates() {
		if !in[pr.Config.App.Source.Value] {
			cands = append(cands, pr)
		}
	}
	btn := ui.ButtonBase(c).Label(T("sites.add")).Row().MinHeight(design.Row+4).Padding(8, 14).Gap(12).AlignItems(ui.Center).
		Justify(ui.Start).Cursor(ui.CursorPointer).FocusRing(false)
	if btn.Hovered() || btn.FocusVisible() {
		btn.Background(p.Hover)
	}
	btn.Children(func() {
		ui.Row(c).Size(28, 28).Center().Shrink(0).Radius(8).Border(1, p.Divider).Children(func() { kit.Glyph(c, icons.Plus, 14, p.Accent) })
		ui.Text(c, T("sites.add")).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Accent)
	})
	addLocal := func(path string) {
		for _, m := range g.Members {
			if m.Local == path {
				return
			}
		}
		name := localName(path)
		g.Members = append(g.Members, config.GroupMember{ID: config.MemberID(name, taken), Name: name, Local: path})
		changed()
	}
	btn.Menu(func(m *ui.Menu) {
		if m.Item(T("create.addSite.folder")).Chosen() {
			a.pickLocalSite(true, addLocal)
		}
		if m.Item(T("create.addSite.file")).Chosen() {
			a.pickLocalSite(false, addLocal)
		}
		if len(cands) > 0 {
			m.Separator()
		}
		for _, pr := range cands {
			if m.Item(pr.Config.App.Name).Chosen() {
				g.Members = append(g.Members, memberOfProject(pr, taken))
				changed()
			}
		}
	})
}

// syncGroupSource keeps the group's source on its first website.
func syncGroupSource(cfg *config.Config) {
	if g := cfg.App.Group; g != nil && len(g.Members) > 0 {
		cfg.App.Source.Value = g.Members[0].URL
	}
}

// memberLabel is a member's address, or its folder.
func memberLabel(m config.GroupMember) string {
	if m.IsLocal() {
		return shortPath(m.Local)
	}
	return hostLabel(m.URL)
}

func hostLabel(raw string) string {
	return addressLabel(config.Source{Type: config.SourceURL, Value: raw}, "")
}

// memberOfProject is the group member a Library app becomes: its address,
// or its folder.
func memberOfProject(pr *domain.Project, taken map[string]bool) config.GroupMember {
	c := pr.Config
	m := config.GroupMember{ID: config.MemberID(c.App.Name, taken), Name: c.App.Name, ProjectID: pr.ID}
	if c.App.Source.Type == config.SourceURL {
		m.URL = c.App.Source.Value
	} else {
		m.Local, m.Entry = c.App.Source.Value, c.App.Source.Entry
	}
	return m
}
