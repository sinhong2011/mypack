package app

import (
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/sampleicons"
)

// template is a declarative quick-start preset (spec §09A: local, static,
// no executable rules). Choosing one only prefills Create.
type template struct {
	name, url, category string
}

var templates = []template{
	{"GitHub", "https://github.com/", "dev"},
	{"Notion", "https://www.notion.so/", "work"},
	{"ChatGPT", "https://chatgpt.com/", "ai"},
	{"Figma", "https://www.figma.com/", "design"},
	{"Linear", "https://linear.app/", "work"},
	{"YouTube Music", "https://music.youtube.com/", "media"},
	{"WhatsApp", "https://web.whatsapp.com/", "chat"},
	{"Gmail", "https://mail.google.com/", "work"},
	{"Slack", "https://app.slack.com/", "chat"},
	{"Discord", "https://discord.com/app", "chat"},
}

// templateIcon is the neutral generated icon a preset shows.
func templateIcon(name string) []byte { return sampleicons.Icon(name) }

func (a *App) discoverView(c *ui.Context) {
	p := design.Of(c)
	page(c, design.ContentMaxW, func() {
		// Hero: an editorial headline beside a few floating apps.
		hero := ui.Row(c).Gap(28).Padding(28, 32).Radius(design.RadiusPanel).AlignItems(ui.Center).Clip().
			Background(p.Group).Border(0.5, p.GroupBorder)
		hero.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
				kit.Eyebrow(c, T("discover.hero.eyebrow")).TextColor(p.AccentText)
				ui.Text(c, T("discover.hero.title")).FontSize(design.FontDisplay).FontWeight(design.WeightBold).TextColor(p.Text).
					LetterSpacing(design.TrackDisplay).LineHeight(1.1)
				ui.Text(c, T("discover.later.body")).FontSize(design.FontBody).TextColor(p.TextSecondary).LineHeight(1.45).MaxWidth(420)
			})
			ui.Column(c).Shrink(0).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(14).AlignItems(ui.End).Padding(0, 16).Children(func() {
					for i, n := range []string{"Figma", "Linear", "ChatGPT"} {
						sz := float32(64)
						if i == 1 {
							sz = 80
						}
						icon := ui.Box(c.Key(n))
						kit.Arrive(icon, i, design.StaggerGrid)
						icon.Children(func() { a.presetIconView(c, presetNamed(n), sz, true) })
					}
				})
			})
		})
		ui.Column(c).Gap(4).Children(func() {
			kit.SectionHeader(c, T("discover.popular"), "")
			kit.Caption(c, T("discover.footnote"))
		})
		// Shelves by kind of site.
		shelves := []struct {
			title string
			cats  []string
		}{
			{T("discover.shelf.work"), []string{"work", "dev"}},
			{T("discover.shelf.chat"), []string{"chat"}},
			{T("discover.shelf.create"), []string{"design", "ai", "media"}},
		}
		for si, sh := range shelves {
			var items []template
			for _, t := range templates {
				for _, cat := range sh.cats {
					if t.category == cat {
						items = append(items, t)
					}
				}
			}
			ui.Column(c.Key(si)).Gap(10).Children(func() {
				kit.Eyebrow(c, sh.title).Padding(0, 2)
				ui.Row(c).Gap(4).Wrap().Margin(0, -6).Children(func() {
					for i, t := range items {
						a.presetTile(c.Key(t.url), t, si*4+i)
					}
				})
			})
		}
	})
}

// presetTile is a floating preset: hover lifts it and turns its caption
// into "Make App".
func (a *App) presetTile(c *ui.Context, t template, i int) {
	p := design.Of(c)
	const size = 76
	b := ui.ButtonBase(c).Label(T("discover.get.label", "name", t.name)).Column().Width(132).AlignItems(ui.Center).Padding(12, 6, 10, 6).
		Cursor(ui.CursorPointer)
	kit.Arrive(b, i, design.StaggerGrid)
	hov := b.Hovered()
	b.Children(func() {
		holder := ui.Box(c).Size(size, size).Center()
		pose := ui.Pose{}
		if hov {
			pose = ui.Pose{Y: -3, Scale: 1.04}
		}
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
			ui.Text(c, t.name).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
			sub, col := hostOf(t.url), p.TextTertiary
			if hov {
				sub, col = T("discover.get"), p.AccentText
			}
			ui.Text(c, sub).FontSize(design.FontCaption).TextColor(col).SingleLine()
		})
	})
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
