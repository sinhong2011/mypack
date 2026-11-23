package app

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/catalog"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/runtime/userscript"
)

// Extensions: userscripts every app runs (runtime/userscript). The page
// lists what is installed (on/off, update, remove) and the hub
// (catalog/scripts.json), and installs from an address or a file. Every
// install goes through a review sheet that says where the script runs and
// what it may do.

type extState struct {
	hub       []catalog.Script
	installed []userscript.Entry
	metas     map[string]userscript.Meta // by entry id
	loaded    bool
	busy      map[string]bool // hub URL or entry id with work in flight
	addr      string          // "Install from address" field
	addrBusy  bool
	review    extReview
	detail    extDetail
	query     string // the page's search
}

// extReview is the sheet shown before anything is installed.
type extReview struct {
	open      bool
	src       []byte
	meta      userscript.Meta
	sourceURL string
	sha       string // set for hub entries
	reviewed  bool   // from the hub
	busy      bool
	err       string
	update    bool // replaces an installed copy
}

func (a *App) extStore() userscript.Store { return userscript.Store{Dir: userscript.Dir()} }

// loadExtensions reads the installed scripts and the hub list (bundled at
// once, then the latest in the background).
func (a *App) loadExtensions() {
	a.ext.hub = catalog.BundledScripts().Scripts
	a.reloadInstalledExtensions()
	go func() {
		s := catalog.LatestScripts(context.Background())
		if a.win == nil {
			return
		}
		a.win.Update(func() { a.ext.hub = s.Scripts })
	}()
}

func (a *App) reloadInstalledExtensions() {
	st := a.extStore()
	ix, err := st.Read()
	a.ext.loaded = true
	if err != nil {
		a.toastErr(err)
		return
	}
	a.ext.installed = ix.Scripts
	a.ext.metas = map[string]userscript.Meta{}
	for _, e := range ix.Scripts {
		if m, err := st.ReadMeta(e.ID); err == nil {
			a.ext.metas[e.ID] = m
		}
	}
}

// hubInstalled finds the installed copy of a hub entry.
func (a *App) hubInstalled(s catalog.Script) *userscript.Entry {
	for i, e := range a.ext.installed {
		if e.SourceURL == s.URL || strings.EqualFold(e.Name, s.Name) {
			return &a.ext.installed[i]
		}
	}
	return nil
}

func (a *App) extBusy(key string) bool { return a.ext.busy[key] }
func (a *App) setExtBusy(key string, on bool) {
	if a.ext.busy == nil {
		a.ext.busy = map[string]bool{}
	}
	if on {
		a.ext.busy[key] = true
	} else {
		delete(a.ext.busy, key)
	}
}

// fetchForReview downloads a script and opens the review sheet.
func (a *App) fetchForReview(key, rawURL, sha string, reviewed bool) {
	a.setExtBusy(key, true)
	go func() {
		b, _, err := userscript.HTTPFetcher(nil)(context.Background(), rawURL)
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			a.setExtBusy(key, false)
			if key == "addr" {
				a.ext.addrBusy = false
			}
			if err != nil {
				a.toast(T("ext.fetch.failed"), err.Error(), 2)
				return
			}
			a.openReview(b, rawURL, sha, reviewed)
		})
	}()
}

func (a *App) openReview(src []byte, sourceURL, sha string, reviewed bool) {
	m, err := userscript.Parse(string(src))
	if err != nil {
		a.toast(T("ext.notScript"), err.Error(), 2)
		return
	}
	update := false
	for _, e := range a.ext.installed {
		update = update || e.ID == m.ID()
	}
	a.ext.review = extReview{open: true, src: src, meta: m, sourceURL: sourceURL, sha: sha, reviewed: reviewed, update: update}
}

func (a *App) chooseScriptFile() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: T("ext.file.title"),
			ButtonLabel: T("ext.file.button"),
			Filters:     []mygo.FileFilter{{Name: "Userscript", Extensions: []string{"js"}}}})
		if err != nil || len(paths) == 0 {
			return
		}
		b, err := os.ReadFile(paths[0])
		a.win.Update(func() {
			if err != nil {
				a.toastErr(err)
				return
			}
			a.openReview(b, "", "", false)
		})
	}()
}

