// ==UserScript==
// @name         X Thread View
// @namespace    https://mypack.dev/userscripts
// @version      0.3.0
// @description  Open a reply's nested replies in place, in an X-style dialog, without leaving the post you are reading.
// @match        https://x.com/*
// @match        https://twitter.com/*
// @run-at       document-start
// @grant        none
// ==/UserScript==

/*
 * On a post's page, X opens a reply by leaving the page: the route
 * changes and you lose your place in the conversation. With this script a
 * reply opens in a dialog over the page instead, in X's own look:
 *
 *  - Click a reply (or "Show N replies" under it): the reply and its
 *    replies open in the dialog. The page, its route and scroll stay.
 *  - Click a reply in the dialog to go a level deeper; ← goes back up
 *    (with its scroll), Esc or ✕ closes. ⌘/Ctrl-click opens normally.
 *  - "Open post" leaves the page for the post, when you want to.
 *
 * Data: X loads a post with the GraphQL call "TweetDetail". The script
 * keeps the page's own request (its headers, signature included) and asks
 * again with another post's id, with the page's sign-in. It never changes
 * the page's route. Nothing leaves x.com.
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
      return { type: m.type, url: m.media_url_https, video: m.type === 'photo' ? '' : bestVideo(m), tco: m.url,
        w: (m.original_info && m.original_info.width) || 0, h: (m.original_info && m.original_info.height) || 0 };
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
      reposts: (L.retweet_count || 0) + (L.quote_count || 0),
      views: r.views && r.views.count ? +r.views.count : 0,
      media: media,
      replyTo: L.in_reply_to_status_id_str || '',
      replyToHandle: L.in_reply_to_screen_name || '',
      quoted: null
    };
    if ((depth || 0) < 1 && r.quoted_status_result) t.quoted = normTweet(r.quoted_status_result.result, 1);
    return t;
  }

  function itemTweet(itemContent) {
    if (!itemContent || itemContent.itemType !== 'TimelineTweet') return null;
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
    function cursorOf(c) { return (c && (c.value || (c.itemContent && c.itemContent.value))) || ''; }
    ins.forEach(function (i) {
      if (i.type === 'TimelineAddToModule') {
        var last = out.threads[out.threads.length - 1];
        (i.moduleItems || []).forEach(function (it) {
          var tw = itemTweet(it.item && it.item.itemContent);
          if (tw && last) last.push(tw);
        });
        return;
      }
      if (i.type !== 'TimelineAddEntries') return;
      (i.entries || []).forEach(function (e) {
        var id = e.entryId || '', c = e.content || {};
        if (id.indexOf('tweet-') === 0) {
          var tw = itemTweet(c.itemContent);
          if (!tw) return;
          if (tw.id === focalId) { out.focal = tw; seenFocal = true; }
          else if (!seenFocal) out.ancestors.push(tw);
          else out.threads.push([tw]);
        } else if (id.indexOf('conversationthread-') === 0) {
          var th = [];
          (c.items || []).forEach(function (it) {
            var tw = itemTweet(it.item && it.item.itemContent);
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
    post: '貼文', back: '返回', close: '關閉', open: '開啟貼文', showReplies: function (n) { return '顯示 ' + n + ' 則回覆'; },
    more: '顯示更多回覆', none: '還沒有回覆', replyingTo: '回覆給', failed: '無法載入這篇貼文。', openInstead: '改為開啟貼文',
    views: '次查看'
  } : {
    post: 'Post', back: 'Back', close: 'Close', open: 'Open post', showReplies: function (n) { return 'Show ' + n + (n === 1 ? ' reply' : ' replies'); },
    more: 'Show more replies', none: 'No replies yet', replyingTo: 'Replying to', failed: 'This post could not be loaded.', openInstead: 'Open the post instead',
    views: 'Views'
  };

  // ---- capture X's own TweetDetail calls ------------------------------------

  var Win = window;
  var origFetch = Win.fetch;
  var tpl = null;   // { url, headers } of the page's last TweetDetail request
  var cache = {};   // focalId -> parsed first page
  var waiters = {}; // key -> [fn]
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
      // The transaction id signs the method and path, which are the same
      // for every TweetDetail call: the page's own works for ours.
      if (lk === 'authorization' || lk.indexOf('x-twitter-') === 0 || lk === 'content-type' || lk === 'x-client-transaction-id') h[lk] = headers[k];
    });
    tpl = { url: new URL(url, location.href).href, headers: h };
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
    var bg = getComputedStyle(document.body).backgroundColor || '';
    var m = bg.match(/\d+/g) || [255, 255, 255];
    var r = +m[0], g = +m[1], b = +m[2];
    if (r < 10 && g < 10 && b < 10) return 'dark';
    if (r + g + b < 200) return 'dim';
    return 'light';
  }

  var CSS = '\
.xtv-veil{position:fixed;inset:0;z-index:2147483000;display:flex;align-items:center;justify-content:center;background:rgba(91,112,131,.4);animation:xtv-fade .12s ease-out}\
.xtv-veil.dark{background:rgba(91,112,131,.4)}\
@keyframes xtv-fade{from{opacity:0}to{opacity:1}}\
.xtv-dlg{--bg:#fff;--fg:#0f1419;--sub:#536471;--line:#eff3f4;--thread:#cfd9de;--hover:rgba(0,0,0,.03);--accent:#1d9bf0;--btnhover:rgba(15,20,25,.1);\
width:600px;max-width:100vw;height:min(90vh,800px);background:var(--bg);color:var(--fg);border-radius:16px;display:flex;flex-direction:column;overflow:hidden;\
font:15px/20px TwitterChirp,-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;box-shadow:rgba(101,119,134,.2) 0 0 15px,rgba(101,119,134,.15) 0 0 3px 1px}\
.xtv-dlg.dim{--bg:#15202b;--fg:#f7f9f9;--sub:#8b98a5;--line:#38444d;--thread:#38444d;--hover:rgba(255,255,255,.03);--btnhover:rgba(239,243,244,.1)}\
.xtv-dlg.dark{--bg:#000;--fg:#e7e9ea;--sub:#71767b;--line:#2f3336;--thread:#333639;--hover:rgba(255,255,255,.03);--btnhover:rgba(239,243,244,.1);box-shadow:rgba(255,255,255,.2) 0 0 15px,rgba(255,255,255,.15) 0 0 3px 1px}\
@media (max-width:700px){.xtv-dlg{height:100vh;border-radius:0}}\
.xtv-dlg a{color:var(--accent);text-decoration:none}.xtv-dlg a:hover{text-decoration:underline}\
.xtv-top{display:flex;align-items:center;gap:12px;height:53px;padding:0 16px;flex:none;backdrop-filter:blur(12px)}\
.xtv-top h2{flex:1;margin:0;font-size:20px;line-height:24px;font-weight:700;min-width:0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}\
.xtv-ib{appearance:none;border:0;background:transparent;color:var(--fg);width:34px;height:34px;border-radius:50%;display:grid;place-items:center;cursor:pointer;flex:none;transition:background .2s}\
.xtv-ib:hover{background:var(--btnhover)}\
.xtv-ib svg{width:20px;height:20px}\
.xtv-crumb{display:flex;gap:4px;align-items:center;color:var(--sub);font-size:13px;overflow:hidden;white-space:nowrap}\
.xtv-scroll{flex:1;overflow-y:auto;overscroll-behavior:contain}\
.xtv-level{animation:xtv-in .18s cubic-bezier(.2,.8,.2,1)}\
.xtv-level.back{animation:xtv-out .18s cubic-bezier(.2,.8,.2,1)}\
@keyframes xtv-in{from{transform:translateX(24px);opacity:.4}to{transform:none;opacity:1}}\
@keyframes xtv-out{from{transform:translateX(-24px);opacity:.4}to{transform:none;opacity:1}}\
.xtv-row{display:flex;gap:12px;padding:12px 16px 0;cursor:pointer;transition:background .2s}\
.xtv-row:hover{background:var(--hover)}\
.xtv-rail{display:flex;flex-direction:column;align-items:center;flex:none;width:40px}\
.xtv-rail .up{width:2px;height:8px;margin:-12px 0 4px;background:var(--thread)}\
.xtv-rail .up.none{background:transparent}\
.xtv-rail .down{width:2px;flex:1;min-height:12px;margin-top:4px;background:var(--thread)}\
.xtv-av{width:40px;height:40px;border-radius:50%;object-fit:cover;background:var(--line);flex:none}\
.xtv-main{flex:1;min-width:0;padding-bottom:12px}\
.xtv-row.last .xtv-main{border-bottom:1px solid var(--line)}\
.xtv-who{display:flex;gap:4px;align-items:baseline;min-width:0;white-space:nowrap}\
.xtv-who b{font-weight:700;overflow:hidden;text-overflow:ellipsis;color:var(--fg)}\
.xtv-who .sub{color:var(--sub);overflow:hidden;text-overflow:ellipsis}\
.xtv-badge{width:18px;height:18px;align-self:center;color:var(--accent);flex:none}\
.xtv-text{margin-top:2px;white-space:normal;overflow-wrap:anywhere}\
.xtv-media{display:grid;gap:2px;margin-top:12px;border-radius:16px;overflow:hidden;border:1px solid var(--line)}\
.xtv-media.n2,.xtv-media.n3,.xtv-media.n4{grid-template-columns:1fr 1fr}\
.xtv-media img,.xtv-media video{width:100%;max-height:510px;object-fit:cover;display:block;background:var(--line)}\
.xtv-media.n2 img,.xtv-media.n3 img,.xtv-media.n4 img{aspect-ratio:1;height:100%}\
.xtv-quote{margin-top:12px;border:1px solid var(--line);border-radius:16px;padding:12px;transition:background .2s}\
.xtv-quote:hover{background:var(--hover)}\
.xtv-quote .xtv-av{width:20px;height:20px}\
.xtv-acts{display:flex;justify-content:space-between;max-width:425px;margin-top:12px;color:var(--sub);font-size:13px}\
.xtv-act{display:flex;align-items:center;gap:4px;min-width:0}\
.xtv-act svg{width:18.75px;height:18.75px}\
.xtv-focal{padding:12px 16px 0;border-bottom:1px solid var(--line)}\
.xtv-focal .xtv-head{display:flex;gap:12px;align-items:center;margin-bottom:12px}\
.xtv-focal .xtv-text{font-size:17px;line-height:24px}\
.xtv-focal .xtv-when{color:var(--sub);margin:16px 0;font-size:15px}\
.xtv-focal .xtv-when b{color:var(--fg);font-weight:700}\
.xtv-focal .xtv-acts{border-top:1px solid var(--line);margin:0;max-width:none;padding:4px 4px;height:48px;align-items:center}\
.xtv-reply-to{color:var(--sub);margin:4px 0 0}\
.xtv-parent{opacity:.95}\
.xtv-more{display:block;width:100%;appearance:none;border:0;border-bottom:1px solid var(--line);background:none;color:var(--accent);font:inherit;padding:16px;text-align:left;cursor:pointer;transition:background .2s}\
.xtv-more:hover{background:var(--hover)}\
.xtv-msg{color:var(--sub);text-align:center;padding:40px 16px}\
.xtv-msg a{display:inline-block;margin-top:12px}\
.xtv-spin{width:26px;height:26px;margin:40px auto;border-radius:50%;border:3px solid rgba(29,155,240,.2);border-top-color:var(--accent);animation:xtv-rot .75s linear infinite}\
@keyframes xtv-rot{to{transform:rotate(360deg)}}\
.xtv-show{display:inline-block;margin:0 0 12px 68px;color:#1d9bf0;font-size:15px;cursor:pointer}\
.xtv-show:hover{text-decoration:underline}';

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

  // Simple line icons in X's weight (not X's own drawings).
  var ICON = {
    back: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5"/><path d="M11 18l-6-6 6-6"/></svg>',
    close: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg>',
    open: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 4h6v6"/><path d="M20 4l-9 9"/><path d="M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/></svg>',
    reply: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linejoin="round"><path d="M4 11.5C4 7.4 7.4 4.5 11.5 4.5h1C16.6 4.5 20 7.4 20 11.3c0 4-3.4 6.7-7.6 6.7H11l-4.5 3v-3.5C4.9 16.4 4 14.1 4 11.5z"/></svg>',
    repost: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="M7 20V8a2 2 0 0 1 2-2h7"/><path d="M4 17l3 3 3-3"/><path d="M17 4v12a2 2 0 0 1-2 2H8"/><path d="M20 7l-3-3-3 3"/></svg>',
    like: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linejoin="round"><path d="M12 20s-7.5-4.6-7.5-10.2A4.3 4.3 0 0 1 12 7.2a4.3 4.3 0 0 1 7.5 2.6C19.5 15.4 12 20 12 20z"/></svg>',
    views: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round"><path d="M5 20V12M10 20V4M15 20v-9M20 20v-5"/></svg>',
    badge: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2.5l2.4 1.8 3-.2.9 2.9 2.5 1.7-1 2.8 1 2.8-2.5 1.7-.9 2.9-3-.2L12 21.5l-2.4-1.8-3 .2-.9-2.9-2.5-1.7 1-2.8-1-2.8 2.5-1.7.9-2.9 3 .2z"/><path d="M8.5 12.2l2.3 2.3 4.7-4.9" fill="none" stroke="#fff" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>'
  };

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

  function mediaEl(t) {
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
        var a = el('a');
        a.href = m.url + '?name=orig'; a.target = '_blank'; a.rel = 'noopener';
        a.addEventListener('click', function (e) { e.stopPropagation(); });
        var img = document.createElement('img');
        img.src = sized(m.url, n > 1 ? 'small' : 'medium'); img.loading = 'lazy'; img.alt = '';
        a.appendChild(img);
        box.appendChild(a);
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
    w.appendChild(name);
    if (t.verified) w.appendChild(el('span', 'xtv-badge', ICON.badge));
    w.appendChild(el('span', 'sub', '@' + esc(t.handle)));
    if (!long && t.created) {
      w.appendChild(el('span', 'sub', '·'));
      var tm = el('span', 'sub', esc(ago(t.created)));
      tm.title = t.created.toLocaleString();
      w.appendChild(tm);
    }
    return w;
  }

  function actsEl(t, big) {
    var a = el('div', 'xtv-acts');
    [['reply', t.replies], ['repost', t.reposts], ['like', t.likes], ['views', t.views]].forEach(function (p) {
      if (big && p[0] === 'views') return;
      a.appendChild(el('div', 'xtv-act', ICON[p[0]] + '<span>' + num(p[1]) + '</span>'));
    });
    return a;
  }

  function quoteEl(q, go) {
    var box = el('div', 'xtv-quote');
    var head = el('div', 'xtv-who');
    var img = document.createElement('img');
    img.className = 'xtv-av'; img.alt = ''; img.src = q.avatar || '';
    head.appendChild(img);
    head.appendChild(el('b', '', esc(q.name || q.handle)));
    head.appendChild(el('span', 'sub', '@' + esc(q.handle) + ' · ' + esc(ago(q.created))));
    box.appendChild(head);
    box.appendChild(el('div', 'xtv-text', renderText(q)));
    var m = mediaEl(q);
    if (m) box.appendChild(m);
    box.addEventListener('click', function (e) { e.stopPropagation(); if (!e.target.closest('a,video')) go(q.id); });
    return box;
  }

  // rowEl is a post in a list, X's way: avatar rail on the left, joined
  // to the posts above and below in the same thread.
  function rowEl(t, go, up, down, last) {
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
    var m = mediaEl(t);
    if (m) main.appendChild(m);
    if (t.quoted) main.appendChild(quoteEl(t.quoted, go));
    main.appendChild(actsEl(t, false));
    row.appendChild(main);
    row.addEventListener('click', function (e) {
      if (e.target.closest('a,video,button')) return;
      if (e.metaKey || e.ctrlKey) { window.open('/' + t.handle + '/status/' + t.id, '_blank'); return; }
      go(t.id);
    });
    row.addEventListener('keydown', function (e) { if (e.key === 'Enter') go(t.id); });
    return row;
  }

  // focalEl is the post a level is about, large, as X shows a post's page.
  function focalEl(t, go) {
    var box = el('div', 'xtv-focal');
    var head = el('div', 'xtv-head');
    head.appendChild(avatarEl(t));
    var who = el('div', '');
    who.style.minWidth = '0';
    var w = whoEl(t, true);
    w.style.flexDirection = 'column'; w.style.gap = '0'; w.style.alignItems = 'flex-start';
    who.appendChild(w);
    head.appendChild(who);
    box.appendChild(head);
    if (t.replyToHandle) box.appendChild(el('div', 'xtv-reply-to', esc(W_.replyingTo) + ' <a href="/' + esc(t.replyToHandle) + '">@' + esc(t.replyToHandle) + '</a>'));
    box.appendChild(el('div', 'xtv-text', renderText(t)));
    var m = mediaEl(t);
    if (m) box.appendChild(m);
    if (t.quoted) box.appendChild(quoteEl(t.quoted, go));
    if (t.created) {
      var when = t.created.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' }) + ' · ' +
        t.created.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
      box.appendChild(el('div', 'xtv-when', esc(when) + (t.views ? ' · <b>' + esc(num(t.views)) + '</b> ' + esc(W_.views) : '')));
    }
    box.appendChild(actsEl(t, true));
    return box;
  }

  // ---- the dialog: a stack of levels, never a route change ----------------------

  var dlg = null; // { veil, box, scroll, title, backBtn, stack: [{id, y}] }

  function closeDialog() {
    if (!dlg) return;
    dlg.veil.remove();
    document.documentElement.style.overflow = dlg.overflow;
    dlg = null;
  }

  function openDialog(id) {
    ensureStyle();
    if (dlg) { push(id); return; }
    var th = theme();
    var veil = el('div', 'xtv-veil ' + th);
    var box = el('div', 'xtv-dlg ' + th);
    box.setAttribute('role', 'dialog');
    box.setAttribute('aria-modal', 'true');
    var top = el('div', 'xtv-top');
    var back = el('button', 'xtv-ib', ICON.close);
    back.type = 'button';
    back.title = W_.close;
    back.addEventListener('click', function () { if (dlg && dlg.stack.length > 1) pop(); else closeDialog(); });
    var title = el('h2', '', esc(W_.post));
    var open = el('a', 'xtv-ib', ICON.open);
    open.title = W_.open;
    open.addEventListener('click', function () { closeDialog(); });
    top.appendChild(back); top.appendChild(title); top.appendChild(open);
    var scroll = el('div', 'xtv-scroll');
    box.appendChild(top); box.appendChild(scroll);
    veil.appendChild(box);
    veil.addEventListener('mousedown', function (e) { if (e.target === veil) closeDialog(); });
    document.body.appendChild(veil);
    dlg = { veil: veil, box: box, scroll: scroll, title: title, back: back, open: open, stack: [], overflow: document.documentElement.style.overflow };
    document.documentElement.style.overflow = 'hidden';
    push(id);
  }

  function push(id) {
    if (!dlg) return;
    var cur = dlg.stack[dlg.stack.length - 1];
    if (cur && cur.id === id) return;
    if (cur) cur.y = dlg.scroll.scrollTop;
    dlg.stack.push({ id: id, y: 0 });
    show(false);
  }
  function pop() {
    if (!dlg || dlg.stack.length < 2) return;
    dlg.stack.pop();
    show(true);
  }

  function show(goingBack) {
    var level = dlg.stack[dlg.stack.length - 1];
    var depth = dlg.stack.length;
    dlg.back.innerHTML = depth > 1 ? ICON.back : ICON.close;
    dlg.back.title = depth > 1 ? W_.back : W_.close;
    dlg.open.href = '/i/status/' + level.id;
    var view = el('div', 'xtv-level' + (goingBack ? ' back' : ''));
    view.appendChild(el('div', 'xtv-spin'));
    dlg.scroll.innerHTML = '';
    dlg.scroll.appendChild(view);
    var mine = dlg;
    load(level.id).then(function (d) {
      if (dlg !== mine || dlg.stack[dlg.stack.length - 1] !== level) return;
      view.innerHTML = '';
      if (!d.focal) { failed(view, level.id); return; }
      // The post it answers, small and joined, as X shows it above.
      var parent = d.ancestors[d.ancestors.length - 1];
      if (parent) {
        var p = rowEl(parent, push, false, true, false);
        p.classList.add('xtv-parent');
        view.appendChild(p);
      }
      view.appendChild(focalEl(d.focal, push));
      renderThreads(view, d.threads);
      moreButton(view, level.id, d.cursor);
      if (!d.threads.length && !d.cursor) view.appendChild(el('div', 'xtv-msg', esc(W_.none)));
      dlg.scroll.scrollTop = goingBack ? level.y : 0;
    }, function () {
      if (dlg !== mine) return;
      view.innerHTML = '';
      failed(view, level.id);
    });
  }

  function renderThreads(view, threads) {
    threads.forEach(function (th) {
      th.forEach(function (t, i) {
        view.appendChild(rowEl(t, push, i > 0, i < th.length - 1, i === th.length - 1));
      });
    });
  }

  function moreButton(view, id, cursor) {
    if (!cursor) return;
    var b = el('button', 'xtv-more', esc(W_.more));
    b.type = 'button';
    b.addEventListener('click', function () {
      b.disabled = true;
      b.innerHTML = '<div class="xtv-spin" style="margin:0 auto;width:20px;height:20px"></div>';
      fetchDetail(id, cursor).then(function (p) {
        var frag = el('div');
        renderThreads(frag, p.threads);
        while (frag.firstChild) view.insertBefore(frag.firstChild, b);
        b.remove();
        if (p.cursor && p.cursor !== cursor) moreButton(view, id, p.cursor);
      }, function () { b.remove(); });
    });
    view.appendChild(b);
  }

  function failed(view, id) {
    var m = el('div', 'xtv-msg', esc(W_.failed) + '<br>');
    var a = el('a', '', esc(W_.openInstead));
    a.href = '/i/status/' + id;
    a.addEventListener('click', function () { closeDialog(); });
    m.appendChild(a);
    view.appendChild(m);
  }

  document.addEventListener('keydown', function (e) {
    if (!dlg) return;
    if (e.key === 'Escape') { e.preventDefault(); e.stopImmediatePropagation(); closeDialog(); }
    else if ((e.key === 'Backspace' || (e.key === 'ArrowLeft' && e.altKey)) && dlg.stack.length > 1 &&
      !/^(INPUT|TEXTAREA)$/.test((e.target && e.target.tagName) || '')) { e.preventDefault(); e.stopImmediatePropagation(); pop(); }
  }, true);

  // ---- on X's page: replies open in the dialog --------------------------------

  var STATUS = /^\/(?:[^/]+|i\/web|i)\/status\/(\d+)/;
  function pageStatus() {
    var m = location.pathname.match(STATUS);
    return m ? m[1] : '';
  }
  function articleStatus(art) {
    var t = art.querySelector('a[href*="/status/"] time');
    var a = t && t.closest('a');
    return a ? statusIdOf(a.getAttribute('href')) : '';
  }

  // A click on a reply in the conversation opens it in place. Links, the
  // action bar, media and menus inside it keep doing what X does.
  document.addEventListener('click', function (e) {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    var page = pageStatus();
    if (!page) return;
    var art = e.target.closest && e.target.closest('article[data-testid="tweet"]');
    if (!art || (dlg && dlg.veil.contains(art))) return;
    if (e.target.closest('a,button,[role="button"],[role="menu"],video,[data-testid="tweetPhoto"],[data-testid="card.wrapper"],input,textarea')) return;
    var id = articleStatus(art);
    if (!id || id === page) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    openDialog(id);
  }, true);

  // "Show N replies" under a reply that has replies, in X's blue.
  function decorate() {
    var page = pageStatus();
    if (!page) return;
    var arts = document.querySelectorAll('article[data-testid="tweet"]:not([data-xtv])');
    for (var i = 0; i < arts.length; i++) {
      var art = arts[i];
      art.setAttribute('data-xtv', '1');
      var id = articleStatus(art);
      if (!id || id === page) continue;
      var rb = art.querySelector('[data-testid="reply"]');
      var n = parseInt(((rb && rb.getAttribute('aria-label')) || '').replace(/[^\d]/g, ''), 10) || 0;
      if (!n) continue;
      ensureStyle();
      var link = el('div', 'xtv-show', esc(W_.showReplies(n)));
      link.setAttribute('role', 'link');
      link.tabIndex = 0;
      (function (id) {
        link.addEventListener('click', function (e) { e.preventDefault(); e.stopPropagation(); openDialog(id); });
      })(id);
      art.appendChild(link);
    }
  }
  var pending = 0;
  new MutationObserver(function () {
    if (pending) return;
    pending = requestAnimationFrame(function () { pending = 0; decorate(); });
  }).observe(document.documentElement, { childList: true, subtree: true });
  window.addEventListener('popstate', closeDialog);
})();
