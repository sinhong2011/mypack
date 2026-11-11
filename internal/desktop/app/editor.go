package app

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
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

	query    string
	search   searchState // the header's search: icon → field
	group    int
	headers  map[string]*ui.Handle // accordion rows, for ↑/↓
	moveDown string
	newDom   string
	cssBuf   string
	jsBuf    string
	nums     map[string]string // number fields' text while being edited
	inited   bool
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
			kit.Glyph(c, icons.Warning, 13, p.Warning)
			ui.Text(c, ed.saveErr).FontSize(design.FontSmall).TextColor(p.Warning).SingleLine().MaxWidth(360)
		case ed.saving || ed.dirty:
			kit.Spinner(c, 12, p.TextTertiary)
			kit.Caption(c, T("editor.saving"))
		case !ed.savedAt.IsZero():
			kit.Glyph(c, icons.Check, 13, p.Success)
			kit.Caption(c, T("editor.saved"))
		default:
			kit.Caption(c, T("editor.autosave"))
		}
	})
}

func (a *App) conflictBanner(c *ui.Context, pr *domain.Project, ed *editorState) {
	if !ed.conflict {
		return
	}
	p := design.Of(c)
	ui.Row(c).Gap(10).Padding(10, 14).Radius(design.RadiusCard - 2).Background(p.WarningSoft).AlignItems(ui.Center).Children(func() {
		kit.Glyph(c, icons.Warning, 15, p.Warning)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, T("editor.conflict.title")).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text)
			ui.Text(c, T("editor.conflict.body")).FontSize(design.FontSmall).TextColor(p.TextSecondary)
		})
		if kit.Button(c.Key("reload"), T("editor.conflict.reload"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
			ed.conflict, ed.dirty = false, false
			ed.cfg, ed.rev, ed.inited = pr.Config.Clone(), pr.Revision, false
			a.reload("projects")
		}
		if kit.Button(c.Key("keep"), T("editor.conflict.keep"), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlSM}).Clicked() {
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
	// summary is a quiet line of the group's current values; modified
	// reports a change from the recommended defaults; reset restores
	// them for this group only (nil: no reset).
	summary  func() string
	modified func() bool
	reset    func()
}

// settingOrder is the accordion's order, most-used first.
var settingOrder = []string{"window", "webview", "custom", "native", "network", "identity", "runtime", "packaging", "diagnostics"}

func matches(q string, s setting) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	return q == "" || strings.Contains(strings.ToLower(s.label), q) || strings.Contains(strings.ToLower(s.desc), q)
}

