# mypack

**Any website. Any desktop. One beautiful native builder.**

mypack turns a website (or a local HTML project) into a real, standalone desktop
app for the computer you're on — no Go, Rust, Node or Xcode project required.

```
Paste URL → confirm name & icon → Build for This Mac → Install & Open
```

- **Native builder** — the GUI is drawn by [MyGo](https://github.com/egoist/mygo) in pure Go (no Electron, no web frontend).
- **Tiny, independent apps** — generated apps use the system WebView (WKWebView / WebView2 / WebKitGTK), have their own identity, icon and isolated profile, and keep working if mypack is uninstalled.
- **Prebuilt runtime + config injection** — no compiling on your machine. The runtime is downloaded once from a signed (Ed25519) registry, verified and cached.
- **Manage what you built** — Library › My Apps: Install & Open, one-click Update (keeps your logins), Repair, Uninstall.
- **Agent-ready** — headless CLI with `--json` and stable exit codes, plus an MCP stdio server (`mypack mcp serve`). No built-in chat or LLM.
- **Seven languages** — English, 繁體中文, 简体中文, 粵語, Español, Français, 日本語.

See [ARCHITECTURE.md](ARCHITECTURE.md) for the implementation design and
`mypack_product_technical_spec_v1.7.html` for the product specification.

## Building from source

Requires Go ≥ 1.27.3 (`GOTOOLCHAIN=auto` fetches it automatically).

```sh
make runtimes   # cross-compile + sign generated-app runtimes into dist/registry
make app        # build dist/mypack.app (macOS)
make test
```
