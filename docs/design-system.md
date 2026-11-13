# mypack design concept

## The idea: light comes from the content

mypack is a quiet instrument for other people's websites. Its own surfaces
step back:

- flat and modern: a near-neutral canvas inset in a slightly darker chrome,
  flat fills and 1 px hairlines, no textures or bevels;
- shadows only where something floats, and no boxes for their own sake;
- one vivid accent, the blue of the mypack icon, used only for action and
  selection.

Colour comes from the sites themselves:

- Every app is a crisp squircle floating on the canvas with a soft, neutral
  shadow.
- A project's page carries a faint wash of the site's colour, taken from
  its icon.
- The build scene is tinted by it.

Confident typography carries the hierarchy: large, tight display names,
tabular numerals, and small open-tracked eyebrows. The chrome nearly
disappears.

## The signature

1. **Floating icons, light from the content.** The Library and Discover are
   generous grids of each site's real icon, normalised to one crisp squircle
   with a soft neutral shadow. Hover lifts an icon and fades in its quick
   actions. The colour of a site appears only as a soft wash of the page
   behind it, never as a shape under the icon.
2. **The build scene.** A website becomes an app, flat and native: the
   site's browser window is checked, its address bar fills with the
   download, it morphs into the app's icon, a verified badge springs on,
   and the icon drops into a Dock and bounces as it launches.
3. **⌘K.** One keyboard-first palette reaches every app, project and action.
   Typing a URL offers "Make app from …".

## Recognisable in one second

A quiet graphite (or warm white) canvas inset from the window edge. A big
tight title. Then a grid of real app icons floating as crisp squircles, and a
single blue accent taken from the mypack logo. No shelves, no paper, no glass
blobs.

## How it moves

Motion only where it explains a change of state; content otherwise just
shows. All of it is interruptible:

- a page change is a quick 140 ms fade-in of the new page, with no slide;
  Settings switches panes at once, as a settings window does;
- lists and grids show at once (no staggered arrivals); a removed tile or
  row folds its space away so its neighbours close the gap;
- the sidebar pill and segmented indicators glide between items, and the
  toolbar firms up as the page scrolls under it;
- controls answer input: hover and press colours (140 ms), a press sink on
  icons, switches, checkboxes, disclosure chevrons and progress;
- sheets, menus and the palette spring in (.96 → 1) while the page behind
  blurs;
- the build scene is the one story: the site's window becomes its app.

Every motion has a cause and payoffs hold. Reduce Motion turns all of it
into settled states.

---

# mypack design system

mypack has its own design language. It is meant to look calm and tactile, with
an editorial feel and one accent colour, and it looks the same on macOS, Windows
and Linux. The only per-platform differences are the window controls (traffic
lights on the left on macOS, caption buttons on the right elsewhere), shortcut
glyphs (⌘ or Ctrl) and file-manager wording.

Layers, each depending only on the one before it:

```
internal/desktop/design   tokens: colour ramps → semantic roles, type, space, radius,
                          elevation, surfaces, motion  (the single source of truth)
internal/desktop/kit      mypack's motion-first components on base MyGo ui
                          (+ kit/icons: the vendored Lucide icons we use, ISC)
internal/desktop/app      screens: compose kit components, read roles, never raw values
```

`lobe-mygo` is gone from go.mod. The Liquid Glass plugin and SF Symbols are no
longer used.

## Never

These rules hold on every screen. A screen that breaks one is a bug.

- No eyebrow label above a row of three feature cards.
- No cards made of an icon in a tinted square, a bold title and a line.
  Settings rows show the glyph alone.
- No compositions that centre everything. Align to the left content edge
  by default; centre only the build scene and true empty states.
- No hero headlines on utility screens: Discover, Create, an empty
  Library, build results. The toolbar names the screen; content starts
  with the content.
- No gradients, glows or glossy sweeps on UI surfaces.
- No decorative, emoji-like icons (rockets, sparkles, magic wands). Use an
  icon only where it says something, such as a folder on "Show in Finder".
- No duplicate status: one source of truth per state. A result on screen
  gets no toast; toasts are for builds that finish while you are elsewhere.
- At most one primary button per view. Secondary actions are ghost
  buttons or 32 pt circular icon buttons; no rows of dot-separated text
  links.
