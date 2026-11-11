package appservice

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, c := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResolveWebProject(t *testing.T) {
	t.Run("built vite project uses dist", func(t *testing.T) {
		d := t.TempDir()
		writeFiles(t, d, map[string]string{
			"package.json":    `{"name":"@acme/trader-memos","devDependencies":{"vite":"^6"}}`,
			"index.html":      `<script type="module" src="/src/main.tsx"></script>`,
			"dist/index.html": `<!doctype html><title>built</title>`,
		})
		site, proj, ok, err := resolveWebProject(d)
		if err != nil || !ok || site != filepath.Join(d, "dist") {
			t.Fatalf("site=%q ok=%v err=%v", site, ok, err)
		}
		if proj.Framework != "Vite" || proj.Name != "Trader Memos" {
			t.Fatalf("proj=%+v", proj)
		}
	})
	t.Run("unbuilt vite project explains how to build", func(t *testing.T) {
		d := t.TempDir()
		writeFiles(t, d, map[string]string{
			"package.json": `{"name":"app","devDependencies":{"vite":"^6"}}`,
			"index.html":   `<script type="module" src="/src/main.tsx"></script>`,
		})
		_, _, ok, err := resolveWebProject(d)
		if !ok || err == nil || !strings.Contains(err.Error(), "hasn't been built") {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})
	t.Run("next.js SSR is refused with guidance", func(t *testing.T) {
		d := t.TempDir()
		writeFiles(t, d, map[string]string{"package.json": `{"dependencies":{"next":"15"}}`})
		_, proj, _, err := resolveWebProject(d)
		if err == nil || proj.Static {
			t.Fatalf("expected SSR refusal, got %v %+v", err, proj)
		}
	})
	t.Run("next.js static export uses out", func(t *testing.T) {
		d := t.TempDir()
		writeFiles(t, d, map[string]string{
			"package.json":   `{"dependencies":{"next":"15"}}`,
			"next.config.ts": "export default { output: 'export' }",
			"out/index.html": "<!doctype html>",
		})
		site, _, ok, err := resolveWebProject(d)
		if err != nil || !ok || site != filepath.Join(d, "out") {
			t.Fatalf("site=%q err=%v", site, err)
		}
	})
	t.Run("plain folder is used as is", func(t *testing.T) {
		d := t.TempDir()
		writeFiles(t, d, map[string]string{"index.html": "<!doctype html>"})
		site, _, ok, err := resolveWebProject(d)
		if err != nil || ok || site != d {
			t.Fatalf("site=%q ok=%v err=%v", site, ok, err)
		}
	})
}
