package app

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
	mpicons "mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/pkg/config"
)

// Groups in Create: an app is one site or several. "Add another site"
// (from the Library, or a pasted address) turns it into a group; the
// preview draws the rail and the inspector's choices (Show as …) apply to
// the whole group. Removing the last added site makes it a single app
// again.

// createSites is the row of the app's sites under its name: a chip per
// site and the Add button.
func (a *App) createSites(c *ui.Context) {
	s := &a.create
	if s.local != "" || s.normalized == "" {
		return // a local project is a single app
	}
	p := design.Of(c)
	ui.Column(c.Key("sites")).AlignItems(ui.Center).Gap(8).Margin(10, 0, 0, 0).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Justify(ui.Center).Wrap().Children(func() {
			if s.isGroup() {
				a.siteChip(c.Key("main"), a.createMainIcon(), s.mainName(), nil)
				for i := range s.more {
					i := i
					site := s.more[i]
					a.siteChip(c.Key("m-"+site.url), a.siteIcon(site), site.name, func() {
						s.more = append(s.more[:i:i], s.more[i+1:]...)
						a.syncCreateGroup()
					})
				}
			}
			if len(s.more)+1 < config.MaxGroupMembers {
				a.addSiteButton(c, p)
			}
		})
		if s.addingAddr {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				f := kit.TextField(c.Key("more-addr"), &s.moreAddr, T("group.addr"), kit.FieldOpts{Placeholder: T("group.addr")}).Width(260).AutoFocus()
				if (f.Submitted() || kit.Button(c.Key("more-add"), T("group.addr.add"), kit.ButtonOpts{Size: design.ControlSM, Variant: kit.Primary,
					Disabled: strings.TrimSpace(s.moreAddr) == ""}).Clicked()) && strings.TrimSpace(s.moreAddr) != "" {
					a.addCreateURL(s.moreAddr)
				}
				if kit.IconButton(c.Key("more-x"), icons.Close, T("common.cancel")).Clicked() {
					s.addingAddr, s.moreAddr = false, ""
				}
			})
		}
		if s.isGroup() && a.license.FreeGroups > 0 && a.license.Groups >= a.license.FreeGroups {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				kit.Glyph(c, icons.Sparkles, 12, p.Accent)
				ui.Text(c, T("group.limit.hint")).FontSize(design.FontCaption).TextColor(p.TextSecondary)
			})
		}
	})
}

// siteChip is a site of the group: its icon and name, × to remove it.
func (a *App) siteChip(c *ui.Context, icon []byte, name string, remove func()) {
	p := design.Of(c)
	chip := ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(4, 10, 4, 4).Radius(999).Background(p.Hover).MaxWidth(180)
	chip.Children(func() {
		kit.AppIcon(c, icon, name, 20)
		ui.Text(c, name).FontSize(design.FontSmall).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine().MinWidth(0).Shrink(1)
		if remove != nil {
			x := ui.ButtonBase(c.Key("x")).Label(T("sites.remove")).Size(16, 16).Center().Radius(8).Cursor(ui.CursorPointer)
			if x.Hovered() {
				x.Background(p.Pressed)
			}
			x.Children(func() { kit.Glyph(c, icons.Close, 10, p.TextSecondary) })
			if x.Clicked() {
				remove()
			}
		}
	})
}

// addSiteButton opens the menu of sites to add: Library apps, or an
// address.
func (a *App) addSiteButton(c *ui.Context, p *design.Palette) {
	s := &a.create
	label := T("create.addSite")
	if s.isGroup() {
		label = T("sites.add")
	}
	btn := ui.ButtonBase(c.Key("add-site")).Label(label).Row().Gap(5).AlignItems(ui.Center).Padding(5, 10).Radius(999).
		Cursor(ui.CursorPointer).Border(1, p.Divider)
	if btn.Hovered() || btn.FocusVisible() {
		btn.Background(p.Hover)
	}
	btn.Children(func() {
		kit.Glyph(c, icons.Plus, 12, p.Accent)
		ui.Text(c, label).FontSize(design.FontSmall).FontWeight(design.WeightMedium).TextColor(p.Accent)
	})
	btn.Menu(func(m *ui.Menu) {
		if m.Item(T("create.addSite.paste")).Chosen() {
			s.addingAddr = true
		}
		have := map[string]bool{s.normalized: true}
		for _, x := range s.more {
			have[x.url] = true
		}
		var cands []*domain.Project
		for _, pr := range a.groupCandidates() {
			if !have[pr.Config.App.Source.Value] {
				cands = append(cands, pr)
			}
		}
		if len(cands) > 0 {
			m.Separator()
			for _, pr := range cands {
				if m.Item(pr.Config.App.Name).Chosen() {
					s.more = append(s.more, createSite{url: pr.Config.App.Source.Value, projectID: pr.ID, name: pr.Config.App.Name})
					a.syncCreateGroup()
				}
			}
		}
	})
}

