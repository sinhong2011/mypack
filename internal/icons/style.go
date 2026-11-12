package icons

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"mypack.dev/mypack/pkg/config"
)

// The icon studio: Styled draws an app icon from a style (config.IconStyle)
// on the same rounded plate and margins as Default, so every later step
// (MacAppIcon, ICO, Linux PNGs) treats it like any other icon.

// DefaultBadgeColor is the badge ribbon's colour when the style names none.
const DefaultBadgeColor = "#e5484d"

// Styled renders base (the icon's own image; may be nil) in style st as a
// size×size PNG. name picks colours and the fallback letter.
func Styled(base []byte, st config.IconStyle, name string, size int) ([]byte, error) {
	img := StyledImage(base, st, name, size)
	return EncodePNG(img)
}

// StyledImage is Styled without the PNG encoding.
func StyledImage(base []byte, st config.IconStyle, name string, size int) *image.RGBA {
	if size <= 0 || size > 4096 {
		size = CanvasSize
	}
	k := float64(size) / CanvasSize
	margin := int(math.Round(plateMargin * k))
	plate := size - 2*margin
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	full := dst.Bounds()

	glyph, kind := styleGlyph(base, st, name)
	bg := st.Background
	if bg == "" {
		bg = config.BackgroundAuto
	}
	scale := st.Scale
	if bg == config.BackgroundNone {
		// The image as it is (full canvas, like Normalize), or a bare glyph.
		if scale == 0 {
			scale = 100
			if kind != glyphImage {
				scale = 80
			}
		}
		g := tinted(glyph, st, kind, false)
		box := size * scale / 100
		layer := image.NewRGBA(full)
		fitSmooth(layer, g, full, box)
		draw.Draw(dst, full, layer, image.Point{}, draw.Over)
		drawBadge(dst, st, size, margin, plate)
		return dst
	}

	top, bottom := styleColors(st, glyph, kind, name, bg)
	drawPlateAt(dst, size, margin, top, bottom)
	if bg == config.BackgroundGlass {
		drawGlass(dst, size, margin)
	}
	if scale == 0 {
		scale = 64
		if kind == glyphText {
			scale = 58
		}
	}
	light := luminance(mix(top, bottom, 0.5)) > 0.72
	g := tinted(glyph, st, kind, light)
	box := plate * scale / 100
	layer := image.NewRGBA(full)
	fitSmooth(layer, g, full, box)
	if bg == config.BackgroundGradient || bg == config.BackgroundGlass || kind != glyphImage {
		drawSoftShadow(dst, layer, size, 0.22)
	}
	draw.Draw(dst, full, layer, image.Point{}, draw.Over)
	drawBadge(dst, st, size, margin, plate)
	return dst
}

type glyphKind int

const (
	glyphImage glyphKind = iota
	glyphText
	glyphEmoji
)

// styleGlyph is what sits in the middle, trimmed to its visible pixels.
func styleGlyph(base []byte, st config.IconStyle, name string) (*image.NRGBA, glyphKind) {
	text := strings.TrimSpace(st.Text)
	switch st.Glyph {
	case config.GlyphEmoji:
		if img := emojiImage(text); img != nil {
			return trimTransparent(toNRGBA(img)), glyphEmoji
		}
		return letterGlyph(monogram(name), st.Font), glyphText
	case config.GlyphText:
		if text == "" {
			text = monogram(name)
		}
		return letterGlyph(text, st.Font), glyphText
	}
	if len(base) > 0 {
		if img, err := Decode(base); err == nil {
			n := trimTransparent(toNRGBA(img))
			if n.Bounds().Dx() > 0 {
				return n, glyphImage
			}
		}
	}
	return letterGlyph(monogram(name), st.Font), glyphText
}

func monogram(name string) string {
	if r := firstLetter(name); r != 0 {
		return string(r)
	}
	return "•"
}

// styleColors picks the background's top and bottom colours.
func styleColors(st config.IconStyle, glyph *image.NRGBA, kind glyphKind, name, bg string) (top, bottom color.NRGBA) {
	switch {
	case len(st.Colors) >= 2:
		top, bottom = parseHex(st.Colors[0]), parseHex(st.Colors[1])
	case len(st.Colors) == 1:
		c := parseHex(st.Colors[0])
		top, bottom = mix(c, color.NRGBA{255, 255, 255, 255}, 0.14), mix(c, color.NRGBA{0, 0, 0, 255}, 0.16)
	case kind == glyphImage:
		pc := plateColors(glyph)
		top, bottom = pc[0], pc[1]
		if bg == config.BackgroundGradient || bg == config.BackgroundGlass {
			// The soft auto plate is nearly flat: deepen it into a real gradient.
			top, bottom = mix(pc[0], color.NRGBA{255, 255, 255, 255}, 0.25), mix(pc[1], color.NRGBA{0, 0, 0, 255}, 0.12)
		}
	default:
		g := palette[PaletteIndex(name)]
		top, bottom = g.top, g.bottom
	}
	if bg == config.BackgroundSolid {
		if len(st.Colors) > 0 {
			top = parseHex(st.Colors[0])
		}
		bottom = top
	}
	return top, bottom
}

