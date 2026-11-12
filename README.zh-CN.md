<div align="center">

# Mypack

**任何网站。任何桌面。一个漂亮的原生构建工具。**

只需点击几下，就能把网站或本地 HTML 项目，变成真正、独立的桌面应用。
无需准备 Go、Rust、Node 或 Xcode 项目。

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![CI](https://github.com/sinhong2011/mypack/actions/workflows/ci.yml/badge.svg)](https://github.com/sinhong2011/mypack/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27.3%2B-00ADD8?logo=go&logoColor=white)](go.mod)
![Platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey)
[![PRs welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Listed on Awesome MyGo](https://awesomemygo.dev/badge.svg)](https://awesomemygo.dev/apps/mypack)

[English](README.md) · [繁體中文](README.zh-TW.md) · **简体中文** · [粵語](README.yue.md) · [Español](README.es.md) · [Français](README.fr.md) · [日本語](README.ja.md)

[功能](#功能) · [开始使用](#开始使用) · [CLI 与智能体](#cli-与-ai-智能体) · [参与贡献](#参与贡献) · [灵感](#灵感与致谢)

![Mypack 资源库](docs/screenshots/real-library.png)

</div>

```
粘贴网址  →  确认名称与图标  →  构建  →  安装并打开
```

Mypack 以 [Apache License 2.0](LICENSE) 免费开源。
所有功能都包含在内：没有应用数量限制、不需要许可证密钥、不必注册账户。

## 为什么选择 Mypack？

我们有很多工作是在网页应用里完成的，但浏览器标签页并不是理想的归宿。
标签页容易埋没在其他标签之中，每个网站共用同一组 cookie，而且它们都没有自己的 Dock 图标、通知或窗口。

Mypack 让每个网站成为独立的应用，并持续帮你打理：

- **你的电脑上不需要工具链。** Mypack 不会编译任何东西。它使用小型的预构建 runtime，把你的应用设置写入其中，因此构建只需几秒钟。
- **应用可以独立运行。** 每个应用都有自己的名称、图标和独立的登录状态，使用系统 WebView，即使你卸载 Mypack 也还能继续使用。
- **不只是做出来，还会持续维护。** 一键更新应用，并且不会丢失登录状态（如果更新失败，会回退到先前的版本），也可以修复，或迁移到另一台电脑。

## 功能

**制作应用**

- 🌐 **来自任何网站**：Mypack 会获取名称和最佳图标。两者都可以更改，也可以在图标工作室里自行设计。
- 📁 **来自本地项目**：HTML 文件、包含 `index.html` 的文件夹，或 Vite、CRA、Astro、Nuxt、SvelteKit、Next 的导出结果。Mypack 会先运行项目的构建，但只在你批准之后。
- 🧭 **发现**：数十个热门网站，一键即可开始。列表是任何人都可以贡献的纯 [JSON 文件](catalog/discover.json)。
- 🗂️ **组合应用**：多个网站放在同一个窗口中，带有侧边栏、每个网站独立的会话，以及分栏视图。
- 📥 **Pake 配置**：可以把你的 [Pake](https://github.com/tw93/Pake) `pake.json` 一并带过来。

**你做出来的应用里**

- 🔔 网页通知，以及未读数量的 Dock 标记
- 🔗 你的 Mypack 应用之间的链接会在正确的应用中打开
- 🪟 窗口模式：窗口、紧凑、侧边栏，或放在菜单栏
- 📑 标签页，并可固定页面
- 🛡️ 内容拦截、自定义 CSS 与 JavaScript、proxy 与 user-agent 设置
- 🔑 登录弹出窗口（Google、Microsoft 等）可直接使用

**管理它们**

- 📚 资源库收录你做过的所有内容，支持安装并打开、更新、修复与卸载
- 🔄 通过 iCloud Drive 这类共享文件夹，在多台电脑之间同步资源库，或导出、导入 `.mypack` 文件
- 🧩 可选的浏览器扩展，把你当前的标签页做成应用
- 🌏 七种语言：English、繁體中文、简体中文、粵語、Español、Français、日本語

**面向开发者和 AI 智能体**

- ⌨️ 无界面 CLI，提供 `--json` 输出和稳定的退出码
- 🤖 内置 [MCP](https://modelcontextprotocol.io) 服务器，让 Claude Code、Codex 和其他智能体可以替你创建并构建应用。Mypack 本身没有内置聊天或 LLM。

## 工作原理

```
               ┌───────────── Mypack（构建工具） ─────────────┐
  网站 /       │  名称与图标  →  mypack.json  →  组装应用    │      你的应用
  文件夹   ──▶ │                                       ▲        │ ──▶  .app / .exe /
               └───────────────────────────────────────┼────────┘      Linux 文件夹
                                                       │
                          预构建的 runtime，只会从已签名（Ed25519）
                          的注册表下载一次，验证后缓存起来
```

runtime 是每个平台一个小巧的二进制文件，对任何特定网站一无所知。Mypack 复制它，加入你的应用名称、图标和 `mypack.json`，并在 macOS 上为结果签名。细节见
[ARCHITECTURE.md](ARCHITECTURE.md)。

| | Mypack（构建工具） | 它制作的应用 |
|---|---|---|
| **macOS**（Apple silicon 与 Intel） | ✅ 主要平台，打包为 `.app` / `.dmg` | ✅ WKWebView |
| **Linux**（x64 与 arm64） | 🧪 从源码构建 | ✅ WebKitGTK，已在 Ubuntu 24.04 上测试 |
| **Windows**（x64 与 arm64） | 🧪 从源码构建 | 🧪 WebView2，测试较少 |

## 开始使用

### 安装

**macOS**，使用 [Homebrew](https://brew.sh)：

```sh
brew install --cask sinhong2011/tap/mypack
```

**Windows**，使用 winget、[Scoop](https://scoop.sh) 或 [Chocolatey](https://chocolatey.org)：

```powershell
winget install Mypack.Mypack
```

```powershell
scoop bucket add mypack https://github.com/sinhong2011/scoop-bucket
scoop install mypack/mypack
```

```powershell
choco install mypack
```

**Debian 与 Ubuntu**（APT）：

```sh
curl -fsSL https://sinhong2011.github.io/mypack/mypack.gpg | sudo tee /usr/share/keyrings/mypack.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/mypack.gpg] https://sinhong2011.github.io/mypack/apt stable main" | sudo tee /etc/apt/sources.list.d/mypack.list
sudo apt update && sudo apt install mypack
```

**Fedora、RHEL 与 openSUSE**（DNF、YUM、Zypper）：

```sh
sudo curl -fsSL -o /etc/yum.repos.d/mypack.repo https://sinhong2011.github.io/mypack/rpm/mypack.repo
sudo dnf install mypack        # sudo zypper install mypack
```

**Arch Linux**，从 [AUR](https://aur.archlinux.org/packages/mypack-bin)：

```sh
yay -S mypack-bin
```

每个 [Release](https://github.com/sinhong2011/mypack/releases/latest) 也提供可直接下载的安装程序：macOS 的 `.dmg`、Windows 的安装程序，以及 Linux 的 `.deb` 与 `.rpm`。通过包管理器安装后，`mypack` 命令就在 PATH 中，之后用该包管理器的升级命令更新即可。

> [!NOTE]
> 发行版本尚未签名。macOS 若被 Gatekeeper 拦截，请右键点击 Mypack.app 并选择“打开”；Windows 的 SmartScreen 可能会要求确认（“更多信息 › 仍要运行”）。

### 从源码构建

你需要 Go 1.27.3 或更新版本。若使用 `GOTOOLCHAIN=auto`（默认值），Go 会自动为你获取正确的版本。

```sh
git clone https://github.com/sinhong2011/mypack.git
cd mypack
make runtimes   # build the app runtimes and sign them with a local dev key
make app        # build dist/Mypack.app (macOS)
open dist/Mypack.app
```

`make dmg` 还会生成可拖到 Applications 的磁盘映像。若只想先看看而不构建任何东西，`make run-fake` 会以演示数据启动应用。

### 制作你的第一个应用

1. 点击 **新建应用**（⌘N），粘贴地址，例如 `github.com`。
2. 检查名称与图标。
3. 点击 **构建**，然后点击 **安装并打开**。

你也可以把 HTML 文件或文件夹拖放到窗口上，或在 **发现** 里挑选网站。

## CLI 与 AI 智能体

应用的可执行文件同时也是 CLI：

```sh
alias mypack=/Applications/Mypack.app/Contents/MacOS/Mypack   # not needed after a package-manager install

mypack create github.com --json          # a draft project; nothing is built yet
mypack build <project-id> --yes --json   # build it (--yes allows the runtime download)
mypack install <artifact-id> --open
mypack apps list
```

使用 `--json` 时，每条命令会打印一个 JSON 对象。退出码是稳定的：`0` 成功、`1` 错误、`2` 用法不正确、`3` 输入无效、`4` runtime 或网络、`5` 构建失败、`6` 需要批准、`130` 已取消。运行 `mypack help` 可查看所有命令。

**MCP 服务器。** 用以下方式连接智能体：

```sh
claude mcp add mypack -- mypack mcp serve
```

应用里的 **设置 › 智能体连接** 提供可直接复制的 Claude Code、Codex 和其他 MCP 客户端配置。智能体可以列出、创建和构建项目，但不能安装应用；项目的构建脚本也只有在你批准后才会运行。

**浏览器扩展（可选）。** 请见 [docs/companion.md](docs/companion.md)。

## 项目结构

```
cmd/          入口：应用、CLI、浏览器 native-messaging host
runtime/      Mypack 做出的每个应用里的 runtime
internal/     构建工具、安装程序、服务、存储、CLI、MCP 服务器与应用的 UI
pkg/config/   mypack.json 格式（以及 Pake 导入）
catalog/      发现目录，discover.json
apps/         浏览器扩展（WXT + SolidJS）
```

## 参与贡献

各种规模的贡献都欢迎：

- **把网站加到发现**：编辑 [`catalog/discover.json`](catalog/discover.json)。
  一天内就会送达每一位用户，不必等新版本发布。
- **报告表现异常的网站**：请使用 *Website compatibility* 议题表单。
- **在 Linux 与 Windows 上测试**：这是目前最需要帮助的地方。
- **翻译、修复缺陷、改进文档。**

请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md)。安全问题请按 [SECURITY.md](SECURITY.md) 的说明私下报告，并遵守
[Code of Conduct](CODE_OF_CONDUCT.md)。

## 灵感与致谢

Mypack 站在两个项目的肩膀上：

- **[Pake](https://github.com/tw93/Pake)**，作者是 [tw93](https://github.com/tw93)
  展示了网站作为小巧、专注的桌面应用可以有多好用，以及有多少人想要这样的体验。Mypack 用不同的方式实践同一个想法：用预构建的 runtime 取代构建工具链，并提供原生应用，让你制作并长期管理自己的应用。它能读取 Pake 配置，所以你可以把已有的配置带过来。
- **[MyGo](https://github.com/egoist/mygo)**，作者是 [egoist](https://github.com/egoist)
  是 Mypack 界面与它所制作应用背后的纯 Go 原生 UI 工具包：没有 Electron、没有网页前端、没有 cgo。

也感谢 [Lucide](https://lucide.dev) 提供图标，以及
[modernc.org/sqlite](https://gitlab.com/cznic/sqlite)。所有第三方声明
都在 [NOTICE](NOTICE)。

## 许可

Mypack 采用 [Apache License 2.0](LICENSE) 许可。

“Mypack” 这个名称及其标志是商标，不在代码许可的范围内。若你分发修改过的构建版本，请使用不同的名称与图标；详见 [TRADEMARKS.md](TRADEMARKS.md)。分发构建版本的 fork 应自行托管 runtime 注册表，并用自己的密钥签名。
