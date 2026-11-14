package icons

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const sampleIcon = "testdata/Sample.icon"

func TestIsIconComposer(t *testing.T) {
	dir := t.TempDir()
	noJSON := filepath.Join(dir, "Empty.icon")
	os.MkdirAll(noJSON, 0o755)
	plain := filepath.Join(dir, "x.png")
	os.WriteFile(plain, []byte("x"), 0o644)
	for path, want := range map[string]bool{
		sampleIcon: true, noJSON: false, plain: false, filepath.Join(dir, "missing.icon"): false, "testdata": false,
	} {
		if got := IsIconComposer(path); got != want {
			t.Errorf("IsIconComposer(%s) = %v, want %v", path, got, want)
		}
	}
}

func requireIconComposer(t *testing.T) {
	t.Helper()
	if !IconComposerAvailable() {
		t.Skip("needs macOS with Xcode 26+ (actool)")
	}
}

func TestCompileIconComposer(t *testing.T) {
	requireIconComposer(t)
	out := t.TempDir()
	c, err := CompileIconComposer(context.Background(), sampleIcon, out)
	if err != nil {
		t.Fatal(err)
	}
	if c.Car != filepath.Join(out, "Assets.car") || !nonEmpty(c.Car) {
		t.Fatalf("Assets.car: %+v", c)
	}
	if c.ICNS != "" {
		b, _ := os.ReadFile(c.ICNS)
		if _, err := ICNSLargestPNG(b); err != nil {
			t.Fatalf("icns: %v", err)
		}
	}
}

func TestCompileIconComposerRejectsNonDocument(t *testing.T) {
	if _, err := CompileIconComposer(context.Background(), t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("want an error for a folder that is not a .icon document")
	}
}

func TestIconComposerPNG(t *testing.T) {
	requireIconComposer(t)
	b, err := IconComposerPNG(context.Background(), sampleIcon)
	if err != nil {
		t.Fatal(err)
	}
	img, err := Decode(b)
	if err != nil || img.Bounds().Dx() != CanvasSize || img.Bounds().Dy() != CanvasSize {
		t.Fatalf("preview: %v %v", err, img.Bounds())
	}
}
