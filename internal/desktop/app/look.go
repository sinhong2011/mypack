package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/pkg/config"
)

// Look & Behaviour: the settings most people want, as visual pickers and a
// short inset group, outside Advanced (Create and project Settings).

// lookAndBehaviour draws the pickers for cfg; changed is called after
// each edit. account, when non-nil, shows the Account label field.
func (a *App) lookAndBehaviour(c *ui.Context, cfg *config.Config, changed func(), account *string) {
	p := design.Of(c)
	w := &cfg.Window
	ui.Column(c).Gap(18).Children(func() {
		// Title bar and appearance side by side.
		ui.Row(c).Gap(28).Wrap().AlignItems(ui.Start).Children(func() {
			pickerGroup(c, T("look.titleBar"), func() {
				seamless := w.TitleBarStyle == "overlay" || w.TitleBarStyle == "hidden"
				if thumb(c.Key("tb-std"), T("look.titleBar.standard"), "", !seamless, 116, 76, func() { windowThumb(c, p, false, 116, 76) }) {
					w.TitleBarStyle = "default"
					changed()
				}
				if thumb(c.Key("tb-seam"), T("look.titleBar.seamless"), "", seamless, 116, 76, func() { windowThumb(c, p, true, 116, 76) }) {
					w.TitleBarStyle = "overlay"
					changed()
				}
			})
			pickerGroup(c, T("set.appearance"), func() {
				for i, mode := range []string{"system", "light", "dark"} {
					label := []string{T("appearance.system"), T("appearance.light"), T("appearance.dark")}[i]
					cur := w.DarkMode
					if cur == "" {
						cur = "system"
					}
					if thumb(c.Key("ap-"+mode), label, "", cur == mode, 76, 76, func() { appearanceThumb(c, mode, 76, 76) }) {
						w.DarkMode = mode
						changed()
					}
				}
			})
		})
		// Window size.
		pickerGroup(c, T("look.size"), func() {
			for _, sp := range sizePresets {
				on := !w.Maximized && w.Width == sp.w && w.Height == sp.h
				label := map[string]string{"compact": T("set.size.compact"), "standard": T("set.size.standard"), "large": T("set.size.large")}[sp.id]
				sp := sp
				if thumb(c.Key("sz-"+sp.id), label, fmt.Sprintf("%d × %d", sp.w, sp.h), on, 116, 76, func() {
					sizeThumb(c, p, float32(sp.w)/1600, float32(sp.h)/1000, 116, 76)
				}) {
					w.Width, w.Height, w.Maximized = sp.w, sp.h, false
					changed()
				}
			}
			if thumb(c.Key("sz-fill"), T("look.size.fill"), T("look.size.fill.sub"), w.Maximized, 116, 76, func() { sizeThumb(c, p, 1, 1, 116, 76) }) {
				w.Maximized = true
				changed()
			}
		})
		// Behaviour.
		kit.GroupBox(c).Children(func() {
			signedIn := !cfg.WebView.Incognito
			desc := T("look.signedIn.on")
			if !signedIn {
				desc = T("look.signedIn.off")
			}
			kit.SettingIconRow(c.Key("si"), icons.Person, ui.Color{}, T("look.signedIn"), desc, func() {
				old := signedIn
				kit.Switch(c.Key("sw-si"), &signedIn, T("look.signedIn"))
				if signedIn != old {
					cfg.WebView.Incognito = !signedIn
					changed()
				}
			})
			ext := cfg.WebView.Navigation.ExternalByDefault == nil || *cfg.WebView.Navigation.ExternalByDefault
			kit.SettingIconRow(c.Key("ln"), icons.Link, ui.Color{}, T("look.links"), T("look.links.desc"), func() {
				v := "browser"
				if !ext {
					v = "app"
				}
				old := v
				kit.PopUp(c.Key("pp-ln"), &v, []kit.Option{{Value: "browser", Label: T("look.links.browser")}, {Value: "app", Label: T("look.links.app")}}, T("look.links")).Width(190)
				if v != old {
					if v == "browser" {
						cfg.WebView.Navigation.ExternalByDefault = nil
					} else {
						f := false
						cfg.WebView.Navigation.ExternalByDefault = &f
					}
					changed()
				}
			})
			tray := cfg.Native.Tray
			kit.SettingIconRow(c.Key("tr"), icons.Bell, ui.Color{}, T("look.tray"), T("look.tray.desc"), func() {
				kit.Switch(c.Key("sw-tr"), &tray, T("look.tray"))
				if tray != cfg.Native.Tray {
					cfg.Native.Tray, w.HideOnClose = tray, tray
					if !tray {
						cfg.Native.StartToTray = false
					}
					changed()
				}
			})
			if account != nil {
				kit.SettingIconRow(c.Key("ac"), icons.Person, ui.Color{}, T("look.account"), T("look.account.desc"), func() {
					kit.TextField(c.Key("tf-ac"), account, T("look.account"), kit.FieldOpts{Placeholder: T("look.account.placeholder")}).Width(170)
				})
			}
		})
	})
}

