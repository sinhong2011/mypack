package app

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
)

// Moving apps between computers: File > Export Library… and Import Apps…
// (.mypack files with icons, identity kept), a project's Export…, and
// Library sync (Pro) through a synced folder, run in the background while
// mypack is open.

const syncEvery = 2 * time.Minute

type librarySync struct {
	mu      sync.Mutex
	last    *appservice.SyncResult
	err     string
	busy    bool
	pending *time.Timer
}

// importSkip is the sheet that offers to replace apps an import skipped.
type importSkip struct {
	open  bool
	path  string
	names []string
}

func saveMypack(a *App, name string) (string, error) {
	return mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, DefaultPath: name,
		Filters: []mygo.FileFilter{{Name: "Mypack", Extensions: []string{"mypack"}}}})
}

// exportLibrary saves projects ids (all when empty) as a .mypack file.
func (a *App) exportLibrary(ids []string, name string) {
	a.showWindow()
	go func() {
		path, err := saveMypack(a, strings.ReplaceAll(name, "/", "-")+appservice.LibraryExt)
		if err != nil || path == "" {
			return
		}
		if !strings.HasSuffix(strings.ToLower(path), appservice.LibraryExt) {
			path += appservice.LibraryExt
		}
		var n int
		a.do(func(ctx context.Context) error {
			var err error
			n, err = a.svc.ExportLibrary(ctx, ids, path)
			return err
		}, func(err error) {
			if err != nil {
				a.toastErr(err)
				return
			}
			a.toast(T("library.export.done", "count", n), path, 1)
		})
	}()
}

// importApps picks a .mypack file and imports it.
func (a *App) importApps() {
	a.showWindow()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: T("library.import.title"),
			ButtonLabel: T("library.import.button"), Filters: []mygo.FileFilter{{Name: "Mypack", Extensions: []string{"mypack"}}}})
		if err != nil || len(paths) == 0 {
			return
		}
		a.win.Update(func() { a.importLibraryFile(paths[0], false) })
	}()
}

func (a *App) importLibraryFile(path string, replace bool) {
	var res appservice.ImportResult
	a.do(func(ctx context.Context) error {
		var err error
		res, err = a.svc.ImportLibrary(ctx, path, appservice.ImportOptions{Replace: replace, Actor: "gui"})
		return err
	}, func(err error) {
		if err != nil {
			a.toastErr(err)
			return
		}
		n := len(res.Added) + len(res.Updated)
		if n > 0 {
			a.toast(T("library.import.done", "count", n), strings.Join(append(res.Warnings, res.Refused...), "\n"), 1)
			a.nav(secLibrary)
		} else if len(res.Refused) > 0 {
			a.toast(T("library.import.none"), strings.Join(res.Refused, "\n"), 2)
		}
		if len(res.Skipped) > 0 && !replace {
			a.dlg.importSkip = importSkip{open: true, path: path, names: res.Skipped}
		}
	})
}

// importSkipSheet offers to replace apps that are already in the Library.
func (a *App) importSkipSheet(c *ui.Context) {
	d := &a.dlg.importSkip
	if !d.open {
		return
	}
	p := design.Of(c)
	kit.Sheet(c.Key("dlg-impskip"), &d.open, 460, func() {
		kit.SheetHeader(c, T("library.import.skip.title", "count", len(d.names)), nil)
		ui.Column(c).Gap(10).Children(func() {
			kit.SheetBody(c, T("library.import.skip.body"))
			names := d.names
			more := ""
			if len(names) > 6 {
				more = T("library.import.skip.more", "count", len(names)-6)
				names = names[:6]
			}
			ui.Text(c, strings.Join(names, ", ")+more).FontSize(design.FontBody).TextColor(p.Text).LineHeight(1.4)
		})
		keep, replace := false, false
		ui.Row(c).Gap(8).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
			keep = kit.Button(c.Key("keep"), T("library.import.keep"), kit.ButtonOpts{}).Clicked()
			replace = kit.Button(c.Key("replace"), T("library.import.replace"), kit.ButtonOpts{Variant: kit.Primary}).Clicked()
		})
		if keep {
			d.open = false
		}
		if replace {
			d.open = false
			a.importLibraryFile(d.path, true)
		}
	})
}

// ---- sync ----

func (a *App) syncOn() bool { return strings.TrimSpace(a.settings.SyncFolder) != "" }

