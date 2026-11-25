package inject

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserScriptsScriptEmpty(t *testing.T) {
	if UserScriptsScript(nil) != "" {
		t.Fatal("no scripts should add nothing")
	}
}

func TestUserScriptsEscaping(t *testing.T) {
	out := Build(Options{UserScripts: []UserScript{{Token: "t", Name: "</script><b>", Include: []URLRule{{Source: "^"}}, Code: "1"}}})
	if strings.Contains(out, "</script>") || strings.Contains(out, "<b>") {
		t.Fatalf("descriptor not escaped: %s", out)
	}
}

// usHarness runs the dispatcher in a fake page. window.mygo.call answers the
// bridge methods; the scripts write what they saw to `seen`.
const usHarness = `
const fs = require('fs');
const [,, scriptPath, href] = process.argv;
const calls = [];
const store = { tokA: { n: 1, obj: { a: [1, 2] } } };
global.window = global;
global.location = { href, origin: new URL(href).origin };
global.document = { readyState: 'complete', head: { appendChild(el) { global.styles.push(el.textContent); } },
  documentElement: {}, createElement() { return {}; }, addEventListener() {} };
global.styles = [];
global.seen = {};
global.window.top = global;
global.mygo = { call(method, ...args) {
  calls.push([method, ...args]);
  switch (method) {
    case 'mypack.GMValues': return Promise.resolve(store[args[0]] || {});
    case 'mypack.GMXhr': return Promise.resolve({ status: 200, statusText: 'OK', headers: 'a: b\r\n', finalUrl: args[1].url, text: '{"ok":true}' });
    default: return Promise.resolve(null);
  }
} };
eval(fs.readFileSync(scriptPath, 'utf8'));
setTimeout(() => {
  console.log(JSON.stringify({ seen: global.seen, styles: global.styles, calls }));
}, 50);
`

func TestUserScriptsRunInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	scripts := []UserScript{
		{Token: "tokA", Name: "A", RunAt: "document-start", Include: []URLRule{{Source: `^https://x\.com/`}},
			Requires: []string{"var LIB = 'lib';"},
			Code: `// a comment at the end of the script
seen.a = { lib: LIB, n: GM_getValue('n'), missing: GM_getValue('nope', 7), keys: GM_listValues(), name: GM_info.script.name, uw: unsafeWindow === window };
GM_setValue('n', 2); GM_addStyle('body{}');
GM_xmlhttpRequest({ url: '/api', responseType: 'json', onload: function (r) { seen.xhr = r.response; } });
seen.res = GM_getResourceText('r');
// trailing comment`,
			Resources: []Resource{{Name: "r", MIME: "text/plain", Data: []byte("héllo")}}},
		{Token: "tokB", Name: "B", Include: []URLRule{{Source: `^https://x\.com/`}}, Exclude: []URLRule{{Source: `/home$`}},
			Code: `seen.b = true;`},
		{Token: "tokC", Name: "C", Include: []URLRule{{Source: `^https://other\.com/`}}, Code: `seen.c = true;`},
		{Token: "tokD", Name: "D", Include: []URLRule{{Source: `^`}}, Code: `throw new Error('boom');`},
		{Token: "tokE", Name: "E", Include: []URLRule{{Source: `^`}}, Code: `seen.e = true;`},
	}
	dir := t.TempDir()
	harness := filepath.Join(dir, "h.js")
	script := filepath.Join(dir, "s.js")
	_ = os.WriteFile(harness, []byte(usHarness), 0o644)
	_ = os.WriteFile(script, []byte(Build(Options{UserScripts: scripts})), 0o644)

	run := func(href string) string {
		out, err := exec.Command(node, harness, script, href).CombinedOutput()
		if err != nil {
			t.Fatalf("node: %v\n%s", err, out)
		}
		return string(out)
	}
	out := run("https://x.com/home")
	for _, want := range []string{
		`"lib":"lib"`, `"n":1`, `"missing":7`, `"keys":["n","obj"]`, `"name":"A"`, `"uw":true`,
		`"xhr":{"ok":true}`, `"res":"héllo"`, `"styles":["body{}"]`, `"e":true`,
		`["mypack.GMSetValue","tokA","n","2"]`, `"url":"https://x.com/api"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
	for _, bad := range []string{`"b":true`, `"c":true`} {
		if strings.Contains(out, bad) {
			t.Errorf("unexpected %s in %s", bad, out)
		}
	}
	if out := run("https://x.com/u/status/1"); !strings.Contains(out, `"b":true`) {
		t.Errorf("B should run off /home: %s", out)
	}
}
