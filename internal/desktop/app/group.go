package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

// New Group: one app made of several websites, shown side by side in a
// rail, each with its own logins (app.group). The sheet has two parts:
// the group itself (its sites in rail order, which can be renamed,
// reordered and removed) and the sites to add from (pasted addresses,
// local folders, Library apps). A site can be added more than once: each
// copy is a member with its own session, so several accounts of one site
// sit side by side.

// groupPick is one site of the group being made.
type groupPick struct {
	id   int    // keys the row
	src  string // "p:<project id>", "u:<address>" or "l:<path>"
	name string // the name the user gave it ("" = automatic)
}

type groupDialog struct {
	open   bool
	name   string
	picked []groupPick // in rail order
	urls   []string    // addresses pasted into the sheet (stay listed when removed)
	locals []string    // folders and HTML files chosen in the sheet
	titles map[string]string
	addr   string // the address field
	busy   bool
	err    string
	nextID int
}

// add puts src at the end of the group; false when the group is full.
func (d *groupDialog) add(src string) bool {
	if len(d.picked) >= config.MaxGroupMembers {
		d.err = T("group.full", "n", config.MaxGroupMembers)
		return false
	}
	d.nextID++
	d.picked = append(d.picked, groupPick{id: d.nextID, src: src})
	d.err = ""
	return true
}

// count is how many times src is in the group.
func (d *groupDialog) count(src string) int {
	n := 0
	for _, p := range d.picked {
		if p.src == src {
			n++
		}
	}
	return n
}

// nth is which copy of its site pick i is (1 for the first).
func (d *groupDialog) nth(i int) int {
	n := 0
	for j := 0; j <= i; j++ {
		if d.picked[j].src == d.picked[i].src {
			n++
		}
	}
	return n
}

func (d *groupDialog) remove(i int) {
	d.picked = append(d.picked[:i:i], d.picked[i+1:]...)
	d.err = ""
}

func (d *groupDialog) move(i, by int) {
	j := i + by
	if j < 0 || j >= len(d.picked) {
		return
	}
	d.picked[i], d.picked[j] = d.picked[j], d.picked[i]
}

// addURL lists a pasted address (normalized) and adds it to the group; it
// returns the address, "" when it isn't one.
func (d *groupDialog) addURL(raw string) string {
	u, err := config.NormalizeURL(strings.TrimSpace(raw))
	if err != nil {
		d.err = T("group.badURL")
		return ""
	}
	if !slices.Contains(d.urls, u) {
		d.urls = append(d.urls, u)
	}
	d.addr = ""
	d.add("u:" + u)
	return u
}

// addLocal lists a chosen folder or HTML file and adds it to the group.
func (d *groupDialog) addLocal(path string) {
	if !slices.Contains(d.locals, path) {
		d.locals = append(d.locals, path)
	}
	d.add("l:" + path)
}

// newGroup opens the New Group sheet.
func (a *App) newGroup() { a.newGroupWith("") }

