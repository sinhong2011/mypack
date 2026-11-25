package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mypack.dev/mypack/runtime/userscript"
)

func TestScriptsCommands(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(userscript.EnvDir, filepath.Join(dir, "store"))
	file := filepath.Join(dir, "a.user.js")
	src := "// ==UserScript==\n// @name A\n// @version 1\n// @match https://x.com/*\n// @grant GM_xmlhttpRequest\n// ==/UserScript==\n1;\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFake()
	code, out, _ := run(t, f, "scripts", "install", file, "--app", "com.x")
	if code != ExitOK || !strings.Contains(out, "apps: com.x") || !strings.Contains(out, "page's own site only") {
		t.Fatalf("install: %d %s", code, out)
	}
	ix, _ := scriptsStore().Read()
	if len(ix.Scripts) != 1 {
		t.Fatalf("index = %+v", ix)
	}
	id := ix.Scripts[0].ID

	code, out, _ = run(t, f, "--json", "scripts", "list")
	if m := oneJSON(t, out); code != ExitOK || len(m["scripts"].([]any)) != 1 {
		t.Fatalf("list: %d %s", code, out)
	}
	if code, out, _ = run(t, f, "scripts", "disable", id); code != ExitOK || !strings.Contains(out, "off") {
		t.Fatalf("disable: %d %s", code, out)
	}
	if code, out, _ = run(t, f, "scripts", "scope", id, "--all"); code != ExitOK || !strings.Contains(out, "all apps") {
		t.Fatalf("scope: %d %s", code, out)
	}
	if code, _, _ = run(t, f, "scripts", "scope", id); code != ExitUsage {
		t.Fatalf("scope without target: %d", code)
	}
	if code, _, _ = run(t, f, "scripts", "update", id); code == ExitOK {
		t.Fatal("update of a file install should fail")
	}
	if code, _, _ = run(t, f, "scripts", "remove", id); code != ExitOK {
		t.Fatalf("remove: %d", code)
	}
	code, out, _ = run(t, f, "--json", "scripts", "remove", id)
	if m := oneJSON(t, out); code == ExitOK || m["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Fatalf("second remove: %d %s", code, out)
	}
	if code, _, _ = run(t, f, "scripts", "install", filepath.Join(dir, "missing.js")); code != ExitValidation {
		t.Fatalf("missing file: %d", code)
	}
}
