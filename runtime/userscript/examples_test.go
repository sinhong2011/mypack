package userscript

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The example scripts in examples/userscripts install, and their own node
// tests pass.
func TestExampleScripts(t *testing.T) {
	files, _ := filepath.Glob("../../examples/userscripts/*.user.js")
	if len(files) == 0 {
		t.Fatal("no example userscripts found")
	}
	s := Store{Dir: t.TempDir()}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.Install(context.Background(), src, InstallOptions{}); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	got, errs := s.Load("any.app")
	if len(errs) != 0 || len(got) != len(files) {
		t.Fatalf("load = %d scripts, errors %v", len(got), errs)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	tests, _ := filepath.Glob("../../examples/userscripts/*.test.js")
	for _, tf := range tests {
		if out, err := exec.Command(node, "--test", tf).CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", tf, err, out)
		}
	}
}