- Copy is specific and short, with no "You can…" or "Let's…" filler.
- Density over decoration.

## Colour: the primary comes from the logo

The accent is extracted from `assets/AppIcon-1024.png`. Its blue runs from
`#27A7FA` at the top, through `#1578FA` (the dominant, and so the primary),
to `#0657F7` at the bottom. These are `design.LogoLight`, `LogoMid` and
`LogoDeep`, and the `Blue` ramp is built around them as steps 4, 6 and 8.

- **Dark mode** uses the same hue lifted for contrast (step 5), not a
  different blue.
- **No brand gradient in the UI.** The logo keeps its gradient. Buttons and
  fills use the flat `#1578FA` (`design.BrandFill` is now a flat fill).
- **Changing the logo.** Re-measure it (`scripts/logocolor`), update the
  three anchors, and the ramp, buttons, focus and selection all follow.

## Research: 10 takeaways

These came from looking at desktop apps known for their craft: Linear, Raycast,
Arc, Things 3, Craft, Amie, Superlist, Notion Calendar, Screen Studio,
CleanShot and Mercury. They also draw on two references the owner sent.

1. **Use one confident accent.** Linear, Things and Mercury keep colour for
   meaning: the selection, the primary action, a status. Everything else stays
   in quiet greys. mypack uses the brand blue from the app icon and nothing
   else.
2. **Put content on a canvas inset from the chrome.** Arc and Linear inset
   the content panel from the window edges. mypack's canvas is inset 8 pt,
   rounded 14 and set apart by a flat fill and a 1 px hairline.
3. **Take colour from the content.** Arc and the Apple App Library let the
   content colour the interface. mypack keeps near-neutral, faintly warm
   ramps ("Paper" and "Ink") and takes every other colour from the apps
   themselves, as the light under their icons.
4. **Build hierarchy with type, not boxes.** Things and Craft use a big tight
   display line, small uppercase eyebrows and secondary text, and draw few
   borders. mypack uses Display 28/−0.7, Title 20/−0.3, Eyebrow 10.5 caps
   +0.7, and Body 13.
5. **Stay flat; let light and motion do the depth.** Surfaces are flat
   fills with 1 px hairlines. One soft ambient shadow marks what floats
   (menus, sheets, ⌘K, toasts), and a very soft one sits under app icons.
   Nothing is bevelled, embossed, grained or pressed in. In mypack this is
   `design.Elevate`.
6. **Motion has a cause and a held payoff** (Benji Taylor, *Stageshow*). Every
   change follows an input or an earlier motion. Nothing pops: things arrive and
   leave. The result holds for a beat. Labels roll in place, and only the part
   that changed moves.
7. **Simplify, but keep it real** (*Stageshow*'s five failure modes: too much,
   too little, boxed, flat, bland). The build scene shows the site as a real
   window with its real icon and name, plus placeholder lines in its own
   colour. It is not a pasted screenshot and not a wireframe.
8. **Selections glide.** Linear's sidebar and segmented controls, and Amie's
   tabs, move one indicator from the old item to the new one. In mypack the
   sidebar pill, the segmented controls, the grid/list switch and the project
   tabs share one glide helper.
9. **Make a signature moment.** CleanShot has its capture overlay and Arc its
   spaces. mypack's is the build scene: the site's window morphs into its
   app icon, is signed with a badge, and lands in the Dock.
10. **Keep feedback fast and story motion rare** (*Stageshow*). Hover and press
    take 140–260 ms. Only the build scene tells a story (600 ms or more).
    Navigation, lists and grids do not animate their arrival.

**Inspora (Product and Motion).** The gallery lazy-loads its images, so the
tools could read only titles such as "Usage Meter Component", "Project Card
Component", "Folders UI" and "Agent Dashboard", not the visuals. What carried
over is the calm, tactile direction: fine tick-like detail, neutral surfaces
that feel physical, and colour kept for the meaning.

**mygo-music (motion study only; its source was read, nothing copied;
AGPL).** Its techniques, and where mypack uses the same idea with its own
code:

