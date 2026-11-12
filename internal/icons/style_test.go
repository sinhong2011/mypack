package icons

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"mypack.dev/mypack/pkg/config"
)

func decodeStyled(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestStyledShapes(t *testing.T) {
	base := Default("Notion")
	for _, st := range []config.IconStyle{
		{},
		{Background: config.BackgroundGlass, Colors: []string{"#7c83fd", "#4b4fd1"}, Tint: config.TintWhite},
		{Glyph: config.GlyphText, Text: "Gm", Background: config.BackgroundSolid, Colors: []string{"#111111"}},
		{Glyph: config.GlyphText, Text: "中", Background: config.BackgroundGradient},
		{Glyph: config.GlyphEmoji, Text: "🚀", Badge: "Work"},
		{Background: config.BackgroundNone, Badge: "Beta"},
	} {
		b, err := Styled(base, st, "Notion", 256)
		if err != nil {
			t.Fatal(err)
		}
		img := decodeStyled(t, b)
		if img.Bounds().Dx() != 256 || img.Bounds().Dy() != 256 {
			t.Fatalf("%+v: size %v", st, img.Bounds())
		}
		if st.Background == config.BackgroundNone {
			continue
		}
		// On a plate: transparent corners, opaque middle (same grid as Default).
		if _, _, _, a := img.At(2, 2).RGBA(); a != 0 {
			t.Errorf("%+v: corner not transparent", st)
		}
		if _, _, _, a := img.At(128, 40).RGBA(); a < 0xff00 {
			t.Errorf("%+v: plate not opaque", st)
		}
	}
}

func TestStyledColorsAndBadge(t *testing.T) {
	st := config.IconStyle{Glyph: config.GlyphText, Text: "A", Background: config.BackgroundSolid, Colors: []string{"#ff0000"}, Scale: 20}
	img := StyledImage(nil, st, "x", 256)
	if c := img.RGBAAt(40, 128); c.R < 250 || c.G > 5 || c.B > 5 {
		t.Fatalf("solid red background, got %v", c)
	}
	st.Badge, st.BadgeColor = "Work", "#0000ff"
	img = StyledImage(nil, st, "x", 256)
	// The ribbon sits low on the plate, centred.
	found := false
	for y := 170; y < 230; y++ {
		if c := img.RGBAAt(128-40, y); c.B > 200 && c.R < 40 {
			found = true
		}
	}
	if !found {
		t.Fatal("no blue badge near the bottom")
	}
}

func TestEmojiFallback(t *testing.T) {
	// With or without an emoji font, an emoji style always draws something.
	img := StyledImage(nil, config.IconStyle{Glyph: config.GlyphEmoji, Text: "🐙"}, "Octo", 128)
	if _, _, _, a := img.At(64, 64).RGBA(); a == 0 {
		t.Fatal("empty icon")
	}
	if EmojiAvailable() && !HasEmoji("🐙") {
		t.Fatal("the emoji font is loaded but has no octopus")
	}
	if HasEmoji("") {
		t.Fatal("empty text is not an emoji")
	}
}
