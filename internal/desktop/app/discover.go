package app

import (
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/components"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
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

func (a *App) discoverView(c *ui.Context) {
	p := design.Of(c)
	page(c, design.ContentMaxW, func() {
		ui.Row(c).Gap(14).Padding(16, 18).Radius(design.RadiusCard).Background(p.AccentSoft).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lobeui.IconDiscover()).FontSize(20).TextColor(p.AccentText)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, T("discover.later.title")).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text)
				ui.Text(c, T("discover.later.body")).FontSize(design.FontSmall).TextColor(p.TextSecondary).LineHeight(1.4)
			})
		})
		components.Heading(c, T("discover.popular"))
		ui.Grid(c).ColumnTracks(ui.Fr(1), ui.Fr(1), ui.Fr(1)).Gap(12).Children(func() {
			for _, t := range templates {
				t := t
				tile := ui.ButtonBase(c.Key(t.url)).Label(t.name).Row().Justify(ui.Start).Height(64).Padding(0, 14).Gap(12).AlignItems(ui.Center).
					Radius(design.RadiusCard).Background(p.Surface).Border(1, p.Border).Cursor(ui.CursorPointer).Transition(design.Transition(c))
				hov := tile.Hovered()
				if hov {
					tile.Border(1, p.BorderStrong).Shadow(0, 2, 8, 0, p.Shadow)
				}
				tile.Children(func() {
					components.Placeholder(c, t.name, 36)
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, t.name).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
						ui.Text(c, hostOf(t.url)).FontSize(design.FontSmall).TextColor(p.TextTertiary).SingleLine()
					})
					fg := p.TextTertiary
					if hov {
						fg = p.AccentText
					}
					ui.Icon(c, lobeui.IconArrowRight()).FontSize(15).TextColor(fg)
				})
				if tile.Clicked() {
					a.create.reset(a.host)
					a.create.setInput(t.url)
					a.nav(secCreate)
				}
			}
		})
		components.Caption(c, T("discover.footnote"))
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