func (a *App) installReviewed() {
	r := &a.ext.review
	r.busy, r.err = true, ""
	src, sourceURL, sha := r.src, r.sourceURL, r.sha
	go func() {
		e, _, err := a.extStore().Install(context.Background(), src, userscript.InstallOptions{SourceURL: sourceURL, SHA256: sha})
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			r := &a.ext.review
			r.busy = false
			if err != nil {
				if errors.Is(err, userscript.ErrHashMismatch) {
					r.err = T("ext.review.hash")
				} else {
					r.err = err.Error()
				}
				return
			}
			r.open = false
			a.reloadInstalledExtensions()
			a.toast(T("ext.installed.toast", "name", e.Name), T("ext.restartHint"), 1)
		})
	}()
}

func (a *App) setExtEnabled(id string, on bool) {
	if _, err := a.extStore().SetEnabled(id, on); err != nil {
		a.toastErr(err)
	}
	a.reloadInstalledExtensions()
}

func (a *App) removeExtension(e userscript.Entry) {
	if err := a.extStore().Remove(e.ID); err != nil {
		a.toastErr(err)
		return
	}
	a.reloadInstalledExtensions()
	a.toast(T("ext.removed.toast", "name", e.Name), T("ext.restartHint"), 0)
}

// updateExtension fetches the script again and reviews the new version. A
// hub entry is fetched at its current pin.
func (a *App) updateExtension(e userscript.Entry) {
	for _, s := range a.ext.hub {
		if strings.EqualFold(s.Name, e.Name) || s.URL == e.SourceURL {
			a.fetchForReview(e.ID, s.URL, s.SHA256, true)
			return
		}
	}
	if e.SourceURL != "" {
		a.fetchForReview(e.ID, e.SourceURL, "", false)
	}
}

// ---- view ---------------------------------------------------------------------

func sitesLabel(sites []string) string {
	if len(sites) == 1 && sites[0] == "*" {
		return T("ext.sites.all")
	}
	return strings.Join(sites, ", ")
}

func (a *App) extensionsView(c *ui.Context) {
	if !a.ext.loaded {
		a.reloadInstalledExtensions()
	}
	page(c, func() {
		kit.Secondary(c, T("ext.body")).LineHeight(1.45)
		kit.Note(c, icons.Info, T("ext.runtimeNote"), kit.ToneNeutral)
		if len(a.ext.installed)+len(a.ext.hub) > 3 || a.ext.query != "" {
			kit.SearchField(c.Key("q"), &a.ext.query, T("ext.search"), nil, 0)
		}
		a.extInstalledSection(c)
		a.extHubSection(c)
		a.extAddSection(c)
	})
	a.extReviewSheet(c)
	a.extDetailSheet(c)
}

