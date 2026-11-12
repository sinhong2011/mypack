# GUI handoff: window modes (runtime 1.0.6)

Menu-bar, compact and sidebar window modes.
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

# Group apps (runtime 1.0.6)

One app made of several websites, like a workspace in Zen: a rail of the sites' icons, each site in its own session, with separate logins.

## Config
- `app.group.members[]`: `{id, name, url | local (+entry), icon?, projectId?}`, from 2 to 12 members. A member is a website (`url`) or a local folder or HTML file (`local`), never both. `id` is stable (`[a-z0-9-]`, at most 32 characters) and keys the member's session (`<profileId>.<id>`), so renaming a member keeps its logins. Helpers: `Config.IsGroup()`, `config.MemberID(name, taken)`.
- `app.source` is the first website member (or, with only local members, the first folder), so identifiers, installs, routes and metadata work as for any app. The runtime opens the members, never Source.
- Local members: the Builder copies each folder (or the HTML file's folder) to `members/<id>/site` and rewrites `local`/`entry`. The group's Source is not bundled a second time.
- `icon` is a PNG path in a project. The Builder bundles it as `members/<id>.png`. With no icon, the rail draws the member's initial. Before a build, the service fills in missing icons from the member's Library project (`fillGroupIcons`).

## Service
- `CreateGroup(GroupRequest{Name, Sites: []GroupSite{ProjectID | URL | Path}})` keeps the site order. Addresses get their name and icon fetched, and a repeated site is dropped. A `Path` is a folder with index.html or an HTML file. A JavaScript project joins with its built output and is refused while unbuilt, because a group never runs build scripts. Library apps built from local folders join as local members. The group icon is a 2×2 grid of the first four member icons (`icons.Group`). Member icons are stored beside the database in `groups/<project>/`.
- Routes: an installed group claims every member's site, so links from other Mypack apps open in it.

## Runtime
- Every member loads at start (so each can notify and badge) and is kept alive while hidden. A local member is served at `mypack://<member id>/` (a private loopback server per member on Linux), which gives each one its own origin and storage. The rail shows each member's unread count, and the Dock (or the menu-bar title) shows the **sum**.
- Rail position: down the left in a window, compact or right-docked sidebar; down the right in a left-docked sidebar; as a row along the top in the menu-bar panel.
- A user's click on a link to another member's site opens it in that member. An exact host wins. A domain shared by two members (Gmail and Calendar on google.com) goes to the member whose exact host matches, otherwise the link stays put. Sign-in pages on a member's own domain stay in that member.
- **Split view**: ⌘\ toggles it, or use the rail's context menu (Open in Split View). The divider can be dragged, and a double-click resets it to 50/50. Clicking the split member swaps the sides.
- Shortcuts: ⌘1…⌘9 pick a member; Ctrl+Tab and Ctrl+Shift+Tab move to the next or previous member. Reload, Home and zoom act on the visible member (zoom applies to all members). The last member shown is remembered (`state.json` `groupActive`).
- Notification clicks show the member that sent them. Links handed to the app open in the member they belong to.

## GUI
- **Create (New App)**: after the first address or folder, "Add another site" (paste an address, choose a folder or HTML file, or pick a Library app) turns the app into a group. The name defaults to "Workspace", the icon becomes the grid, a chip per site can be removed, and the preview draws the rail in the chosen Show as mode. The toolbar above the inspector holds the settings search and Build.
- **New Group…** (⌘⇧N, the Library header, the command palette) opens a sheet for grouping existing Library apps or pasted addresses.
- **Group settings**: a Sites card at the top (icons; move up or down and remove on hover; "Add a site" from the Library).

## Not done
- Choosing which members load at start (all load today).
