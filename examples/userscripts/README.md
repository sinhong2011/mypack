# Example userscripts

Userscripts in the Greasemonkey format that MyPack apps (runtime 1.0.11+) run. They also work in Violentmonkey,
Tampermonkey and ScriptCat.

| Script | What it does |
| --- | --- |
| [x-thread-view.user.js](x-thread-view.user.js) | Reads an X post as a forum thread, in the style of HK forums: numbered floors (#1 is the post), the original poster tagged OP, a reply's own chain kept in one floor, and each reply's sub-thread opened in a modal (stackable, Esc closes). Open it with the **☰ Thread view** button on a post page or Alt+T. |

Install one into all apps, or only into your X app:

```sh
mypack scripts install examples/userscripts/x-thread-view.user.js
mypack scripts install examples/userscripts/x-thread-view.user.js --app <your X app's identifier>
```

Apps load scripts when they start, so quit and reopen the app afterwards.

## How X Thread View gets its data

It reads X's own `TweetDetail` GraphQL answers as the page loads them. It needs no API key, and nothing leaves x.com.
For a sub-thread or the next page of replies, it sends the request the page last made again with another post id.
X signs some requests. If X refuses the repeated one, the script lets X's own router open that post behind the
thread view and reads that answer. X changes its web API often, so expect to update the parser
(`parseDetail`) from time to time. `x-thread-view.test.js` covers it: `node --test examples/userscripts/`.