// pickerGroup is a captioned row of thumbnails.
func pickerGroup(c *ui.Context, title string, body func()) {
	p := design.Of(c)
	ui.Column(c).Gap(10).Children(func() {
		ui.Text(c, title).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).Padding(0, 2).Role(ui.RoleHeading)
		ui.Row(c).Gap(12).Wrap().Role(ui.RoleRadioGroup).Label(title).Children(body)
	})
}

// thumb is kit.ChoiceTile.
func thumb(c *ui.Context, label, sub string, selected bool, w, h float32, pic func()) bool {
	return kit.ChoiceTile(c, label, sub, selected, w, h, pic)
}

// desktop paints the quiet workbench a thumbnail's window sits on.
func desktop(c *ui.Context, dark bool) ui.Element {
	pal := &design.Light
	from, to := design.Paper[3], design.Paper[5]
	if dark {
		pal = &design.Dark
		from, to = design.Ink[5], design.Ink[3]
	}
	_ = pal
	return ui.Box(c).Absolute().Fill().Background(from.Mix(to, 0.5))
}

func dots(c *ui.Context) {
	ui.Row(c).Gap(3).Children(func() {
		for _, col := range []ui.Color{ui.RGB(255, 95, 87), ui.RGB(254, 188, 46), ui.RGB(40, 200, 64)} {
			ui.Box(c).Size(5, 5).Radius(2.5).Background(col)
		}
	})
}

// pageLines draws a few lines of a web page.
func pageLines(c *ui.Context, ink ui.Color, n int) {
	ui.Column(c).Gap(4).Padding(6, 8).Children(func() {
		for i := 0; i < n; i++ {
			w := []float32{80, 60, 70, 45}[i%4]
			ui.Box(c.Key(i)).Height(4).WidthPercent(w).Radius(2).Background(ink)
		}
	})
}

// windowThumb is a little window with a standard or seamless title bar.
func windowThumb(c *ui.Context, p *design.Palette, seamless bool, w, h float32) {
	desktop(c, p.Dark)
	ui.Column(c).Absolute().Left(10).Top(10).Right(10).Bottom(-6).Radius(7).Clip().Background(ui.RGB(255, 255, 255)).
		Shadow(0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.18)).Children(func() {
		bar := ui.Row(c).Height(13).PaddingX(5).AlignItems(ui.Center)
		if seamless {
			bar.Background(p.Accent.Mix(ui.RGB(255, 255, 255), 0.15))
		} else {
			bar.Background(ui.RGB(236, 236, 238)).BorderWidth(0, 0, 0.5, 0).BorderColor(ui.RGBA(0, 0, 0, 0.12))
		}
		bar.Children(func() { dots(c) })
		if seamless {
			ui.Column(c).Background(p.Accent.Mix(ui.RGB(255, 255, 255), 0.15)).Padding(0, 8, 6, 8).Children(func() {
				ui.Box(c).Height(5).WidthPercent(55).Radius(2.5).Background(ui.RGBA(255, 255, 255, 0.85))
			})
		}
		pageLines(c, ui.RGBA(0, 0, 0, 0.1), 3)
	})
}

