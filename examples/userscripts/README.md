# Example userscripts

Userscripts in the Greasemonkey format that MyPack apps (runtime 1.0.11+) run. They also work in Violentmonkey,
Tampermonkey and ScriptCat.

| Script | What it does |
| --- | --- |
| [x-thread-view.user.js](x-thread-view.user.js) | Reads an X conversation as a forum thread, in the way Hong Kong forums do: every post a numbered floor in time order from the first post; a floor that answers another quotes it at its top, quotes of quotes nested; 💬 N opens a panel with the post, "Quoted by" and its replies (each with "Go to reply"), panels stacking; 👁 shows one author's floors; images open in a viewer of the whole thread. It opens by itself on a post's page (turn off Auto-open in its bar); ☰ Thread or Alt+T opens it by hand. |

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
