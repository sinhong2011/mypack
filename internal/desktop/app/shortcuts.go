package app

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
)

// Keyboard shortcuts: the keys MyPack's menus and quick launcher answer.
// Each has a default the user can change, or clear, in Settings ›
// Shortcuts; Help › Keyboard Shortcuts lists them all. They are kept as
// accelerators ("CmdOrCtrl+Shift+N"), which the menus take as they are
// and kit.HotkeyText shows the platform's way.

// Shortcut IDs, which ui-state.json keeps the user's choices under.
const (
	scLauncher  = "launcher"
	scPalette   = "palette"
	scSettings  = "settings"
	scShortcuts = "shortcuts"
	scNewApp    = "newApp"
	scNewGroup  = "newGroup"
	scBuild     = "build"
	scOpen      = "open"
	scLibrary   = "library"
	scDiscover  = "discover"
	scBuilds    = "builds"
	scRuntimes  = "runtimes"
	scFind      = "find"
)

type shortcutDef struct {
	id    string
	label string // message ID
	group string // message ID of its group
	def   string // the default accelerator
	// global shortcuts work while another app is in front.
	global bool
}

// shortcutDefs lists the shortcuts the user can change, in the order
// Settings and the reference show them.
func shortcutDefs() []shortcutDef {
	const gen, file, view = "shortcuts.group.general", "menu.file", "menu.view"
	return []shortcutDef{
		{scLauncher, "launcher.setting", "shortcuts.group.global", launcherDefault(), true},
		{scPalette, "shortcuts.palette", gen, "CmdOrCtrl+K", false},
		{scSettings, "nav.settings", gen, "CmdOrCtrl+,", false},
		{scShortcuts, "menu.shortcuts", gen, shortcutsDefault(), false},
		{scNewApp, "menu.newApp", file, "CmdOrCtrl+N", false},
		{scNewGroup, "shortcuts.newGroup", file, "CmdOrCtrl+Shift+N", false},
		{scBuild, "menu.build", file, "CmdOrCtrl+B", false},
		{scOpen, "shortcuts.open", file, "CmdOrCtrl+O", false},
		{scLibrary, "nav.library", view, "CmdOrCtrl+1", false},
		{scDiscover, "nav.discover", view, "CmdOrCtrl+2", false},
		{scBuilds, "nav.builds", view, "CmdOrCtrl+3", false},
		{scRuntimes, "nav.runtimes", view, "CmdOrCtrl+4", false},
		{scFind, "menu.find", view, "CmdOrCtrl+F", false},
	}
}

// launcherDefault is the quick launcher's default shortcut: Option-Shift-
// Space on macOS, where Option-Space is the shortcut of many launchers
// and chat apps; Alt-Space opens the window menu on Windows, and
// Alt-Shift switches its keyboard layout, so Ctrl+Alt+Space elsewhere.
func launcherDefault() string {
	if runtime.GOOS == "darwin" {
		return "Alt+Shift+Space"
	}
	return "Ctrl+Alt+Space"
}

// shortcutsDefault opens the shortcut reference: ⌃⇧/ on macOS, as in
// Triwarden (⌘? is the system's Help search), Ctrl+Shift+/ elsewhere.
func shortcutsDefault() string {
	if runtime.GOOS == "darwin" {
		return "Ctrl+Shift+/"
	}
	return "CmdOrCtrl+Shift+/"
}

func shortcutByID(id string) (shortcutDef, bool) {
	for _, d := range shortcutDefs() {
		if d.id == id {
			return d, true
		}
	}
	return shortcutDef{}, false
}

// accel is shortcut id's accelerator: the user's choice, else its
// default; "" when the user cleared it.
func (a *App) accel(id string) string {
	if v, ok := a.ui.Shortcuts[id]; ok {
		return v
	}
	d, _ := shortcutByID(id)
	return d.def
}

// menuAccel is id's accelerator for a menu item: none while a shortcut
// is being recorded, so that the keys reach the recorder.
func (a *App) menuAccel(id string) string {
	if a.set.recording != "" {
		return ""
	}
	return a.accel(id)
}

// hotkey is id's shortcut as the UI shows it, "" for none.
func (a *App) hotkey(id string) string { return kit.HotkeyText(a.accel(id)) }