| mygo-music technique | MyGo API | mypack usage |
|---|---|---|
| Pages are scrolls keyed by the route, so a new route is a new element that transitions in | `c.Key(route)`, `ElementTransition{Enter, Exit}` | `kit.PageSwap(page)`: the new page fades in over 140 ms; the old one goes at once |
| Popovers and menus come in at .94 and leave at .96 on `Snappy` | `Enter/Exit: &Motion{Scale}` | `kit.Pop`; sheets and ⌘K at .96 → 1 on `Spring` |
| Press sinks | `AnimatePose(Pose{Scale})` | `kit.Press` (press .98); tile icons sink to .97. No hover lift |
| Removed comments collapse their height | `Motion{Collapse: true}` | `kit.Arrive`: tiles and rows leave with `Collapse` and .96, so the grid closes up |
| Scroll position tracked per page | `TrackScroll(&ScrollState)` | `pageScroll`: the toolbar's subtitle folds away and a hairline appears as content scrolls under it |
| Blur as a transition material | `Element.Blur` | the page blurs (6 pt, on a spring) behind sheets and ⌘K |

## Tokens (`internal/desktop/design`)

**Colour.** Primitive ramps:

| Ramp | Steps | Use |
|---|---|---|
| `Paper` | 13 | light neutrals |
| `Ink` | 13 | dark neutrals |
| `Blue` | 10 | brand; step 6 = `#1474f6`, the icon's blue |
| `Green` / `Amber` / `Red` | soft / base / strong | status |

Semantic roles for each appearance (`design.Light`, `design.Dark`):

| Group | Roles |
|---|---|
| Surfaces | `Chrome` (s0, window/sidebar), `Window` (s1, canvas), `Raised` (s2, cards, fields, buttons), `Overlay` (s3, sheets, toasts, popovers), `Sunken` (wells) |
| Strokes | `Border`, `BorderStrong`, `Divider` (1 px hairlines) |
| Text | `Text`, `TextSecondary`, `TextTertiary`, `TextDisabled`, `TextOnAccent` |
| Accent | `Accent`, `AccentHover`, `AccentPressed`, `AccentSoft`, `AccentText` |
| Status | `Success`, `Warning`, `Danger`, each with a `*Soft` |
| Fills | `Fill`, `Hover`, `Pressed`, `Selected`, `Alt` (table rows) |
| Fields | `FieldRest` (one step sunken), `FieldFocus` (one step brighter), `FieldStroke` (the 1 px hairline of fields and choices) |
| Selection | `NavSelected` (text/primary at 7 %, 9 % in dark), `NavHover` (4 %), `ChoiceFill` (accent at 8–10 %) |
| Buttons | `ButtonFill`/`ButtonHover` (secondary, flat), `ButtonStroke` (its hairline); primary is the flat `Accent` |
| Focus | no rings: the theme's ring is transparent; `kit.FocusMark` shows keyboard focus |

`design/contrast_test.go` checks WCAG AA. On every surface, primary,
secondary and accent text must reach 4.5:1, and tertiary text 3:1. The label
on an accent fill must reach 3:1.

**Type.** One stack, decided once: the platform UI sans (SF Pro, Segoe UI
Variable, or the desktop sans), set at mypack's own sizes:

| Style | Size | Weight / tracking |
|---|---|---|
| Micro/eyebrow | 10.5 | +0.7, caps |
| Caption | 11 | |
| Small | 12 | |
| Body | 13 | |
| Headline | 15 | semibold |
| Title2 | 17 | |
| Title | 20 | −0.3 |
| Display | 28 | bold −0.7 |

Mono is Menlo, Cascadia Mono or monospace.

**Space and radius.** A 4 pt grid (S1–S12).

- **One rule for buttons.** Every button is a pill, with radius = height/2
  (`kit.PillRadius`), and every icon-only button is a circle. This covers
  primary, secondary, ghost, danger, link, icon, toolbar and sheet-footer
  buttons, the segmented thumbs, and the New App and Open pills. All of
  them go through `kit.Button`, `IconButton`, `ToolbarIcon` and `NewPill`.
  `kit/buttons_test.go` checks every variant and size.
- **Other surfaces keep their token radii:** fields 10 (`RadiusField`),
  rows and the sidebar pill 8, cards 12, the canvas 14 and panels 16.
