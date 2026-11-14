package app

import "runtime"

// runtimeShortcut is one menu key this app's Settings can remap.
type runtimeShortcut struct {
	ID, Label, Group, Def string
}

// runtimeShortcutDefs is the shortcuts a built app owns, in Settings order.
func runtimeShortcutDefs() []runtimeShortcut {
	dev := "Ctrl+Shift+I"
	if runtime.GOOS == "darwin" {
		dev = "Alt+Cmd+I"
	}
	const gen, file, edit, view = "General", "File", "Edit", "View"
	return []runtimeShortcut{
		{ID: "settings", Label: "Settings…", Group: gen, Def: "CmdOrCtrl+,"},
		{ID: "global", Label: "Bring to front", Group: gen, Def: "CmdOrCtrl+Shift+Space"},
		{ID: "newWindow", Label: "New Window", Group: file, Def: "CmdOrCtrl+N"},
		{ID: "downloads", Label: "Show Downloads", Group: file, Def: "CmdOrCtrl+Shift+J"},
		{ID: "newTab", Label: "New Tab", Group: file, Def: "CmdOrCtrl+T"},
		{ID: "closeTab", Label: "Close Tab", Group: file, Def: "CmdOrCtrl+W"},
		{ID: "find", Label: "Find…", Group: edit, Def: "CmdOrCtrl+F"},
		{ID: "reload", Label: "Reload", Group: view, Def: "CmdOrCtrl+R"},
		{ID: "hardReload", Label: "Hard Reload", Group: view, Def: "CmdOrCtrl+Shift+R"},
		{ID: "back", Label: "Back", Group: view, Def: "CmdOrCtrl+["},
		{ID: "forward", Label: "Forward", Group: view, Def: "CmdOrCtrl+]"},
		{ID: "home", Label: "Home", Group: view, Def: "CmdOrCtrl+Shift+H"},
		{ID: "zoomIn", Label: "Zoom In", Group: view, Def: "CmdOrCtrl+="},
		{ID: "zoomOut", Label: "Zoom Out", Group: view, Def: "CmdOrCtrl+-"},
		{ID: "zoomReset", Label: "Actual Size", Group: view, Def: "CmdOrCtrl+0"},
		{ID: "devtools", Label: "Developer Tools", Group: view, Def: dev},
		{ID: "layoutWindow", Label: "Window", Group: view, Def: "CmdOrCtrl+Alt+W"},
		{ID: "layoutCompact", Label: "Compact Window", Group: view, Def: "CmdOrCtrl+Alt+P"},
		{ID: "layoutSidebar", Label: "Sidebar", Group: view, Def: "CmdOrCtrl+Alt+S"},
	}
}

func (a *App) shortcutAccel(d runtimeShortcut) string {
	if d.ID == "global" {
		if a.cfg != nil {
			return a.cfg.Native.GlobalShortcut
		}
		return ""
	}
	if a.cfg != nil && a.cfg.Native.Shortcuts != nil {
		if v, ok := a.cfg.Native.Shortcuts[d.ID]; ok {
			return v
		}
	}
	return d.Def
}

func (a *App) shortcutsBoot() []shortcutBoot {
	out := make([]shortcutBoot, 0, len(runtimeShortcutDefs()))
	for _, d := range runtimeShortcutDefs() {
		out = append(out, shortcutBoot{
			ID: d.ID, Label: d.Label, Group: d.Group, Def: d.Def, Value: a.shortcutAccel(d),
		})
	}
	return out
}

type shortcutBoot struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Group string `json:"group"`
	Def   string `json:"def"`
	Value string `json:"value"`
}
