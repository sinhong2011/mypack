<div align="center">

# MyPack

**任何網站。任何桌面。一個靚嘅原生建立工具。**

只需撳幾下，就可以將一個網站或者本機 HTML 專案，變成真正、獨立嘅桌面 App。
唔使自己開 Go、Rust、Node 或者 Xcode 專案。

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![CI](https://github.com/sinhong2011/mypack/actions/workflows/ci.yml/badge.svg)](https://github.com/sinhong2011/mypack/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27.3%2B-00ADD8?logo=go&logoColor=white)](go.mod)
![Platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey)
[![PRs welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Listed on Awesome MyGo](https://awesomemygo.dev/badge.svg)](https://awesomemygo.dev/apps/mypack)

[English](README.md) · [繁體中文](README.zh-TW.md) · [简体中文](README.zh-CN.md) · **粵語** · [Español](README.es.md) · [Français](README.fr.md) · [日本語](README.ja.md)

[功能](#功能) · [開始使用](#開始使用) · [CLI 同 agent](#cli-同-ai-agent) · [參與貢獻](#參與貢獻) · [靈感](#靈感同致謝)

![MyPack 資料庫](docs/screenshots/real-library.png)

</div>

```
貼上網址  →  確認名稱同圖示  →  建立  →  安裝並開啟
```

MyPack 以 [Apache License 2.0](LICENSE) 免費開源。
所有功能都包晒喺內：冇 App 數量限制、唔使授權金鑰、亦唔使註冊帳戶。

## 點解要用 MyPack？

我哋好多工作都係喺網頁應用程式度做，但瀏覽器分頁並唔係一個理想嘅屋企。
分頁好易埋沒喺其他分頁之中，每個網站共用同一組 cookie，而且佢哋都冇自己嘅 Dock 圖示、通知或者視窗。

MyPack 令每個網站都成為獨立嘅 App，仲會繼續幫你睇住：

- **你部機唔使裝工具鏈。** MyPack 唔會編譯任何嘢。佢用細細嘅預先建置 runtime，將你嘅 App 設定寫入去，所以建立只需幾秒鐘。
- **App 可以獨立運作。** 每個 App 都有自己嘅名稱、圖示同獨立嘅登入狀態，用系統 WebView，就算你解除安裝 MyPack 都仲用得。
- **唔只係整出嚟，仲會持續照顧。** 一鍵更新 App，而且唔會搞丟登入狀態（如果更新失敗，會返返前一個版本），亦可以修復，或者搬去另一部電腦。

## 功能

**整 App**

- 🌐 **由任何網站嚟**：MyPack 會攞名稱同最佳圖示。兩個都可以改，亦可以喺圖示工作室度自己設計。
- 📁 **由本機專案嚟**：HTML 檔、入面有 `index.html` 嘅資料夾，或者 Vite、CRA、Astro、Nuxt、SvelteKit、Next 嘅匯出結果。MyPack 會先跑專案嘅建置，但只會喺你核准之後先進行。
- 🧭 **探索**：數十個熱門網站，撳一下就可以開始。清單係任何人都可以貢獻嘅純 [JSON 檔](catalog/discover.json)。
- 🗂️ **組合 App**：幾個網站放喺同一個視窗，有側邊欄、每個網站獨立嘅工作階段，同埋分割檢視。
- 📥 **Pake 設定**：可以將你嘅 [Pake](https://github.com/tw93/Pake) `pake.json` 一併帶過嚟。

**你整出嚟嘅 App 入面**

- 🔔 網頁通知，同埋未讀數量嘅 Dock 標記
- 🎵 音樂同影片網站支援媒體鍵同「正在播放」（macOS 同 Linux；Windows 本身已經有）
- 🔗 你嘅 MyPack App 之間嘅連結會喺正確嘅 App 開啟
- 🪟 視窗模式：視窗、精簡、側邊欄，或者放喺選單列
- 📑 分頁，而且可以釘選頁面
- 🛡️ 內容阻擋、自訂 CSS 同 JavaScript、proxy 同 user-agent 設定
- 🔑 登入彈出視窗（Google、Microsoft 等）可以直接用

**管理佢哋**

- 📚 資料庫收齊你整過嘅所有嘢，支援安裝並開啟、更新、修復同解除安裝
- 🔄 透過 iCloud Drive 呢類共用資料夾，喺多部電腦之間同步資料庫，或者匯出、匯入 `.mypack` 檔
- 🧩 可選嘅瀏覽器擴充功能，將你而家開緊嘅分頁做成 App
- 🌏 七種語言：English、繁體中文、简体中文、粵語、Español、Français、日本語

**畀開發者同 AI agent**

- ⌨️ 無介面 CLI，提供 `--json` 輸出同穩定嘅結束代碼
- 🤖 內建 [MCP](https://modelcontextprotocol.io) 伺服器，等 Claude Code、Codex 同其他 agent 可以幫你建立 App。MyPack 本身冇內建傾偈或者 LLM。

## 點樣運作

```
               ┌───────────── MyPack（建立工具） ─────────────┐
  網站 /       │  名稱同圖示  →  mypack.json  →  組裝 App    │      你嘅 App
  資料夾   ──▶ │                                       ▲        │ ──▶  .app / .exe /
               └───────────────────────────────────────┼────────┘      Linux 資料夾
                                                       │
                          預先建置嘅 runtime，只會由已簽名（Ed25519）
                          嘅登錄庫下載一次，驗證之後快取起嚟
```

runtime 係每個平台一個細細嘅二進位檔，對任何特定網站都一無所知。MyPack 複製佢，加入你嘅 App 名稱、圖示同 `mypack.json`，並喺 macOS 上為結果簽名。細節請見
[ARCHITECTURE.md](ARCHITECTURE.md)。

| | MyPack（建立工具） | 佢整出嚟嘅 App |
|---|---|---|
| **macOS**（Apple silicon 同 Intel） | ✅ 主要平台，封裝成 `.app` / `.dmg` | ✅ WKWebView |
| **Linux**（x64 同 arm64） | 🧪 由原始碼編譯 | ✅ WebKitGTK，已喺 Ubuntu 24.04 測試 |
| **Windows**（x64 同 arm64） | 🧪 由原始碼編譯 | 🧪 WebView2，測試較少 |

## 開始使用

### 安裝

**macOS**，用 [Homebrew](https://brew.sh)：

```sh
brew install --cask sinhong2011/tap/mypack
```

**Windows**，用 winget、[Scoop](https://scoop.sh) 或者 [Chocolatey](https://chocolatey.org)：

```powershell
winget install MyPack.MyPack
```

```powershell
scoop bucket add mypack https://github.com/sinhong2011/scoop-bucket
scoop install mypack/mypack
```

```powershell
choco install mypack
```

**Debian 同 Ubuntu**（APT）：

```sh
curl -fsSL https://sinhong2011.github.io/mypack/mypack.gpg | sudo tee /usr/share/keyrings/mypack.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/mypack.gpg] https://sinhong2011.github.io/mypack/apt stable main" | sudo tee /etc/apt/sources.list.d/mypack.list
sudo apt update && sudo apt install mypack
```

**Fedora、RHEL 同 openSUSE**（DNF、YUM、Zypper）：

```sh
sudo curl -fsSL -o /etc/yum.repos.d/mypack.repo https://sinhong2011.github.io/mypack/rpm/mypack.repo
sudo dnf install mypack        # sudo zypper install mypack
```

**Arch Linux**，由 [AUR](https://aur.archlinux.org/packages/mypack-bin)：

```sh
yay -S mypack-bin
```

每個 [Release](https://github.com/sinhong2011/mypack/releases/latest) 都有安裝程式可以直接下載：macOS 嘅 `.dmg`、Windows 嘅安裝程式，同埋 Linux 嘅 `.deb` 同 `.rpm`。用套件管理員裝咗之後，`mypack` 指令就喺 PATH 入面，之後用嗰個套件管理員嘅升級指令更新就得。

> [!NOTE]
> 發行版本仲未簽名。macOS 如果俾 Gatekeeper 擋咗，喺 MyPack.app 撳右掣揀「打開」；Windows 嘅 SmartScreen 可能會要你確認（「其他資訊 › 仍要執行」）。

### 由原始碼編譯

你需要 Go 1.27.3 或者更新版本。若使用 `GOTOOLCHAIN=auto`（預設值），Go 會自動幫你攞正確嘅版本。

```sh
git clone https://github.com/sinhong2011/mypack.git
cd mypack
make runtimes   # build the app runtimes and sign them with a local dev key
make app        # build dist/MyPack.app (macOS)
open dist/MyPack.app
```

`make dmg` 亦會整成可以拖去 Applications 嘅磁碟映像。如果只係想先睇下、唔建置任何嘢，`make run-fake` 會用示範資料啟動應用程式。

### 整你第一個 App

1. 撳 **新增 App**（⌘N），貼上地址，例如 `github.com`。
2. 檢查名稱同圖示。
3. 撳 **建立**，然後撳 **安裝並開啟**。

你亦可以將 HTML 檔或者資料夾拖放去視窗度，或者喺 **探索** 度揀網站。

## CLI 同 AI agent

應用程式嘅執行檔同時亦係 CLI：

```sh
alias mypack=/Applications/MyPack.app/Contents/MacOS/MyPack   # not needed after a package-manager install

mypack create github.com --json          # a draft project; nothing is built yet
mypack build <project-id> --yes --json   # build it (--yes allows the runtime download)
mypack install <artifact-id> --open
mypack apps list
```

用 `--json` 嘅時候，每條指令會印出一個 JSON 物件。結束代碼係穩定嘅：`0` 成功、`1` 錯誤、`2` 用法不正確、`3` 輸入無效、`4` runtime 或者網路、`5` 建立失敗、`6` 需要核准、`130` 已取消。執行 `mypack help` 可以睇晒所有指令。

**MCP 伺服器。** 用以下方式連接 agent：

```sh
claude mcp add mypack -- mypack mcp serve
```

App 入面嘅 **設定 › Agent 連線** 提供可以直接複製嘅 Claude Code、Codex 同其他 MCP 客戶端設定。Agent 可以列出、建立同建置專案，但唔可以安裝 App；專案嘅建置指令碼亦只有喺你核准之後先會執行。

**瀏覽器擴充功能（選用）。** 請見 [docs/companion.md](docs/companion.md)。

## 專案結構

```
cmd/          進入點：應用程式、CLI、瀏覽器 native-messaging host
runtime/      MyPack 整出嚟嘅每個 App 入面嘅 runtime
internal/     建立工具、安裝程式、服務、儲存、CLI、MCP 伺服器同 App 嘅 UI
pkg/config/   mypack.json 格式（以及 Pake 匯入）
catalog/      探索目錄，discover.json
apps/         瀏覽器擴充功能（WXT + SolidJS）
```

## 參與貢獻

各種規模嘅貢獻都歡迎：

- **將網站加到探索**：編輯 [`catalog/discover.json`](catalog/discover.json)。
  一日之內就會送到每一位使用者，唔使等新版本發行。
- **回報運作唔正常嘅網站**：請使用 *Website compatibility* 議題表單。
- **喺 Linux 同 Windows 上測試**：呢度係而家最需要幫手嘅地方。
- **翻譯、修正錯誤、改進文件。**

請先閱讀 [CONTRIBUTING.md](CONTRIBUTING.md)。安全性問題請跟 [SECURITY.md](SECURITY.md) 嘅說明私下回報，並遵守
[Code of Conduct](CODE_OF_CONDUCT.md)。

## 靈感同致謝

MyPack 企喺兩個專案嘅肩膀上：

- **[Pake](https://github.com/tw93/Pake)**，作者係 [tw93](https://github.com/tw93)
  展示咗網站作為細巧、專注嘅桌面 App 可以有幾好用，以及有幾多人想要咁嘅體驗。MyPack 用另一種方式實踐同一個諗法：用預先建置嘅 runtime 取代建置工具鏈，並提供原生應用程式，等你可以整同長期管理自己嘅 App。佢能讀取 Pake 設定，所以你可以將已有嘅設定帶過嚟。
- **[MyGo](https://github.com/egoist/mygo)**，作者係 [egoist](https://github.com/egoist)
  係 MyPack 介面同佢所整出嚟 App 背後嘅純 Go 原生 UI 工具組：冇 Electron、冇網頁前端、冇 cgo。

亦多謝 [Lucide](https://lucide.dev) 提供圖示，以及
[modernc.org/sqlite](https://gitlab.com/cznic/sqlite)。所有第三方聲明
都喺 [NOTICE](NOTICE)。

## 授權條款

MyPack 採用 [Apache License 2.0](LICENSE) 授權。

「MyPack」呢個名稱同埋佢嘅標誌係商標，唔屬於程式碼授權範圍。如果你散布修改過嘅建置版本，請用唔同嘅名稱同圖示；詳見 [TRADEMARKS.md](TRADEMARKS.md)。散布建置版本嘅 fork 應該自行託管 runtime 登錄庫，並用自己嘅金鑰簽名。
