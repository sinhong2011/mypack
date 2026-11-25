package inject

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// UserScript is one installed userscript as the preload runs it.
type UserScript struct {
	// Token identifies the script to the Go bridge for this launch. It lives
	// only in the preload's closure, so page scripts cannot use the script's
	// privileged calls through window.mygo by themselves.
	Token       string
	Name        string
	Namespace   string
	Version     string
	Description string
	Include     []URLRule
	Exclude     []URLRule
	RunAt       string // document-start | document-end | document-idle
	NoFrames    bool
	Code        string   // the script
	Requires    []string // @require code, run before the script in the same scope
	Resources   []Resource
}

// URLRule is a regular expression source (and the i flag) the page tests
// location.href against.
type URLRule struct {
	Source     string `json:"s"`
	IgnoreCase bool   `json:"i,omitempty"`
}

// Resource is a @resource, inlined as a data: URL.
type Resource struct {
	Name string
	MIME string
	Data []byte
}

type usDesc struct {
	Token string            `json:"token"`
	Inc   []URLRule         `json:"inc"`
	Exc   []URLRule         `json:"exc"`
	RunAt string            `json:"runAt"`
	NoFr  bool              `json:"noframes,omitempty"`
	Info  map[string]string `json:"info"`
	Res   map[string]string `json:"res"`
}

// UserScriptsScript returns the dispatcher that runs the scripts matching the
// page ("" when there are none). Every script runs in its own function with
// the GM_* API as parameters, inside its own try block.
func UserScriptsScript(scripts []UserScript) string {
	if len(scripts) == 0 {
		return ""
	}
	descs := make([]usDesc, len(scripts))
	var bodies strings.Builder
	for i, s := range scripts {
		res := map[string]string{}
		for _, r := range s.Resources {
			mt := r.MIME
			if mt == "" {
				mt = "application/octet-stream"
			}
			res[r.Name] = "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(r.Data)
		}
		inc, exc := s.Include, s.Exclude
		if inc == nil {
			inc = []URLRule{}
		}
		if exc == nil {
			exc = []URLRule{}
		}
		descs[i] = usDesc{Token: s.Token, Inc: inc, Exc: exc, RunAt: s.RunAt, NoFr: s.NoFrames, Res: res,
			Info: map[string]string{"name": s.Name, "namespace": s.Namespace, "version": s.Version, "description": s.Description}}
		if i > 0 {
			bodies.WriteString(",\n")
		}
		// Sloppy mode on purpose: many scripts rely on it. The trailing
		// newline ends a final line comment.
		bodies.WriteString("function (GM_info, GM, unsafeWindow, GM_getValue, GM_setValue, GM_deleteValue, GM_listValues, GM_xmlhttpRequest, GM_addStyle, GM_setClipboard, GM_notification, GM_openInTab, GM_registerMenuCommand, GM_unregisterMenuCommand, GM_getResourceText, GM_getResourceURL, GM_log) {\n")
		for _, r := range s.Requires {
			bodies.WriteString(r)
			bodies.WriteString("\n;\n")
		}
		bodies.WriteString(s.Code)
		bodies.WriteString("\n}")
	}
	b, _ := json.Marshal(descs)
	cfg := strings.NewReplacer("<", `<`, ">", `>`, " ", ` `, " ", ` `).Replace(string(b))
	js := strings.Replace(userScriptsJS, "__MYPACK_US__", cfg, 1)
	return strings.Replace(js, "__MYPACK_US_BODIES__", bodies.String(), 1)
}