// tinted applies the style's tint to the glyph (letters default to white,
// or near-black on a light background).
func tinted(g *image.NRGBA, st config.IconStyle, kind glyphKind, lightBG bool) *image.NRGBA {
	var c color.NRGBA
	switch st.Tint {
	case config.TintWhite:
		c = color.NRGBA{255, 255, 255, 255}
	case config.TintBlack:
		c = color.NRGBA{0x1d, 0x1d, 0x1f, 255}
	case config.TintColor:
		c = parseHex(st.TintColor)
	default:
		if kind != glyphText {
			return g
		}
		c = color.NRGBA{255, 255, 255, 255}
		if lightBG {
			c = color.NRGBA{0x1d, 0x1d, 0x1f, 255}
		}
	}
	out := image.NewNRGBA(g.Bounds())
	for i := 0; i+3 < len(g.Pix); i += 4 {
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = c.R, c.G, c.B, g.Pix[i+3]
	}
	return out
}

// ---- letters ---------------------------------------------------------------

var (
	fontMu    sync.Mutex
	fontCache = map[string]*sfnt.Font{}
)

// Fallbacks for letters the Go fonts lack (Chinese, Japanese, Korean…).
var fallbackFonts = []string{
	"/System/Library/Fonts/Hiragino Sans GB.ttc",
	"/System/Library/Fonts/AppleSDGothicNeo.ttc",
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc",
	"/usr/share/fonts/noto-cjk/NotoSansCJK-Bold.ttc",
	`C:\Windows\Fonts\msyh.ttc`,
}

func loadFont(key string) *sfnt.Font {
	fontMu.Lock()
	defer fontMu.Unlock()
	if f, ok := fontCache[key]; ok {
		return f
	}
	var f *sfnt.Font
	switch key {
	case "bold":
		f, _ = opentype.Parse(gobold.TTF)
	case "regular":
		f, _ = opentype.Parse(goregular.TTF)
	case "mono":
		f, _ = opentype.Parse(gomonobold.TTF)
	default: // a file path
		if b, err := os.ReadFile(key); err == nil {
			if c, err := opentype.ParseCollection(b); err == nil && c.NumFonts() > 0 {
				// Collections list weights light to heavy: take the second
				// (W6 in Hiragino), which reads like the bold Go font.
				f, _ = c.Font(min(1, c.NumFonts()-1))
			}
		}
	}
	fontCache[key] = f
	return f
}

// fontFor picks the font that has every letter of s.
func fontFor(s, name string) *sfnt.Font {
	keys := []string{"bold"}
	switch name {
	case "regular", "mono":
		keys = []string{name, "bold"}
	case "rounded":
		keys = []string{"/System/Library/Fonts/SFNSRounded.ttf", "bold"}
	}
	keys = append(keys, fallbackFonts...)
	var sb sfnt.Buffer
	for _, k := range keys {
		f := loadFont(k)
		if f == nil {
			continue
		}
		ok := true
		for _, r := range s {
			if i, err := f.GlyphIndex(&sb, r); err != nil || i == 0 {
				ok = false
				break
			}
		}
		if ok {
			return f
		}
	}
	return loadFont("bold")
}

