package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
	mpicons "mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/pkg/config"
)

// The Icon Studio: a sheet that dresses an app icon up — letters or an
// emoji instead of the image, a new background (solid, gradient, glass),
// the glyph's size and colour, and a badge ("Work", "Beta"). The result is
// a style in the config (app.icon.style), drawn by internal/icons: the
// preview here is exactly what the build makes.

type iconStudio struct {
	open      bool
	projectID string // "" = the app being made in New App
	name      string
	base      []byte // the image beneath the style
	baseNew   bool   // the user chose a new image here
	st        config.IconStyle

	letters string
	emoji   string
	c1, c2  ui.Color // custom background colours
	tintC   ui.Color
	scale   float64

	cache map[string]*ui.Bitmap
}

// Studio choices, in the order they are shown.
var (
	studioGlyphs      = []string{config.GlyphImage, config.GlyphText, config.GlyphEmoji}
	studioFonts       = []string{"bold", "regular", "mono", "rounded"}
	studioBackgrounds = []string{config.BackgroundNone, config.BackgroundAuto, config.BackgroundSolid, config.BackgroundGradient, config.BackgroundGlass}
	studioTints       = []string{"", config.TintWhite, config.TintBlack, config.TintColor}
	studioEmoji       = []string{
		"🚀", "💬", "📮", "📅", "📝", "📊", "🎵", "🎬", "🛒", "💼", "🏠", "🔥",
		"⚡", "🌈", "🌙", "☀️", "🍀", "🌊", "🎨", "🧠", "🤖", "👾", "🐙", "🦊",
		"🐱", "🐶", "🦄", "🍕", "☕", "🎮", "📚", "💡", "🔒", "⭐", "❤️", "✅",
	}
	studioBadgeColors = []string{"#e5484d", "#f97316", "#eab308", "#16a34a", "#2563eb", "#7c3aed", "#db2777", "#1d1d1f"}
)

func studioIndex(list []string, v string) int {
	for i, s := range list {
		if s == v {
			return i
		}
	}
	return 0
}

// openIconStudio opens the studio for a project, or for New App when
// projectID is "".
func (a *App) openIconStudio(projectID string) {
	s := &a.studio
	*s = iconStudio{open: true, projectID: projectID, cache: map[string]*ui.Bitmap{}, scale: 64}
	var st *config.IconStyle
	if projectID == "" {
		s.name = a.create.displayName()
		s.base = a.createBaseIcon()
		st = a.create.iconStyle
	} else if pr := a.project(projectID); pr != nil {
		s.name = pr.Config.App.Name
		st = pr.Config.App.Icon.Style
		if ed := a.editors[projectID]; ed != nil {
			st = ed.cfg.App.Icon.Style
		}
		s.base = pr.IconPNG
		if st != nil {
			// The styled icon is what the Library shows; fetch the image beneath.
			id := projectID
			go func() {
				b, err := a.svc.ProjectIconImage(context.Background(), id)
				if err == nil {
					a.win.Update(func() {
						if a.studio.open && a.studio.projectID == id && !a.studio.baseNew {
							a.studio.base = b
						}
					})
				}
			}()
		}
	}
	if st != nil {
		s.st = *st
		s.st.Colors = append([]string(nil), st.Colors...)
	} else {
		s.st.Background = config.BackgroundNone
	}
	switch s.st.Glyph {
	case config.GlyphText:
		s.letters = s.st.Text
	case config.GlyphEmoji:
		s.emoji = s.st.Text
	}
	if s.letters == "" {
		s.letters = initials(s.name)
	}
	if s.emoji == "" {
		s.emoji = "🚀"
	}
	if len(s.st.Colors) > 0 {
		s.c1 = colorOf(s.st.Colors[0])
		s.c2 = s.c1
		if len(s.st.Colors) > 1 {
			s.c2 = colorOf(s.st.Colors[1])
		}
	} else {
		top, bottom := kit.PlaceholderColors(s.name)
		s.c1, s.c2 = top, bottom
	}
	s.tintC = ui.RGB(255, 255, 255)
	if config.IsHexColor(s.st.TintColor) {
		s.tintC = colorOf(s.st.TintColor)
	}
	if s.st.Scale != 0 {
		s.scale = float64(s.st.Scale)
	}
}

// initials is up to two letters of a name ("Google Mail" → "GM").
func initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		for _, r := range w {
			if len(out) < 2 && (r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 0x2e80) {
				out = append(out, r)
			}
			break
		}
	}
	if len(out) == 0 {
		return "A"
	}
	return strings.ToUpper(string(out))
}