func (a *App) extInstalledSection(c *ui.Context) {
	p := design.Of(c)
	ui.Column(c.Key("installed")).Gap(10).Children(func() {
		var shown []userscript.Entry
		for _, e := range a.ext.installed {
			m := a.ext.metas[e.ID]
			if extMatches(a.ext.query, e.Name, e.Description, strings.Join(m.Sites(), " ")) {
				shown = append(shown, e)
			}
		}
		title := T("ext.installed")
		if n := len(a.ext.installed); n > 0 {
			on := 0
			for _, e := range a.ext.installed {
				if e.Enabled {
					on++
				}
			}
			title = T("ext.installed.count", "on", on, "n", n)
		}
		kit.Eyebrow(c, title).Padding(0, 2)
		if len(a.ext.installed) > 0 && len(shown) == 0 {
			kit.GroupBox(c).Children(func() {
				ui.Text(c, T("ext.search.none")).FontSize(design.FontBody).TextColor(p.TextSecondary).Padding(14)
			})
			return
		}
		if len(a.ext.installed) == 0 {
			kit.GroupBox(c).Children(func() {
				ui.Text(c, T("ext.installed.none")).FontSize(design.FontBody).TextColor(p.TextSecondary).Padding(14)
			})
			return
		}
		kit.GroupBox(c).Children(func() {
			for _, e := range shown {
				e := e
				m := a.ext.metas[e.ID]
				ui.Row(c.Key(e.ID)).MinHeight(design.Row+18).Padding(12, 14).Gap(12).AlignItems(ui.Center).Children(func() {
					kit.Placeholder(c, e.Name, 32)
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.Text(c, e.Name).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
							if e.Version != "" {
								ui.Text(c, e.Version).FontSize(design.FontCaption).TextColor(p.TextTertiary).SingleLine()
							}
						})
						scope := T("ext.scope.all")
						if len(e.Apps) > 0 {
							scope = T("ext.scope.some", "n", len(e.Apps))
						}
						ui.Text(c, sitesLabel(m.Sites())+" · "+scope).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine()
					})
					if a.extBusy(e.ID) {
						kit.Spinner(c, 14, p.TextSecondary)
					}
					if kit.Button(c.Key("more"), T("ext.details"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM, Trailing: icons.ChevronRt}).Clicked() {
						a.openExtDetail(e.ID, "")
					}
					on := e.Enabled
					kit.Switch(c.Key("on"), &on, T("ext.enabled", "name", e.Name))
					if on != e.Enabled {
						a.setExtEnabled(e.ID, on)
					}
				})
			}
		})
	})
}

func (a *App) extHubSection(c *ui.Context) {
	p := design.Of(c)
	ui.Column(c.Key("hub")).Gap(10).Children(func() {
		kit.Eyebrow(c, T("ext.hub")).Padding(0, 2)
		ui.Row(c).Gap(12).Wrap().AlignItems(ui.Stretch).Children(func() {
			n := 0
			for _, s := range a.ext.hub {
				s := s
				if !extMatches(a.ext.query, s.Name, s.Description, s.Author, strings.Join(s.Sites, " ")) {
					continue
				}
				n++
				inst := a.hubInstalled(s)
				card := kit.Card(c.Key(s.URL)).Width(300).Padding(16).Gap(10)
				card.Children(func() {
					ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
						kit.Placeholder(c, s.Name, 36)
						ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
							ui.Text(c, s.Name).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
							author := s.Author
							if author == "" {
								author = T("ext.reviewed")
							}
							ui.Text(c, author).FontSize(design.FontCaption).TextColor(p.TextTertiary).SingleLine()
						})
					})
					ui.Text(c, s.Description).FontSize(design.FontSmall).TextColor(p.TextSecondary).LineHeight(1.4).MaxLines(3).Grow(1)
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						kit.Glyph(c, icons.Globe, 13, p.TextTertiary)
						ui.Text(c, strings.Join(s.Sites, ", ")).FontSize(design.FontCaption).TextColor(p.TextTertiary).SingleLine().Grow(1).MinWidth(0)
						if kit.Button(c.Key("info"), T("ext.details"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ControlSM}).Clicked() {
							id := ""
							if inst != nil {
								id = inst.ID
							}
							a.openExtDetail(id, s.URL)
						}
						switch {
						case inst != nil:
							kit.StatusPill(c, T("ext.installed.pill"), kit.ToneSuccess)
						default:
							if kit.Button(c.Key("get"), T("ext.install"), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlSM,
								Loading: a.extBusy(s.URL)}).Clicked() {
								a.fetchForReview(s.URL, s.URL, s.SHA256, true)
							}
						}
					})
				})
			}
			if n == 0 {
				ui.Text(c, T("ext.search.none")).FontSize(design.FontBody).TextColor(p.TextSecondary).Padding(4, 2)
			}
		})
	})
}

