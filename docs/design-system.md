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

Springs everywhere, all interruptible:

- pages slide in from the way you travelled, the old one easing back;
- an app's icon flies between screens: from its Library tile, sidebar row
  or ⌘K row to the project hero and home again, and from Create into the
  build scene;
- staggered arrivals (30 ms), tiles that re-flow as you filter, and
  removals that collapse;
- indicators that glide between items, and a toolbar that firms up as the
  page scrolls under it;
- labels that roll when they change, and buttons that morph into their
  loading state;
- sheets and the palette that spring in (.96 → 1) while the page behind
  blurs.

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
10. **Keep feedback fast and story motion slow** (*Stageshow*). Hover and press
    take 140–260 ms springs. Storytelling motion takes 600 ms or more on
    travel/arrive curves. Grids and lists stagger 30 ms.

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
| Pages are scrolls keyed by the route, so a new route is a new element that transitions in | `c.Key(route)`, `ElementTransition{Enter, Exit}` | `kit.PageSwap(page, dir)`: slide 28 pt from the way of travel; the old page eases back 12 pt and to .985 (`SpringPage`, 280 ms) |
| The now-playing cover enters from the side it is on in the play order and the old one leaves the other way, scaled .92 | `Motion{X, Scale}`, direction from the order | `App.travel` ranks sections, so going deeper slides from the right and coming back from the left |
| Rows of a list roll in one after another | `Animation.Delay` per row (30 ms) | `kit.Arrive` / `kit.Block` / `kit.Stagger`; `StaggerList` = `StaggerGrid` = 30 ms |
| Popovers and menus come in at .94 and leave at .96 on `Snappy` | `Enter/Exit: &Motion{Scale}` | `kit.Pop`; sheets and ⌘K at .96 → 1 on `Spring` |
| Hover lifts artwork; press sinks | `AnimatePose(Pose{Y, Scale})` | `kit.Lift`, `kit.Press` (press .98) |
| Inserted and removed comments collapse their height | `Motion{Collapse: true}` | tiles leave with `Collapse` and .92, so the grid closes up |
| Scroll position tracked per page | `TrackScroll(&ScrollState)` | `pageScroll`: the toolbar's subtitle folds away and a hairline appears as content scrolls under it |
| Blur as a transition material | `Element.Blur` | the page blurs (6 pt, on a spring) behind sheets and ⌘K |

mypack adds the shared icon flight (`app/motion.go`): the source notes its
icon's rect in the window (`Element.Bounds`), the destination hides its own
icon until the flight lands, and one overlay carries the icon on a spring
with a slight arc. (MyGo's `Match`/`Fly` did not take across our keyed page
swap, so the flight is explicit.)

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
| Staggers | `StaggerList` 35, `StaggerGrid` 55 ms |
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
| `Arrive` | enter from below and fade; leave upward; staggered |
| `Pop` | grow in |
| `Reveal` | open and close its own height |
| `Roll` | a label rolls up when it changes |
| `PageSwap` | page change: slide from the way of travel, old page eases back |
| `Block` | a page's blocks stagger in after it |
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
  - On the right is a fixed 364 pt inspector, set off by a hairline and
    scrolling on its own: Look & Behaviour with compact thumbnails, the
    rows, Advanced Settings and the targets.
  - The inspector's footer is pinned: the download hint and the "Build for
    This Mac" pill.
  - When a build starts, the build scene plays in the preview, with no page
    jump, and then the success state. The footer then offers View Project
    and New App.
  - Below about 900 pt of window width it becomes one column.
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
