package builder

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
)

// writePlist renders a map as an XML property list (strings, bools, ints,
// nested dicts/arrays). Keys are sorted for reproducible output.
func writePlist(d map[string]any) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
`)
	plistValue(&b, d, "")
	b.WriteString("</plist>\n")
	return b.Bytes()
}

func plistEsc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func plistValue(b *bytes.Buffer, v any, indent string) {
	switch x := v.(type) {
	case string:
		fmt.Fprintf(b, "%s<string>%s</string>\n", indent, plistEsc(x))
	case bool:
		if x {
			fmt.Fprintf(b, "%s<true/>\n", indent)
		} else {
			fmt.Fprintf(b, "%s<false/>\n", indent)
		}
	case int:
		fmt.Fprintf(b, "%s<integer>%d</integer>\n", indent, x)
	case []any:
		fmt.Fprintf(b, "%s<array>\n", indent)
		for _, e := range x {
			plistValue(b, e, indent+"\t")
		}
		fmt.Fprintf(b, "%s</array>\n", indent)
	case map[string]any:
		fmt.Fprintf(b, "%s<dict>\n", indent)
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(b, "%s\t<key>%s</key>\n", indent, plistEsc(k))
			plistValue(b, x[k], indent+"\t")
		}
		fmt.Fprintf(b, "%s</dict>\n", indent)
	default:
		panic(fmt.Sprintf("plist: unsupported type %T", v))
	}
}
