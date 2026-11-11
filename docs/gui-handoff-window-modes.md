# GUI handoff: window modes (runtime 1.0.6)

Roadmap P1 item 5 (`docs/research/competitors-2026-11.md`): menu-bar, compact and sidebar modes.
All fields are optional and additive in `mypack.json`; absent = default, and defaults are never written.

## Fields

| Field | Values | Default | GUI |
|---|---|---|---|
| `window.mode` | `"window"`, `"compact"`, `"sidebar"`, `"menubar"` | `window` (empty) | Look & Behaviour › **Show as** (four tiles). Picking Window stores `""`. |
| `window.sidebarEdge` | `"left"`, `"right"` | `right` (empty) | Advanced › Window › **Sidebar edge** (sidebar only). |
| `window.sidebarAutoHide` | bool | false | Advanced › Window › **Hide when not in use** (sidebar only). |
| `window.panelWidth`, `window.panelHeight` | 240–1600 each, 0 = default | 400 × 640 (a sidebar uses the width) | Advanced › Window › **Panel size** (only shown when mode ≠ window). The default is stored as 0. |

Helpers: `WindowConfig.EffectiveMode()`, `WindowConfig.PanelSize()`, `config.RuntimeAtLeast(v, min)`.
`config.DefaultRuntimeVersion` is now `1.0.6`.

## Behaviour

**Menu bar** (`menubar`)
- The app has no Dock icon. The Builder writes `LSUIElement` into Info.plist, but only when the project's runtime is 1.0.6 or later. An older runtime would open a window that has no Dock icon and no menu-bar icon to bring it back.
- A click on the menu-bar icon opens the page in a panel under the icon: a system popover on macOS, a flyout above the taskbar on Windows. The panel is hidden, never destroyed, when the user clicks elsewhere, presses Escape or clicks the icon again, so the page keeps its state and keeps notifying.
- A right-click opens a menu: Open, Open in a Window (the current page in a normal window), Reload, Home, Quit. Linux trays report no clicks, so on Linux that menu is the icon's only action.
- The unread badge is shown as text next to the menu-bar icon.
- The global shortcut (`native.globalShortcut`), notification clicks and second launches open the panel.
- `native.tray`, `hideOnClose`, `startToTray`, the window size and the title bar do not apply. The GUI hides those rows.
- If no icon can be made, the runtime falls back to a normal window with a Dock icon.

**Layouts at runtime** (`window`, `compact`, `sidebar`)
- The app's View menu has Window (⌥⌘W), Compact Window (⌥⌘P) and Sidebar (⌥⌘S), plus Move Sidebar to Left/Right. Each can be turned off with `webview.disabledShortcuts`. Users switch layouts while the app runs, with no rebuild.
- The runtime keeps the user's pick in `state.json` (`layout`, `layoutMode`). It applies only while `window.mode` is the one it was picked under, so a rebuild with another mode starts fresh.

**Sidebar** (`sidebar`)
- The window docks to the left or right edge of its display at the full height of the work area. It stays above other windows and shows on every Space.
- Dragging the inner edge resizes it. The width is remembered (`sidebarWidth`), between 240 pt and the screen width minus 200 pt. A move snaps it back to its edge. The edge the user picks from the View menu wins over `window.sidebarEdge`.
- Closing it hides it. The global shortcut or the Dock icon brings it back. With `sidebarAutoHide` it also hides when another app takes the focus.
- The window size does not apply, and the GUI hides that row.

**Compact** (`compact`)
- The main window starts small (panel size), stays on top, shows on every Space, and sits in the bottom-right corner of the work area or wherever the user last left it.
- Its position is remembered in `<profile>/state.json` (`compactBounds`), and so is the normal window's (`normalBounds`).

## Not done / ideas
- Group apps (workspaces, as in Zen): one generated app that wraps several sites, with a switcher rail, a separate session per site, a combined badge and split view. Needs a runtime that hosts several sites. Not started.
- Menu bar stays a build-time choice, because macOS sets the Dock icon at launch (LSUIElement).
