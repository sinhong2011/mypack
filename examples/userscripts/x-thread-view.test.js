// node --test examples/userscripts/x-thread-view.test.js
'use strict';
const test = require('node:test');
const assert = require('node:assert');
const { parseDetail, renderText } = require('./x-thread-view.user.js');

function tweet(id, handle, text, extra) {
  return Object.assign({
    __typename: 'Tweet',
    rest_id: id,
    core: { user_results: { result: { core: { name: handle.toUpperCase(), screen_name: handle }, avatar: { image_url: 'https://pbs.twimg.com/a.jpg' } } } },
    legacy: Object.assign({ full_text: text, created_at: 'Wed Oct 10 20:19:24 +0000 2018', reply_count: 0, favorite_count: 3,
      entities: { urls: [] } }, extra || {})
  });
}
const item = (t) => ({ itemType: 'TimelineTweet', tweet_results: { result: t } });

const fixture = {
  data: { threaded_conversation_with_injections_v2: { instructions: [{
    type: 'TimelineAddEntries',
    entries: [
      { entryId: 'tweet-1', content: { itemContent: item(tweet('1', 'root', 'the root')) } },
      { entryId: 'tweet-2', content: { itemContent: item(tweet('2', 'op', '@root focal <b>post</b> https://t.co/abc #香港', {
        display_text_range: [6, 46], reply_count: 2,
        entities: { urls: [{ url: 'https://t.co/abc', expanded_url: 'https://example.com/x', display_url: 'example.com/x' }] }
      })) } },
      { entryId: 'conversationthread-3', content: { items: [
        { item: { itemContent: item(tweet('3', 'alice', 'first reply', { reply_count: 4 })) } },
        { item: { itemContent: item({ __typename: 'TweetWithVisibilityResults', tweet: tweet('4', 'alice', 'continued') }) } },
        { item: { itemContent: { itemType: 'TimelineTimelineCursor', value: 'x' } } }
      ] } },
      { entryId: 'conversationthread-5', content: { items: [
        { item: { itemContent: item({ __typename: 'TweetTombstone' }) } }
      ] } },
      { entryId: 'cursor-bottom-9', content: { itemContent: { value: 'CURSOR' } } }
    ]
  }] } }
};

test('parseDetail splits ancestors, focal, floors and cursor', () => {
  const d = parseDetail(fixture, '2');
  assert.strictEqual(d.focal.id, '2');
  assert.deepStrictEqual(d.ancestors.map((t) => t.id), ['1']);
  assert.strictEqual(d.floors.length, 1, 'tombstone-only thread is dropped');
  assert.strictEqual(d.floors[0].tweet.id, '3');
  assert.deepStrictEqual(d.floors[0].chain.map((t) => t.id), ['4']);
  assert.strictEqual(d.floors[0].tweet.replies, 4);
  assert.strictEqual(d.cursor, 'CURSOR');
  assert.strictEqual(d.focal.handle, 'op');
});

test('renderText escapes, trims the reply prefix and expands links', () => {
  const d = parseDetail(fixture, '2');
  const html = renderText(d.focal);
  assert.ok(!html.includes('@root'), html);
  assert.ok(html.includes('&lt;b&gt;post&lt;/b&gt;'), html);
  assert.ok(html.includes('<a href="https://example.com/x" target="_blank" rel="noopener">example.com/x</a>'), html);
  assert.ok(html.includes('<a href="/hashtag/%E9%A6%99%E6%B8%AF">#香港</a>'), html);
});

test('renderText links mentions and keeps emails', () => {
  const t = parseDetail({ data: { threaded_conversation_with_injections_v2: { instructions: [{ type: 'TimelineAddEntries',
    entries: [{ entryId: 'tweet-7', content: { itemContent: item(tweet('7', 'a', 'hi @bob, mail me@x.com\nbye')) } }] }] } } }, '7').focal;
  const html = renderText(t);
  assert.ok(html.includes('hi <a href="/bob">@bob</a>,'), html);
  assert.ok(html.includes('me@x.com'), html);
  assert.ok(html.includes('<br>bye'), html);
});
