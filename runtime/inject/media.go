package inject

// MediaScript reports what the page plays to the runtime, which shows it
// where the system shows media (Now Playing on macOS, MPRIS on Linux) and
// sends the media keys back as commands. It reads navigator.mediaSession
// (polyfilled where the webview lacks it) and falls back to the <audio> or
// <video> element that played last. It talks to the bound Go service
// "mypack" through window.mygo.call:
//
//	mypack.SetMedia({state, title, artist, album, artwork, duration, position, rate, actions})
//
// and the runtime calls window.__mypackMedia.command(action, seconds).
func MediaScript() string { return mediaJS }

const mediaJS = `;(function () {
  'use strict';
  var mg = window.mygo;
  if (!mg || typeof mg.call !== 'function') return; // frame without the native bridge
  try { if (window.top !== window) return; } catch (e) { return; }
  if (window.__mypackMedia) return;

  var ACTIONS = ['play', 'pause', 'stop', 'nexttrack', 'previoustrack', 'seekto'];
  var handlers = {};
  var meta = null, pbState = 'none', pos = null, active = null;
  var lastKey = 'none', timer = 0, forcePos = false; // the load reports 'none' (below)

  function send(p) { try { mg.call('mypack.SetMedia', p).then(function () {}, function () {}); } catch (e) {} }
  function schedule(seek) {
    if (seek) forcePos = true;
    if (timer) return;
    timer = setTimeout(flush, 200);
  }
  function num(v) { v = Number(v); return isFinite(v) && v > 0 ? v : 0; }
  function str(v) { return v == null ? '' : String(v).slice(0, 300); }

  // An element counts once it played with sound: muted autoplay (hero
  // videos, previews) must not take the media keys.
  function audible(el) { return el && !el.muted && el.volume > 0; }

  function artworkURL() {
    var list = meta && meta.artwork;
    if (!list || !list.length) return '';
    var best = null, bestSize = -1;
    for (var i = 0; i < list.length; i++) {
      var a = list[i]; if (!a || !a.src) continue;
      var size = 0, m = /(\d+)x(\d+)/.exec(String(a.sizes || ''));
      if (m) size = Number(m[1]) * Number(m[2]);
      if (size > bestSize) { best = a; bestSize = size; }
    }
    if (!best) return '';
    try { return new URL(String(best.src), location.href).href; } catch (e) { return ''; }
  }

  function snapshot() {
    var el = audible(active) ? active : null;
    var playing = pbState === 'playing' || (pbState === 'none' && !!el && !el.paused && !el.ended);
    if (!meta && !el) return { state: 'none' };
    var p = {
      state: playing ? 'playing' : 'paused',
      title: str(meta && meta.title) || str(document.title),
      artist: str(meta && meta.artist), album: str(meta && meta.album),
      artwork: artworkURL(), duration: 0, position: 0, rate: 1, actions: []
    };
    if (pos) {
      p.duration = num(pos.duration); p.rate = num(pos.playbackRate) || 1;
      var at = num(pos.position);
      if (playing) at += (Date.now() - pos.at) / 1000 * p.rate;
      p.position = p.duration ? Math.min(at, p.duration) : at;
    } else if (el) {
      p.duration = num(el.duration); p.position = num(el.currentTime); p.rate = num(el.playbackRate) || 1;
    }
    var acts = {};
    ACTIONS.forEach(function (a) { if (handlers[a]) acts[a] = 1; });
    if (el) { acts.play = acts.pause = 1; if (p.duration) acts.seekto = 1; }
    p.actions = Object.keys(acts);
    return p;
  }

  function flush() {
    timer = 0;
    var p = snapshot();
    // Position advances by itself: it only matters on a jump.
    var key = p.state === 'none' ? 'none'
      : JSON.stringify([p.state, p.title, p.artist, p.album, p.artwork, Math.round(p.duration), p.rate, p.actions]);
    if (key === 'none' && lastKey === 'none') { forcePos = false; return; }
    if (key === lastKey && !forcePos) return;
    lastKey = key; forcePos = false;
    send(p);
  }

  // ---- navigator.mediaSession ------------------------------------------------
  var ms = navigator.mediaSession;
  if (!ms) {
    if (typeof window.MediaMetadata !== 'function') {
      window.MediaMetadata = function MediaMetadata(init) {
        init = init || {};
        this.title = str(init.title); this.artist = str(init.artist); this.album = str(init.album);
        this.artwork = Array.isArray(init.artwork) ? init.artwork.slice() : [];
      };
    }
    ms = { metadata: null, playbackState: 'none', setActionHandler: function () {}, setPositionState: function () {} };
    try { Object.defineProperty(navigator, 'mediaSession', { value: ms, configurable: true }); } catch (e) {}
  }
  var target = Object.getPrototypeOf(ms);
  if (!target || target === Object.prototype) target = ms;
  function wrapProp(name, onSet) {
    var d = Object.getOwnPropertyDescriptor(target, name);
    var val = ms[name];
    try {
      Object.defineProperty(ms, name, {
        configurable: true, enumerable: true,
        get: function () { return d && d.get ? d.get.call(ms) : val; },
        set: function (v) {
          if (d && d.set) d.set.call(ms, v); // invalid values throw, as natively
          val = v; onSet(v);
        }
      });
    } catch (e) {}
  }
  wrapProp('metadata', function (v) { meta = v || null; schedule(); });
  wrapProp('playbackState', function (v) { pbState = v === 'playing' || v === 'paused' ? v : 'none'; schedule(true); });
  var origSAH = ms.setActionHandler, origSPS = ms.setPositionState;
  try {
    Object.defineProperty(ms, 'setActionHandler', { configurable: true, value: function (action, fn) {
      handlers[action] = typeof fn === 'function' ? fn : null;
      schedule();
      try { if (origSAH) return origSAH.call(ms, action, fn); } catch (e) {}
    } });
    Object.defineProperty(ms, 'setPositionState', { configurable: true, value: function (s) {
      pos = s && typeof s === 'object' ? { duration: s.duration, position: s.position, playbackRate: s.playbackRate, at: Date.now() } : null;
      schedule(true);
      if (origSPS) return origSPS.call(ms, s);
    } });
  } catch (e) {}

  // ---- <audio> / <video> ------------------------------------------------------
  function onMedia(e) {
    var el = e.target;
    if (!el || (el.tagName !== 'AUDIO' && el.tagName !== 'VIDEO')) return;
    if (e.type === 'play' || e.type === 'playing') { if (audible(el)) active = el; }
    else if (el !== active) return;
    schedule(e.type === 'seeked' || e.type === 'play' || e.type === 'pause');
  }
  ['play', 'playing', 'pause', 'ended', 'seeked', 'durationchange', 'ratechange', 'volumechange', 'emptied'].forEach(function (t) {
    try { document.addEventListener(t, onMedia, true); } catch (e) {}
  });

  // ---- commands from the runtime ----------------------------------------------
  function command(action, seconds) {
    var fn = handlers[action];
    if (!fn && action === 'stop') { action = 'pause'; fn = handlers.pause; }
    if (fn) {
      try { fn({ action: action, seekTime: seconds }); } catch (e) { setTimeout(function () { throw e; }); }
    } else if (active) {
      try {
        if (action === 'play') { var r = active.play(); if (r && r.catch) r.catch(function () {}); }
        else if (action === 'pause') active.pause();
        else if (action === 'seekto' && isFinite(seconds)) active.currentTime = seconds;
      } catch (e) {}
    }
    schedule(true);
  }
  try { Object.defineProperty(window, '__mypackMedia', { value: Object.freeze({ command: command }) }); } catch (e) {}

  // A new document starts with nothing playing, whatever the last one played.
  send({ state: 'none' });
})();
`