func colorOf(hex string) ui.Color {
	if !config.IsHexColor(hex) {
		return ui.RGB(142, 142, 147)
	}
	return ui.Hex(hex)
}

func hexOfColor(c ui.Color) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// plainStyle reports whether st changes nothing (the icon as it is).
func plainStyle(st config.IconStyle) bool {
	return (st.Glyph == "" || st.Glyph == config.GlyphImage) && st.Background == config.BackgroundNone &&
		st.Tint == "" && strings.TrimSpace(st.Badge) == "" && (st.Scale == 0 || st.Scale == 100)
}

// finalStyle is the style the studio applies (nil = no style).
func (s *iconStudio) finalStyle() *config.IconStyle {
	st := s.st
	st.Badge = strings.TrimSpace(st.Badge)
	if st.Glyph == config.GlyphImage {
		st.Glyph, st.Text, st.Font = "", "", ""
	}
	if st.Background == config.BackgroundAuto {
		st.Background = ""
	}
	if st.Background == config.BackgroundNone || st.Background == "" {
		st.Colors = nil
	}
	if st.Tint != config.TintColor {
		st.TintColor = ""
	}
	if st.Badge == "" {
		st.BadgeColor = ""
	}
	cmp := st
	if cmp.Background == "" {
		cmp.Background = config.BackgroundAuto
	}
	if plainStyle(cmp) {
		return nil
	}
	return &st
}

// render draws st at px pixels, cached.
func (s *iconStudio) render(st config.IconStyle, px int) *ui.Bitmap {
	b, _ := json.Marshal(st)
	key := fmt.Sprintf("%d|%p|%s", px, firstByte(s.base), b)
	if bm, ok := s.cache[key]; ok {
		return bm
	}
	if len(s.cache) > 64 {
		s.cache = map[string]*ui.Bitmap{}
	}
	bm := ui.NewBitmap(mpicons.StyledImage(s.base, st, s.name, px))
	s.cache[key] = bm
	return bm
}

func firstByte(b []byte) *byte {
	if len(b) == 0 {
		return nil
	}
	return &b[0]
}

func (a *App) iconStudioSheet(c *ui.Context) {
	a.e2eStudioHook()
	s := &a.studio
	if !s.open {
		return
	}
	p := design.Of(c)
	kit.Sheet(c.Key("dlg-studio"), &s.open, 760, func() {
		kit.SheetHeader(c, T("studio.title"), func() {
			kit.SymbolTile(c, icons.Wand, p.Accent, 26)
		})
		ui.Row(c).Gap(24).AlignItems(ui.Start).Children(func() {
			a.studioPreview(c)
			ui.Scroll(c).Grow(1).MinWidth(0).MaxHeight(470).Children(func() {
				ui.Column(c).Gap(18).Padding(0, 6, 4, 0).Children(func() {
					a.studioGlyph(c)
					a.studioBackground(c)
					a.studioGlyphLook(c)
					a.studioBadge(c)
				})
			})
		})
		ok, dismissed := kit.SheetFooter(c, T("common.cancel"), T("studio.apply"), kit.Primary, false, false)
		if dismissed {
			s.open = false
		}
		if ok {
			a.applyIconStudio()
		}
	})
}

// studioPreview is the big live icon, how it looks small on light and dark,
// and Surprise Me / Reset.
func (a *App) studioPreview(c *ui.Context) {
	s := &a.studio
	p := design.Of(c)
	bm := s.render(s.st, 512)
	ui.Column(c).Width(220).Shrink(0).Gap(14).AlignItems(ui.Center).Children(func() {
		stage := ui.Box(c).Size(220, 220).Center().Radius(design.RadiusCard+4).Background(p.Group).Border(0.5, p.GroupBorder)
		stage.Children(func() {
			iconImage(c, bm, 188).Label(T("studio.preview"))
		})
		ui.Row(c).Gap(8).AccessHidden(true).Children(func() {
			for i, bg := range []ui.Color{ui.RGB(246, 246, 248), ui.RGB(30, 30, 34)} {
				ui.Row(c.Key(fmt.Sprint("chip", i))).Width(106).Height(54).Center().Gap(10).Radius(12).Background(bg).
					Border(0.5, p.GroupBorder).Children(func() {
					iconImage(c, bm, 36)
					iconImage(c, bm, 18)
				})
			}
		})
		ui.Row(c).Gap(6).Children(func() {
			if kit.Button(c.Key("shuffle"), T("studio.surprise"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM, Icon: icons.Sparkles}).Clicked() {
				a.studioSurprise()
			}
			if kit.Button(c.Key("reset"), T("studio.reset"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM, Icon: icons.Undo}).Clicked() {
				s.st = config.IconStyle{Background: config.BackgroundNone}
				s.scale = 64
			}
		})
	})
}