- App icons use 22.37 % continuous corners.
Controls are 24/30/36/44 high and rows 36/44/52.

**Flat surfaces, hairlines, light.**

- Every surface is a flat fill: the chrome, the canvas, cards, fields and
  buttons.
- Edges are 1 px hairlines (`Border`, `FieldStroke`, `ButtonStroke`).
- `Elevate(e, Floating|Modal)` gives what floats over the window one soft
  ambient shadow. `Flat` and `Lifted` add nothing.
- App icons get one very soft shadow (`IconShadow`).
- Colour comes in as tints: the accent's 8–10 % selection fill, and a faint
  wash of a site's colour on its project page.
- Depth and life come from motion, not material. No grain or noise, no
  dot-grid textures, no bevels or embossing, no inner top highlights, no
  pressed-in wells, no shelves, no glossy sweeps.
- `design.Well` is a flat sunken fill with a hairline (logs, code, path
  chips). `RaisedControl` and `Float` remain as no-ops for older callers.

**Motion.**

| Kind | Tokens |
|---|---|
| Durations | `MotionInstant` 90, `Fast` 140, `Base` 220, `Slow` 320, `Scene` 640 ms |
| Easings | `EaseTravel` (0.45,0,0.1,1), `EaseArrive` (0.25,0.8,0.25,1), `EaseStd` |
| Springs | `Spring` 260 ms / bounce .16, `SpringSoft` 320/0, `SpringPop` 540/.42, `SpringGlide` 620/.06 |

`SetSpeed` scales all of these; the kit gallery uses it. MyGo's animations
already honour Reduce Motion, and painted scenes check `kit.Reduced`.

## Kit (`internal/desktop/kit`)

The kit contains only what mypack uses. Every component reads tokens and has
its motion built in.

**The motion engine (`motion.go`).**

| Helper | What it does |
|---|---|
| `Arrive` | shows at once; leaves by folding its space away |
| `Pop` | grow in |
| `Reveal` | open and close its own height |
| `Roll` | a label rolls up when it changes |
| `PageSwap` | page change: a quick fade-in of the new page |
| `MarkModal` / `ModalOpen` | a sheet or palette is up: blur the page behind |
| `Press` / `Lift` | hover lift, press sink |
| `Glide` | shared-element indicator |

It also holds pure maths, unit-tested: `Seg`, `Lerp`, `EaseOutCubic`,
`EaseInOutCubic`, `EaseOutBack`, `SpringAt` and `Shake`.

**Components.**

- **Buttons.** `Button` comes in primary, secondary, ghost, danger, link and
  toolbar variants. Primary is the flat logo accent `#1578FA` (lifted in
  dark), with no gradient, highlight or glow, and its label white
  at 600. Secondary is a flat subtle fill with a 1 px hairline (white 8 %
  in dark). Pressed is .98 and 8 % darker. The loading state fades the label
  under a spinner in the same slot. There are also `IconButton`,
  `GlassIcon` (toolbar chip; its plus turns on hover), `CopyButton` (its icon
  becomes a check) and `Kbd`.
- **Fields.**
  - `FieldStyle` is the one field look: a sunken fill with a 1 px hairline
    at rest. Focused, the fill brightens a step and the hairline darkens a
    little (140 ms), with the caret. Nothing is drawn outside the field: no
    ring, no halo, no glow.
  - `TextField` and `TextArea` use it, and shake once when the value turns
    invalid.
  - `SearchField`.
  - `URLField`, the hero: paste button, drop target, and continue.
- **Controls.**
  - `Switch`: the thumb stretches while pressed and travels on a spring.
  - `Checkbox`: the check draws itself on.
  - `Segmented`: the indicator glides.
  - `PopUp`: the native select, themed.
  - `Disclosure`, paired with `Reveal`.
  - `TargetChip`, `ChoiceTile` (thumbnail pickers) and `ChoiceStyle`:
    selected is an 8–10 % accent fill with a small filled `CheckBadge` and
    no outline ring; unselected is a hairline, and hover is a quiet fill.
    The chosen label is primary text at 600; the others are secondary.
  - `NavRow`, `NavPill` and `NavCount`: the sidebar's soft selection pill
    (primary text at 7 %, 9 % in dark; radius 8; no border or shadow) that
    glides between rows, a 4 % hover, and neutral count pills (accent only
    for updates).
