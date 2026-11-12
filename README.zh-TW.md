<div align="center">

# Mypack

**任何網站。任何桌面。一個漂亮的原生建構工具。**

只要點幾下，就能把網站或本機 HTML 專案，變成真正、獨立的桌面 App。
不必準備 Go、Rust、Node 或 Xcode 專案。

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![CI](https://github.com/sinhong2011/mypack/actions/workflows/ci.yml/badge.svg)](https://github.com/sinhong2011/mypack/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27.3%2B-00ADD8?logo=go&logoColor=white)](go.mod)
![Platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey)
[![PRs welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

[English](README.md) · **繁體中文** · [简体中文](README.zh-CN.md) · [粵語](README.yue.md) · [Español](README.es.md) · [Français](README.fr.md) · [日本語](README.ja.md)

[功能](#功能) · [開始使用](#開始使用) · [CLI 與代理程式](#cli-與-ai-代理程式) · [貢獻](#貢獻) · [靈感](#靈感與致謝)

![Mypack 資料庫](docs/screenshots/real-library.png)

</div>

```
貼上網址  →  確認名稱與圖示  →  建立  →  安裝並開啟
```

Mypack 以 [Apache License 2.0](LICENSE) 免費開源。
所有功能都包含在內：沒有 App 數量限制、不需授權金鑰、不必註冊帳號。

## 為什麼選擇 Mypack？

我們有很多工作是在網頁應用程式裡完成的，但瀏覽器分頁並不是理想的家。
分頁容易埋在其他分頁之中，每個網站共用同一組 cookie，而且它們都沒有自己的 Dock 圖示、通知或視窗。

Mypack 讓每個網站成為獨立的 App，並且持續幫你打理：

- **你的電腦上不需要工具鏈。** Mypack 不會編譯任何東西。它使用小型的預先建置 runtime，把你的 App 設定寫進去，所以建立只需幾秒鐘。
- **App 可以獨立運作。** 每個 App 都有自己的名稱、圖示和獨立的登入狀態，使用系統 WebView，即使你解除安裝 Mypack 也還能繼續使用。
- **不只是做出來，還會持續照顧。** 一鍵更新 App，而且不會弄丟登入狀態（如果更新失敗，會回到先前的版本），也可以修復，或搬到另一台電腦。

## 功能

**製作 App**

- 🌐 **來自任何網站**：Mypack 會擷取名稱與最佳圖示。兩者都可以改，也可以在圖示工作室裡自己設計。
- 📁 **來自本機專案**：HTML 檔、含 `index.html` 的資料夾，或 Vite、CRA、Astro、Nuxt、SvelteKit、Next 的匯出結果。Mypack 會先執行專案的建置，但只在你核准之後。
- 🧭 **探索**：數十個熱門網站，一鍵就能開始。清單是任何人都可以貢獻的純 [JSON 檔](catalog/discover.json)。
- 🗂️ **組合 App**：多個網站放在同一個視窗，具有側邊欄、每個網站獨立的工作階段，以及分割檢視。
- 📥 **Pake 設定**：可以把你的 [Pake](https://github.com/tw93/Pake) `pake.json` 一併帶進來。

**你做出來的 App 裡**

- 🔔 網頁通知，以及未讀數量的 Dock 標記
- 🔗 你的 Mypack App 之間的連結會在正確的 App 開啟
- 🪟 視窗模式：視窗、精簡、側邊欄，或放在選單列
- 📑 分頁，並可釘選頁面
- 🛡️ 內容阻擋、自訂 CSS 與 JavaScript、proxy 與 user-agent 設定
- 🔑 登入彈出視窗（Google、Microsoft 等）可直接使用

**管理它們**

- 📚 資料庫收錄你做過的所有東西，支援安裝並開啟、更新、修復與解除安裝
- 🔄 透過 iCloud Drive 這類共用資料夾，在多台電腦之間同步資料庫，或匯出、匯入 `.mypack` 檔
- 🧩 可選的瀏覽器擴充功能，把你目前的分頁做成 App
- 🌏 七種語言：English、繁體中文、简体中文、粵語、Español、Français、日本語

**給開發者與 AI 代理程式**

- ⌨️ 無介面 CLI，提供 `--json` 輸出與穩定的結束代碼
- 🤖 內建 [MCP](https://modelcontextprotocol.io) 伺服器，讓 Claude Code、Codex 和其他代理程式可以替你建立 App。Mypack 本身沒有內建聊天或 LLM。

## 運作方式

```
               ┌───────────── Mypack（建構工具） ─────────────┐
  網站 /       │  名稱與圖示  →  mypack.json  →  組裝 App    │      你的 App
  資料夾   ──▶ │                                       ▲        │ ──▶  .app / .exe /
               └───────────────────────────────────────┼────────┘      Linux 資料夾
                                                       │
                          預先建置的 runtime，只會從已簽名（Ed25519）
                          的登錄庫下載一次，驗證後快取起來
```

runtime 是每個平台一個小巧的二進位檔，對任何特定網站一無所知。Mypack 複製它，加入你的 App 名稱、圖示和 `mypack.json`，並在 macOS 上為結果簽名。細節請見
[ARCHITECTURE.md](ARCHITECTURE.md)。

| | Mypack（建構工具） | 它製作的 App |
|---|---|---|
| **macOS**（Apple silicon 與 Intel） | ✅ 主要平台，封裝為 `.app` / `.dmg` | ✅ WKWebView |
| **Linux**（x64 與 arm64） | 🧪 從原始碼建置 | ✅ WebKitGTK，已在 Ubuntu 24.04 測試 |
| **Windows**（x64 與 arm64） | 🧪 從原始碼建置 | 🧪 WebView2，測試較少 |

## 開始使用

> [!NOTE]
> Mypack 還很新，已簽名的發行版本即將推出。在那之前，請從原始碼建置。大約一分鐘就能完成。

### 從原始碼建置

你需要 Go 1.27.3 或更新版本。若使用 `GOTOOLCHAIN=auto`（預設值），Go 會自動為你取得正確的版本。

```sh
git clone https://github.com/sinhong2011/mypack.git
cd mypack
make runtimes   # build the app runtimes and sign them with a local dev key
make app        # build dist/Mypack.app (macOS)
open dist/Mypack.app
```

`make dmg` 也會做成可拖到 Applications 的磁碟映像。若只想先看看而不建置任何東西，`make run-fake` 會以示範資料啟動應用程式。

### 建立你的第一個 App

1. 點一下 **新增 App**（⌘N），貼上地址，例如 `github.com`。
2. 檢查名稱與圖示。
3. 點一下 **建立**，然後點 **安裝並開啟**。

你也可以把 HTML 檔或資料夾拖放到視窗上，或在 **探索** 裡挑選網站。

## CLI 與 AI 代理程式

應用程式的執行檔同時也是 CLI：

```sh
alias mypack=/Applications/Mypack.app/Contents/MacOS/mypack

mypack create github.com --json          # a draft project; nothing is built yet
mypack build <project-id> --yes --json   # build it (--yes allows the runtime download)
mypack install <artifact-id> --open
mypack apps list
```

使用 `--json` 時，每條指令會印出一個 JSON 物件。結束代碼是穩定的：`0` 成功、`1` 錯誤、`2` 用法不正確、`3` 輸入無效、`4` runtime 或網路、`5` 建立失敗、`6` 需要核准、`130` 已取消。執行 `mypack help` 可查看所有指令。

**MCP 伺服器。** 用以下方式連接代理程式：

```sh
claude mcp add mypack -- /Applications/Mypack.app/Contents/MacOS/mypack mcp serve
```

App 裡的 **設定 › 代理程式連線** 提供可直接複製的 Claude Code、Codex 和其他 MCP 客戶端設定。代理程式可以列出、建立和建置專案，但不能安裝 App；專案的建置指令碼也只有在你核准後才會執行。

**瀏覽器擴充功能（選用）。** 請見 [docs/companion.md](docs/companion.md)。

## 專案結構

```
cmd/          進入點：應用程式、CLI、瀏覽器 native-messaging host
runtime/      Mypack 做出的每個 App 裡的 runtime
internal/     建構工具、安裝程式、服務、儲存、CLI、MCP 伺服器與 App 的 UI
pkg/config/   mypack.json 格式（以及 Pake 匯入）
catalog/      探索目錄，discover.json
apps/         瀏覽器擴充功能（WXT + SolidJS）
```

## 貢獻

各種規模的貢獻都歡迎：

- **把網站加到探索**：編輯 [`catalog/discover.json`](catalog/discover.json)。
  一天內就會送到每一位使用者，不必等新版本發行。
- **回報運作異常的網站**：請使用 *Website compatibility* 議題表單。
- **在 Linux 與 Windows 上測試**：這是目前最需要協助的地方。
- **翻譯、修正錯誤、改進文件。**

請先閱讀 [CONTRIBUTING.md](CONTRIBUTING.md)。安全性問題請依 [SECURITY.md](SECURITY.md) 的說明私下回報，並遵守
[Code of Conduct](CODE_OF_CONDUCT.md)。

## 靈感與致謝

Mypack 站在兩個專案的肩膀上：

- **[Pake](https://github.com/tw93/Pake)**，作者是 [tw93](https://github.com/tw93)
  展示了網站作為小巧、專注的桌面 App 可以有多好用，以及有多少人想要這樣的體驗。Mypack 用不同的方式實踐同一個想法：用預先建置的 runtime 取代建置工具鏈，並提供原生應用程式，讓你製作並長期管理自己的 App。它能讀取 Pake 設定，所以你可以把既有的設定帶過來。
- **[MyGo](https://github.com/egoist/mygo)**，作者是 [egoist](https://github.com/egoist)
  是 Mypack 介面與它所製作 App 背後的純 Go 原生 UI 工具組：沒有 Electron、沒有網頁前端、沒有 cgo。

也感謝 [Lucide](https://lucide.dev) 提供圖示，以及
[modernc.org/sqlite](https://gitlab.com/cznic/sqlite)。所有第三方聲明
都在 [NOTICE](NOTICE)。

## 授權條款

Mypack 採用 [Apache License 2.0](LICENSE) 授權。

「Mypack」這個名稱及其標誌是商標，不在程式碼授權的範圍內。若你散布修改過的建置版本，請使用不同的名稱與圖示；詳見 [TRADEMARKS.md](TRADEMARKS.md)。散布建置版本的 fork 應自行託管 runtime 登錄庫，並用自己的金鑰簽名。
