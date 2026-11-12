package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The button rules (kit/buttons.go): every button is a pill and every
// icon-only button a circle, and a button beside a text field is as tall
// as the field. These checks keep hand-made buttons from drifting from
// them; build buttons with kit.Button / kit.IconButton instead.

var (
	reSquare = regexp.MustCompile(`ui\.ButtonBase\([^)]*\)[^\n]*\.Size\((\d+), (\d+)\)[^\n]*\.Radius\(([\d.]+)\)`)
	reSmall  = regexp.MustCompile(`kit\.Button\([^\n]*Size: design\.ControlSM`)
)

func TestButtonsFollowThePillRule(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(b), "\n")
		for i, line := range lines {
			// A square hand-made button must be a circle.
			if m := reSquare.FindStringSubmatch(line); m != nil && m[1] == m[2] {
				h, _ := strconv.ParseFloat(m[2], 64)
				r, _ := strconv.ParseFloat(m[3], 64)
				if r != h/2 {
					t.Errorf("%s:%d: a %vx%v button with radius %v: icon-only buttons are circles (use kit.Button with an Icon)", f, i+1, h, h, r)
				}
			}
			// A button beside a text field matches the field's height.
			if reSmall.MatchString(line) {
				for j := max(0, i-6); j < i; j++ {
					if strings.Contains(lines[j], "kit.TextField(") || strings.Contains(lines[j], "kit.URLField(") {
						t.Errorf("%s:%d: a small button beside a text field: use the default size so it lines up with the field", f, i+1)
						break
					}
				}
			}
		}
	}
}