- **Toolbar** (`toolbar.go`): one quiet cluster of 30 pt pills, 8 pt
  apart, sitting on the canvas with no box of their own.
  - `ToolbarSearch`: a sunken pill with a magnifier, "Search" and a ⌘K chip.
    It widens on a spring to 280 pt while in use. On the Library it filters
    in place; elsewhere it opens the ⌘K palette.
  - `PillSegmented`: the Library's grid/list toggle and the project's
    Overview/Settings tabs. A soft track with no border and a raised thumb
    that glides.
  - `NewPill`: "New App" with a plus in the accent; a 30 pt circle when the
    toolbar is narrow.
  - `ToolbarIcon`: back, more and plus as quiet circles that fill on hover.
  - As the page scrolls under the toolbar, the canvas fade firms up and a
    hairline appears.
- **Focus without rings.** Nothing rings a control when it is clicked.
  Focus that arrived by keyboard (`FocusVisible`: Tab or arrow keys) shows
  as the hover fill plus a 1.5 pt accent underline inset at the control's
  foot (`kit.FocusMark`). Buttons, icon buttons, toolbar pills, segments,
  sidebar rows, choice tiles, target chips, switches, checkboxes,
  disclosures and Library tiles all use it.
- **Grouped rows and status.**
  - `Group`, `GroupBox`, `SettingRow`, `SettingIconRow`, `SettingBlock`,
    `SymbolTile` and `KeyValues`.
  - `StatusPill`, `StatusDot`, `Note`, `EmptyState` and `Card`.
  - `TableHeader` and `TableRowBG` (alternating rows, hover, selection).
- **Overlays.** `Sheet`: a dimmed backdrop and a panel that springs in from
  under the toolbar and leaves the same way. `DialogBase` traps focus.
  `SheetHeader`, `SheetFooter` (Return triggers the default), `Toasts` (slide
  up, stack, collapse out, with action and close) and native tooltips.
- **Progress and celebration.** `Spinner`, `ProgressRing`, `ProgressBar`
  (tweened) and `Shimmer` (skeleton).
- **App icons.**
  - `AppIcon` / `AppIconOpts` crop the art to its body, add a squircle and
    layered shadow, and fall back to a generated letter tile.
  - `IconBadge`, `IconTint` and `BrandMark`.
- **The build scene.** `BuildScene` is the signature component; see below.

Accessibility is built in:

- Every control has a role and a label, and icons are hidden from assistive
  technology.
- Sheets are dialogs: they hold the focus and close with Escape.
- Segmented controls and tabs report their selection.
- Scenes are images labelled with the site's name.

### Screen-level patterns (internal/desktop/app)

- **⌘K command palette** (`palette.go`):
  - Fuzzy search over apps, projects and actions: Open, Build, New App,
    sections, appearance, language and import.
  - Typing a website address offers "Make app from …".
  - Keyboard-first (↑↓ ↩ esc), with a spring-in panel over the dimmed window.