// settingsEditor renders Advanced Settings as an accordion (§9C.2): one
// full-width row per group (icon, name, a dot when it differs from the
// recommended defaults, a quiet summary of its values, a chevron), opening
// in place; several can be open, remembered per project. Search filters to
// matching settings, opens their groups and marks the matches; clearing
// it restores what was open.
func (a *App) settingsEditor(c *ui.Context, ed *editorState, pr *domain.Project) {
	p := design.Of(c)
	if !ed.inited {
		ed.inited = true
		ed.cssBuf, ed.jsBuf = injection(ed.cfg, "css"), injection(ed.cfg, "js")
	}
	groups := a.settingGroups(c, ed, pr)
	a.describeGroups(ed, groups)
	sort.SliceStable(groups, func(i, j int) bool {
		return slices.Index(settingOrder, groups[i].id) < slices.Index(settingOrder, groups[j].id)
	})
	scope := "create"
	if pr != nil {
		scope = pr.ID
	}
	key := func(gid string) string { return scope + "/" + gid }
	if a.ui.SettingsOpen == nil {
		a.ui.SettingsOpen = map[string]bool{}
	}
	if ed.headers == nil {
		ed.headers = map[string]*ui.Handle{}
	}
	q := strings.TrimSpace(ed.query)
	// The same grouped card as the common settings above: each group is a
	// row (icon, name, summary, chevron) and opens in place into its own
	// hairline rows, indented to the name.
	type shownGroup struct {
		g    settingGroup
		rows []setting
	}
	var list []shownGroup
	for _, g := range groups {
		var rows []setting
		for _, r := range g.rows {
			if matches(q, r) {
				rows = append(rows, r)
			}
		}
		if len(rows) > 0 {
			list = append(list, shownGroup{g, rows})
		}
	}
	shown := make([]string, len(list))
	for i, sg := range list {
		shown[i] = sg.g.id
		if q != "" {
			a.settingHits += len(sg.rows)
		}
	}
	ui.Column(c).Gap(0).Children(func() {
		if len(list) > 0 {
			kit.GroupBox(c.Key("card")).Children(func() {
				for _, sg := range list {
					g, rows := sg.g, sg.rows
					open := q != "" || a.ui.SettingsOpen[key(g.id)]
					h := ed.headers[g.id]
					if h == nil {
						h = &ui.Handle{}
						ed.headers[g.id] = h
					}
					ui.Column(c.Key(g.id)).Children(func() {
						hdr := ui.ButtonBase(c.Key("h")).Label(g.title).Row().Justify(ui.Start).MinHeight(design.Row+4).Padding(10, 14).Gap(12).
							AlignItems(ui.Center).Cursor(ui.CursorPointer).Role(ui.RoleDisclosure).Expanded(open).
							FocusRing(false).Transition(design.Transition(c)).Bind(h)
						if hdr.Hovered() || hdr.FocusVisible() {
							hdr.Background(p.Hover)
						}
						hdr.Children(func() {
							kit.FocusMark(c, hdr.FocusVisible(), 0)
							ui.Row(c).Size(20, 20).Center().Shrink(0).Children(func() { kit.Glyph(c, g.icon, 16, p.TextSecondary) })
							ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
								ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
									ui.Text(c, g.title).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine()
									if g.modified != nil && g.modified() {
										ui.Box(c).Size(6, 6).Radius(3).Background(p.Accent).Label(T("editor.modified"))
									}
								})
								if g.summary != nil {
									if sm := g.summary(); sm != "" {
										ui.Text(c, sm).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine().MinWidth(0)
									}
								}
							})
							ch := kit.Glyph(c, icons.ChevronRt, 14, p.TextTertiary)
							turn := float32(0)
							if open {
								turn = 90
							}
							ch.Rotate(ch.AnimateTo("turn", turn, design.Spring))
						})
						if hdr.Clicked() && q == "" {
							a.ui.SettingsOpen[key(g.id)] = !a.ui.SettingsOpen[key(g.id)]
							a.ui.save()
						}
						// ↑/↓ move between the rows.
						if hdr.Focused() {
							i := slices.Index(shown, g.id)
							if c.Shortcut(0, ui.KeyUp) && i > 0 {
								ed.headers[shown[i-1]].Focus()
							}
							if c.Shortcut(0, ui.KeyDown) {
								ed.moveDown = g.id
							}
						}
						if open {
							kit.Reveal(ui.Column(c.Key("body")).Padding(0, 0, 4, 32).BorderWidth(0.5, 0, 0, 0).BorderColor(p.Divider).
								Dividers(0.5, p.Divider)).Children(func() {
								for k, r := range rows {
									r := r
									row := ui.Column(c.Key(k))
									if q != "" {
										row.Background(p.ChoiceFill) // a match
									}
									row.Children(func() {
										if r.wide {
											kit.SettingBlock(c, r.label, r.desc, r.control)
										} else {
											kit.SettingRow(c, r.label, r.desc, r.control)
										}
									})
								}
								if g.reset != nil && q == "" {
									reset := g.reset
									ui.Row(c.Key("resetrow")).Padding(6, 8).Children(func() {
										if kit.Button(c.Key("reset"), T("editor.resetGroup"), kit.ButtonOpts{Variant: kit.Ghost, Size: 26}).FontWeight(design.WeightRegular).Clicked() {
											reset()
											ed.changed()
										}
									})
								}
							})
						}
					})
				}
			})
		}
		// ↓ from the row before: focus the next one (it has a handle by now).
		if ed.moveDown != "" {
			if i := slices.Index(shown, ed.moveDown); i >= 0 && i+1 < len(shown) {
				ed.headers[shown[i+1]].Focus()
			}
			ed.moveDown = ""
		}
		if q == "" && kit.Button(c.Key("resetall"), T("editor.reset"), kit.ButtonOpts{Variant: kit.Ghost, Size: 26}).FontWeight(design.WeightRegular).Margin(10, 0, 0, 0).AlignSelf(ui.Start).Clicked() {
			a.dlg.reset = resetDialog{open: true, ed: ed}
		}
	})
	a.autosave(c, ed)
}

