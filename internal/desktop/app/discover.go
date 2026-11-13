package app

import (
	"context"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/catalog"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/desktop/sampleicons"
)

// template is a declarative quick-start preset from the Discover catalog
// (catalog/discover.json): a name, an address and a category, nothing that
// runs. Choosing one only prefills Create.
type template struct {
	name, url, category string
}

// templates is the Discover list. It starts as the catalog built into this
// binary and is replaced on the UI thread by a newer one (loadCatalog).
var templates = fromCatalog(catalog.Bundled())

func fromCatalog(c catalog.Catalog) []template {
	out := make([]template, 0, len(c.Apps))
	for _, a := range c.Apps {
		out = append(out, template{a.Name, a.URL, a.Category})
	}
	return out
}

// loadCatalog fetches a newer catalog in the background (cached for a day;
// offline, the bundled list stays).
func (a *App) loadCatalog() {
	go func() {
		c := catalog.Latest(context.Background())
		if a.win == nil {
			return
		}
		a.win.Update(func() { templates = fromCatalog(c) })
	}()
}

// refreshCatalog fetches the catalog now, skipping the day-long cache (the
// Discover toolbar's refresh button). A failed fetch keeps the list shown.
func (a *App) refreshCatalog() {
	if a.catalogBusy {
		return
	}
	a.catalogBusy = true
	go func() {
		c, err := catalog.Fetch(context.Background())
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			a.catalogBusy = false
			if err != nil {
				a.toast(T("discover.refresh.failed"), "", 2)
				return
			}
			templates = fromCatalog(c)
			a.toast(T("discover.refreshed"), "", 1)
		})
	}()
}

// discoverToolbar holds the refresh button, a spinner while it fetches.
func (a *App) discoverToolbar(c *ui.Context) {
	if a.catalogBusy {
		ui.Box(c.Key("refresh")).Size(design.ToolbarControl, design.ToolbarControl).Center().Shrink(0).
			Role(ui.RoleProgress).Label(T("discover.refreshing")).Children(func() {
			kit.Spinner(c, 16, design.Of(c).TextSecondary)
		})
		return
	}
	if kit.ToolbarIcon(c.Key("refresh"), icons.Refresh, T("discover.refresh")).Clicked() {
		a.refreshCatalog()
	}
}

// templateIcon is the neutral generated icon a preset shows.
func templateIcon(name string) []byte { return sampleicons.Icon(name) }

func (a *App) discoverView(c *ui.Context) {
	page(c, func() {
		// The bar names the page and counts it, as on every page; the body
		// opens with one plain line, as Runtimes does.
		kit.Secondary(c, T("discover.body")).LineHeight(1.45)
		// One shelf per category, in the catalog's order.
		n := 0
		for si, cat := range catalog.Categories {
			var items []template
			for _, t := range templates {
				if t.category == cat {
					items = append(items, t)
				}
			}
			if len(items) == 0 {
				continue
			}
			ui.Column(c.Key(si)).Gap(10).Children(func() {
				kit.Eyebrow(c, catLabel(cat)).Padding(0, 2)
				// Top-aligned: a name on two lines must not lift its icon
				// out of line with its neighbours.
				ui.Row(c).Gap(4).Wrap().AlignItems(ui.Start).Margin(0, -6).Children(func() {
					for _, t := range items {
						a.presetTile(c.Key(t.url), t, min(n, 12))
						n++
					}
				})
			})
		}
	})
}

// presetTile is a floating preset: hover raises its caption plate and turns it
// into "Make App".
func (a *App) presetTile(c *ui.Context, t template, i int) {
	p := design.Of(c)
	const size = 76
	b := ui.ButtonBase(c).Label(T("discover.get.label", "name", t.name)).Column().Width(132).AlignItems(ui.Center).Padding(12, 6, 10, 6).
		Cursor(ui.CursorPointer)
	hov := b.Hovered()
	b.Children(func() {
		holder := ui.Box(c).Size(size, size).Center()
		pose := ui.Pose{}
		if b.Pressed() {
			pose = ui.Pose{Scale: 0.97}
		}
		holder.AnimatePose(pose, design.Spring)
		holder.Children(func() {
			a.presetIconView(c, t, size, true)
		})
		plate := ui.Column(c).AlignItems(ui.Center).Gap(1).Margin(20, 0, 0, 0).Padding(4, 10).Radius(design.RadiusControl).
			Transition(design.Transition(c))
		if hov {
			plate.Background(p.Raised)
			design.RaisedControl(c, plate)
		}
		plate.Children(func() {
			// A long name wraps to a second line rather than losing its end.
			ui.Text(c, t.name).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).MaxLines(2).
				TextAlign(ui.Center).LineHeight(1.2)
			sub, col := hostOf(t.url), p.TextTertiary
			if hov {
				sub, col = T("discover.get"), p.AccentText
			}
			ui.Text(c, sub).FontSize(design.FontCaption).TextColor(col).SingleLine()
		})
	})
	// The full name and address, for when the caption is cut short.
	b.Tooltip(t.name + " · " + hostOf(t.url))
	if b.Clicked() {
		a.create.reset(a.host)
		a.create.setInput(t.url)
		a.nav(secCreate)
	}
}

func hostOf(u string) string {
	for _, pre := range []string{"https://", "http://"} {
		if len(u) > len(pre) && u[:len(pre)] == pre {
			u = u[len(pre):]
		}
	}
	for i, r := range u {
		if r == '/' {
			return u[:i]
		}
	}
	return u
}

func catLabel(cat string) string {
	switch cat {
	case "dev":
		return T("discover.cat.dev")
	case "work":
		return T("discover.cat.work")
	case "ai":
		return T("discover.cat.ai")
	case "design":
		return T("discover.cat.design")
	case "media":
		return T("discover.cat.media")
	case "social":
		return T("discover.cat.social")
	}
	return T("discover.cat.chat")
}

func presetNamed(name string) template {
	for _, t := range templates {
		if t.name == name {
			return t
		}
	}
	return templates[0]
}