// startLibrarySync syncs now and then every few minutes while it is on.
func (a *App) startLibrarySync() {
	go func() {
		if st, err := a.svc.SyncStatus(context.Background()); err == nil {
			a.sync.mu.Lock()
			a.sync.last = st
			a.sync.mu.Unlock()
		}
		time.Sleep(3 * time.Second) // settings load first
		for {
			a.syncNow(false)
			time.Sleep(syncEvery)
		}
	}()
}

// syncSoon syncs a few seconds after the Library changed.
func (a *App) syncSoon() {
	a.sync.mu.Lock()
	defer a.sync.mu.Unlock()
	if a.sync.pending != nil {
		a.sync.pending.Stop()
	}
	a.sync.pending = time.AfterFunc(4*time.Second, func() { a.syncNow(false) })
}

// syncNow runs a sync when it is on; manual reports a refusal too.
func (a *App) syncNow(manual bool) {
	if a.win == nil {
		return
	}
	ch := make(chan bool, 1)
	a.win.Update(func() { ch <- a.syncOn() }) // settings live on the UI thread
	on := <-ch
	if !on && !manual {
		return
	}
	a.sync.mu.Lock()
	if a.sync.busy {
		a.sync.mu.Unlock()
		return
	}
	a.sync.busy = true
	a.sync.mu.Unlock()
	a.win.Invalidate()
	res, err := a.svc.SyncLibrary(context.Background())
	a.sync.mu.Lock()
	a.sync.busy = false
	if err != nil {
		a.sync.err = friendly(err)
	} else {
		a.sync.err = ""
		a.sync.last = &res
		if len(res.Errors) > 0 {
			a.sync.err = res.Errors[0]
		}
	}
	a.sync.mu.Unlock()
	a.win.Invalidate()
}

// syncGroup is Settings' Sync section.
func (a *App) syncGroup(c *ui.Context) {
	p := design.Of(c)
	kit.Group(c, T("settings.sync"), func() {
		on := strings.TrimSpace(a.settings.SyncFolder) != ""
		kit.SettingIconRow(c.Key("sync"), icons.Refresh, ui.Color{}, T("sync.library"), T("sync.library.desc"), func() {
			v := on
			kit.Switch(c.Key("sw"), &v, T("sync.library"))
			if v != on {
				if v {
					a.settings.SyncFolder = appservice.DefaultSyncFolder()
				} else {
					a.settings.SyncFolder = ""
				}
				a.saveSettings()
				if v {
					go func() { time.Sleep(500 * time.Millisecond); a.syncNow(true) }()
				}
			}
		})
		if !on {
			return
		}
		kit.SettingIconRow(c.Key("folder"), icons.Folder, ui.Color{}, T("sync.folder"), T("sync.folder.desc"), func() {
			ui.Text(c, syncFolderLabel(a.settings.SyncFolder)).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine().MaxWidth(240)
			if kit.Button(c.Key("ch"), T("common.choose"), kit.ButtonOpts{Size: design.ControlSM}).Clicked() {
				go func() {
					paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Directory: true, CreateDirectories: true, DefaultPath: a.settings.SyncFolder})
					if err == nil && len(paths) > 0 {
						a.win.Update(func() { a.settings.SyncFolder = paths[0]; a.saveSettings(); go a.syncNow(true) })
					}
				}()
			}
		})
		a.sync.mu.Lock()
		last, serr, busy := a.sync.last, a.sync.err, a.sync.busy
		a.sync.mu.Unlock()
		status := T("sync.never")
		if last != nil && !last.At.IsZero() {
			status = T("sync.last", "when", ago(last.At), "count", last.Apps)
		}
		kit.SettingIconRow(c.Key("status"), icons.Clock, ui.Color{}, T("sync.status"), status, func() {
			if kit.Button(c.Key("now"), T("sync.now"), kit.ButtonOpts{Size: design.ControlSM, Loading: busy}).Clicked() {
				go a.syncNow(true)
			}
		})
		if serr != "" {
			ui.Column(c).Padding(0, 14, 12, 14).Children(func() {
				kit.Note(c, icons.Warning, serr, kit.ToneWarning)
			})
		}
	})
}

// syncFolderLabel names the sync folder as Finder does ("iCloud Drive ›
// Mypack"), else as a short path.
func syncFolderLabel(path string) string {
	const icloud = "/Library/Mobile Documents/com~apple~CloudDocs"
	if i := strings.Index(path, icloud); i >= 0 {
		rest := strings.Trim(path[i+len(icloud):], "/")
		if rest == "" {
			return "iCloud Drive"
		}
		return "iCloud Drive › " + strings.ReplaceAll(rest, "/", " › ")
	}
	return shortPath(path)
}
