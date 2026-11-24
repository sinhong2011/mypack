// ==UserScript==
// @name         X Thread View
// @namespace    https://mypack.dev/userscripts
// @version      0.8.0
// @description  Open posts and their nested replies full screen over the page, in X's look and icons, without leaving where you are.
// @match        https://x.com/*
// @match        https://twitter.com/*
// @run-at       document-start
// @grant        none
// ==/UserScript==

/*
 * On a post's page, X opens a reply by leaving the page: the route changes
 * and you lose your place in the conversation. Here a reply opens in a
 * panel over the page instead, in X's own look, the way Hong Kong forums
 * open a post's replies:
 *
 *  - A post's 💬 N count shows its replies: they open in a panel, and the
 *    page, its route and scroll stay. (With no replies it does what X
 *    does.) Writing a reply moves to a ↩ in the post's header.
 *  - On a post's page a click on a reply opens it the same way.
 *  - In a panel, each post's 💬 N opens its replies in a new panel on top;
 *    the ones below stay, stepped back, so you see how deep you are, and
 *    closing the top one shows the one below as you left it.
 *  - "Go to reply" closes the panels and shows that reply where it is on
 *    the page, highlighted.
 *  - A "Replies" bar and an end mark say where the replies start and end.
 *  - Photos open in a viewer over the panels (arrows, count), not on X's
 *    photo page. Esc closes the top layer; ⌘/Ctrl-click opens normally.
 *
 * Data: X loads a post with the GraphQL call "TweetDetail". The script
 * keeps the page's own request (its headers, signature included) and asks
 * again with another post's id, with the page's sign-in. It never changes
 * the page's route. Nothing leaves X (x.com, api.x.com).
 *
 * Works in MyPack apps and in Violentmonkey / Tampermonkey / ScriptCat.
 */
