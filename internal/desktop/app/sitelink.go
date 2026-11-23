package app

import (
	"context"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// siteLink is a website's host as a quiet pill link: the link glyph, the
// host and an arrow that brightens on hover. Clicking it opens url in the
// default browser; the tooltip shows the full address.
func (a *App) siteLink(c *ui.Context, url, host string, fontSize float32) ui.Element {
	p := design.Of(c)
	b := ui.ButtonBase(c.Key("site-link")).Label(host).Row().Gap(5).AlignItems(ui.Center).
		Padding(2, 8, 2, 7).Radius(999).MinWidth(0).Shrink(1).
		Cursor(ui.CursorPointer).Tooltip(url).Transition(design.Transition(c))
	hot := b.Hovered() || b.FocusVisible()
	text, glyph := p.TextSecondary, p.TextTertiary
	if hot {
		b.Background(p.Hover)
		text, glyph = p.AccentText, p.AccentText
	}
	if b.Pressed() {
		b.Background(p.Fill)
	}
	b.Children(func() {
		kit.Glyph(c, icons.Link, 12, glyph)
		ui.Text(c, host).FontSize(fontSize).FontWeight(design.WeightMedium).TextColor(text).SingleLine().MinWidth(0).Shrink(1)
		arrow := ui.Box(c).PassThrough()
		arrow.Children(func() { kit.Glyph(c, icons.External, 10, glyph) })
		op := float32(0.55)
		if hot {
			op = 1
		}
		arrow.Opacity(arrow.AnimateTo("op", op, design.Spring))
	})
	if b.Clicked() {
		a.do(func(ctx context.Context) error { return a.svc.OpenURL(ctx, url) }, a.toastErr)
	}
	return b
}

// projectSource is where a project comes from: a clickable site link for a
// website, the quiet folder path otherwise.
func (a *App) projectSource(c *ui.Context, pr *domain.Project, fontSize float32) {
	src := pr.Config.App.Source
	if src.Type == config.SourceURL {
		a.siteLink(c, src.Value, domainOf(src), fontSize)
		return
	}
	p := design.Of(c)
	ui.Row(c).Gap(5).AlignItems(ui.Center).MinWidth(0).Children(func() {
		kit.Glyph(c, icons.Folder, 12, p.TextTertiary)
		ui.Text(c, domainOf(src)).FontSize(fontSize).TextColor(p.TextSecondary).SingleLine().MinWidth(0).Shrink(1)
	})
}
