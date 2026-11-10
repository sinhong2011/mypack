// Package design owns mypack's brand tokens: semantic colors for light and
// dark, the 4 DIP spacing scale, control and row metrics, radii, type ramp
// and motion (spec §08, §08A.3, §9.7). Screens and components read colors
// from Of(c) and never hard-code hex values, so appearance stays coherent and
// third-party kits (Lobe MyGo) remain an implementation detail.
package design

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Palette is the semantic color set for one appearance.
type Palette struct {
	Dark bool

	// Surfaces, from back to front.
	Sidebar       ui.Color // sidebar fallback when vibrancy is unavailable
	Bg            ui.Color // main content background
	Surface       ui.Color // cards, panels
	SurfaceRaised ui.Color // popovers, menus, the hero field
	Well          ui.Color // inset areas: log viewer, code, inputs
	Hover         ui.Color // translucent hover fill
	Pressed       ui.Color // translucent pressed fill
	Selected      ui.Color // sidebar / row selection fill

	Border       ui.Color // hairlines
	BorderStrong ui.Color // control outlines
	Divider      ui.Color

	Text          ui.Color
	TextSecondary ui.Color
	TextTertiary  ui.Color
	TextDisabled  ui.Color
	TextOnAccent  ui.Color

	Accent        ui.Color
	AccentHover   ui.Color
	AccentPressed ui.Color
	AccentSoft    ui.Color // tinted backgrounds
	AccentText    ui.Color // accent-colored text on Bg

	Success     ui.Color
	SuccessSoft ui.Color
	Warning     ui.Color
	WarningSoft ui.Color
	Danger      ui.Color
	DangerSoft  ui.Color
	DangerHover ui.Color

	Shadow     ui.Color // ambient card shadow
	ShadowDeep ui.Color // floating surfaces
	Focus      ui.Color
	Scrim      ui.Color
}

// Light is the light appearance.
var Light = Palette{
	Dark:          false,
	Sidebar:       ui.RGB(242, 242, 244),
	Bg:            ui.RGB(255, 255, 255),
	Surface:       ui.RGB(255, 255, 255),
	SurfaceRaised: ui.RGB(255, 255, 255),
	Well:          ui.RGB(246, 246, 248),
	Hover:         ui.RGBA(0, 0, 0, 0.045),
	Pressed:       ui.RGBA(0, 0, 0, 0.08),
	Selected:      ui.RGBA(0, 0, 0, 0.075),

	Border:       ui.RGBA(0, 0, 0, 0.085),
	BorderStrong: ui.RGBA(0, 0, 0, 0.14),
	Divider:      ui.RGBA(0, 0, 0, 0.065),

	Text:          ui.RGB(28, 28, 30),
	TextSecondary: ui.RGB(99, 99, 105),
	TextTertiary:  ui.RGB(138, 138, 145),
	TextDisabled:  ui.RGB(180, 180, 186),
	TextOnAccent:  ui.RGB(255, 255, 255),

	Accent:        ui.RGB(31, 112, 240),
	AccentHover:   ui.RGB(26, 99, 222),
	AccentPressed: ui.RGB(22, 88, 202),
	AccentSoft:    ui.RGBA(31, 112, 240, 0.09),
	AccentText:    ui.RGB(24, 98, 216),

	Success:     ui.RGB(28, 140, 76),
	SuccessSoft: ui.RGBA(28, 140, 76, 0.10),
	Warning:     ui.RGB(178, 112, 8),
	WarningSoft: ui.RGBA(214, 140, 20, 0.12),
	Danger:      ui.RGB(206, 56, 48),
	DangerSoft:  ui.RGBA(206, 56, 48, 0.08),
	DangerHover: ui.RGB(186, 46, 40),

	Shadow:     ui.RGBA(0, 0, 0, 0.05),
	ShadowDeep: ui.RGBA(0, 0, 0, 0.14),
	Focus:      ui.RGBA(31, 112, 240, 0.55),
	Scrim:      ui.RGBA(0, 0, 0, 0.28),
}

// Dark is the restrained dark appearance: neutral charcoal, not pure black.
var Dark = Palette{
	Dark:          true,
	Sidebar:       ui.RGB(30, 30, 33),
	Bg:            ui.RGB(23, 23, 25),
	Surface:       ui.RGB(30, 30, 33),
	SurfaceRaised: ui.RGB(36, 36, 40),
	Well:          ui.RGB(18, 18, 20),
	Hover:         ui.RGBA(255, 255, 255, 0.055),
	Pressed:       ui.RGBA(255, 255, 255, 0.09),
	Selected:      ui.RGBA(255, 255, 255, 0.085),

	Border:       ui.RGBA(255, 255, 255, 0.08),
	BorderStrong: ui.RGBA(255, 255, 255, 0.14),
	Divider:      ui.RGBA(255, 255, 255, 0.06),

	Text:          ui.RGB(236, 236, 239),
	TextSecondary: ui.RGB(160, 160, 167),
	TextTertiary:  ui.RGB(116, 116, 124),
	TextDisabled:  ui.RGB(84, 84, 90),
	TextOnAccent:  ui.RGB(255, 255, 255),

	Accent:        ui.RGB(44, 124, 245),
	AccentHover:   ui.RGB(64, 138, 250),
	AccentPressed: ui.RGB(36, 108, 224),
	AccentSoft:    ui.RGBA(56, 136, 255, 0.15),
	AccentText:    ui.RGB(104, 162, 255),

	Success:     ui.RGB(70, 190, 120),
	SuccessSoft: ui.RGBA(70, 190, 120, 0.14),
	Warning:     ui.RGB(232, 172, 70),
	WarningSoft: ui.RGBA(232, 172, 70, 0.14),
	Danger:      ui.RGB(240, 98, 90),
	DangerSoft:  ui.RGBA(240, 98, 90, 0.13),
	DangerHover: ui.RGB(250, 118, 110),

	Shadow:     ui.RGBA(0, 0, 0, 0.28),
	ShadowDeep: ui.RGBA(0, 0, 0, 0.5),
	Focus:      ui.RGBA(96, 150, 255, 0.6),
	Scrim:      ui.RGBA(0, 0, 0, 0.5),
}

// Spacing: the 4 DIP scale.
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
	ControlSM float32 = 28
	Control   float32 = 32
	ControlLG float32 = 36
	ControlXL float32 = 38 // hero field / primary CTA

	RowCompact float32 = 44
	Row        float32 = 48
	RowTall    float32 = 56

	RadiusXS      float32 = 4
	RadiusSM      float32 = 6
	RadiusControl float32 = 7
	RadiusCard    float32 = 12
	RadiusPanel   float32 = 14

	SidebarWidth  float32 = 232
	TitleBarH     float32 = 52
	ContentMaxW   float32 = 920
	ContentNarrow float32 = 640
)

// Type ramp (DIPs). macOS body text is 13.
const (
	FontCaption float32 = 11
	FontSmall   float32 = 12
	FontBody    float32 = 13
	FontBodyLG  float32 = 14
	FontTitle3  float32 = 15
	FontTitle2  float32 = 17
	FontTitle1  float32 = 22
	FontDisplay float32 = 26

	WeightRegular  = 400
	WeightMedium   = 500
	WeightSemibold = 600
	WeightBold     = 700
)

// Motion durations (120–220 ms; zero under Reduce Motion, see Dur).
const (
	MotionFast = 120 * time.Millisecond
	MotionBase = 170 * time.Millisecond
	MotionSlow = 220 * time.Millisecond
)

// AppIconRadius returns the macOS-like continuous corner for an icon size.
func AppIconRadius(size float32) float32 { return size * 0.225 }
