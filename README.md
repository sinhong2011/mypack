# Mypack

**Any website. Any desktop. One beautiful native builder.**

Mypack is free and open source under the [Apache License 2.0](LICENSE).
Every feature is included: no app limits, no license keys, no account.

Mypack turns a website (or a local HTML project) into a real, standalone desktop
app for the computer you're on — no Go, Rust, Node or Xcode project required.

```
Paste URL → confirm name & icon → Build for This Mac → Install & Open
```

- **Native builder** — the GUI is drawn by [MyGo](https://github.com/egoist/mygo) in pure Go (no Electron, no web frontend).
- **Tiny, independent apps** — generated apps use the system WebView (WKWebView / WebView2 / WebKitGTK), have their own identity, icon and isolated profile, and keep working if Mypack is uninstalled.
- **Prebuilt runtime + config injection** — no compiling on your machine. The runtime is downloaded once from a signed (Ed25519) registry, verified and cached.
- **Manage what you built** — Library › My Apps: Install & Open, one-click Update (keeps your logins), Repair, Uninstall.
- **Agent-ready** — headless CLI with `--json` and stable exit codes, plus an MCP stdio server (`mypack mcp serve`). No built-in chat or LLM.
- **Seven languages** — English, 繁體中文, 简体中文, 粵語, Español, Français, 日本語.
- **Discover** — dozens of popular sites, one click to make an app. The list is [`catalog/discover.json`](catalog/discover.json): suggest more with a pull request.
- **Library sync** — keep your apps in step across computers through a shared folder such as iCloud Drive.

See [ARCHITECTURE.md](ARCHITECTURE.md) for the implementation design.

## Building from source

Requires Go ≥ 1.27.3 (`GOTOOLCHAIN=auto` fetches it automatically).

```sh
make runtimes   # cross-compile + sign generated-app runtimes into dist/registry
make app        # build dist/Mypack.app (macOS)
make test
```

## Using it

- **App:** open `dist/Mypack-1.0.0.dmg` and drag Mypack to Applications (or `open dist/Mypack.app`).
- **CLI:** the same binary is the CLI — `Mypack.app/Contents/MacOS/mypack help`.
  `mypack create github.com --json`, `mypack build <id> --yes --json`, exit codes in ARCHITECTURE.md.
- **Agents (MCP):** `Mypack.app/Contents/MacOS/mypack mcp serve --transport stdio`
  (Settings › Agent Connections shows copy-ready config for Claude Code / Codex).
- **Browser Companion (optional):** build it (`cd apps/browser-companion && pnpm install && pnpm build`), load
  `apps/browser-companion/.output/chrome-mv3` unpacked in Chrome/Edge, then
  `Mypack.app/Contents/MacOS/mypack-native-host install --extension-id <ID> --browser all`.
  See [docs/companion.md](docs/companion.md).
- **Demo mode:** `make run-fake` runs the GUI against an in-memory demo service.

Dev runtime registry: `make runtimes` signs `dist/registry` with a dev key in `dist/keys/`
(never commit it); `make app` embeds that registry URL and public key into the build.
A production release needs a hosted registry and a real signing key.
Forks that distribute builds should host their own registry and sign it with their own key;
official Mypack builds only trust the project's registry key.

## Contributing

Bug reports, website compatibility reports, platform testing, translations and
code are all welcome. Start with [CONTRIBUTING.md](CONTRIBUTING.md). Please
report security problems privately as described in [SECURITY.md](SECURITY.md).

## License

Mypack is licensed under the [Apache License 2.0](LICENSE); third-party notices
are in [NOTICE](NOTICE). The name "Mypack" and its logo are trademarks and are
not covered by the code license. If you distribute a modified build, please give
it a different name and icon; see [TRADEMARKS.md](TRADEMARKS.md).