func (a *App) extAddSection(c *ui.Context) {
	ui.Column(c.Key("add")).Gap(10).Children(func() {
		kit.Eyebrow(c, T("ext.add")).Padding(0, 2)
		kit.GroupBox(c).Children(func() {
			ui.Column(c).Padding(14).Gap(10).Children(func() {
				kit.Caption(c, T("ext.add.body")).LineHeight(1.4)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					kit.TextField(c.Key("addr"), &a.ext.addr, T("ext.add.url"), kit.FieldOpts{Placeholder: "https://greasyfork.org/…/script.user.js"}).Grow(1)
					u, err := url.Parse(strings.TrimSpace(a.ext.addr))
					valid := err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
					if kit.Button(c.Key("go"), T("ext.add.fetch"), kit.ButtonOpts{Disabled: !valid, Loading: a.ext.addrBusy}).Clicked() ||
						(valid && !a.ext.addrBusy && c.Shortcut(0, ui.KeyEnter)) {
						a.ext.addrBusy = true
						a.fetchForReview("addr", strings.TrimSpace(a.ext.addr), "", false)
					}
					if kit.Button(c.Key("file"), T("ext.add.file"), kit.ButtonOpts{Icon: icons.File}).Clicked() {
						a.chooseScriptFile()
					}
				})
			})
		})
	})
}

// extReviewSheet says what a script will do before it is installed.
func (a *App) extReviewSheet(c *ui.Context) {
	r := &a.ext.review
	if !r.open {
		return
	}
	p := design.Of(c)
	m := r.meta
	kit.Sheet(c.Key("ext-review"), &r.open, 500, func() {
		title := T("ext.review.title", "name", m.Name)
		if r.update {
			title = T("ext.review.updateTitle", "name", m.Name)
		}
		kit.SheetHeader(c, title, func() { kit.Placeholder(c, m.Name, 32) })
		ui.Column(c).Gap(12).Children(func() {
			if m.Description != "" {
				kit.SheetBody(c, m.Description)
			}
			if r.reviewed {
				kit.Note(c, icons.Shield, T("ext.review.reviewed"), kit.ToneSuccess)
			} else {
				kit.Note(c, icons.Warning, T("ext.review.unreviewed"), kit.ToneWarning)
			}
			rows := [][2]string{{T("ext.review.runsOn"), sitesLabel(m.Sites())}}
			if m.Version != "" {
				rows = append(rows, [2]string{T("ext.review.version"), m.Version})
			}
			if m.Author != "" {
				rows = append(rows, [2]string{T("ext.review.author"), m.Author})
			}
			g := m.Grants()
			var can []string
			if g["values"] {
				can = append(can, T("ext.grant.values"))
			}
			if g["setclipboard"] {
				can = append(can, T("ext.grant.clipboard"))
			}
			if g["xmlhttprequest"] {
				to := strings.Join(m.Connect, ", ")
				if to == "" {
					to = T("ext.grant.self")
				}
				if len(m.Connect) == 1 && m.Connect[0] == "*" {
					to = T("ext.sites.all")
				}
				can = append(can, T("ext.grant.requests", "hosts", to))
			}
			if len(can) == 0 {
				can = append(can, T("ext.grant.none"))
			}
			rows = append(rows, [2]string{T("ext.review.can"), strings.Join(can, "\n")})
			if len(m.Require) > 0 {
				rows = append(rows, [2]string{T("ext.review.requires"), strings.Join(m.Require, "\n")})
			}
			if r.sourceURL != "" {
				rows = append(rows, [2]string{T("ext.review.source"), r.sourceURL})
			}
			kit.KeyValues(c, rows, nil)
			ui.Text(c, T("ext.review.pageAccess")).FontSize(design.FontCaption).TextColor(p.TextSecondary).LineHeight(1.4)
			if r.err != "" {
				kit.Note(c, icons.Error, r.err, kit.ToneDanger)
			}
			action := T("ext.install")
			if r.update {
				action = T("ext.update")
			}
			ok, cancel := kit.SheetFooter(c, T("common.cancel"), action, kit.Primary, r.busy, false)
			if cancel {
				r.open = false
			}
			if ok {
				a.installReviewed()
			}
		})
	})
}
