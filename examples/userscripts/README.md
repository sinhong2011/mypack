# Example userscripts

Userscripts in the Greasemonkey format that MyPack apps (runtime 1.0.11+) run. They also work in Violentmonkey,
Tampermonkey and ScriptCat.

| Script | What it does |
| --- | --- |
| [x-thread-view.user.js](x-thread-view.user.js) | Opens a reply's nested replies in place. On a post's page, clicking a reply (or "Show N replies" under it) opens it in an X-style dialog instead of leaving the page: the route, the page and your scroll stay where they are. Click a reply in the dialog to go a level deeper, ← to go back up, Esc to close; ⌘/Ctrl-click opens a reply the normal way. Follows X's light, dim and dark themes. |

Install one into all apps, or only into your X app:

```sh
mypack scripts install examples/userscripts/x-thread-view.user.js
mypack scripts install examples/userscripts/x-thread-view.user.js --app <your X app's identifier>
```

Apps load scripts when they start, so quit and reopen the app afterwards.

## How X Thread View gets its data

It sends the page's own `TweetDetail` GraphQL request again, with another post's id: the page's headers (its request
signature included) and sign-in, no API key, nothing leaving x.com. It never changes the page's route; if X refuses the
request, the dialog offers to open the post the normal way. X changes its web API often, so expect to update the parser
(`parseDetail`) from time to time. `x-thread-view.test.js` covers it: `node --test examples/userscripts/`.
