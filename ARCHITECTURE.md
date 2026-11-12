# mypack — Implementation Architecture

Stack: **Go + MyGo**
(github.com/egoist/mygo v0.9.0, pure Go, no cgo). Native UI Builder, prebuilt
MyGo WebView runtime + configuration injection, SQLite state, CLI + MCP stdio.
MyGo API reference for contributors: `docs/mygo-api-digest.md`.

Toolchain: Go ≥ 1.27.3 (`GOTOOLCHAIN=auto` fetches it). Module: `mypack.dev/mypack`.
Everything must build with `CGO_ENABLED=0` (SQLite via `modernc.org/sqlite`).

## Layout

```
cmd/mypack/              Native GUI entry (MyGo native UI). `mypack` with args → CLI
cmd/mypack-cli/          Headless CLI entry (same code as `mypack <cmd>`)
cmd/mypack-native-host/  Browser-companion Native Messaging host
runtime/cmd/mypack-runtime/  Generated-app runtime (prebuilt, downloaded on demand)
pkg/config/              mypack.json schema, defaults, validation, migration, Pake import
internal/domain/         Entities: Project, BuildJob, Artifact, Installation …
internal/appservice/     Service interface (api.go) + implementation (service.go)
internal/storage/        SQLite (WAL) persistence, revisions, audit log
internal/metadata/       Website title / favicon / manifest fetch (timeouts, size caps)
internal/icons/          PNG/ICO decode, resize, .icns / .ico writers, default icon
internal/runtimemanager/ Signed registry manifest (Ed25519), download, verify, cache
internal/builder/        Pipeline + platform assemblers (darwin/windows/linux)
internal/installer/      Install & Open, update (atomic swap), repair, uninstall
internal/i18n/           7 locales: en, zh-Hant, zh-Hans, yue-Hant-HK, es, fr, ja
internal/desktop/        GUI: design tokens (design), motion-first kit (kit), screens (app), menubar
internal/cli/            CLI commands, --json output contract, exit codes
internal/mcp/            MCP stdio server (JSON-RPC 2.0)
internal/companion/      Native-messaging protocol for the browser extension
apps/browser-companion/  Chrome/Edge MV3 extension, WXT + SolidJS (7 locales)
scripts/                 release-runtimes.sh, bundle-runtime.sh (installer-shipped host runtime)
```

## Generated-app runtime contract (Builder ⇄ Runtime)

The runtime is ONE prebuilt binary per `os-arch` that knows nothing about a
specific website. The Builder injects identity + config:

| Platform | Executable | Config | Icon |
|---|---|---|---|
| macOS | `<Name>.app/Contents/MacOS/<Name>` | `Contents/Resources/mypack.json` | `Contents/Resources/AppIcon.icns` + `icon.png` |
| Windows | `<Name>/<Name>.exe` | `<Name>/resources/mypack.json` | `resources/icon.png`, `icon.ico` |
| Linux | `<Name>/<name>` | `<Name>/resources/mypack.json` | `resources/icon.png` + `.desktop` |

Runtime lookup order for config: `$MYPACK_CONFIG` → `../Resources/mypack.json`
(relative to executable) → `./resources/mypack.json` → `./mypack.json`.
Injection files are inlined into `mypack.json` at assembly time (no paths at runtime).

