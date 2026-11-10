package app

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/components"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// editorState is the Advanced Settings editor for one project (or for the
// pending config on Create when local is true).
type editorState struct {
	projectID string
	cfg       *config.Config
	rev       int64
	local     bool

	dirty    bool
	editSeq  int
	lastEdit time.Time
	saving   bool
	savedAt  time.Time
	saveErr  string
	conflict bool

	query  string
	group  int
	newDom string
	cssBuf string
	jsBuf  string
	inited bool
}

func (a *App) editorFor(pr *domain.Project) *editorState {
	ed := a.editors[pr.ID]
	if ed == nil {
		ed = &editorState{projectID: pr.ID, cfg: pr.Config.Clone(), rev: pr.Revision}
		a.editors[pr.ID] = ed
	}
	return ed
}

// syncEditors adopts newer revisions for editors without local edits.
func (a *App) syncEditors() {
	for id, ed := range a.editors {
		pr := a.project(id)
		if pr == nil {
			delete(a.editors, id)
			continue
		}
		if pr.Revision > ed.rev && !ed.dirty && !ed.saving {
			ed.cfg, ed.rev, ed.inited = pr.Config.Clone(), pr.Revision, false
		}
	}
}

func (ed *editorState) changed() {
	ed.dirty = true
	ed.editSeq++
	ed.lastEdit = time.Now()
}

// autosave writes after a short pause (spec §9.5 Autosave).
func (a *App) autosave(c *ui.Context, ed *editorState) {
	if ed.local || !ed.dirty || ed.saving || ed.conflict {
		return
	}
	wait := 700*time.Millisecond - time.Since(ed.lastEdit)
	if wait > 0 {
		c.After(wait)
		return
	}
	a.saveEditor(ed, false, ed.rev)
}

func (a *App) saveEditor(ed *editorState, confirmIdentity bool, rev int64) {
	if err := ed.cfg.Validate(); err != nil {
		ed.saveErr = validationText(err)
		ed.dirty = false
		return
	}
	ed.saving, ed.saveErr = true, ""
	cfg, seq := ed.cfg.Clone(), ed.editSeq
	var out domain.Project
	a.do(func(ctx context.Context) error {
		var err error
		out, err = a.svc.UpdateProject(ctx, appservice.UpdateRequest{ID: ed.projectID, ExpectedRevision: rev, Config: cfg,
			ConfirmIdentityChange: confirmIdentity, Actor: "gui"})
		return err
	}, func(err error) {
		ed.saving = false
		switch errCode(err) {
		case "":
			ed.rev = out.Revision
			ed.savedAt = time.Now()
			if ed.editSeq == seq {
				ed.dirty = false
			}
		case appservice.CodeRevisionConflict:
			ed.conflict = true
		case appservice.CodeIdentityChange:
			a.dlg.identity = identityDialog{open: true, ed: ed, newID: cfg.App.Identifier, fromSave: true}
		default:
			ed.saveErr = friendly(err)
			ed.dirty = false
		}
	})
}

func validationText(err error) string {
	if ve, ok := err.(*config.ValidationError); ok && len(ve.Issues) > 0 {
		return ve.Issues[0].Field + ": " + ve.Issues[0].Message
	}
	return err.Error()
}

func (a *App) saveIndicator(c *ui.Context, ed *editorState) {
	p := design.Of(c)
	ui.Row(c).Gap(6).AlignItems(ui.Center).Role(ui.RoleStatus).Children(func() {
		switch {
		case ed.saveErr != "":
			ui.Icon(c, lobeui.IconWarning()).FontSize(13).TextColor(p.Warning)
			ui.Text(c, ed.saveErr).FontSize(design.FontSmall).TextColor(p.Warning).SingleLine().MaxWidth(360)
		case ed.saving || ed.dirty:
			components.Spinner(c, 12, p.TextTertiary)
			components.Caption(c, T("editor.saving"))
		case !ed.savedAt.IsZero():
			ui.Icon(c, lobeui.IconCheck()).FontSize(13).TextColor(p.Success)
			components.Caption(c, T("editor.saved"))
		default:
			components.Caption(c, T("editor.autosave"))
		}
	})
}