// iconImage shows a rendered icon clipped to the app-icon shape, as the
// Dock masks it (styled icons sit inside that shape already).
func iconImage(c *ui.Context, bm *ui.Bitmap, size float32) ui.Element {
	box := ui.Box(c).Size(size, size).Shrink(0).Radius(design.AppIconRadius(size)).Clip()
	box.Children(func() { ui.Image(c, bm).Size(size, size) })
	return box
}

func studioLabel(c *ui.Context, s string) {
	p := design.Of(c)
	ui.Text(c, s).FontSize(design.FontSmall).FontWeight(design.WeightSemibold).TextColor(p.TextSecondary).Role(ui.RoleHeading)
}

// studioGlyph: the image, letters or an emoji.
func (a *App) studioGlyph(c *ui.Context) {
	s := &a.studio
	p := design.Of(c)
	ui.Column(c.Key("glyph")).Gap(10).Children(func() {
		studioLabel(c, T("studio.symbol"))
		labels := []string{T("studio.symbol.image"), T("studio.symbol.letters")}
		if mpicons.EmojiAvailable() {
			labels = append(labels, T("studio.symbol.emoji"))
		}
		gi := min(studioIndex(studioGlyphs, s.st.Glyph), len(labels)-1)
		kit.Segmented(c.Key("seg"), &gi, labels, false)
		s.st.Glyph = studioGlyphs[gi]
		switch s.st.Glyph {
		case config.GlyphText:
			ui.Row(c.Key("letters")).Gap(10).AlignItems(ui.Center).Children(func() {
				kit.TextField(c.Key("tf"), &s.letters, T("studio.letters"), kit.FieldOpts{Placeholder: initials(s.name)}).Width(96)
				if r := []rune(s.letters); len(r) > config.MaxIconText {
					s.letters = string(r[:config.MaxIconText])
				}
				fi := studioIndex(studioFonts, s.st.Font)
				kit.Segmented(c.Key("font"), &fi, []string{T("studio.font.bold"), T("studio.font.regular"), T("studio.font.mono"), T("studio.font.rounded")}, true)
				s.st.Font = studioFonts[fi]
				if fi == 0 {
					s.st.Font = ""
				}
			})
			s.st.Text = strings.TrimSpace(s.letters)
			if s.st.Text == "" {
				s.st.Text = initials(s.name)
			}
		case config.GlyphEmoji:
			ui.Grid(c.Key("emoji")).ColumnTracks(repeatFr(12)...).Gap(2).Children(func() {
				for _, e := range studioEmoji {
					sel := e == s.emoji
					b := ui.ButtonBase(c.Key(e)).Label(e).Height(34).Center().Radius(8).Cursor(ui.CursorPointer).FontSize(20)
					if sel {
						b.Background(p.AccentSoft).Border(1, p.Accent)
					} else if b.Hovered() {
						b.Background(p.Hover)
					}
					b.Children(func() { ui.Text(c, e).FontSize(20) })
					if b.Clicked() {
						s.emoji = e
					}
				}
			})
			ui.Row(c.Key("etype")).Gap(10).AlignItems(ui.Center).Children(func() {
				kit.TextField(c.Key("tf"), &s.emoji, T("studio.emoji.type"), kit.FieldOpts{Placeholder: "🚀"}).Width(96)
				if !mpicons.HasEmoji(s.emoji) && strings.TrimSpace(s.emoji) != "" {
					ui.Text(c, T("studio.emoji.missing")).FontSize(design.FontSmall).TextColor(p.TextTertiary)
				}
			})
			s.st.Text = strings.TrimSpace(s.emoji)
		default:
			s.st.Text, s.st.Font = "", ""
			ui.Row(c.Key("img")).Gap(10).AlignItems(ui.Center).Children(func() {
				if kit.Button(c.Key("choose"), T("create.icon.choose"), kit.ButtonOpts{Size: design.ControlSM, Icon: icons.Photo}).Clicked() {
					a.chooseIcon(func(b []byte) {
						if n, err := mpicons.Normalize(b); err == nil {
							b = n
						}
						s.base, s.baseNew = b, true
					})
				}
				ui.Text(c, T("studio.image.hint")).FontSize(design.FontSmall).TextColor(p.TextTertiary).Grow(1).MinWidth(0)
			})
		}
	})
}

func repeatFr(n int) []ui.Track {
	out := make([]ui.Track, n)
	for i := range out {
		out[i] = ui.Fr(1)
	}
	return out
}

