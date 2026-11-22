// ==UserScript==
// @name         X Thread View
// @namespace    https://mypack.dev/userscripts
// @version      0.1.0
// @description  Read an X post as a forum thread: numbered floors, and each reply's sub-thread in a modal.
// @match        https://x.com/*
// @match        https://twitter.com/*
// @run-at       document-start
// @grant        none
// ==/UserScript==

/*
 * How it gets the data: X's web app loads a post with the GraphQL call
 * "TweetDetail". This script watches that call (fetch and XMLHttpRequest),
 * keeps the answers, and remembers the request so it can ask again for
 * another post (a reply's sub-thread) or the next page of replies. Nothing
 * leaves the page: requests go to x.com with the page's own sign-in.
 *
 * If X refuses a replayed request (it signs some requests), the script lets
 * X's own router open the post behind the thread view and reads that answer.
 *
 * Works in MyPack apps and in Violentmonkey / Tampermonkey / ScriptCat.
 */
(function () {
  'use strict';

  // ---- parsing (pure; exported for tests under node) -----------------------

  function unwrap(r) {
    if (!r) return null;
    if (r.__typename === 'TweetWithVisibilityResults' && r.tweet) r = r.tweet;
    if (!r.legacy || !r.rest_id) return null; // tombstone, unavailable
    return r;
  }

  function bestVideo(m) {
    var vs = (m.video_info && m.video_info.variants) || [];
    var best = null;
    vs.forEach(function (v) {
      if (v.content_type === 'video/mp4' && (!best || (v.bitrate || 0) > (best.bitrate || 0))) best = v;
    });
    return best ? best.url : '';
  }

  function normTweet(raw, depth) {
    var r = unwrap(raw);
    if (!r) return null;
    var L = r.legacy;
    var u = (r.core && r.core.user_results && r.core.user_results.result) || {};
    var ul = u.legacy || {}, uc = u.core || {};
    var note = r.note_tweet && r.note_tweet.note_tweet_results && r.note_tweet.note_tweet_results.result;
    var ents = (note && note.entity_set) || L.entities || {};
    var media = ((L.extended_entities && L.extended_entities.media) || (L.entities && L.entities.media) || []).map(function (m) {
      return { type: m.type, url: m.media_url_https, video: m.type === 'photo' ? '' : bestVideo(m), tco: m.url };
    });
    var t = {
      id: r.rest_id,
      name: uc.name || ul.name || '',
      handle: uc.screen_name || ul.screen_name || '',
      avatar: (u.avatar && u.avatar.image_url) || ul.profile_image_url_https || '',
      verified: !!u.is_blue_verified,
      text: note ? note.text : L.full_text || '',
      range: note ? null : L.display_text_range || null,
      urls: (ents.urls || []).map(function (x) { return { tco: x.url, url: x.expanded_url, shown: x.display_url }; }),
      created: L.created_at ? new Date(L.created_at) : null,
      replies: L.reply_count || 0,
      likes: L.favorite_count || 0,
      reposts: L.retweet_count || 0,
      views: r.views && r.views.count ? +r.views.count : 0,
      media: media,
      replyTo: L.in_reply_to_status_id_str || '',
      quoted: null
    };
    if ((depth || 0) < 1 && r.quoted_status_result) t.quoted = normTweet(r.quoted_status_result.result, 1);
    return t;
  }

  function itemTweet(itemContent) {
    if (!itemContent || itemContent.itemType !== 'TimelineTweet') return null;
    return normTweet(itemContent.tweet_results && itemContent.tweet_results.result);
  }

  // parseDetail turns a TweetDetail answer into
  // { focal, ancestors[], floors[{ tweet, chain[] }], cursor }.
  function parseDetail(json, focalId) {
    var out = { focal: null, ancestors: [], floors: [], cursor: '' };
    var conv = json && json.data && (json.data.threaded_conversation_with_injections_v2 || json.data.threaded_conversation_with_injections);
    var ins = (conv && conv.instructions) || [];
    var seenFocal = false;
    function cursorOf(c) {
      if (!c) return '';
      return c.value || (c.itemContent && c.itemContent.value) || '';
    }
    function addModule(items) {
      var tweets = [];
      (items || []).forEach(function (it) {
        var ic = it.item && it.item.itemContent;
        var tw = itemTweet(ic);
        if (tw) tweets.push(tw);
      });
      if (!tweets.length) return;
      out.floors.push({ tweet: tweets[0], chain: tweets.slice(1) });
    }
    ins.forEach(function (i) {
      var entries = i.type === 'TimelineAddEntries' ? i.entries : null;
      if (i.type === 'TimelineAddToModule') {
        // "Show more" inside a reply chain: extends the last floor.
        var last = out.floors[out.floors.length - 1];
        (i.moduleItems || []).forEach(function (it) {
          var tw = itemTweet(it.item && it.item.itemContent);
          if (tw && last) last.chain.push(tw);
        });
        return;
      }
      (entries || []).forEach(function (e) {
        var id = e.entryId || '', c = e.content || {};
        if (id.indexOf('tweet-') === 0) {
          var tw = itemTweet(c.itemContent);
          if (!tw) return;
          if (tw.id === focalId) {
            out.focal = tw; seenFocal = true;
          } else if (!seenFocal) {
            out.ancestors.push(tw);
          } else {
            out.floors.push({ tweet: tw, chain: [] });
          }
        } else if (id.indexOf('conversationthread-') === 0) {
          addModule(c.items);
        } else if (id.indexOf('cursor-bottom-') === 0) {
          out.cursor = cursorOf(c);
        }
      });
    });
    // A focal post X answered with only as an ancestor (e.g. a deleted reply
    // context) falls back to the last ancestor.
    if (!out.focal && out.ancestors.length) out.focal = out.ancestors.pop();
    return out;
  }

  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  // renderText returns safe HTML: the display range only (no leading
  // @mentions of a reply, no trailing media link), t.co links expanded,
  // @mentions, #hashtags and links clickable.
  function renderText(t) {
    var cps = Array.from(t.text || '');
    if (t.range && t.range.length === 2) cps = cps.slice(t.range[0], t.range[1]);
    var text = cps.join('');
    t.media.forEach(function (m) { if (m.tco) text = text.split(m.tco).join(''); });
    var links = {};
    t.urls.forEach(function (u) { if (u.tco) links[u.tco] = u; });
    var html = '';
    var re = /(https:\/\/t\.co\/\w+)|(^|[^\w])@(\w{1,15})|(^|[^\w&])#([\p{L}\p{N}_]+)/gu;
    var last = 0, m;
    while ((m = re.exec(text))) {
      html += esc(text.slice(last, m.index));
      if (m[1]) {
        var u = links[m[1]];
        html += u ? '<a href="' + esc(u.url) + '" target="_blank" rel="noopener">' + esc(u.shown || u.url) + '</a>' : esc(m[1]);
      } else if (m[3]) {
        html += esc(m[2]) + '<a href="/' + esc(m[3]) + '">@' + esc(m[3]) + '</a>';
      } else {
        html += esc(m[4]) + '<a href="/hashtag/' + encodeURIComponent(m[5]) + '">#' + esc(m[5]) + '</a>';
      }
      last = re.lastIndex;
    }
    html += esc(text.slice(last));
    return html.trim().replace(/\n/g, '<br>');
  }

  if (typeof document === 'undefined') {
    if (typeof module === 'object' && module.exports) module.exports = { parseDetail: parseDetail, renderText: renderText, normTweet: normTweet };
    return;
  }

  // ---- capture X's own TweetDetail calls ------------------------------------

  var W = window;
  var origFetch = W.fetch;
  var tpl = null;              // { url, headers } of the last TweetDetail request
  var cache = {};              // focalId -> parsed (first page)
  var waiters = {};            // focalId -> [resolve]
  var DETAIL = /\/i\/api\/graphql\/[^/]+\/TweetDetail\?/;

  function focalOf(url) {
    try {
      var v = JSON.parse(new URL(url, location.href).searchParams.get('variables') || '{}');
      return { id: v.focalTweetId || '', cursor: v.cursor || '' };
    } catch (e) { return { id: '', cursor: '' }; }
  }
  function remember(url, headers) {
    var h = {};
    Object.keys(headers || {}).forEach(function (k) {
      var lk = k.toLowerCase();
      if (lk === 'authorization' || lk.indexOf('x-twitter-') === 0 || lk === 'content-type' || lk === 'x-client-transaction-id') h[lk] = headers[k];
    });
    tpl = { url: new URL(url, location.href).href, headers: h };
  }
  function captured(url, json) {
    var f = focalOf(url);
    if (!f.id || f.cursor) return;
    try {
      cache[f.id] = parseDetail(json, f.id);
      (waiters[f.id] || []).forEach(function (fn) { fn(cache[f.id]); });
      delete waiters[f.id];
    } catch (e) { console.warn('[x-thread-view] parse failed', e); }
  }
  function headersToObj(h) {
    var o = {};
    if (!h) return o;
    if (typeof h.forEach === 'function' && !Array.isArray(h)) h.forEach(function (v, k) { o[k] = v; });
    else if (Array.isArray(h)) h.forEach(function (p) { o[p[0]] = p[1]; });
    else Object.keys(h).forEach(function (k) { o[k] = h[k]; });
    return o;
  }

  if (origFetch) {
    W.fetch = function (input, init) {
      var url = typeof input === 'string' ? input : (input && input.url) || String(input);
      var p = origFetch.apply(this, arguments);
      if (DETAIL.test(url)) {
        remember(url, Object.assign(headersToObj(input && input.headers), headersToObj(init && init.headers)));
        p.then(function (r) {
          if (r.ok) r.clone().json().then(function (j) { captured(url, j); }, function () {});
        }, function () {});
      }
      return p;
    };
  }
  var XO = W.XMLHttpRequest && W.XMLHttpRequest.prototype;
  if (XO) {
    var open = XO.open, setH = XO.setRequestHeader, send = XO.send;
    XO.open = function (method, url) {
      this.__xtv = DETAIL.test(String(url)) ? { url: String(url), headers: {} } : null;
      return open.apply(this, arguments);
    };
    XO.setRequestHeader = function (k, v) {
      if (this.__xtv) this.__xtv.headers[k] = v;
      return setH.apply(this, arguments);
    };
    XO.send = function () {
      var st = this.__xtv;
      if (st) {
        remember(st.url, st.headers);
        this.addEventListener('load', function () {
          if (this.status !== 200) return;
          try {
            var j = this.responseType === 'json' ? this.response : JSON.parse(this.responseText);
            captured(st.url, j);
          } catch (e) {}
        });
      }
      return send.apply(this, arguments);
    };
  }

  function cookie(name) {
    var m = document.cookie.match(new RegExp('(?:^|; )' + name + '=([^;]*)'));
    return m ? decodeURIComponent(m[1]) : '';
  }

  // replay asks TweetDetail again for focalId (and a page cursor).
  function replay(focalId, cursor) {
    if (!tpl) return Promise.reject(new Error('no-template'));
    var u = new URL(tpl.url);
    var v = {};
    try { v = JSON.parse(u.searchParams.get('variables') || '{}'); } catch (e) {}
    v.focalTweetId = focalId;
    delete v.cursor;
    if (cursor) { v.cursor = cursor; v.referrer = 'tweet'; }
    u.searchParams.set('variables', JSON.stringify(v));
    var h = Object.assign({}, tpl.headers);
    delete h['x-client-transaction-id'];
    var csrf = cookie('ct0');
    if (csrf) h['x-csrf-token'] = csrf;
    return origFetch.call(W, u.href, { headers: h, credentials: 'include' }).then(function (r) {
      if (!r.ok) throw new Error('HTTP ' + r.status);
      return r.json();
    }).then(function (j) {
      if (j && j.errors && !j.data) throw new Error(j.errors[0] && j.errors[0].message || 'error');
      return parseDetail(j, focalId);
    });
  }

  // viaRouter lets X itself load the post (behind the thread view), then
  // goes back. X signs its own request, so this works when replay does not.
  function viaRouter(focalId) {
    return new Promise(function (resolve, reject) {
      var back = location.href;
      var timer = setTimeout(function () { done(); reject(new Error('timeout')); }, 15000);
      function done() {
        clearTimeout(timer);
        if (location.href !== back) history.back();
      }
      (waiters[focalId] = waiters[focalId] || []).push(function (d) { done(); resolve(d); });
      history.pushState({}, '', '/i/status/' + focalId);
      W.dispatchEvent(new PopStateEvent('popstate', { state: {} }));
    });
  }

  function load(focalId) {
    if (cache[focalId]) return Promise.resolve(cache[focalId]);
    return replay(focalId).then(function (d) {
      cache[focalId] = d;
      return d;
    }, function (e) {
      console.info('[x-thread-view] replay failed (' + e.message + '); loading through X');
      return viaRouter(focalId);
    });
  }

  // ---- UI -------------------------------------------------------------------

  var CSS = '\
.xtv-root,.xtv-modal{color:#e6e6e6;font:15px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang HK","Microsoft JhengHei",sans-serif}\
.xtv-root{position:fixed;inset:0;z-index:2147483000;background:#141414;display:flex;flex-direction:column}\
.xtv-root a,.xtv-modal a{color:#6cb6ff;text-decoration:none}.xtv-root a:hover,.xtv-modal a:hover{text-decoration:underline}\
.xtv-root a.xtv-name,.xtv-modal a.xtv-name{color:#b0b0b0}.xtv-op a.xtv-name{color:#f5c542}.xtv-root a.xtv-time,.xtv-modal a.xtv-time{color:#6f6f6f}\
.xtv-bar{display:flex;align-items:center;gap:12px;padding:10px 16px;background:#1f1f1f;border-bottom:1px solid #2c2c2c;flex:none}\
.xtv-title{flex:1;min-width:0;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}\
.xtv-btn{background:#2c2c2c;color:#e6e6e6;border:0;border-radius:6px;padding:6px 12px;font:inherit;font-size:13px;cursor:pointer}\
.xtv-btn:hover{background:#3a3a3a}.xtv-btn:disabled{opacity:.5;cursor:default}\
.xtv-scroll{flex:1;overflow-y:auto;overscroll-behavior:contain}\
.xtv-list{max-width:760px;margin:0 auto;padding:8px 0 40px}\
.xtv-floor{padding:14px 18px;border-bottom:1px solid #262626}\
.xtv-head{display:flex;align-items:center;gap:8px;font-size:13px;color:#8a8a8a;margin-bottom:6px;flex-wrap:wrap}\
.xtv-no{color:#8a8a8a;min-width:28px}\
.xtv-av{width:22px;height:22px;border-radius:50%;flex:none}\
.xtv-name{color:#b0b0b0;font-weight:600}.xtv-op .xtv-name{color:#f5c542}\
.xtv-optag{background:#f5c542;color:#141414;border-radius:3px;padding:0 4px;font-size:11px;font-weight:700}\
.xtv-time{color:#6f6f6f}\
.xtv-body{word-wrap:break-word;overflow-wrap:anywhere}\
.xtv-chain{margin-top:10px;padding-top:10px;border-top:1px dashed #2c2c2c}\
.xtv-media{display:grid;grid-template-columns:repeat(auto-fill,minmax(160px,1fr));gap:6px;margin-top:8px}\
.xtv-media img,.xtv-media video{width:100%;max-height:360px;object-fit:cover;border-radius:6px;background:#000;cursor:zoom-in}\
.xtv-quote{margin-top:8px;border:1px solid #2c2c2c;border-radius:8px;padding:8px 10px;font-size:14px;background:#1a1a1a}\
.xtv-foot{display:flex;gap:16px;margin-top:8px;font-size:13px;color:#7a7a7a;align-items:center}\
.xtv-rep{background:none;border:1px solid #333;color:#6cb6ff;border-radius:12px;padding:1px 10px;font:inherit;font-size:13px;cursor:pointer}\
.xtv-rep:hover{background:#1d2a38}\
.xtv-ctx{max-width:760px;margin:8px auto 0;padding:8px 18px;font-size:13px;color:#8a8a8a}\
.xtv-more{display:block;margin:16px auto}\
.xtv-msg{text-align:center;color:#8a8a8a;padding:40px 16px}\
.xtv-modal{position:fixed;inset:0;z-index:2147483001;background:rgba(0,0,0,.6);display:flex;justify-content:center;align-items:flex-end}\
.xtv-sheet{background:#141414;width:100%;max-width:760px;height:88vh;border-radius:12px 12px 0 0;display:flex;flex-direction:column;box-shadow:0 -8px 40px rgba(0,0,0,.5);animation:xtv-up .18s ease-out}\
.xtv-sheet .xtv-bar{border-radius:12px 12px 0 0}\
@keyframes xtv-up{from{transform:translateY(24px);opacity:.4}to{transform:none;opacity:1}}\
.xtv-light{position:fixed;inset:0;z-index:2147483002;background:rgba(0,0,0,.9);display:flex;align-items:center;justify-content:center;cursor:zoom-out}\
.xtv-light img{max-width:96vw;max-height:96vh}\
.xtv-fab{position:fixed;right:20px;bottom:24px;z-index:2147482999;background:#f5c542;color:#141414;border:0;border-radius:22px;padding:10px 16px;font:600 14px -apple-system,BlinkMacSystemFont,sans-serif;box-shadow:0 4px 16px rgba(0,0,0,.35);cursor:pointer}\
@media (max-width:600px){.xtv-sheet{height:94vh}.xtv-floor{padding:12px}}';

  var styled = false;
  function ensureStyle() {
    if (styled) return;
    styled = true;
    var el = document.createElement('style');
    el.textContent = CSS;
    (document.head || document.documentElement).appendChild(el);
  }

  function el(tag, cls, html) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (html != null) e.innerHTML = html;
    return e;
  }

  function ago(d) {
    if (!d) return '';
    var s = (Date.now() - d.getTime()) / 1000;
    if (s < 60) return Math.max(1, Math.floor(s)) + 's';
    if (s < 3600) return Math.floor(s / 60) + 'm';
    if (s < 86400) return Math.floor(s / 3600) + 'h';
    if (s < 86400 * 30) return Math.floor(s / 86400) + 'd';
    return d.toLocaleDateString();
  }
  function num(n) {
    if (n >= 1e6) return (n / 1e6).toFixed(1).replace(/\.0$/, '') + 'M';
    if (n >= 1e3) return (n / 1e3).toFixed(1).replace(/\.0$/, '') + 'K';
    return String(n);
  }

  function mediaHTML(t) {
    if (!t.media.length) return null;
    var box = el('div', 'xtv-media');
    t.media.forEach(function (m) {
      if (m.video) {
        var v = document.createElement('video');
        v.src = m.video; v.poster = m.url; v.controls = true; v.preload = 'none';
        if (m.type === 'animated_gif') { v.loop = true; v.muted = true; }
        box.appendChild(v);
      } else {
        var img = document.createElement('img');
        img.src = m.url + '?name=small'; img.loading = 'lazy'; img.alt = '';
        img.addEventListener('click', function () { lightbox(m.url + '?name=orig'); });
        box.appendChild(img);
      }
    });
    return box;
  }

  function lightbox(src) {
    var lb = el('div', 'xtv-light');
    var img = document.createElement('img');
    img.src = src;
    lb.appendChild(img);
    document.body.appendChild(lb);
    var layer = { close: function () { lb.remove(); } };
    lb.addEventListener('click', function () { if (stack[stack.length - 1] === layer) closeTop(); });
    stack.push(layer);
  }

  function postBody(t) {
    var wrap = el('div');
    wrap.appendChild(el('div', 'xtv-body', renderText(t)));
    var m = mediaHTML(t);
    if (m) wrap.appendChild(m);
    if (t.quoted) {
      var q = el('div', 'xtv-quote');
      q.appendChild(el('div', 'xtv-head', '<span class="xtv-name">' + esc(t.quoted.name) + '</span> @' + esc(t.quoted.handle)));
      q.appendChild(el('div', 'xtv-body', renderText(t.quoted)));
      var qm = mediaHTML(t.quoted);
      if (qm) q.appendChild(qm);
      q.style.cursor = 'pointer';
      q.addEventListener('click', function (e) {
        if (e.target.closest('a,img,video')) return;
        openModal(t.quoted.id, '@' + t.quoted.handle);
      });
      wrap.appendChild(q);
    }
    return wrap;
  }

  // floor renders one numbered post (plus its same-author chain).
  function floor(no, f, opHandle) {
    var t = f.tweet;
    var isOP = t.handle && t.handle === opHandle;
    var box = el('div', 'xtv-floor' + (isOP ? ' xtv-op' : ''));
    var head = el('div', 'xtv-head');
    head.appendChild(el('span', 'xtv-no', '#' + no));
    if (t.avatar) { var av = el('img', 'xtv-av'); av.src = t.avatar; av.alt = ''; head.appendChild(av); }
    head.appendChild(el('a', 'xtv-name', esc(t.name || t.handle))).href = '/' + t.handle;
    if (isOP) head.appendChild(el('span', 'xtv-optag', 'OP'));
    var time = el('a', 'xtv-time', esc(ago(t.created)));
    time.href = '/' + t.handle + '/status/' + t.id;
    time.title = t.created ? t.created.toLocaleString() : '';
    head.appendChild(time);
    box.appendChild(head);
    box.appendChild(postBody(t));
    f.chain.forEach(function (c) {
      var part = el('div', 'xtv-chain');
      part.appendChild(postBody(c));
      box.appendChild(part);
    });
    var foot = el('div', 'xtv-foot');
    foot.appendChild(el('span', '', '♥ ' + num(t.likes)));
    if (t.reposts) foot.appendChild(el('span', '', '⟲ ' + num(t.reposts)));
    if (t.views) foot.appendChild(el('span', '', '👁 ' + num(t.views)));
    if (t.replies > 0) {
      var b = el('button', 'xtv-rep', '💬 ' + num(t.replies) + ' ' + (t.replies === 1 ? 'reply' : 'replies'));
      b.addEventListener('click', function () { openModal(t.id, '#' + no + ' ' + (t.name || t.handle)); });
      foot.appendChild(b);
    }
    box.appendChild(foot);
    return box;
  }

  // thread fills container with the conversation around focalId.
  var rootOP = ''; // the author of the thread the view opened on
  function thread(container, focalId, setTitle, isRoot) {
    var list = el('div', 'xtv-list');
    container.innerHTML = '';
    container.appendChild(el('div', 'xtv-msg', 'Loading…'));
    load(focalId).then(function (d) {
      container.innerHTML = '';
      if (!d.focal) { container.appendChild(el('div', 'xtv-msg', 'This post is not available.')); return; }
      if (isRoot) rootOP = d.focal.handle;
      var op = rootOP || d.focal.handle;
      setTitle(d.focal);
      if (d.ancestors.length) {
        var ctx = el('div', 'xtv-ctx');
        var a = el('a', '', '↑ Replying in a conversation of ' + (d.ancestors.length + 1) + ' posts. Read it from the start');
        a.href = '#';
        a.addEventListener('click', function (e) { e.preventDefault(); openModal(d.ancestors[0].id, 'Conversation'); });
        ctx.appendChild(a);
        container.appendChild(ctx);
      }
      container.appendChild(list);
      var no = 1;
      list.appendChild(floor(no++, { tweet: d.focal, chain: [] }, op));
      function addFloors(fs) { fs.forEach(function (f) { list.appendChild(floor(no++, f, op)); }); }
      addFloors(d.floors);
      if (!d.floors.length && !d.cursor) list.appendChild(el('div', 'xtv-msg', 'No replies yet.'));
      var cursor = d.cursor;
      var more = el('button', 'xtv-btn xtv-more', 'Load more replies');
      function sync() { if (cursor) container.appendChild(more); else more.remove(); }
      more.addEventListener('click', function () {
        more.disabled = true; more.textContent = 'Loading…';
        replay(focalId, cursor).then(function (p) {
          addFloors(p.floors);
          cursor = p.cursor && p.cursor !== cursor ? p.cursor : '';
          more.disabled = false; more.textContent = 'Load more replies';
          sync();
        }, function () {
          more.disabled = false; more.textContent = 'Could not load more. Open on X';
          more.onclick = function () { location.href = '/i/status/' + focalId; };
        });
      });
      sync();
    }, function (e) {
      container.innerHTML = '';
      var msg = e && e.message === 'no-template'
        ? 'Thread view needs X to load one post first. Reload this page, then try again.'
        : 'Could not load this thread (' + esc((e && e.message) || e) + ').';
      container.appendChild(el('div', 'xtv-msg', msg));
    });
  }

  var stack = [];       // open layers, top last
  var lockedScroll = '';
  function lock() {
    if (stack.length === 0) {
      lockedScroll = document.documentElement.style.overflow;
      document.documentElement.style.overflow = 'hidden';
    }
  }
  function unlockIfEmpty() {
    if (stack.length === 0) document.documentElement.style.overflow = lockedScroll;
  }
  function closeTop() {
    var l = stack.pop();
    if (l) l.close();
    unlockIfEmpty();
  }
  function closeAll() { while (stack.length) closeTop(); }

  function bar(titleText, onClose, extra) {
    var b = el('div', 'xtv-bar');
    var title = el('div', 'xtv-title', esc(titleText));
    b.appendChild(title);
    if (extra) b.appendChild(extra);
    var x = el('button', 'xtv-btn', '✕');
    x.title = 'Close (Esc)';
    x.addEventListener('click', onClose);
    b.appendChild(x);
    return { el: b, title: title };
  }

  function openView(focalId) {
    ensureStyle();
    lock();
    var root = el('div', 'xtv-root');
    var orig = el('a', 'xtv-btn', 'Open on X');
    orig.href = '/i/status/' + focalId;
    orig.addEventListener('click', function () { closeAll(); });
    var b = bar('Thread', closeTop, orig);
    var scroll = el('div', 'xtv-scroll');
    root.appendChild(b.el);
    root.appendChild(scroll);
    document.body.appendChild(root);
    stack.push({ close: function () { root.remove(); }, focal: focalId });
    thread(scroll, focalId, function (t) {
      b.title.textContent = (t.text ? Array.from(t.text).slice(0, 80).join('').replace(/\s+/g, ' ') : 'Thread') + '  ·  ' + t.name;
    }, true);
  }

  function openModal(focalId, label) {
    ensureStyle();
    lock();
    var m = el('div', 'xtv-modal');
    var sheet = el('div', 'xtv-sheet');
    var b = bar('Replies · ' + label, closeTop);
    var scroll = el('div', 'xtv-scroll');
    sheet.appendChild(b.el);
    sheet.appendChild(scroll);
    m.appendChild(sheet);
    m.addEventListener('click', function (e) { if (e.target === m) closeTop(); });
    document.body.appendChild(m);
    stack.push({ close: function () { m.remove(); } });
    thread(scroll, focalId, function () {});
  }

  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && stack.length) { e.stopPropagation(); e.preventDefault(); closeTop(); }
  }, true);

  // ---- entry point: a button on post pages ----------------------------------

  var STATUS = /^\/(?:[^/]+|i\/web|i)\/status\/(\d+)/;
  var fab = null;
  function statusId() {
    var m = location.pathname.match(STATUS);
    return m ? m[1] : '';
  }
  function tick() {
    if (!document.body) return;
    var id = statusId();
    if (id && !fab) {
      ensureStyle();
      fab = el('button', 'xtv-fab', '☰ Thread view');
      fab.title = 'Read this post as a thread (Alt+T)';
      fab.addEventListener('click', function () { var i = statusId(); if (i) openView(i); });
      document.body.appendChild(fab);
    } else if (!id && fab) {
      fab.remove(); fab = null;
    }
    if (fab) fab.style.display = stack.length ? 'none' : '';
  }
  setInterval(tick, 600);
  document.addEventListener('keydown', function (e) {
    // Alt+T: plain T is X's own repost shortcut.
    if (e.code !== 'KeyT' || !e.altKey || e.metaKey || e.ctrlKey || stack.length) return;
    var t = e.target;
    if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
    var id = statusId();
    if (id) { e.preventDefault(); openView(id); }
  });
})();
