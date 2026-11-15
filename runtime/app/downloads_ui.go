//go:build !mypack_lite

package app

import (
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// drawDownloadNotice is a pill over the page after a download ends: the
// file name, Show (Finder / Explorer / the file manager), and Dismiss.
func (a *App) drawDownloadNotice(c *ui.Context) {
	a.mu.Lock()
	n := a.dlNotice
	a.mu.Unlock()
	if n == nil {
		return
	}
	if time.Since(n.at) > 8*time.Second {
		a.clearDownloadNotice(n.id)
		return
	}
	t := c.Theme()
	failed := n.err != ""
	ui.Row(c.Key("dl-notice")).Absolute().Left(0).Right(0).Bottom(16).Justify(ui.Center).PassThrough().Children(func() {
		bar := ui.Row(c).Gap(10).Padding(10, 14).Radius(12).AlignItems(ui.Center).
			Background(t.Surface).Border(1, t.Border).MaxWidth(460)
		bar.Children(func() {
			ui.Column(c).Gap(2).MinWidth(0).Grow(1).Children(func() {
				title := "Downloaded " + filepath.Base(n.path)
				if failed {
					title = "Download failed"
				}
				col := t.Text
				if failed {
					col = t.Danger
				}
				ui.Text(c, title).Bold().TextColor(col).SingleLine()
				sub := n.path
				if failed {
					sub = n.err
				}
				if sub != "" {
					ui.Text(c, sub).FontSize(12).TextColor(t.TextMuted).SingleLine()
				}
			})
			if !failed && n.path != "" {
				path := n.path
				id := n.id
				if ui.PrimaryButton(c, "Show").Clicked() {
					go mygo.Shell.ShowItemInFolder(path)
					a.clearDownloadNotice(id)
				}
			}
			id := n.id
			if ui.Button(c, "Dismiss").Clicked() {
				a.clearDownloadNotice(id)
			}
		})
	})
}
