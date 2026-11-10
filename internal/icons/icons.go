// Package icons decodes, normalizes and converts app icons (pure Go).
package icons

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"  // register decoder
	_ "image/jpeg" // register decoder
	"image/png"
	"math"

	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	// CanvasSize is the size of normalized icons.
	CanvasSize = 1024
	// plateMargin is the Apple icon-grid margin around the squircle (1024 canvas).
	plateMargin = 100
	// tinyThreshold: icons smaller than this (on their longest side) are put
	// on a plate so that they look like a proper macOS app icon.
	tinyThreshold = 128
	// maxPixels guards against decompression bombs.
	maxPixels = 64 << 20
)

var pngSig = []byte("\x89PNG\r\n\x1a\n")

// Decode decodes PNG, JPEG, GIF, BMP, WebP and ICO data.
func Decode(data []byte) (image.Image, error) {
	if len(data) < 8 {
		return nil, errors.New("icons: data too short")
	}
	if isICO(data) {
		return decodeICO(data)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("icons: unsupported image: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		return nil, errors.New("icons: image dimensions out of range")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("icons: decode: %w", err)
	}
	return img, nil
}

// EncodePNG encodes img as PNG.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Normalize decodes PNG/JPEG/GIF/ICO/BMP/WebP bitmaps and returns a square
// 1024x1024 PNG (letterboxed on transparent background if not square,
// upscaled with good quality if small, with a rounded "squircle"
// backing plate when the source is tiny like a 16/32px favicon).
func Normalize(data []byte) ([]byte, error) {
	src, err := Decode(data)
	if err != nil {
		return nil, err
	}
	nrgba := toNRGBA(src)
	b := nrgba.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, errors.New("icons: empty image")
	}
	// Trim fully transparent borders so glyphs are centered properly.
	nrgba = trimTransparent(nrgba)
	b = nrgba.Bounds()
	w, h = b.Dx(), b.Dy()
	longest := max(w, h)

	dst := image.NewRGBA(image.Rect(0, 0, CanvasSize, CanvasSize))
	if longest < tinyThreshold {
		plate := plateColors(nrgba)
		drawPlate(dst, plate[0], plate[1])
		inner := plateSize() * 64 / 100 // ~18% padding each side
		fitInto(dst, nrgba, image.Rect(0, 0, CanvasSize, CanvasSize), inner)
	} else {
		fitInto(dst, nrgba, image.Rect(0, 0, CanvasSize, CanvasSize), CanvasSize)
	}
	return EncodePNG(dst)
}

// Resize returns a size x size PNG (aspect ratio kept, centered on a
// transparent canvas when the source is not square).
func Resize(data []byte, size int) ([]byte, error) {
	if size <= 0 || size > 8192 {
		return nil, fmt.Errorf("icons: invalid size %d", size)
	}
	src, err := Decode(data)
	if err != nil {
		return nil, err
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	fitInto(dst, toNRGBA(src), dst.Bounds(), size)
	return EncodePNG(dst)
}

// fitInto scales src to fit a box of side `box` centered in area and draws it.
func fitInto(dst *image.RGBA, src *image.NRGBA, area image.Rectangle, box int) {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	scale := float64(box) / float64(max(sw, sh))
	tw := max(1, int(math.Round(float64(sw)*scale)))
	th := max(1, int(math.Round(float64(sh)*scale)))
	cx := area.Min.X + (area.Dx()-tw)/2
	cy := area.Min.Y + (area.Dy()-th)/2
	r := image.Rect(cx, cy, cx+tw, cy+th)
	if tw == sw && th == sh {
		draw.Draw(dst, r, src, sb.Min, draw.Over)
		return
	}
	// Tiny sources upscale better with a pre-step of nearest-neighbour when
	// the factor is large, which keeps pixel art crisp instead of mushy.
	if scale >= 6 {
		xdraw.NearestNeighbor.Scale(dst, r, src, sb, xdraw.Over, nil)
		return
	}
	xdraw.CatmullRom.Scale(dst, r, src, sb, xdraw.Over, nil)
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

func trimTransparent(img *image.NRGBA) *image.NRGBA {
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A > 8 {
				minX, maxX = min(minX, x), max(maxX, x)
				minY, maxY = min(minY, y), max(maxY, y)
			}
		}
	}
	if maxX < minX {
		return img // fully transparent; leave as is
	}
	r := image.Rect(minX, minY, maxX+1, maxY+1)
	if r == b {
		return img
	}
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
	return out
}