func (a *App) shortcutCustomized(id string) bool {
	d, _ := shortcutByID(id)
	v, ok := a.ui.Shortcuts[id]
	return ok && normAccel(v, isMac()) != normAccel(d.def, isMac())
}

// setShortcut keeps acc ("" for none) as id's shortcut and puts it in
// force.
func (a *App) setShortcut(id, acc string) {
	d, _ := shortcutByID(id)
	if normAccel(acc, isMac()) == normAccel(d.def, isMac()) {
		delete(a.ui.Shortcuts, id)
	} else {
		if a.ui.Shortcuts == nil {
			a.ui.Shortcuts = map[string]string{}
		}
		a.ui.Shortcuts[id] = acc
	}
	a.ui.save()
	a.applyShortcuts()
}

func (a *App) resetShortcuts() {
	a.ui.Shortcuts = nil
	a.ui.save()
	a.applyShortcuts()
}

// applyShortcuts puts the shortcuts in force: the menus' and the
// launcher's.
func (a *App) applyShortcuts() {
	a.installMenu()
	a.syncLauncherShortcut()
	if a.win != nil {
		a.win.Invalidate()
	}
	a.redrawSettings()
}

func isMac() bool { return runtime.GOOS == "darwin" }

// normAccel is an accelerator in one spelling, for comparing two: its
// modifiers in a fixed order, the platform's command key as CmdOrCtrl.
func normAccel(s string, mac bool) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "+")
	if strings.HasSuffix(s, "++") {
		parts = append(parts[:len(parts)-2], "+")
	}
	var cmd, ctrl, alt, shift, super bool
	key := ""
	for i, p := range parts {
		lp := strings.ToLower(p)
		if i == len(parts)-1 {
			key = strings.ToUpper(lp)
			break
		}
		switch lp {
		case "cmdorctrl", "commandorcontrol", "mod":
			cmd = true
		case "cmd", "command", "super", "meta", "win":
			if mac {
				cmd = true
			} else {
				super = true
			}
		case "ctrl", "control":
			if mac {
				ctrl = true
			} else {
				cmd = true
			}
		case "alt", "option", "opt":
			alt = true
		case "shift":
			shift = true
		}
	}
	var out []string
	for _, m := range []struct {
		on bool
		s  string
	}{{cmd, "CmdOrCtrl"}, {ctrl, "Ctrl"}, {super, "Super"}, {alt, "Alt"}, {shift, "Shift"}} {
		if m.on {
			out = append(out, m.s)
		}
	}
	return strings.Join(append(out, key), "+")
}

// reservedAccels are the system's and the Edit menu's shortcuts, which no
// shortcut of MyPack's takes.
var reservedAccels = []string{
	"CmdOrCtrl+Q", "CmdOrCtrl+W", "CmdOrCtrl+H", "CmdOrCtrl+M", "CmdOrCtrl+Alt+H",
	"CmdOrCtrl+C", "CmdOrCtrl+V", "CmdOrCtrl+X", "CmdOrCtrl+A", "CmdOrCtrl+Z", "CmdOrCtrl+Shift+Z",
	"CmdOrCtrl+Space", "CmdOrCtrl+Tab",
}

// accelKeyNames names the keys a shortcut may end with.
var accelKeyNames = func() map[ui.Key]string {
	m := map[ui.Key]string{
		ui.KeySpace: "Space", ui.KeyEnter: "Enter", ui.KeyTab: "Tab", ui.KeyBackspace: "Backspace", ui.KeyDelete: "Delete",
		ui.KeyHome: "Home", ui.KeyEnd: "End", ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown",
		ui.KeyUp: "Up", ui.KeyDown: "Down", ui.KeyLeft: "Left", ui.KeyRight: "Right",
		ui.KeyMinus: "-", ui.KeyEqual: "=", ui.KeyComma: ",", ui.KeyPeriod: ".", ui.KeySlash: "/",
		ui.KeySemicolon: ";", ui.KeyQuote: "'", ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]",
		ui.KeyBackslash: "\\", ui.KeyBackquote: "`",
	}
	letters := []ui.Key{ui.KeyA, ui.KeyB, ui.KeyC, ui.KeyD, ui.KeyE, ui.KeyF, ui.KeyG, ui.KeyH, ui.KeyI, ui.KeyJ, ui.KeyK, ui.KeyL, ui.KeyM,
		ui.KeyN, ui.KeyO, ui.KeyP, ui.KeyQ, ui.KeyR, ui.KeyS, ui.KeyT, ui.KeyU, ui.KeyV, ui.KeyW, ui.KeyX, ui.KeyY, ui.KeyZ}
	for i, k := range letters {
		m[k] = string(rune('A' + i))
	}
	for i, k := range []ui.Key{ui.Key0, ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9} {
		m[k] = string(rune('0' + i))
	}
	for i, k := range []ui.Key{ui.KeyF1, ui.KeyF2, ui.KeyF3, ui.KeyF4, ui.KeyF5, ui.KeyF6, ui.KeyF7, ui.KeyF8, ui.KeyF9, ui.KeyF10, ui.KeyF11, ui.KeyF12} {
		m[k] = "F" + string(rune('1'+i))
		if i >= 9 {
			m[k] = "F1" + string(rune('0'+i-9))
		}
	}
	return m
}()

