package design

import (
	"image"
	"image/color"
	"math/rand/v2"

	"github.com/egoist/mygo/ui"
)

// Texture tokens: a fine film grain on the window chrome, a dot grid used
// as the "workbench" under illustrated moments, and hairlines. Used
// sparingly: texture is felt, not seen.

const (
	grainTile  = 160
	DotSpacing = float32(14)
	DotRadius  = float32(0.8)
)

var grainBmp [2]*ui.Bitmap

// grain returns the noise tile: light specks (for dark) or dark specks
// (for light) of random alpha.
func grain(dark bool, strength float32) *ui.Bitmap {
	i := 0
	if dark {
		i = 1
	}
	if grainBmp[i] != nil {
		return grainBmp[i]
	}
	img := image.NewNRGBA(image.Rect(0, 0, grainTile, grainTile))
	rng := rand.New(rand.NewPCG(7, 11))
	for y := 0; y < grainTile; y++ {
		for x := 0; x < grainTile; x++ {
			v := uint8(0)
			if dark {
				v = 255
			}
			a := uint8(float32(rng.IntN(256)) * strength)
			img.SetNRGBA(x, y, color.NRGBA{v, v, v, a})
		}
	}
	grainBmp[i] = ui.NewBitmap(img)
	return grainBmp[i]
}

// PaintGrain tiles film grain over r at the palette's strength.
func PaintGrain(p *ui.Painter, r ui.Rect, pal *Palette) {
	if pal.Grain <= 0 {
		return
	}
	bmp := grain(pal.Dark, pal.Grain)
	t := float32(grainTile) / 2 // two device pixels per grain on Retina
	p.Clip(r, 0, func() {
		for y := r.Y; y < r.Y+r.H; y += t {
			for x := r.X; x < r.X+r.W; x += t {
				p.ImageRegion(bmp, ui.Rect{W: grainTile, H: grainTile}, ui.Rect{X: x, Y: y, W: t, H: t}, ui.ImageOptions{})
			}
		}
	})
}

// PaintDots draws the workbench dot grid over r, fading toward its edges
// when vignette is set.
func PaintDots(p *ui.Painter, r ui.Rect, pal *Palette, vignette bool) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	maxd := r.W*r.W/4 + r.H*r.H/4
	for y := r.Y + DotSpacing/2; y < r.Y+r.H; y += DotSpacing {
		for x := r.X + DotSpacing/2; x < r.X+r.W; x += DotSpacing {
			col := pal.Dots
			if vignette {
				d := ((x-cx)*(x-cx) + (y-cy)*(y-cy)) / maxd
				k := 1 - d*1.2
				if k <= 0.02 {
					continue
				}
				col = Fade(col, k)
			}
			p.Fill(ui.Rect{X: x - DotRadius, Y: y - DotRadius, W: 2 * DotRadius, H: 2 * DotRadius}, col, DotRadius)
		}
	}
}
