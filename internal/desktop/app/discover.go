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
		// Hero: an editorial headline beside a shelf of apps on the
		// workbench.
		hero := ui.Row(c).Gap(28).Padding(28, 32).Radius(design.RadiusPanel).AlignItems(ui.Center).Clip().
			Background(p.Group).Border(0.5, p.GroupBorder)
		hero.Children(func() {
			ui.Box(c).Absolute().Fill().PassThrough().AccessHidden(true).Draw(func(pt *ui.Painter, r ui.Rect) { design.PaintDots(pt, r, p, false) })
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
				ui.Box(c).Height(9).FillWidth().Radius(2).Margin(-3, 0, 0, 0).
					LinearGradient(ui.LinearGradient{From: p.ShelfTop, To: p.ShelfBottom, Angle: 180}).
					Shadow(0, 6, 10, -4, p.ShadowDeep).InsetShadow(0, 1, 0, 0, p.Highlight)
			})
		})
		ui.Column(c).Gap(4).Children(func() {
			kit.SectionHeader(c, T("discover.popular"), "")
			kit.Caption(c, T("discover.footnote"))
		})
		ui.Grid(c).ColumnTracks(ui.Fr(1), ui.Fr(1)).GapX(24).Children(func() {
			for i, t := range templates {
				t := t
				ui.Column(c.Key(t.url)).Children(func() {
					row := ui.Row(c).Height(72).Gap(14).AlignItems(ui.Center).Padding(0, 4)
					row.Children(func() {
						a.presetIconView(c, t, 52, true)
						ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
							ui.Text(c, t.name).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
							ui.Text(c, catLabel(t.category)).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine()
							ui.Text(c, hostOf(t.url)).FontSize(design.FontCaption).TextColor(p.TextTertiary).SingleLine()
						})
						if kit.Button(c.Key("get"), T("discover.get"), kit.ButtonOpts{Size: 26}).
							Label(T("discover.get.label", "name", t.name)).Clicked() {
							a.create.reset(a.host)
							a.create.setInput(t.url)
							a.nav(secCreate)
						}
					})
					if i < len(templates)-2 {
						ui.Box(c).Height(0.5).Background(p.Divider).Margin(0, 0, 0, 70)
					}
				})
			}
		})
	})
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
