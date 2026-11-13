package icons

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func mustPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	b, err := EncodePNG(img)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func decodePNG(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestNormalizeLargeSquare(t *testing.T) {
	out, err := Normalize(mustPNG(t, solid(256, 256, color.NRGBA{200, 30, 30, 255})))
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, out)
	if img.Bounds().Dx() != 1024 || img.Bounds().Dy() != 1024 {
		t.Fatalf("size %v", img.Bounds())
	}
	r, _, _, a := img.At(512, 512).RGBA()
	if a>>8 != 255 || r>>8 < 190 {
		t.Fatalf("center pixel wrong: %v", img.At(512, 512))
	}
}

func TestNormalizeNonSquareLetterbox(t *testing.T) {
	out, err := Normalize(mustPNG(t, solid(400, 200, color.NRGBA{0, 0, 255, 255})))
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, out)
	if _, _, _, a := img.At(512, 10).RGBA(); a != 0 {
		t.Fatal("top should be transparent")
	}
	if _, _, _, a := img.At(512, 512).RGBA(); a == 0 {
		t.Fatal("center should be opaque")
	}
}

func TestNormalizeTinyGetsPlate(t *testing.T) {
	out, err := Normalize(mustPNG(t, solid(16, 16, color.NRGBA{220, 40, 40, 255})))
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, out)
	if _, _, _, a := img.At(2, 2).RGBA(); a != 0 {
		t.Fatal("corner outside plate must be transparent")
	}
	// plate area (outside the glyph but inside the squircle) must be opaque
	if _, _, _, a := img.At(130, 512).RGBA(); a>>8 != 255 {
		t.Fatalf("plate not opaque: %v", img.At(130, 512))
	}
	// glyph in the middle
	r, g, _, _ := img.At(512, 512).RGBA()
	if r>>8 < 200 || g>>8 > 80 {
		t.Fatalf("glyph color wrong: %v", img.At(512, 512))
	}
}

func TestDecodeFormats(t *testing.T) {
	src := solid(40, 40, color.NRGBA{10, 200, 10, 255})
	var j, g bytes.Buffer
	_ = jpeg.Encode(&j, src, nil)
	_ = gif.Encode(&g, src, nil)
	for name, data := range map[string][]byte{"png": mustPNG(t, src), "jpeg": j.Bytes(), "gif": g.Bytes()} {
		out, err := Normalize(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if decodePNG(t, out).Bounds().Dx() != 1024 {
			t.Fatalf("%s: wrong size", name)
		}
	}
	if _, err := Normalize([]byte("not an image at all")); err == nil {
		t.Fatal("expected error")
	}
}

// buildICO makes an ICO with a PNG-embedded 32px entry and a BMP 16px entry.
func buildICO(pngData []byte, bmp16 []byte) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, [3]uint16{0, 1, 2})
	off := 6 + 32
	// entry 1: 16x16 BMP
	b.Write([]byte{16, 16, 0, 0})
	binary.Write(&b, binary.LittleEndian, [2]uint16{1, 32})
	binary.Write(&b, binary.LittleEndian, uint32(len(bmp16)))
	binary.Write(&b, binary.LittleEndian, uint32(off))
	off += len(bmp16)
	// entry 2: 32x32 PNG
	b.Write([]byte{32, 32, 0, 0})
	binary.Write(&b, binary.LittleEndian, [2]uint16{1, 32})
	binary.Write(&b, binary.LittleEndian, uint32(len(pngData)))
	binary.Write(&b, binary.LittleEndian, uint32(off))
	b.Write(bmp16)
	b.Write(pngData)
	return b.Bytes()
}

func bmpDIB32(w, h int, c color.NRGBA) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, uint32(40))
	binary.Write(&b, binary.LittleEndian, int32(w))
	binary.Write(&b, binary.LittleEndian, int32(h*2))
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, uint16(32))
	b.Write(make([]byte, 24))
	for i := 0; i < w*h; i++ {
		b.Write([]byte{c.B, c.G, c.R, c.A})
	}
	b.Write(make([]byte, ((w+31)/32)*4*h)) // AND mask
	return b.Bytes()
}

func TestICODecodeBMPAndPNG(t *testing.T) {
	bmp := bmpDIB32(16, 16, color.NRGBA{255, 0, 0, 255})
	// Only BMP: decoded pixels red
	one := func() []byte {
		var b bytes.Buffer
		binary.Write(&b, binary.LittleEndian, [3]uint16{0, 1, 1})
		b.Write([]byte{16, 16, 0, 0})
		binary.Write(&b, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&b, binary.LittleEndian, uint32(len(bmp)))
		binary.Write(&b, binary.LittleEndian, uint32(22))
		b.Write(bmp)
		return b.Bytes()
	}()
	img, err := Decode(one)
	if err != nil {
		t.Fatal(err)
	}
	if r, g, _, a := img.At(3, 3).RGBA(); r>>8 != 255 || g != 0 || a>>8 != 255 {
		t.Fatalf("bmp pixel %v", img.At(3, 3))
	}
	// Mixed: largest (PNG 32px, green) wins
	ico := buildICO(mustPNG(t, solid(32, 32, color.NRGBA{0, 255, 0, 255})), bmp)
	img, err = Decode(ico)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 32 {
		t.Fatalf("expected 32px, got %v", img.Bounds())
	}
	if _, err := Normalize(ico); err != nil {
		t.Fatal(err)
	}
}