// accelFromKey is the accelerator of a key pressed with mods: "" and
// false for a key no shortcut takes; "" and true for a modifier on its own,
// or a key that needs a modifier pressed alone (only function keys go
// without one).
func accelFromKey(k ui.Key, mods ui.Modifiers, mac bool) (string, bool) {
	name, ok := accelKeyNames[k]
	if !ok {
		return "", k == ui.KeyUnknown
	}
	cmdMod := ui.Ctrl
	if mac {
		cmdMod = ui.Super
	}
	var parts []string
	if mods&cmdMod != 0 {
		parts = append(parts, "CmdOrCtrl")
	}
	if mac && mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if !mac && mods&ui.Super != 0 {
		parts = append(parts, "Super")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if len(parts) == 0 && !(len(name) > 1 && name[0] == 'F' && name[1] >= '0' && name[1] <= '9') {
		return "", true // a key alone, or with Shift alone, is typing
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	return strings.Join(append(parts, name), "+"), true
}

// shortcutClash names what else answers acc: another shortcut's label,
// or the system's message for a reserved one; "" for none.
func (a *App) shortcutClash(id, acc string) string {
	mac := isMac()
	n := normAccel(acc, mac)
	for _, r := range reservedAccels {
		if normAccel(r, mac) == n {
			return T("shortcuts.reserved")
		}
	}
	for _, d := range shortcutDefs() {
		if d.id != id && normAccel(a.accel(d.id), mac) == n {
			return T("shortcuts.taken", "name", T(d.label))
		}
	}
	return ""
}

// ---- recording ---------------------------------------------------------------

// startRecording takes the next keys pressed as id's shortcut. The menus
// and the launcher let go of theirs meanwhile, so that pressing one of
// them records it rather than runs it.
func (a *App) startRecording(id string) {
	s := &a.set
	s.recording, s.recFocused, s.recErr, s.recErrID = id, false, "", id
	a.applyShortcuts()
}

func (a *App) stopRecording() {
	if a.set.recording == "" {
		return
	}
	a.set.recording, a.set.recFocused, a.set.recErr = "", false, ""
	a.applyShortcuts()
}

// recordKey takes one key pressed while id records; it reports whether
// it took it.
func (a *App) recordKey(id string, ev ui.InputEvent) bool {
	if a.set.recording != id || ev.Kind != ui.InputKeyDown {
		return false
	}
	if ev.Mods == 0 {
		switch ev.Key {
		case ui.KeyEscape:
			a.stopRecording()
			return true
		case ui.KeyBackspace, ui.KeyDelete:
			a.set.recErr = ""
			a.setShortcutStop(id, "")
			return true
		case ui.KeyTab:
			a.stopRecording()
			return false // the focus moves on
		}
	}
	k := ev.Key
	if k == ui.KeyUnknown {
		k = ev.PhysicalKey
	}
	acc, ok := accelFromKey(k, ev.Mods, isMac())
	switch {
	case acc == "" && ok && k == ui.KeyUnknown:
		return true // a modifier on its own: wait for the key
	case acc == "":
		a.set.recErr = T("shortcuts.needsModifier")
		if !isMac() {
			a.set.recErr = T("shortcuts.needsModifier.other")
		}
		return true
	}
	if clash := a.shortcutClash(id, acc); clash != "" {
		a.set.recErr = clash
		return true
	}
	a.set.recErr = ""
	a.setShortcutStop(id, acc)
	return true
}

func (a *App) setShortcutStop(id, acc string) {
	a.set.recording, a.set.recFocused = "", false
	a.setShortcut(id, acc)
}

// keycaps draws a hotkey's keys one keycap each, as Settings › Shortcuts
// and the reference show them.
func keycaps(c *ui.Context, acc string) {
	p := design.Of(c)
	ui.Row(c).Gap(3).AlignItems(ui.Center).Shrink(0).AccessHidden(true).Children(func() {
		for i, k := range kit.HotkeyParts(acc) {
			ui.Text(c.Key(i), k).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).TextColor(p.Text).
				MinWidth(20).Height(20).PaddingX(5).Center().TextAlign(ui.Center).Radius(5).Background(p.Fill).Border(0.5, p.Border).SingleLine()
		}
	})
}

// shortcutRecorder is a shortcut's field, as System Settings' are: its
// keys as keycaps, or a dashed "Record Shortcut" when it has none. A click
// records new keys (Esc gives up); ✕, on hover, turns it off. Why keys
// were refused shows under it.
func (a *App) shortcutRecorder(c *ui.Context, id string) {
	p := design.Of(c)
	s := &a.set
	rec := s.recording == id
	acc := a.accel(id)
	d, _ := shortcutByID(id)
	ui.Column(c.Key("rec-" + id)).Gap(4).AlignItems(ui.End).Shrink(0).Children(func() {
		b := ui.ButtonBase(c.Key("field")).Label(T(d.label)).Description(kit.HotkeyText(acc)).Row().Width(156).Height(30).
			PaddingX(8).Gap(4).Center().Radius(design.RadiusControl).Cursor(ui.CursorPointer).FocusRing(false).
			Transition(design.Transition(c))
		b.HandleInput(func(ev ui.InputEvent) bool { return a.recordKey(id, ev) })
		hov := b.Hovered()
		switch {
		case rec:
			b.Border(1.5, design.Fade(p.Text, 0.45)).Background(p.Fill)
			if b.Focused() {
				s.recFocused = true
			} else if s.recFocused {
				a.stopRecording() // the focus went elsewhere: give up
			} else {
				b.Focus()
			}
		case s.recErr != "" && s.recErrID == id:
			b.Border(1, p.Warning)
		case acc == "":
			b.Border(1, p.BorderStrong).BorderStyle(ui.BorderDashed)
		default:
			b.Border(1, p.Border).Background(p.FieldRest)
		}
		b.Children(func() {
			switch {
			case rec:
				ui.Text(c, T("shortcuts.recording")).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine()
			case acc == "":
				ui.Text(c, T("shortcuts.recordNew")).FontSize(design.FontSmall).TextColor(p.TextTertiary).SingleLine()
			default:
				keycaps(c, acc)
			}
		})
		if b.Clicked() && !rec {
			a.startRecording(id)
		}
		// ✕ turns it off: over the field's right end, while hovered.
		if acc != "" && !rec && (hov || s.recHover == id) {
			ui.Box(c).Absolute().Top(0).Right(0).Height(30).Width(26).Center().Children(func() {
				x := ui.ButtonBase(c.Key("off")).Label(T("shortcuts.turnOff")).Tooltip(T("shortcuts.turnOff")).Size(18, 18).Center().
					Radius(9).Cursor(ui.CursorPointer).FocusRing(false)
				s.recHover = ""
				if x.Hovered() {
					s.recHover = id
				}
				x.Children(func() { kit.Glyph(c, icons.Close, 11, p.TextTertiary) })
				if x.Clicked() {
					s.recErr = ""
					a.setShortcut(id, "")
				}
			})
		}
		if s.recErr != "" && s.recErrID == id {
			ui.Text(c, s.recErr).FontSize(design.FontCaption).TextColor(p.Warning).MaxWidth(220).TextAlign(ui.End)
		}
	})
}

// settingsShortcuts is Settings › Shortcuts: the keys that work anywhere
// (the quick launcher's), then those of MyPack's menus.
func (a *App) settingsShortcuts(c *ui.Context) {
	s := &a.set
	row := func(d shortcutDef) {
		label := T(d.label)
		if d.id == scLauncher {
			label = T("shortcuts.launcherKeys") // under the launcher's own row
		}
		kit.SettingRow(c.Key("sc-"+d.id), label, "", func() {
			if a.shortcutCustomized(d.id) && s.recording != d.id {
				if kit.IconButton(c.Key("reset"), icons.Undo, T("shortcuts.reset", "keys", kit.HotkeyText(d.def))).Size(24, 24).Clicked() {
					s.recErr = ""
					a.setShortcut(d.id, d.def)
				}
			}
			a.shortcutRecorder(c, d.id)
		})
	}
	kit.Group(c, T("shortcuts.group.global"), func() {
		on := a.launcherEnabled()
		desc := T("shortcuts.launcher.desc")
		if on && a.launcher.registered == "" && a.accel(scLauncher) != "" && s.recording != scLauncher {
			desc = T("launcher.setting.taken", "keys", a.hotkey(scLauncher))
		}
		kit.SettingIconRow(c.Key("launcher"), icons.Search, ui.Color{}, T("launcher.setting"), desc, func() {
			old := on
			kit.Switch(c.Key("sw-launcher"), &on, T("launcher.setting"))
			if on != old {
				a.ui.LauncherOff = !on
				a.ui.save()
				a.syncLauncherShortcut()
			}
		})
		if on {
			d, _ := shortcutByID(scLauncher)
			row(d)
		}
	})
	group := ""
	var defs []shortcutDef
	flush := func() {
		if len(defs) == 0 {
			return
		}
		list := defs
		kit.Group(c.Key("g-"+group), T(group), func() {
			for _, d := range list {
				row(d)
			}
		})
		defs = nil
	}
	for _, d := range shortcutDefs() {
		if d.global {
			continue
		}
		if d.group != group {
			flush()
			group = d.group
		}
		defs = append(defs, d)
	}
	flush()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		kit.Caption(c, T("shortcuts.hint")).Grow(1).MinWidth(0)
		if kit.Button(c.Key("reset-all"), T("shortcuts.resetAll"), kit.ButtonOpts{Size: design.ControlSM, Disabled: len(a.ui.Shortcuts) == 0}).Clicked() {
			s.recErr = ""
			a.resetShortcuts()
		}
	})
}

