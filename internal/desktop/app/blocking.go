package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/pkg/config"
)

// Content blocking (webview.blocking): one choice in Look & Behaviour, and
// the switches, allow list and hide list in Advanced Settings.

const (
	blockOff    = "off"
	blockTrack  = "trackers"
	blockAll    = "all" // trackers and cookie banners
	blockCustom = "custom"
)

// blockingMode summarises cfg's blocking for the Look & Behaviour choice.
func blockingMode(cfg *config.Config) string {
	b := cfg.WebView.Blocking
	switch {
	case b == nil || !b.Trackers && !b.CookieBanners:
		return blockOff
	case b.Trackers && b.CookieBanners:
		return blockAll
	case b.Trackers:
		return blockTrack
	}
	return blockCustom // banners alone (Advanced Settings)
}

func setBlockingMode(cfg *config.Config, mode string) {
	b := ensureBlocking(cfg)
	b.Trackers = mode == blockTrack || mode == blockAll
	b.CookieBanners = mode == blockAll
	tidyBlocking(cfg)
}

func ensureBlocking(cfg *config.Config) *config.BlockingConfig {
	if cfg.WebView.Blocking == nil {
		cfg.WebView.Blocking = &config.BlockingConfig{}
	}
	return cfg.WebView.Blocking
}

// tidyBlocking drops a blocking section that holds nothing, so mypack.json
// stays as it was.
func tidyBlocking(cfg *config.Config) {
	if b := cfg.WebView.Blocking; b != nil && !b.Active() && len(b.Allow) == 0 {
		cfg.WebView.Blocking = nil
	}
}

// blockingRow is the Look & Behaviour row.
func (a *App) blockingRow(c *ui.Context, cfg *config.Config, changed func()) {
	a.lookRow(c.Key("blk"), icons.Shield, ui.Color{}, T("look.blocking"), T("look.blocking.desc"), func() {
		cur := blockingMode(cfg)
		v := cur
		opts := []kit.Option{{Value: blockOff, Label: T("look.blocking.off")}, {Value: blockTrack, Label: T("look.blocking.trackers")},
			{Value: blockAll, Label: T("look.blocking.all")}}
		if cur == blockCustom {
			opts = append(opts, kit.Option{Value: blockCustom, Label: T("look.blocking.custom")})
		}
		kit.PopUp(c.Key("pp-blk"), &v, opts, T("look.blocking")).Width(150)
		if v != cur && v != blockCustom {
			setBlockingMode(cfg, v)
			changed()
		}
	})
}

// chipList edits a list of short strings as removable chips and a field
// that adds one (norm cleans what was typed; "" refuses it).
func chipList(c *ui.Context, list *[]string, buf *string, placeholder string, mono bool, norm func(string) string, changed func()) {
	p := design.Of(c)
	ui.Column(c).Gap(8).Children(func() {
		if len(*list) > 0 {
			ui.Row(c).Gap(6).Wrap().Children(func() {
				remove := -1
				for i, d := range *list {
					i := i
					ui.Row(c.Key(d)).Height(26).Padding(0, 4, 0, 10).Gap(4).AlignItems(ui.Center).Radius(13).Background(p.Hover).Children(func() {
						t := ui.Text(c, d).FontSize(design.FontSmall).TextColor(p.Text)
						if mono {
							t.Font(kit.MonoFont)
						}
						x := ui.ButtonBase(c).Label(T("common.remove")+" "+d).Size(18, 18).Radius(9).Center().Cursor(ui.CursorPointer)
						if x.Hovered() {
							x.Background(p.Pressed)
						}
						x.Children(func() { kit.Glyph(c, icons.Close, 11, p.TextSecondary) })
						if x.Clicked() {
							remove = i
						}
					})
				}
				if remove >= 0 {
					*list = append((*list)[:remove:remove], (*list)[remove+1:]...)
					changed()
				}
			})
		}
		v := norm(*buf)
		bad := strings.TrimSpace(*buf) != "" && v == ""
		add := func() {
			if v == "" {
				return
			}
			for _, x := range *list {
				if x == v {
					*buf = ""
					return
				}
			}
			*list = append(*list, v)
			*buf = ""
			changed()
		}
		ui.Row(c).Gap(8).Children(func() {
			in := kit.TextField(c.Key("new"), buf, placeholder, kit.FieldOpts{Placeholder: placeholder, Invalid: bad, Mono: mono}).Grow(1)
			if in.Submitted() {
				add()
			}
			if kit.Button(c.Key("add"), T("common.add"), kit.ButtonOpts{Disabled: v == ""}).Clicked() {
				add()
			}
		})
	})
}

// cssSelector accepts what can sit in a hide rule.
func cssSelector(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 300 || strings.ContainsAny(s, "{}") {
		return ""
	}
	return s
}
