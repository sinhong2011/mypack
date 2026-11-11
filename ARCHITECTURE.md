# mypack — Implementation Architecture

Implements `mypack_product_technical_spec_v1.7.html`. Stack per spec: **Go + MyGo**
(github.com/egoist/mygo v0.8.3, pure Go, no cgo). Native UI Builder, prebuilt
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
apps/browser-companion/  Chrome/Edge MV3 extension (7 locales)
scripts/                 release-runtimes.sh: cross-compile runtimes → signed registry
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

## Runtime registry

`registry.json` (schemaVersion 1) + `registry.json.sig` (Ed25519 over the exact bytes).
Root public key embedded in Builder (`internal/runtimemanager/trust.go`).
Archive: `mypack-runtime-<version>-<os>-<arch>.tar.gz` containing the single binary.
Verify: signature → sha256 + size → safe extract to temp → atomic rename into
`<cache>/runtimes/<version>/<os-arch>/`, with a `.verified` marker.
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