func (a *App) conflictBanner(c *ui.Context, pr *domain.Project, ed *editorState) {
	if !ed.conflict {
		return
	}
	p := design.Of(c)
	ui.Row(c).Gap(10).Padding(10, 14).Radius(design.RadiusCard - 2).Background(p.WarningSoft).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, lobeui.IconWarning()).FontSize(15).TextColor(p.Warning)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, T("editor.conflict.title")).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text)
			ui.Text(c, T("editor.conflict.body")).FontSize(design.FontSmall).TextColor(p.TextSecondary)
		})
		if components.Button(c.Key("reload"), T("editor.conflict.reload"), components.ButtonOpts{Size: design.ControlSM}).Clicked() {
			ed.conflict, ed.dirty = false, false
			ed.cfg, ed.rev, ed.inited = pr.Config.Clone(), pr.Revision, false
			a.reload("projects")
		}
		if components.Button(c.Key("keep"), T("editor.conflict.keep"), components.ButtonOpts{Variant: components.Primary, Size: design.ControlSM}).Clicked() {
			ed.conflict = false
			id := ed.projectID
			a.do(func(ctx context.Context) error {
				cur, err := a.svc.GetProject(ctx, id)
				if err == nil {
					a.win.Update(func() { a.saveEditor(ed, false, cur.Revision) })
				}
				return err
			}, a.toastErr)
		}
	})
}

// ---- declarative settings ---------------------------------------------------------

type setting struct {
	label, desc string
	wide        bool
	control     func()
}

type settingGroup struct {
	id    string
	title string
	icon  *ui.SVG
	rows  []setting
}

func matches(q string, s setting) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	return q == "" || strings.Contains(strings.ToLower(s.label), q) || strings.Contains(strings.ToLower(s.desc), q)
}

// settingsEditor renders the searchable, grouped Advanced Settings (§9C.2).
func (a *App) settingsEditor(c *ui.Context, ed *editorState, pr *domain.Project) {
	p := design.Of(c)
	if !ed.inited {
		ed.inited = true
		ed.cssBuf, ed.jsBuf = injection(ed.cfg, "css"), injection(ed.cfg, "js")
	}
	groups := a.settingGroups(c, ed, pr)
	ui.Column(c).Gap(14).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			lobeui.SearchBar(c.Key("search"), &ed.query, T("editor.search"), "").Grow(1)
			if components.Button(c.Key("reset"), T("editor.reset"), components.ButtonOpts{Variant: components.Ghost, Icon: lobeui.IconUndo(), Size: design.ControlSM}).Clicked() {
				a.dlg.reset = resetDialog{open: true, ed: ed}
			}
		})
		if ed.query == "" {
			// Category chips.
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for i, g := range groups {
					i := i
					sel := ed.group == i
					chip := ui.ButtonBase(c.Key(g.id)).Label(g.title).Row().Justify(ui.Start).Height(28).PaddingX(10).Gap(6).AlignItems(ui.Center).
						Radius(14).Cursor(ui.CursorPointer).Transition(design.Transition(c)).Selected(sel)
					fg := p.TextSecondary
					switch {
					case sel:
						chip.Background(p.AccentSoft)
						fg = p.AccentText
					case chip.Hovered():
						chip.Background(p.Hover)
						fg = p.Text
					}
					chip.Children(func() {
						ui.Icon(c, g.icon).FontSize(13).TextColor(fg)
						ui.Text(c, g.title).FontSize(design.FontSmall).FontWeight(design.WeightMedium).TextColor(fg)
					})
					if chip.Clicked() {
						ed.group = i
					}
				}
			})
		}
		any := false
		for i, g := range groups {
			if ed.query == "" && i != ed.group {
				continue
			}
			var rows []setting
			for _, r := range g.rows {
				if matches(ed.query, r) {
					rows = append(rows, r)
				}
			}
			if len(rows) == 0 {
				continue
			}
			any = true
			title := ""
			if ed.query != "" {
				title = g.title
			}
			ui.Column(c.Key(g.id)).Children(func() {
				components.Group(c, title, func() {
					for k, r := range rows {
						r := r
						ui.Column(c.Key(k)).Children(func() {
							if r.wide {
								components.SettingBlock(c, r.label, r.desc, r.control)
							} else {
								components.SettingRow(c, r.label, r.desc, r.control)
							}
						})
					}
				})
			})
		}
		if !any {
			components.EmptyState(c, lobeui.IconSearch(), T("editor.noResults"), "", nil)
		}
	})
	a.autosave(c, ed)
}