// renderText draws s in white at px pixels, trimmed to its ink.
func renderText(s, fontName string, px float64) *image.NRGBA {
	f := fontFor(s, fontName)
	if f == nil {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	defer face.Close()
	b, adv := font.BoundString(face, s)
	w := max((b.Max.X-b.Min.X).Ceil(), adv.Ceil()) + 4
	h := (b.Max.Y - b.Min.Y).Ceil() + 4
	if w <= 4 || h <= 4 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	d := font.Drawer{Dst: img, Src: image.White, Face: face,
		Dot: fixed.Point26_6{X: -b.Min.X + fixed.I(2), Y: -b.Min.Y + fixed.I(2)}}
	d.DrawString(s)
	return trimTransparent(img)
}

func letterGlyph(s, fontName string) *image.NRGBA {
	if utf8.RuneCountInString(s) > config.MaxIconText {
		s = string([]rune(s)[:config.MaxIconText])
	}
	return renderText(s, fontName, 480)
}

// ---- drawing ---------------------------------------------------------------

// fitSmooth is fitInto that always resamples smoothly: a styled glyph is a
// shape on a plate, where blocky pixel-art upscaling looks broken.
func fitSmooth(dst *image.RGBA, src *image.NRGBA, area image.Rectangle, box int) {
	sb := src.Bounds()
	scale := float64(box) / float64(max(sb.Dx(), sb.Dy()))
	tw := max(1, int(math.Round(float64(sb.Dx())*scale)))
	th := max(1, int(math.Round(float64(sb.Dy())*scale)))
	cx := area.Min.X + (area.Dx()-tw)/2
	cy := area.Min.Y + (area.Dy()-th)/2
	xdraw.CatmullRom.Scale(dst, image.Rect(cx, cy, cx+tw, cy+th), src, sb, xdraw.Over, nil)
}

// drawPlateAt is drawPlate for any canvas size.
func drawPlateAt(dst *image.RGBA, size, margin int, top, bottom color.NRGBA) {
	ps := float64(size - 2*margin)
	radius := ps * 0.2237
	c := float64(size) / 2
	for y := 0; y < size; y++ {
		t := math.Max(0, math.Min(1, (float64(y)-float64(margin))/ps))
		col := mix(top, bottom, t)
		for x := 0; x < size; x++ {
			a := roundRectCoverage(float64(x)+0.5-c, float64(y)+0.5-c, ps/2, radius)
			if a <= 0 {
				continue
			}
			i := dst.PixOffset(x, y)
			al := uint32(math.Round(a * 255))
			dst.Pix[i+0] = uint8(uint32(col.R) * al / 255)
			dst.Pix[i+1] = uint8(uint32(col.G) * al / 255)
			dst.Pix[i+2] = uint8(uint32(col.B) * al / 255)
			dst.Pix[i+3] = uint8(al)
		}
	}
}

// drawGlass adds a frosted sheen on the upper half of the plate and a thin
// light rim, like a pane of glass.
func drawGlass(dst *image.RGBA, size, margin int) {
	ps := float64(size - 2*margin)
	radius := ps * 0.2237
	c := float64(size) / 2
	rimW := math.Max(1.2, ps*0.012)
	for y := margin; y < size-margin; y++ {
		t := (float64(y) - float64(margin)) / ps
		sheen := 0.0
		if t < 0.52 {
			u := 1 - t/0.52
			sheen = 0.30 * u * u * (3 - 2*u)
		}
		rimA := 0.12 + 0.5*(1-t)
		for x := margin; x < size-margin; x++ {
			px, py := float64(x)+0.5-c, float64(y)+0.5-c
			cov := roundRectCoverage(px, py, ps/2, radius)
			if cov <= 0 {
				continue
			}
			inner := roundRectCoverage(px, py, ps/2-rimW, radius-rimW)
			a := sheen*inner + rimA*math.Max(0, cov-inner)
			if a <= 0 {
				continue
			}
			i := dst.PixOffset(x, y)
			A := float64(dst.Pix[i+3])
			for k := 0; k < 3; k++ {
				v := float64(dst.Pix[i+k])
				dst.Pix[i+k] = uint8(v + (A-v)*a)
			}
		}
	}
}

// drawSoftShadow draws a blurred, dimmed copy of layer's ink under it,
// clipped to what is already painted (the plate).
func drawSoftShadow(dst, layer *image.RGBA, size int, strength float64) {
	n := max(8, size/20)
	small := image.NewRGBA(image.Rect(0, 0, n, n))
	xdraw.ApproxBiLinear.Scale(small, small.Bounds(), layer, layer.Bounds(), xdraw.Src, nil)
	blur := image.NewRGBA(dst.Bounds())
	off := size * 15 / 1000
	r := dst.Bounds().Add(image.Pt(0, off))
	xdraw.BiLinear.Scale(blur, r, small, small.Bounds(), xdraw.Src, nil)
	for i := 0; i+3 < len(dst.Pix); i += 4 {
		A := dst.Pix[i+3]
		if A == 0 {
			continue
		}
		a := float64(blur.Pix[i+3]) / 255 * strength * float64(A) / 255
		if a <= 0 {
			continue
		}
		for k := 0; k < 3; k++ {
			dst.Pix[i+k] = uint8(float64(dst.Pix[i+k]) * (1 - a))
		}
	}
}

// drawBadge draws the style's badge: a pill at the bottom of the plate.
func drawBadge(dst *image.RGBA, st config.IconStyle, size, margin, plate int) {
	text := strings.TrimSpace(st.Badge)
	if text == "" {
		return
	}
	bc := parseHex(DefaultBadgeColor)
	if config.IsHexColor(st.BadgeColor) {
		bc = parseHex(st.BadgeColor)
	}
	fg := color.NRGBA{255, 255, 255, 255}
	if luminance(bc) > 0.7 {
		fg = color.NRGBA{0x1d, 0x1d, 0x1f, 255}
	}
	ps := float64(plate)
	h := ps * 0.2
	txt := renderText(strings.ToUpper(text), "bold", h*0.62)
	maxW := ps * 0.86
	tw, th := float64(txt.Bounds().Dx()), float64(txt.Bounds().Dy())
	// Text box: cap height ~42% of the pill, never wider than the pill allows.
	sc := math.Min(h*0.42/math.Max(th, 1), (maxW-h*0.8)/math.Max(tw, 1))
	tw, th = tw*sc, th*sc
	w := math.Min(maxW, tw+h*0.8)
	cx := float64(size) / 2
	cy := float64(margin) + ps - ps*0.06 - h/2
	outline := math.Max(1, ps*0.012)
	paintPill(dst, cx, cy, w/2+outline, h/2+outline, color.NRGBA{255, 255, 255, 235})
	paintPill(dst, cx, cy, w/2, h/2, bc)
	r := image.Rect(int(math.Round(cx-tw/2)), int(math.Round(cy-th/2)), int(math.Round(cx+tw/2)), int(math.Round(cy+th/2)))
	if r.Dx() < 1 || r.Dy() < 1 {
		return
	}
	mask := image.NewNRGBA(r)
	xdraw.CatmullRom.Scale(mask, r, txt, txt.Bounds(), xdraw.Src, nil)
	draw.DrawMask(dst, r, image.NewUniform(fg), image.Point{}, mask, r.Min, draw.Over)
}

// paintPill composites a solid pill (fully rounded rect) centered at cx,cy.
func paintPill(dst *image.RGBA, cx, cy, hw, hh float64, col color.NRGBA) {
	b := dst.Bounds()
	x0, x1 := max(b.Min.X, int(cx-hw)-1), min(b.Max.X, int(cx+hw)+2)
	y0, y1 := max(b.Min.Y, int(cy-hh)-1), min(b.Max.Y, int(cy+hh)+2)
	ca := float64(col.A) / 255
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			a := rectCoverage(float64(x)+0.5-cx, float64(y)+0.5-cy, hw, hh, hh) * ca
			if a <= 0 {
				continue
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i+0] = uint8(float64(col.R)*a + float64(dst.Pix[i+0])*(1-a))
			dst.Pix[i+1] = uint8(float64(col.G)*a + float64(dst.Pix[i+1])*(1-a))
			dst.Pix[i+2] = uint8(float64(col.B)*a + float64(dst.Pix[i+2])*(1-a))
			dst.Pix[i+3] = uint8(255*a + float64(dst.Pix[i+3])*(1-a))
		}
	}
}

