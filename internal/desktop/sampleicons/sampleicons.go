// Package sampleicons draws neutral, generated macOS-style app icons (a
// gradient squircle with a simple white glyph) for the demo service and the
// Discover presets. The glyphs are generic shapes, never a site's logo.
package sampleicons

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

const (
	canvas = 512
	margin = 50 // the macOS icon-grid margin (10%)
)

type style struct {
	top, bottom color.NRGBA
	glyph       string // SVG body in a 24×24 box, white
}

func rgb(r, g, b uint8) color.NRGBA { return color.NRGBA{r, g, b, 255} }

const (
	gMusic   = `<path d="M9 18V5.5l11-2V16" fill="none" stroke="#fff" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/><circle cx="6.5" cy="18" r="2.8" fill="#fff"/><circle cx="17.5" cy="16" r="2.8" fill="#fff"/>`
	gChat    = `<path d="M12 3.5c5 0 9 3.4 9 7.7s-4 7.7-9 7.7c-1 0-2-.1-2.9-.4L4.5 20.5l1.2-3.9C4 15.2 3 13.3 3 11.2 3 6.9 7 3.5 12 3.5z" fill="#fff"/>`
	gDoc     = `<rect x="5" y="3" width="14" height="18" rx="2.5" fill="#fff"/><path d="M8.5 8h7M8.5 11.5h7M8.5 15h4.5" stroke-width="1.7" stroke-linecap="round" stroke="currentColor" opacity=".45"/>`
	gShapes  = `<circle cx="8.5" cy="8.5" r="4.5" fill="#fff"/><rect x="11" y="11" width="9.5" height="9.5" rx="2.2" fill="#fff" opacity=".78"/>`
	gTarget  = `<circle cx="12" cy="12" r="8.5" fill="none" stroke="#fff" stroke-width="2.2"/><path d="M7.5 12.5l3 3 6-7" fill="none" stroke="#fff" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"/>`
	gSpark   = `<path d="M12 2.5c.6 4.6 2.9 6.9 7.5 7.5-4.6.6-6.9 2.9-7.5 7.5-.6-4.6-2.9-6.9-7.5-7.5 4.6-.6 6.9-2.9 7.5-7.5z" fill="#fff"/><path d="M18.5 15.5c.3 2 1.2 2.9 3.2 3.2-2 .3-2.9 1.2-3.2 3.2-.3-2-1.2-2.9-3.2-3.2 2-.3 2.9-1.2 3.2-3.2z" fill="#fff" opacity=".8"/>`
	gCode    = `<path d="M8.5 7 3.5 12l5 5M15.5 7l5 5-5 5" fill="none" stroke="#fff" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"/>`
	gMail    = `<rect x="3" y="5.5" width="18" height="13" rx="2.6" fill="#fff"/><path d="M4 7.5l8 6 8-6" fill="none" stroke-width="1.8" stroke="currentColor" opacity=".4" stroke-linejoin="round"/>`
	gHash    = `<path d="M9.5 4 8 20M16 4l-1.5 16M4.5 9h16M3.5 15h16" fill="none" stroke="#fff" stroke-width="2.3" stroke-linecap="round"/>`
	gWave    = `<path d="M3 12h2.5l2-5 3 10 3-12 3 10 2-3H21" fill="none" stroke="#fff" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>`
	gGlobe   = `<circle cx="12" cy="12" r="8.5" fill="none" stroke="#fff" stroke-width="2"/><path d="M3.5 12h17M12 3.5c2.6 2.6 3.6 5.4 3.6 8.5s-1 5.9-3.6 8.5c-2.6-2.6-3.6-5.4-3.6-8.5S9.4 6.1 12 3.5z" fill="none" stroke="#fff" stroke-width="1.8"/>`
	gPlay    = `<rect x="3" y="5" width="18" height="14" rx="4" fill="#fff"/><path d="M10.2 9.2v5.6l4.8-2.8z" fill="currentColor" opacity=".55"/>`
	gCompass = `<circle cx="12" cy="12" r="8.5" fill="none" stroke="#fff" stroke-width="2"/><path d="M15.5 8.5l-2 5-5 2 2-5z" fill="#fff"/>`
)