// searchState is the Advanced Settings header's search.
type searchState struct {
	h ui.Handle
}

// settingsSearch is the inspector's search, pinned at its top: it filters
// every setting in the column, common and advanced. ⌘F focuses it; Esc
// clears it.
func (a *App) settingsSearch(c *ui.Context, ed *editorState) {
	s := &ed.search
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		s.h.Focus()
	}
	kit.SearchField(c.Key("search"), &ed.query, T("editor.search"), &s.h, 0)
	if s.h.Focused(c) && c.Shortcut(0, ui.KeyEscape) {
		ed.query = ""
	}
}

// searchEmpty is the column's empty state when the search matched nothing.
func (a *App) searchEmpty(c *ui.Context) {
	if a.settingsQ != "" && a.settingHits == 0 {
		kit.EmptyState(c, icons.Search, T("editor.noResults"), "", nil)
	}
}

// describeGroups gives the groups their summaries, default checks and
// resets, against a fresh config with the recommended defaults.
func (a *App) describeGroups(ed *editorState, groups []settingGroup) {
	cfg := ed.cfg
	def := config.New(cfg.App.Name, cfg.App.Source, a.host)
	for i := range groups {
		g := &groups[i]
		switch g.id {
		case "window":
			g.summary = func() string {
				w := cfg.Window
				tb := T("look.titleBar.standard")
				if w.TitleBarStyle == "overlay" || w.TitleBarStyle == "hidden" {
					tb = T("look.titleBar.seamless")
				}
				ap := map[string]string{"": T("appearance.system"), "system": T("appearance.system"), "light": T("appearance.light"), "dark": T("appearance.dark")}[w.DarkMode]
				size := fmt.Sprintf("%d × %d", w.Width, w.Height)
				if w.Maximized {
					size = T("look.size.fullScreen")
				}
				return strings.Join([]string{tb, ap, size}, " · ")
			}
			g.modified = func() bool { return !reflect.DeepEqual(cfg.Window, def.Window) }
			g.reset = func() { cfg.Window = def.Window }
		case "webview":
			g.summary = func() string {
				ua := map[string]string{config.UAPresetSafari: "Safari", config.UAPresetChrome: "Chrome", config.UAPresetEdge: "Edge",
					config.UAPresetFirefox: "Firefox", config.UAPresetMobileSafari: T("set.userAgent.mobileSafari"),
					config.UAPresetCustom: T("set.userAgent.custom")}[cfg.WebView.EffectiveUAPreset()]
				links := T("editor.sum.linksBrowser")
				if e := cfg.WebView.Navigation.ExternalByDefault; e != nil && !*e {
					links = T("editor.sum.linksApp")
				}
				return T("editor.sum.ua", "ua", ua) + " · " + links
			}
			g.modified = func() bool {
				a, b := cfg.WebView, def.WebView
				a.Injections, b.Injections = nil, nil
				return !reflect.DeepEqual(a, b)
			}
			g.reset = func() {
				inj := cfg.WebView.Injections
				cfg.WebView = def.WebView
				cfg.WebView.Injections = inj
			}
		case "custom":
			g.summary = func() string {
				var parts []string
				if injection(cfg, "css") != "" {
					parts = append(parts, "CSS")
				}
				if injection(cfg, "js") != "" {
					parts = append(parts, "JavaScript")
				}
				if len(parts) == 0 {
					return T("editor.sum.none")
				}
				return strings.Join(parts, " · ")
			}
			g.modified = func() bool { return len(cfg.WebView.Injections) > 0 }
			g.reset = func() { cfg.WebView.Injections, ed.cssBuf, ed.jsBuf = nil, "", "" }
		case "native":
			g.summary = func() string {
				n := map[string]string{"allow": T("look.notifications.allow"), "ask": T("look.notifications.ask"), "deny": T("look.notifications.off")}[cfg.Native.NotificationsMode()]
				parts := []string{T("editor.sum.notifications", "mode", n)}
				if cfg.Native.Tray {
					parts = append(parts, T("editor.sum.menuBar"))
				}
				return strings.Join(parts, " · ")
			}
			g.modified = func() bool { return !reflect.DeepEqual(cfg.Native, def.Native) }
			g.reset = func() { cfg.Native = def.Native; cfg.Window.HideOnClose = def.Window.HideOnClose }
		case "network":
			g.summary = func() string {
				if cfg.Network.Proxy != "" {
					return T("editor.sum.proxy", "proxy", cfg.Network.Proxy)
				}
				return T("editor.sum.direct")
			}
			g.modified = func() bool { return !reflect.DeepEqual(cfg.Network, def.Network) }
			g.reset = func() { cfg.Network = def.Network }
		case "identity":
			g.summary = func() string { return cfg.App.Identifier + " · " + cfg.App.Version }
		case "runtime":
			g.summary = func() string { return T("editor.sum.runtime", "version", cfg.Runtime.Version) }
		}
	}
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
}{{"compact", 1000, 700}, {"standard", 1200, 780}, {"large", 1440, 900}}