func TestICOWriter(t *testing.T) {
	src := Default("MyPack")
	ico, err := ICO(src)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint16(ico[0:2]) != 0 || binary.LittleEndian.Uint16(ico[2:4]) != 1 {
		t.Fatal("bad ICONDIR")
	}
	n := int(binary.LittleEndian.Uint16(ico[4:6]))
	if n != 7 {
		t.Fatalf("entries=%d", n)
	}
	want := []int{16, 24, 32, 48, 64, 128, 256}
	for i := 0; i < n; i++ {
		e := ico[6+16*i:]
		w := int(e[0])
		if w == 0 {
			w = 256
		}
		if w != want[i] {
			t.Fatalf("entry %d width %d", i, w)
		}
		size := binary.LittleEndian.Uint32(e[8:12])
		off := binary.LittleEndian.Uint32(e[12:16])
		img, err := png.Decode(bytes.NewReader(ico[off : off+size]))
		if err != nil || img.Bounds().Dx() != w {
			t.Fatalf("entry %d decode: %v", i, err)
		}
	}
	// round trip via our own decoder
	img, err := Decode(ico)
	if err != nil || img.Bounds().Dx() != 256 {
		t.Fatalf("roundtrip: %v %v", err, img)
	}
}

func TestICNS(t *testing.T) {
	src := Default("Notion")
	b, err := ICNS(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(b[:4]) != "icns" || int(binary.BigEndian.Uint32(b[4:8])) != len(b) {
		t.Fatal("bad header/length")
	}
	got := map[string]int{}
	for pos := 8; pos < len(b); {
		typ := string(b[pos : pos+4])
		l := int(binary.BigEndian.Uint32(b[pos+4 : pos+8]))
		img, err := png.Decode(bytes.NewReader(b[pos+8 : pos+l]))
		if err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		got[typ] = img.Bounds().Dx()
		pos += l
	}
	want := map[string]int{"ic07": 128, "ic08": 256, "ic09": 512, "ic10": 1024, "ic11": 32, "ic12": 64, "ic13": 256, "ic14": 512}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %d want %d", k, got[k], v)
		}
	}
	p, err := ICNSLargestPNG(b)
	if err != nil || decodePNG(t, p).Bounds().Dx() != 1024 {
		t.Fatalf("largest: %v", err)
	}
}

func TestDefaultIcon(t *testing.T) {
	a, b := Default("GitHub"), Default("GitHub")
	if !bytes.Equal(a, b) {
		t.Fatal("not deterministic")
	}
	img := decodePNG(t, a)
	if img.Bounds().Dx() != 1024 {
		t.Fatal("size")
	}
	if _, _, _, al := img.At(5, 5).RGBA(); al != 0 {
		t.Fatal("margin must be transparent")
	}
	if _, _, _, al := img.At(300, 800).RGBA(); al>>8 != 255 {
		t.Fatal("plate must be opaque")
	}
	// the letter is white somewhere near the center
	white := 0
	for y := 400; y < 624; y++ {
		for x := 400; x < 624; x++ {
			r, g, bb, _ := img.At(x, y).RGBA()
			if r>>8 > 245 && g>>8 > 245 && bb>>8 > 245 {
				white++
			}
		}
	}
	if white < 500 {
		t.Fatalf("expected white letter pixels, got %d", white)
	}
	// different names pick different slots over a spread of names
	seen := map[int]bool{}
	for _, n := range []string{"a", "b", "c", "Notion", "Slack", "Figma", "Linear", "Gmail", "Zoom", "Trello", "Canva", "Miro"} {
		seen[PaletteIndex(n)] = true
	}
	if len(seen) < 5 {
		t.Fatalf("palette poorly distributed: %d", len(seen))
	}
	// CJK / empty names still produce a valid icon
	for _, n := range []string{"知乎", "", "   ", "🙂"} {
		if p := Default(n); len(p) == 0 {
			t.Fatalf("empty icon for %q", n)
		}
	}
}

func TestResizeRoundTrip(t *testing.T) {
	out, err := Resize(Default("x"), 64)
	if err != nil {
		t.Fatal(err)
	}
	if decodePNG(t, out).Bounds().Dx() != 64 {
		t.Fatal("size")
	}
	if _, err := Resize(Default("x"), 0); err == nil {
		t.Fatal("want error")
	}
}
