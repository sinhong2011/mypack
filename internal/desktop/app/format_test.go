package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadFolderStatus(t *testing.T) {
	if _, ok := downloadFolderStatus(""); !ok {
		t.Fatal("empty is the system folder")
	}
	dir := t.TempDir()
	if got, ok := downloadFolderStatus(dir); !ok || got != dir {
		t.Fatalf("existing dir: %q %v", got, ok)
	}
	child := filepath.Join(dir, "inbox")
	if got, ok := downloadFolderStatus(child); !ok || got != child {
		t.Fatalf("parent exists: %q %v", got, ok)
	}
	missing := filepath.Join(dir, "nope", "deeper")
	if _, ok := downloadFolderStatus(missing); ok {
		t.Fatal("missing parent should be unusable")
	}
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := downloadFolderStatus(file); ok {
		t.Fatal("a file is not a folder")
	}
}
