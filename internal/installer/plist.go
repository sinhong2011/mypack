package installer

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// ParsePlist parses an XML or binary (bplist00) property list whose root is a
// dictionary. Values are string, bool, int64, float64, time.Time, []byte,
// []any and map[string]any.
func ParsePlist(data []byte) (map[string]any, error) {
	var v any
	var err error
	if bytes.HasPrefix(data, []byte("bplist00")) {
		v, err = parseBinaryPlist(data)
	} else {
		v, err = parseXMLPlist(data)
	}
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("plist root is not a dictionary")
	}
	return m, nil
}

// ---- XML ----

func parseXMLPlist(data []byte) (any, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("plist: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "plist" {
			break
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("plist: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			return parseXMLValue(dec, se, 0)
		}
	}
}

func parseXMLValue(dec *xml.Decoder, start xml.StartElement, depth int) (any, error) {
	if depth > 64 {
		return nil, errors.New("plist: nesting too deep")
	}
	text := func() (string, error) {
		var s string
		err := dec.DecodeElement(&s, &start)
		return strings.TrimSpace(s), err
	}
	switch start.Name.Local {
	case "string":
		var s string
		err := dec.DecodeElement(&s, &start)
		return s, err
	case "integer":
		s, err := text()
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseInt(s, 10, 64)
		return n, err
	case "real":
		s, err := text()
		if err != nil {
			return nil, err
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err
	case "true", "false":
		if err := dec.Skip(); err != nil {
			return nil, err
		}
		return start.Name.Local == "true", nil
	case "date":
		s, err := text()
		if err != nil {
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339, s)
		return t, nil
	case "data":
		s, err := text()
		if err != nil {
			return nil, err
		}
		return []byte(s), nil
	case "array":
		out := []any{}
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				v, err := parseXMLValue(dec, t, depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			case xml.EndElement:
				return out, nil
			}
		}
	case "dict":
		out := map[string]any{}
		var key string
		haveKey := false
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					var k string
					if err := dec.DecodeElement(&k, &t); err != nil {
						return nil, err
					}
					key, haveKey = k, true
					continue
				}
				v, err := parseXMLValue(dec, t, depth+1)
				if err != nil {
					return nil, err
				}
				if haveKey {
					out[key] = v
					haveKey = false
				}
			case xml.EndElement:
				return out, nil
			}
		}
	}
	if err := dec.Skip(); err != nil {
		return nil, err
	}
	return nil, nil
}

// ---- binary ----

type bplist struct {
	d       []byte
	offs    []int
	refSize int
}

func parseBinaryPlist(d []byte) (any, error) {
	if len(d) < 8+32 {
		return nil, errors.New("bplist: too short")
	}
	tr := d[len(d)-32:]
	offSize, refSize := int(tr[6]), int(tr[7])
	n := binary.BigEndian.Uint64(tr[8:16])
	top := binary.BigEndian.Uint64(tr[16:24])
	tabOff := binary.BigEndian.Uint64(tr[24:32])
	if offSize < 1 || offSize > 8 || refSize < 1 || refSize > 8 || n == 0 || n > 1<<20 || tabOff >= uint64(len(d)) || top >= n {
		return nil, errors.New("bplist: bad trailer")
	}
	if tabOff+n*uint64(offSize) > uint64(len(d)) {
		return nil, errors.New("bplist: bad offset table")
	}
	p := &bplist{d: d, refSize: refSize, offs: make([]int, n)}
	for i := range p.offs {
		o := readUint(d[int(tabOff)+i*offSize : int(tabOff)+(i+1)*offSize])
		if o >= uint64(len(d)) {
			return nil, errors.New("bplist: offset out of range")
		}
		p.offs[i] = int(o)
	}
	return p.object(int(top), 0)
}

func readUint(b []byte) uint64 {
	var v uint64
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v
}

