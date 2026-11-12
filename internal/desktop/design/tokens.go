// Package design is mypack's single source of design truth: primitive color
// ramps → semantic roles for light and dark, the type scale, the 4 pt
// spacing grid, radii, elevation, texture and motion tokens. The kit
// (internal/desktop/kit) reads only these; screens read the kit and the
// semantic roles, never raw colors or sizes. See docs/design-system.md.
package design

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// ---- Primitive ramps -------------------------------------------------------------

// Paper is the neutral ramp for light: a faintly warm grey, as paper and
// stone are, so large neutral areas feel tactile rather than clinical.
var Paper = [13]ui.Color{
	ui.Hex("#ffffff"), ui.Hex("#fbfbfa"), ui.Hex("#f5f5f2"), ui.Hex("#efefeb"),
	ui.Hex("#e7e7e2"), ui.Hex("#ddddd7"), ui.Hex("#cdcdc6"), ui.Hex("#b3b3ab"),
	ui.Hex("#8e8e86"), ui.Hex("#6d6d66"), ui.Hex("#51514c"), ui.Hex("#343431"),
	ui.Hex("#1b1b19"),
}

// Ink is the neutral ramp for dark: graphite with the same faint warmth.
var Ink = [13]ui.Color{
	ui.Hex("#0f0f10"), ui.Hex("#141415"), ui.Hex("#18181a"), ui.Hex("#1d1d1f"),
	ui.Hex("#232325"), ui.Hex("#2b2b2e"), ui.Hex("#37373a"), ui.Hex("#4a4a4e"),
	ui.Hex("#6a6a6f"), ui.Hex("#8c8c91"), ui.Hex("#b1b1b5"), ui.Hex("#d8d8db"),
	ui.Hex("#f3f3f4"),
}

// Blue is the brand ramp, extracted from the logo (see LogoMid).
var Blue = [10]ui.Color{
	ui.Hex("#f1f8ff"), ui.Hex("#e0efff"), ui.Hex("#bfdcfe"), ui.Hex("#8cc6fc"),
	ui.Hex("#27a7fa"), ui.Hex("#1f90fa"), ui.Hex("#1578fa"), ui.Hex("#0e67f8"),
	ui.Hex("#0657f7"), ui.Hex("#0a45c4"),
}

// The logo's own gradient, measured from assets/AppIcon-1024.png: its blue
// runs LogoLight (top) → LogoMid (the dominant, the primary) → LogoDeep
// (bottom). Blue is built around these three anchors (steps 4, 6, 8); a
// new logo re-derives them (scripts/logocolor) and the ramp follows.
var (
	LogoLight = ui.Hex("#27a7fa")
	LogoMid   = ui.Hex("#1578fa")
	LogoDeep  = ui.Hex("#0657f7")
)

// Status hues: soft, base and strong for light; base for dark.
var (
	Green = [3]ui.Color{ui.Hex("#e7f6ec"), ui.Hex("#1f9d55"), ui.Hex("#17773f")}
	Amber = [3]ui.Color{ui.Hex("#fff3dc"), ui.Hex("#c27a00"), ui.Hex("#995f00")}
	Red   = [3]ui.Color{ui.Hex("#fdeceb"), ui.Hex("#d93a31"), ui.Hex("#ae2a22")}

	GreenDark = ui.Hex("#3ccf7a")
	AmberDark = ui.Hex("#f5b543")
	RedDark   = ui.Hex("#ff6a5f")
)

// ---- Semantic roles --------------------------------------------------------------

