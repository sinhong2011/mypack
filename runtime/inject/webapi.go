package inject

import (
	"encoding/json"
	"strings"
)

// WebAPIs configures the Web API shims that WKWebView / WebKitGTK lack:
// window.Notification, ServiceWorkerRegistration.showNotification and the
// Badging API (navigator.setAppBadge / clearAppBadge). They talk to the
// runtime through the bound Go service "mypack" (window.mygo.call).
type WebAPIs struct {
	// Notifications is "allow", "ask" or "deny" ("" disables the shim).
	Notifications string
	// Perms are the remembered per-origin answers for "ask" mode
	// ("granted" / "denied"), keyed by origin such as "https://x.com".
	Perms map[string]string
	// Badge makes setAppBadge/clearAppBadge reach the Dock/taskbar; when
	// false they still exist (sites call them unconditionally) but do nothing.
	Badge bool
}

type webAPIsConfig struct {
	Mode  string            `json:"mode"`
	Perms map[string]string `json:"perms"`
	Badge bool              `json:"badge"`
}

// WebAPIsScript returns the shim script ("" when o.Notifications is empty and
// there is nothing to expose).
func WebAPIsScript(o WebAPIs) string {
	if o.Notifications == "" && !o.Badge {
		return ""
	}
	mode := o.Notifications
	if mode == "" {
		mode = "deny"
	}
	b, _ := json.Marshal(webAPIsConfig{Mode: mode, Perms: o.Perms, Badge: o.Badge})
	cfg := strings.NewReplacer("<", `<`, ">", `>`, " ", ` `, " ", ` `).Replace(string(b))
	return strings.Replace(webAPIsJS, "__MYPACK_CFG__", cfg, 1)
}

