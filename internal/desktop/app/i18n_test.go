package app

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"mypack.dev/mypack/internal/i18n"
)

// Every user-visible string in the GUI goes through T with a stable ID; this
// fails when an ID is missing from the English catalog.
func TestGUIMessageIDsExist(t *testing.T) {
	re := regexp.MustCompile(`T\("([a-zA-Z0-9_.]+)"`)
	dynamic := []string{"set.size.compact", "set.size.standard", "set.size.large"}
	for _, v := range visibleSteps {
		dynamic = append(dynamic, v.id, v.id+".active")
	}
	files, _ := filepath.Glob("../*/*.go")
	seen := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			seen[m[1]] = true
		}
	}
	for _, id := range dynamic {
		seen[id] = true
	}
	delete(seen, "set.size.")
	if len(seen) < 100 {
		t.Fatalf("found only %d IDs; scan broken?", len(seen))
	}
	for id := range seen {
		if !i18n.Has("en", id) {
			t.Errorf("missing English string for %q", id)
		}
	}
}