// rectCoverage is roundRectCoverage for a rect of half-size hw×hh.
func rectCoverage(px, py, hw, hh, radius float64) float64 {
	qx := math.Abs(px) - (hw - radius)
	qy := math.Abs(py) - (hh - radius)
	outside := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
	inside := math.Min(math.Max(qx, qy), 0)
	d := outside + inside - radius
	return math.Max(0, math.Min(1, 0.5-d))
}

// parseHex reads "#rrggbb" (anything else is a neutral grey).
func parseHex(s string) color.NRGBA {
	if !config.IsHexColor(s) {
		return color.NRGBA{0x8e, 0x8e, 0x93, 255}
	}
	return hex(strings.ToLower(s))
}

// SuggestedColors are the studio's ready-made backgrounds (top, bottom), the
// same calm gradients the default icons use.
func SuggestedColors() [][2]string {
	out := make([][2]string, len(palette))
	for i, g := range palette {
		out[i] = [2]string{hexOf(g.top), hexOf(g.bottom)}
	}
	return out
}

func hexOf(c color.NRGBA) string {
	const d = "0123456789abcdef"
	return string([]byte{'#', d[c.R>>4], d[c.R&15], d[c.G>>4], d[c.G&15], d[c.B>>4], d[c.B&15]})
}