// The dispatcher. It keeps its own reference to window.mygo.call, made before
// any page script runs.
const userScriptsJS = `;(function () {
  var mg = window.mygo;
  if (!mg || typeof mg.call !== 'function') return; // frame without the native bridge
  if (window.__mypackUS) return;
  try { Object.defineProperty(window, '__mypackUS', { value: true }); } catch (e) {}
  var call = mg.call.bind(mg);
  var DESCS = __MYPACK_US__;
  var BODIES = [
__MYPACK_US_BODIES__
  ];
  var href = String(location.href);
  var top = true; try { top = window.top === window; } catch (e) { top = false; }

  function test(rules) {
    rules = rules || [];
    for (var i = 0; i < rules.length; i++) {
      try { if (new RegExp(rules[i].s, rules[i].i ? 'i' : '').test(href)) return true; } catch (e) {}
    }
    return false;
  }
  function clone(v) { return v === undefined ? undefined : JSON.parse(JSON.stringify(v)); }
  function warn(d, what, e) { console.warn('[mypack] userscript "' + d.info.name + '": ' + what, e); }
  function onReady(fn) {
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', fn, { once: true });
    else fn();
  }
  function b64ToBytes(s) {
    var bin = atob(s), out = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
  }
  function bytesToB64(buf) {
    var u = new Uint8Array(buf), s = '';
    for (var i = 0; i < u.length; i += 0x8000) s += String.fromCharCode.apply(null, u.subarray(i, i + 0x8000));
    return btoa(s);
  }

  function makeAPI(d, vals) {
    var tok = d.token;
    var info = { script: { name: d.info.name, namespace: d.info.namespace, version: d.info.version,
      description: d.info.description }, scriptHandler: 'MyPack', version: '1', scriptMetaStr: '' };

    function getValue(k, def) {
      return Object.prototype.hasOwnProperty.call(vals, k) ? clone(vals[k]) : def;
    }
    function setValue(k, v) {
      k = String(k);
      if (v === undefined) return deleteValue(k);
      var text = JSON.stringify(v);
      vals[k] = JSON.parse(text);
      return call('mypack.GMSetValue', tok, k, text).catch(function (e) { warn(d, 'GM_setValue failed', e); });
    }
    function deleteValue(k) {
      k = String(k);
      delete vals[k];
      return call('mypack.GMDeleteValue', tok, k).catch(function (e) { warn(d, 'GM_deleteValue failed', e); });
    }
    function listValues() { return Object.keys(vals); }

    function addStyle(css) {
      var el = document.createElement('style');
      el.textContent = String(css);
      var put = function () { (document.head || document.documentElement).appendChild(el); };
      if (document.head || document.documentElement) put(); else onReady(put);
      return el;
    }

    function xhr(o) {
      o = o || {};
      var aborted = false;
      var req = { method: String(o.method || 'GET').toUpperCase(), url: '', headers: {},
        timeout: +o.timeout || 0, responseType: String(o.responseType || ''), anonymous: true };
      try { req.url = new URL(String(o.url), location.href).href; } catch (e) { req.url = String(o.url); }
      if (o.headers) for (var h in o.headers) if (Object.prototype.hasOwnProperty.call(o.headers, h)) req.headers[h] = String(o.headers[h]);
      if (o.overrideMimeType) req.overrideMimeType = String(o.overrideMimeType);
      var data = o.data, prep = Promise.resolve();
      if (data instanceof ArrayBuffer || ArrayBuffer.isView(data)) {
        req.dataBase64 = bytesToB64(data instanceof ArrayBuffer ? data : data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength));
      } else if (typeof Blob !== 'undefined' && data instanceof Blob) {
        prep = data.arrayBuffer().then(function (ab) { req.dataBase64 = bytesToB64(ab); });
      } else if (typeof URLSearchParams !== 'undefined' && data instanceof URLSearchParams) {
        req.data = data.toString();
        if (!req.headers['Content-Type'] && !req.headers['content-type']) req.headers['Content-Type'] = 'application/x-www-form-urlencoded;charset=UTF-8';
      } else if (data !== undefined && data !== null) {
        req.data = typeof data === 'string' ? data : (typeof data === 'object' ? JSON.stringify(data) : String(data));
      }
      function fire(name, arg) { try { if (typeof o[name] === 'function') o[name].call(o, arg); } catch (e) { warn(d, name + ' threw', e); } }
      var p = prep.then(function () { return call('mypack.GMXhr', tok, req); }).then(function (r) {
        if (aborted) return;
        var resp = { readyState: 4, status: r.status, statusText: r.statusText, responseHeaders: r.headers,
          finalUrl: r.finalUrl, responseText: '', response: null, context: o.context };
        var type = req.responseType.toLowerCase();
        if (r.bodyBase64 !== undefined && r.bodyBase64 !== '') {
          var bytes = b64ToBytes(r.bodyBase64);
          if (type === 'blob') resp.response = new Blob([bytes], { type: r.mime || '' });
          else resp.response = bytes.buffer;
        } else {
          resp.responseText = r.text || '';
          if (type === 'json') { try { resp.response = JSON.parse(resp.responseText); } catch (e) { resp.response = null; } }
          else if (type === 'document') { try { resp.response = new DOMParser().parseFromString(resp.responseText, r.mime && r.mime.indexOf('xml') >= 0 ? r.mime : 'text/html'); } catch (e) {} }
          else resp.response = resp.responseText;
        }
        fire('onreadystatechange', resp);
        fire('onload', resp);
        fire('onloadend', resp);
        return resp;
      }, function (e) {
        if (aborted) return;
        var msg = (e && e.message) || String(e);
        var resp = { readyState: 4, status: 0, statusText: '', responseHeaders: '', responseText: '', error: msg, finalUrl: req.url, context: o.context };
        fire(/timeout/i.test(msg) ? 'ontimeout' : 'onerror', resp);
        fire('onloadend', resp);
        throw resp;
      });
      return { abort: function () { if (!aborted) { aborted = true; fire('onabort', {}); } }, _promise: p };
    }
    function gm4xhr(o) {
      return new Promise(function (resolve, reject) {
        var c = Object.assign({}, o, {
          onload: function (r) { if (o && o.onload) o.onload(r); resolve(r); },
          onerror: function (r) { if (o && o.onerror) o.onerror(r); reject(r); },
          ontimeout: function (r) { if (o && o.ontimeout) o.ontimeout(r); reject(r); }
        });
        xhr(c)._promise.catch(function () {});
      });
    }

    function setClipboard(text) {
      return call('mypack.GMSetClipboard', tok, String(text)).catch(function (e) { warn(d, 'GM_setClipboard failed', e); });
    }
    function notification(details, title, image, onclick) {
      if (typeof details !== 'object' || details === null) details = { text: String(details), title: title, image: image, onclick: onclick };
      try {
        var n = new Notification(details.title || d.info.name, { body: details.text || '', icon: details.image || undefined, tag: details.tag });
        if (typeof details.onclick === 'function') n.onclick = details.onclick;
        if (typeof details.ondone === 'function') n.onclose = details.ondone;
      } catch (e) { warn(d, 'GM_notification failed', e); }
    }
    function openInTab(url) {
      var w = window.open(new URL(String(url), location.href).href, '_blank');
      return { close: function () { try { w && w.close(); } catch (e) {} }, closed: false };
    }
    var menuSeq = 0;
    function registerMenuCommand() { return ++menuSeq; } // no menu yet in v1
    function resourceURL(name) { return d.res[name] || null; }
    function resourceText(name) {
      var u = d.res[name];
      if (!u) return null;
      var bytes = b64ToBytes(u.slice(u.indexOf(',') + 1));
      try { return new TextDecoder().decode(bytes); } catch (e) { return String.fromCharCode.apply(null, bytes); }
    }

    var GM = {
      info: info,
      getValue: function (k, def) { return Promise.resolve(getValue(k, def)); },
      setValue: function (k, v) { return Promise.resolve(setValue(k, v)); },
      deleteValue: function (k) { return Promise.resolve(deleteValue(k)); },
      listValues: function () { return Promise.resolve(listValues()); },
      xmlHttpRequest: gm4xhr,
      addStyle: function (css) { return Promise.resolve(addStyle(css)); },
      setClipboard: function (t) { return Promise.resolve(setClipboard(t)); },
      notification: function (a, b, c, e) { return Promise.resolve(notification(a, b, c, e)); },
      openInTab: function (u) { return Promise.resolve(openInTab(u)); },
      registerMenuCommand: function () { return Promise.resolve(registerMenuCommand()); },
      getResourceUrl: function (n) { return Promise.resolve(resourceURL(n)); },
      getResourceText: function (n) { return Promise.resolve(resourceText(n)); }
    };
    return [info, GM, window, getValue, setValue, deleteValue, listValues, xhr, addStyle, setClipboard,
      notification, openInTab, registerMenuCommand, function () {}, resourceText, resourceURL,
      function () { console.log.apply(console, arguments); }];
  }

  function run(d, body, vals) {
    var go = function () {
      try { body.apply(window, makeAPI(d, vals)); } catch (e) { warn(d, 'failed', e); }
    };
    if (d.runAt === 'document-start') go();
    else if (d.runAt === 'document-end') onReady(go);
    else onReady(function () { setTimeout(go, 0); });
  }

  DESCS.forEach(function (d, i) {
    if (!top && d.noframes) return;
    if (!test(d.inc) || test(d.exc)) return;
    call('mypack.GMValues', d.token).then(function (vals) {
      run(d, BODIES[i], vals && typeof vals === 'object' ? vals : {});
    }, function (e) {
      warn(d, 'could not read its values', e);
      run(d, BODIES[i], {});
    });
  });
})();
`