func (a *App) settingGroups(c *ui.Context, ed *editorState, pr *domain.Project) []settingGroup {
	p := design.Of(c)
	cfg := ed.cfg
	k := func(s string) *ui.Context { return c.Key(s) }
	sw := func(key string, v *bool, label string) {
		old := *v
		kit.Switch(k(key), v, label)
		if *v != old {
			ed.changed()
		}
	}
	text := func(key string, v *string, label, placeholder string, width float32) {
		in := kit.TextField(k(key), v, label, kit.FieldOpts{Placeholder: placeholder})
		_ = in
		if in.Changed() {
			ed.changed()
		}
		in.Width(width)
	}
	sel := func(key string, v *string, opts []kit.Option, label string, width float32) {
		old := *v
		kit.PopUp(k(key), v, opts, label).Width(width)
		if *v != old {
			ed.changed()
		}
	}
	// num is a compact numeric field (no stepper buttons): typing applies
	// as soon as the text is a whole number in range; anything else is
	// kept as typed and marked invalid until it is fixed.
	num := func(key string, v *int, lo, hi float64) {
		if ed.nums == nil {
			ed.nums = map[string]string{}
		}
		buf, ok := ed.nums[key]
		if !ok || (buf != strconv.Itoa(*v) && validInt(buf, lo, hi)) {
			buf = strconv.Itoa(*v)
		}
		bad := !validInt(buf, lo, hi)
		in := kit.TextField(k(key), &buf, T("set.dimensions"), kit.FieldOpts{Invalid: bad}).Width(72)
		ed.nums[key] = buf
		if in.Changed() && validInt(buf, lo, hi) {
			n, _ := strconv.Atoi(buf)
			if n != *v {
				*v = n
				ed.changed()
			}
		}
	}

	// Identity & Version.
	identity := settingGroup{id: "identity", title: T("group.identity"), icon: icons.Package, rows: []setting{
		{label: T("set.name"), desc: T("set.name.desc"), control: func() { text("name", &cfg.App.Name, T("set.name"), T("set.name"), 240) }},
		{label: T("set.title"), desc: T("set.title.desc"), control: func() { text("title", &cfg.App.Title, T("set.title"), cfg.App.Name, 240) }},
		{label: T("set.version"), control: func() { text("version", &cfg.App.Version, T("set.version"), "1.0.0", 120) }},
		{label: T("set.identifier"), desc: T("set.identifier.desc"), control: func() {
			kit.Mono(c, cfg.App.Identifier).SingleLine().MaxWidth(260)
			if !ed.local {
				if kit.Button(k("chid"), T("set.identifier.change"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
					a.dlg.identity = identityDialog{open: true, ed: ed, newID: cfg.App.Identifier}
				}
			}
		}},
		{label: T("set.executable"), desc: T("set.executable.desc"), control: func() {
			kit.Mono(c, config.ExecutableName(cfg.App.Name))
		}},
	}}

	// Runtime & Compatibility.
	rtVersions := []kit.Option{{Value: config.DefaultRuntimeVersion, Label: config.DefaultRuntimeVersion + " · " + T("set.runtime.recommended")}}
	if cfg.Runtime.Version != config.DefaultRuntimeVersion {
		rtVersions = append(rtVersions, kit.Option{Value: cfg.Runtime.Version, Label: cfg.Runtime.Version})
	}
	webview := map[string]string{"darwin": "WKWebView (WebKit)", "windows": "WebView2 (Edge)", "linux": "WebKitGTK"}[a.host.OS]
	runtimeG := settingGroup{id: "runtime", title: T("group.runtime"), icon: icons.Runtimes, rows: []setting{
		{label: T("set.runtime"), desc: T("set.runtime.desc"), control: func() { sel("rt", &cfg.Runtime.Version, rtVersions, T("set.runtime"), 220) }},
		{label: T("set.webview"), desc: T("set.webview.desc"), control: func() { kit.Secondary(c, webview) }},
		{label: T("set.minOS"), control: func() {
			kit.Secondary(c, map[string]string{"darwin": "macOS 12", "windows": "Windows 10", "linux": "glibc 2.31"}[a.host.OS])
		}},
	}}

	// Window & Appearance.
	zoomOpts := []kit.Option{}
	for _, z := range []int{80, 90, 100, 110, 125, 150} {
		zoomOpts = append(zoomOpts, kit.Option{Value: strconv.Itoa(z), Label: fmt.Sprintf("%d%%", z)})
	}
	windowG := settingGroup{id: "window", title: T("group.window"), icon: icons.Window, rows: []setting{
		{label: T("set.dimensions"), control: func() {
			num("w", &cfg.Window.Width, 320, 7680)
			kit.Caption(c, "×")
			num("h", &cfg.Window.Height, 240, 4320)
			kit.Caption(c, "pt")
		}},
		{label: T("set.minSize"), control: func() {
			num("mw", &cfg.Window.MinWidth, 0, 7680)
			kit.Caption(c, "×")
			num("mh", &cfg.Window.MinHeight, 0, 4320)
			kit.Caption(c, "pt")
		}},
		{label: T("set.zoom"), control: func() {
			z := strconv.Itoa(cfg.Window.Zoom)
			old := z
			kit.PopUp(k("zoom"), &z, zoomOpts, T("set.zoom")).Width(100)
			if z != old {
				cfg.Window.Zoom, _ = strconv.Atoi(z)
				ed.changed()
			}
		}},
		{label: T("set.alwaysOnTop"), control: func() { sw("aot", &cfg.Window.AlwaysOnTop, T("set.alwaysOnTop")) }},
		{label: T("set.fullscreen"), control: func() { sw("fs", &cfg.Window.Fullscreen, T("set.fullscreen")) }},
	}}

	// WebView & Navigation.
	regexErr := ""
	if r := cfg.WebView.Navigation.InternalURLRegex; r != "" {
		if _, err := regexp.Compile(r); err != nil {
			regexErr = T("set.regex.invalid")
		}
	}
	webG := settingGroup{id: "webview", title: T("group.webview"), icon: icons.Globe, rows: []setting{
		{label: T("set.safeDomains"), desc: T("set.safeDomains.desc"), wide: true, control: func() { a.domainList(c, ed) }},
		{label: T("set.regex"), desc: T("set.regex.desc"), wide: true, control: func() {
			in := kit.TextField(k("regex"), &cfg.WebView.Navigation.InternalURLRegex, T("set.regex"), kit.FieldOpts{Placeholder: `^https://(www\.)?example\.com/`, Invalid: regexErr != "", Mono: true})
			if in.Changed() {
				ed.changed()
			}
			if regexErr != "" {
				ui.Text(c, regexErr).FontSize(design.FontSmall).TextColor(p.Danger)
			}
		}},
		{label: T("set.popups"), control: func() {
			sel("popups", &cfg.WebView.Navigation.Popups, []kit.Option{{Value: "internal", Label: T("set.popups.internal")},
				{Value: "external", Label: T("set.popups.external")}, {Value: "block", Label: T("set.popups.block")}}, T("set.popups"), 220)
		}},
		{label: T("set.userAgent"), desc: T("set.userAgent.desc"), wide: true, control: func() {
			// A preset by default; Custom shows the text field.
			preset := cfg.WebView.EffectiveUAPreset()
			v := preset
			kit.PopUp(k("uap"), &v, []kit.Option{{Value: config.UAPresetSafari, Label: "Safari"}, {Value: config.UAPresetChrome, Label: "Chrome"},
				{Value: config.UAPresetEdge, Label: "Edge"}, {Value: config.UAPresetFirefox, Label: "Firefox"},
				{Value: config.UAPresetMobileSafari, Label: T("set.userAgent.mobileSafari")}, {Value: config.UAPresetCustom, Label: T("set.userAgent.custom")}},
				T("set.userAgent")).Width(200)
			if v != preset {
				cfg.WebView.UserAgentPreset = v
				if v != config.UAPresetCustom {
					cfg.WebView.UserAgent = ""
				}
				ed.changed()
			}
			if v == config.UAPresetCustom {
				in := kit.TextField(k("ua"), &cfg.WebView.UserAgent, T("set.userAgent"), kit.FieldOpts{Placeholder: T("set.userAgent.placeholder")})
				if in.Changed() {
					ed.changed()
				}
			}
		}},
		{label: T("set.restore"), desc: T("set.restore.desc"), control: func() { sw("restore", &cfg.WebView.RestoreLastURL, T("set.restore")) }},
		{label: T("set.find"), control: func() { sw("find", &cfg.WebView.FindInPage, T("set.find")) }},
	}}

	// Network & Privacy.
	netG := settingGroup{id: "network", title: T("group.network"), icon: icons.Network, rows: []setting{
		{label: T("set.proxy"), desc: T("set.proxy.desc"), control: func() { text("proxy", &cfg.Network.Proxy, T("set.proxy"), "http://127.0.0.1:7890", 240) }},
		{label: T("set.basicAuth"), desc: T("set.basicAuth.desc"), control: func() {
			text("bau", &cfg.Network.BasicAuthUser, T("set.basicAuth"), T("set.basicAuth.placeholder"), 200)
		}},
		{label: T("set.downloads"), desc: T("set.downloads.desc"), control: func() {
			text("dl", &cfg.Network.DownloadDirectory, T("set.downloads"), "~/Downloads", 200)
			if kit.Button(k("dlc"), T("common.choose"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
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
	custG := settingGroup{id: "custom", title: T("group.custom"), icon: icons.Code, rows: []setting{
		{label: T("set.css"), desc: T("set.css.desc"), wide: true, control: func() {
			in := ui.TextArea(k("css"), &ed.cssBuf).Placeholder("/* body { font-size: 15px } */").Lines(3, 10).Font(kit.MonoFont).FontSize(12).Label(T("set.css")).Radius(design.RadiusControl)
			if in.Changed() {
				setInjection(cfg, "css", ed.cssBuf)
				ed.changed()
			}
		}},
		{label: T("set.js"), desc: T("set.js.desc"), wide: true, control: func() {
			ui.Text(c, T("set.js.risk")).FontSize(design.FontCaption).TextColor(p.TextTertiary)
			in := ui.TextArea(k("js"), &ed.jsBuf).Placeholder("// document.title = 'Hello'").Lines(3, 10).Font(kit.MonoFont).FontSize(12).Label(T("set.js")).Radius(design.RadiusControl)
			if in.Changed() {
				setInjection(cfg, "js", ed.jsBuf)
				ed.changed()
			}
		}},
	}}

	// Native.
	nativeG := settingGroup{id: "native", title: T("group.native"), icon: icons.Layers, rows: []setting{
		{label: T("set.startTray"), desc: T("set.startTray.desc"), control: func() {
			if !cfg.Native.Tray {
				cfg.Native.StartToTray = false
			}
			old := cfg.Native.StartToTray
			kit.Switch(k("st"), &cfg.Native.StartToTray, T("set.startTray")).Disabled(!cfg.Native.Tray)
			if old != cfg.Native.StartToTray {
				ed.changed()
			}
		}},
		{label: T("set.shortcut"), desc: T("set.shortcut.desc"), control: func() { text("gs", &cfg.Native.GlobalShortcut, T("set.shortcut"), "CmdOrCtrl+Shift+Space", 200) }},
		{label: T("set.linkRouting"), desc: T("set.linkRouting.desc"), control: func() {
			on := cfg.Native.LinkRoutingEnabled()
			old := on
			kit.Switch(k("lr"), &on, T("set.linkRouting"))
			if on != old {
				if on {
					cfg.Native.LinkRouting = nil
				} else {
					off := false
					cfg.Native.LinkRouting = &off
				}
				ed.changed()
			}
		}},
		{label: T("set.camera"), desc: T("set.camera.desc"), control: func() { sw("cam", &cfg.Native.Camera, T("set.camera")) }},
		{label: T("set.mic"), desc: T("set.mic.desc"), control: func() { sw("mic", &cfg.Native.Microphone, T("set.mic")) }},
		{label: T("set.dragDrop"), control: func() { sw("dd", &cfg.Native.DragDrop, T("set.dragDrop")) }},
	}}

	// Packaging.
	packG := settingGroup{id: "packaging", title: T("group.packaging"), icon: icons.Package, rows: []setting{
		{label: T("set.targets"), desc: T("set.targets.desc"), wide: true, control: func() { a.targetEditor(c, ed) }},
		{label: T("set.output"), desc: T("set.output.desc"), control: func() { kit.Mono(c, shortPath(a.settings.OutputDir)) }},
	}}

	groups := []settingGroup{identity, runtimeG, windowG, webG, netG, custG, nativeG, packG}

	// Diagnostics (projects only).
	if pr != nil {
		diag := settingGroup{id: "diagnostics", title: T("group.diagnostics"), icon: icons.Stethoscope, rows: []setting{
			{label: T("set.diag.config"), desc: T("set.diag.config.desc"), control: func() {
				if kit.Button(k("cc"), T("common.copy"), kit.ButtonOpts{Size: design.ControlSM, Icon: icons.Copy}).Clicked() {
					b, _ := cfg.Marshal()
					c.WriteClipboard(string(b))
					a.toast(T("set.diag.copied"), "", 1)
				}
				if kit.Button(k("ex"), T("action.export"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
					a.exportProject(pr.ID, pr.Config.App.Name)
				}
			}},
			{label: T("set.diag.revision"), control: func() { kit.Mono(c, fmt.Sprintf("rev %d · %s", ed.rev, pr.ID)) }},
			{label: T("set.diag.profile"), desc: T("set.diag.profile.desc"), control: func() { kit.Mono(c, cfg.App.ProfileID).SingleLine().MaxWidth(280) }},
			{label: T("set.diag.lastBuild"), control: func() {
				if j := a.latestJob(pr.ID); j != nil {
					kit.Secondary(c, stageLabel(*j)+" · "+ago(j.CreatedAt))
					if kit.Button(k("vl"), T("build.viewLog"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
						a.projTab = 0
						a.watch[pr.ID] = j.ID
						a.logOpen[j.ID] = true
					}
				} else {
					kit.Caption(c, T("set.diag.never"))
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
					x := ui.ButtonBase(c).Label(T("common.remove")+" "+d).Size(18, 18).Radius(9).Center().Cursor(ui.CursorPointer)
					if x.Hovered() {
						x.Background(p.Pressed)
					}
					x.Children(func() { kit.Glyph(c, icons.Close, 11, p.TextSecondary) })
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
			in := kit.TextField(c.Key("newdom"), &ed.newDom, T("set.safeDomains.add"), kit.FieldOpts{Placeholder: T("set.safeDomains.add")}).Grow(1)
			if in.Submitted() {
				add()
			}
			if kit.Button(c.Key("adddom"), T("common.add"), kit.ButtonOpts{Size: design.ControlSM, Disabled: strings.TrimSpace(ed.newDom) == ""}).Clicked() {
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
				kit.Checkbox(c, &on, label).Disabled(unsupported)
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

// validInt reports whether s is a whole number within [lo, hi].
func validInt(s string, lo, hi float64) bool {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	return err == nil && float64(n) >= lo && float64(n) <= hi
}