// newGroupWith opens the New Group sheet with an address already in it
// (from New App).
func (a *App) newGroupWith(addr string) {
	a.showWindow()
	a.dlg.group = groupDialog{open: true, name: T("group.defaultName")}
	if strings.TrimSpace(addr) != "" {
		a.pasteGroupURL(addr)
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

// pasteGroupURL adds a pasted address to the sheet's group and fetches its
// name and icon, so the row reads as the site rather than a bare address.
func (a *App) pasteGroupURL(raw string) {
	d := &a.dlg.group
	u := d.addURL(raw)
	if u == "" || (d.titles[u] != "" && len(a.siteIcons[u]) > 0) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		md, err := a.svc.FetchMetadata(ctx, u)
		if err != nil {
			return
		}
		a.win.Update(func() {
			if d.titles == nil {
				d.titles = map[string]string{}
			}
			if t := strings.TrimSpace(md.Title); t != "" {
				d.titles[u] = t
			}
			if len(md.IconPNG) > 0 {
				if a.siteIcons == nil {
					a.siteIcons = map[string][]byte{}
				}
				a.siteIcons[u] = md.IconPNG
			}
		})
	}()
}

// groupCandidates are the Library's apps that can join a group.
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

// groupSrc is how the sheet shows a site: its name, the line under it and
// its icon.
type groupSrc struct {
	name, sub string
	icon      []byte
	globe     bool // no icon yet: a pasted address's globe
}

func (a *App) groupSource(src string) groupSrc {
	d := &a.dlg.group
	switch {
	case strings.HasPrefix(src, "p:"):
		if pr := a.project(src[2:]); pr != nil && pr.Config != nil {
			return groupSrc{name: pr.Config.App.Name, sub: addressLabel(pr.Config.App.Source, pr.Config.App.Name), icon: pr.IconPNG}
		}
		return groupSrc{name: "?", globe: true}
	case strings.HasPrefix(src, "u:"):
		u := src[2:]
		name := d.titles[u]
		if name == "" {
			name = titleForURL(u)
		}
		ic := a.siteIcons[u]
		return groupSrc{name: name, sub: hostLabel(u), icon: ic, globe: len(ic) == 0}
	default:
		l := strings.TrimPrefix(src, "l:")
		return groupSrc{name: localName(l), sub: shortPath(l), icon: a.letterIcon(localName(l))}
	}
}

// pickName is the name pick i gets: the one typed, else its site's name,
// numbered from the second copy of a site on ("ChatGPT 2").
func (a *App) pickName(i int) string {
	d := &a.dlg.group
	if n := strings.TrimSpace(d.picked[i].name); n != "" {
		return n
	}
	name := a.groupSource(d.picked[i].src).name
	if n := d.nth(i); n > 1 {
		name += " " + itoa(n)
	}
	return name
}

func (a *App) groupSheet(c *ui.Context) {
	d := &a.dlg.group
	if !d.open {
		return
	}
	p := design.Of(c)
	kit.Sheet(c.Key("dlg-group"), &d.open, 540, func() {
		kit.SheetHeader(c, T("group.title"), nil)
		ui.Column(c).Gap(14).Children(func() {
			ui.Text(c, T("group.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45)
			kit.TextField(c.Key("g-name"), &d.name, T("group.name"), kit.FieldOpts{Placeholder: T("group.defaultName")})
			a.groupPicked(c, p)
			a.groupSources(c, p)
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

// sectionTitle is a small heading in the sheet, with a note on the right.
func sectionTitle(c *ui.Context, p *design.Palette, title, note string) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(0, 2).Children(func() {
		ui.Text(c, title).FontSize(design.FontSmall).FontWeight(design.WeightSemibold).TextColor(p.Text).Role(ui.RoleHeading).Grow(1)
		if note != "" {
			ui.Text(c, note).FontSize(design.FontCaption).TextColor(p.TextSecondary).FontFeatures("tnum")
		}
	})
}

// groupPicked is the group as it will be: its sites in rail order, each
// numbered; hovering a row shows the buttons to move or remove it. A site
// that is in more than once gets a name field, to tell its accounts apart.
func (a *App) groupPicked(c *ui.Context, p *design.Palette) {
	d := &a.dlg.group
	ui.Column(c.Key("g-picked")).Gap(6).Children(func() {
		sectionTitle(c, p, T("group.inGroup"), T("group.count", "n", len(d.picked), "max", config.MaxGroupMembers))
		if len(d.picked) == 0 {
			ui.Row(c).Padding(14, 12).Radius(design.RadiusControl).Border(1, p.Divider).Justify(ui.Center).Children(func() {
				ui.Text(c, T("group.empty")).FontSize(design.FontSmall).TextColor(p.TextSecondary).TextAlign(ui.Center)
			})
			return
		}
		kit.GroupBox(c).Children(func() {
			ui.Scroll(c).MaxHeight(232).Children(func() {
				ui.Column(c).Dividers(0.5, p.Divider).Children(func() {
					for i := range d.picked {
						a.pickedRow(c.Key("pk-"+itoa(d.picked[i].id)), p, i)
					}
				})
			})
		})
	})
}

func (a *App) pickedRow(c *ui.Context, p *design.Palette, i int) {
	d := &a.dlg.group
	pk := &d.picked[i]
	src := a.groupSource(pk.src)
	repeated := d.count(pk.src) > 1
	row := ui.Row(c).MinHeight(design.Row+4).Padding(6, 10, 6, 10).Gap(10).AlignItems(ui.Center)
	show := row.Hovered() || row.FocusWithin()
	row.Children(func() {
		kit.Pop(ui.Row(c.Key("n")).Size(20, 20).Radius(10).Center().Shrink(0).Background(p.Accent).Children(func() {
			ui.Text(c, itoa(i+1)).FontSize(11).FontWeight(design.WeightSemibold).TextColor(p.TextOnAccent).FontFeatures("tnum")
		}))
		groupIcon(c, p, src, 28)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
			if repeated {
				// Several accounts of one site: name each ("Work", "Home").
				auto := src.name
				if n := d.nth(i); n > 1 {
					auto += " " + itoa(n)
				}
				kit.TextField(c.Key("name"), &pk.name, T("group.accountName"), kit.FieldOpts{Placeholder: auto}).Height(26)
			} else {
				ui.Text(c, a.pickName(i)).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine()
			}
			sub := src.sub
			if repeated {
				sub += " · " + T("group.ownLogin")
			}
			ui.Text(c, sub).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine()
		})
		// A fixed slot, so the name fields line up whichever buttons show.
		ui.Row(c).Width(84).Justify(ui.End).Gap(0).Shrink(0).Opacity(map[bool]float32{true: 1, false: 0.35}[show]).Children(func() {
			if i > 0 && kit.IconButton(c.Key("up"), icons.ArrowUp, T("sites.up")).Clicked() && !d.busy {
				d.move(i, -1)
			}
			if i < len(d.picked)-1 && kit.IconButton(c.Key("down"), icons.ChevronDown, T("sites.down")).Clicked() && !d.busy {
				d.move(i, +1)
			}
			if kit.IconButton(c.Key("rm"), icons.Close, T("sites.remove")).Clicked() && !d.busy {
				d.remove(i)
			}
		})
	})
}

// groupSources lists what can be added: an address field, a folder or
// HTML file, and the Library's apps. A row adds its site each time it is
// clicked, so the same site can be in the group for several accounts.
func (a *App) groupSources(c *ui.Context, p *design.Palette) {
	d := &a.dlg.group
	full := len(d.picked) >= config.MaxGroupMembers
	ui.Column(c.Key("g-sources")).Gap(8).Children(func() {
		sectionTitle(c, p, T("group.addSites"), "")
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			f := kit.TextField(c.Key("g-addr"), &d.addr, T("group.addr"), kit.FieldOpts{Placeholder: T("group.addr")}).Grow(1)
			empty := strings.TrimSpace(d.addr) == ""
			if (f.Submitted() || kit.Button(c.Key("g-addr-add"), T("group.addr.add"), kit.ButtonOpts{Disabled: empty || full}).Clicked()) &&
				!empty && !d.busy {
				a.pasteGroupURL(d.addr)
			}
			folder := kit.Button(c.Key("g-local"), "", kit.ButtonOpts{Icon: icons.Folder, Tooltip: T("create.addSite.local"), Disabled: full}).
				Label(T("create.addSite.local"))
			folder.Menu(func(m *ui.Menu) {
				if m.Item(T("create.addSite.folder")).Chosen() {
					a.pickLocalSite(true, d.addLocal)
				}
				if m.Item(T("create.addSite.file")).Chosen() {
					a.pickLocalSite(false, d.addLocal)
				}
			})
		})
		var srcs []string
		for _, l := range d.locals {
			srcs = append(srcs, "l:"+l)
		}
		for _, u := range d.urls {
			srcs = append(srcs, "u:"+u)
		}
		for _, pr := range a.groupCandidates() {
			srcs = append(srcs, "p:"+pr.ID)
		}
		if len(srcs) == 0 {
			return
		}
		ui.Scroll(c).MaxHeight(220).Children(func() {
			ui.Column(c).Gap(2).Padding(0, 10, 0, 0).Children(func() { // room for the scroll bar
				for _, src := range srcs {
					if a.sourceRow(c.Key("src-"+src), p, src, full) && !d.busy {
						d.add(src)
					}
				}
			})
		})
		ui.Text(c, T("group.accountsHint")).FontSize(design.FontCaption).TextColor(p.TextSecondary).LineHeight(1.4)
	})
}

// sourceRow is a site that can be added: its icon and name, and on the
// right "Add", or once it is in the group how many times and "Add another
// account". It reports a click.
func (a *App) sourceRow(c *ui.Context, p *design.Palette, src string, full bool) bool {
	d := &a.dlg.group
	s := a.groupSource(src)
	n := d.count(src)
	action := T("group.addSite")
	if n > 0 {
		action = T("group.addAccount")
	}
	row := ui.ButtonBase(c).Row().Justify(ui.Start).Gap(10).AlignItems(ui.Center).Padding(6, 8, 6, 8).Radius(design.RadiusControl).
		Label(s.name + ", " + action).Cursor(ui.CursorPointer).Transition(design.Transition(c)).Disabled(full)
	hov := row.Hovered() || row.FocusVisible()
	if hov && !full {
		row.Background(p.Hover)
	}
	row.Children(func() {
		groupIcon(c, p, s, 28)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, s.name).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
			ui.Text(c, s.sub).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine()
		})
		if n > 0 {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Padding(2, 8).Radius(999).Background(p.Accent.Mix(p.Overlay, 0.82)).Shrink(0).Children(func() {
				kit.Glyph(c, icons.Check, 10, p.Accent)
				ui.Text(c, T("group.added", "n", n)).FontSize(design.FontCaption).FontWeight(design.WeightMedium).TextColor(p.Accent).FontFeatures("tnum")
			})
		}
		fg := p.TextSecondary
		if hov && !full {
			fg = p.Accent
		}
		ui.Row(c).Gap(4).AlignItems(ui.Center).Shrink(0).Opacity(map[bool]float32{true: 0.4, false: 1}[full]).Children(func() {
			kit.Glyph(c, icons.Plus, 12, fg)
			ui.Text(c, action).FontSize(design.FontSmall).FontWeight(design.WeightMedium).TextColor(fg)
		})
	})
	return row.Clicked() && !full
}

