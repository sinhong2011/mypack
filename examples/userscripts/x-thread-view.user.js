// ==UserScript==
// @name         X Thread View
// @namespace    https://mypack.dev/userscripts
// @version      0.2.0
// @description  Read an X conversation as a forum thread: floors in time order, quotes of the post each one answers, and each post's replies in a panel.
// @match        https://x.com/*
// @match        https://twitter.com/*
// @run-at       document-start
// @grant        none
// ==/UserScript==

/*
 * A forum reading of an X conversation, in the way Hong Kong forums
 * present a thread:
 *
 *  - Every post of the conversation is a numbered floor, in time order,
 *    from the conversation's first post (also when you open a reply).
 *  - A floor that answers another floor quotes it at its top, and a quote
 *    of a quote nests inside it (three levels), images shrunk.
 *  - 💬 N under a floor opens a panel: the floor, "Quoted by", the replies
 *    to it (without repeating the quote), each with "Go to reply", and an
 *    end mark. Panels stack; Esc closes the top one.
 *  - 👁 shows only one author's floors; ↩ opens X's reply box.
 *  - Images open in a viewer of every image of the thread.
 *
 * It opens by itself on a post's page (Auto-open in its bar turns that
 * off; the ☰ Thread button opens it by hand).
 *
 * Data: X's web app loads a post with the GraphQL call "TweetDetail". The
 * script reads those answers as the page gets them, and asks again, with
 * the page's own request and sign-in, for another post or the next page.
 * If X refuses a repeated request, X's own router opens the post behind the
 * view and the script reads that answer. Nothing leaves x.com.
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
      conv: L.conversation_id_str || '',
      quoted: null
    };
    if ((depth || 0) < 1 && r.quoted_status_result) t.quoted = normTweet(r.quoted_status_result.result, 1);
    return t;
  }

  function itemTweet(itemContent) {
    if (!itemContent || itemContent.itemType !== 'TimelineTweet') return null;
    return normTweet(itemContent.tweet_results && itemContent.tweet_results.result);
  }

  // parseDetail reads a TweetDetail answer: { focal, ancestors[], items[],
  // cursor }. items are the other posts, each on its own (a reply chain's
  // posts are separate floors, linked by replyTo).
  function parseDetail(json, focalId) {
    var out = { focal: null, ancestors: [], items: [], cursor: '' };
    var conv = json && json.data && (json.data.threaded_conversation_with_injections_v2 || json.data.threaded_conversation_with_injections);
    var ins = (conv && conv.instructions) || [];
    var seenFocal = false;
    function cursorOf(c) { return (c && (c.value || (c.itemContent && c.itemContent.value))) || ''; }
    function push(tw) { if (tw) out.items.push(tw); }
    ins.forEach(function (i) {
      if (i.type === 'TimelineAddToModule') {
        (i.moduleItems || []).forEach(function (it) { push(itemTweet(it.item && it.item.itemContent)); });
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
          else push(tw);
        } else if (id.indexOf('conversationthread-') === 0) {
          (c.items || []).forEach(function (it) { push(itemTweet(it.item && it.item.itemContent)); });
        } else if (id.indexOf('cursor-bottom-') === 0) {
          out.cursor = cursorOf(c);
        }
      });
    });
    if (!out.focal && out.ancestors.length) out.focal = out.ancestors.pop();
    return out;
  }

  // Thread is the posts of one conversation, by id.
  function Thread(rootId) {
    this.rootId = rootId;
    this.by = {};
  }
  Thread.prototype.add = function (list) {
    var self = this, n = 0;
    (list || []).forEach(function (t) {
      if (t && !self.by[t.id]) { self.by[t.id] = t; n++; }
    });
    return n;
  };
  Thread.prototype.root = function () { return this.by[this.rootId] || null; };
  // floors: the root first, then every other post in time order.
  Thread.prototype.floors = function () {
    var root = this.root(), self = this;
    var rest = Object.keys(this.by).map(function (k) { return self.by[k]; }).filter(function (t) { return !root || t.id !== root.id; });
    rest.sort(function (a, b) { return ((a.created && a.created.getTime()) || 0) - ((b.created && b.created.getTime()) || 0) || (a.id < b.id ? -1 : 1); });
    return root ? [root].concat(rest) : rest;
  };
  // quotes: the posts t answers, nearest first, up to depth levels; a
  // direct answer to the root quotes nothing.
  Thread.prototype.quotes = function (t, depth) {
    var out = [], cur = t;
    while (out.length < (depth || 3) && cur && cur.replyTo && cur.replyTo !== this.rootId) {
      var p = this.by[cur.replyTo];
      if (!p) break;
      out.push(p);
      cur = p;
    }
    return out;
  };
  Thread.prototype.children = function (id) {
    var self = this;
    return this.floors().filter(function (t) { return t.replyTo === id && t.id !== id && self.by[t.id]; });
  };

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
    if (typeof module === 'object' && module.exports) {
      module.exports = { parseDetail: parseDetail, renderText: renderText, normTweet: normTweet, Thread: Thread };
    }
    return;
  }

  // ---- words -----------------------------------------------------------------

  var zh = /^zh|^yue/i.test(navigator.language || '');
  var W_ = zh ? {
    thread: '☰ 討論串', title: '討論串', open: '在 X 開啟', auto: '自動開啟', images: '只看圖片',
    quotedBy: '被引用回覆', goTo: '前往回覆', end: '完', loading: '載入中…', more: '載入更多回覆',
    onlyOf: function (n) { return '只看 ' + n + ' 的回覆'; }, showAll: '顯示全部', reply: '回覆',
    only: '只看此人', none: '未有回覆', noTemplate: '請先等 X 載入這篇貼文，或重新整理頁面。',
    failed: '未能載入：', unavailable: '這篇貼文已無法查看。', goToPost: '前往留言', floors: function (n) { return '共 ' + n + ' 樓'; },
    close: '關閉 (Esc)', earlier: '由第一篇開始'
  } : {
    thread: '☰ Thread', title: 'Thread', open: 'Open on X', auto: 'Auto-open', images: 'Images only',
    quotedBy: 'Quoted by', goTo: 'Go to reply', end: 'End', loading: 'Loading…', more: 'Load more replies',
    onlyOf: function (n) { return 'Only ' + n + '’s posts'; }, showAll: 'Show all', reply: 'Reply',
    only: 'Only this author', none: 'No replies yet', noTemplate: 'Wait for X to load this post, or reload the page.',
    failed: 'Could not load: ', unavailable: 'This post is not available.', goToPost: 'Go to post', floors: function (n) { return n + ' floors'; },
    close: 'Close (Esc)', earlier: 'From the first post'
  };

  // ---- capture X's own TweetDetail calls ------------------------------------

  var Win = window;
  var origFetch = Win.fetch;
  var tpl = null;      // { url, headers } of the last TweetDetail request
  var cache = {};      // focalId -> parsed first page
  var waiters = {};    // focalId -> [fn]
  var routing = false; // X's router is loading a post for us
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
      if (lk === 'authorization' || lk.indexOf('x-twitter-') === 0 || lk === 'content-type') h[lk] = headers[k];
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

  // replay asks TweetDetail again for focalId (and a page cursor).
  function replay(focalId, cursor) {
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
      return origFetch.call(Win, u.href, { headers: h, credentials: 'include' });
    }).then(function (r) {
      if (!r.ok) throw new Error('HTTP ' + r.status);
      return r.json();
    }).then(function (j) {
      if (j && j.errors && !j.data) throw new Error((j.errors[0] && j.errors[0].message) || 'error');
      return parseDetail(j, focalId);
    });
  }

  // viaRouter lets X itself load the post (behind the view), then goes back.
  function viaRouter(focalId) {
    var back = location.href;
    routing = true;
    var p = wait(focalId, 15000);
    history.pushState({}, '', '/i/status/' + focalId);
    Win.dispatchEvent(new PopStateEvent('popstate', { state: {} }));
    var done = function () { routing = false; if (location.href !== back) history.back(); };
    p.then(done, done);
    return p;
  }

  // load is the first page of focalId: what the page already loaded, else
  // what X loads in a moment, else a request of our own.
  function load(focalId) {
    if (cache[focalId]) return Promise.resolve(cache[focalId]);
    // X loads the page's own post itself: give it a moment first.
    var soon = focalId === statusId() ? wait(focalId, 2500) : Promise.reject(new Error('not the page'));
    return soon.catch(function () {
      return replay(focalId).then(function (d) { cache[focalId] = d; return d; }, function (e) {
        console.info('[x-thread-view] replay failed (' + e.message + '); loading through X');
        return viaRouter(focalId);
      });
    });
  }

  // ---- UI --------------------------------------------------------------------

  var CSS = '\
.xtv,.xtv-panel{--bg:#151515;--card:#1e1e1e;--band:#101010;--line:#2a2a2a;--fg:#e6e6e6;--mute:#8d8d8d;--dim:#6b6b6b;--name:#4aa8ff;--op:#f5c542;--accent:#f5c542;--chip:#2a2a2a;\
color:var(--fg);font:15px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang HK","Microsoft JhengHei",sans-serif}\
.xtv{position:fixed;inset:0;z-index:2147483000;background:var(--bg);display:flex;flex-direction:column;animation:xtv-in .16s ease-out}\
@keyframes xtv-in{from{opacity:0}to{opacity:1}}\
.xtv a,.xtv-panel a{color:var(--name);text-decoration:none}.xtv a:hover,.xtv-panel a:hover{text-decoration:underline}\
.xtv-bar{display:flex;align-items:center;gap:6px;padding:0 10px 0 14px;height:52px;background:#1a1a1a;border-bottom:1px solid var(--line);flex:none}\
.xtv-ttl{flex:1;min-width:0;font-weight:650;color:var(--accent);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;font-size:16px}\
.xtv-ib{appearance:none;border:0;background:none;color:var(--mute);font:inherit;font-size:13px;height:34px;min-width:34px;padding:0 9px;border-radius:8px;cursor:pointer;display:inline-flex;align-items:center;gap:6px}\
.xtv-ib:hover{background:#262626;color:var(--fg)}.xtv-ib.on{color:var(--accent)}\
.xtv-ib .sw{width:26px;height:15px;border-radius:8px;background:#3a3a3a;position:relative;transition:background .15s}\
.xtv-ib .sw:after{content:"";position:absolute;top:2px;left:2px;width:11px;height:11px;border-radius:50%;background:#bbb;transition:transform .15s}\
.xtv-ib.on .sw{background:var(--accent)}.xtv-ib.on .sw:after{transform:translateX(11px);background:#151515}\
.xtv-sub{display:flex;justify-content:center;gap:12px;align-items:center;height:40px;color:var(--accent);font-size:14px;background:var(--bg);border-bottom:6px solid var(--band);flex:none}\
.xtv-sub .mute{color:var(--mute)}\
.xtv-filter{display:flex;justify-content:center;gap:10px;align-items:center;padding:8px;background:#1f1a0b;color:var(--accent);font-size:13px}\
.xtv-scroll{flex:1;overflow-y:auto;overscroll-behavior:contain}\
.xtv-list{max-width:780px;margin:0 auto}\
.xtv-floor{background:var(--card);padding:14px 20px 12px;border-bottom:8px solid var(--band);transition:background .6s}\
.xtv-floor.flash{background:#2b2715}\
.xtv-head{display:flex;align-items:center;gap:8px;font-size:13px;color:var(--mute);margin-bottom:8px}\
.xtv-no{color:var(--dim);font-variant-numeric:tabular-nums}.xtv-op .xtv-no{color:var(--op)}\
.xtv-name{color:var(--name)!important;font-weight:500;max-width:220px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}\
.xtv-op .xtv-name{color:var(--op)!important}\
.xtv-dot{color:var(--dim)}.xtv-time{color:var(--mute)!important}\
.xtv-head .xtv-ib{height:26px;min-width:26px;padding:0 5px;font-size:14px}\
.xtv-head .sp{flex:1}\
.xtv-q{border-left:3px solid #4a4a4a;padding:2px 0 2px 12px;margin:0 0 10px;color:var(--mute);font-size:14px}\
.xtv-q .xtv-q{margin-top:0}\
.xtv-q .xtv-qby{font-size:12px;color:var(--dim);margin-bottom:2px}\
.xtv-q .xtv-media img,.xtv-q .xtv-media video{max-height:96px;width:auto;max-width:140px}\
.xtv-body{word-wrap:break-word;overflow-wrap:anywhere}\
.xtv-media{display:flex;flex-wrap:wrap;gap:6px;margin-top:8px}\
.xtv-media img,.xtv-media video{max-width:min(100%,520px);max-height:420px;border-radius:6px;background:#000;cursor:zoom-in;display:block}\
.xtv-quote{margin-top:10px;border:1px solid var(--line);border-radius:8px;padding:8px 12px;font-size:14px;background:#191919;cursor:pointer}\
.xtv-foot{display:flex;gap:8px;align-items:center;margin-top:10px}\
.xtv-pill{display:inline-flex;gap:12px;align-items:center;height:30px;padding:0 12px;border-radius:6px;background:var(--chip);color:var(--mute);font-size:13px}\
.xtv-cbtn{appearance:none;border:0;display:inline-flex;gap:7px;align-items:center;height:30px;padding:0 12px;border-radius:6px;background:var(--chip);color:var(--fg);font:inherit;font-size:13px;cursor:pointer}\
.xtv-cbtn:hover{background:#353535}\
.xtv-goto{margin-left:auto;color:var(--mute)!important;font-size:13px;cursor:pointer;background:none;border:0;font:inherit}\
.xtv-goto:hover{color:var(--fg)!important}\
.xtv-end,.xtv-div{text-align:center;color:var(--accent);padding:18px 0;font-size:15px}\
.xtv-div{background:var(--band);padding:12px 0}\
.xtv-msg{text-align:center;color:var(--mute);padding:48px 16px}\
.xtv-more{display:block;margin:16px auto;appearance:none;border:1px solid var(--line);background:none;color:var(--fg);border-radius:8px;padding:8px 16px;font:inherit;font-size:14px;cursor:pointer}\
.xtv-ctr{position:fixed;right:24px;bottom:24px;z-index:2147483001;min-width:56px;height:56px;padding:0 12px;border-radius:28px;background:#2a2a2a;color:var(--fg);display:flex;align-items:center;justify-content:center;font:600 15px -apple-system,sans-serif;box-shadow:0 4px 18px rgba(0,0,0,.5);cursor:pointer;font-variant-numeric:tabular-nums}\
.xtv-veil{position:fixed;inset:0;z-index:2147483002;background:rgba(0,0,0,.62);display:flex;justify-content:center;animation:xtv-in .14s ease-out}\
.xtv-panel{width:min(760px,100%);height:100%;background:var(--bg);display:flex;flex-direction:column;box-shadow:0 0 60px rgba(0,0,0,.6);animation:xtv-up .2s cubic-bezier(.2,.8,.2,1)}\
@keyframes xtv-up{from{transform:translateY(18px);opacity:.3}to{transform:none;opacity:1}}\
.xtv-panel .xtv-bar{justify-content:flex-end}\
.xtv-light{position:fixed;inset:0;z-index:2147483005;background:rgba(0,0,0,.94);display:flex;flex-direction:column;color:#ddd;font:14px -apple-system,sans-serif}\
.xtv-light .top{display:flex;justify-content:space-between;padding:12px 16px}\
.xtv-light .mid{flex:1;display:flex;align-items:center;justify-content:center;min-height:0;gap:8px}\
.xtv-light img{max-width:calc(100vw - 140px);max-height:calc(100vh - 120px)}\
.xtv-light .nav{appearance:none;border:0;background:none;color:#ddd;font-size:30px;width:56px;height:56px;cursor:pointer}\
.xtv-light .bot{display:flex;justify-content:flex-end;padding:12px 16px;gap:16px}\
.xtv-light button.lk{appearance:none;border:0;background:none;color:var(--accent,#f5c542);font:inherit;cursor:pointer}\
.xtv-fab{position:fixed;right:96px;bottom:24px;z-index:2147482999;background:#f5c542;color:#151515;border:0;border-radius:22px;padding:10px 16px;font:600 14px -apple-system,BlinkMacSystemFont,sans-serif;box-shadow:0 4px 16px rgba(0,0,0,.35);cursor:pointer}\
@media (max-width:640px){.xtv-floor{padding:12px 14px}.xtv-ctr{right:14px;bottom:14px}.xtv-fab{right:14px}}';

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
  function btn(cls, html, title, fn) {
    var b = el('button', cls, html);
    b.type = 'button';
    if (title) b.title = title;
    if (fn) b.addEventListener('click', function (e) { e.stopPropagation(); fn(e); });
    return b;
  }
  function ago(d) {
    if (!d) return '';
    var s = (Date.now() - d.getTime()) / 1000;
    if (zh) {
      if (s < 60) return '剛剛';
      if (s < 3600) return Math.floor(s / 60) + ' 分鐘前';
      if (s < 86400) return Math.floor(s / 3600) + ' 小時前';
      if (s < 86400 * 30) return Math.floor(s / 86400) + ' 日前';
      return d.toLocaleDateString();
    }
    if (s < 60) return 'just now';
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
  // sized asks X's image host for a size; other addresses stay as they are.
  function sized(u, name) { return /^https:\/\/pbs\.twimg\.com\//.test(u) ? u + (u.indexOf('?') < 0 ? '?' : '&') + 'name=' + name : u; }
  function pref(k, def) { try { var v = localStorage.getItem('xtv:' + k); return v == null ? def : v === '1'; } catch (e) { return def; } }
  function setPref(k, v) { try { localStorage.setItem('xtv:' + k, v ? '1' : '0'); } catch (e) {} }

  // ---- layers (view, panels, image viewer): Esc closes the top one ----------

  var layers = [];
  var savedOverflow = '';
  function pushLayer(l) {
    if (!layers.length) { savedOverflow = document.documentElement.style.overflow; document.documentElement.style.overflow = 'hidden'; }
    layers.push(l);
  }
  function closeTop() {
    var l = layers.pop();
    if (l) l.close();
    if (!layers.length) document.documentElement.style.overflow = savedOverflow;
  }
  function closeAll() { while (layers.length) closeTop(); }
  document.addEventListener('keydown', function (e) {
    if (!layers.length) return;
    var top = layers[layers.length - 1];
    if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); closeTop(); return; }
    if (top.key && top.key(e)) { e.preventDefault(); e.stopPropagation(); }
  }, true);

  // ---- the view --------------------------------------------------------------

  var view = null; // { thread, focalId, list, floorsEls, filter, imagesOnly, cursor, ... }

  function mediaEl(t, small) {
    if (!t.media.length) return null;
    var box = el('div', 'xtv-media');
    t.media.forEach(function (m) {
      if (m.video) {
        var v = document.createElement('video');
        v.src = m.video; v.poster = m.url; v.controls = !small; v.preload = 'none'; v.muted = m.type === 'animated_gif'; v.loop = v.muted;
        box.appendChild(v);
      } else {
        var img = document.createElement('img');
        img.src = sized(m.url, small ? 'thumb' : 'small'); img.loading = 'lazy'; img.alt = '';
        img.addEventListener('click', function (e) { e.stopPropagation(); openViewer(m.url, t.id); });
        box.appendChild(img);
      }
    });
    return box;
  }

  // quoteEl nests the posts t answers, the farthest innermost.
  function quoteEl(chain) {
    if (!chain.length) return null;
    var outer = null, inner = null;
    for (var i = 0; i < chain.length; i++) {
      var q = el('div', 'xtv-q');
      var p = chain[i];
      var body = el('div', 'xtv-body', renderText(p));
      var m = mediaEl(p, true);
      if (!outer) outer = q; else inner.insertBefore(q, inner.firstChild);
      q.appendChild(body);
      if (m) q.appendChild(m);
      inner = q;
    }
    return outer;
  }

  // floorEl is one floor. opts: { no, quote(bool), panel(bool) }
  function floorEl(t, opts) {
    var th = view.thread;
    var root = th.root();
    var op = root && t.handle === root.handle;
    var box = el('div', 'xtv-floor' + (op ? ' xtv-op' : ''));
    box.dataset.id = t.id;
    var head = el('div', 'xtv-head');
    head.appendChild(el('span', 'xtv-no', '#' + (opts.no || '?')));
    var nm = el('a', 'xtv-name', esc(t.name || t.handle)); nm.href = '/' + t.handle; nm.title = '@' + t.handle;
    head.appendChild(nm);
    head.appendChild(el('span', 'xtv-dot', '•'));
    var tm = el('a', 'xtv-time', esc(ago(t.created))); tm.href = '/' + t.handle + '/status/' + t.id; tm.title = t.created ? t.created.toLocaleString() : '';
    head.appendChild(tm);
    if (!opts.panel) head.appendChild(btn('xtv-ib', '👁', W_.only, function () { setFilter(t.handle, t.name || t.handle); }));
    head.appendChild(btn('xtv-ib', '↩', W_.reply, function () { closeAll(); location.href = '/intent/post?in_reply_to=' + t.id; }));
    head.appendChild(el('span', 'sp'));
    var ext = el('a', 'xtv-ib', '↗'); ext.href = '/' + t.handle + '/status/' + t.id; ext.title = W_.open;
    ext.addEventListener('click', function () { closeAll(); });
    head.appendChild(ext);
    box.appendChild(head);
    if (opts.quote) {
      var q = quoteEl(th.quotes(t, 3));
      if (q) box.appendChild(q);
    }
    box.appendChild(el('div', 'xtv-body', renderText(t)));
    var m = mediaEl(t, false);
    if (m) box.appendChild(m);
    if (t.quoted) {
      var qc = el('div', 'xtv-quote');
      qc.appendChild(el('div', 'xtv-head', '<span class="xtv-name">' + esc(t.quoted.name) + '</span><span class="xtv-dot">@' + esc(t.quoted.handle) + '</span>'));
      qc.appendChild(el('div', 'xtv-body', renderText(t.quoted)));
      var qm = mediaEl(t.quoted, true);
      if (qm) qc.appendChild(qm);
      qc.addEventListener('click', function (e) { if (!e.target.closest('a,img,video')) openPanel(t.quoted.id); });
      box.appendChild(qc);
    }
    var foot = el('div', 'xtv-foot');
    foot.appendChild(el('span', 'xtv-pill', '♥ ' + num(t.likes) + '<span>⟲ ' + num(t.reposts) + '</span>'));
    var kids = Math.max(th.children(t.id).length, t.replies);
    if (kids > 0 && !opts.source) foot.appendChild(btn('xtv-cbtn', '💬 ' + num(kids), W_.quotedBy, function () { openPanel(t.id); }));
    if (opts.panel && opts.inThread) foot.appendChild(btn('xtv-goto', '↘ ' + W_.goTo, '', function () { goToFloor(t.id); }));
    box.appendChild(foot);
    return box;
  }

  function setFilter(handle, name) {
    view.filter = view.filter && view.filter.handle === handle ? null : { handle: handle, name: name };
    renderFloors();
  }

  function renderFloors() {
    var v = view, list = v.list;
    list.innerHTML = '';
    v.filterBar.innerHTML = '';
    v.filterBar.style.display = v.filter ? '' : 'none';
    if (v.filter) {
      v.filterBar.appendChild(el('span', '', esc(W_.onlyOf(v.filter.name))));
      v.filterBar.appendChild(btn('xtv-ib on', W_.showAll, '', function () { v.filter = null; renderFloors(); }));
    }
    var floors = v.thread.floors();
    v.numbers = {};
    floors.forEach(function (t, i) { v.numbers[t.id] = i + 1; });
    v.sub.innerHTML = '<span>' + esc(W_.floors(floors.length)) + '</span>';
    var shown = floors.filter(function (t) {
      if (v.filter && t.handle !== v.filter.handle) return false;
      if (v.imagesOnly && !t.media.length) return false;
      return true;
    });
    shown.forEach(function (t) { list.appendChild(floorEl(t, { no: v.numbers[t.id], quote: true })); });
    if (v.cursor) {
      var more = btn('xtv-more', W_.more, '', function () { loadMore(more); });
      list.appendChild(more);
      v.sentinel = more;
      watchSentinel();
    } else {
      list.appendChild(el('div', 'xtv-end', esc(W_.end)));
    }
    updateCounter();
  }

  function loadMore(b) {
    var v = view;
    if (!v || v.loadingMore || !v.cursor) return;
    v.loadingMore = true;
    if (b) { b.disabled = true; b.textContent = W_.loading; }
    replay(v.focalId, v.cursor).then(function (p) {
      v.loadingMore = false;
      var added = v.thread.add(p.items);
      v.cursor = p.cursor && p.cursor !== v.cursor && (added || p.items.length) ? p.cursor : '';
      var y = v.scroll.scrollTop;
      renderFloors();
      v.scroll.scrollTop = y;
    }, function () {
      v.loadingMore = false;
      v.cursor = '';
      renderFloors();
    });
  }
  function watchSentinel() {
    var v = view;
    if (!('IntersectionObserver' in window) || !v.sentinel) return;
    if (v.io) v.io.disconnect();
    v.io = new IntersectionObserver(function (es) {
      es.forEach(function (e) { if (e.isIntersecting) loadMore(v.sentinel); });
    }, { root: v.scroll, rootMargin: '600px' });
    v.io.observe(v.sentinel);
  }

  // The floating counter shows the floor at the top of the view.
  function updateCounter() {
    var v = view;
    if (!v) return;
    var total = v.thread.floors().length;
    var top = v.scroll.getBoundingClientRect().top + 60;
    var cur = 1;
    var els = v.list.querySelectorAll('.xtv-floor');
    for (var i = 0; i < els.length; i++) {
      if (els[i].getBoundingClientRect().bottom > top) { cur = v.numbers[els[i].dataset.id] || 1; break; }
    }
    v.ctr.textContent = '#' + cur + ' / ' + total;
  }

  function goToFloor(id) {
    while (layers.length > 1) closeTop();
    var v = view;
    if (!v) return;
    if (v.filter || v.imagesOnly) { v.filter = null; v.imagesOnly = false; v.imgBtn.classList.remove('on'); renderFloors(); }
    var f = v.list.querySelector('.xtv-floor[data-id="' + id + '"]');
    if (!f) return;
    v.scroll.scrollTo({ top: f.offsetTop - 8, behavior: 'smooth' });
    f.classList.add('flash');
    setTimeout(function () { f.classList.remove('flash'); }, 1400);
  }

  function openView(focalId) {
    if (view) return;
    ensureStyle();
    var root = el('div', 'xtv');
    var bar = el('div', 'xtv-bar');
    var ttl = el('div', 'xtv-ttl', esc(W_.title));
    bar.appendChild(btn('xtv-ib', '✕', W_.close, closeTop));
    bar.appendChild(ttl);
    var imgBtn = btn('xtv-ib', '🖼 ' + esc(W_.images), '', function () { view.imagesOnly = !view.imagesOnly; imgBtn.classList.toggle('on', view.imagesOnly); renderFloors(); });
    bar.appendChild(imgBtn);
    var autoBtn = btn('xtv-ib' + (pref('auto', true) ? ' on' : ''), '<span class="sw"></span>' + esc(W_.auto), '', function () {
      var on = !pref('auto', true); setPref('auto', on); autoBtn.classList.toggle('on', on);
    });
    bar.appendChild(autoBtn);
    var openX = el('a', 'xtv-ib', '↗ ' + esc(W_.open)); openX.href = '/i/status/' + focalId;
    openX.addEventListener('click', function () { closeAll(); });
    bar.appendChild(openX);
    var sub = el('div', 'xtv-sub');
    var filterBar = el('div', 'xtv-filter'); filterBar.style.display = 'none';
    var scroll = el('div', 'xtv-scroll');
    var list = el('div', 'xtv-list');
    list.appendChild(el('div', 'xtv-msg', esc(W_.loading)));
    scroll.appendChild(list);
    var ctr = el('div', 'xtv-ctr', '#1');
    ctr.title = 'Top';
    ctr.addEventListener('click', function () { scroll.scrollTo({ top: 0, behavior: 'smooth' }); });
    root.appendChild(bar); root.appendChild(sub); root.appendChild(filterBar); root.appendChild(scroll); root.appendChild(ctr);
    document.body.appendChild(root);
    view = { focalId: focalId, root: root, list: list, scroll: scroll, sub: sub, filterBar: filterBar, ctr: ctr, imgBtn: imgBtn, ttl: ttl, numbers: {} };
    var tick = 0;
    scroll.addEventListener('scroll', function () { if (!tick) { tick = requestAnimationFrame(function () { tick = 0; updateCounter(); }); } });
    pushLayer({ close: function () { if (view && view.io) view.io.disconnect(); root.remove(); view = null; closedFor = statusId(); } });

    load(focalId).then(function (d) {
      if (!view) return;
      // A reply: read the conversation from its first post.
      if (d.ancestors.length) {
        var first = d.ancestors[0].id;
        view.thread = new Thread(first);
        view.thread.add(d.ancestors); view.thread.add([d.focal]); view.thread.add(d.items);
        view.focalId = first;
        view.cursor = '';
        return load(first).then(function (r) {
          if (!view) return;
          view.thread.add([r.focal]); view.thread.add(r.items);
          view.cursor = r.cursor;
          show(focalId);
        }, function () { show(focalId); });
      }
      if (!d.focal) { list.innerHTML = ''; list.appendChild(el('div', 'xtv-msg', esc(W_.unavailable))); return; }
      view.thread = new Thread(d.focal.id);
      view.thread.add([d.focal]); view.thread.add(d.items);
      view.cursor = d.cursor;
      show('');
    }, function (e) {
      if (!view) return;
      list.innerHTML = '';
      list.appendChild(el('div', 'xtv-msg', esc(e && e.message === 'timeout' ? W_.noTemplate : W_.failed + ((e && e.message) || e))));
    });

    function show(highlight) {
      var r = view.thread.root();
      if (r) ttl.textContent = Array.from((r.text || '').replace(/https:\/\/t\.co\/\w+/g, '').replace(/\s+/g, ' ').trim()).slice(0, 90).join('') || W_.title;
      renderFloors();
      if (highlight && highlight !== view.thread.rootId) setTimeout(function () { goToFloor(highlight); }, 60);
    }
  }

  // openPanel shows id, then the replies to it.
  function openPanel(id) {
    ensureStyle();
    var veil = el('div', 'xtv-veil');
    var panel = el('div', 'xtv-panel');
    var bar = el('div', 'xtv-bar');
    bar.appendChild(btn('xtv-ib', '✕', W_.close, closeTop));
    var scroll = el('div', 'xtv-scroll');
    scroll.appendChild(el('div', 'xtv-msg', esc(W_.loading)));
    panel.appendChild(bar); panel.appendChild(scroll); veil.appendChild(panel);
    veil.addEventListener('click', function (e) { if (e.target === veil) closeTop(); });
    document.body.appendChild(veil);
    var layer = { close: function () { veil.remove(); } };
    pushLayer(layer);

    var th = view && view.thread;
    var local = th && th.by[id];
    function render(src, replies) {
      scroll.innerHTML = '';
      if (!src) { scroll.appendChild(el('div', 'xtv-msg', esc(W_.unavailable))); return; }
      var nos = view ? view.numbers : {};
      scroll.appendChild(floorEl(src, { no: nos[src.id], quote: !!th && !!th.by[src.id], panel: true, source: true, inThread: !!nos[src.id] }));
      scroll.appendChild(el('div', 'xtv-div', esc(W_.quotedBy)));
      if (!replies.length) scroll.appendChild(el('div', 'xtv-msg', esc(W_.none)));
      replies.forEach(function (t) { scroll.appendChild(floorEl(t, { no: nos[t.id], quote: false, panel: true, inThread: !!nos[t.id] })); });
      scroll.appendChild(el('div', 'xtv-end', esc(W_.end)));
    }
    var mine = th ? th.children(id) : [];
    if (local && mine.length >= local.replies) { render(local, mine); return; }
    if (local) render(local, mine);
    load(id).then(function (d) {
      if (layers.indexOf(layer) < 0) return;
      var src = d.focal || local;
      var seen = {}, all = [];
      mine.concat(d.items.filter(function (t) { return t.replyTo === id; })).forEach(function (t) { if (!seen[t.id]) { seen[t.id] = 1; all.push(t); } });
      all.sort(function (a, b) { return ((a.created && a.created.getTime()) || 0) - ((b.created && b.created.getTime()) || 0); });
      render(src, all);
    }, function () { if (!local) render(null, []); });
  }

  // ---- image viewer: every image of the thread -------------------------------

  function openViewer(url, fromId) {
    var shots = [];
    if (view && view.thread) {
      view.thread.floors().forEach(function (t) {
        t.media.forEach(function (m) { if (!m.video) shots.push({ url: m.url, id: t.id }); });
      });
    }
    var at = 0;
    shots.forEach(function (s, i) { if (s.url === url && s.id === fromId) at = i; });
    if (!shots.length) { shots = [{ url: url, id: fromId }]; }
    var box = el('div', 'xtv-light');
    var top = el('div', 'top'), mid = el('div', 'mid'), bot = el('div', 'bot');
    var count = el('span', ''), img = document.createElement('img');
    top.appendChild(count);
    top.appendChild(btn('xtv-ib', '✕', W_.close, closeTop));
    var prev = btn('nav', '‹', '', function () { go(-1); }), next = btn('nav', '›', '', function () { go(1); });
    mid.appendChild(prev); mid.appendChild(img); mid.appendChild(next);
    var goPost = btn('lk', '↘ ' + esc(W_.goToPost), '', function () { var id = shots[at].id; closeTop(); goToFloor(id); });
    bot.appendChild(goPost);
    box.appendChild(top); box.appendChild(mid); box.appendChild(bot);
    box.addEventListener('click', function (e) { if (e.target === box || e.target === mid) closeTop(); });
    function go(d) {
      at = (at + d + shots.length) % shots.length;
      img.src = sized(shots[at].url, 'orig');
      count.textContent = (at + 1) + ' / ' + shots.length;
      prev.style.visibility = next.style.visibility = shots.length > 1 ? '' : 'hidden';
      goPost.style.display = view && view.numbers[shots[at].id] ? '' : 'none';
    }
    go(0);
    document.body.appendChild(box);
    pushLayer({
      close: function () { box.remove(); },
      key: function (e) {
        if (e.key === 'ArrowLeft') { go(-1); return true; }
        if (e.key === 'ArrowRight') { go(1); return true; }
        return false;
      }
    });
  }

  // ---- entry: auto-open on a post's page, and a button --------------------------

  var STATUS = /^\/(?:[^/]+|i\/web|i)\/status\/(\d+)/;
  var fab = null, lastId = '', closedFor = '';
  function statusId() {
    var m = location.pathname.match(STATUS);
    return m ? m[1] : '';
  }
  function tick() {
    if (!document.body) return;
    var id = statusId();
    if (id && !fab) {
      ensureStyle();
      fab = btn('xtv-fab', esc(W_.thread), '', function () { var i = statusId(); if (i) openView(i); });
      document.body.appendChild(fab);
    } else if (!id && fab) {
      fab.remove(); fab = null;
    }
    if (fab) fab.style.display = layers.length ? 'none' : '';
    if (id !== lastId) {
      lastId = id;
      if (!id) closedFor = '';
      if (id && !routing && !view && closedFor !== id && pref('auto', true)) openView(id);
    }
  }
  setInterval(tick, 400);
  document.addEventListener('keydown', function (e) {
    // Alt+T: plain T is X's own repost shortcut.
    if (e.code !== 'KeyT' || !e.altKey || e.metaKey || e.ctrlKey || layers.length) return;
    var t = e.target;
    if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
    var id = statusId();
    if (id) { e.preventDefault(); openView(id); }
  });
})();