// studioBackground: Original, Auto, Solid, Gradient or Glass, and colours.
func (a *App) studioBackground(c *ui.Context) {
	s := &a.studio
	p := design.Of(c)
	ui.Column(c.Key("bg")).Gap(10).Children(func() {
		studioLabel(c, T("studio.background"))
		cur := s.st.Background
		if cur == "" {
			cur = config.BackgroundAuto
		}
		names := []string{T("studio.bg.original"), T("studio.bg.auto"), T("studio.bg.solid"), T("studio.bg.gradient"), T("studio.bg.glass")}
		ui.Row(c.Key("tiles")).Gap(8).Children(func() {
			for i, bg := range studioBackgrounds {
				st := s.st
				st.Background = bg
				if bg == config.BackgroundAuto {
					st.Colors = nil
				} else if bg != config.BackgroundNone && len(st.Colors) == 0 {
					st.Colors = []string{hexOfColor(s.c1), hexOfColor(s.c2)}
				}
				thumb := s.render(st, 112)
				if kit.ChoiceTileSmall(c.Key(bg), names[i], "", cur == bg, 72, 1, func() {
					ui.Box(c).Fill().Center().Background(p.Group).Children(func() {
						iconImage(c, thumb, 58)
					})
				}) {
					s.st.Background, s.st.Colors = bg, st.Colors
					if bg == config.BackgroundSolid && len(s.st.Colors) > 1 {
						s.st.Colors = s.st.Colors[:1]
					}
				}
			}
		})
		if cur == config.BackgroundNone {
			return
		}
		// Colours: Auto, the ready-made gradients, and custom wells.
		ui.Row(c.Key("swatches")).Gap(6).AlignItems(ui.Center).Children(func() {
			auto := ui.ButtonBase(c.Key("auto")).Label(T("studio.colors.auto")).Size(24, 24).Radius(12).Cursor(ui.CursorPointer).
				Border(1.5, p.BorderStrong).Center()
			auto.Children(func() { kit.Glyph(c, icons.Wand, 12, p.TextSecondary) })
			auto.Tooltip(T("studio.colors.auto"))
			if len(s.st.Colors) == 0 {
				auto.Border(2, p.Accent)
			}
			if auto.Clicked() {
				s.st.Colors = nil // keep the shape, take the colours from the image
			}
			for i, g := range mpicons.SuggestedColors() {
				g := g
				sw := ui.ButtonBase(c.Key(fmt.Sprint("sw", i))).Label(g[0]).Size(24, 24).Radius(12).Cursor(ui.CursorPointer).
					Gradient(colorOf(g[0]), colorOf(g[1]), 180)
				if len(s.st.Colors) > 0 && s.st.Colors[0] == g[0] {
					sw.Border(2, p.Accent)
				} else if sw.Hovered() {
					sw.Border(2, p.BorderStrong)
				}
				if sw.Clicked() {
					s.c1, s.c2 = colorOf(g[0]), colorOf(g[1])
					s.st.Colors = []string{g[0], g[1]}
					if s.st.Background == config.BackgroundSolid {
						s.st.Colors = []string{g[1]}
					}
					if s.st.Background == "" || s.st.Background == config.BackgroundAuto {
						s.st.Background = config.BackgroundGradient
					}
				}
			}
		})
		ui.Row(c.Key("wells")).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, T("studio.colors.custom")).FontSize(design.FontSmall).TextColor(p.TextSecondary)
			w1 := ui.ColorWell(c.Key("w1"), &s.c1).Label(T("studio.colors.top"))
			changed := w1.Changed()
			if s.st.Background != config.BackgroundSolid {
				if ui.ColorWell(c.Key("w2"), &s.c2).Label(T("studio.colors.bottom")).Changed() {
					changed = true
				}
			}
			if changed {
				s.st.Colors = []string{hexOfColor(s.c1), hexOfColor(s.c2)}
				if s.st.Background == config.BackgroundSolid {
					s.st.Colors = s.st.Colors[:1]
				}
				if s.st.Background == "" || s.st.Background == config.BackgroundAuto {
					s.st.Background = config.BackgroundGradient
				}
			}
		})
	})
}

