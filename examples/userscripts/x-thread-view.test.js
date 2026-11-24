// node --test examples/userscripts/x-thread-view.test.js
'use strict';
const test = require('node:test');
const assert = require('node:assert');
const { parseDetail, renderText, statusIdOf } = require('./x-thread-view.user.js');

let clock = Date.UTC(2026, 0, 1);
function tweet(id, handle, text, extra) {
  clock += 60000;
  return Object.assign({
    __typename: 'Tweet',
    rest_id: id,
    core: { user_results: { result: { core: { name: handle.toUpperCase(), screen_name: handle }, avatar: { image_url: 'https://pbs.twimg.com/a.jpg' }, is_blue_verified: handle === 'op' } } },
    legacy: Object.assign({ full_text: text, created_at: new Date(clock).toUTCString(), reply_count: 0, favorite_count: 3, retweet_count: 1, quote_count: 1,
      entities: { urls: [] } }, extra || {})
  });
}
const item = (t) => ({ itemType: 'TimelineTweet', tweet_results: { result: t } });
const reply = (id, handle, text, to, toHandle, extra) =>
  tweet(id, handle, text, Object.assign({ in_reply_to_status_id_str: to, in_reply_to_screen_name: toHandle }, extra || {}));

function detail() {
  const t1 = tweet('1', 'op', 'the root', { reply_count: 2 });
  const t2 = reply('2', 'alice', '@op first reply', '1', 'op', { display_text_range: [4, 15], reply_count: 1 });
  const t3 = reply('3', 'op', '@alice answer to alice', '2', 'alice', { display_text_range: [7, 22] });
  const t5 = reply('5', 'carol', 'carol <b>here</b> https://t.co/abc #香港', '1', 'op', {
    entities: { urls: [{ url: 'https://t.co/abc', expanded_url: 'https://example.com/x', display_url: 'example.com/x' }] }
  });
  return { data: { threaded_conversation_with_injections_v2: { instructions: [
    { type: 'TimelineAddEntries', entries: [
      { entryId: 'tweet-0', content: { itemContent: item(tweet('0', 'up', 'an ancestor')) } },
      { entryId: 'tweet-1', content: { itemContent: item(t1) } },
      { entryId: 'conversationthread-2', content: { items: [t2, t3].map((t) => ({ item: { itemContent: item(t) } })) } },
      { entryId: 'conversationthread-5', content: { items: [{ item: { itemContent: item(t5) } }] } },
      { entryId: 'conversationthread-6', content: { items: [{ item: { itemContent: item({ __typename: 'TweetTombstone' }) } }] } },
      { entryId: 'cursor-bottom-9', content: { itemContent: { value: 'CURSOR' } } }
    ] },
    { type: 'TimelineAddToModule', moduleItems: [{ item: { itemContent: item(reply('4', 'bob', 'more of carol', '5', 'carol')) } }] }
  ] } } };
}

test('parseDetail groups replies into threads, as X shows them', () => {
  const d = parseDetail(detail(), '1');
  assert.strictEqual(d.focal.id, '1');
  assert.deepStrictEqual(d.ancestors.map((t) => t.id), ['0']);
  assert.deepStrictEqual(d.threads.map((th) => th.map((t) => t.id)), [['2', '3'], ['5', '4']]);
  assert.strictEqual(d.cursor, 'CURSOR');
  const t3 = d.threads[0][1];
  assert.strictEqual(t3.replyTo, '2');
  assert.strictEqual(t3.replyToHandle, 'alice');
  assert.strictEqual(d.focal.verified, true);
  assert.strictEqual(d.focal.reposts, 2, 'reposts count quotes too, as X does');
});

test('a missing focal falls back to the last ancestor', () => {
  const d = parseDetail(detail(), 'gone');
  assert.strictEqual(d.focal.id, '1');
});

test('statusIdOf reads post links', () => {
  assert.strictEqual(statusIdOf('/alice/status/123'), '123');
  assert.strictEqual(statusIdOf('https://x.com/i/web/status/456?s=20'), '456');
  assert.strictEqual(statusIdOf('/alice/status/789/photo/1'), '789');
  assert.strictEqual(statusIdOf('/home'), '');
});

test('renderText escapes, trims the reply prefix and expands links', () => {
  const d = parseDetail(detail(), '1');
  assert.strictEqual(renderText(d.threads[0][0]), 'first reply');
  const html = renderText(d.threads[1][0]);
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
