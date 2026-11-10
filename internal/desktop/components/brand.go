package components

import (
	"bytes"
	"hash/fnv"
	"image/color"
	"image/png"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/assets"
	"mypack.dev/mypack/internal/desktop/design"
)

// The mypack mark: an isometric cube whose three faces carry different
// weights, read as a packed, solid object. No animals, no mascots.
var cubeSVG = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path d="M12 2.6 20.4 7.3 12 12 3.6 7.3Z" fill="currentColor"/>
<path d="M3.6 8.7 11.3 13.1V21.6L3.6 17.2Z" fill="currentColor" fill-opacity="0.62"/>
<path d="M20.4 8.7 12.7 13.1V21.6L20.4 17.2Z" fill="currentColor" fill-opacity="0.36"/>
</svg>`))

// Template outline cube for the macOS menu bar (monochrome, tinted by the OS).
var cubeTemplateSVG = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#000" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
<path d="M11 21.73a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73z"/><path d="M12 22V12"/><path d="M3.29 7 12 12l8.71-5"/><path d="m7.5 4.27 9 5.15"/>
</svg>`))

// CubeSVG returns the brand cube glyph (currentColor faces).
func CubeSVG() *ui.SVG { return cubeSVG }

// BrandMark draws the official app icon (embedded, pre-masked squircle).
func BrandMark(c *ui.Context, size float32) ui.Element {
	src := assets.Brand64
	if size > 40 {
		src = assets.Brand256
	}
	bmp := bitmapFor(src)
	return ui.Box(c).Size(size, size).Shrink(0).Children(func() {
		if bmp != nil {
			ui.Image(c, bmp).Size(size, size).Fit(ui.Contain)
		}
	}).AccessHidden(true)
}

// TrayIconPNG renders the monochrome template cube for the status item at
// 2x (36 px for an 18 pt icon).
func TrayIconPNG(active bool) []byte {
	img := cubeTemplateSVG.Image(36, 36, color.Black)
	if active {
		// A small dot at the lower right signals a build in progress.
		for y := 26; y < 36; y++ {
			for x := 26; x < 36; x++ {
				dx, dy := float64(x)-31, float64(y)-31
				if dx*dx+dy*dy <= 20 {
					img.Set(x, y, color.Black)
				} else if dx*dx+dy*dy <= 32 {
					img.Set(x, y, color.Transparent)
				}
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// ---- App icons ----------------------------------------------------------------

var bitmapCache = map[*byte]*ui.Bitmap{}

func bitmapFor(png []byte) *ui.Bitmap {
	if len(png) == 0 {
		return nil
	}
	k := &png[0]
	if b, ok := bitmapCache[k]; ok {
		return b
	}
	b, err := ui.DecodeBitmap(png)
	if err != nil {
		b = nil
	}
	if len(bitmapCache) > 400 {
		bitmapCache = map[*byte]*ui.Bitmap{}
	}
	bitmapCache[k] = b
	return b
}

var placeholderGradients = [][2]ui.Color{
	{ui.RGB(99, 138, 255), ui.RGB(56, 92, 222)},
	{ui.RGB(124, 110, 245), ui.RGB(86, 70, 214)},
	{ui.RGB(54, 178, 160), ui.RGB(26, 134, 122)},
	{ui.RGB(245, 150, 82), ui.RGB(222, 104, 54)},
	{ui.RGB(236, 98, 128), ui.RGB(200, 60, 96)},
	{ui.RGB(120, 128, 142), ui.RGB(82, 90, 104)},
}

// AppIcon shows an app icon (PNG bytes), or a neutral generated tile with
// the name's initial when there is none.
func AppIcon(c *ui.Context, png []byte, name string, size float32) ui.Element {
	r := design.AppIconRadius(size)
	if bmp := bitmapFor(png); bmp != nil {
		return ui.Box(c).Size(size, size).Shrink(0).Children(func() {
			ui.Image(c, bmp).Size(size, size).Fit(ui.Cover).Radius(r)
		}).AccessHidden(true)
	}
	return Placeholder(c, name, size)
}

// Placeholder is the generated icon tile.
func Placeholder(c *ui.Context, name string, size float32) ui.Element {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(name))))
	g := placeholderGradients[int(h.Sum32()%uint32(len(placeholderGradients)))]
	letter := ""
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			letter = strings.ToUpper(string(r))
			break
		}
	}
	return ui.Row(c).Size(size, size).Radius(design.AppIconRadius(size)).Center().Shrink(0).
		LinearGradient(ui.LinearGradient{Angle: 160, Oklab: true, Stops: []ui.GradientStop{{Color: g[0], At: 0}, {Color: g[1], At: 1}}}).
		InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.22)).
		Children(func() {
			if letter == "" {
				ui.Icon(c, cubeSVG).FontSize(size * 0.5).TextColor(ui.RGB(255, 255, 255))
				return
			}
			ui.Text(c, letter).FontSize(size * 0.46).FontWeight(design.WeightSemibold).TextColor(ui.RGB(255, 255, 255))
		}).AccessHidden(true)
}
