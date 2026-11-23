package app

import (
	"context"
	"errors"
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
	edit      extEditor
	query     string // the page's search
	tab       int    // 0 installed, 1 hub
	tabDir    int    // the side the last tab change came from
	adding    bool   // the Add Extension sheet
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
	a.ext.adding = false
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
			if r.update {
				if e := a.installedByID(m.ID()); e != nil && e.Edited {
					kit.Note(c, icons.Warning, T("ext.review.replacesEdits"), kit.ToneWarning)
				}
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