// groupIcon is a site's icon in the sheet; a pasted address shows a globe
// until its icon comes.
func groupIcon(c *ui.Context, p *design.Palette, s groupSrc, size float32) {
	if s.globe {
		ui.Row(c).Size(size, size).Center().Shrink(0).Radius(size / 4).Background(p.Hover).Children(func() {
			kit.Glyph(c, icons.Globe, size*0.54, p.TextSecondary)
		})
		return
	}
	kit.AppIcon(c, s.icon, s.name, size)
}

// createGroup makes the group project and opens its settings.
func (a *App) createGroup() {
	d := &a.dlg.group
	d.busy, d.err = true, ""
	req := appservice.GroupRequest{Name: d.name, Actor: "gui"}
	for i, pk := range d.picked {
		site := appservice.GroupSite{Name: strings.TrimSpace(pk.name)}
		if d.count(pk.src) > 1 {
			site.Name = a.pickName(i) // as the sheet shows it ("ChatGPT 3")
		}
		if id, ok := strings.CutPrefix(pk.src, "p:"); ok {
			site.ProjectID = id
		} else if u, ok := strings.CutPrefix(pk.src, "u:"); ok {
			site.URL = u
		} else if l, ok := strings.CutPrefix(pk.src, "l:"); ok {
			site.Path = l
		}
		req.Sites = append(req.Sites, site)
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
	} else if b := a.memberIconFile(m.Icon); len(b) > 0 {
		icon = b // an address member: the icon fetched when it joined
	} else if m.IsLocal() {
		icon = a.letterIcon(m.Name)
	}
	row := ui.Row(c).MinHeight(design.Row+4).Padding(8, 14).Gap(12).AlignItems(ui.Center)
	show := row.Hovered() || row.FocusWithin()
	row.Children(func() {
		kit.AppIcon(c, icon, m.Name, 28)
		repeated := false
		for j, o := range g.Members {
			if j != i && o.URL == m.URL && o.Local == m.Local {
				repeated = true
			}
		}
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(1).Children(func() {
			if repeated {
				// Accounts of one site: a name tells them apart in the rail.
				in := kit.TextField(c.Key("name"), &g.Members[i].Name, T("group.accountName"), kit.FieldOpts{Placeholder: T("group.accountName")}).Height(26)
				if in.Changed() && strings.TrimSpace(g.Members[i].Name) != "" {
					changed()
				}
			} else {
				ui.Text(c, m.Name).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine()
			}
			sub := memberLabel(m)
			if repeated {
				sub += " · " + T("group.ownLogin")
			}
			ui.Text(c, sub).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine()
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
	taken := map[string]bool{}
	for _, m := range g.Members {
		taken[m.ID] = true
	}
	// A site already in the group can come again: another account of it.
	cands := a.groupCandidates()
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
		name := config.RepeatName(localName(path), g.Members)
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
				m := memberOfProject(pr, nil)
				m.Name = config.RepeatName(m.Name, g.Members)
				m.ID = config.MemberID(m.Name, taken)
				g.Members = append(g.Members, m)
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

// memberIconFile is the icon a member keeps beside the database (cached).
func (a *App) memberIconFile(path string) []byte {
	if path == "" || !filepath.IsAbs(path) {
		return nil
	}
	if a.siteIcons == nil {
		a.siteIcons = map[string][]byte{}
	}
	k := "file:" + path
	if b, ok := a.siteIcons[k]; ok {
		return b
	}
	b, _ := os.ReadFile(path)
	a.siteIcons[k] = b
	return b
}