// Palette is the set of semantic roles for one appearance. Surfaces go
// from the window chrome (0) to overlays (3).
type Palette struct {
	Dark bool

	Chrome  ui.Color // surface/0: the window behind the canvas (sidebar)
	Window  ui.Color // surface/1: the content canvas
	Bg      ui.Color // alias of Window
	Raised  ui.Color // surface/2: cards, fields, buttons
	Overlay ui.Color // surface/3: sheets, popovers, menus, toasts
	Sunken  ui.Color // wells: logs, code
	Sidebar ui.Color // alias of Chrome

	Group         ui.Color // grouped rows on the canvas
	GroupBorder   ui.Color
	Surface       ui.Color // alias of Raised
	SurfaceRaised ui.Color // alias of Overlay
	Well          ui.Color // alias of Sunken
	Fill          ui.Color // quiet fills: tracks, chips, placeholders
	Alt           ui.Color // alternate table rows
	Hover         ui.Color
	Pressed       ui.Color
	Selected      ui.Color // neutral selection pill

	Border       ui.Color // stroke: hairlines
	BorderStrong ui.Color // stroke/strong: control outlines
	Divider      ui.Color

	Text          ui.Color // text/primary
	TextSecondary ui.Color // text/secondary
	TextTertiary  ui.Color // text/tertiary
	TextDisabled  ui.Color
	TextOnAccent  ui.Color

	Accent        ui.Color
	AccentHover   ui.Color
	AccentPressed ui.Color
	AccentSoft    ui.Color
	AccentText    ui.Color

	Success     ui.Color
	SuccessSoft ui.Color
	Warning     ui.Color
	WarningSoft ui.Color
	Danger      ui.Color
	DangerSoft  ui.Color
	DangerHover ui.Color

	ButtonFill  ui.Color
	ButtonHover ui.Color
	FieldFill   ui.Color // alias of FieldRest
	FieldRest   ui.Color // text fields at rest: one step sunken
	FieldFocus  ui.Color // … focused: one step brighter
	FieldStroke ui.Color // the 1 px hairline of a resting field or choice

	NavSelected  ui.Color // the sidebar's selection pill: text/primary at 7–9 %
	NavHover     ui.Color // … and its hover: text/primary at 4 %
	ChoiceFill   ui.Color // a selected choice tile: accent at 8–10 %
	ButtonStroke ui.Color // the hairline of a secondary button

	Shadow     ui.Color // ambient shadow
	ShadowDeep ui.Color // overlays
	IconShadow ui.Color
	Focus      ui.Color
	Scrim      ui.Color
	ChromeTop  ui.Color // warm → cool vertical tint of the chrome
	ChromeBot  ui.Color
	CanvasTop  ui.Color // … and of the canvas
	CanvasBot  ui.Color
}

// Light is the light appearance.
var Light = func() Palette {
	g, b := Paper, Blue
	return Palette{
		Chrome: g[3], Window: g[0], Bg: g[0], Raised: g[0], Overlay: g[0], Sunken: g[2], Sidebar: g[3],
		Group: g[1], GroupBorder: ui.RGBA(27, 27, 25, 0.075), Surface: g[0], SurfaceRaised: g[0], Well: g[2],
		Fill: ui.RGBA(27, 27, 25, 0.05), Alt: ui.RGBA(27, 27, 25, 0.022),
		Hover: ui.RGBA(27, 27, 25, 0.045), Pressed: ui.RGBA(27, 27, 25, 0.08), Selected: ui.RGBA(27, 27, 25, 0.07),
		Border: ui.RGBA(27, 27, 25, 0.08), BorderStrong: ui.RGBA(27, 27, 25, 0.15), Divider: ui.RGBA(27, 27, 25, 0.07),
		Text: g[12], TextSecondary: g[9], TextTertiary: ui.Hex("#82827b"), TextDisabled: g[7], TextOnAccent: g[0],
		Accent: b[6], AccentHover: b[7], AccentPressed: b[8], AccentSoft: b[6].Alpha(0.1), AccentText: b[8],
		Success: Green[1], SuccessSoft: Green[0], Warning: Amber[1], WarningSoft: Amber[0],
		Danger: Red[1], DangerSoft: Red[0], DangerHover: Red[2],
		ButtonFill: g[0], ButtonHover: g[1], FieldFill: g[1], FieldRest: g[1], FieldFocus: g[0], FieldStroke: ui.RGBA(27, 27, 25, 0.13),
		NavSelected: g[12].Alpha(0.07), NavHover: g[12].Alpha(0.04), ChoiceFill: b[6].Alpha(0.08), ButtonStroke: ui.RGBA(27, 27, 25, 0.13),
		Shadow: ui.RGBA(40, 36, 28, 0.08), ShadowDeep: ui.RGBA(40, 36, 28, 0.2), IconShadow: ui.RGBA(30, 26, 20, 0.26),
		Focus: b[6].Alpha(0.9), Scrim: ui.RGBA(30, 28, 24, 0.22),
		ChromeTop: g[3], ChromeBot: g[3], CanvasTop: g[0], CanvasBot: g[0],
	}
}()

