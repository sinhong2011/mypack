package inject

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebAPIsScriptConfig(t *testing.T) {
	if WebAPIsScript(WebAPIs{}) != "" {
		t.Fatal("nothing to expose should give an empty script")
	}
	s := WebAPIsScript(WebAPIs{Notifications: "ask", Perms: map[string]string{"https://a.b": "granted"}, Badge: true})
	if !strings.Contains(s, `"mode":"ask"`) || !strings.Contains(s, `"https://a.b":"granted"`) || !strings.Contains(s, `"badge":true`) {
		t.Fatalf("config not embedded: %.200s", s)
	}
	// Config values can never break out of the script.
	evil := WebAPIsScript(WebAPIs{Notifications: "allow", Perms: map[string]string{"</script><b>": "granted "}})
	if strings.Contains(evil, "</script>") || strings.Contains(evil, " ") {
		t.Fatal("config not escaped")
	}
	if !strings.Contains(Build(Options{WebAPIs: WebAPIs{Notifications: "allow"}}), "mypack.") {
		t.Fatal("shim missing from Build")
	}
}

// nodeHarness runs the shim in a fake page: window.mygo.call records calls and
// answers like the Go side would.
const nodeHarness = `
const vm = require('vm');
const fs = require('fs');
const src = fs.readFileSync(process.argv[2], 'utf8');
const calls = [];
let answers = { Notify: 'shown', RequestPermission: 'granted', Permission: 'default' };
const win = {
  location: { origin: 'https://mail.example.com', href: 'https://mail.example.com/' },
  mygo: { call: (m, ...a) => { calls.push([m, ...a]); const k = m.split('.')[1]; return Promise.resolve(answers[k]); } },
  EventTarget, Event, Promise, setTimeout, clearTimeout, Date, Math, Object, Array, String, Number, TypeError, URL,
  Navigator: function () {}, console,
  ServiceWorkerRegistration: function () {},
};
win.navigator = Object.create(win.Navigator.prototype);
win.window = win;
const ctx = vm.createContext(win);
function load(cfgAnswers) { if (cfgAnswers) answers = Object.assign(answers, cfgAnswers); vm.runInContext(src, ctx); }
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const assert = (c, m) => { if (!c) { console.error('FAIL: ' + m); process.exit(1); } };
(async () => {
  const mode = process.argv[3];
  load();
  const N = ctx.Notification;
  if (mode === 'allow') {
    assert(N.permission === 'granted', 'allow => granted');
    const n = new N('Hi', { body: 'there', tag: 't1', silent: true });
    let shown = 0, clicked = 0;
    n.onshow = () => shown++;
    n.addEventListener('click', () => clicked++);
    n.onclick = () => clicked += 10;
    await sleep(20);
    const c = calls.find((x) => x[0] === 'mypack.Notify');
    assert(c && c[1].title === 'Hi' && c[1].body === 'there' && c[1].tag === 't1' && c[1].silent === true && c[1].id, 'Notify payload ' + JSON.stringify(calls));
    assert(shown === 1, 'show event');
    ctx.__mypackNotify.click(c[1].id);
    assert(clicked === 11, 'click handlers: ' + clicked);
    // same tag replaces: the old one gets close
    let closed = 0; n.onclose = () => closed++;
    new N('Hi2', { tag: 't1' });
    await sleep(20);
    assert(closed === 1, 'replaced notification closes');
    // requestPermission resolves with the setting and supports callbacks
    let cb = null;
    const r = await N.requestPermission((v) => (cb = v));
    assert(r === 'granted' && cb === 'granted', 'requestPermission');
    assert(N.maxActions === 0 && typeof N === 'function', 'shape');
    let threw = false; try { new N(); } catch (e) { threw = e instanceof TypeError; }
    assert(threw, 'no-arg constructor throws TypeError');
    // showNotification delegates
    const before = calls.filter((x) => x[0] === 'mypack.Notify').length;
    await ctx.ServiceWorkerRegistration.prototype.showNotification.call({}, 'From SW', { body: 'x' });
    await sleep(20);
    assert(calls.filter((x) => x[0] === 'mypack.Notify').length === before + 1, 'showNotification');
  }
  if (mode === 'deny') {
    assert(N.permission === 'denied', 'deny => denied');
    let err = 0; const n = new N('x'); n.onerror = () => err++;
    await sleep(20);
    assert(err === 1 && !calls.some((x) => x[0] === 'mypack.Notify'), 'denied: error event, no native call');
    assert((await N.requestPermission()) === 'denied', 'denied request');
  }
  if (mode === 'ask') {
    assert(N.permission === 'default' || N.permission === 'granted', 'ask initial');
    const r = await N.requestPermission();
    assert(r === 'granted' && N.permission === 'granted', 'ask => prompt answer');
    assert(calls.some((x) => x[0] === 'mypack.RequestPermission'), 'asked Go');
  }
  if (mode === 'badge') {
    const nav = ctx.navigator;
    await nav.setAppBadge(5);
    await sleep(300);
    assert(calls.some((x) => x[0] === 'mypack.SetBadge' && x[1] === 5), 'setAppBadge(5): ' + JSON.stringify(calls));
    await nav.setAppBadge(6); await nav.setAppBadge(7); await nav.setAppBadge(8);
    await sleep(600);
    const sets = calls.filter((x) => x[0] === 'mypack.SetBadge').map((x) => x[1]);
    assert(sets.length === 2 && sets[1] === 8, 'throttled to the last value: ' + JSON.stringify(sets));
    await nav.setAppBadge();
    await sleep(600);
    assert(calls.filter((x) => x[0] === 'mypack.SetBadge').pop()[1] === -1, 'flag badge');
    await nav.clearAppBadge();
    await sleep(600);
    assert(calls.filter((x) => x[0] === 'mypack.SetBadge').pop()[1] === 0, 'clear');
    let rejected = false; try { await nav.setAppBadge(-3); } catch (e) { rejected = true; }
    assert(rejected, 'negative count rejects');
  }
  if (mode === 'badge-off') {
    await ctx.navigator.setAppBadge(5);
    await sleep(300);
    assert(!calls.some((x) => x[0] === 'mypack.SetBadge'), 'badge off: API exists but does nothing');
  }
  if (mode === 'nobridge') {
    assert(ctx.Notification === undefined, 'no bridge => no shim');
  }
  console.log('ok ' + mode);
})();
`

func TestWebAPIsInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	dir := t.TempDir()
	harness := filepath.Join(dir, "h.js")
	if err := os.WriteFile(harness, []byte(nodeHarness), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		mode string
		o    WebAPIs
	}{
		{"allow", WebAPIs{Notifications: "allow", Badge: true}},
		{"deny", WebAPIs{Notifications: "deny", Badge: true}},
		{"ask", WebAPIs{Notifications: "ask", Badge: true}},
		{"badge", WebAPIs{Notifications: "allow", Badge: true}},
		{"badge-off", WebAPIs{Notifications: "allow", Badge: false}},
	}
	for _, c := range cases {
		script := filepath.Join(dir, c.mode+".js")
		if err := os.WriteFile(script, []byte(WebAPIsScript(c.o)), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(node, harness, script, c.mode).CombinedOutput()
		if err != nil {
			t.Errorf("%s: %v\n%s", c.mode, err, out)
		}
	}
}
