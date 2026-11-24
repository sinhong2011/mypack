// node --test examples/userscripts/x-thread-view.test.js
'use strict';
const test = require('node:test');
const assert = require('node:assert');
const { parseDetail, renderText, Thread } = require('./x-thread-view.user.js');

let clock = Date.UTC(2026, 0, 1);
function tweet(id, handle, text, extra) {
  clock += 60000;
  return Object.assign({
    __typename: 'Tweet',
    rest_id: id,
    core: { user_results: { result: { core: { name: handle.toUpperCase(), screen_name: handle }, avatar: { image_url: 'https://pbs.twimg.com/a.jpg' } } } },
    legacy: Object.assign({ full_text: text, created_at: new Date(clock).toUTCString(), reply_count: 0, favorite_count: 3,
      entities: { urls: [] }, conversation_id_str: '1' }, extra || {})
  });
}
const item = (t) => ({ itemType: 'TimelineTweet', tweet_results: { result: t } });
const reply = (id, handle, text, to, extra) => tweet(id, handle, text, Object.assign({ in_reply_to_status_id_str: to }, extra || {}));

// Conversation: 1 (root, op) <- 2 (alice) <- 3 (op answers alice) <- 4 (bob answers op)
//                            <- 5 (carol) ; 6 is a tombstone
function detail(focal) {
  const entries = [];
  const t1 = tweet('1', 'op', 'the root', { reply_count: 2 });
  const t2 = reply('2', 'alice', '@op first reply', '1', { display_text_range: [4, 15], reply_count: 1 });
  const t3 = reply('3', 'op', '@alice answer to alice', '2', { display_text_range: [7, 22], reply_count: 1 });
  const t4 = reply('4', 'bob', 'bob answers op', '3');
  const t5 = reply('5', 'carol', 'carol <b>here</b> https://t.co/abc #香港', '1', {
    entities: { urls: [{ url: 'https://t.co/abc', expanded_url: 'https://example.com/x', display_url: 'example.com/x' }] }
  });
  if (focal === '1') {
    entries.push({ entryId: 'tweet-1', content: { itemContent: item(t1) } });
    entries.push({ entryId: 'conversationthread-2', content: { items: [t2, t3, t4].map((t) => ({ item: { itemContent: item(t) } })) } });
    entries.push({ entryId: 'conversationthread-5', content: { items: [{ item: { itemContent: item(t5) } }] } });
    entries.push({ entryId: 'conversationthread-6', content: { items: [{ item: { itemContent: item({ __typename: 'TweetTombstone' }) } }] } });
    entries.push({ entryId: 'cursor-bottom-9', content: { itemContent: { value: 'CURSOR' } } });
  } else {
    entries.push({ entryId: 'tweet-1', content: { itemContent: item(t1) } });
    entries.push({ entryId: 'tweet-2', content: { itemContent: item(t2) } });
    entries.push({ entryId: 'tweet-3', content: { itemContent: item(t3) } });
    entries.push({ entryId: 'conversationthread-4', content: { items: [{ item: { itemContent: item(t4) } }] } });
  }
  return { data: { threaded_conversation_with_injections_v2: { instructions: [{ type: 'TimelineAddEntries', entries }] } } };
}

test('parseDetail keeps every post of a chain as its own item', () => {
  const d = parseDetail(detail('1'), '1');
  assert.strictEqual(d.focal.id, '1');
  assert.deepStrictEqual(d.items.map((t) => t.id), ['2', '3', '4', '5']);
  assert.strictEqual(d.cursor, 'CURSOR');
  assert.strictEqual(d.items[1].replyTo, '2');
});

test('a reply lists its ancestors', () => {
  const d = parseDetail(detail('3'), '3');
  assert.strictEqual(d.focal.id, '3');
  assert.deepStrictEqual(d.ancestors.map((t) => t.id), ['1', '2']);
  assert.deepStrictEqual(d.items.map((t) => t.id), ['4']);
});

test('Thread orders floors by time, root first, and nests quotes', () => {
  const d = parseDetail(detail('1'), '1');
  const th = new Thread('1');
  th.add([d.focal]);
  assert.strictEqual(th.add(d.items), 4);
  assert.strictEqual(th.add(d.items), 0, 'adding again is a no-op');
  assert.deepStrictEqual(th.floors().map((t) => t.id), ['1', '2', '3', '4', '5']);
  // A direct answer to the root quotes nothing.
  assert.deepStrictEqual(th.quotes(th.by['2']).map((t) => t.id), []);
  assert.deepStrictEqual(th.quotes(th.by['5']).map((t) => t.id), []);
  // 4 answers 3, which answers 2: nearest first, stopping at the root.
  assert.deepStrictEqual(th.quotes(th.by['4']).map((t) => t.id), ['3', '2']);
  assert.deepStrictEqual(th.quotes(th.by['4'], 1).map((t) => t.id), ['3']);
  assert.deepStrictEqual(th.children('1').map((t) => t.id), ['2', '5']);
  assert.deepStrictEqual(th.children('3').map((t) => t.id), ['4']);
});

test('renderText escapes, trims the reply prefix and expands links', () => {
  const d = parseDetail(detail('1'), '1');
  const th = new Thread('1');
  th.add(d.items);
  assert.strictEqual(renderText(th.by['2']), 'first reply');
  const html = renderText(th.by['5']);
  assert.ok(html.includes('&lt;b&gt;here&lt;/b&gt;'), html);
  assert.ok(html.includes('<a href="https://example.com/x" target="_blank" rel="noopener">example.com/x</a>'), html);
  assert.ok(html.includes('<a href="/hashtag/%E9%A6%99%E6%B8%AF">#香港</a>'), html);
});

test('renderText links mentions and keeps emails and newlines', () => {
  const d = parseDetail({ data: { threaded_conversation_with_injections_v2: { instructions: [{ type: 'TimelineAddEntries',
    entries: [{ entryId: 'tweet-7', content: { itemContent: item(tweet('7', 'a', 'hi @bob, mail me@x.com\nbye')) } }] }] } } }, '7');
  const html = renderText(d.focal);
  assert.ok(html.includes('hi <a href="/bob">@bob</a>,'), html);
  assert.ok(html.includes('me@x.com'), html);
  assert.ok(html.includes('<br>bye'), html);
});
