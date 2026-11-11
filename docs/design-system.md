# mypack design system

mypack has its own design language. It is meant to look calm and tactile, with
an editorial feel and one accent colour, and it looks the same on macOS, Windows
and Linux. The only per-platform differences are the window controls (traffic
lights on the left on macOS, caption buttons on the right elsewhere), shortcut
glyphs (⌘ or Ctrl) and file-manager wording.

Layers, each depending only on the one before it:

```
internal/desktop/design   tokens: colour ramps → semantic roles, type, space, radius,
                          elevation, texture, motion  (the single source of truth)
internal/desktop/kit      mypack's motion-first components on base MyGo ui
                          (+ kit/icons: the vendored Lucide icons we use, ISC)
internal/desktop/app      screens: compose kit components, read roles, never raw values
```

`lobe-mygo` is gone from go.mod. The Liquid Glass plugin and SF Symbols are no
longer used.

## Research: 10 takeaways

These came from looking at desktop apps known for their craft: Linear, Raycast,
Arc, Things 3, Craft, Amie, Superlist, Notion Calendar, Screen Studio,
CleanShot and Mercury. They also draw on two references the owner sent.

1. **Use one confident accent.** Linear, Things and Mercury keep colour for
   meaning: the selection, the primary action, a status. Everything else stays
   in quiet greys. mypack uses the brand blue from the app icon and nothing
   else.
2. **Put content on a raised canvas over a textured chrome.** Arc and Linear
   inset the content panel from the window edges, so the sidebar reads as the
   desk and the canvas as the paper. mypack's canvas is inset 8 pt and rounded
   14, with layered shadows.
3. **Use texture you feel more than see.** Screen Studio and Craft use a faint
   grain and paper-warm neutrals instead of clinical greys. mypack's neutral
   ramps are faintly warm ("Paper" and "Ink"), and the chrome carries a film
   grain of 3–5%.
4. **Build hierarchy with type, not boxes.** Things and Craft use a big tight
   display line, small uppercase eyebrows and secondary text, and draw few
   borders. mypack uses Display 28/−0.7, Title 20/−0.3, Eyebrow 10.5 caps
   +0.7, and Body 13.
5. **Give depth with layered shadows and a 1 px top highlight.** This is
   Raycast's and Mercury's trick: raised things get a tight contact shadow, a
   soft ambient one and an inner highlight, so they feel physical without
   gloss. In mypack this is `design.Elevate`.
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
   spaces. mypack's is the build scene: the site folds into the open box from
   the logo, gets sealed and ribboned, and the app springs out.
10. **Keep feedback fast and story motion slow** (*Stageshow*). Hover and press
    take 140–260 ms springs. Storytelling motion takes 600 ms or more on
    travel/arrive curves. Grids stagger 55 ms, lists 35 ms.

**Inspora (Product and Motion).** The gallery lazy-loads its images, so the
tools could read only titles such as "Usage Meter Component", "Project Card
Component", "Folders UI" and "Agent Dashboard", not the visuals. What carried
over is the calm, tactile direction: fine tick-like detail, neutral surfaces
that feel physical, and colour kept for the meaning.

**mygo-music (motion study only, not copied; AGPL).** Its patterns, rebuilt
here with MyGo's own APIs:

- `ElementTransition` with `Enter`/`Exit` `Motion` for presence: things
  arrive, and leave in a collapse.
- `AnimatePose` for cheap hover lift and press sink.
- Springs with small bounces, around 260–600 ms.
- Staggered `Delay` for lists.
- A track that animates its selected index so the capsule slides.

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
| Strokes | `Border`, `BorderStrong`, `Divider`, `Highlight` (the inner top highlight) |
| Text | `Text`, `TextSecondary`, `TextTertiary`, `TextDisabled`, `TextOnAccent` |
| Accent | `Accent`, `AccentHover`, `AccentPressed`, `AccentSoft`, `AccentText` |
| Status | `Success`, `Warning`, `Danger`, each with a `*Soft` |
| Fills | `Fill`, `Hover`, `Pressed`, `Selected`, `Alt` (table rows) |
| Signature | `ShelfTop`/`ShelfBottom` (the app shelf), `Dots` (workbench grid), `Grain` (strength) |

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