// studioGlyphLook: the glyph's size and colour.
func (a *App) studioGlyphLook(c *ui.Context) {
	s := &a.studio
	p := design.Of(c)
	ui.Column(c.Key("look")).Gap(10).Children(func() {
		// The image as it is has no size to change.
		if s.st.Background == config.BackgroundNone && s.st.Glyph == config.GlyphImage {
			s.st.Scale = 0
		} else {
			studioLabel(c, T("studio.size"))
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Slider(c.Key("scale"), &s.scale, 20, 100).Width(260).Label(T("studio.size"))
				ui.Text(c, fmt.Sprintf("%d%%", int(s.scale+0.5))).FontSize(design.FontSmall).FontFeatures("tnum").TextColor(p.TextSecondary)
			})
			s.st.Scale = int(s.scale + 0.5)
			if s.st.Scale == 64 {
				s.st.Scale = 0
			}
		}
		studioLabel(c, T("studio.tint"))
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ti := studioIndex(studioTints, s.st.Tint)
			kit.Segmented(c.Key("tint"), &ti, []string{T("studio.tint.original"), T("studio.tint.white"), T("studio.tint.black"), T("studio.tint.custom")}, true)
			s.st.Tint = studioTints[ti]
			if s.st.Tint == config.TintColor {
				ui.ColorWell(c.Key("tw"), &s.tintC).Label(T("studio.tint.custom"))
				s.st.TintColor = hexOfColor(s.tintC)
			}
		})
	})
}

// studioBadge: a short label on a ribbon.
func (a *App) studioBadge(c *ui.Context) {
	s := &a.studio
	p := design.Of(c)
	ui.Column(c.Key("badge")).Gap(10).Children(func() {
		studioLabel(c, T("studio.badge"))
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			kit.TextField(c.Key("tf"), &s.st.Badge, T("studio.badge"), kit.FieldOpts{Placeholder: T("studio.badge.placeholder")}).Width(150)
			if r := []rune(s.st.Badge); len(r) > config.MaxIconBadge {
				s.st.Badge = string(r[:config.MaxIconBadge])
			}
			for _, h := range studioBadgeColors {
				h := h
				cur := s.st.BadgeColor
				if cur == "" {
					cur = mpicons.DefaultBadgeColor
				}
				sw := ui.ButtonBase(c.Key(h)).Label(h).Size(20, 20).Radius(10).Background(colorOf(h)).Cursor(ui.CursorPointer)
				if cur == h {
					sw.Border(2, p.Accent)
				}
				if sw.Clicked() {
					s.st.BadgeColor = h
					if h == mpicons.DefaultBadgeColor {
						s.st.BadgeColor = ""
					}
				}
			}
		})
	})
}

// studioSurprise picks a random, good-looking style.
func (a *App) studioSurprise() {
	s := &a.studio
	g := mpicons.SuggestedColors()[rand.IntN(len(mpicons.SuggestedColors()))]
	s.c1, s.c2 = colorOf(g[0]), colorOf(g[1])
	bgs := []string{config.BackgroundGradient, config.BackgroundGlass, config.BackgroundGlass, config.BackgroundSolid}
	st := config.IconStyle{Background: bgs[rand.IntN(len(bgs))], Colors: []string{g[0], g[1]}, Badge: s.st.Badge, BadgeColor: s.st.BadgeColor}
	if st.Background == config.BackgroundSolid {
		st.Colors = st.Colors[:1]
	}
	switch n := rand.IntN(3); {
	case n == 0 && mpicons.EmojiAvailable():
		s.emoji = studioEmoji[rand.IntN(len(studioEmoji))]
		st.Glyph, st.Text = config.GlyphEmoji, s.emoji
	case n == 1:
		st.Glyph, st.Text = config.GlyphText, s.letters
		st.Font = studioFonts[rand.IntN(len(studioFonts))]
		if st.Font == "bold" {
			st.Font = ""
		}
	default:
		st.Tint = config.TintWhite
	}
	s.st = st
	s.scale = 64
}

// applyIconStudio saves the style (and a newly chosen image).
func (a *App) applyIconStudio() {
	s := &a.studio
	s.open = false
	st := s.finalStyle()
	if s.projectID == "" {
		a.create.iconStyle = st
		if s.baseNew {
			a.create.iconPNG, a.create.iconCustom = s.base, true
		}
		return
	}
	pr := a.project(s.projectID)
	if pr == nil {
		return
	}
	ed := a.editorFor(pr)
	save := func() {
		ed.cfg.App.Icon.Style = st
		ed.changed()
		a.saveEditor(ed, false, ed.rev)
	}
	if !s.baseNew {
		save()
		return
	}
	id, base := pr.ID, s.base
	var out domain.Project
	a.do(func(ctx context.Context) error {
		var err error
		out, err = a.svc.SetProjectIcon(ctx, id, base, "gui")
		return err
	}, func(err error) {
		if err != nil {
			a.toastErr(err)
			return
		}
		if !ed.dirty {
			ed.cfg = out.Config.Clone()
		}
		ed.rev = out.Revision
		save()
	})
}