func injection(cfg *config.Config, typ string) string {
	for _, in := range cfg.WebView.Injections {
		if in.Type == typ && in.Path == "" {
			return in.Content
		}
	}
	return ""
}

func setInjection(cfg *config.Config, typ, content string) {
	var out []config.Injection
	for _, in := range cfg.WebView.Injections {
		if in.Type == typ && in.Path == "" {
			continue
		}
		out = append(out, in)
	}
	if strings.TrimSpace(content) != "" {
		out = append(out, config.Injection{Type: typ, Content: content})
	}
	cfg.WebView.Injections = out
}

var sizePresets = []struct {
	id   string
	w, h int
}{{"compact", 1024, 700}, {"standard", 1200, 780}, {"large", 1440, 900}}

func (a *App) settingGroups(c *ui.Context, ed *editorState, pr *domain.Project) []settingGroup {
	p := design.Of(c)
	cfg := ed.cfg
	k := func(s string) *ui.Context { return c.Key(s) }
	sw := func(key string, v *bool, label string) {
		old := *v
		lobeui.Switch(k(key), v, label)
		if *v != old {
			ed.changed()
		}
	}
	text := func(key string, v *string, placeholder string, width float32) {
		in := lobeui.Input(k(key), v, lobeui.InputOpts{Placeholder: placeholder, Size: lobeui.Small}).Label(placeholder)
		_ = in
		if in.Changed() {
			ed.changed()
		}
		in.Width(width)
	}
	sel := func(key string, v *string, opts []lobeui.Option, width float32) {
		old := *v
		lobeui.Select(k(key), v, opts, "", false).Width(width)
		if *v != old {
			ed.changed()
		}
	}
	num := func(key string, v *int, lo, hi float64) {
		f := float64(*v)
		ui.NumberInput(k(key), &f, lo, hi, 10).Width(132)
		if int(f) != *v {
			*v = int(f)
			ed.changed()
		}
	}

	// Identity & Version.
	identity := settingGroup{id: "identity", title: T("group.identity"), icon: lobeui.IconPackage(), rows: []setting{
		{label: T("set.name"), desc: T("set.name.desc"), control: func() { text("name", &cfg.App.Name, T("set.name"), 240) }},
		{label: T("set.title"), desc: T("set.title.desc"), control: func() { text("title", &cfg.App.Title, cfg.App.Name, 240) }},
		{label: T("set.version"), control: func() { text("version", &cfg.App.Version, "1.0.0", 120) }},
		{label: T("set.identifier"), desc: T("set.identifier.desc"), control: func() {
			components.Mono(c, cfg.App.Identifier).SingleLine().MaxWidth(260)
			if !ed.local {
				if components.Button(k("chid"), T("set.identifier.change"), components.ButtonOpts{Size: design.ControlSM}).Clicked() {
					a.dlg.identity = identityDialog{open: true, ed: ed, newID: cfg.App.Identifier}
				}
			}
		}},
		{label: T("set.executable"), desc: T("set.executable.desc"), control: func() {
			components.Mono(c, config.ExecutableName(cfg.App.Name))
		}},
	}}

	// Runtime & Compatibility.
	rtVersions := []lobeui.Option{{Value: config.DefaultRuntimeVersion, Label: config.DefaultRuntimeVersion + " · " + T("set.runtime.recommended")}}
	if cfg.Runtime.Version != config.DefaultRuntimeVersion {
		rtVersions = append(rtVersions, lobeui.Option{Value: cfg.Runtime.Version, Label: cfg.Runtime.Version})
	}
	webview := map[string]string{"darwin": "WKWebView (WebKit)", "windows": "WebView2 (Edge)", "linux": "WebKitGTK"}[a.host.OS]
	runtimeG := settingGroup{id: "runtime", title: T("group.runtime"), icon: lobeui.IconRuntimes(), rows: []setting{
		{label: T("set.runtime"), desc: T("set.runtime.desc"), control: func() { sel("rt", &cfg.Runtime.Version, rtVersions, 220) }},
		{label: T("set.webview"), desc: T("set.webview.desc"), control: func() { components.Secondary(c, webview) }},
		{label: T("set.minOS"), control: func() { components.Secondary(c, map[string]string{"darwin": "macOS 12", "windows": "Windows 10", "linux": "glibc 2.31"}[a.host.OS]) }},
	}}

	// Window & Appearance.
	preset := "custom"
	for _, sp := range sizePresets {
		if cfg.Window.Width == sp.w && cfg.Window.Height == sp.h {
			preset = sp.id
		}
	}
	var presetOpts []lobeui.Option
	for _, sp := range sizePresets {
		presetOpts = append(presetOpts, lobeui.Option{Value: sp.id, Label: T("set.size."+sp.id) + fmt.Sprintf("  %d × %d", sp.w, sp.h)})
	}
	presetOpts = append(presetOpts, lobeui.Option{Value: "custom", Label: T("set.size.custom")})
	zoomOpts := []lobeui.Option{}
	for _, z := range []int{80, 90, 100, 110, 125, 150} {
		zoomOpts = append(zoomOpts, lobeui.Option{Value: strconv.Itoa(z), Label: fmt.Sprintf("%d%%", z)})
	}
	darkIdx := map[string]int{"system": 0, "light": 1, "dark": 2}[cfg.Window.DarkMode]
	windowG := settingGroup{id: "window", title: T("group.window"), icon: lobeui.IconWindow(), rows: []setting{
		{label: T("set.size"), desc: T("set.size.desc"), control: func() {
			v := preset
			lobeui.Select(k("preset"), &v, presetOpts, "", false).Width(220)
			if v != preset {
				for _, sp := range sizePresets {
					if sp.id == v {
						cfg.Window.Width, cfg.Window.Height = sp.w, sp.h
						ed.changed()
					}
				}
			}
		}},
		{label: T("set.dimensions"), control: func() {
			num("w", &cfg.Window.Width, 320, 7680)
			components.Caption(c, "×")
			num("h", &cfg.Window.Height, 240, 4320)
		}},
		{label: T("set.minSize"), control: func() {
			num("mw", &cfg.Window.MinWidth, 0, 7680)
			components.Caption(c, "×")
			num("mh", &cfg.Window.MinHeight, 0, 4320)
		}},
		{label: T("set.titleBar"), control: func() {
			sel("tb", &cfg.Window.TitleBarStyle, []lobeui.Option{{Value: "default", Label: T("set.titleBar.default")},
				{Value: "hidden", Label: T("set.titleBar.hidden")}, {Value: "overlay", Label: T("set.titleBar.overlay")}}, 200)
		}},
		{label: T("set.appearance"), control: func() {
			i := darkIdx
			lobeui.Segmented(k("dm"), &i, []string{T("appearance.system"), T("appearance.light"), T("appearance.dark")}, lobeui.Small)
			if i != darkIdx {
				cfg.Window.DarkMode = []string{"system", "light", "dark"}[i]
				ed.changed()
			}
		}},
		{label: T("set.zoom"), control: func() {
			z := strconv.Itoa(cfg.Window.Zoom)
			old := z
			lobeui.Select(k("zoom"), &z, zoomOpts, "", false).Width(100)
			if z != old {
				cfg.Window.Zoom, _ = strconv.Atoi(z)
				ed.changed()
			}
		}},
		{label: T("set.alwaysOnTop"), control: func() { sw("aot", &cfg.Window.AlwaysOnTop, T("set.alwaysOnTop")) }},
		{label: T("set.maximized"), control: func() { sw("max", &cfg.Window.Maximized, T("set.maximized")) }},
		{label: T("set.fullscreen"), control: func() { sw("fs", &cfg.Window.Fullscreen, T("set.fullscreen")) }},
		{label: T("set.hideOnClose"), desc: T("set.hideOnClose.desc"), control: func() { sw("hoc", &cfg.Window.HideOnClose, T("set.hideOnClose")) }},
	}}

	// WebView & Navigation.
	ext := cfg.WebView.Navigation.ExternalByDefault == nil || *cfg.WebView.Navigation.ExternalByDefault
	regexErr := ""
	if r := cfg.WebView.Navigation.InternalURLRegex; r != "" {
		if _, err := regexp.Compile(r); err != nil {
			regexErr = T("set.regex.invalid")
		}
	}
	webG := settingGroup{id: "webview", title: T("group.webview"), icon: lobeui.IconGlobe(), rows: []setting{
		{label: T("set.safeDomains"), desc: T("set.safeDomains.desc"), wide: true, control: func() { a.domainList(c, ed) }},
		{label: T("set.external"), desc: T("set.external.desc"), control: func() {
			v := ext
			lobeui.Switch(k("ext"), &v, T("set.external"))
			if v != ext {
				cfg.WebView.Navigation.ExternalByDefault = &v
				ed.changed()
			}
		}},
		{label: T("set.regex"), desc: T("set.regex.desc"), wide: true, control: func() {
			in := lobeui.Input(k("regex"), &cfg.WebView.Navigation.InternalURLRegex, lobeui.InputOpts{Placeholder: `^https://(www\.)?example\.com/`, Size: lobeui.Small, Invalid: regexErr != ""})
			in.Font("Menlo")
			if in.Changed() {
				ed.changed()
			}
			if regexErr != "" {
				ui.Text(c, regexErr).FontSize(design.FontSmall).TextColor(p.Danger)
			}
		}},
		{label: T("set.popups"), control: func() {
			sel("popups", &cfg.WebView.Navigation.Popups, []lobeui.Option{{Value: "internal", Label: T("set.popups.internal")},
				{Value: "external", Label: T("set.popups.external")}, {Value: "block", Label: T("set.popups.block")}}, 220)
		}},
		{label: T("set.userAgent"), desc: T("set.userAgent.desc"), wide: true, control: func() {
			in := lobeui.Input(k("ua"), &cfg.WebView.UserAgent, lobeui.InputOpts{Placeholder: T("set.userAgent.placeholder"), Size: lobeui.Small})
			if in.Changed() {
				ed.changed()
			}
		}},
		{label: T("set.incognito"), desc: T("set.incognito.desc"), control: func() { sw("inc", &cfg.WebView.Incognito, T("set.incognito")) }},
		{label: T("set.restore"), desc: T("set.restore.desc"), control: func() { sw("restore", &cfg.WebView.RestoreLastURL, T("set.restore")) }},
		{label: T("set.find"), control: func() { sw("find", &cfg.WebView.FindInPage, T("set.find")) }},
	}}

	// Network & Privacy.
	netG := settingGroup{id: "network", title: T("group.network"), icon: lobeui.IconNetwork(), rows: []setting{
		{label: T("set.proxy"), desc: T("set.proxy.desc"), control: func() { text("proxy", &cfg.Network.Proxy, "http://127.0.0.1:7890", 240) }},
		{label: T("set.basicAuth"), desc: T("set.basicAuth.desc"), control: func() { text("bau", &cfg.Network.BasicAuthUser, T("set.basicAuth.placeholder"), 200) }},
		{label: T("set.downloads"), desc: T("set.downloads.desc"), control: func() {
			text("dl", &cfg.Network.DownloadDirectory, "~/Downloads", 200)
			if components.Button(k("dlc"), T("common.choose"), components.ButtonOpts{Size: design.ControlSM}).Clicked() {
				go func() {
					paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Directory: true, CreateDirectories: true})
					if err == nil && len(paths) > 0 {
						a.win.Update(func() { cfg.Network.DownloadDirectory = paths[0]; ed.changed() })
					}
				}()
			}
		}},
	}}

	// Customization.
	custG := settingGroup{id: "custom", title: T("group.custom"), icon: lobeui.IconCode(), rows: []setting{
		{label: T("set.css"), desc: T("set.css.desc"), wide: true, control: func() {
			in := lobeui.TextArea(k("css"), &ed.cssBuf, "/* body { font-size: 15px } */", 3, 10).Font("Menlo").FontSize(12)
			if in.Changed() {
				setInjection(cfg, "css", ed.cssBuf)
				ed.changed()
			}
		}},
		{label: T("set.js"), desc: T("set.js.desc"), wide: true, control: func() {
			components.Note(c, lobeui.IconWarning(), T("set.js.risk"), components.ToneWarning)
			in := lobeui.TextArea(k("js"), &ed.jsBuf, "// document.title = 'Hello'", 3, 10).Font("Menlo").FontSize(12)
			if in.Changed() {
				setInjection(cfg, "js", ed.jsBuf)
				ed.changed()
			}
		}},
	}}

	// Native.
	nativeG := settingGroup{id: "native", title: T("group.native"), icon: lobeui.IconLayers(), rows: []setting{
		{label: T("set.tray"), desc: T("set.tray.desc"), control: func() { sw("tray", &cfg.Native.Tray, T("set.tray")) }},
		{label: T("set.startTray"), desc: T("set.startTray.desc"), control: func() {
			if !cfg.Native.Tray {
				cfg.Native.StartToTray = false
			}
			old := cfg.Native.StartToTray
			lobeui.Switch(k("st"), &cfg.Native.StartToTray, T("set.startTray")).Disabled(!cfg.Native.Tray)
			if old != cfg.Native.StartToTray {
				ed.changed()
			}
		}},
		{label: T("set.shortcut"), desc: T("set.shortcut.desc"), control: func() { text("gs", &cfg.Native.GlobalShortcut, "CmdOrCtrl+Shift+Space", 200) }},
		{label: T("set.camera"), desc: T("set.camera.desc"), control: func() { sw("cam", &cfg.Native.Camera, T("set.camera")) }},
		{label: T("set.mic"), desc: T("set.mic.desc"), control: func() { sw("mic", &cfg.Native.Microphone, T("set.mic")) }},
		{label: T("set.dragDrop"), control: func() { sw("dd", &cfg.Native.DragDrop, T("set.dragDrop")) }},
	}}

	// Packaging.
	packG := settingGroup{id: "packaging", title: T("group.packaging"), icon: lobeui.IconPackage(), rows: []setting{
		{label: T("set.targets"), desc: T("set.targets.desc"), wide: true, control: func() { a.targetEditor(c, ed) }},
		{label: T("set.output"), desc: T("set.output.desc"), control: func() { components.Mono(c, shortPath(a.settings.OutputDir)) }},
	}}

	groups := []settingGroup{identity, runtimeG, windowG, webG, netG, custG, nativeG, packG}

	// Diagnostics (projects only).
	if pr != nil {
		diag := settingGroup{id: "diagnostics", title: T("group.diagnostics"), icon: lobeui.IconStethoscope(), rows: []setting{
			{label: T("set.diag.config"), desc: T("set.diag.config.desc"), control: func() {
				if components.Button(k("cc"), T("common.copy"), components.ButtonOpts{Size: design.ControlSM, Icon: lobeui.IconCopy()}).Clicked() {
					b, _ := cfg.Marshal()
					c.WriteClipboard(string(b))
					a.toast(T("set.diag.copied"), "", 1)
				}
				if components.Button(k("ex"), T("action.export"), components.ButtonOpts{Size: design.ControlSM}).Clicked() {
					a.exportProject(pr.ID, pr.Config.App.Name)
				}
			}},
			{label: T("set.diag.revision"), control: func() { components.Mono(c, fmt.Sprintf("rev %d · %s", ed.rev, pr.ID)) }},
			{label: T("set.diag.profile"), desc: T("set.diag.profile.desc"), control: func() { components.Mono(c, cfg.App.ProfileID).SingleLine().MaxWidth(280) }},
			{label: T("set.diag.lastBuild"), control: func() {
				if j := a.latestJob(pr.ID); j != nil {
					components.Secondary(c, stageLabel(*j)+" · "+ago(j.CreatedAt))
					if components.Button(k("vl"), T("build.viewLog"), components.ButtonOpts{Size: design.ControlSM}).Clicked() {
						a.projTab = 0
						a.watch[pr.ID] = j.ID
						a.logOpen[j.ID] = true
					}
				} else {
					components.Caption(c, T("set.diag.never"))
				}
			}},
		}}
		groups = append(groups, diag)
	}
	return groups
}

