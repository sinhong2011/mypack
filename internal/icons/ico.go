package icons

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

func isICO(b []byte) bool {
	return len(b) >= 22 && b[0] == 0 && b[1] == 0 && (b[2] == 1 || b[2] == 2) && b[3] == 0 && b[4] > 0
}

type icoEntry struct {
	w, h   int
	bpp    int
	offset uint32
	size   uint32
}

// decodeICO decodes the best (largest, then deepest) image of an ICO file.
// PNG-embedded and BMP (1/4/8/24/32 bit) entries are supported.
func decodeICO(b []byte) (image.Image, error) {
	n := int(binary.LittleEndian.Uint16(b[4:6]))
	if len(b) < 6+16*n {
		return nil, errors.New("icons: truncated ICO directory")
	}
	var entries []icoEntry
	for i := 0; i < n; i++ {
		e := b[6+16*i : 22+16*i]
		w, h := int(e[0]), int(e[1])
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		entries = append(entries, icoEntry{w: w, h: h, bpp: int(binary.LittleEndian.Uint16(e[6:8])),
			size: binary.LittleEndian.Uint32(e[8:12]), offset: binary.LittleEndian.Uint32(e[12:16])})
	}
	// order: largest area first, then deeper
	for i := range entries {
		for j := i + 1; j < len(entries); j++ {
			a, c := entries[i], entries[j]
			if c.w*c.h > a.w*a.h || c.w*c.h == a.w*a.h && c.bpp > a.bpp {
				entries[i], entries[j] = c, a
			}
		}
	}
	var lastErr error
	for _, e := range entries {
		end := uint64(e.offset) + uint64(e.size)
		if end > uint64(len(b)) || e.size < 8 {
			lastErr = errors.New("icons: ICO entry out of range")
			continue
		}
		data := b[e.offset:end]
		var img image.Image
		var err error
		if bytes.HasPrefix(data, pngSig) {
			img, err = png.Decode(bytes.NewReader(data))
		} else {
			img, err = decodeDIB(data)
		}
		if err == nil {
			return img, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("icons: empty ICO")
	}
	return nil, lastErr
}

// decodeDIB decodes the BITMAPINFOHEADER image + AND mask stored in ICO entries.
func decodeDIB(d []byte) (image.Image, error) {
	if len(d) < 40 {
		return nil, errors.New("icons: DIB too short")
	}
	hdr := int(binary.LittleEndian.Uint32(d[0:4]))
	if hdr < 40 || hdr > len(d) {
		return nil, errors.New("icons: unsupported DIB header")
	}
	w := int(int32(binary.LittleEndian.Uint32(d[4:8])))
	hh := int(int32(binary.LittleEndian.Uint32(d[8:12])))
	planes := binary.LittleEndian.Uint16(d[12:14])
	bpp := int(binary.LittleEndian.Uint16(d[14:16]))
	comp := binary.LittleEndian.Uint32(d[16:20])
	colorsUsed := int(binary.LittleEndian.Uint32(d[32:36]))
	if planes != 1 || w <= 0 || w > 1024 || hh == 0 {
		return nil, errors.New("icons: invalid DIB")
	}
	if comp != 0 && !(comp == 3 && bpp == 32) { // BI_RGB or BI_BITFIELDS(32bpp, assume BGRA)
		return nil, fmt.Errorf("icons: unsupported DIB compression %d", comp)
	}
	topDown := hh < 0
	if topDown {
		hh = -hh
	}
	h := hh / 2 // XOR + AND mask stacked
	if h <= 0 || h > 1024 {
		return nil, errors.New("icons: invalid DIB height")
	}
	pos := hdr
	if comp == 3 && hdr == 40 {
		pos += 12 // bit masks follow header
	}
	var pal []color.NRGBA
	if bpp <= 8 {
		nc := colorsUsed
		if nc == 0 {
			nc = 1 << bpp
		}
		if pos+4*nc > len(d) {
			return nil, errors.New("icons: truncated palette")
		}
		for i := 0; i < nc; i++ {
			p := d[pos+4*i:]
			pal = append(pal, color.NRGBA{p[2], p[1], p[0], 255})
		}
		pos += 4 * nc
	}
	stride := ((w*bpp + 31) / 32) * 4
	xorSize := stride * h
	if pos+xorSize > len(d) {
		return nil, errors.New("icons: truncated DIB pixels")
	}
	xor := d[pos : pos+xorSize]
	maskStride := ((w + 31) / 32) * 4
	var mask []byte
	if pos+xorSize+maskStride*h <= len(d) {
		mask = d[pos+xorSize : pos+xorSize+maskStride*h]
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	anyAlpha := false
	for y := 0; y < h; y++ {
		sy := h - 1 - y
		if topDown {
			sy = y
		}
		row := xor[sy*stride : (sy+1)*stride]
		for x := 0; x < w; x++ {
			var c color.NRGBA
			switch bpp {
			case 32:
				c = color.NRGBA{row[4*x+2], row[4*x+1], row[4*x], row[4*x+3]}
				if c.A != 0 {
					anyAlpha = true
				}
			case 24:
				c = color.NRGBA{row[3*x+2], row[3*x+1], row[3*x], 255}
			case 8:
				if int(row[x]) < len(pal) {
					c = pal[row[x]]
				}
			case 4:
				v := row[x/2]
				if x%2 == 0 {
					v >>= 4
				} else {
					v &= 0xf
				}
				if int(v) < len(pal) {
					c = pal[v]
				}
			case 1:
				v := (row[x/8] >> (7 - uint(x%8))) & 1
				if int(v) < len(pal) {
					c = pal[v]
				}
			default:
				return nil, fmt.Errorf("icons: unsupported DIB depth %d", bpp)
			}
			img.SetNRGBA(x, y, c)
		}
	}
	// 32-bit images with real alpha ignore the AND mask; everything else uses it.
	if mask != nil && !(bpp == 32 && anyAlpha) {
		for y := 0; y < h; y++ {
			sy := h - 1 - y
			if topDown {
				sy = y
			}
			for x := 0; x < w; x++ {
				if mask[sy*maskStride+x/8]>>(7-uint(x%8))&1 == 1 {
					img.SetNRGBA(x, y, color.NRGBA{})
				} else if bpp == 32 {
					c := img.NRGBAAt(x, y)
					c.A = 255
					img.SetNRGBA(x, y, c)
				}
			}
		}
	}
	return img, nil
}

// ICO builds a Windows .ico (16,24,32,48,64,128,256; PNG-embedded) from a
// square PNG.
func ICO(src []byte) ([]byte, error) {
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	imgs := make([][]byte, len(sizes))
	for i, s := range sizes {
		p, err := Resize(src, s)
		if err != nil {
			return nil, err
		}
		imgs[i] = p
	}
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	off := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim := byte(s)
		if s >= 256 {
			dim = 0
		}
		buf.Write([]byte{dim, dim, 0, 0})
		_ = binary.Write(&buf, binary.LittleEndian, [2]uint16{1, 32})
		_ = binary.Write(&buf, binary.LittleEndian, uint32(len(imgs[i])))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(off))
		off += len(imgs[i])
	}
	for _, p := range imgs {
		buf.Write(p)
	}
	return buf.Bytes(), nil
}