const webAPIsJS = `;(function () {
  'use strict';
  var CFG = __MYPACK_CFG__;
  var mg = window.mygo;
  if (!mg || typeof mg.call !== 'function') return; // frame without the native bridge
  if (window.__mypackWeb) return;
  try { Object.defineProperty(window, '__mypackWeb', { value: true }); } catch (e) {}

  function call(method) {
    var args = Array.prototype.slice.call(arguments, 1);
    return mg.call.apply(mg, ['mypack.' + method].concat(args));
  }
  function def(obj, name, value) {
    try { Object.defineProperty(obj, name, { value: value, configurable: true, writable: true, enumerable: false }); }
    catch (e) { try { obj[name] = value; } catch (e2) {} }
  }

  // ---- Notifications -------------------------------------------------------
  var origin = ''; try { origin = String(location.origin); } catch (e) {}
  var perm = CFG.mode === 'allow' ? 'granted'
           : CFG.mode === 'deny' ? 'denied'
           : (CFG.perms && CFG.perms[origin]) || 'default';
  if (CFG.mode === 'ask') {
    call('Permission').then(function (p) { if (p === 'granted' || p === 'denied' || p === 'default') perm = p; }, function () {});
  }
  var seq = 0, live = {}, liveOrder = [], byTag = {};

  function toDataURL(src) {
    return new Promise(function (resolve) {
      if (typeof src !== 'string' || !src) return resolve('');
      if (src.indexOf('data:image/') === 0) return resolve(src.length < 600000 ? src : '');
      var done = false, t = setTimeout(function () { if (!done) { done = true; resolve(''); } }, 2500);
      try {
        fetch(new URL(src, location.href).href, { credentials: 'include' }).then(function (r) {
          return r.ok ? r.blob() : null;
        }).then(function (blob) {
          if (!blob || blob.size > 400000) return null;
          return new Promise(function (res) {
            var fr = new FileReader();
            fr.onload = function () { res(String(fr.result || '')); };
            fr.onerror = function () { res(''); };
            fr.readAsDataURL(blob);
          });
        }).then(function (d) { if (!done) { done = true; clearTimeout(t); resolve(d || ''); } },
                function () { if (!done) { done = true; clearTimeout(t); resolve(''); } });
      } catch (e) { done = true; clearTimeout(t); resolve(''); }
    });
  }

  function fire(n, type) {
    var ev;
    try { ev = new Event(type); } catch (e) { return; }
    try { var h = n['on' + type]; if (typeof h === 'function') h.call(n, ev); } catch (e) { setTimeout(function () { throw e; }); }
    try { n.dispatchEvent(ev); } catch (e) {}
  }
  function forget(id) {
    var n = live[id];
    delete live[id];
    var i = liveOrder.indexOf(id); if (i >= 0) liveOrder.splice(i, 1);
    if (n && n.tag && byTag[n.tag] === id) delete byTag[n.tag];
    return n;
  }

  class Notification extends EventTarget {
    constructor(title, options) {
      super();
      if (arguments.length < 1) throw new TypeError("Failed to construct 'Notification': 1 argument required, but only 0 present.");
      var o = options && typeof options === 'object' ? options : {};
      var self = this;
      var props = {
        title: String(title), body: o.body == null ? '' : String(o.body), tag: o.tag == null ? '' : String(o.tag),
        icon: o.icon == null ? '' : String(o.icon), image: o.image == null ? '' : String(o.image),
        badge: o.badge == null ? '' : String(o.badge), lang: o.lang == null ? '' : String(o.lang),
        dir: o.dir === 'ltr' || o.dir === 'rtl' ? o.dir : 'auto', silent: o.silent == null ? null : !!o.silent,
        requireInteraction: !!o.requireInteraction, renotify: !!o.renotify, data: o.data === undefined ? null : o.data,
        timestamp: typeof o.timestamp === 'number' ? o.timestamp : Date.now(), vibrate: o.vibrate || [], actions: []
      };
      Object.keys(props).forEach(function (k) {
        Object.defineProperty(self, k, { value: props[k], enumerable: true });
      });
      self.onclick = self.onshow = self.onclose = self.onerror = null;
      var id = 'n' + (++seq) + '-' + Math.random().toString(36).slice(2, 8);
      Object.defineProperty(self, '__id', { value: id });
      if (perm !== 'granted') {
        setTimeout(function () { fire(self, 'error'); }, 0);
        return;
      }
      if (props.tag && byTag[props.tag]) {
        var old = forget(byTag[props.tag]);
        if (old) setTimeout(function () { fire(old, 'close'); }, 0);
      }
      live[id] = self; liveOrder.push(id);
      if (props.tag) byTag[props.tag] = id;
      while (liveOrder.length > 100) forget(liveOrder[0]);
      toDataURL(props.icon).then(function (icon) {
        return call('Notify', { id: id, title: props.title, body: props.body, tag: props.tag, icon: icon, silent: props.silent === true });
      }).then(function (r) {
        if (r === 'shown') fire(self, 'show');
        else { if (r === 'denied') perm = 'denied'; forget(id); fire(self, 'error'); }
      }, function () { forget(id); fire(self, 'error'); });
    }
    close() {
      var id = this.__id;
      if (id && live[id]) {
        forget(id);
        call('CloseNotification', id).then(function () {}, function () {});
        var self = this;
        setTimeout(function () { fire(self, 'close'); }, 0);
      }
    }
    static get permission() { return perm; }
    static get maxActions() { return 0; }
    static requestPermission(cb) {
      var p;
      if (perm !== 'default') p = Promise.resolve(perm);
      else p = call('RequestPermission').then(function (r) {
        perm = r === 'granted' ? 'granted' : r === 'denied' ? 'denied' : 'default';
        return perm;
      }, function () { return 'denied'; });
      if (typeof cb === 'function') p.then(function (r) { try { cb(r); } catch (e) {} });
      return p;
    }
  }
  def(window, 'Notification', Notification);
  // Called by the runtime when the user clicks / dismisses the native notification.
  def(window, '__mypackNotify', Object.freeze({
    click: function (id) { var n = live[id]; if (n) fire(n, 'click'); },
    close: function (id) { var n = forget(id); if (n) fire(n, 'close'); }
  }));

  try {
    if (typeof ServiceWorkerRegistration !== 'undefined' && ServiceWorkerRegistration.prototype) {
      def(ServiceWorkerRegistration.prototype, 'showNotification', function (title, options) {
        try { new Notification(title, options); } catch (e) { return Promise.reject(e); }
        return Promise.resolve();
      });
      def(ServiceWorkerRegistration.prototype, 'getNotifications', function () { return Promise.resolve([]); });
    }
  } catch (e) {}

  // ---- Badging API ---------------------------------------------------------
  var lastSent = '', lastAt = 0, timer = 0, pendingVal = null;
  function sendBadge(v) {
    if (!CFG.badge) return;
    var key = String(v);
    var now = Date.now();
    pendingVal = v;
    if (timer) return;
    var wait = Math.max(0, 250 - (now - lastAt));
    timer = setTimeout(function () {
      timer = 0; lastAt = Date.now();
      var val = pendingVal, k = String(val);
      if (k === lastSent) return;
      lastSent = k;
      call('SetBadge', val).then(function () {}, function () {});
    }, wait);
  }
  try {
    var nav = typeof Navigator !== 'undefined' && Navigator.prototype ? Navigator.prototype : navigator;
    def(nav, 'setAppBadge', function (n) {
      var v = -1; // no argument: a flag ("•"), as the specification says
      if (n !== undefined) {
        var c = Math.floor(Number(n));
        if (!isFinite(c) || c < 0) return Promise.reject(new TypeError("Failed to execute 'setAppBadge' on 'Navigator': Value is not a valid unsigned long long."));
        v = c;
      }
      sendBadge(v);
      return Promise.resolve();
    });
    def(nav, 'clearAppBadge', function () { sendBadge(0); return Promise.resolve(); });
  } catch (e) {}
})();
`