- **Preview + inspector** (`inspector.go`): Create and a project's Settings
  tab share one layout.
  - On the left is the live preview: the URL field, the app icon
    (shimmering while the site is looked up), its name to rename in place,
    its host, a compact "couldn't reach the site" hint when needed, and the
    Dock.
  - On the right is the inspector: about 38 % of the content area, kept
    between 400 and 480 pt. It is set off by a hairline and scrolls on its
    own: Look & Behaviour, the rows, Advanced Settings and the targets.
  - The pickers are one family of single rows on one grid: Title bar (2),
    Appearance (3) and Window size (4: Compact, Standard, Large, Fill).
    - Every slot is (inner width − 3 × 10) / 4, so tiles align across rows
      and never clip, at 400 pt as at 480 pt.
    - Each tile keeps a 0.7 aspect, with a 12 pt label under it.
    - Window sizes add their pixels under the label in 10.5 pt tabular
      numerals ("1200 × 780", "Full screen").
  - Setting rows keep the label on the left and the control on the right.
  - The Look & Behaviour rows are Stay signed in, Links, Notifications
    (Allow / Ask / Off), Dock badge for unread count, Keep running in the
    menu bar, and the Account label.
  - **Advanced Settings is an accordion.**
    - Each group is one full-width row with hairlines between: a 16 pt
      icon, the name, an accent dot when it differs from the recommended
      defaults, a one-line summary of its values (such as "Standard ·
      System · 1200 × 780") and a chevron that turns as it opens.
    - Groups open in place, and several can be open; this is remembered
      per project. ↑/↓ move between rows, and Space or Return toggles.
    - Each open group has a quiet Reset link, and "Reset to Recommended
      Defaults" is a link at the bottom.
    - Search filters to matching settings, opens their groups and tints
      the matches; clearing it restores what was open.
    - Order: Window & Appearance, WebView & Navigation, Customization,
      Native, Network & Privacy, Identity & Version, Runtime &
      Compatibility, Packaging, Diagnostics.
  - Advanced adds the user agent as a preset (Safari, Chrome, Edge,
    Firefox, Mobile Safari, or Custom, which shows the text field) and
    "Open links to my other MyPack apps in those apps".
  - While a build runs from Create, the inspector is held: it is quieter
    (0.8) but still readable, and does not take clicks.
  - The build action is one pill ("Build & Open" or "Build for This Mac")
    in a centred footer at the bottom of the left column. ⌘B builds too.
    "Install & Open when done" is the inspector's first row.
  - When a build starts, the stage plays the scene and the footer becomes
    a live step list (about 420 pt wide), one 28 pt row per step the build
    actually uses: Build project (JS projects), Get app engine (only when
    a download is needed), Make it an app, Sign, Check, Install & Open
    (when that switch is on). Each row has a 16 pt glyph (hollow circle,
    spinner, a check that draws on in ~220 ms, a red ✕, a dash), the label
    and, on the right, the live detail or the step's time ("2.1s"). The
    running row opens under its label (a 2 pt bar for a download, the
    latest output line for a project build); three or more finished steps
    fold into "✓ 3 steps done · 4.2s". Cancel (Esc) sits under the list.
    The menu-bar flyout and the HUD show the same list, compact. The
    detail and the scene's address bar read one clock, and progress never
    goes backwards.
  - On success the list gives way to the result bar and the stage shows
    the app launched: its window springs open at its size, appearance and
    title-bar style, the favicon in its title. The app's icon appears once
    per view (the identity block); the mockup has no Dock. The result bar
    is a 56 pt pill with the name and state (no icon), a hairline, then Open (or an
    "Opened" chip that offers Open again), Show in Finder, Rebuild, View
    log and a "⋯" menu (View Project, Add Another Account…, Copy Path,
    Reveal Build Folder), with one caption of size and build time under
    it. A failure keeps the list, its failed row open, and the bar says
    "Build failed · Step 3 of 5" with Retry, View log and Copy diagnostic.
  - Failures are said where they happen, never in a box: under the failed
    step (or in the update, repair or uninstall sheet), what happened in
    one plain sentence, up to two small fixes ("Get Node.js", "Project
    Settings", "Open Settings", "Manage Apps"), the remediation when no
    button covers it, and a folded "Details" with the code, the cause and
    the last lines of the log (monospace, sunken, Copy). Red is the ✕ and
    the word "failed", nothing else. Return opens
    (or retries), ⌘R rebuilds, ⌘L shows the log. The project page uses
    the same bar. New App stays in the toolbar, as a quiet plus while on
    Create.
  - The project's Settings tab is the same: Build or Rebuild (and an Open
    link when installed) sits under the preview. The toolbar drops its
    Build there.
  - Below about 960 pt of window width it becomes one column.
- **Build result** (`buildPanel`, in the preview column):
  - The scene's last frame is the hero: the app settled in the Dock.
  - Under it, aligned to the content edge: the app's name (Title 20) and
    one quiet line, such as "Installed in Applications · opened just now ·
    11.2 MB · 7 s".
  - At most one primary action: "Install & Open", only when nothing is
    installed yet.
  - Everything else is one row of links: Open again · Show in Finder ·
    Rebuild · View log · Add another account…
  - No headline, no eyebrow, no cards and no path bar; the path shows in
    View log. No toast while the result is on screen.
  - A running build shows a title, a rolling stage line, a Cancel link and
    the timeline.
- **Install & Open when done:**
  - A switch sits in Create's footer, on by default, and turns the pill
    into "Build & Open".
  - The same switch is per project in Settings › General, and the default
    for new apps is in Settings › Builds. They are stored GUI-side in
    ui-state.
  - On success the build's artifact for this device is installed and
    opened with `InstallAndOpen`. Only a MyPack-managed install is
    replaced. Anything else asks (the conflict dialog); the free limit
    shows its dialog, and a running app gets "Quit X to update it".
- **Web projects** (a dropped folder with package.json):
  - The preview shows "Vite project · pnpm" under the host.
  - The approval for `project_build` reads "Build TraderMemos first?" and
    shows the exact command in monospace (copyable), the folder and
    whether dependencies install first. "Run & Continue" is the only
    primary action, beside a Cancel link.
  - While `preparing` runs, the stage line reads "Building your
    project…", with the latest output line in monospace under it, and the
    timeline starts with "Build project".
  - Errors:
    - `NODE_NOT_FOUND` offers "Get Node.js".
    - `PROJECT_BUILD_FAILED` shows the last log lines as code.
    - `PROJECT_OUTPUT_MISSING` explains through its remediation, with
      Retry.
- **Whole-window drop:** drag an HTML file or folder anywhere and the window
  becomes a drop target ("Drop to make an app").
- **Project bento:** a hero tile (icon, display name, the one-click Open or
  Build) beside status, size, runtime and last-build tiles, in tabular
  numerals, over a faint wash of the site's colour.
- **One-click Open:**
  - Every Library item can be opened directly: the round ▶ on hover or
    focus, a double-click, Return, or the context menu.
  - An installed app opens; a built project launches its build in place
    (`OpenArtifact`); a draft offers Build.

### The build scene

A website becomes an app. The scene is native, flat and 2D, drawn on the
MyGo canvas (`kit/scene.go`). It is ported frame for frame from the
reference page `internal/desktop/assets/scene/scene.html`, with its spring
constants and timings. It is drawn in a 400×320 space and scaled to fit:
about 300 pt on the project page and 120 pt in the menu-bar flyout, where
the host text and page lines hide below 160 pt.

| Beat | Frame | What happens |
|---|---|---|
| Validating | 1 | The site floats as a browser window: traffic lights, an address bar with its host, a block in its colour and three lines. A scan line in the site's colour sweeps down it. |
| Downloading | 2 | The address bar fills with the real download progress, as a tint and a 2 px bar. |
| Assembling | 3 | The chrome fades, and the window morphs into the app icon on springs (224×150 r16 → 92×92 r22). Its fill turns the site's colour, and the favicon (the real, normalised icon) grows from the address bar (12 pt) to the middle (54 pt). |
| Signing | 4 | A verified badge (r15, `#1578FA`, white ring and check) springs onto the top-right corner, with a ring pulse. |
| Verifying | 5 | One light sweep crosses the icon. |
| Done | 6 | A flat Dock (216×60 r22, a hairline and one soft shadow) slides up with three neutral tiles. The icon shrinks into the empty slot (36×36 r10), bounces twice as macOS launches do, and its running dot appears. |

Failure shakes the icon back into the window. Cancel reverts it to the
window, dimmed. Behind it all there is only a soft radial halo in the
site's colour. The labels stay native under the scene and roll as the
stage changes. Under Reduce Motion each state shows at once. The scene asks
for frames only while a spring moves, a timer waits, the window floats or
the scan runs. Frames: `docs/screenshots/scene-beat-N-{light,dark}.png`.

## Gallery

`MYPACK_KIT_GALLERY=1` opens a hidden page inside mypack. With
`MYPACK_SCENE_BEAT=1…6` it shows one beat of the build scene held still, at
300 pt and at 132 pt. `MYPACK_MOTION_SPEED=0.15` slows every spring for
frame-by-frame review; the motion contact sheets in
`docs/screenshots/motion-*.png` were taken that way. It shows every kit
component in its states, the build scene at each stage, and a slider for
motion speed. It is a developer page, so its words are not translated.
Screenshots are in `docs/screenshots/kit-*.png`.