func (p *bplist) length(pos int, info byte) (n int, next int, err error) {
	if info != 0xF {
		return int(info), pos + 1, nil
	}
	if pos+1 >= len(p.d) || p.d[pos+1]>>4 != 0x1 {
		return 0, 0, errors.New("bplist: bad length marker")
	}
	sz := 1 << (p.d[pos+1] & 0xF)
	if sz > 8 || pos+2+sz > len(p.d) {
		return 0, 0, errors.New("bplist: bad length")
	}
	v := readUint(p.d[pos+2 : pos+2+sz])
	if v > uint64(len(p.d)) {
		return 0, 0, errors.New("bplist: length out of range")
	}
	return int(v), pos + 2 + sz, nil
}

func (p *bplist) object(idx, depth int) (any, error) {
	if depth > 64 || idx < 0 || idx >= len(p.offs) {
		return nil, errors.New("bplist: bad object reference")
	}
	pos := p.offs[idx]
	m := p.d[pos]
	typ, info := m>>4, m&0xF
	switch typ {
	case 0x0:
		switch info {
		case 0x8:
			return false, nil
		case 0x9:
			return true, nil
		}
		return nil, nil
	case 0x1:
		sz := 1 << info
		if sz > 8 || pos+1+sz > len(p.d) {
			return nil, errors.New("bplist: bad int")
		}
		return int64(readUint(p.d[pos+1 : pos+1+sz])), nil
	case 0x2:
		sz := 1 << info
		if pos+1+sz > len(p.d) {
			return nil, errors.New("bplist: bad real")
		}
		if sz == 4 {
			return float64(math.Float32frombits(uint32(readUint(p.d[pos+1 : pos+5])))), nil
		}
		return math.Float64frombits(readUint(p.d[pos+1 : pos+9])), nil
	case 0x3:
		if pos+9 > len(p.d) {
			return nil, errors.New("bplist: bad date")
		}
		f := math.Float64frombits(readUint(p.d[pos+1 : pos+9]))
		return time.Unix(978307200, 0).Add(time.Duration(f * float64(time.Second))).UTC(), nil
	case 0x4, 0x5, 0x6:
		n, next, err := p.length(pos, info)
		if err != nil {
			return nil, err
		}
		unit := 1
		if typ == 0x6 {
			unit = 2
		}
		if next+n*unit > len(p.d) {
			return nil, errors.New("bplist: string out of range")
		}
		raw := p.d[next : next+n*unit]
		switch typ {
		case 0x4:
			return append([]byte(nil), raw...), nil
		case 0x5:
			return string(raw), nil
		}
		u := make([]uint16, n)
		for i := range u {
			u[i] = binary.BigEndian.Uint16(raw[2*i:])
		}
		return string(utf16.Decode(u)), nil
	case 0x8:
		return nil, nil // UID
	case 0xA, 0xD:
		n, next, err := p.length(pos, info)
		if err != nil {
			return nil, err
		}
		cnt := n
		if typ == 0xD {
			cnt = 2 * n
		}
		if next+cnt*p.refSize > len(p.d) {
			return nil, errors.New("bplist: collection out of range")
		}
		ref := func(i int) int { return int(readUint(p.d[next+i*p.refSize : next+(i+1)*p.refSize])) }
		if typ == 0xA {
			out := make([]any, n)
			for i := 0; i < n; i++ {
				v, err := p.object(ref(i), depth+1)
				if err != nil {
					return nil, err
				}
				out[i] = v
			}
			return out, nil
		}
		out := make(map[string]any, n)
		for i := 0; i < n; i++ {
			k, err := p.object(ref(i), depth+1)
			if err != nil {
				return nil, err
			}
			v, err := p.object(ref(n+i), depth+1)
			if err != nil {
				return nil, err
			}
			if ks, ok := k.(string); ok {
				out[ks] = v
			}
		}
		return out, nil
	}
	return nil, io.ErrUnexpectedEOF
}