func plateSize() int { return CanvasSize - 2*plateMargin }

// plateColors derives a top/bottom gradient pair from the icon.
func plateColors(img *image.NRGBA) [2]color.NRGBA {
	b := img.Bounds()
	type bin struct {
		n       float64
		r, g, b float64
	}
	bins := map[int]*bin{}
	var sumL, cnt float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A < 128 {
				continue
			}
			l := luminance(c)
			sumL += l
			cnt++
			// ignore near-white and near-black from dominant color, they
			// are usually backgrounds or outlines.
			if l > 0.93 || l < 0.06 {
				continue
			}
			k := int(c.R>>5)<<6 | int(c.G>>5)<<3 | int(c.B>>5)
			bn := bins[k]
			if bn == nil {
				bn = &bin{}
				bins[k] = bn
			}
			// weight saturated colors higher
			mx := max(c.R, max(c.G, c.B))
			mn := min(c.R, min(c.G, c.B))
			wgt := 1 + 3*float64(mx-mn)/255
			bn.n += wgt
			bn.r += float64(c.R) * wgt
			bn.g += float64(c.G) * wgt
			bn.b += float64(c.B) * wgt
		}
	}
	var best *bin
	for _, bn := range bins {
		if best == nil || bn.n > best.n {
			best = bn
		}
	}
	avgL := 0.5
	if cnt > 0 {
		avgL = sumL / cnt
	}
	dom := color.NRGBA{0x4a, 0x55, 0x78, 255}
	if best != nil {
		dom = color.NRGBA{uint8(best.r / best.n), uint8(best.g / best.n), uint8(best.b / best.n), 255}
	}
	if avgL > 0.82 || best == nil && avgL > 0.6 {
		// Mostly light glyph: a dark plate keeps it visible.
		return [2]color.NRGBA{mix(dom, color.NRGBA{0, 0, 0, 255}, 0.45), mix(dom, color.NRGBA{0, 0, 0, 255}, 0.62)}
	}
	// Lightened dominant color, soft vertical gradient.
	return [2]color.NRGBA{mix(dom, color.NRGBA{255, 255, 255, 255}, 0.90), mix(dom, color.NRGBA{255, 255, 255, 255}, 0.78)}
}

func luminance(c color.NRGBA) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

// mix returns a*(1-t)+b*t.
func mix(a, b color.NRGBA, t float64) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*(1-t) + float64(y)*t)) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}

// drawPlate fills dst (1024 canvas) with a macOS-style rounded-rect plate with
// a vertical gradient top→bottom.
func drawPlate(dst *image.RGBA, top, bottom color.NRGBA) {
	size := float64(plateSize())
	radius := size * 0.2237
	c := float64(CanvasSize) / 2
	for y := 0; y < CanvasSize; y++ {
		t := (float64(y) - plateMargin) / size
		t = math.Max(0, math.Min(1, t))
		col := lerpColor(top, bottom, t)
		for x := 0; x < CanvasSize; x++ {
			a := roundRectCoverage(float64(x)+0.5-c, float64(y)+0.5-c, size/2, radius)
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

func lerpColor(a, b color.NRGBA, t float64) color.NRGBA {
	return mix(a, b, t)
}

// roundRectCoverage returns anti-aliased coverage (0..1) of point (px,py),
// relative to the rect center, for a rounded rect of half-size `half`.
func roundRectCoverage(px, py, half, radius float64) float64 {
	qx := math.Abs(px) - (half - radius)
	qy := math.Abs(py) - (half - radius)
	outside := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
	inside := math.Min(math.Max(qx, qy), 0)
	d := outside + inside - radius // signed distance, <0 inside
	return math.Max(0, math.Min(1, 0.5-d))
}