// appearanceThumb previews light, dark or both (System: a diagonal split).
// Each side is a real little window in that appearance: its desktop, a
// chrome bar with the traffic lights, page lines and an accent button.
func appearanceThumb(c *ui.Context, mode string, w, h float32) {
	ui.Box(c).Absolute().Fill().Draw(func(pt *ui.Painter, r ui.Rect) {
		switch mode {
		case "system":
			paintAppearance(pt, r, false)
			split := &ui.Path{}
			split.MoveTo(r.X+r.W*0.62, r.Y).LineTo(r.X+r.W, r.Y).LineTo(r.X+r.W, r.Y+r.H).LineTo(r.X+r.W*0.38, r.Y+r.H).Close()
			pt.ClipPath(split, func() { paintAppearance(pt, r, true) })
		default:
			paintAppearance(pt, r, mode == "dark")
		}
	})
}

// paintAppearance paints one appearance's desktop and window into r.
func paintAppearance(pt *ui.Painter, r ui.Rect, dark bool) {
	pal := &design.Light
	desk, chrome, page, ink, rule := design.Paper[4], design.Paper[2], design.Paper[0], ui.RGBA(27, 27, 25, 0.13), ui.RGBA(27, 27, 25, 0.1)
	if dark {
		pal = &design.Dark
		desk, chrome, page, ink, rule = design.Ink[0], design.Ink[4], design.Ink[2], ui.RGBA(255, 255, 255, 0.2), ui.RGBA(0, 0, 0, 0.5)
	}
	pt.Fill(r, desk, 0)
	win := ui.Rect{X: r.X + 10, Y: r.Y + 12, W: r.W - 6, H: r.H - 6}
	pt.Shadow(win, 6, 0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.22))
	pt.Clip(win, 6, func() {
		pt.Fill(win, page, 0)
		bar := ui.Rect{X: win.X, Y: win.Y, W: win.W, H: 12}
		pt.Fill(bar, chrome, 0)
		pt.Fill(ui.Rect{X: win.X, Y: win.Y + 12, W: win.W, H: 0.5}, rule, 0)
		for i, col := range []ui.Color{ui.RGB(255, 95, 87), ui.RGB(254, 188, 46), ui.RGB(40, 200, 64)} {
			pt.Fill(ui.Rect{X: win.X + 5 + float32(i)*7, Y: win.Y + 3.5, W: 5, H: 5}, col, 2.5)
		}
		y := win.Y + 19
		for i, f := range []float32{0.62, 0.46, 0.54} {
			pt.Fill(ui.Rect{X: win.X + 8, Y: y + float32(i)*8, W: (win.W - 16) * f, H: 4}, ink, 2)
		}
		accent := pal.Accent
		pt.Fill(ui.Rect{X: win.X + 8, Y: y + 27, W: 20, H: 7}, accent, 3.5)
	})
}

// sizeThumb shows a window of fraction fw×fh of a screen.
func sizeThumb(c *ui.Context, p *design.Palette, fw, fh, w, h float32) {
	desktop(c, p.Dark)
	ww, wh := (w-8)*fw, (h-8)*fh
	ui.Column(c).Absolute().Left((w-ww)/2).Top((h-wh)/2).Size(ww, wh).Radius(5).Clip().Background(ui.RGB(255, 255, 255)).
		Shadow(0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.2)).Children(func() {
		ui.Row(c).Height(10).PaddingX(4).AlignItems(ui.Center).Background(ui.RGB(236, 236, 238)).Children(func() { dots(c) })
	})
}
