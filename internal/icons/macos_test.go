package icons

import (
	"bytes"
	"image/png"
	"testing"
)

func TestMacAppIconFullBleed(t *testing.T) {
	for name, src := range map[string][]byte{"placeholder": Default("Notion")} {
		out, err := MacAppIcon(src)
		if err != nil {
			t.Fatal(name, err)
		}
		img, err := png.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != CanvasSize || b.Dy() != CanvasSize {
			t.Fatalf("%s: size %v", name, b)
		}
		for _, p := range [][2]int{{0, 0}, {CanvasSize - 1, 0}, {0, CanvasSize - 1}, {CanvasSize - 1, CanvasSize - 1}, {CanvasSize / 2, 0}} {
			if _, _, _, a := img.At(p[0], p[1]).RGBA(); a != 0xffff {
				t.Fatalf("%s: pixel %v not opaque (macOS would jail this icon)", name, p)
			}
		}
	}
}