// ---- Help › Keyboard Shortcuts -------------------------------------------------

// showShortcuts shows the keyboard shortcut reference in the main window.
func (a *App) showShortcuts() {
	a.showWindow()
	a.palette.open = false
	a.dlg.shortcuts, a.dlg.shortcutsQuery = true, ""
}

type scLine struct{ label, keys string } // keys: an accelerator

type scSection struct {
	title string
	lines []scLine
}

// shortcutSections is every key MyPack answers to, by where it works:
// anywhere (the launcher), its menus' (as the user set them), then the
// keys of the launcher and the palette, and of its windows.
func (a *App) shortcutSections() []scSection {
	var secs []scSection
	for _, d := range shortcutDefs() {
		if d.global && !a.launcherEnabled() {
			continue
		}
		if len(secs) == 0 || secs[len(secs)-1].title != T(d.group) {
			secs = append(secs, scSection{title: T(d.group)})
		}
		s := &secs[len(secs)-1]
		s.lines = append(s.lines, scLine{T(d.label), a.accel(d.id)})
	}
	return append(secs, scSection{T("shortcuts.group.lists"), []scLine{
		{T("shortcuts.move"), "Up  Down"},
		{T("shortcuts.choose"), "Enter"},
		{T("shortcuts.second"), "CmdOrCtrl+Enter"},
		{T("launcher.close"), "Esc"},
	}}, scSection{T("shortcuts.group.window"), []scLine{
		{T("shortcuts.closeWindow"), "CmdOrCtrl+W"},
		{T("menu.quit"), "CmdOrCtrl+Q"},
	}})
}

