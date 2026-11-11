//go:build e2e

package app

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// Test builds only: with MYPACK_E2E_DIR set, <dir>/appcmd is polled for one
// command per file write, standing in for gestures the automation CLI cannot
// make inside a page ("navigate <url>" = a user clicking a link,
// "incoming <url>" = another app handing a URL over, "open-link <url>").
// Results land in <dir>/appcmd.out.
func (a *App) e2eAppHook() {
	dir := os.Getenv("MYPACK_E2E_DIR")
	if dir == "" {
		return
	}
	go func() {
		cmdf := filepath.Join(dir, "appcmd")
		for {
			time.Sleep(300 * time.Millisecond)
			b, err := os.ReadFile(cmdf)
			if err != nil || len(b) == 0 {
				continue
			}
			_ = os.Remove(cmdf)
			verb, arg, _ := strings.Cut(strings.TrimSpace(string(b)), " ")
			out := "ok"
			switch verb {
			case "navigate":
				e := &mygo.NavigateEvent{URL: arg, UserInitiated: true}
				a.onWillNavigate(e)
				out = "navigate handled"
			case "incoming":
				a.handleIncomingURL(arg)
			case "open-link":
				a.openLink(arg)
			}
			_ = os.WriteFile(filepath.Join(dir, "appcmd.out"), []byte(out+"\n"), 0o644)
		}
	}()
}
