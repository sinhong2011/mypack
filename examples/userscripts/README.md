# Example userscripts

Userscripts in the Greasemonkey format that MyPack apps (runtime 1.0.11+) run. They also work in Violentmonkey,
Tampermonkey and ScriptCat.

| Script | What it does |
| --- | --- |
| [x-thread-view.user.js](x-thread-view.user.js) | Opens a reply's nested replies in place, in stacked X-style panels. On a post's page, clicking a reply (or "Show N replies" under it) opens it in a panel instead of leaving the page: the route, the page and your scroll stay. Every reply shows its own "N replies" chip; opening one stacks a panel on top, with the ones below stepped back (Hong Kong forum style), and a breadcrumb of the path. "Go to reply" closes the panels and highlights the reply on the page. Photos open in a viewer over the panels. Esc closes the top layer; ⌘/Ctrl-click opens normally. Follows X's light, dim and dark themes. |

Install one into all apps, or only into your X app:

```sh
mypack scripts install examples/userscripts/x-thread-view.user.js
mypack scripts install examples/userscripts/x-thread-view.user.js --app <your X app's identifier>
```

Apps load scripts when they start, so quit and reopen the app afterwards.

## How X Thread View gets its data

It sends the page's own `TweetDetail` GraphQL request again, with another post's id: the page's headers (its request
signature included) and sign-in, no API key, nothing leaving X (x.com and its api.x.com). It never changes the page's
route; if X refuses the request, the dialog offers to open the post the normal way. The script keeps the last request it
saw (in x.com's local storage) for the next launch; until it has seen one, the first post you open opens X's way.

X changes its web app often: the 2026 redesign moved the API to `api.x.com`, reshaped its answers and dropped the
`data-testid` markup. The parser (`parseDetail`) reads both answer shapes and the page code both markups; expect to update
them from time to time. `x-thread-view.test.js` covers the parser: `node --test examples/userscripts/`.