(function () {
  'use strict';

  // ---- parsing (pure; exported for tests under node) -----------------------

  function unwrap(r) {
    if (!r) return null;
    if (r.__typename === 'TweetWithVisibilityResults' && r.tweet) r = r.tweet;
    if (!r.rest_id || !(r.legacy || r.details)) return null; // tombstone, unavailable
    return r;
  }

  function bestVideo(m) {
    var vs = (m.video_info && m.video_info.variants) || [];
    var best = null;
    vs.forEach(function (v) {
      if (v.content_type === 'video/mp4' && (!best || (v.bitrate || 0) > (best.bitrate || 0))) best = v;
    });
    if (best) return best.url;
    // X's 2026 shape lists playlists (HLS, which WebKit plays, or mp4).
    var pl = m.ext_playlists || [], hls = '';
    for (var i = 0; i < pl.length; i++) {
      var u = (pl[i] && (pl[i].url || pl[i].playlist_url)) || '';
      if (/\.mp4(\?|$)/.test(u)) return u;
      if (!hls && u) hls = u;
    }
    return hls;
  }

  function normTweet(raw, depth) {
    var r = unwrap(raw);
    if (!r) return null;
    // X has two shapes: "legacy" holds everything (until 2026), or
    // "details" (text, time), "counts" and *_entities beside it (2026).
    var L = r.legacy || {}, D = r.details || {}, C = r.counts || {};
    var u = (r.core && r.core.user_results && r.core.user_results.result) || {};
    var ul = u.legacy || {}, uc = u.core || {};
    var note = r.note_tweet && r.note_tweet.note_tweet_results && r.note_tweet.note_tweet_results.result;
    var ents = (note && note.entity_set) || L.entities || {};
    var urls = (note && note.entity_set && note.entity_set.urls) || r.url_entities || ents.urls || [];
    var rawMedia = (r.media_entities2 && r.media_entities2.length && r.media_entities2) ||
      (L.extended_entities && L.extended_entities.media) || (L.entities && L.entities.media) || [];
    var media = rawMedia.filter(function (m) { return m && m.media_url_https; }).map(function (m) {
      return { type: m.type, url: m.media_url_https, video: m.type === 'photo' ? '' : bestVideo(m), tco: m.url || '',
        w: (m.original_info && m.original_info.width) || 0, h: (m.original_info && m.original_info.height) || 0 };
    });
    function count(k) { return (C[k] != null ? C[k] : L[k]) || 0; }
    var created = D.created_at_ms ? new Date(+D.created_at_ms) : L.created_at ? new Date(L.created_at) : null;
    var rtu = r.reply_to_user_results && r.reply_to_user_results.result;
    var quoted = r.quoted_status_result || r.quoted_tweet_results || r.quoted_post_results;
    var t = {
      id: r.rest_id,
      name: uc.name || ul.name || '',
      handle: uc.screen_name || ul.screen_name || '',
      avatar: (u.avatar && u.avatar.image_url) || ul.profile_image_url_https || '',
      verified: !!(u.is_blue_verified || (u.verification && u.verification.is_blue_verified)),
      text: note ? note.text : D.full_text || L.full_text || '',
      range: note ? null : D.display_text_range || L.display_text_range || null,
      urls: urls.map(function (x) { return { tco: x.url, url: x.expanded_url, shown: x.display_url }; }),
      created: created,
      replies: count('reply_count'),
      likes: count('favorite_count'),
      reposts: count('retweet_count') + count('quote_count'),
      views: r.views && r.views.count ? +r.views.count : 0,
      media: media,
      replyTo: L.in_reply_to_status_id_str || (r.reply_to_results && r.reply_to_results.rest_id) || '',
      replyToHandle: L.in_reply_to_screen_name || (rtu && rtu.core && rtu.core.screen_name) || (rtu && rtu.legacy && rtu.legacy.screen_name) || '',
      quoted: null
    };
    if ((depth || 0) < 1 && quoted && quoted.result) t.quoted = normTweet(quoted.result, 1);
    return t;
  }

  function itemTweet(itemContent) {
    if (!itemContent || (itemContent.itemType || itemContent.__typename) !== 'TimelineTweet') return null;
    return normTweet(itemContent.tweet_results && itemContent.tweet_results.result);
  }

  // parseDetail reads a TweetDetail answer: { focal, ancestors[],
  // threads[][], cursor }. Each thread is a reply and the posts that
  // continue it, as X groups them (drawn joined by a line).
  function parseDetail(json, focalId) {
    var out = { focal: null, ancestors: [], threads: [], cursor: '' };
    var conv = json && json.data && (json.data.threaded_conversation_with_injections_v2 || json.data.threaded_conversation_with_injections);
    var ins = (conv && conv.instructions) || [];
    var seenFocal = false;
    // An item's content is "itemContent" (until 2026) or "content".
    function inner(x) { return x && (x.itemContent || x.content); }
    function cursorOf(c) { return (c && (c.value || (inner(c) && inner(c).value))) || ''; }
    ins.forEach(function (i) {
      var type = i.type || i.__typename;
      if (type === 'TimelineAddToModule') {
        var last = out.threads[out.threads.length - 1];
        (i.moduleItems || i.module_items || []).forEach(function (it) {
          var tw = itemTweet(inner(it.item));
          if (tw && last) last.push(tw);
        });
        return;
      }
      if (type !== 'TimelineAddEntries') return;
      (i.entries || []).forEach(function (e) {
        var id = e.entryId || e.entry_id || '', c = e.content || {};
        if (id.indexOf('tweet-') === 0) {
          var tw = itemTweet(inner(c));
          if (!tw) return;
          if (tw.id === focalId) { out.focal = tw; seenFocal = true; }
          else if (!seenFocal) out.ancestors.push(tw);
          else out.threads.push([tw]);
        } else if (id.indexOf('conversationthread-') === 0) {
          var th = [];
          (c.items || []).forEach(function (it) {
            var tw = itemTweet(inner(it.item));
            if (tw) th.push(tw);
          });
          if (th.length) out.threads.push(th);
        } else if (id.indexOf('cursor-bottom-') === 0) {
          out.cursor = cursorOf(c);
        }
      });
    });
    if (!out.focal && out.ancestors.length) out.focal = out.ancestors.pop();
    return out;
  }

  function unent(s) {
    return String(s).replace(/&(amp|lt|gt|quot|#39);/g, function (m, n) {
      return { amp: '&', lt: '<', gt: '>', quot: '"', '#39': "'" }[n];
    });
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
    // X sends &, < and > as entities: shown as themselves, escaped once below.
    var text = unent(cps.join(''));
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

  // statusIdOf reads the post id of an X status address ("" if none).
  function statusIdOf(href) {
    var m = /\/status\/(\d+)/.exec(String(href || ''));
    return m ? m[1] : '';
  }

  if (typeof document === 'undefined') {
    if (typeof module === 'object' && module.exports) {
      module.exports = { parseDetail: parseDetail, renderText: renderText, normTweet: normTweet, statusIdOf: statusIdOf };
    }
    return;
  }

  // ---- words (X's own wording, in the page's language) -----------------------

  var zh = /^zh/i.test(document.documentElement.lang || navigator.language || '');
  var W_ = zh ? {
    post: '貼文', close: '關閉', open: '開啟貼文', showReplies: function (n) { return '顯示 ' + n + ' 則回覆'; },
    replies: function (n) { return n + ' 則回覆'; }, more: '顯示更多回覆', none: '還沒有回覆', end: '已顯示所有回覆',
    replyingTo: '回覆給', failed: '無法載入這篇貼文。', openInstead: '改為開啟貼文', views: '次查看',
    goTo: '前往回覆', repliesHead: '回覆', photo: '相片', reply: '回覆', showRepliesOf: function (n) { return n + ' 則回覆，按此查看'; }
  } : {
    post: 'Post', close: 'Close', open: 'Open post', showReplies: function (n) { return 'Show ' + n + (n === 1 ? ' reply' : ' replies'); },
    replies: function (n) { return n + (n === 1 ? ' reply' : ' replies'); }, more: 'Show more replies', none: 'No replies yet', end: 'That’s all the replies',
    replyingTo: 'Replying to', failed: 'This post could not be loaded.', openInstead: 'Open the post instead', views: 'Views',
    goTo: 'Go to reply', repliesHead: 'Replies', photo: 'Photo', reply: 'Reply', showRepliesOf: function (n) { return n + (n === 1 ? ' reply' : ' replies') + ': show'; }
  };

  // ---- capture X's own TweetDetail calls ------------------------------------

  var Win = window;
  var origFetch = Win.fetch;
  var tpl = null;   // { url, headers } of the page's last TweetDetail request
  var cache = {};   // focalId -> parsed first page
  var waiters = {}; // key -> [fn]
  var DETAIL = /\/graphql\/[^/?]+\/TweetDetail(?:\?|$)/;
  var TPL_KEY = 'xtv-tpl';
  try { tpl = JSON.parse(localStorage.getItem(TPL_KEY) || 'null'); } catch (e) {}

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
      // The transaction id signs the method and path, which are the same
      // for every TweetDetail call: the page's own works for ours.
      // The CSRF token is read from its cookie for each call instead.
      if (lk === 'x-csrf-token') return;
      if (lk === 'authorization' || lk.indexOf('x-') === 0 || lk === 'content-type') h[lk] = headers[k];
    });
    tpl = { url: new URL(url, location.href).href, headers: h };
    // Kept for the next launch, so posts open over the page from the start.
    try { localStorage.setItem(TPL_KEY, JSON.stringify(tpl)); } catch (e) {}
    flush('*tpl*', true);
  }
  function flush(key, val) {
    (waiters[key] || []).forEach(function (fn) { fn(val); });
    delete waiters[key];
  }
  function captured(url, json) {
    var f = focalOf(url);
    if (!f.id || f.cursor) return;
    try {
      cache[f.id] = parseDetail(json, f.id);
      flush(f.id, cache[f.id]);
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
    Win.fetch = function (input, init) {
      var url = typeof input === 'string' ? input : (input && input.url) || String(input);
      var p = origFetch.apply(this, arguments);
      if (DETAIL.test(url)) {
        remember(url, Object.assign(headersToObj(input && input.headers), headersToObj(init && init.headers)));
        p.then(function (r) { if (r.ok) r.clone().json().then(function (j) { captured(url, j); }, function () {}); }, function () {});
      }
      return p;
    };
  }
  var XO = Win.XMLHttpRequest && Win.XMLHttpRequest.prototype;
  if (XO) {
    var xOpen = XO.open, xSetH = XO.setRequestHeader, xSend = XO.send;
    XO.open = function (method, url) {
      this.__xtv = DETAIL.test(String(url)) ? { url: String(url), headers: {} } : null;
      return xOpen.apply(this, arguments);
    };
    XO.setRequestHeader = function (k, v) {
      if (this.__xtv) this.__xtv.headers[k] = v;
      return xSetH.apply(this, arguments);
    };
    XO.send = function () {
      var st = this.__xtv;
      if (st) {
        remember(st.url, st.headers);
        this.addEventListener('load', function () {
          if (this.status !== 200) return;
          try { captured(st.url, this.responseType === 'json' ? this.response : JSON.parse(this.responseText)); } catch (e) {}
        });
      }
      return xSend.apply(this, arguments);
    };
  }

  function cookie(name) {
    var m = document.cookie.match(new RegExp('(?:^|; )' + name + '=([^;]*)'));
    return m ? decodeURIComponent(m[1]) : '';
  }
  function wait(key, ms) {
    return new Promise(function (resolve, reject) {
      var t = setTimeout(function () { reject(new Error('timeout')); }, ms);
      (waiters[key] = waiters[key] || []).push(function (v) { clearTimeout(t); resolve(v); });
    });
  }

  // fetchDetail asks TweetDetail for focalId (and a page cursor), with the
  // page's own request: without its transaction id first if X refuses it.
  function fetchDetail(focalId, cursor) {
    var ready = tpl ? Promise.resolve() : wait('*tpl*', 8000);
    return ready.then(function () {
      var u = new URL(tpl.url);
      var v = {};
      try { v = JSON.parse(u.searchParams.get('variables') || '{}'); } catch (e) {}
      v.focalTweetId = focalId;
      delete v.cursor;
      if (cursor) { v.cursor = cursor; v.referrer = 'tweet'; }
      u.searchParams.set('variables', JSON.stringify(v));
      var h = Object.assign({}, tpl.headers);
      var csrf = cookie('ct0');
      if (csrf) h['x-csrf-token'] = csrf;
      var ask = function (headers) {
        return origFetch.call(Win, u.href, { headers: headers, credentials: 'include' }).then(function (r) {
          if (!r.ok) throw new Error('HTTP ' + r.status);
          return r.json();
        });
      };
      return ask(h).catch(function (e) {
        if (!h['x-client-transaction-id']) throw e;
        var h2 = Object.assign({}, h);
        delete h2['x-client-transaction-id'];
        return ask(h2);
      });
    }).then(function (j) {
      if (j && j.errors && !j.data) throw new Error((j.errors[0] && j.errors[0].message) || 'error');
      return parseDetail(j, focalId);
    });
  }

  function load(focalId) {
    if (cache[focalId]) return Promise.resolve(cache[focalId]);
    return fetchDetail(focalId).then(function (d) { cache[focalId] = d; return d; });
  }

  // ---- X's look ------------------------------------------------------------------

  // theme reads the page's background: X's light, dim or dark ("lights out").
  function theme() {
    var bg = '', nodes = [document.body, document.documentElement, document.querySelector('main')];
    for (var i = 0; i < nodes.length && !bg; i++) {
      var c = nodes[i] && getComputedStyle(nodes[i]).backgroundColor;
      if (c && !/^transparent$|rgba\([^)]*,\s*0\)$/.test(c)) bg = c;
    }
    var m = bg.match(/\d+/g) || [255, 255, 255];
    var r = +m[0], g = +m[1], b = +m[2];
    if (r < 10 && g < 10 && b < 10) return 'dark';
    if (r + g + b < 200) return 'dim';
    return 'light';
  }

  var CSS = '\
.xtv-layer{position:fixed;inset:0;z-index:2147483000;overflow:hidden}\
.xtv-dlg{--bg:#fff;--fg:#0f1419;--sub:#536471;--line:#eff3f4;--thread:#cfd9de;--hover:rgba(0,0,0,.03);--accent:#1d9bf0;--chip:rgba(29,155,240,.1);--chiphover:rgba(29,155,240,.2);--btnhover:rgba(15,20,25,.1);--bar:rgba(255,255,255,.85);--skel:rgba(15,20,25,.06);\
position:absolute;inset:0;background:var(--bg);color:var(--fg);display:flex;flex-direction:column;overflow:hidden;will-change:transform;\
font:15px/20px TwitterChirp,-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;box-shadow:-10px 0 30px rgba(0,0,0,.18)}\
.xtv-dlg.dim{--bg:#15202b;--fg:#f7f9f9;--sub:#8b98a5;--line:#38444d;--thread:#38444d;--hover:rgba(255,255,255,.03);--btnhover:rgba(239,243,244,.1);--bar:rgba(21,32,43,.75);--skel:rgba(255,255,255,.07)}\
.xtv-dlg.dark{--bg:#000;--fg:#e7e9ea;--sub:#71767b;--line:#2f3336;--thread:#333639;--hover:rgba(255,255,255,.03);--btnhover:rgba(239,243,244,.1);--bar:rgba(0,0,0,.65);--skel:rgba(255,255,255,.07);box-shadow:0 -10px 30px rgba(0,0,0,.6)}\
.xtv-dlg.under{pointer-events:none}\
.xtv-shade{position:absolute;inset:0;background:#000;opacity:0;pointer-events:none;transition:opacity .34s cubic-bezier(.2,.8,.2,1)}\
.xtv-dlg.under .xtv-shade{opacity:.35}\
.xtv-col{max-width:600px;margin:0 auto;min-height:100%;border-left:1px solid var(--line);border-right:1px solid var(--line)}\
@media (max-width:640px){.xtv-col{border:0}}\
.xtv-dlg a{color:var(--accent);text-decoration:none}.xtv-dlg a:hover{text-decoration:underline}\
.xtv-top{display:flex;align-items:center;gap:20px;height:53px;padding:0 16px;position:sticky;top:0;z-index:2;background:var(--bar);-webkit-backdrop-filter:blur(12px);backdrop-filter:blur(12px)}\
.xtv-ttl{flex:1;min-width:0;display:flex;flex-direction:column;justify-content:center}\
.xtv-ttl h2{margin:0;font-size:20px;line-height:24px;font-weight:700;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}\
.xtv-crumb{color:var(--sub);font-size:13px;line-height:16px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}\
.xtv-ib{appearance:none;border:0;background:transparent;color:var(--fg);width:34px;height:34px;border-radius:50%;display:grid;place-items:center;cursor:pointer;flex:none;transition:background .2s}\
.xtv-ib:hover{background:var(--btnhover)}\
.xtv-ib svg{width:20px;height:20px}\
.xtv-scroll{flex:1;overflow-y:auto;overscroll-behavior:contain}\
.xtv-ib svg,.xtv-act svg,.xtv-rp svg,.xtv-hrp svg,.xtv-badge svg,.xtv-light .lb svg{fill:currentColor}\
.xtv-ib svg path[fill="none"],.xtv-act svg path[fill="none"]{fill:none}\
.xtv-skel{display:flex;gap:12px;padding:12px 16px;border-bottom:1px solid var(--line)}\
.xtv-skel i{display:block;border-radius:999px;background:linear-gradient(90deg,var(--skel) 25%,rgba(128,128,128,.14) 50%,var(--skel) 75%);background-size:300% 100%;animation:xtv-shim 1.2s ease-in-out infinite}\
.xtv-skel .a{width:40px;height:40px;flex:none}\
.xtv-skel .l{display:flex;flex-direction:column;gap:8px;flex:1;padding-top:4px}\
@keyframes xtv-shim{0%{background-position:100% 0}100%{background-position:0 0}}\
.xtv-row{display:flex;gap:12px;padding:12px 16px 0;cursor:pointer;transition:background .2s}\
.xtv-row:hover{background:var(--hover)}\
.xtv-rail{display:flex;flex-direction:column;align-items:center;flex:none;width:40px}\
.xtv-rail .up{width:2px;height:8px;margin:-12px 0 4px;background:var(--thread)}\
.xtv-rail .up.none{background:transparent}\
.xtv-rail .down{width:2px;flex:1;min-height:12px;margin-top:4px;background:var(--thread)}\
.xtv-av{width:40px;height:40px;border-radius:50%;object-fit:cover;background:var(--line);flex:none;display:block}\
.xtv-main{flex:1;min-width:0;padding-bottom:12px}\
.xtv-row.last .xtv-main{border-bottom:1px solid var(--line)}\
.xtv-who{display:flex;gap:4px;align-items:baseline;min-width:0;white-space:nowrap}\
.xtv-who b{font-weight:700;overflow:hidden;text-overflow:ellipsis;color:var(--fg)}\
.xtv-who .sub{color:var(--sub);overflow:hidden;text-overflow:ellipsis}\
.xtv-badge{width:18px;height:18px;align-self:center;color:var(--accent);flex:none}\
.xtv-badge svg{display:block;width:18px;height:18px}\
.xtv-nr{display:inline-flex;gap:2px;align-items:center;min-width:0;max-width:100%}\
.xtv-text{margin-top:2px;white-space:normal;overflow-wrap:anywhere}\
.xtv-media{display:grid;gap:2px;margin-top:12px;border-radius:16px;overflow:hidden;border:1px solid var(--line)}\
.xtv-media.n2,.xtv-media.n3,.xtv-media.n4{grid-template-columns:1fr 1fr}\
.xtv-media img,.xtv-media video{width:100%;max-height:510px;object-fit:cover;display:block;background:var(--line)}\
.xtv-media img{cursor:zoom-in}\
.xtv-media.n2 img,.xtv-media.n3 img,.xtv-media.n4 img{aspect-ratio:1;height:100%}\
.xtv-quote{margin-top:12px;border:1px solid var(--line);border-radius:16px;padding:12px;transition:background .2s}\
.xtv-quote:hover{background:var(--hover)}\
.xtv-quote .xtv-av{width:20px;height:20px}\
.xtv-acts{display:flex;justify-content:space-between;max-width:425px;margin-top:12px;color:var(--sub);font-size:13px}\
.xtv-act{display:flex;align-items:center;gap:4px;min-width:0}\
.xtv-act svg{width:18.75px;height:18.75px}\
.xtv-act.open{appearance:none;border:0;background:none;font:inherit;font-size:13px;color:var(--sub);cursor:pointer;padding:0;margin:-8px 0 -8px -8px;height:34px;padding:0 8px;border-radius:999px;transition:color .2s,background .2s}\
.xtv-act.open:hover{color:var(--accent);background:var(--chip)}\
.xtv-rp{appearance:none;border:0;background:none;color:var(--sub);width:30px;height:30px;margin:-6px -6px -6px 2px;border-radius:50%;display:inline-grid;place-items:center;cursor:pointer;flex:none;align-self:center;transition:color .2s,background .2s}\
.xtv-rp:hover{color:var(--accent);background:var(--chip)}\
.xtv-rp svg{width:17px;height:17px}\
.xtv-hrp{appearance:none;border:0;background:none;color:rgb(113,118,123);width:30px;height:30px;margin:-5px 0 -5px 6px;border-radius:50%;display:inline-grid;place-items:center;cursor:pointer;vertical-align:middle;transition:color .2s,background .2s}\
.xtv-hrp:hover{color:#1d9bf0;background:rgba(29,155,240,.1)}\
.xtv-hrp svg{width:17px;height:17px}\
.xtv-foot{display:flex;align-items:center;gap:12px;margin-top:10px;min-height:28px}\
.xtv-chip{appearance:none;border:0;display:inline-flex;align-items:center;gap:6px;height:28px;padding:0 12px;border-radius:999px;background:var(--chip);color:var(--accent);font:inherit;font-size:13px;font-weight:700;cursor:pointer;transition:background .2s}\
.xtv-chip:hover{background:var(--chiphover)}\
.xtv-chip svg{width:16px;height:16px}\
.xtv-goto{appearance:none;border:0;background:none;color:var(--sub);font:inherit;font-size:13px;cursor:pointer;display:inline-flex;align-items:center;gap:4px;padding:0}\
.xtv-goto:hover{color:var(--accent)}\
.xtv-goto svg{width:15px;height:15px}\
.xtv-focal{padding:12px 16px 0}\
.xtv-focal .xtv-head{display:flex;gap:12px;align-items:center;margin-bottom:12px}\
.xtv-focal .xtv-text{font-size:17px;line-height:24px}\
.xtv-focal .xtv-when{color:var(--sub);margin:16px 0 0;font-size:15px}\
.xtv-focal .xtv-when b{color:var(--fg);font-weight:700}\
.xtv-focal .xtv-acts{border-top:1px solid var(--line);margin:12px 0 0;max-width:none;padding:0 4px;height:48px;align-items:center}\
.xtv-focal .xtv-foot{margin:0 0 12px}\
.xtv-reply-to{color:var(--sub);margin:4px 0 0}\
.xtv-div{display:flex;align-items:center;justify-content:space-between;padding:14px 16px;border-top:1px solid var(--line);border-bottom:1px solid var(--line);font-weight:700;font-size:17px;position:sticky;top:0;background:var(--bg);z-index:1}\
.xtv-div .n{color:var(--sub);font-weight:400;font-size:15px}\
.xtv-more{display:block;width:100%;appearance:none;border:0;border-bottom:1px solid var(--line);background:none;color:var(--accent);font:inherit;padding:16px;text-align:left;cursor:pointer;transition:background .2s}\
.xtv-more:hover{background:var(--hover)}\
.xtv-end{display:flex;align-items:center;gap:12px;color:var(--sub);font-size:13px;padding:20px 16px 28px}\
.xtv-end:before,.xtv-end:after{content:"";flex:1;height:1px;background:var(--line)}\
.xtv-msg{color:var(--sub);text-align:center;padding:40px 16px}\
.xtv-msg a{display:inline-block;margin-top:12px}\
.xtv-spin{width:26px;height:26px;margin:40px auto;border-radius:50%;border:3px solid rgba(29,155,240,.2);border-top-color:var(--accent,#1d9bf0);animation:xtv-rot .75s linear infinite}\
@keyframes xtv-rot{to{transform:rotate(360deg)}}\
@keyframes xtv-flash{0%,30%{background:rgba(29,155,240,.18)}100%{background:transparent}}\
.xtv-flash{animation:xtv-flash 1.6s ease-out}\
.xtv-light{position:fixed;inset:0;z-index:2147483010;background:rgba(0,0,0,.9);display:flex;align-items:center;justify-content:center}\
@keyframes xtv-fade{from{opacity:0}to{opacity:1}}\
.xtv-light img{max-width:calc(100vw - 160px);max-height:calc(100vh - 96px);object-fit:contain}\
.xtv-light .lb{position:absolute;appearance:none;border:0;width:36px;height:36px;border-radius:50%;background:rgba(15,20,25,.75);color:#fff;display:grid;place-items:center;cursor:pointer;transition:background .2s}\
.xtv-light .lb:hover{background:rgba(39,44,48,.75)}\
.xtv-light .lb svg{width:20px;height:20px}\
.xtv-light .x{top:12px;left:12px}.xtv-light .p{left:16px;top:50%;margin-top:-18px}.xtv-light .nx{right:16px;top:50%;margin-top:-18px}\
.xtv-light .ct{position:absolute;top:18px;left:60px;color:#fff;font:600 15px -apple-system,sans-serif}\
.xtv-light .go{position:absolute;bottom:16px;right:16px;appearance:none;border:0;border-radius:999px;background:rgba(15,20,25,.75);color:#fff;font:600 14px -apple-system,sans-serif;padding:8px 14px;cursor:pointer}';

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
  function button(cls, html, title, fn) {
    var b = el('button', cls, html);
    b.type = 'button';
    if (title) b.title = title;
    b.addEventListener('click', function (e) { e.preventDefault(); e.stopPropagation(); fn(e); });
    return b;
  }

  // Simple line icons in X's weight (not X's own drawings).
  var ICON = {
    back: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5"/><path d="M11 18l-6-6 6-6"/></svg>',
    close: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg>',
    open: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 4h6v6"/><path d="M20 4l-9 9"/><path d="M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/></svg>',
    reply: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linejoin="round"><path d="M4 11.5C4 7.4 7.4 4.5 11.5 4.5h1C16.6 4.5 20 7.4 20 11.3c0 4-3.4 6.7-7.6 6.7H11l-4.5 3v-3.5C4.9 16.4 4 14.1 4 11.5z"/></svg>',
    repost: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="M7 20V8a2 2 0 0 1 2-2h7"/><path d="M4 17l3 3 3-3"/><path d="M17 4v12a2 2 0 0 1-2 2H8"/><path d="M20 7l-3-3-3 3"/></svg>',
    like: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linejoin="round"><path d="M12 20s-7.5-4.6-7.5-10.2A4.3 4.3 0 0 1 12 7.2a4.3 4.3 0 0 1 7.5 2.6C19.5 15.4 12 20 12 20z"/></svg>',
    views: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round"><path d="M5 20V12M10 20V4M15 20v-9M20 20v-5"/></svg>',
    answer: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 8L5 13l5 5"/><path d="M5 13h9a5 5 0 0 0 5-5V5"/></svg>',
    goto: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M7 7v6a3 3 0 0 0 3 3h8"/><path d="M14 12l4 4-4 4"/></svg>',
    prev: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6"/></svg>',
    next: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18l6-6-6-6"/></svg>',
    badge: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2.5l2.4 1.8 3-.2.9 2.9 2.5 1.7-1 2.8 1 2.8-2.5 1.7-.9 2.9-3-.2L12 21.5l-2.4-1.8-3 .2-.9-2.9-2.5-1.7 1-2.8-1-2.8 2.5-1.7.9-2.9 3 .2z"/><path d="M8.5 12.2l2.3 2.3 4.7-4.9" fill="none" stroke="#fff" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>'
  };

  // I is an icon: X's own, taken from the page where X already drew it
  // (its buttons and bars), so it is always X's current icon; the line
  // icons above stand in when the page has none to lend.
  // X (2026) draws its icons from a sprite of <symbol id="icon-…">, added
  // as the page first needs each: a <use> of it is X's icon itself.
  var XSPRITE = {
    reply: ['icon-reply'], repost: ['icon-repost'], like: ['icon-heart'], views: ['icon-bar-chart'],
    back: ['icon-arrow-left', 'icon-back'], close: ['icon-close', 'icon-x'],
    badge: ['icon-verified-premium-color', 'icon-verified'], answer: ['icon-compose', 'icon-reply']
  };
  // Until 2026, X's icons were inline in its buttons: they are copied.
  var XICON = {
    reply: '[data-testid="reply"] svg',
    repost: '[data-testid="retweet"] svg, [data-testid="unretweet"] svg',
    like: '[data-testid="like"] svg',
    views: 'a[href*="/analytics"] svg',
    back: '[data-testid="app-bar-back"] svg',
    close: '[data-testid="app-bar-close"] svg',
    badge: '[data-testid="icon-verified"]',
    answer: '[data-testid="SideNav_NewTweet_Button"] svg'
  };
  var xicons = {};
  function I(name) {
    if (xicons[name]) return xicons[name];
    var ids = XSPRITE[name] || [];
    for (var i = 0; i < ids.length; i++) {
      if (document.getElementById(ids[i])) {
        xicons[name] = '<svg viewBox="0 0 24 24" aria-hidden="true" fill="currentColor"><use href="#' + ids[i] + '"></use></svg>';
        return xicons[name];
      }
    }
    var q = XICON[name];
    var src = q && document.querySelector(q);
    if (src && !(layer && layer.root.contains(src))) {
      var c = src.cloneNode(true);
      c.removeAttribute('class');
      c.removeAttribute('style');
      c.setAttribute('aria-hidden', 'true');
      c.querySelectorAll('[class]').forEach(function (n) { n.removeAttribute('class'); });
      xicons[name] = c.outerHTML;
      return xicons[name];
    }
    return ICON[name] || '';
  }

  // Motion: one easing, X's speed; none when the system asks for less.
  var RM = !!(window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches);
  var EASE = 'cubic-bezier(.2,.8,.2,1)', EASE_OUT = 'cubic-bezier(.4,0,.2,1)';
  function anim(node, frames, opts) {
    if (RM || !node.animate) return { finished: Promise.resolve() };
    var a = node.animate(frames, opts);
    var done = a.finished || new Promise(function (r) { a.onfinish = r; });
    // A hidden window pauses animations: never wait on one longer than it lasts.
    var cap = new Promise(function (r) { setTimeout(r, (opts.duration || 0) + (opts.delay || 0) + 150); });
    return { finished: Promise.race([done.catch(function () {}), cap]).then(function () { if (a.playState !== 'finished') a.finish && a.finish(); }) };
  }
  function onScreen(node) {
    if (!node || !node.isConnected) return null;
    var r = node.getBoundingClientRect();
    return r.width > 0 && r.bottom > 0 && r.top < window.innerHeight ? r : null;
  }

  function num(n) {
    if (!n) return '';
    if (n >= 1e6) return (n / 1e6).toFixed(1).replace(/\.0$/, '') + 'M';
    if (n >= 1e4) return Math.round(n / 1e3) + 'K';
    if (n >= 1e3) return (n / 1e3).toFixed(1).replace(/\.0$/, '') + 'K';
    return String(n);
  }
  function ago(d) {
    if (!d) return '';
    var s = (Date.now() - d.getTime()) / 1000;
    if (s < 60) return Math.max(1, Math.floor(s)) + 's';
    if (s < 3600) return Math.floor(s / 60) + 'm';
    if (s < 86400) return Math.floor(s / 3600) + 'h';
    var opts = { month: 'short', day: 'numeric' };
    if (d.getFullYear() !== new Date().getFullYear()) opts.year = 'numeric';
    return d.toLocaleDateString(undefined, opts);
  }
  function sized(u, name) { return /^https:\/\/pbs\.twimg\.com\//.test(u) ? u + (u.indexOf('?') < 0 ? '?' : '&') + 'name=' + name : u; }

  // ---- the page's own posts: "Go to reply" finds them there -----------------------

  var STATUS = /^\/(?:[^/]+|i\/web|i)\/status\/(\d+)/;
  function pageStatus() {
    var m = location.pathname.match(STATUS);
    return m ? m[1] : '';
  }
  // X's markup: until 2026 a post is article[data-testid=tweet] with a
  // <time> in its link; since, a plain <article> whose time is a link to
  // /<user>/status/<id> (its photos and stats link deeper than that).
  var POST = 'article';
  var REPLY = '[data-testid="reply"], [data-engagement-action="reply"] a, [data-engagement-action="reply"] button';
  var POST_LINK = /^\/(?:[^/]+|i\/web|i)\/status\/\d+\/?$/;
  function timeLink(art) {
    var t = art.querySelector('a[href*="/status/"] time');
    if (t) return t.closest('a');
    // Its own post links, not a quoted post's: on the post's page the one
    // to the page's post, else one by the post's author (its avatar's).
    var links = [].filter.call(art.querySelectorAll('a[href*="/status/"]'), function (a) {
      return POST_LINK.test(a.getAttribute('href') || '');
    });
    var here = pageStatus();
    for (var i = 0; i < links.length; i++) {
      if (here && statusIdOf(links[i].getAttribute('href')) === here) return links[i];
    }
    var av = art.querySelector('[data-post-avatar] a[href], a[href] img[alt^="@"]');
    var who = av ? ((av.closest('a') || av).getAttribute('href') || '').split('/')[1] : '';
    for (var j = 0; who && j < links.length; j++) {
      if ((links[j].getAttribute('href').split('/')[1] || '').toLowerCase() === who.toLowerCase()) return links[j];
    }
    return links[0] || null;
  }
  function postOf(node) {
    var art = node && node.closest && node.closest(POST);
    // A quoted post is not a post of the page's own.
    while (art && art.parentElement && art.parentElement.closest(POST)) art = art.parentElement.closest(POST);
    return art;
  }
  function articleStatus(art) {
    var a = timeLink(art);
    return a ? statusIdOf(a.getAttribute('href')) : '';
  }
  function pageArticle(id) {
    var arts = document.querySelectorAll(POST);
    for (var i = 0; i < arts.length; i++) {
      if (!(layer && layer.root.contains(arts[i])) && articleStatus(arts[i]) === id) return arts[i];
    }
    return null;
  }
  // goTo closes the panels and shows id where it is on the page.
  function goTo(id) {
    var art = pageArticle(id);
    closeAll(function () {
      if (!art) return;
      var cell = art.closest('[data-testid="cellInnerDiv"], li') || art;
      cell.scrollIntoView({ block: 'center', behavior: RM ? 'auto' : 'smooth' });
      cell.classList.remove('xtv-flash');
      void cell.offsetWidth;
      cell.classList.add('xtv-flash');
    });
  }


  // compose opens X's own reply box for id: the post's reply button on the
  // page when it is there, else X's reply window.
  var composing = false;
  function compose(id) {
    var art = pageArticle(id);
    var rb = art && art.querySelector(REPLY);
    closeAll();
    if (rb) {
      composing = true;
      try { rb.click(); } finally { composing = false; }
      return;
    }
    window.open('/intent/post?in_reply_to=' + id, '_blank', 'width=600,height=640');
  }

  // ---- posts, X's way ---------------------------------------------------------------

  // sheetCtx is what a panel's posts can do: open a reply, show a photo.
  function mediaEl(t, ctx) {
    if (!t.media.length) return null;
    var n = Math.min(t.media.length, 4);
    var box = el('div', 'xtv-media n' + n);
    t.media.slice(0, 4).forEach(function (m) {
      if (m.video) {
        var v = document.createElement('video');
        v.src = m.video; v.poster = m.url; v.controls = true; v.preload = 'none';
        if (m.type === 'animated_gif') { v.loop = true; v.muted = true; v.autoplay = true; }
        v.addEventListener('click', function (e) { e.stopPropagation(); });
        box.appendChild(v);
      } else {
        var img = document.createElement('img');
        img.src = sized(m.url, n > 1 ? 'small' : 'medium'); img.loading = 'lazy'; img.alt = W_.photo;
        var shot = ctx.photo(m.url, t.id);
        img.addEventListener('click', function (e) { e.stopPropagation(); ctx.viewer(shot, img); });
        box.appendChild(img);
      }
    });
    return box;
  }

  function avatarEl(t) {
    var a = el('a');
    a.href = '/' + t.handle;
    a.addEventListener('click', function (e) { e.stopPropagation(); });
    var img = document.createElement('img');
    img.className = 'xtv-av'; img.alt = ''; img.src = t.avatar || '';
    a.appendChild(img);
    return a;
  }

  function whoEl(t, long) {
    var w = el('div', 'xtv-who');
    var name = el('a', '', '<b>' + esc(t.name || t.handle) + '</b>');
    name.href = '/' + t.handle;
    name.style.color = 'inherit';
    name.addEventListener('click', function (e) { e.stopPropagation(); });
    // The badge stays on the name's line, as X has it.
    var nr = el('span', 'xtv-nr');
    nr.appendChild(name);
    if (t.verified) nr.appendChild(el('span', 'xtv-badge', I('badge')));
    w.appendChild(nr);
    w.appendChild(el('span', 'sub', '@' + esc(t.handle)));
    if (!long && t.created) {
      w.appendChild(el('span', 'sub', '·'));
      var tm = el('span', 'sub', esc(ago(t.created)));
      tm.title = t.created.toLocaleString();
      w.appendChild(tm);
    }
    // Writing a reply lives here; the 💬 count shows the replies.
    w.appendChild(button('xtv-rp', I('answer'), W_.reply, function () { compose(t.id); }));
    return w;
  }

  function actsEl(t, big, ctx) {
    var a = el('div', 'xtv-acts');
    [['reply', t.replies], ['repost', t.reposts], ['like', t.likes], ['views', t.views]].forEach(function (p) {
      if (big && p[0] === 'views') return;
      if (p[0] === 'reply' && t.replies > 0 && ctx) {
        a.appendChild(button('xtv-act open', I('reply') + '<span>' + num(p[1]) + '</span>', W_.showRepliesOf(t.replies), function () { ctx.open(t.id); }));
        return;
      }
      a.appendChild(el('div', 'xtv-act', I(p[0]) + '<span>' + num(p[1]) + '</span>'));
    });
    return a;
  }

  // footEl: the 💬 N chip that opens the replies on top, and "Go to reply"
  // when the post is on the page underneath.
  function footEl(t) {
    var f = el('div', 'xtv-foot');
    if (pageArticle(t.id)) f.appendChild(button('xtv-goto', ICON.goto + '<span>' + esc(W_.goTo) + '</span>', '', function () { goTo(t.id); }));
    return f.childNodes.length ? f : null;
  }

  function quoteEl(q, ctx) {
    var box = el('div', 'xtv-quote');
    var head = el('div', 'xtv-who');
    var img = document.createElement('img');
    img.className = 'xtv-av'; img.alt = ''; img.src = q.avatar || '';
    head.appendChild(img);
    head.appendChild(el('b', '', esc(q.name || q.handle)));
    head.appendChild(el('span', 'sub', '@' + esc(q.handle) + ' · ' + esc(ago(q.created))));
    box.appendChild(head);
    box.appendChild(el('div', 'xtv-text', renderText(q)));
    var m = mediaEl(q, ctx);
    if (m) box.appendChild(m);
    box.addEventListener('click', function (e) { e.stopPropagation(); if (!e.target.closest('a,video,img')) ctx.open(q.id); });
    return box;
  }

  // rowEl is a post in a list: avatar rail joined to the posts above and
  // below in its thread; a click opens it on top.
  function rowEl(t, ctx, up, down, last) {
    var row = el('div', 'xtv-row' + (last ? ' last' : ''));
    row.setAttribute('role', 'link');
    row.tabIndex = 0;
    var rail = el('div', 'xtv-rail');
    rail.appendChild(el('div', 'up' + (up ? '' : ' none')));
    rail.appendChild(avatarEl(t));
    if (down) rail.appendChild(el('div', 'down'));
    row.appendChild(rail);
    var main = el('div', 'xtv-main');
    main.appendChild(whoEl(t, false));
    main.appendChild(el('div', 'xtv-text', renderText(t)));
    var m = mediaEl(t, ctx);
    if (m) main.appendChild(m);
    if (t.quoted) main.appendChild(quoteEl(t.quoted, ctx));
    main.appendChild(actsEl(t, false, ctx));
    var f = footEl(t);
    if (f) main.appendChild(f);
    row.appendChild(main);
    row.addEventListener('click', function (e) {
      if (e.target.closest('a,video,button,img')) return;
      if (e.metaKey || e.ctrlKey) { window.open('/' + t.handle + '/status/' + t.id, '_blank'); return; }
      ctx.open(t.id);
    });
    row.addEventListener('keydown', function (e) { if (e.key === 'Enter') ctx.open(t.id); });
    return row;
  }

  // focalEl is the post a panel is about, large, as X shows a post's page.
  function focalEl(t, ctx) {
    var box = el('div', 'xtv-focal');
    var head = el('div', 'xtv-head');
    head.appendChild(avatarEl(t));
    var w = whoEl(t, true);
    w.style.flexDirection = 'column'; w.style.gap = '0'; w.style.alignItems = 'flex-start';
    w.style.flex = '1';
    var rp = w.querySelector('.xtv-rp');
    head.appendChild(w);
    if (rp) { rp.style.margin = '0'; head.appendChild(rp); }
    box.appendChild(head);
    if (t.replyToHandle) box.appendChild(el('div', 'xtv-reply-to', esc(W_.replyingTo) + ' <a href="/' + esc(t.replyToHandle) + '">@' + esc(t.replyToHandle) + '</a>'));
    box.appendChild(el('div', 'xtv-text', renderText(t)));
    var m = mediaEl(t, ctx);
    if (m) box.appendChild(m);
    if (t.quoted) box.appendChild(quoteEl(t.quoted, ctx));
    if (t.created) {
      var when = t.created.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' }) + ' · ' +
        t.created.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
      box.appendChild(el('div', 'xtv-when', esc(when) + (t.views ? ' · <b>' + esc(num(t.views)) + '</b> ' + esc(W_.views) : '')));
    }
    box.appendChild(actsEl(t, true, ctx));
    var f = footEl(t);
    if (f) box.appendChild(f);
    return box;
  }

  // ---- full-screen levels: a post opens over the page, its replies push on top ---------

  var layer = null;   // { root, theme, overflow }
  var sheets = [];    // open levels, bottom first: { id, el, body, shade, name, from }
  var viewer = null;  // the photo viewer, when open

  function crumb() {
    var top = sheets[sheets.length - 1];
    if (!top) return;
    var c = top.el.querySelector('.xtv-crumb');
    var names = sheets.map(function (x) { return x.name; }).filter(Boolean);
    c.textContent = names.length > 1 ? names.join(' › ') : '';
    c.style.display = names.length > 1 ? '' : 'none';
  }

  function skeleton() {
    var box = el('div', '');
    for (var i = 0; i < 4; i++) {
      var row = el('div', 'xtv-skel');
      row.appendChild(el('i', 'a'));
      var l = el('div', 'l');
      [38, 92, 74].forEach(function (w, j) { var b = el('i'); b.style.width = (j ? w : w) + '%'; b.style.height = j ? '12px' : '14px'; l.appendChild(b); });
      row.appendChild(l);
      box.appendChild(row);
    }
    return box;
  }

  // openSheet shows post id full screen: it rises from the bottom of the
  // screen, over the page or over the level below (which steps back).
  function openSheet(id, from) {
    ensureStyle();
    var prev = sheets[sheets.length - 1];
    if (prev && prev.id === id) return;
    if (!layer) {
      var th = theme();
      var root = el('div', 'xtv-layer');
      document.body.appendChild(root);
      layer = { root: root, theme: th, overflow: document.documentElement.style.overflow };
      document.documentElement.style.overflow = 'hidden';
    }
    var sheet = el('div', 'xtv-dlg ' + layer.theme);
    sheet.setAttribute('role', 'dialog');
    sheet.setAttribute('aria-modal', 'true');
    var scroll = el('div', 'xtv-scroll');
    var col = el('div', 'xtv-col');
    var bar = el('div', 'xtv-top');
    bar.appendChild(button('xtv-ib', I('back'), W_.close, closeTop));
    var ttl = el('div', 'xtv-ttl', '<h2>' + esc(W_.post) + '</h2>');
    ttl.appendChild(el('div', 'xtv-crumb'));
    bar.appendChild(ttl);
    var open = el('a', 'xtv-ib', ICON.open);
    open.href = '/i/status/' + id;
    open.title = W_.open;
    open.addEventListener('click', function (e) { e.preventDefault(); closeAll(function () { location.assign(open.href); }); });
    bar.appendChild(open);
    var body = el('div', '');
    body.appendChild(skeleton());
    col.appendChild(bar);
    col.appendChild(body);
    scroll.appendChild(col);
    sheet.appendChild(scroll);
    var shade = el('div', 'xtv-shade');
    sheet.appendChild(shade);
    layer.root.appendChild(sheet);
    var s = { id: id, el: sheet, body: body, shade: shade, name: '', from: from || null };
    sheets.push(s);
    crumb();
    // Each level rises from the bottom of the screen into place, over a
    // dimming page; the level under it steps back.
    if (!prev) {
      anim(layer.root, [{ backgroundColor: 'rgba(0,0,0,0)' }, { backgroundColor: 'rgba(0,0,0,.4)' }], { duration: RISE, easing: EASE_OUT });
    } else {
      park(prev, true);
    }
    rise(sheet);
    fill(s, body);
  }

  // rise brings a level up from below the screen to its place.
  var RISE = 460, SHEET = 'cubic-bezier(.32,.72,0,1)';
  function rise(sheet) {
    anim(sheet, [{ transform: 'translateY(100%)' }, { transform: 'translateY(0)' }], { duration: RISE, easing: SHEET });
  }
  // sink takes a level back down below the screen.
  function sink(sheet) {
    return anim(sheet, [{ transform: 'translateY(0)' }, { transform: 'translateY(100%)' }], { duration: 300, easing: EASE_OUT });
  }

  // park steps the level under the top one back (smaller, dimmed), or forward.
  function park(s, under) {
    var a = under ? 'scale(1)' : 'scale(.94)', b = under ? 'scale(.94)' : 'scale(1)';
    s.el.style.transform = b;
    s.el.classList.toggle('under', under);
    s.el.setAttribute('aria-hidden', under ? 'true' : 'false');
    s.el.style.transformOrigin = '50% 0';
    anim(s.el, [{ transform: a }, { transform: b }], { duration: under ? RISE : 300, easing: under ? SHEET : EASE_OUT });
  }

  // fill loads a panel: the post it answers, the post, "Replies", the
  // replies (each with its own 💬 N), and the end mark.
  function fill(s, scroll) {
    var shots = [];
    var ctx = {
      open: function (id) { openSheet(id); },
      photo: function (url, from) { var i = shots.length; shots.push({ url: url, from: from }); return i; },
      viewer: function (i, thumb) { openViewer(shots, i, thumb); }
    };
    load(s.id).then(function (d) {
      if (sheets.indexOf(s) < 0) return;
      scroll.innerHTML = '';
      if (!d.focal) { failed(scroll, s.id); return; }
      s.name = d.focal.name || '@' + d.focal.handle;
      crumb();
      var parent = d.ancestors[d.ancestors.length - 1];
      if (parent) scroll.appendChild(rowEl(parent, ctx, false, true, false));
      scroll.appendChild(focalEl(d.focal, ctx));
      var head = el('div', 'xtv-div', esc(W_.repliesHead));
      if (d.focal.replies) head.appendChild(el('span', 'n', esc(num(d.focal.replies))));
      scroll.appendChild(head);
      var list = el('div', '');
      scroll.appendChild(list);
      renderThreads(list, d.threads, ctx);
      tail(list, s.id, d.cursor, ctx, !d.threads.length);
      anim(scroll, [{ opacity: 0, transform: 'translateY(8px)' }, { opacity: 1, transform: 'none' }], { duration: 240, easing: EASE });
    }, function () {
      if (sheets.indexOf(s) < 0) return;
      scroll.innerHTML = '';
      failed(scroll, s.id);
    });
  }

  function renderThreads(list, threads, ctx) {
    threads.forEach(function (th) {
      th.forEach(function (t, i) {
        list.appendChild(rowEl(t, ctx, i > 0, i < th.length - 1, i === th.length - 1));
      });
    });
  }

  // tail is "Show more replies" while there are more, then the end mark.
  function tail(list, id, cursor, ctx, empty) {
    if (!cursor) {
      list.appendChild(el('div', empty ? 'xtv-msg' : 'xtv-end', esc(empty ? W_.none : W_.end)));
      return;
    }
    var b = button('xtv-more', esc(W_.more), '', function () {
      b.disabled = true;
      b.innerHTML = '<div class="xtv-spin" style="margin:0 auto;width:20px;height:20px"></div>';
      fetchDetail(id, cursor).then(function (p) {
        b.remove();
        var before = list.children.length;
        renderThreads(list, p.threads, ctx);
        for (var k = before; k < list.children.length; k++) {
          anim(list.children[k], [{ opacity: 0, transform: 'translateY(8px)' }, { opacity: 1, transform: 'none' }], { duration: 260, delay: Math.min(k - before, 10) * 30, easing: EASE, fill: 'backwards' });
        }
        tail(list, id, p.cursor && p.cursor !== cursor ? p.cursor : '', ctx, false);
      }, function () { b.remove(); tail(list, id, '', ctx, false); });
    });
    list.appendChild(b);
  }

  function failed(scroll, id) {
    var m = el('div', 'xtv-msg', esc(W_.failed) + '<br>');
    var a = el('a', '', esc(W_.openInstead));
    a.href = '/i/status/' + id;
    a.addEventListener('click', function () { closeAll(); });
    m.appendChild(a);
    scroll.appendChild(m);
  }

  function closeTop() {
    if (viewer) { closeViewer(); return; }
    var s = sheets.pop();
    if (!s) return;
    if (sheets.length) {
      park(sheets[sheets.length - 1], false);
      crumb();
      sink(s.el).finished.then(function () { s.el.remove(); });
      return;
    }
    var l = layer;
    layer = null;
    document.documentElement.style.overflow = l.overflow;
    var a = sink(s.el);
    anim(l.root, [{ backgroundColor: 'rgba(0,0,0,.4)' }, { backgroundColor: 'rgba(0,0,0,0)' }], { duration: 300, easing: EASE_OUT });
    l.root.style.pointerEvents = 'none';
    a.finished.then(function () { l.root.remove(); });
  }
  // closeAll fades every level out at once, then runs then.
  function closeAll(then) {
    if (viewer) { viewer.remove(); viewer = null; }
    if (!layer) { if (then) then(); return; }
    var l = layer;
    layer = null;
    sheets = [];
    document.documentElement.style.overflow = l.overflow;
    l.root.style.pointerEvents = 'none';
    anim(l.root, [{ opacity: 1 }, { opacity: 0 }], { duration: 180, easing: EASE_OUT }).finished.then(function () {
      l.root.remove();
      if (then) then();
    });
  }

  // ---- photos zoom out of their thumbnail, over the levels ---------------------------

  function closeViewer() {
    var v = viewer;
    viewer = null;
    if (!v) return;
    anim(v, [{ opacity: 1 }, { opacity: 0 }], { duration: 180, easing: EASE_OUT }).finished.then(function () { v.remove(); });
  }

  function openViewer(shots, at, thumb) {
    if (!shots.length) return;
    var box = el('div', 'xtv-light');
    var img = document.createElement('img');
    img.alt = W_.photo;
    var ct = el('div', 'ct');
    box.appendChild(img);
    box.appendChild(button('lb x', I('close'), W_.close, closeViewer));
    var many = shots.length > 1;
    if (many) {
      box.appendChild(ct);
      box.appendChild(button('lb p', ICON.prev, '', function () { go(-1); }));
      box.appendChild(button('lb nx', ICON.next, '', function () { go(1); }));
    }
    var goBtn = button('go', esc(W_.goTo), '', function () { var id = shots[at].from; if (viewer) { viewer.remove(); viewer = null; } goTo(id); });
    box.appendChild(goBtn);
    box.addEventListener('mousedown', function (e) { if (e.target === box) closeViewer(); });
    var first = true;
    function go(d) {
      at = (at + d + shots.length) % shots.length;
      var next = sized(shots[at].url, 'large');
      if (!first) anim(img, [{ opacity: 0.3, transform: 'translateX(' + (d > 0 ? 24 : -24) + 'px)' }, { opacity: 1, transform: 'none' }], { duration: 220, easing: EASE });
      img.src = next;
      ct.textContent = (at + 1) + ' / ' + shots.length;
      goBtn.style.display = pageArticle(shots[at].from) ? '' : 'none';
    }
    go(0);
    first = false;
    box.go = go;
    document.body.appendChild(box);
    viewer = box;
    anim(box, [{ backgroundColor: 'rgba(0,0,0,0)' }, { backgroundColor: 'rgba(0,0,0,.9)' }], { duration: 260, easing: EASE });
    // Zoom from the thumbnail once the photo has its size.
    var from = onScreen(thumb);
    var zoom = function () {
      var to = img.getBoundingClientRect();
      if (!from || !to.width) { anim(img, [{ opacity: 0, transform: 'scale(.96)' }, { opacity: 1, transform: 'none' }], { duration: 240, easing: EASE }); return; }
      var sx = from.width / to.width, sy = from.height / to.height;
      var dx = from.left + from.width / 2 - (to.left + to.width / 2), dy = from.top + from.height / 2 - (to.top + to.height / 2);
      anim(img, [{ transform: 'translate(' + dx + 'px,' + dy + 'px) scale(' + sx + ',' + sy + ')', borderRadius: '16px' }, { transform: 'none', borderRadius: '0' }], { duration: 320, easing: EASE });
    };
    if (img.complete && img.naturalWidth) zoom(); else img.addEventListener('load', zoom, { once: true });
  }

  document.addEventListener('keydown', function (e) {
    if (!layer && !viewer) return;
    if (e.key === 'Escape') { e.preventDefault(); e.stopImmediatePropagation(); closeTop(); return; }
    if (!viewer && e.key === 'ArrowLeft' && e.altKey) { e.preventDefault(); e.stopImmediatePropagation(); closeTop(); return; }
    if (viewer && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
      e.preventDefault(); e.stopImmediatePropagation(); viewer.go(e.key === 'ArrowLeft' ? -1 : 1);
    }
  }, true);

  // ---- on X's page: the 💬 count shows replies; writing one moves up -----------------

  function replyCount(rb) {
    return parseInt(((rb && rb.getAttribute('aria-label')) || '').replace(/[^\d]/g, ''), 10) || 0;
  }

  document.addEventListener('click', function (e) {
    if (composing || e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    var art = postOf(e.target);
    // X (2026) wraps each post in a clickable box around its <article>.
    if (!art && e.target.closest) {
      var box = e.target.closest('li');
      art = box && !(layer && layer.root.contains(box)) && box.querySelector(POST);
    }
    if (!art || (layer && layer.root.contains(art))) return;
    var id = articleStatus(art);
    if (!id) return;
    // Until X's request for a post has been seen once, there is nothing to
    // ask X with: X opens this one, and the next ones open here.
    if (!tpl && !cache[id]) return;
    // The count opens the replies (on any page), when there are some.
    var rb = e.target.closest(REPLY);
    if (rb) {
      if (!replyCount(rb)) return; // none: X's own reply box
      e.preventDefault();
      e.stopImmediatePropagation();
      openSheet(id, art);
      return;
    }
    // A click on a post opens it full screen, over the page (not the post
    // a post's page is about: that one is already open).
    if (id === pageStatus()) return;
    if (e.target.closest('a,button,[role="button"],[role="menu"],[role="group"],video,[data-testid="tweetPhoto"],[data-testid="card.wrapper"],[data-engagement-action],[data-post-avatar],[data-slot],input,textarea,.xtv-hrp')) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    openSheet(id, art);
  }, true);

  // decorate puts a ↩ reply button in each post's header, after its time,
  // and says on the count that it shows the replies.
  function decorate() {
    var arts = document.querySelectorAll(POST + ':not([data-xtv])');
    for (var i = 0; i < arts.length; i++) {
      var art = arts[i];
      if (layer && layer.root.contains(art)) continue;
      if (postOf(art) !== art) continue; // a quoted post
      var ta = timeLink(art);
      if (!ta) continue;
      art.setAttribute('data-xtv', '1');
      var id = statusIdOf(ta.getAttribute('href'));
      ensureStyle();
      var b = el('button', 'xtv-hrp', I('answer'));
      b.type = 'button';
      b.title = W_.reply;
      b.setAttribute('aria-label', W_.reply);
      (function (id) {
        b.addEventListener('click', function (e) { e.preventDefault(); e.stopPropagation(); compose(id); });
      })(id);
      ta.insertAdjacentElement('afterend', b);
      var rb = art.querySelector(REPLY);
      var n = replyCount(rb);
      if (rb && n) rb.setAttribute('title', W_.showRepliesOf(n));
    }
  }
  var pending = 0;
  new MutationObserver(function () {
    if (pending) return;
    pending = requestAnimationFrame(function () { pending = 0; decorate(); });
  }).observe(document.documentElement, { childList: true, subtree: true });
  window.addEventListener('popstate', closeAll);
})();