// filterShortcuts keeps the lines matching q; a section's own title
// matching keeps all of it.
func filterShortcuts(secs []scSection, q string) []scSection {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return secs
	}
	var out []scSection
	for _, s := range secs {
		if strings.Contains(strings.ToLower(s.title), q) {
			out = append(out, s)
			continue
		}
		var keep []scLine
		for _, l := range s.lines {
			if strings.Contains(strings.ToLower(l.label), q) || strings.Contains(strings.ToLower(kit.HotkeyText(l.keys)), q) {
				keep = append(keep, l)
			}
		}
		if len(keep) > 0 {
			out = append(out, scSection{s.title, keep})
		}
	}
	return out
}

// splitShortcuts is where the reference's second column starts: the
// section that leaves the two columns closest in height (a section's
// heading counts as two lines).
func splitShortcuts(secs []scSection) int {
	h := func(s []scSection) (n int) {
		for _, x := range s {
			n += len(x.lines) + 2
		}
		return n
	}
	best, bestH := len(secs), h(secs)
	for i := 1; i < len(secs); i++ {
		if m := max(h(secs[:i]), h(secs[i:])); m < bestH {
			best, bestH = i, m
		}
	}
	return best
}

// shortcutsSheet is Help › Keyboard Shortcuts: a filterable two-column
// list of every key, drawn as keycaps, with what the modifier symbols
// mean.
func (a *App) shortcutsSheet(c *ui.Context) {
	if !a.dlg.shortcuts {
		return
	}
	p := design.Of(c)
	kit.Sheet(c.Key("dlg-shortcuts"), &a.dlg.shortcuts, 680, func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Size(32, 32).Center().Radius(9).Background(p.Fill).Shrink(0).Children(func() {
				kit.Glyph(c, icons.Keyboard, 17, p.TextSecondary)
			})
			ui.Text(c, T("menu.shortcuts")).FontSize(design.FontTitle3).FontWeight(design.WeightBold).TextColor(p.Text).Role(ui.RoleHeading).
				LetterSpacing(design.TrackTitle).Grow(1).MinWidth(0)
			kit.SearchField(c.Key("filter"), &a.dlg.shortcutsQuery, T("shortcuts.filter"), nil, 200)
		})
		secs := filterShortcuts(a.shortcutSections(), a.dlg.shortcutsQuery)
		ui.Scroll(c).MaxHeight(440).Children(func() {
			if len(secs) == 0 {
				kit.Secondary(c, T("palette.empty")).Padding(24, 0)
				return
			}
			split := splitShortcuts(secs)
			ui.Row(c).Gap(28).AlignItems(ui.Start).Children(func() {
				for col, part := range [][]scSection{secs[:split], secs[split:]} {
					ui.Column(c.Key(col)).Grow(1).Basis(0).MinWidth(0).Gap(16).Children(func() {
						for _, s := range part {
							ui.Column(c.Key(s.title)).Gap(1).Children(func() {
								kit.Eyebrow(c, s.title).Padding(0, 0, 4, 0)
								for _, l := range s.lines {
									ui.Row(c.Key(l.label)).Height(28).Gap(10).AlignItems(ui.Center).Children(func() {
										ui.Text(c, l.label).FontSize(design.FontBody).TextColor(p.Text).SingleLine().Grow(1).MinWidth(0)
										switch {
										case l.keys == "":
											ui.Text(c, T("shortcuts.none")).FontSize(design.FontSmall).TextColor(p.TextTertiary)
										case strings.Contains(l.keys, "  "): // alternatives
											ui.Row(c).Gap(6).Children(func() {
												for _, k := range strings.Split(l.keys, "  ") {
													keycaps(c.Key(k), k)
												}
											})
										default:
											keycaps(c, l.keys)
										}
									})
								}
							})
						}
					})
				}
			})
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
			// What the symbols mean, on the Mac's keyboards.
			if isMac() {
				ui.Row(c).Gap(12).Grow(1).MinWidth(0).Children(func() {
					for _, m := range [][2]string{{"⌘", "shortcuts.key.command"}, {"⌃", "shortcuts.key.control"}, {"⌥", "shortcuts.key.option"}, {"⇧", "shortcuts.key.shift"}} {
						ui.Row(c.Key(m[0])).Gap(4).AlignItems(ui.Center).Children(func() {
							ui.Text(c, m[0]).FontSize(design.FontSmall).FontWeight(design.WeightSemibold).TextColor(p.TextSecondary)
							kit.Caption(c, T(m[1]))
						})
					}
				})
			} else {
				ui.Spacer(c)
			}
			if kit.Button(c.Key("customize"), T("shortcuts.customize"), kit.ButtonOpts{Icon: icons.Sliders}).Clicked() {
				a.dlg.shortcuts = false
				a.openSettings(paneShortcuts)
			}
			if kit.Button(c.Key("done"), T("common.close"), kit.ButtonOpts{Variant: kit.Primary}).MinWidth(84).Clicked() {
				a.dlg.shortcuts = false
			}
		})
	})
}