var styles = map[string]style{
	"linear":        {rgb(122, 132, 255), rgb(76, 72, 210), gTarget},
	"youtube music": {rgb(255, 104, 112), rgb(214, 30, 62), gMusic},
	"notion":        {rgb(120, 124, 134), rgb(52, 54, 62), gDoc},
	"figma":         {rgb(186, 120, 255), rgb(120, 60, 230), gShapes},
	"whatsapp":      {rgb(92, 226, 140), rgb(24, 160, 92), gChat},
	"chatgpt":       {rgb(64, 214, 178), rgb(14, 140, 120), gSpark},
	"github":        {rgb(96, 102, 116), rgb(34, 38, 48), gCode},
	"gmail":         {rgb(255, 128, 104), rgb(222, 64, 52), gMail},
	"slack":         {rgb(214, 96, 196), rgb(140, 44, 140), gHash},
	"discord":       {rgb(132, 140, 255), rgb(82, 90, 232), gWave},
	"youtube":       {rgb(255, 92, 92), rgb(204, 24, 24), gPlay},
	"safari":        {rgb(98, 196, 255), rgb(24, 120, 232), gCompass},
}

var fallbacks = []style{
	{rgb(98, 160, 255), rgb(36, 96, 236), gGlobe},
	{rgb(255, 176, 84), rgb(236, 110, 36), gGlobe},
	{rgb(80, 214, 196), rgb(16, 146, 140), gGlobe},
	{rgb(170, 140, 255), rgb(106, 70, 226), gGlobe},
}

// Known reports whether name has a dedicated generated icon.
func Known(name string) bool {
	_, ok := styles[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

var (
	mu    sync.Mutex
	cache = map[string][]byte{}
)

// Icon returns a 512 px PNG app icon for name (cached).
func Icon(name string) []byte {
	key := strings.ToLower(strings.TrimSpace(name))
	mu.Lock()
	defer mu.Unlock()
	if b, ok := cache[key]; ok {
		return b
	}
	st, ok := styles[key]
	if !ok {
		h := 0
		for _, r := range key {
			h = h*31 + int(r)
		}
		st = fallbacks[(h%len(fallbacks)+len(fallbacks))%len(fallbacks)]
	}
	b := render(st)
	cache[key] = b
	return b
}

func render(st style) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, canvas, canvas))
	size := float64(canvas - 2*margin)
	half := size / 2
	c := float64(canvas) / 2
	const n = 5.0 // superellipse exponent: Apple's continuous squircle
	for y := 0; y < canvas; y++ {
		t := clamp((float64(y)-margin)/size, 0, 1)
		col := lerp(st.top, st.bottom, t)
		for x := 0; x < canvas; x++ {
			dx, dy := math.Abs(float64(x)+0.5-c)/half, math.Abs(float64(y)+0.5-c)/half
			d := math.Pow(math.Pow(dx, n)+math.Pow(dy, n), 1/n)
			cov := clamp((1-d)*half+0.5, 0, 1)
			if cov <= 0 {
				continue
			}
			px := col
			// A soft gloss on the upper half and a lit rim at the top.
			gloss := clamp(1-(float64(y)-margin)/(size*0.55), 0, 1) * 0.12
			rim := clamp(1-(1-d)*half/3, 0, 1) * clamp(1-(float64(y)-margin)/(size*0.4), 0, 1) * 0.35
			px = lerp(px, color.NRGBA{255, 255, 255, 255}, gloss+rim)
			px.A = uint8(cov * 255)
			img.SetNRGBA(x, y, px)
		}
	}
	svg, err := ui.ParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" color="#000">` + st.glyph + `</svg>`))
	if err == nil {
		g := int(size * 0.56)
		off := (canvas - g) / 2
		shadow := svg.Image(g, g, color.Black)
		for i := 3; i < len(shadow.Pix); i += 4 {
			shadow.Pix[i] = uint8(int(shadow.Pix[i]) * 50 / 255)
			shadow.Pix[i-1], shadow.Pix[i-2], shadow.Pix[i-3] = 0, 0, 0
		}
		draw.Draw(img, image.Rect(off, off+7, off+g, off+g+7), shadow, image.Point{}, draw.Over)
		glyph := svg.Image(g, g, st.bottom)
		draw.Draw(img, image.Rect(off, off, off+g, off+g), glyph, image.Point{}, draw.Over)
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func lerp(a, b color.NRGBA, t float64) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}
