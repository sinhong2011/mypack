package icons

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	xdraw "golang.org/x/image/draw"
)

// Group draws a group app's icon: the first four members' icons in a 2x2
// grid on a light plate (512x512 PNG). Members without a usable icon are
// skipped; nil when none has one.
func Group(members [][]byte) []byte {
	var imgs []image.Image
	for _, b := range members {
		if len(imgs) == 4 {
			break
		}
		if len(b) == 0 {
			continue
		}
		if img, _, err := image.Decode(bytes.NewReader(b)); err == nil {
			imgs = append(imgs, img)
		}
	}
	if len(imgs) == 0 {
		return nil
	}
	const size, pad, gap = 512, 56, 24
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	plate := color.RGBA{R: 242, G: 242, B: 246, A: 255}
	xdraw.Draw(dst, dst.Bounds(), &image.Uniform{C: plate}, image.Point{}, xdraw.Src)
	cell := (size - 2*pad - gap) / 2
	slots := [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
	if len(imgs) == 1 { // one icon: centered, large
		cell = size - 2*pad
		slots = [][2]int{{0, 0}}
	}
	for i, img := range imgs {
		if i >= len(slots) {
			break
		}
		x := pad + slots[i][0]*(cell+gap)
		y := pad + slots[i][1]*(cell+gap)
		xdraw.CatmullRom.Scale(dst, image.Rect(x, y, x+cell, y+cell), img, img.Bounds(), xdraw.Over, nil)
	}
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil
	}
	return out.Bytes()
}
