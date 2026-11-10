package inject

import (
	"strings"
	"testing"
)

func TestBuildEmpty(t *testing.T) {
	if got := Build(Options{}); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestBuildCSSEscaping(t *testing.T) {
	css := `body::after{content:"</style><script>alert(1)</script>"}` + "\nbody{color:red}"
	out := Build(Options{CSS: []string{css}})
	if strings.Contains(out, "</style>") || strings.Contains(out, "<script>") {
		t.Fatalf("css not escaped: %s", out)
	}
	if !strings.Contains(out, "DOMContentLoaded") || !strings.Contains(out, "MutationObserver") {
		t.Fatal("css keeper missing")
	}
}

func TestBuildJSOrderAndIsolation(t *testing.T) {
	out := Build(Options{JS: []string{"window.a=1", "  ", "window.b=2"}})
	ia, ib := strings.Index(out, "window.a=1"), strings.Index(out, "window.b=2")
	if ia < 0 || ib < ia {
		t.Fatalf("order wrong: %s", out)
	}
	if strings.Count(out, "try {") != 2 {
		t.Fatalf("each script should be wrapped: %s", out)
	}
}

func TestShortcuts(t *testing.T) {
	out := Build(Options{DisabledShortcuts: []string{"CmdOrCtrl+R", "Ctrl+Shift+I", "bogus++"}})
	if !strings.Contains(out, `"cmdOrCtrl":true,"key":"r"`) || !strings.Contains(out, `"ctrl":true,"shift":true,"key":"i"`) {
		t.Fatalf("specs missing: %s", out)
	}
}

func TestNormalize(t *testing.T) {
	a, b := Normalize("CommandOrControl+Shift+R"), Normalize("shift+cmdorctrl+r")
	if a != b {
		t.Fatalf("%q != %q", a, b)
	}
	if Normalize("CmdOrCtrl+Plus") != Normalize("cmdorctrl++") {
		t.Fatal("plus alias")
	}
}

func TestDrag(t *testing.T) {
	if !strings.Contains(Build(Options{DragStrip: true}), "--app-region:drag") {
		t.Fatal("drag strip missing")
	}
}