// Dark is the dark appearance.
var Dark = func() Palette {
	g, b := Ink, Blue
	return Palette{
		Dark:   true,
		Chrome: g[1], Window: g[2], Bg: g[2], Raised: g[4], Overlay: g[4], Sunken: g[1], Sidebar: g[1],
		Group: ui.RGBA(255, 255, 255, 0.03), GroupBorder: ui.RGBA(255, 255, 255, 0.07), Surface: g[4], SurfaceRaised: g[4], Well: g[1],
		Fill: ui.RGBA(255, 255, 255, 0.06), Alt: ui.RGBA(255, 255, 255, 0.022),
		Hover: ui.RGBA(255, 255, 255, 0.05), Pressed: ui.RGBA(255, 255, 255, 0.09), Selected: ui.RGBA(255, 255, 255, 0.075),
		Border: ui.RGBA(255, 255, 255, 0.075), BorderStrong: ui.RGBA(255, 255, 255, 0.14), Divider: ui.RGBA(255, 255, 255, 0.065),
		Text: g[12], TextSecondary: g[10], TextTertiary: g[9], TextDisabled: g[7], TextOnAccent: ui.RGB(255, 255, 255),
		Accent: b[5], AccentHover: b[4], AccentPressed: b[6], AccentSoft: b[5].Alpha(0.16), AccentText: b[4],
		Success: GreenDark, SuccessSoft: GreenDark.Alpha(0.14), Warning: AmberDark, WarningSoft: AmberDark.Alpha(0.14),
		Danger: RedDark, DangerSoft: RedDark.Alpha(0.14), DangerHover: ui.Hex("#ff857b"),
		ButtonFill: g[5], ButtonHover: g[6], FieldFill: g[1], FieldRest: g[1], FieldFocus: g[3], FieldStroke: ui.RGBA(255, 255, 255, 0.09),
		NavSelected: g[12].Alpha(0.09), NavHover: g[12].Alpha(0.04), ChoiceFill: b[5].Alpha(0.1), ButtonStroke: ui.RGBA(255, 255, 255, 0.08),
		Shadow: ui.RGBA(0, 0, 0, 0.35), ShadowDeep: ui.RGBA(0, 0, 0, 0.6), IconShadow: ui.RGBA(0, 0, 0, 0.55),
		Focus: b[5].Alpha(0.95), Scrim: ui.RGBA(0, 0, 0, 0.4),
		ChromeTop: g[1], ChromeBot: g[1], CanvasTop: g[2], CanvasBot: g[2],
	}
}()

// BrandBlue is the app icon's blue.
var BrandBlue = Blue[6]

// ---- Spacing, radius, metrics ---------------------------------------------------

// Spacing: the 4 pt grid.
const (
	S1  float32 = 4
	S2  float32 = 8
	S3  float32 = 12
	S4  float32 = 16
	S5  float32 = 20
	S6  float32 = 24
	S8  float32 = 32
	S10 float32 = 40
	S12 float32 = 48
)

// Metrics.
const (
	ControlSM      float32 = 24
	Control        float32 = 30
	ControlLG      float32 = 36
	ControlXL      float32 = 44
	ToolbarControl float32 = 30 // every toolbar control, one height

	RowCompact float32 = 36
	Row        float32 = 44
	RowTall    float32 = 52

	RadiusXS      float32 = 4
	RadiusSM      float32 = 6
	RadiusControl float32 = 8
	RadiusField   float32 = 10 // text fields of every size
	RadiusCard    float32 = 12
	RadiusPanel   float32 = 16
	RadiusCanvas  float32 = 14

	SidebarWidth float32 = 228
	CanvasInset  float32 = 8 // the canvas floats this far from the window edges
	TitleBarH    float32 = 52
	// PageGutter is the side inset of every page, level with the toolbar
	// title.
	PageGutter float32 = 20
)

// ---- Typography ----------------------------------------------------------------
//
// One stack, decided once: the platform's UI sans (SF Pro on macOS, Segoe
// UI Variable on Windows, the desktop's sans on Linux) at mypack's own
// sizes, weights and tracking, so the hierarchy is identical everywhere.

const (
	FontMicro   float32 = 10 // eyebrows (uppercase, +0.6 tracking)
	FontCaption float32 = 11
	FontSmall   float32 = 12
	FontBody    float32 = 13
	FontBodyLG  float32 = 14
	FontTitle3  float32 = 15 // headline
	FontTitle2  float32 = 17
	FontTitle1  float32 = 20 // title
	FontDisplay float32 = 28 // display

	FontHeroField float32 = 17

	WeightRegular  = 400
	WeightMedium   = 500
	WeightSemibold = 600
	WeightBold     = 700

	TrackDisplay float32 = -0.7
	TrackTitle   float32 = -0.3
	TrackEyebrow float32 = 0.7
)

// ---- Motion --------------------------------------------------------------------

// Durations.
const (
	MotionInstant = 90 * time.Millisecond
	MotionFast    = 140 * time.Millisecond
	MotionBase    = 220 * time.Millisecond
	MotionSlow    = 320 * time.Millisecond
	MotionScene   = 640 * time.Millisecond

	StaggerList  = 30 * time.Millisecond // rows in reading order
	StaggerGrid  = 30 * time.Millisecond // tiles of a grid
	StaggerBlock = 45 * time.Millisecond // a page's blocks: header, then content
)

// Easings: travel (departure, clear travel, soft arrival) and arrival
// from nowhere.
var (
	EaseTravel = ui.CubicBezier(0.45, 0, 0.1, 1)
	EaseArrive = ui.CubicBezier(0.25, 0.8, 0.25, 1)
	EaseStd    = ui.CubicBezier(0.25, 0.1, 0.25, 1)
)

// AppIconRadius returns the continuous corner for an icon body size.
func AppIconRadius(size float32) float32 { return size * 0.2237 }
