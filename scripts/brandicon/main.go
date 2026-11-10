// Command brandicon cleans the brand artwork ("app icon.png") into a
// macOS-grid 1024px icon: finds the squircle body, masks everything outside
// a continuous rounded rect (removes background-removal specks) and centres
// it at 824px with Apple's 100px margin. Usage: brandicon <in.png> <out.png>
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"golang.org/x/image/draw"
)

func main() {
	f, err := os.Open(os.Args[1])
	check(err)
	src, err := png.Decode(f)
	check(err)
	b := src.Bounds()
	// Body bounds: scan the middle row/column for long opaque runs.
	opaque := func(x, y int) bool { _, _, _, a := src.At(x, y).RGBA(); return a > 0xF000 }
	cy, cx := (b.Min.Y+b.Max.Y)/2, (b.Min.X+b.Max.X)/2
	left, right, top, bottom := b.Max.X, b.Min.X, b.Max.Y, b.Min.Y
	for x := b.Min.X; x < b.Max.X; x++ {
		if run(src, x, cy, true) {
			left = min(left, x)
			right = max(right, x+23)
		}
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		if run(src, cx, y, false) {
			top = min(top, y)
			bottom = max(bottom, y+23)
		}
	}
	_ = opaque
	body := image.Rect(left, top, right+1, bottom+1)
	fmt.Fprintln(os.Stderr, "body:", body)
	const canvas, size = 1024, 824
	out := image.NewNRGBA(image.Rect(0, 0, canvas, canvas))
	dst := image.Rect((canvas-size)/2, (canvas-size)/2, (canvas+size)/2, (canvas+size)/2)
	draw.CatmullRom.Scale(out, dst, src, body, draw.Src, nil)
	// Squircle mask (superellipse n=5 approximates Apple's continuous corners).
	r := float64(size) / 2
	c := float64(canvas) / 2
	for y := 0; y < canvas; y++ {
		for x := 0; x < canvas; x++ {
			// supersample 4x4 for anti-aliasing
			in := 0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					dx := math.Abs((float64(x)+(float64(sx)+0.5)/4-c)/r)
					dy := math.Abs((float64(y)+(float64(sy)+0.5)/4-c)/r)
					if math.Pow(dx, 5)+math.Pow(dy, 5) <= 1 {
						in++
					}
				}
			}
			p := out.NRGBAAt(x, y)
			p.A = uint8(int(p.A) * in / 16)
			if p.A == 0 {
				p = color.NRGBA{}
			}
			out.SetNRGBA(x, y, p)
		}
	}
	w, err := os.Create(os.Args[2])
	check(err)
	check(png.Encode(w, out))
	check(w.Close())
}

// run reports whether a 24px opaque run starts at (x,y) along the axis.
func run(img image.Image, x, y int, horizontal bool) bool {
	for i := 0; i < 24; i++ {
		px, py := x, y
		if horizontal {
			px += i
		} else {
			py += i
		}
		if !(image.Pt(px, py).In(img.Bounds())) {
			return false
		}
		if _, _, _, a := img.At(px, py).RGBA(); a < 0xF000 {
			return false
		}
	}
	return true
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
