package icons

import (
	"errors"
	"image"
	"image/color"
	"image/draw"

	xdraw "golang.org/x/image/draw"
)

// MacAppIcon returns full-bleed, opaque 1024×1024 artwork for a macOS app
// bundle. macOS 26+ applies its own rounded-rect mask to app icons; artwork
// that already carries rounded corners and transparent margins is instead
// shrunk onto a light "jail" plate. So:
//
//   - icons that already look like app icons (opaque edges, rounded or
//     transparent corners — e.g. our placeholders or a site's touch icon)
//     are scaled to fill the canvas, with the corners filled from the
//     artwork's own edge colors;
//   - logos on a transparent background are centered on a full-bleed
//     gradient derived from their colors.
func MacAppIcon(data []byte) ([]byte, error) {
	src, err := Decode(data)
	if err != nil {
		return nil, err
	}
	img := trimTransparent(toNRGBA(src))
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return nil, errors.New("icons: empty image")
	}
	dst := image.NewRGBA(image.Rect(0, 0, CanvasSize, CanvasSize))
	full := dst.Bounds()

	if looksLikeAppIcon(img) {
		// Crop 2.5% so the artwork's own edge highlight and rounded rim fall
		// outside the system mask.
		in := b.Dx() * 25 / 1000
		crop := image.Rect(b.Min.X+in, b.Min.Y+in, b.Max.X-in, b.Max.Y-in)
		scaled := image.NewNRGBA(full)
		xdraw.CatmullRom.Scale(scaled, full, img, crop, xdraw.Src, nil)
		fillBackground(dst, cornerColors(scaled))
		draw.Draw(dst, full, scaled, image.Point{}, draw.Over)
		return EncodePNG(dst)
	}

	// A bare logo: full-bleed plate, glyph at ~64% like Apple's grid.
	plate := plateColors(img)
	fillBackground(dst, [4]color.NRGBA{plate[0], plate[0], plate[1], plate[1]})
	fitInto(dst, img, full, CanvasSize*64/100)
	return EncodePNG(dst)
}

// looksLikeAppIcon reports whether the image's edge midpoints are opaque —
// i.e. it is a filled (possibly rounded) square rather than a free-form logo.
func looksLikeAppIcon(img *image.NRGBA) bool {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 16 || h < 16 {
		return false
	}
	in := 2 + w/64
	pts := []image.Point{
		{b.Min.X + w/2, b.Min.Y + in}, {b.Min.X + w/2, b.Max.Y - 1 - in},
		{b.Min.X + in, b.Min.Y + h/2}, {b.Max.X - 1 - in, b.Min.Y + h/2},
		{b.Min.X + w/2, b.Min.Y + h/2},
	}
	// 10% in along each diagonal is inside a rounded square (corner radius
	// ≤ ~22%) but outside a circle — circular logos are not app icons.
	dx, dy := w/10, h/10
	pts = append(pts,
		image.Point{b.Min.X + dx, b.Min.Y + dy}, image.Point{b.Max.X - 1 - dx, b.Min.Y + dy},
		image.Point{b.Min.X + dx, b.Max.Y - 1 - dy}, image.Point{b.Max.X - 1 - dx, b.Max.Y - 1 - dy})
	for _, p := range pts {
		if img.NRGBAAt(p.X, p.Y).A < 230 {
			return false
		}
	}
	// Roughly square: app icons are; banners/wordmarks are not.
	r := float64(w) / float64(h)
	return r > 0.85 && r < 1.18
}

// cornerColors samples opaque colors just inside each corner along the
// diagonals (top-left, top-right, bottom-left, bottom-right).
func cornerColors(img *image.NRGBA) [4]color.NRGBA {
	s := CanvasSize
	at := func(fx, fy float64) color.NRGBA {
		c := img.NRGBAAt(int(fx*float64(s)), int(fy*float64(s)))
		c.A = 255
		return c
	}
	const k = 0.12 // inside a ~22% corner radius
	return [4]color.NRGBA{at(k, k), at(1-k, k), at(k, 1-k), at(1-k, 1-k)}
}

// fillBackground paints a bilinear gradient between four corner colors.
func fillBackground(dst *image.RGBA, c [4]color.NRGBA) {
	n := float64(CanvasSize - 1)
	for y := 0; y < CanvasSize; y++ {
		ty := float64(y) / n
		left, right := mix(c[0], c[2], ty), mix(c[1], c[3], ty)
		for x := 0; x < CanvasSize; x++ {
			col := mix(left, right, float64(x)/n)
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = col.R, col.G, col.B, 255
		}
	}
}