func (a *App) domainList(c *ui.Context, ed *editorState) {
	p := design.Of(c)
	nav := &ed.cfg.WebView.Navigation
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).Gap(6).Wrap().Children(func() {
			remove := -1
			for i, d := range nav.SafeDomains {
				i := i
				ui.Row(c.Key(d)).Height(26).Padding(0, 4, 0, 10).Gap(4).AlignItems(ui.Center).Radius(13).Background(p.Hover).Children(func() {
					ui.Text(c, d).FontSize(design.FontSmall).TextColor(p.Text)
					x := ui.ButtonBase(c).Label(T("common.remove") + " " + d).Size(18, 18).Radius(9).Center().Cursor(ui.CursorPointer)
					if x.Hovered() {
						x.Background(p.Pressed)
					}
					x.Children(func() { ui.Icon(c, lobeui.IconClose()).FontSize(11).TextColor(p.TextSecondary) })
					if x.Clicked() {
						remove = i
					}
				})
			}
			if remove >= 0 {
				nav.SafeDomains = append(nav.SafeDomains[:remove:remove], nav.SafeDomains[remove+1:]...)
				ed.changed()
			}
		})
		add := func() {
			d := strings.ToLower(strings.TrimSpace(ed.newDom))
			d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
			d = strings.TrimSuffix(d, "/")
			if d == "" {
				return
			}
			for _, x := range nav.SafeDomains {
				if x == d {
					ed.newDom = ""
					return
				}
			}
			nav.SafeDomains = append(nav.SafeDomains, d)
			ed.newDom = ""
			ed.changed()
		}
		ui.Row(c).Gap(8).Children(func() {
			in := lobeui.Input(c.Key("newdom"), &ed.newDom, lobeui.InputOpts{Placeholder: T("set.safeDomains.add"), Size: lobeui.Small}).Label(T("set.safeDomains.add"))
			if in.Submitted() {
				add()
			}
			if components.Button(c.Key("adddom"), T("common.add"), components.ButtonOpts{Size: design.ControlSM, Disabled: strings.TrimSpace(ed.newDom) == ""}).Clicked() {
				add()
			}
		})
	})
}