// addCreateURL adds a pasted address and fetches its name and icon.
func (a *App) addCreateURL(raw string) {
	s := &a.create
	u, err := config.NormalizeURL(strings.TrimSpace(raw))
	if err != nil {
		a.toast(T("group.badURL"), "", 2)
		return
	}
	if u == s.normalized {
		s.addingAddr, s.moreAddr = false, ""
		return
	}
	for _, x := range s.more {
		if x.url == u {
			s.addingAddr, s.moreAddr = false, ""
			return
		}
	}
	s.more = append(s.more, createSite{url: u, name: titleForURL(u)})
	s.addingAddr, s.moreAddr = false, ""
	a.syncCreateGroup()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		md, err := a.svc.FetchMetadata(ctx, u)
		if err != nil {
			return
		}
		a.win.Update(func() {
			if a.siteIcons == nil {
				a.siteIcons = map[string][]byte{}
			}
			if len(md.IconPNG) > 0 {
				a.siteIcons[u] = md.IconPNG
			}
			for i := range s.more {
				if s.more[i].url == u && strings.TrimSpace(md.Title) != "" {
					s.more[i].name = strings.TrimSpace(md.Title)
				}
			}
			a.syncCreateGroup()
		})
	}()
}

func titleForURL(u string) string {
	h := strings.TrimPrefix(config.RegistrableHost(u), "www.")
	if i := strings.IndexByte(h, '.'); i > 0 {
		h = h[:i]
	}
	if h == "" {
		return u
	}
	return strings.ToUpper(h[:1]) + h[1:]
}

func (a *App) siteIcon(site createSite) []byte {
	if pr := a.project(site.projectID); pr != nil {
		return pr.IconPNG
	}
	return a.siteIcons[site.url]
}

// createMainIcon is the first site's own icon (not the group's grid).
func (a *App) createMainIcon() []byte {
	s := &a.create
	if s.meta != nil && s.metaFor == s.normalized {
		return s.meta.IconPNG
	}
	return nil
}

// mainName is the first site's name.
func (s *createState) mainName() string {
	if s.meta != nil && s.meta.Title != "" {
		return s.meta.Title
	}
	return titleForURL(s.normalized)
}

// createGroupIcon is the grid of the group's site icons (cached).
func (a *App) createGroupIcon() []byte {
	s := &a.create
	pngs := [][]byte{a.createMainIcon()}
	key := s.normalized
	for _, x := range s.more {
		ic := a.siteIcon(x)
		pngs = append(pngs, ic)
		key += "|" + x.url + ":" + itoa(len(ic))
	}
	key += ":" + itoa(len(pngs[0]))
	if key != s.groupKey {
		s.groupKey, s.groupIcon = key, mpicons.Group(pngs)
	}
	return s.groupIcon
}

// syncCreateGroup mirrors the sites into the pending config, so the
// preview and the inspector see the group.
func (a *App) syncCreateGroup() {
	s := &a.create
	cfg := a.ensureCfg()
	if !s.isGroup() {
		cfg.App.Group = nil
		return
	}
	taken := map[string]bool{}
	main := config.GroupMember{Name: s.mainName(), URL: s.normalized}
	main.ID = config.MemberID(main.Name, taken)
	taken[main.ID] = true
	g := &config.GroupConfig{Members: []config.GroupMember{main}}
	for _, x := range s.more {
		m := config.GroupMember{Name: x.name, URL: x.url, ProjectID: x.projectID}
		m.ID = config.MemberID(m.Name, taken)
		taken[m.ID] = true
		g.Members = append(g.Members, m)
	}
	cfg.App.Group = g
}

// groupSites is the request's site list: the first site, then the added.
func (s *createState) groupSites() []appservice.GroupSite {
	out := []appservice.GroupSite{{URL: s.normalized}}
	for _, x := range s.more {
		out = append(out, appservice.GroupSite{ProjectID: x.projectID, URL: x.url})
	}
	return out
}
