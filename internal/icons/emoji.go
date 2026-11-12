package icons

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"io"
	"os"
	"runtime"
	"sync"
)

// Emoji glyphs come from the system's colour emoji font. Only Apple Color
// Emoji (bitmaps in an "sbix" table) is read, so emoji glyphs are drawn on
// macOS; elsewhere EmojiAvailable is false and callers fall back to letters.
// Only single code points are looked up (no ZWJ sequences or flags, which
// need the font's ligature tables).

var emojiFontPaths = []string{
	"/System/Library/Fonts/Apple Color Emoji.ttc",
	"/System/Library/Fonts/Apple Color Emoji.ttf",
}

type emojiFont struct {
	f        *os.File
	cmap     []cmapGroup
	strike   int64 // offset of the largest strike
	nGlyphs  int
	ppem     int
	sbixBase int64
}

type cmapGroup struct{ start, end, glyph uint32 }

var (
	emojiOnce sync.Once
	emojiF    *emojiFont
	emojiMu   sync.Mutex
	emojiPNGs = map[rune]image.Image{}
)

// EmojiAvailable reports whether emoji glyphs can be drawn on this computer.
func EmojiAvailable() bool { return loadEmoji() != nil }

func loadEmoji() *emojiFont {
	emojiOnce.Do(func() {
		if runtime.GOOS != "darwin" {
			return
		}
		for _, p := range emojiFontPaths {
			if ef, err := openEmojiFont(p); err == nil {
				emojiF = ef
				return
			}
		}
	})
	return emojiF
}

func openEmojiFont(path string) (*emojiFont, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	ef, err := parseEmojiFont(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	ef.f = f
	return ef, nil
}

func parseEmojiFont(r io.ReaderAt) (*emojiFont, error) {
	rd := func(off int64, n int) ([]byte, error) {
		b := make([]byte, n)
		_, err := r.ReadAt(b, off)
		return b, err
	}
	be := binary.BigEndian
	head, err := rd(0, 16)
	if err != nil {
		return nil, err
	}
	base := int64(0)
	if string(head[:4]) == "ttcf" {
		base = int64(be.Uint32(head[12:16]))
	}
	dir, err := rd(base, 12)
	if err != nil {
		return nil, err
	}
	n := int(be.Uint16(dir[4:6]))
	recs, err := rd(base+12, 16*n)
	if err != nil {
		return nil, err
	}
	tables := map[string]int64{}
	for i := 0; i < n; i++ {
		rec := recs[16*i:]
		tables[string(rec[:4])] = int64(be.Uint32(rec[8:12]))
	}
	cmapOff, okC := tables["cmap"]
	sbixOff, okS := tables["sbix"]
	maxpOff, okM := tables["maxp"]
	if !okC || !okS || !okM {
		return nil, errors.New("icons: not a bitmap emoji font")
	}
	ef := &emojiFont{sbixBase: sbixOff}
	mp, err := rd(maxpOff, 6)
	if err != nil {
		return nil, err
	}
	ef.nGlyphs = int(be.Uint16(mp[4:6]))

	// cmap: the format 12 subtable (full Unicode).
	ch, err := rd(cmapOff, 4)
	if err != nil {
		return nil, err
	}
	nSub := int(be.Uint16(ch[2:4]))
	encs, err := rd(cmapOff+4, 8*nSub)
	if err != nil {
		return nil, err
	}
	for i := 0; i < nSub && ef.cmap == nil; i++ {
		sub := cmapOff + int64(be.Uint32(encs[8*i+4:]))
		h, err := rd(sub, 16)
		if err != nil || be.Uint16(h[:2]) != 12 {
			continue
		}
		ng := int(be.Uint32(h[12:16]))
		if ng <= 0 || ng > 1<<16 {
			continue
		}
		gs, err := rd(sub+16, 12*ng)
		if err != nil {
			continue
		}
		for g := 0; g < ng; g++ {
			b := gs[12*g:]
			ef.cmap = append(ef.cmap, cmapGroup{be.Uint32(b), be.Uint32(b[4:]), be.Uint32(b[8:])})
		}
	}
	if ef.cmap == nil {
		return nil, errors.New("icons: emoji font has no Unicode map")
	}

	// sbix: the strike with the most pixels per em.
	sh, err := rd(sbixOff, 8)
	if err != nil {
		return nil, err
	}
	ns := int(be.Uint32(sh[4:8]))
	if ns <= 0 || ns > 64 {
		return nil, errors.New("icons: emoji font has no strikes")
	}
	so, err := rd(sbixOff+8, 4*ns)
	if err != nil {
		return nil, err
	}
	for i := 0; i < ns; i++ {
		off := sbixOff + int64(be.Uint32(so[4*i:]))
		ph, err := rd(off, 2)
		if err != nil {
			continue
		}
		if ppem := int(be.Uint16(ph)); ppem > ef.ppem {
			ef.ppem, ef.strike = ppem, off
		}
	}
	if ef.ppem == 0 {
		return nil, errors.New("icons: emoji font has no strikes")
	}
	return ef, nil
}

func (ef *emojiFont) glyphIndex(r rune) (uint32, bool) {
	u := uint32(r)
	lo, hi := 0, len(ef.cmap)
	for lo < hi {
		m := (lo + hi) / 2
		g := ef.cmap[m]
		switch {
		case u < g.start:
			hi = m
		case u > g.end:
			lo = m + 1
		default:
			return g.glyph + u - g.start, true
		}
	}
	return 0, false
}

// glyphPNG returns the bitmap of glyph gid in the largest strike.
func (ef *emojiFont) glyphPNG(gid uint32, depth int) ([]byte, error) {
	if int(gid) >= ef.nGlyphs || depth > 2 {
		return nil, errors.New("icons: no such emoji")
	}
	be := binary.BigEndian
	offs := make([]byte, 8)
	if _, err := ef.f.ReadAt(offs, ef.strike+4+4*int64(gid)); err != nil {
		return nil, err
	}
	a, b := be.Uint32(offs), be.Uint32(offs[4:])
	if b <= a+8 || b-a > 4<<20 {
		return nil, errors.New("icons: no emoji bitmap")
	}
	data := make([]byte, b-a)
	if _, err := ef.f.ReadAt(data, ef.strike+int64(a)); err != nil {
		return nil, err
	}
	switch string(data[4:8]) {
	case "png ":
		return data[8:], nil
	case "dupe":
		return ef.glyphPNG(uint32(be.Uint16(data[8:10])), depth+1)
	}
	return nil, errors.New("icons: unsupported emoji bitmap")
}

// firstEmoji is the first code point of s that is not a variation selector
// or joiner.
func firstEmoji(s string) rune {
	for _, r := range s {
		if r == 0xFE0F || r == 0xFE0E || r == 0x200D || r == ' ' {
			continue
		}
		return r
	}
	return 0
}

// emojiImage decodes the bitmap of s's emoji (nil when there is none).
func emojiImage(s string) image.Image {
	r := firstEmoji(s)
	ef := loadEmoji()
	if ef == nil || r == 0 {
		return nil
	}
	emojiMu.Lock()
	defer emojiMu.Unlock()
	if img, ok := emojiPNGs[r]; ok {
		return img
	}
	var img image.Image
	if gid, ok := ef.glyphIndex(r); ok && gid != 0 {
		if b, err := ef.glyphPNG(gid, 0); err == nil {
			img, _, _ = image.Decode(bytes.NewReader(b))
		}
	}
	emojiPNGs[r] = img
	return img
}

// HasEmoji reports whether s's emoji can be drawn.
func HasEmoji(s string) bool { return emojiImage(s) != nil }
