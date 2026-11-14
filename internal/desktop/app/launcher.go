package app

import (
	"log"
	"runtime"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// The quick launcher: a global shortcut (⌥⇧Space; Ctrl+Alt+Space off
// macOS; Settings › Shortcuts changes it) brings up a small panel over
// whatever app is in front, as Spotlight does, to open one of your apps by typing a few letters.
// Pinned apps come first. Enter opens the app, ⌘Enter shows it in
// MyPack; Esc, or clicking elsewhere, puts it away. It is a panel window,
// so the app the user was in stays active and gets the keyboard back.
// Settings › Shortcuts turns it off.

const (
	launcherW    = 640
	launcherRows = 8
	// The launcher's parts, whose heights size its window.
	launcherFieldH  = 56
	launcherRowH    = 46
	launcherRowGap  = 1
	launcherListPad = 6
	launcherFootH   = 32
)

type launcherState struct {
	win        *mygo.Window
	shown      bool
	query      string
	sel        int
	input      ui.Handle
	registered string // the accelerator held, "" none
	height     int    // the window's content height last given
}

// launcherHeight is the height of the launcher showing rows results:
// the field alone when there is nothing to list (rows < 0), or a line
// saying nothing matched (rows == 0). A window of native UI does not
// take its content's size by itself (mygo's FitContent is for pages), so
// the launcher gives its window this height as its results change.
func launcherHeight(rows int) int {
	if rows < 0 {
		return launcherFieldH
	}
	list := 2*launcherListPad + launcherRowH // the empty line
	if rows > 0 {
		list = 2*launcherListPad + rows*launcherRowH + (rows-1)*launcherRowGap
	}
	return launcherFieldH + 1 + list + launcherFootH // 1: the hairline, rounded up
}

func (a *App) launcherEnabled() bool { return !a.ui.LauncherOff }

// syncLauncherShortcut holds the global shortcut while the launcher is on
// and gives it back when it is turned off (main thread).
func (a *App) syncLauncherShortcut() {
	l := &a.launcher
	want := ""
	if a.launcherEnabled() && a.set.recording == "" {
		want = a.accel(scLauncher)
	}
	if l.registered == want {
		return
	}
	if l.registered != "" {
		mygo.GlobalShortcut.Unregister(l.registered)
		l.registered = ""
	}
	if want != "" {
		if err := mygo.GlobalShortcut.Register(want, a.toggleLauncher); err != nil {
			log.Printf("mypack: launcher shortcut %s: %v", want, err)
			return
		}
		l.registered = want
	}
	if l.registered == "" {
		a.hideLauncher()
	}
	a.syncTray() // its Quick Launch item
}

// toggleLauncher shows the launcher on the display under the pointer, or
// puts it away.
func (a *App) toggleLauncher() {
	l := &a.launcher
	if l.shown {
		a.hideLauncher()
		return
	}
	if l.win == nil {
		l.win = mygo.NewWindow(mygo.WindowOptions{
			Title: "MyPack", Width: launcherW, Height: launcherFieldH,
			Frameless: true, Panel: true, CornerRadius: 14, Hidden: true, DisableResize: true,
			Content: ui.View(a.launcherView),
		})
		l.win.OnBlur(func() { a.hideLauncher() })
	}
	l.query, l.sel = "", 0
	l.fit(len(a.launcherItems("")), "")
	d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	l.win.SetPosition(d.WorkArea.X+(d.WorkArea.Width-launcherW)/2, d.WorkArea.Y+d.WorkArea.Height/5)
	l.shown = true
	l.win.Show()
	l.win.Focus()
	l.input.Focus()
	l.win.Invalidate()
}

// fit sizes the launcher's window to n results for query q.
func (l *launcherState) fit(n int, q string) {
	rows := n
	if n == 0 && strings.TrimSpace(q) == "" {
		rows = -1
	}
	if h := launcherHeight(rows); l.win != nil && h != l.height {
		l.height = h
		l.win.SetContentSize(launcherW, h)
	}
}

func (a *App) hideLauncher() {
	l := &a.launcher
	if l.win != nil && l.shown {
		l.win.Hide()
	}
	l.shown = false
}

// launchItem is one result: an app to open, or an action in MyPack.
type launchItem struct {
	key    string
	name   string
	sub    string
	icon   []byte
	glyph  *ui.SVG
	pinned bool
	open   func() // Enter
	show   func() // ⌘Enter: in MyPack
}

// launcherItems is the results for q: with no query, the pinned apps,
// then the rest by name; with one, the best matches (pinned ahead of an
// equal match), the projects not installed after the apps, and an offer
// to make an app of a website address.
func (a *App) launcherItems(q string) []launchItem {
	q = strings.TrimSpace(q)
	type scored struct {
		it launchItem
		s  int
	}
	var all []scored
	pinRank := map[string]int{}
	for i, k := range a.ui.Pinned {
		pinRank[k] = i
	}
	for i := range a.installs {
		in := a.installs[i]
		key := installPinKey(&in)
		_, pinned := pinRank[key]
		site := a.installDomain(&in)
		if pr := a.project(in.ProjectID); pr != nil && pr.Config.IsGroup() {
			site = groupLine(pr.Config)
		}
		it := launchItem{key: "app:" + in.ID, name: in.Name, sub: site, icon: in.IconPNG, pinned: pinned,
			open: func() { a.openInstall(in.ID) },
			show: func() { a.showWindow(); a.nav(secLibrary); a.dlg.details = detailsDialog{open: true, id: in.ID} }}
		if in.State == domain.InstallMissing {
			it.sub = T("status.missing")
			it.open = it.show
		}
		s := fuzzy(q, in.Name+" "+site)
		if s == 0 {
			continue
		}
		switch {
		case q == "" && pinned:
			s = 1_000_000 - pinRank[key] // pin order
		case q == "":
			s = 0
		case pinned:
			s += 50
		}
		all = append(all, scored{it, s})
	}
	if q != "" {
		for i := range a.projects {
			pr := &a.projects[i]
			if a.installFor(pr.ID) != nil {
				continue
			}
			id := pr.ID
			if s := fuzzy(q, pr.Config.App.Name+" "+domainOf(pr.Config.App.Source)); s > 0 {
				show := func() { a.showWindow(); a.openProject(id, 0) }
				all = append(all, scored{launchItem{key: "prj:" + id, name: pr.Config.App.Name, sub: T("library.tab.projects"),
					icon: pr.IconPNG, open: show, show: show}, s - 400})
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].s != all[j].s {
			return all[i].s > all[j].s
		}
		return strings.ToLower(all[i].it.name) < strings.ToLower(all[j].it.name)
	})
	var out []launchItem
	for _, s := range all {
		if len(out) == launcherRows {
			break
		}
		out = append(out, s.it)
	}
	if q != "" && !strings.ContainsAny(q, " \t") && strings.Contains(q, ".") {
		if u, err := config.NormalizeURL(q); err == nil {
			makeApp := func() {
				a.showWindow()
				a.create.reset(a.host)
				a.create.setInput(u)
				a.nav(secCreate)
			}
			out = append(out, launchItem{key: "make", name: T("palette.make", "url", config.RegistrableHost(u)), sub: "MyPack",
				glyph: icons.Plus, open: makeApp, show: makeApp})
		}
	}
	return out
}

func (a *App) launcherView(c *ui.Context) {
	l := &a.launcher
	p := design.Of(c)
	c.Root().Background(p.Overlay)
	items := a.launcherItems(l.query)
	if l.sel >= len(items) {
		l.sel = max(0, len(items)-1)
	}
	run := func(i int, inMyPack bool) {
		if i < 0 || i >= len(items) {
			return
		}
		a.hideLauncher()
		if inMyPack {
			items[i].show()
		} else {
			items[i].open()
		}
		if a.win != nil {
			a.win.Invalidate()
		}
	}
	if c.Shortcut(0, ui.KeyEscape) {
		a.hideLauncher()
	}
	l.fit(len(items), l.query)
	ui.Column(c).Width(launcherW).Shrink(0).Children(func() {
		ui.Row(c).Height(launcherFieldH).Shrink(0).Padding(0, 18).Gap(12).AlignItems(ui.Center).DragWindow().Children(func() {
			kit.Glyph(c, icons.Search, 19, p.TextTertiary)
			in := ui.TextInputBase(c.Key("q"), &l.query).Bind(&l.input).Grow(1).MinWidth(0).FocusRing(false).FontSize(design.FontTitle2).
				Placeholder(T("launcher.placeholder")).PlaceholderColor(p.TextTertiary).TextColor(p.Text).Label(T("launcher.placeholder"))
			in.OnTextKey(func(k ui.TextKey) bool {
				switch k.Key {
				case ui.KeyDown:
					l.sel = min(l.sel+1, len(items)-1)
					return true
				case ui.KeyUp:
					l.sel = max(l.sel-1, 0)
					return true
				case ui.KeyEnter:
					run(l.sel, k.Mods&ui.Cmd != 0)
					return true
				case ui.KeyEscape:
					a.hideLauncher()
					return true
				}
				return false
			})
			if in.Changed() {
				l.sel = 0
			}
			kit.BrandMark(c, 20)
		})
		if len(items) == 0 && strings.TrimSpace(l.query) == "" {
			return // no apps yet: just the field
		}
		kit.Hairline(c)
		ui.Column(c).Padding(launcherListPad).Gap(launcherRowGap).Shrink(0).Role(ui.RoleListBox).Label(T("launcher.results")).Children(func() {
			if len(items) == 0 {
				ui.Row(c).Height(launcherRowH).PaddingX(14).AlignItems(ui.Center).Children(func() {
					ui.Text(c, T("palette.empty")).FontSize(design.FontBody).TextColor(p.TextTertiary)
				})
				return
			}
			for i, it := range items {
				i, it := i, it
				on := i == l.sel
				row := ui.ButtonBase(c.Key(it.key)).Label(it.name).Row().Justify(ui.Start).Height(launcherRowH).Shrink(0).PaddingX(12).Gap(12).
					AlignItems(ui.Center).Radius(design.RadiusCard - 2).Cursor(ui.CursorPointer).Role(ui.RoleOption).Selected(on).
					FocusRing(false).Description(it.sub)
				if on {
					row.Background(p.Selected)
				}
				if row.Hovered() {
					l.sel = i
				}
				row.Children(func() {
					if it.glyph != nil {
						ui.Row(c).Size(28, 28).Center().Radius(7).Background(p.Fill).Children(func() {
							kit.Glyph(c, it.glyph, 15, p.TextSecondary)
						})
					} else {
						kit.AppIconOpts(c, it.icon, it.name, 28, kit.IconOpts{})
					}
					ui.Column(c).Grow(1).MinWidth(0).Gap(0).Children(func() {
						ui.Text(c, it.name).FontSize(design.FontBodyLG).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine()
						if it.sub != "" {
							ui.Text(c, it.sub).FontSize(design.FontCaption).TextColor(p.TextTertiary).SingleLine()
						}
					})
					if it.pinned {
						kit.Glyph(c, icons.Pin, 13, p.TextTertiary).AccessHidden(true)
					}
					if on {
						selHint(c, T("action.open"))
					}
				})
				if row.Clicked() {
					run(i, false)
				}
			}
		})
		ui.Row(c).MinHeight(launcherFootH).Grow(1).Shrink(0).Padding(0, 16).Gap(14).AlignItems(ui.Center).Background(p.Sunken).Children(func() {
			for _, h := range [][2]string{{"↩", T("action.open")}, {cmdEnter(), T("launcher.showInMyPack")}, {"esc", T("launcher.close")}} {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, h[0]).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).TextColor(p.TextSecondary).
						Padding(1, 6).Radius(5).Background(p.Fill)
					kit.Footnote(c, h[1])
				})
			}
		})
	})
}

// cmdEnter is ⌘Enter as the hint bar shows it.
func cmdEnter() string {
	if runtime.GOOS == "darwin" {
		return "⌘↩"
	}
	return "Ctrl+↩"
}
