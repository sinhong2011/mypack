package inject

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMediaInBuild(t *testing.T) {
	if strings.Contains(Build(Options{}), "__mypackMedia") {
		t.Fatal("media shim present without Media")
	}
	if !strings.Contains(Build(Options{Media: true}), "mypack.SetMedia") {
		t.Fatal("media shim missing with Media")
	}
}

// mediaHarness runs the media shim in a fake page. "native" gives the page a
// MediaSession with prototype accessors, as WebKit has; "polyfill" none.
const mediaHarness = `
const vm = require('vm');
const fs = require('fs');
const src = fs.readFileSync(process.argv[2], 'utf8');
const mode = process.argv[3];
const calls = [];
const listeners = {};
function assert(c, m) { if (!c) { console.error('FAIL: ' + m); process.exit(1); } }
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

class MediaSession {
  constructor() { this._m = null; this._s = 'none'; this.native = []; }
  get metadata() { return this._m; }
  set metadata(v) { this._m = v; }
  get playbackState() { return this._s; }
  set playbackState(v) { if (!['none', 'paused', 'playing'].includes(v)) throw new TypeError('bad'); this._s = v; }
  setActionHandler(a, fn) { this.native.push(a); }
  setPositionState(s) {}
}
const navigator = {};
if (mode === 'native') navigator.mediaSession = new MediaSession();
const win = {
  location: { href: 'https://music.example.com/' },
  mygo: { call: (m, ...a) => { calls.push([m, ...a]); return Promise.resolve(); } },
  navigator, Promise, setTimeout, clearTimeout, Date, Math, Object, Array, String, Number, JSON, URL, TypeError, isFinite,
  document: { title: 'Doc title', addEventListener: (t, fn, cap) => { assert(cap === true, 'capture'); listeners[t] = (e) => fn(Object.assign({ type: t }, e)); } },
};
if (mode === 'native') win.MediaMetadata = class { constructor(i) { Object.assign(this, i); } };
win.window = win; win.top = win;
const ctx = vm.createContext(win);
vm.runInContext(src, ctx);
const media = () => calls.filter((c) => c[0] === 'mypack.SetMedia').map((c) => c[1]);
const last = () => media().pop();

(async () => {
  assert(media().length === 1 && last().state === 'none', 'clears on load');
  const ms = ctx.navigator.mediaSession;
  assert(ms && typeof ctx.MediaMetadata === 'function', 'mediaSession available');

  // An element playing with sound reports; muted autoplay does not.
  const muted = { tagName: 'VIDEO', muted: true, volume: 1, paused: false, ended: false, duration: 30, currentTime: 1, playbackRate: 1 };
  listeners.play({ target: muted });
  await sleep(250);
  assert(media().length === 1, 'muted autoplay ignored: ' + JSON.stringify(media()));
  const el = { tagName: 'AUDIO', muted: false, volume: 1, paused: false, ended: false, duration: 180, currentTime: 5, playbackRate: 1,
    played: 0, play() { this.played++; this.paused = false; return Promise.resolve(); }, pause() { this.paused = true; } };
  listeners.play({ target: el });
  await sleep(250);
  let p = last();
  assert(p.state === 'playing' && p.title === 'Doc title' && p.duration === 180 && p.position === 5, 'element: ' + JSON.stringify(p));
  assert(p.actions.includes('play') && p.actions.includes('seekto'), 'element actions');

  // Without handlers, commands drive the element.
  ctx.__mypackMedia.command('pause', 0);
  assert(el.paused, 'pause the element');
  ctx.__mypackMedia.command('seekto', 42);
  assert(el.currentTime === 42, 'seek the element');
  ctx.__mypackMedia.command('play', 0);
  assert(el.played === 1, 'play the element');

  // mediaSession metadata and handlers win.
  ms.metadata = new ctx.MediaMetadata({ title: 'Song', artist: 'Band', album: 'LP',
    artwork: [{ src: '/s.jpg', sizes: '96x96' }, { src: '/l.jpg', sizes: '512x512' }] });
  let next = 0, seekTo = -1;
  ms.setActionHandler('nexttrack', () => next++);
  ms.setActionHandler('seekto', (d) => { seekTo = d.seekTime; });
  ms.playbackState = 'playing';
  ms.setPositionState({ duration: 300, position: 60, playbackRate: 1 });
  await sleep(250);
  p = last();
  assert(p.title === 'Song' && p.artist === 'Band' && p.album === 'LP', 'metadata: ' + JSON.stringify(p));
  assert(p.artwork === 'https://music.example.com/l.jpg', 'largest artwork: ' + p.artwork);
  assert(p.duration === 300 && p.position >= 60 && p.position < 61, 'position state: ' + JSON.stringify(p));
  assert(p.actions.includes('nexttrack'), 'handler actions');
  assert(ms.metadata.title === 'Song' && ms.playbackState === 'playing', 'getters still work');
  if (mode === 'native') {
    assert(ms.native.includes('nexttrack'), 'native setActionHandler still called');
    let threw = false; try { ms.playbackState = 'bogus'; } catch (e) { threw = true; }
    assert(threw, 'native validation kept');
  }
  ctx.__mypackMedia.command('nexttrack', 0);
  ctx.__mypackMedia.command('seekto', 99);
  assert(next === 1 && seekTo === 99, 'handlers called');

  // Unchanged state is not resent; a pause is.
  const n = media().length;
  listeners.durationchange({ target: el });
  await sleep(250);
  assert(media().length === n || last().state === 'playing', 'no needless resend');
  ms.playbackState = 'paused';
  await sleep(250);
  assert(last().state === 'paused', 'pause reported');
  console.log('ok ' + mode);
})();
`

func TestMediaInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	dir := t.TempDir()
	harness := filepath.Join(dir, "h.js")
	script := filepath.Join(dir, "media.js")
	if err := os.WriteFile(harness, []byte(mediaHarness), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte(MediaScript()), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"native", "polyfill"} {
		if out, err := exec.Command(node, harness, script, mode).CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", mode, err, out)
		}
	}
}
