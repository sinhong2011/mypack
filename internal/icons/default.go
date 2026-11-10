package icons

import (
	"hash/fnv"
	"image"
	"image/color"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

type gradient struct{ top, bottom color.NRGBA }

func hex(s string) color.NRGBA {
	var v [3]uint8
	for i := 0; i < 3; i++ {
		var n uint8
		for _, ch := range s[1+2*i : 3+2*i] {
			n <<= 4
			switch {
			case ch >= '0' && ch <= '9':
				n |= uint8(ch - '0')
			default:
				n |= uint8(ch-'a') + 10
			}
		}
		v[i] = n
	}
	return color.NRGBA{v[0], v[1], v[2], 255}
}

// palette: tasteful, saturated-but-calm gradients; white letters stay legible.
var palette = []gradient{
	{hex("#7c83fd"), hex("#4b4fd1")}, // indigo
	{hex("#2dd4bf"), hex("#0f8f8a")}, // teal
	{hex("#ff8a7a"), hex("#e5484d")}, // coral
	{hex("#b38cff"), hex("#7c3aed")}, // violet
	{hex("#34d399"), hex("#059669")}, // emerald
	{hex("#fbbf24"), hex("#e07b0c")}, // amber
	{hex("#7a8fb8"), hex("#46567d")}, // slate blue
	{hex("#60a5fa"), hex("#2563eb")}, // blue
	{hex("#f472b6"), hex("#be185d")}, // rose
	{hex("#a3c653"), hex("#5b8a1f")}, // lime
	{hex("#38bdf8"), hex("#0e7fb8")}, // sky
	{hex("#fb923c"), hex("#c2410c")}, // orange
}

var (
	faceOnce sync.Once
	face     font.Face
	faceErr  error
	otFont   *opentype.Font
)

func letterFace() (font.Face, error) {
	faceOnce.Do(func() {
		otFont, faceErr = opentype.Parse(gobold.TTF)
		if faceErr != nil {
			return
		}
		face, faceErr = opentype.NewFace(otFont, &opentype.FaceOptions{Size: 560, DPI: 72, Hinting: font.HintingNone})
	})
	return face, faceErr
}

// PaletteIndex returns the deterministic palette slot for a name.
func PaletteIndex(name string) int {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(name))))
	return int(h.Sum32() % uint32(len(palette)))
}

// firstLetter picks the glyph to show: first letter/digit of the name,
// upper-cased; falls back to the first ASCII alphanumeric, else 0.
func firstLetter(name string) rune {
	var fallback rune
	for _, r := range strings.TrimSpace(name) {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			continue
		}
		r = unicode.ToUpper(r)
		if r < 0x250 {
			return r
		}
		if fallback == 0 {
			fallback = r
		}
	}
	return fallback
}

// Default renders a tasteful placeholder icon: macOS-style rounded square
// with a gradient derived from the name and the first letter in white.
func Default(name string) []byte {
	g := palette[PaletteIndex(name)]
	dst := image.NewRGBA(image.Rect(0, 0, CanvasSize, CanvasSize))
	drawPlate(dst, g.top, g.bottom)
	letter := firstLetter(name)
	drawn := false
	if f, err := letterFace(); err == nil && letter != 0 {
		var sb sfnt.Buffer
		if idx, _ := otFont.GlyphIndex(&sb, letter); idx != 0 {
			drawn = drawLetter(dst, f, string(letter))
		}
	}
	if !drawn {
		drawFallbackGlyph(dst)
	}
	b, err := EncodePNG(dst)
	if err != nil {
		return nil
	}
	return b
}

func drawLetter(dst *image.RGBA, f font.Face, s string) bool {
	bounds, _ := font.BoundString(f, s)
	w := bounds.Max.X - bounds.Min.X
	h := bounds.Max.Y - bounds.Min.Y
	if w <= 0 || h <= 0 {
		return false
	}
	x := fixed.I(CanvasSize/2) - w/2 - bounds.Min.X
	y := fixed.I(CanvasSize/2) - (bounds.Min.Y+bounds.Max.Y)/2
	// soft drop shadow, then the letter
	for _, p := range []struct {
		dy int
		a  uint8
	}{{10, 40}, {6, 40}} {
		d := font.Drawer{Dst: dst, Src: image.NewUniform(color.NRGBA{0, 0, 0, p.a}), Face: f,
			Dot: fixed.Point26_6{X: x, Y: y + fixed.I(p.dy)}}
		d.DrawString(s)
	}
	d := font.Drawer{Dst: dst, Src: image.White, Face: f, Dot: fixed.Point26_6{X: x, Y: y}}
	d.DrawString(s)
	return true
}

// drawFallbackGlyph draws a white ring + dot (for names with no renderable letter).
func drawFallbackGlyph(dst *image.RGBA) {
	c := float64(CanvasSize) / 2
	for y := 0; y < CanvasSize; y++ {
		for x := 0; x < CanvasSize; x++ {
			dx, dy := float64(x)+0.5-c, float64(y)+0.5-c
			d := hypot(dx, dy)
			ring := clamp01(30 - absf(d-230) + 0.5)
			dot := clamp01(95 - d + 0.5)
			a := max(ring, dot)
			if a <= 0 {
				continue
			}
			i := dst.PixOffset(x, y)
			bg := dst.Pix[i+3]
			if bg == 0 {
				continue
			}
			f := a * float64(bg) / 255
			for k := 0; k < 3; k++ {
				dst.Pix[i+k] = uint8(float64(dst.Pix[i+k])*(1-a) + 255*f)
			}
		}
	}
}
