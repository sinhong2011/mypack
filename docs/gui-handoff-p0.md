# GUI handoff: P0 runtime features (runtime 1.0.5)

All new fields are optional and additive in `mypack.json`. Absent = the default shown, and an absent field is never written back (so existing projects do not get "settings changed" update prompts). Validation lives in `pkg/config` (`Config.Validate`); helpers: `NativeConfig.NotificationsMode()`, `BadgeMode()`, `LinkRoutingEnabled()`, `WebViewConfig.EffectiveUAPreset()`.

`config.DefaultRuntimeVersion` is now `1.0.5` (new projects pin it; older projects show "Runtime 1.0.5 is available" in Library as before). Only runtime 1.0.5+ understands these fields; an older runtime ignores unknown behaviour but the config still parses (it is strict about unknown keys only in the Builder, and 1.0.5 is the first runtime to read them).

## Fields the GUI must expose

| Field | Values | Default | Suggested control | Where |
|---|---|---|---|---|
| `native.notifications` | `"allow"`, `"ask"`, `"deny"` | `allow` (treat empty as allow) | 3-way segmented control "Notifications: Allow / Ask first / Off" | Settings > Native |
| `native.badge` | `"auto"`, `"off"` | `auto` | Switch "Show unread count on the Dock/taskbar icon" (on = auto) | Settings > Native |
| `native.linkRouting` | `true`/`false` (pointer; absent = true) | true | Switch "Open links in my other MyPack apps". Off also removes this app from other apps' routing table | Settings > Navigation |
| `webview.userAgentPreset` | `"safari"`, `"chrome"`, `"edge"`, `"firefox"`, `"mobile-safari"`, `"custom"` | `safari` (empty) | Popup menu "Browser identity". Show the free-text `webview.userAgent` field only for `custom` | Settings > Advanced |

Rules for the UA control:
- Backward compatibility: a project with a non-empty `webview.userAgent` and no preset is `custom`. Use `cfg.WebView.EffectiveUAPreset()` to initialise the menu.
- Picking a non-custom preset: set `userAgentPreset` and clear `userAgent` (an explicit preset wins over a stale string, but do not leave stale text around).
- Picking `safari`: you may store `""` (empty) instead of `"safari"`.
- `custom` with an empty string fails validation (`webview.userAgent` REQUIRED); disable Build/Save until filled.
- Chrome/Edge/Firefox presets are per-OS strings chosen by the runtime (macOS, Windows, Linux, arm64 variants); the GUI never needs to show them. Chrome/Edge help sites that sniff for Chromium (Meet, Teams, Figma); mobile-safari gives mobile layouts.

Suggested copy (add to i18n):
- Notifications: "Web notifications" / "Let websites in this app show system notifications. Ask first shows a permission prompt once per site."
- Badge: "Unread badge" / "Shows the unread count on the Dock or taskbar icon, from the page title or the site's own badge."
- Link routing: "Open links in the right app" / "Links to a website you also made an app for open in that app instead of your browser."
- Browser identity: "Some sites only work in a specific browser. Chrome and Edge identities help with Meet, Teams and Figma."

## Create flow
No questions needed. Keep defaults (allow / auto / routing on / safari). Optionally surface the three switches only in Settings.

## Library / routing table
- The Builder writes `<UserConfigDir>/mypack-apps/routes.json` automatically (`internal/appservice/routes.go`) at service start and after every installation change (install, update, repair, uninstall, relocate, forget, import, external DB change). The GUI does nothing.
- `MYPACK_ROUTES_FILE` overrides the path; `appservice.Options.RoutesFile` overrides it per service ("-" disables). With a custom `DBPath` the file sits beside the DB (test isolation), so GUI code that builds `Options{}` with defaults needs no change.
- Optional UI idea: in an app's detail view show "Links to <hosts> open in this app" using `appservice.Impl.RoutingEntries(ctx)` (hosts + sourceHost per app). Ambiguity rule: two apps that own the same registrable domain (e.g. Gmail and Calendar both claim google.com) both win only on their exact source host; for other hosts of that domain no app is picked and the link goes to the browser. Surfacing that as a hint is a nice-to-have, not required.

## Behaviour to document in the UI/help
- Notifications: shown by the OS with the app's own name and icon. First notification triggers the macOS "allow notifications" prompt (system-owned; cannot be bypassed). If the user denied it, they must enable it in System Settings > Notifications. Notifications from embedded third-party iframes and from sign-in popups are never shown; at most 10 per minute per app.
- Badge: macOS Dock badge; Windows taskbar overlay and Linux launcher count come from MyGo and are best-effort. Favicon-only badges are not detected.
- Service workers/push: WKWebView has no web push; sites only notify while the app is running (a hidden/tray app keeps running with background throttling off when notifications are enabled).
- "Ask" answers are stored in the app's profile (`state.json`); there is no UI to reset them yet. Optional: a "Reset notification permissions" button that removes the `notify` key from `<profile>/state.json` (or clearing profile data does it).
- Linux popups (OAuth sign-in) open as independent pages without `window.opener` (WebKitGTK limitation, MyGo docs); prefer redirect-based sign-in there.

## Things the GUI must NOT do
- Do not register http/https handling for generated apps. Routing is only MyPack app to MyPack app.
- Do not write routes.json from the GUI; call the service.
