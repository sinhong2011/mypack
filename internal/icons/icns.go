package icons

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// icnsEntries lists the PNG-payload entries written by ICNS.
var icnsEntries = []struct {
	Type string
	Size int
}{
	{"ic11", 32}, {"ic12", 64}, {"ic07", 128}, {"ic13", 256}, {"ic08", 256},
	{"ic14", 512}, {"ic09", 512}, {"ic10", 1024},
}

// ICNS builds a macOS .icns from a square PNG (ic07..ic10 + retina variants,
// PNG payloads).
func ICNS(src []byte) ([]byte, error) {
	var body bytes.Buffer
	for _, e := range icnsEntries {
		p, err := Resize(src, e.Size)
		if err != nil {
			return nil, err
		}
		body.WriteString(e.Type)
		_ = binary.Write(&body, binary.BigEndian, uint32(8+len(p)))
		body.Write(p)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	_ = binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

// ICNSLargestPNG returns the largest PNG payload stored in an .icns file
// (ic10, ic14, ic09, ic13, ic08, ic07 …). Legacy non-PNG entries are skipped.
func ICNSLargestPNG(data []byte) ([]byte, error) {
	if len(data) < 8 || string(data[:4]) != "icns" {
		return nil, errors.New("icons: not an icns file")
	}
	total := int(binary.BigEndian.Uint32(data[4:8]))
	if total > len(data) || total < 8 {
		total = len(data)
	}
	var best []byte
	bestRank := -1
	rank := map[string]int{"ic10": 8, "ic14": 7, "ic09": 6, "ic13": 5, "ic08": 4, "ic12": 3, "ic07": 2, "ic11": 1}
	for pos := 8; pos+8 <= total; {
		typ := string(data[pos : pos+4])
		l := int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))
		if l < 8 || pos+l > total {
			break
		}
		payload := data[pos+8 : pos+l]
		if r, ok := rank[typ]; ok && r > bestRank && bytes.HasPrefix(payload, pngSig) {
			best, bestRank = payload, r
		}
		pos += l
	}
	if best == nil {
		return nil, errors.New("icons: no PNG payload in icns")
	}
	return best, nil
}