func (a *App) targetEditor(c *ui.Context, ed *editorState) {
	p := design.Of(c)
	all := append([]config.Target{config.DefaultTarget(a.host)}, otherTargets(a.host)...)
	has := func(t config.Target) int {
		for i, x := range ed.cfg.Build.Targets {
			if x.OS == t.OS && x.Arch == t.Arch {
				return i
			}
		}
		return -1
	}
	ui.Column(c).Gap(6).Children(func() {
		for _, t := range all {
			t := t
			i := has(t)
			unsupported := t.OS == "darwin" && a.host.OS != "darwin"
			label := targetLabel(t)
			if t.OS == a.host.OS && t.Arch == a.host.Arch {
				label = thisDevice(a.host)
			}
			ui.Row(c.Key(t.Key())).Gap(10).AlignItems(ui.Center).Children(func() {
				on := i >= 0
				old := on
				lobeui.Checkbox(c, &on, label).Disabled(unsupported)
				if on != old {
					if on {
						ed.cfg.Build.Targets = append(ed.cfg.Build.Targets, t)
					} else if len(ed.cfg.Build.Targets) > 1 {
						ed.cfg.Build.Targets = append(ed.cfg.Build.Targets[:i:i], ed.cfg.Build.Targets[i+1:]...)
					}
					ed.changed()
				}
				ui.Spacer(c)
				reason := map[string]string{"darwin": T("target.reason.darwin"), "windows": T("target.reason.windows"), "linux": T("target.reason.linux")}[t.OS]
				if unsupported {
					reason = T("target.needsMac")
				}
				ui.Text(c, reason).FontSize(design.FontSmall).TextColor(p.TextTertiary).SingleLine().MinWidth(0).Shrink(1)
			})
		}
	})
}
