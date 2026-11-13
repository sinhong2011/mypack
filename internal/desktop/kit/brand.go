package kit

import (
	"bytes"
	"hash/fnv"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"math"
	"strings"
	"sync"
	"unicode"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/assets"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	appicons "mypack.dev/mypack/internal/icons"
)

// Template outline cube for the macOS menu bar (monochrome, tinted by the OS).
var cubeTemplateSVG = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#000" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
<path d="M11 21.73a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73z"/><path d="M12 22V12"/><path d="M3.29 7 12 12l8.71-5"/><path d="m7.5 4.27 9 5.15"/>
</svg>`))

// BrandMark draws the official app icon (embedded, pre-masked squircle).
func BrandMark(c *ui.Context, size float32) ui.Element {
	src := assets.Brand64
	if size > 32 {
		src = assets.Brand256
	}
	return AppIconOpts(c, src, "MyPack", size, IconOpts{Shadow: size >= 40})
}

// TrayIconPNG renders the monochrome template cube for the status item at
// 2x (36 px for an 18 pt icon).
func TrayIconPNG(active bool) []byte { return TrayProgressPNG(-2, 0) }

// TrayProgressPNG is the menu-bar template glyph; while a build runs
// (progress ≥ 0, or -1 for unknown) it shrinks inside a thin progress ring:
// a sweep to progress, or a 90° arc turning with phase (0…1) when the
// progress is unknown. progress < -1 is the plain glyph.
func TrayProgressPNG(progress, phase float64) []byte {
	const n = 36
	if progress < -1 {
		img := cubeTemplateSVG.Image(n, n, color.Black)
		var b bytes.Buffer
		_ = png.Encode(&b, img)
		return b.Bytes()
	}
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	glyph := cubeTemplateSVG.Image(22, 22, color.Black)
	draw.Draw(img, image.Rect(7, 7, 29, 29), glyph, image.Point{}, draw.Over)
	from, span := -math.Pi/2, 0.0
	if progress < 0 {
		from, span = -math.Pi/2+phase*2*math.Pi, math.Pi/2
	} else {
		span = math.Max(0.04, math.Min(1, progress)) * 2 * math.Pi
	}
	const cx, cy, r, th = 17.5, 17.5, 16.0, 2.6
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			// 4×4 supersampling for a smooth ring.
			cov := 0.0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					px, py := float64(x)+(float64(sx)+0.5)/4, float64(y)+(float64(sy)+0.5)/4
					d := math.Hypot(px-cx, py-cy)
					if math.Abs(d-r) > th/2 {
						continue
					}
					ang := math.Mod(math.Atan2(py-cy, px-cx)-from+4*math.Pi, 2*math.Pi)
					a := 0.3 // the track
					if ang <= span {
						a = 1
					}
					cov += a / 16
				}
			}
			if cov > 0 {
				_, _, _, ga := img.At(x, y).RGBA()
				al := math.Max(float64(ga)/65535, cov)
				img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, uint8(math.Round(al * 255))})
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// ---- App icons ----------------------------------------------------------------

// iconArt is a decoded icon with the box its opaque body takes, as
// fractions of the image: macOS icons carry a 10% transparent margin
// around the squircle, which the GUI crops so the body fills the tile and
// the drop shadow hugs it.
type iconArt struct {
	bmp            *ui.Bitmap
	x0, y0, x1, y1 float32
	tint           ui.Color // the icon's average opaque color
	normalized     bool     // the art is the normalized full-bleed icon
}

// Icons arrive in every shape: circles, bare logos on transparency, tiny
// favicons, squares on their own plates. Every icon is normalized to the
// same full-bleed art (icons.MacAppIcon) and then masked to one squircle,
// so all tiles share a silhouette. Normalizing runs off the main thread;
// until it lands the raw art shows, cropped to its body.

var (
	artCache  = map[*byte]*iconArt{}  // by slice identity (fast path)
	artByHash = map[uint64]*iconArt{} // by content
	normMu    sync.Mutex
	normDone  = map[uint64][]byte{} // normalized PNGs waiting to be decoded
	normBusy  = map[uint64]bool{}
)

// Invalidate redraws the window once a normalized icon is ready (set by
// the app).
var Invalidate = func() {}

func contentHash(data []byte) uint64 {
	h := fnv.New64a()
	h.Write(data)
	return h.Sum64()
}

func artFor(data []byte) *iconArt {
	if len(data) == 0 {
		return nil
	}
	k := &data[0]
	if a, ok := artCache[k]; ok && (a == nil || a.normalized) {
		return a
	}
	if len(artCache) > 400 {
		artCache = map[*byte]*iconArt{}
	}
	hash := contentHash(data)
	if a, ok := artByHash[hash]; ok && a.normalized {
		artCache[k] = a
		return a
	}
	normMu.Lock()
	norm, ready := normDone[hash]
	if ready {
		delete(normDone, hash)
	} else if !normBusy[hash] {
		normBusy[hash] = true
		src := append([]byte(nil), data...)
		go func() {
			out, err := appicons.MacAppIcon(src)
			normMu.Lock()
			if err != nil {
				out = nil
			}
			normDone[hash] = out
			normMu.Unlock()
			Invalidate()
		}()
	}
	normMu.Unlock()
	if ready {
		if len(norm) > 0 {
			if a := decodeArt(norm); a != nil {
				a.normalized = true
				if raw := decodeArt(data); raw != nil {
					a.tint = raw.tint
				}
				artByHash[hash] = a
				artCache[k] = a
				return a
			}
		}
		// It couldn't be normalized: keep the raw art for good.
		a := decodeArt(data)
		if a != nil {
			a.normalized = true
		}
		artByHash[hash] = a
		artCache[k] = a
		return a
	}
	if a, ok := artByHash[hash]; ok {
		return a
	}
	a := decodeArt(data)
	artByHash[hash] = a
	artCache[k] = a
	return a
}

// decodeArt decodes an icon and measures its opaque body and tint.
func decodeArt(data []byte) *iconArt {
	bmp, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	a := &iconArt{bmp: bmp, x1: 1, y1: 1}
	if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
		b := img.Bounds()
		w, h := b.Dx(), b.Dy()
		minX, minY, maxX, maxY := w, h, -1, -1
		step := max(1, w/256)
		var sr, sg, sb, n uint64
		for y := 0; y < h; y += step {
			for x := 0; x < w; x += step {
				if r, g, bl, al := img.At(b.Min.X+x, b.Min.Y+y).RGBA(); al > 0x2000 {
					minX, minY = min(minX, x), min(minY, y)
					maxX, maxY = max(maxX, x), max(maxY, y)
					if al > 0xf000 {
						sr, sg, sb, n = sr+uint64(r>>8), sg+uint64(g>>8), sb+uint64(bl>>8), n+1
					}
				}
			}
		}
		if n > 0 {
			a.tint = ui.RGB(uint8(sr/n), uint8(sg/n), uint8(sb/n))
		}
		if maxX > minX && maxY > minY {
			a.x0, a.y0 = float32(minX)/float32(w), float32(minY)/float32(h)
			a.x1, a.y1 = float32(maxX+step)/float32(w), float32(maxY+step)/float32(h)
			// Square the body (keep its center).
			bw, bh := a.x1-a.x0, a.y1-a.y0
			if bw > bh {
				cy := (a.y0 + a.y1) / 2
				a.y0, a.y1 = cy-bw/2, cy+bw/2
			} else {
				cx := (a.x0 + a.x1) / 2
				a.x0, a.x1 = cx-bh/2, cx+bh/2
			}
		}
	}
	return a
}

// IconOpts configures AppIconOpts.
type IconOpts struct {
	Shadow bool // a soft, realistic drop shadow under the icon
}

// AppIcon shows an app icon (PNG bytes), or a generated macOS-style tile
// with the name's initial when there is none.
func AppIcon(c *ui.Context, png []byte, name string, size float32) ui.Element {
	return AppIconOpts(c, png, name, size, IconOpts{Shadow: size >= 48})
}

// IconShadow gives an app icon its very soft shadow.
func IconShadow(c *ui.Context, e ui.Element, size float32) ui.Element {
	p := design.Of(c)
	// One very soft shadow: the icon just lifts off the canvas.
	return e.Shadow(0, size*0.04, size*0.12, -size*0.02, design.Fade(p.IconShadow, 0.4))
}

// AppIconOpts is AppIcon with options.
func AppIconOpts(c *ui.Context, png []byte, name string, size float32, o IconOpts) ui.Element {
	r := design.AppIconRadius(size)
	art := artFor(png)
	if art == nil {
		e := Placeholder(c, name, size)
		if o.Shadow {
			IconShadow(c, e, size)
		}
		return e
	}
	box := ui.Box(c).Size(size, size).Shrink(0).Radius(r).Clip()
	if o.Shadow {
		IconShadow(c, box, size)
	}
	bw, bh := art.x1-art.x0, art.y1-art.y0
	iw, ih := size/bw, size/bh
	box.Children(func() {
		ui.Image(c, art.bmp).Absolute().Left(-art.x0*iw).Top(-art.y0*ih).Size(iw, ih).Fit(ui.Cover)
		// A hairline rim.
		ui.Box(c).Absolute().Fill().Radius(r).Border(0.5, ui.RGBA(0, 0, 0, 0.08)).PassThrough()
	})
	return box.AccessHidden(true)
}

// GroupMember is one site's icon in a group's mosaic.
type GroupMember struct {
	PNG  []byte
	Name string
}

// GroupIcon is a group of websites: a frosted tile holding up to four of
// its sites' icons in a 2×2 grid (a folder on the home screen), so a group
// never passes for one of its own apps.
func GroupIcon(c *ui.Context, members []GroupMember, size float32, o IconOpts) ui.Element {
	p := design.Of(c)
	r := design.AppIconRadius(size)
	if len(members) > 4 {
		members = members[:4]
	}
	pad := size * 0.14
	gap := size * 0.07
	cell := (size - 2*pad - gap) / 2
	box := ui.Box(c).Size(size, size).Shrink(0).Radius(r).Background(p.Fill.Over(p.Raised)).Border(0.5, p.BorderStrong)
	if o.Shadow {
		IconShadow(c, box, size)
	}
	// One row (two sites) sits in the middle, not along the top.
	top := pad
	if len(members) <= 2 {
		top = (size - cell) / 2
	}
	box.Children(func() {
		for i, m := range members {
			x := pad + float32(i%2)*(cell+gap)
			y := top + float32(i/2)*(cell+gap)
			ui.Box(c.Key(i)).Absolute().Left(x).Top(y).Size(cell, cell).Children(func() {
				AppIconOpts(c, m.PNG, m.Name, cell, IconOpts{})
			})
		}
	})
	return box.AccessHidden(true)
}

var placeholderGradients = [][2]ui.Color{
	{ui.RGB(102, 157, 255), ui.RGB(40, 92, 232)},
	{ui.RGB(156, 128, 255), ui.RGB(98, 60, 222)},
	{ui.RGB(72, 214, 186), ui.RGB(18, 146, 132)},
	{ui.RGB(255, 178, 92), ui.RGB(240, 106, 42)},
	{ui.RGB(255, 120, 150), ui.RGB(218, 52, 98)},
	{ui.RGB(150, 160, 178), ui.RGB(86, 96, 116)},
	{ui.RGB(98, 206, 255), ui.RGB(20, 132, 214)},
	{ui.RGB(160, 220, 110), ui.RGB(62, 158, 54)},
}

// PlaceholderColors returns the gradient a name's generated icon uses.
func PlaceholderColors(name string) (top, bottom ui.Color) {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(name))))
	g := placeholderGradients[int(h.Sum32()%uint32(len(placeholderGradients)))]
	return g[0], g[1]
}

// Placeholder is the generated icon tile: a gradient squircle with an inner
// highlight and a soft top gloss, as a real macOS icon.
func Placeholder(c *ui.Context, name string, size float32) ui.Element {
	top, bottom := PlaceholderColors(name)
	letter := ""
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			letter = strings.ToUpper(string(r))
			break
		}
	}
	r := design.AppIconRadius(size)
	return ui.Box(c).Size(size, size).Radius(r).Shrink(0).Clip().
		Background(top.MixOklab(bottom, 0.5)).
		Children(func() {
			ui.Row(c).Absolute().Fill().Center().Children(func() {
				if letter == "" {
					Glyph(c, icons.Globe, size*0.5, ui.RGB(255, 255, 255))
					return
				}
				ui.Text(c, letter).FontSize(size * 0.5).FontWeight(design.WeightBold).TextColor(ui.RGB(255, 255, 255)).
					LetterSpacing(-size * 0.01)
			})
		}).AccessHidden(true)
}

// BadgeKind is the small status badge on an icon's corner.
type BadgeKind int

const (
	BadgeNone BadgeKind = iota
	BadgeUpdate
	BadgeWarning
	BadgeFailed
	BadgeLocked
	BadgeDone
)

// IconBadge draws a status badge, sized for an icon of iconSize, at the
// icon's top-right corner (call inside the icon's parent box).
func IconBadge(c *ui.Context, kind BadgeKind, iconSize float32) {
	if kind == BadgeNone {
		return
	}
	p := design.Of(c)
	s := max(18, iconSize*0.3)
	var bg ui.Color
	var ic *ui.SVG
	switch kind {
	case BadgeUpdate:
		bg, ic = p.Accent, icons.ArrowUp
	case BadgeWarning:
		bg, ic = p.Warning, icons.Alert
	case BadgeFailed:
		bg, ic = p.Danger, icons.Alert
	case BadgeLocked:
		bg, ic = p.TextTertiary, icons.Lock
	case BadgeDone:
		bg, ic = p.Success, icons.Check
	}
	off := -s * 0.3
	ui.Row(c).Absolute().Top(off).Right(off).Size(s, s).Radius(s/2).Center().
		Background(bg).Border(2, p.Window).Children(func() {
		Glyph(c, ic, s*0.48, ui.RGB(255, 255, 255))
	}).AccessHidden(true)
}

// Glyph draws a Lucide icon size DIPs square in col. Decorative: hidden
// from assistive technology (label the control that holds it).
func Glyph(c *ui.Context, svg *ui.SVG, size float32, col ui.Color) ui.Element {
	return ui.Icon(c, svg).FontSize(size).TextColor(col).Shrink(0).AccessHidden(true)
}

// IconTint returns an icon's average color (its tint for scenes), or
// the generated icon's color for name when there is no icon.
func IconTint(png []byte, name string) ui.Color {
	if a := artFor(png); a != nil && a.tint.A > 0 {
		return a.tint
	}
	top, _ := PlaceholderColors(name)
	return top
}