Profile (website data) lives OUTSIDE the bundle, keyed by `app.profileId`:
macOS `~/Library/Application Support/mypack-apps/<profileId>/`,
Windows `%APPDATA%\mypack-apps\<profileId>\`, Linux `$XDG_DATA_HOME/mypack-apps/<profileId>/`.
Update/Repair never touch it; Uninstall only with explicit clearData.

## Runtime native features (1.0.5)

The runtime preloads a Web API shim (`runtime/inject/webapi.go`) in each app's main frames: `window.Notification`,
`ServiceWorkerRegistration.showNotification` and the Badging API (`navigator.setAppBadge`). They call the bound Go
service `mypack` (`runtime/app/bridge.go`) through MyGo's page-to-Go bridge; every call is checked against the
navigation policy (`Policy.IsAppURL`), so only the app's own pages (not iframes, not sign-in popups) can notify or
badge. A title heuristic (`(3) Inbox`, `Inbox (12)`, `• Slack`) feeds the same badge controller
(`runtime/app/badge.go`). Config: `native.notifications` (allow|ask|deny), `native.badge` (auto|off).

Link routing: the Builder keeps `<UserConfigDir>/mypack-apps/routes.json` (`internal/appservice/routes.go`, schema in
`runtime/routing`). When the navigation policy sends a clicked link to the browser, the runtime first looks for another
installed Mypack app owning the host (`open -a <bundle> <url>` on macOS, `<app> <url>` on Windows/Linux) and the target
app opens it if the URL is internal for it (else the browser; never re-routed). `native.linkRouting` (default true).

`webview.userAgentPreset` (safari|chrome|edge|firefox|mobile-safari|custom) maps to per-OS user agent strings in
`runtime/app/useragent.go`. See `docs/gui-handoff-p0.md` for the GUI surface.

## Window modes (1.0.6)

`window.mode` (window|compact|sidebar|menubar; `runtime/app/modes.go` menu bar, `runtime/app/layout.go` layouts). Menu bar: a tray icon toggles a hidden-not-closed
`mygo.NewFlyout` popover holding the page; accessory activation policy, and the Builder writes `LSUIElement` for
runtime ≥ 1.0.6 (`internal/builder/darwin.go`). Window/compact/sidebar are layouts of the main window the user
switches at runtime (View menu); the sidebar docks full-height to a screen edge. Choice and bounds live in `state.json`. Panel size: `window.panelWidth/panelHeight` (default 400×640).
See `docs/gui-handoff-window-modes.md`.

## Group apps (1.0.6)

`app.group.members` makes one app of several websites (`runtime/app/group.go`): a native window with a rail and one
embedded web view per member, each in session `<profileId>.<memberId>`. Links route between members by host; split
view shows two. The service's `CreateGroup` (`internal/appservice/group.go`) makes groups from Library projects or
addresses and keeps member icons beside the DB. See
`docs/gui-handoff-window-modes.md`.

## Content blocking (1.0.7)

`webview.blocking` (`config.BlockingConfig`): `trackers` blocks mypack's own list of well-known ad and tracker hosts
(and a few tracker endpoints) when a page loads them from another site; `cookieBanners` hides the banners of common
consent tools; `allow` lists sites where neither applies; `hide` is CSS selectors hidden on every page. The runtime
(`runtime/app/blocking.go`) turns it into WebKit content-blocker JSON and compiles it with `mygo.CompileContentRules`
before the first page loads (the engine caches the compiled list across launches); every page and group member gets
it through `pageOptionsIn`. The list is not EasyList, whose licence would carry over to generated apps.

Feature pins: `Config.MinRuntime()` is the earliest runtime a config's features need (window modes and groups 1.0.6,
blocking 1.0.7, tabs 1.0.8), and `ApplyDefaults` raises a lower `runtime.version` to it, so turning a feature on in an existing
project never builds an app that quietly lacks it. Pins are never lowered.

## Tabs (1.0.8)

`window.tabs` gives the main window a native tab bar (`runtime/app/tabs.go`) and one web view per tab, all in the
app's session. A user-initiated popup with no size (a `target=_blank` link) opens a tab after the current one, or
shows the tab that already has that page; popups that ask for a size stay windows, since sign-in flows need
`window.opener`. Pinned tabs sit first, stay loaded (so they notify and badge) and are not closed by ⌘W. Tabs, pins
and the shown tab are kept in state.json and restored at launch (never in incognito apps); unpinned tabs load when
first shown. Favicons are fetched by the runtime from the page's `<link rel=icon>`. File: New Tab ⌘T, Reopen Closed
Tab ⌘⇧T, Close Tab ⌘W, Close Window ⌘⇧W; View: ⌃Tab / ⌃⇧Tab, ⌘1…⌘9. Not in group apps (validation) or the menu-bar
panel.

## Moving apps between computers

A `.mypack` file (`internal/appservice/library.go`) holds one or more apps: each config plus the pictures a config only
points at (the icon, the image beneath an Icon Studio style, a group's site icons), with local paths stripped.
`ExportLibrary` writes one; `ImportLibrary` adds its apps keeping their identifier and profile id, skipping apps
already in the Library unless asked to replace them (a replaced app keeps its local project id, so builds and installs
stay linked). GUI: File > Export Library… / Import Apps…, a project's Export…; CLI: `mypack export`, `mypack import`.

Library sync (`internal/appservice/sync.go`) keeps one `.mypack` file per app under `<SyncFolder>/apps/` in a
folder a sync service shares (iCloud Drive by default). Each sync compares every app here, its file there and the hash
both had at the last sync (settings key `sync.state`): the side that changed wins, the later edit when both did.
Moving an app to the Trash writes a tombstone that moves it to the Trash elsewhere; restoring brings it back. A sync
never deletes for good, builds or installs. The GUI syncs at launch, every two minutes and a few seconds after a
Library change; `mypack sync` runs one.

## Icon studio

`app.icon.style` (`config.IconStyle`) dresses an icon up: a glyph (the icon's image, up to 3 letters, or an emoji), a
background (`auto` colours from the image, `solid`, `gradient`, `glass`, or `none` = the image as it is), the glyph's
size and tint, and a badge ribbon. `icons.Styled` (`internal/icons/style.go`) draws it on the same squircle grid as
`icons.Default`, so the later steps (MacAppIcon, ICO, Linux PNGs) need nothing new. Emoji come from Apple Color Emoji's
`sbix` bitmaps (`internal/icons/emoji.go`), so they are drawn on macOS only; elsewhere the name's initial is used.

Where the style is drawn: the Builder draws it over `Request.IconPNG` (so CLI builds of a mypack.json get it too). The
service keeps a styled project's `IconPNG` as the finished icon, so the Library and installs show it, and keeps the
image beneath in `icons/<projectId>.png` beside the DB (`internal/appservice/iconstyle.go`); builds are given that
image, `UpdateProject` redraws when the style or name changes, and `ProjectIconImage` returns it to the GUI's studio
(`internal/desktop/app/iconstudio.go`).

## Runtime registry

`registry.json` (schemaVersion 1) + `registry.json.sig` (Ed25519 over the exact bytes).
Root public key embedded in Builder (`internal/runtimemanager/trust.go`).
Archive: `mypack-runtime-<version>-<os>-<arch>.tar.gz` containing the single binary.
Windows and Linux installers ship the matching archive and a copy of the signed
manifest in `Resources/runtimes`. macOS ships `registry.json` and a Mach-O helper
at `Contents/Resources/bin/mypack-runtime` (`resources/darwin-<arch>/bin`, which
`mygo build` signs with the app so notarization passes). First launch copies
those into the cache (`SeedFromDir`). The remote registry is fetched only for a
version or platform that is not already cached. Verify: signature → sha256 + size → safe extract
to temp → atomic rename into `<cache>/runtimes/<version>/<os-arch>/`, with a
`.verified` marker.
Cache: macOS `~/Library/Caches/mypack/runtimes`, Windows `%LOCALAPPDATA%\mypack\cache\runtimes`,
Linux `${XDG_CACHE_HOME:-~/.cache}/mypack/runtimes`.
Dev registry: `scripts/release-runtimes.sh` builds all 7 targets into `dist/registry/`
and signs with the dev key in `dist/keys/` (never committed). Default registry URL is
configurable (`settings.registryUrl`, env `MYPACK_REGISTRY`); `file://` supported.

