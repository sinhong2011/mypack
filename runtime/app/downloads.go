package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"mypack.dev/mypack/pkg/config"
)

// dlNotice is the in-app result of a download, shown until dismissed or
// it ages out. Lite windows have no native UI overlay, so they reveal the
// file instead.
type dlNotice struct {
	path string
	err  string
	at   time.Time
	id   int64
}

func (a *App) downloadDir() string {
	home, _ := os.UserHomeDir()
	osDl := ""
	if p, err := mygo.App.Path(mygo.PathDownloads); err == nil {
		osDl = p
	}
	return config.ResolveDownloadDir(a.cfg.Network.DownloadDirectory, home, osDl, os.TempDir(),
		func(d string) error { return os.MkdirAll(d, 0o755) })
}

// UniquePath returns p, or "name (2).ext" when p exists.
func UniquePath(p string) string {
	if _, err := os.Lstat(p); err != nil {
		return p
	}
	ext := filepath.Ext(p)
	base := p[:len(p)-len(ext)]
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Lstat(c); err != nil {
			return c
		}
	}
}

func (a *App) onWillDownload(e *mygo.DownloadEvent) {
	// Always set Path: an OnWillDownload listener that leaves Path empty
	// cancels the download (MyGo DownloadStarted).
	d := a.downloadDir()
	name := config.SafeDownloadName(e.SuggestedName)
	if d == "" {
		if e.Path != "" {
			return
		}
		log.Printf("mypack-runtime: download: no folder for %s", name)
		e.PreventDefault()
		go a.finishDownload(&mygo.Download{URL: e.URL, Err: errNoDownloadDir})
		return
	}
	e.Path = UniquePath(filepath.Join(d, name))
}

var errNoDownloadDir = &downloadError{msg: "no folder could be created for this download"}

type downloadError struct{ msg string }

func (e *downloadError) Error() string { return e.msg }

var notifySeq atomic.Int64
var noticeSeq atomic.Int64

func (a *App) onDownloadDone(d *mygo.Download) {
	go a.finishDownload(d)
}

func (a *App) finishDownload(d *mygo.Download) {
	if d == nil {
		return
	}
	if d.Err != nil {
		log.Printf("mypack-runtime: download failed: %v", d.Err)
	} else {
		log.Printf("mypack-runtime: downloaded %s", d.Path)
	}
	a.notifyDownload(d)
	if Lite {
		if d.Err != nil {
			mygo.Dialog.Error("Download failed", downloadBody(d))
			return
		}
		if d.Path != "" {
			mygo.Shell.ShowItemInFolder(d.Path)
		}
		return
	}
	a.queueDownloadNotice(d)
}

func (a *App) queueDownloadNotice(d *mygo.Download) {
	n := &dlNotice{path: d.Path, at: time.Now(), id: noticeSeq.Add(1)}
	if d.Err != nil {
		n.err = d.Err.Error()
	}
	a.mu.Lock()
	a.dlNotice = n
	a.mu.Unlock()
	a.invalidateWindows()
	id := n.id
	time.AfterFunc(8*time.Second, func() {
		a.mu.Lock()
		if a.dlNotice != nil && a.dlNotice.id == id {
			a.dlNotice = nil
			a.mu.Unlock()
			a.invalidateWindows()
			return
		}
		a.mu.Unlock()
	})
}

func (a *App) clearDownloadNotice(id int64) {
	a.mu.Lock()
	if a.dlNotice != nil && (id == 0 || a.dlNotice.id == id) {
		a.dlNotice = nil
	}
	a.mu.Unlock()
	a.invalidateWindows()
}

func (a *App) invalidateWindows() {
	mygo.RunOnMain(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.main != nil {
			a.main.Invalidate()
		}
		for w := range a.windows {
			w.Invalidate()
		}
	})
}

func (a *App) revealDownloads() {
	d := a.downloadDir()
	if d == "" {
		mygo.Dialog.Error("Downloads", "MyPack could not find a Downloads folder on this computer.")
		return
	}
	mygo.Shell.ShowItemInFolder(d)
}

func (a *App) notifyDownload(d *mygo.Download) {
	if !mygo.NotificationsSupported() {
		return
	}
	n := mygo.NewNotification(mygo.NotificationOptions{
		ID:    fmt.Sprintf("dl-%d-%d", time.Now().UnixNano(), notifySeq.Add(1)),
		Title: a.title(),
		Body:  downloadBody(d),
	})
	if d.Err == nil && d.Path != "" {
		path := d.Path
		n.OnClick(func() { go mygo.Shell.ShowItemInFolder(path); n.Close() })
	}
	_ = n.Show() // denied/unsupported notifications are not an error for the app
}

func downloadBody(d *mygo.Download) string {
	if d.Err != nil {
		return "Download failed: " + d.Err.Error()
	}
	return "Downloaded " + filepath.Base(d.Path)
}
