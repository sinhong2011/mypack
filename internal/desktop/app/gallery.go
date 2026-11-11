package app

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
)

// The kit gallery is a hidden review page (MYPACK_KIT_GALLERY=1): every
// kit component in its states, a motion-speed slider, and the build scene
// at each stage. Developer-only: its words are not translated.

func galleryEnabled() bool { return os.Getenv("MYPACK_KIT_GALLERY") == "1" }

// init slows every spring and tween by MYPACK_MOTION_SPEED (0.1–1), for
// reviewing motion frame by frame.
func init() {
	if v, err := strconv.ParseFloat(os.Getenv("MYPACK_MOTION_SPEED"), 32); err == nil && v > 0 {
		design.SetSpeed(float32(v))
	}
}

type galleryState struct {
	speed    float64
	on, chk  bool
	seg      int
	text     string
	invalid  bool
	query    string
	open     bool
	sheet    bool
	stage    int
	stageAt  time.Time
	loading  bool
	progress float64
}

func (a *App) galleryView(c *ui.Context) {
	g := &a.gallery
	if g.speed == 0 {
		g.speed, g.progress = 1, 0.42
	}
	p := design.Of(c)
	if beat := os.Getenv("MYPACK_SCENE_BEAT"); beat != "" {
		a.sceneBeat(c, beat)
		return
	}
	page(c, 820, func() {
		kit.LargeTitle(c, "Kit")
		kit.Group(c, "Motion", func() {
			kit.SettingRow(c, "Speed", "Slows every spring and tween", func() {
				ui.Slider(c.Key("speed"), &g.speed, 0.1, 1).Width(220).Label("Motion speed")
				design.SetSpeed(float32(g.speed))
			})
		})
		kit.Group(c, "Buttons", func() {
			// Every variant at every size: all pills, icon-only ones circles.
			ui.Column(c).Padding(14).Gap(10).Children(func() {
				for _, h := range []float32{design.ControlSM, design.Control, design.ControlLG, design.ControlXL} {
					h := h
					ui.Row(c.Key(h)).Gap(10).AlignItems(ui.Center).Children(func() {
						kit.Button(c.Key("p"), "Primary", kit.ButtonOpts{Variant: kit.Primary, Size: h, Icon: icons.Build})
						kit.Button(c.Key("s"), "Secondary", kit.ButtonOpts{Size: h})
						kit.Button(c.Key("g"), "Ghost", kit.ButtonOpts{Variant: kit.Ghost, Size: h})
						kit.Button(c.Key("d"), "Danger", kit.ButtonOpts{Variant: kit.Danger, Size: h})
						kit.Button(c.Key("l"), "Link", kit.ButtonOpts{Variant: kit.Plain, Size: h})
						kit.Button(c.Key("i"), "", kit.ButtonOpts{Variant: kit.Primary, Size: h, Icon: icons.Plus, Tooltip: "Add"})
						kit.Button(c.Key("j"), "", kit.ButtonOpts{Size: h, Icon: icons.More, Tooltip: "More"})
						kit.Button(c.Key("x"), "Disabled", kit.ButtonOpts{Size: h, Disabled: true})
					})
				}
				ui.Row(c.Key("tb")).Gap(8).AlignItems(ui.Center).Children(func() {
					kit.NewPill(c.Key("np"), "New App", "mod+n", false)
					kit.NewPill(c.Key("npc"), "New App", "mod+n", true)
					kit.ToolbarIcon(c.Key("ti"), icons.More, "More")
					kit.IconButton(c.Key("ib"), icons.Settings, "Settings")
					kit.GlassIcon(c.Key("gi"), icons.Plus, "Add", "", true)
					sel := 0
					kit.PillSegmented(c.Key("seg"), &sel, []kit.PillSegment{{Icon: icons.Grid, Label: "Grid"}, {Icon: icons.List, Label: "List"}}, "View", struct{}{}, false)
					if kit.Button(c.Key("b6"), "Save", kit.ButtonOpts{Variant: kit.Primary, Loading: g.loading}).Clicked() {
						g.loading = true
						go func() { time.Sleep(1500 * time.Millisecond); a.win.Update(func() { g.loading = false }) }()
					}
				})
			})
		})
		kit.Group(c, "Controls", func() {
			kit.SettingRow(c, "Switch", "Elastic thumb", func() { kit.Switch(c.Key("sw"), &g.on, "Switch") })
			kit.SettingRow(c, "Checkbox", "Draw-on check", func() { kit.Checkbox(c.Key("ck"), &g.chk, "Checkbox") })
			kit.SettingRow(c, "Segmented", "Gliding indicator", func() {
				kit.Segmented(c.Key("seg"), &g.seg, []string{"One", "Two", "Three"}, false).Label("Segmented")
			})
			kit.SettingRow(c, "Text field", "Focus glow, shake on invalid", func() {
				kit.TextField(c.Key("tf"), &g.text, "Text", kit.FieldOpts{Placeholder: "Type", Invalid: g.invalid}).Width(200)
				kit.Checkbox(c.Key("inv"), &g.invalid, "Invalid")
			})
			kit.SettingRow(c, "Search", "", func() { kit.SearchField(c.Key("sf"), &g.query, "Search", nil, 220) })
		})
		kit.Group(c, "Feedback", func() {
			ui.Row(c).Padding(14).Gap(14).Wrap().AlignItems(ui.Center).Children(func() {
				kit.StatusPill(c, "Installed", kit.ToneSuccess)
				kit.StatusPill(c, "Update", kit.ToneAccent)
				kit.StatusDot(c, "Verified", kit.ToneSuccess)
				kit.StatusDot(c, "Missing", kit.ToneWarning)
				kit.Spinner(c, 16, p.Accent)
				kit.ProgressRing(c, 28, 3, g.progress, p.Accent, p.Fill)
				ui.Box(c).Width(160).Children(func() { kit.ProgressBar(c, g.progress) })
				kit.Shimmer(c, 80, 14, 7)
				if kit.Button(c.Key("toast"), "Toast", kit.ButtonOpts{}).Clicked() {
					c.ToastAction("Moved to Trash", "Undo", func() {})
				}
				if kit.Button(c.Key("sheet"), "Sheet", kit.ButtonOpts{}).Clicked() {
					g.sheet = true
				}
			})
			kit.Disclosure(c.Key("disc"), &g.open, "Disclosure", "opens its height").Margin(0, 6)
			if g.open {
				kit.Reveal(ui.Column(c.Key("rev")).Padding(0, 14, 14, 14)).Children(func() {
					kit.Note(c, icons.Info, "Revealed content grows its height on a spring.", kit.ToneAccent)
				})
			}
		})
		kit.Group(c, "App icons", func() {
			ui.Row(c).Padding(18).Gap(22).Children(func() {
				for i, b := range []kit.BadgeKind{kit.BadgeNone, kit.BadgeUpdate, kit.BadgeWarning, kit.BadgeFailed, kit.BadgeLocked, kit.BadgeDone} {
					ui.Box(c.Key(i)).Size(56, 56).Children(func() {
						name := templates[i].name
						kit.AppIconOpts(c, templateIcon(name), name, 56, kit.IconOpts{Shadow: true})
						kit.IconBadge(c, b, 56)
					})
				}
			})
		})
		kit.Group(c, "Build scene", func() {
			ui.Column(c).Padding(14).Gap(10).Children(func() {
				old := g.stage
				kit.Segmented(c.Key("stage"), &g.stage, []string{"Validate", "Download", "Assemble", "Sign", "Verify", "Done", "Failed"}, true).Label("Stage")
				if old != g.stage || g.stageAt.IsZero() {
					g.stageAt = time.Now()
				}
				st := min(g.stage, kit.SceneDone)
				kit.BuildScene(c, kit.SceneOpts{Stage: st, StageAt: g.stageAt, Progress: g.progress, Failed: g.stage == 6,
					Icon: templateIcon("Linear"), Name: "Linear", Host: "linear.app", Tint: kit.IconTint(templateIcon("Linear"), "Linear"), Height: 300})
			})
		})
	})
	kit.Sheet(c.Key("gsheet"), &g.sheet, 420, func() {
		kit.SheetHeader(c, "A sheet", nil)
		kit.SheetBody(c, "It drops in from under the toolbar on a soft spring, and leaves the same way.")
		if ok, cancel := kit.SheetFooter(c, "Cancel", "Done", kit.Primary, false, false); ok || cancel {
			g.sheet = false
		}
	})
}

