package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
)

func (a *App) downloadDir() string {
	d := strings.TrimSpace(a.cfg.Network.DownloadDirectory)
	if d == "" {
		return ""
	}
	if d == "~" || strings.HasPrefix(d, "~/") || strings.HasPrefix(d, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			d = filepath.Join(home, d[1:])
		}
	}
	if !filepath.IsAbs(d) {
		return ""
	}
	if os.MkdirAll(d, 0o755) != nil {
		return ""
	}
	return d
}

// UniquePath returns p, or "name (2).ext" when p exists.
func UniquePath(p string) string {
	if _, err := os.Lstat(p); err != nil {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Lstat(c); err != nil {
			return c
		}
	}
}

func (a *App) onWillDownload(e *mygo.DownloadEvent) {
	// Default (no custom directory): MyGo saves to ~/Downloads, numbered.
	if d := a.downloadDir(); d != "" {
		name := filepath.Base(e.SuggestedName)
		if name == "" || name == "." || name == string(filepath.Separator) {
			name = "download"
		}
		e.Path = UniquePath(filepath.Join(d, name))
	}
}

var notifySeq atomic.Int64

func (a *App) onDownloadDone(d *mygo.Download) {
	go func() {
		if !mygo.NotificationsSupported() {
			return
		}
		n := mygo.NewNotification(mygo.NotificationOptions{
			ID:    fmt.Sprintf("dl-%d-%d", time.Now().UnixNano(), notifySeq.Add(1)),
			Title: a.title(),
			Body:  downloadBody(d),
		})
		if d.Err == nil {
			path := d.Path
			n.OnClick(func() { go mygo.Shell.ShowItemInFolder(path); n.Close() })
		}
		_ = n.Show() // denied/unsupported notifications are not an error for the app
	}()
}

func downloadBody(d *mygo.Download) string {
	if d.Err != nil {
		return "Download failed: " + d.Err.Error()
	}
	return "Downloaded " + filepath.Base(d.Path)
}
