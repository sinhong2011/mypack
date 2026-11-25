package inject

// GlanceScript returns the script behind Glance (window.glance): a click on
// a link while trigger ("shift", "alt", "ctrl" or "meta") is held, and no
// other modifier, asks the runtime to preview the link over the page
// (window.mygo.call("mypack.Glance", ...)) instead of following it. Every
// click on a link also leaves its box in window.__mypackGlanceLast, where
// the runtime finds where a link that leaves the app was (glance.externalLinks).
// "" when trigger is empty.
func GlanceScript(trigger string) string {
	switch trigger {
	case "shift", "alt", "ctrl", "meta":
	default:
		return ""
	}
	return `;(function () {
  'use strict';
  var mg = window.mygo;
  if (!mg || typeof mg.call !== 'function' || window.__mypackGlance) return;
  try { Object.defineProperty(window, '__mypackGlance', { value: true }); } catch (e) {}
  var MOD = ` + jsString(trigger) + `;
  function held(e) {
    var m = { shift: e.shiftKey, alt: e.altKey, ctrl: e.ctrlKey, meta: e.metaKey };
    for (var k in m) { if (m[k] !== (k === MOD)) return false; }
    return true;
  }
  function linkOf(e) {
    var path = e.composedPath ? e.composedPath() : [];
    for (var i = 0; i < path.length; i++) {
      var el = path[i];
      if (el && el.tagName && (el.tagName === 'A' || el.tagName === 'AREA') && el.href) return el;
    }
    var t = e.target;
    return t && t.closest ? t.closest('a[href],area[href]') : null;
  }
  function boxOf(a, e) {
    var r = null, rs = a.getClientRects ? a.getClientRects() : [];
    // A link that wraps lines: the line that was clicked.
    for (var i = 0; i < rs.length; i++) {
      var q = rs[i];
      if (e.clientX >= q.left && e.clientX <= q.right && e.clientY >= q.top && e.clientY <= q.bottom) { r = q; break; }
    }
    if (!r) r = a.getBoundingClientRect();
    if (!r || r.width < 1 || r.height < 1) return { x: e.clientX, y: e.clientY, w: 0, h: 0 };
    return { x: r.left, y: r.top, w: r.width, h: r.height };
  }
  document.addEventListener('click', function (e) {
    if (e.button !== 0) return;
    var a = linkOf(e);
    if (!a) return;
    var href = String(a.href);
    if (!/^https?:/i.test(href)) return;
    var b = boxOf(a, e);
    window.__mypackGlanceLast = { x: b.x, y: b.y, w: b.w, h: b.h, t: Date.now() };
    if (!held(e)) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    mg.call('mypack.Glance', { url: href, x: b.x, y: b.y, w: b.w, h: b.h });
  }, true);
})();`
}
