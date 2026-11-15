package app

import "testing"

func TestInspectorWidthDefault(t *testing.T) {
	a := &App{toolbarW: 1200}
	if got := a.inspectorWidth(); got != 456 { // 1200 * 0.38
		t.Fatalf("default width = %v, want 456", got)
	}
	a.toolbarW = 900
	if got := a.inspectorWidth(); got != inspectorMin {
		t.Fatalf("narrow default = %v, want %v", got, inspectorMin)
	}
	a.toolbarW = 2000
	if got := a.inspectorWidth(); got != inspectorDefaultMax {
		t.Fatalf("wide default = %v, want %v", got, inspectorDefaultMax)
	}
}

func TestInspectorWidthCustomClamped(t *testing.T) {
	a := &App{toolbarW: 1200}
	a.ui.InspectorWidth = 600
	if got := a.inspectorWidth(); got != 600 {
		t.Fatalf("custom = %v, want 600", got)
	}
	a.setInspectorWidth(900)
	if got := a.inspectorWidth(); got != inspectorMax {
		t.Fatalf("too wide = %v, want %v", got, inspectorMax)
	}
	a.toolbarW = 1000
	a.setInspectorWidth(700) // 1000 - 460 = 540
	if got := a.inspectorWidth(); got != 540 {
		t.Fatalf("preview floor = %v, want 540", got)
	}
	a.toolbarW = 1200
	a.setInspectorWidth(200)
	if got := a.inspectorWidth(); got != inspectorMin {
		t.Fatalf("too narrow = %v, want %v", got, inspectorMin)
	}
}

func TestInspectorWidthReset(t *testing.T) {
	a := &App{toolbarW: 1200}
	a.setInspectorWidth(600)
	a.ui.InspectorWidth = 0
	if got := a.inspectorWidth(); got != 456 {
		t.Fatalf("reset = %v, want default 456", got)
	}
}

func TestInspectorNarrowLeavesPreview(t *testing.T) {
	a := &App{toolbarW: 1200}
	a.setInspectorWidth(720)
	if a.inspectorNarrow() {
		t.Fatal("a 1200 pt bar should keep preview and settings side by side at max width")
	}
	a.toolbarW = 700
	if !a.inspectorNarrow() {
		t.Fatal("a 700 pt bar should stack")
	}
}
