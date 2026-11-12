# mypack Companion (Chrome / Edge)

Optional browser entry point (spec 09G). It turns the current tab into a **project draft** in mypack Desktop, opens an existing managed app, or opens the manager. It never builds, installs, signs or deletes anything; Desktop confirmation is always required.

## Pieces

| Path | Role |
| --- | --- |
| `apps/browser-companion/` | MV3 extension, WXT + SolidJS + TypeScript (permissions: `activeTab`, `nativeMessaging`, `storage`, `contextMenus`; no host permissions, no `tabs`) |
| `cmd/mypack-native-host/` | Native Messaging host binary (stdio only, never listens on a port) + `install` / `uninstall` |
| `internal/companion/` | Protocol v1, framing, rate limit, lookup, manifest writer |

## Extension layout

`apps/browser-companion` is a [WXT](https://wxt.dev) project with SolidJS, managed with pnpm (same setup as the triwarden extension). `wxt.config.ts` holds the manifest; WXT generates `manifest.json` from it and from the entrypoints.

| Path | Role |
| --- | --- |
| `entrypoints/background.ts` | Service worker: the context menu ("Create Desktop App from this page") and its toolbar badge |
| `entrypoints/popup/` | Toolbar popup (`App.tsx`): lookup, open, manage, create draft, error states |
| `entrypoints/options/` | Options page (opens inside the extensions page): language, connection check, privacy note |
| `lib/host.ts` | Native Messaging client (`connectNative`, request IDs, timeouts, error codes) |
| `lib/locale.ts`, `lib/i18n.ts` | Language resolution and the bundled popup catalogs |
| `lib/url.ts` | Which tab addresses may become an app |
| `i18n/*.json` | Popup / options / context-menu catalogs (seven locales) |
| `locales/*.json` | Manifest strings; `@wxt-dev/i18n` turns them into `_locales/*/messages.json` |
| `public/icons/` | Extension icons, copied as-is |

## Protocol v1

Framing: 4-byte little-endian length + JSON. Max 64 KiB in, 1 MiB out. Rate limit 20 req/s (`RATE_LIMITED`).
Request `{protocolVersion, requestId, action, payload}`; response `{requestId, ok, result?, error?: {code, details?}}`.

Actions: `hello` (hostVersion, protocolVersion, actions, `locale` = Desktop `Settings.Language`), `lookup {url,title?}`, `createDraft {url,title?,faviconUrl?}`, `openManagedApp {installationId}`, `openManager {projectId?}`.
Error codes: `HOST_UNAVAILABLE`, `APP_NOT_FOUND`, `AMBIGUOUS_MATCH`, `UNSUPPORTED_URL`, `PROTOCOL_MISMATCH`, `INVALID_REQUEST`, `RATE_LIMITED`. Codes and keys are never translated.

Lookup matches by validated origin only: the tab host (lowercased, no trailing dot, leading `www.` ignored) must equal the project's source host or a configured `safeDomains` entry (or be a subdomain of a safe domain). Never by display name or title. Result `match` is `none | unique | multiple`; the popup never auto-selects among multiple.

`createDraft` accepts http(s) only (max 2048 chars, no credentials), strips the fragment, sanitizes the title (<= 200 chars, no control characters or angle brackets) and calls `Service.CreateProject` with actor `companion`. `openManager` runs `open -b dev.mypack.builder --args --show-project <id>` on macOS (fallback `open -a mypack`), or `mypack[.exe] --show-project <id>` elsewhere. Desktop must handle `--show-project`; this is a request, not yet implemented there.

## Language mapping

Resolution: **Follow browser** (`chrome.i18n.getUILanguage`) -> explicit override (options, `chrome.storage.local.language`) -> optional **Follow mypack Desktop** (locale from the `hello` response, cached). Fallback: exact -> language family -> `en`.

| Tag(s) | Popup catalog (`i18n/`) | Chromium `_locales/` from `locales/` (name, toolbar tooltip, context-menu default) |
| --- | --- | --- |
| en* | `en` | `en` |
| zh-TW, zh-HK, zh-MO, zh-Hant* | `zh-Hant` | `zh_TW` |
| zh, zh-CN, zh-SG, zh-Hans* | `zh-Hans` | `zh_CN` |
| yue* (explicit choice only) | `yue-Hant-HK` | none (Chromium has no `yue`; falls back to `zh_TW` / `en` by browser language) |
| es* / fr* / ja* | `es` / `fr` / `ja` | `es` / `fr` / `ja` |

Cantonese is a separately authored colloquial HK catalog, never a copy of zh-Hant. A browser set to zh-HK gets zh-Hant until the user picks Cantonese in options. The context-menu title is rebuilt from the resolved popup locale (including the Cantonese catalog). All copy still needs native-speaker review.

## Build

Needs Node.js and pnpm. In `apps/browser-companion`:

- `pnpm install` (also runs `wxt prepare`, which generates `.wxt/` types).
- `pnpm build` -> `.output/chrome-mv3/`; `pnpm build:edge` -> `.output/edge-mv3/`. Both are complete unpacked MV3 extensions.
- `pnpm zip` / `pnpm zip:edge` -> store-ready zips in `.output/`.
- `pnpm dev` (or `pnpm dev:edge`) opens a browser with the extension loaded and reloads it on change. Its profile is temporary, so the extension ID differs from your normal profile; register the host for that ID too if you want the popup to connect.
- `pnpm compile` type-checks; `pnpm test` runs the Vitest suite.

## Load and register

1. Build the host: `CGO_ENABLED=0 go build -o mypack-native-host ./cmd/mypack-native-host` (keep it at a stable path; it is referenced by absolute path).
2. Build the extension (above), then Chrome/Edge -> `chrome://extensions` (`edge://extensions`) -> Developer mode -> **Load unpacked** -> select `apps/browser-companion/.output/chrome-mv3` (Edge: `.output/edge-mv3`, or the Chrome build). Copy the extension ID shown on the card.
3. Register the host for that ID: `./mypack-native-host install --extension-id <ID> [--browser chrome|edge|all]` (default `all`). Repeat `--extension-id` for several IDs. This writes `dev.mypack.companion.json` to:
   - macOS: `~/Library/Application Support/Google/Chrome/NativeMessagingHosts/`, `~/Library/Application Support/Microsoft Edge/NativeMessagingHosts/`
   - Linux: `~/.config/google-chrome/NativeMessagingHosts/`, `~/.config/microsoft-edge/NativeMessagingHosts/`
   - Windows: writes the manifest under `%AppData%\mypack\native-messaging\` and prints the `reg add` commands for `HKCU\Software\Google\Chrome\NativeMessagingHosts\dev.mypack.companion` (and the Edge key).
4. Reopen the popup on any http(s) page. `./mypack-native-host uninstall` removes the manifests.

The unpacked extension ID depends on its folder path; re-run `install` if it changes. Rebuilding into the same `.output/` folder keeps the ID; click reload on the extension card after a rebuild. The host uses the same database as Desktop (`MYPACK_DB` overrides).

## Tests

- Host: `go test ./internal/companion ./cmd/mypack-native-host` (the latter builds the real binary and drives hello -> createDraft -> lookup over framed stdio against a temp DB).
- Extension: `pnpm test` in `apps/browser-companion` covers locale resolution, URL checks, catalog key parity, and the popup and options states against a fake Native Messaging port.
- Popup against the real host: build the host, then `MYPACK_NATIVE_HOST=/path/to/mypack-native-host pnpm test`. It renders the popup with the host on a temp DB, creates a draft and finds it again. `openManager` is answered locally so Desktop is not brought to the front.