**Space and radius.** A 4 pt grid (S1–S12). Radius is 4/6/8 (controls), 12
(cards), 14 (canvas) or 16 (panels). App icons use 22.37% continuous corners.
Controls are 24/30/36/44 high and rows 36/44/52.

**Elevation.** `Elevate(e, Flat|Lifted|Floating|Modal)` applies layered
shadows and the highlight.

**Texture.**

- `PaintGrain` is film grain on the chrome.
- `PaintDots` is the workbench dot grid, with optional vignette. It sits under
  illustrated moments: empty states, the Create hero, the build scene and the
  Discover hero.
- The shelf is a raised ledge that app icons stand on in the Library and on
  Discover.

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
| `PageSwap` | section cross-fade with a rise |
| `Press` / `Lift` | hover lift, press sink |
| `Glide` | shared-element indicator |

It also holds pure maths, unit-tested: `Seg`, `Lerp`, `EaseOutCubic`,
`EaseInOutCubic`, `EaseOutBack`, `SpringAt` and `Shake`.

**Components.**

- **Buttons.** `Button` comes in primary, secondary, ghost, danger, link and
  toolbar variants. Its loading state fades the label under a spinner in the
  same slot, and it sinks when pressed. There are also `IconButton`,
  `GlassIcon` (toolbar chip; its plus turns on hover), `CopyButton` (its icon
  becomes a check) and `Kbd`.
- **Fields.**
  - `TextField` and `TextArea`: focus glow that grows in, and a shake when the
    value turns invalid.
  - `SearchField`.
  - `URLField`, the hero: paste button, drop target, and continue.
- **Controls.**
  - `Switch`: the thumb stretches while pressed and travels on a spring.
  - `Checkbox`: the check draws itself on.
  - `Segmented`: the indicator glides.
  - `PopUp`: the native select, themed.
  - `Disclosure`, paired with `Reveal`.
  - `TargetChip`.
- **Grouped rows and status.**
  - `Group`, `GroupBox`, `SettingRow`, `SettingIconRow`, `SettingBlock`,
    `SymbolTile` and `KeyValues`.
  - `StatusPill`, `StatusDot`, `Note`, `EmptyState` (on dots) and `Card`.
  - `TableHeader` and `TableRowBG` (alternating rows, hover, selection).
- **Overlays.** `Sheet`: a dimmed backdrop and a panel that springs in from
  under the toolbar and leaves the same way. `DialogBase` traps focus.
  `SheetHeader`, `SheetFooter` (Return triggers the default), `Toasts` (slide
  up, stack, collapse out, with action and close) and native tooltips.
- **Progress and celebration.** `Spinner`, `ProgressRing`, `ProgressBar`
  (tweened), `Shimmer` (skeleton) and `Confetti`.
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

### The build scene

The scene is drawn on a canvas in a 400×300 space and scaled to fit, so the
same scene fits the project page (300 pt high) and the menu-bar flyout
(132 pt).

- **Validate:** a miniature of the site (real icon, real name, a band in its
  colour, placeholder lines) is swept by a scan line.
- **Download runtime:** bits rain past the window into the open box. The
  mouth glows brighter as progress rises.
- **Assemble:** the window shrinks and drops into the box, then the flaps
  close.
- **Sign:** a seal stamps down with a dust ring, and a signature draws itself
  across the box.
- **Verify:** a ribbon wraps the box and a check pops.
- **Done:** the flaps burst open, and the app icon springs out with a bounce
  and a burst of sparks over a soft glow. The result holds.

Failure and cancel:

- **Failure:** the box shakes once, half open, with an alert tag.
- **Cancel:** the window stays outside, faded.

Stage labels roll beneath the scene. The real timeline and its progress bar
stay readable below that. When nothing moves, the scene asks for no frames.
Under Reduce Motion it shows each stage's settled pose.

## Gallery

`MYPACK_KIT_GALLERY=1` opens a hidden page inside mypack. It shows every kit
component in its states, the build scene at each stage, and a slider for
motion speed. It is a developer page, so its words are not translated.
Screenshots are in `docs/screenshots/kit-*.png`.