## State

SQLite at `<user config dir>/mypack/mypack.db` (WAL, busy_timeout 5s).
GUI/CLI/MCP are separate processes sharing it; every project write bumps
`revision` and checks expected revision. Activity log records actor/action/result.

## Build pipeline

preparing (only for JS projects) → validating → resolving → downloading → assembling → customizing → signing →
packaging → verifying → publishing. Work happens in a temp dir; publish is an
atomic rename into `<outputDir>/<Name>/<os-arch>/`. A failed/cancelled build never
touches the previous artifact. macOS: ad-hoc `codesign --force --deep -s -` after
all resources are final (ADR-019); non-macOS hosts cannot produce macOS targets (capability matrix).

### JS project builds (preparing stage)

A folder with `package.json` becomes a `dir` source with `source.project` + `source.build`
(`packageManager`, `script`, `outDir`). Before the engine runs, the service builds it:
the build needs a human approval keyed by the fingerprint (package manager, script, project
path), stored per project in `settings` (`projectbuild.<projectId>`); MCP can never approve.
It runs `<pm> run <script>` (after `install`/`ci` when `node_modules` is missing) with explicit
args, no shell, in the project root, 15 min timeout, killing the process group on cancel. The
engine never sees `source.project`/`source.build`. See `docs/gui-handoff-project-build.md`.

## Error contract

`domain.Error{Code, Message, Cause, Remediation, Field}`. Stable codes, e.g.
`INVALID_CONFIG`, `REVISION_CONFLICT`, `RUNTIME_DOWNLOAD_REQUIRED`, `RUNTIME_VERIFY_FAILED`,
`RUNTIME_NOT_FOUND`, `TARGET_UNSUPPORTED`, `BUILD_CANCELLED`, `SIGNING_FAILED`,
`INSTALL_CONFLICT`, `NOT_FOUND`, `NOT_MANAGED`, `PROJECT_BUILD_REQUIRED`, `PROJECT_BUILD_FAILED`,
`PROJECT_OUTPUT_MISSING`, `NODE_NOT_FOUND`, `APPROVAL_REQUIRED`. CLI exit codes: 0 ok, 1 generic,
2 usage, 3 validation, 4 runtime/network, 5 build failed, 6 approval required, 130 cancelled.