// sceneBeats are the build scene's beats held still, for review
// (MYPACK_SCENE_BEAT=1…8): stage, time, progress.
var sceneBeats = []struct {
	name  string
	stage int
	t     float64
	prog  float64
}{
	{"Checking the website", kit.SceneValidate, 1.0, -1},
	{"Getting the app engine", kit.SceneDownload, 1.2, 0.6},
	{"Making it an app", kit.SceneAssemble, 0.55, -1},
	{"Signing", kit.SceneSign, 0.7, -1},
	{"Checking everything", kit.SceneVerify, 0.35, -1},
	{"In the Dock", kit.SceneDone, 2.6, -1},
}

func (a *App) sceneBeat(c *ui.Context, beat string) {
	p := design.Of(c)
	i, _ := strconv.Atoi(beat)
	i = min(max(i, 1), len(sceneBeats)) - 1
	b := sceneBeats[i]
	icon := templateIcon("Linear")
	opts := kit.SceneOpts{Stage: b.stage, StageAt: time.Now(), At: b.t, Progress: b.prog, Icon: icon, Name: "Linear", Host: "linear.app",
		Tint: kit.IconTint(icon, "Linear")}
	ui.Column(c).Absolute().Fill().Padding(design.TitleBarH+8, 24, 24, 24).Gap(16).Children(func() {
		ui.Text(c, fmt.Sprintf("%d · %s", i+1, b.name)).FontSize(design.FontTitle2).FontWeight(design.WeightBold).TextColor(p.Text)
		ui.Row(c).Gap(24).AlignItems(ui.End).Children(func() {
			big := opts
			big.Height = 300
			ui.Box(c).Width(400).Children(func() { kit.BuildScene(c, big) })
			small := opts
			small.Height = 132
			ui.Box(c).Width(176).Children(func() { kit.BuildScene(c, small) })
		})
	})
}
