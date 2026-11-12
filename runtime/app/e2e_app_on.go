//go:build e2e

package app

import (
	"fmt"
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
			case "panel": // a click on the menu-bar icon
				mygo.RunOnMain(a.togglePanel)
				time.Sleep(700 * time.Millisecond)
				out = a.e2eInfo()
			case "compact": // View > Compact Window
				mygo.RunOnMain(a.toggleCompact)
				time.Sleep(700 * time.Millisecond)
				out = a.e2eInfo()
			case "eval":
				if _, p := a.pageFor(nil); p != nil {
					v, err := p.Eval(arg)
					out = fmt.Sprintf("%v %v", v, err)
				}
			case "layout": // View > Window / Compact Window / Sidebar
				mygo.RunOnMain(func() { a.pickLayout(arg) })
				time.Sleep(900 * time.Millisecond)
				out = a.e2eInfo()
			case "flip": // View > Move Sidebar to …
				mygo.RunOnMain(a.flipSidebarEdge)
				time.Sleep(700 * time.Millisecond)
				out = a.e2eInfo()
			case "resize": // the user drags the sidebar's inner edge: "resize <w>"
				var nw int
				fmt.Sscan(arg, &nw)
				mygo.RunOnMain(func() {
					b := a.main.Bounds()
					a.main.SetBounds(mygo.Rectangle{X: b.X + b.Width - nw, Y: b.Y, Width: nw, Height: b.Height})
				})
				time.Sleep(900 * time.Millisecond)
				out = a.e2eInfo()
			case "move": // the user drags the window away
				mygo.RunOnMain(func() { b := a.main.Bounds(); a.main.SetPosition(b.X-300, b.Y+100) })
				time.Sleep(900 * time.Millisecond)
				out = a.e2eInfo()
			case "member": // a click on a rail icon: "member <index>"
				var i int
				fmt.Sscan(arg, &i)
				mygo.RunOnMain(func() { a.group.show(i, true) })
				time.Sleep(600 * time.Millisecond)
				out = a.e2eGroupInfo()
			case "split": // View > Toggle Split View
				mygo.RunOnMain(a.group.toggleSplit)
				time.Sleep(600 * time.Millisecond)
				out = a.e2eGroupInfo()
			case "geval": // "geval <index> <js>" in a member's page
				var i int
				idx, js, _ := strings.Cut(arg, " ")
				fmt.Sscan(idx, &i)
				v, err := a.group.members[i].page.Eval(js)
				out = fmt.Sprintf("%v %v", v, err)
			case "gnav": // "gnav <index> <url>": a user clicks a link in member <index>
				var i int
				idx, u, _ := strings.Cut(arg, " ")
				fmt.Sscan(idx, &i)
				e := &mygo.NavigateEvent{URL: u, UserInitiated: true}
				a.group.onNavigate(a.group.members[i], e)
				time.Sleep(900 * time.Millisecond)
				out = a.e2eGroupInfo()
			case "ginfo":
				out = a.e2eGroupInfo()
			case "info":
				out = a.e2eInfo()
			case "tinfo":
				out = a.e2eTabsInfo()
			case "topen": // a target=_blank link clicked in the shown tab
				t := a.tabs
				t.mu.Lock()
				from := t.list[t.active]
				t.mu.Unlock()
				t.onOpen(from, mygo.WindowOpenRequest{URL: arg, UserInitiated: true})
				time.Sleep(1200 * time.Millisecond)
				out = a.e2eTabsInfo()
			case "tpopup": // window.open with a size (a sign-in popup)
				opts := a.tabs.onOpen(nil, mygo.WindowOpenRequest{URL: arg, UserInitiated: true, Width: 500, Height: 600})
				out = fmt.Sprintf("window=%v %s", opts != nil, a.e2eTabsInfo())
			case "tnew":
				mygo.RunOnMain(a.tabs.newTab)
				time.Sleep(800 * time.Millisecond)
				out = a.e2eTabsInfo()
			case "tclose": // ⌘W
				mygo.RunOnMain(func() { a.tabs.close(a.tabs.active, true) })
				time.Sleep(500 * time.Millisecond)
				out = a.e2eTabsInfo()
			case "treopen":
				mygo.RunOnMain(a.tabs.reopen)
				time.Sleep(1000 * time.Millisecond)
				out = a.e2eTabsInfo()
			case "tpin":
				var i int
				fmt.Sscan(arg, &i)
				mygo.RunOnMain(func() { a.tabs.togglePin(i) })
				time.Sleep(300 * time.Millisecond)
				out = a.e2eTabsInfo()
			case "tshow":
				var i int
				fmt.Sscan(arg, &i)
				mygo.RunOnMain(func() { a.tabs.show(i, true) })
				time.Sleep(800 * time.Millisecond)
				out = a.e2eTabsInfo()
			case "shot":
				a.mu.Lock()
				w := a.main
				a.mu.Unlock()
				if w != nil {
					if png, err := w.CapturePage(); err == nil {
						_ = os.WriteFile(filepath.Join(dir, arg+".png"), png, 0o644)
					} else {
						out = "shot: " + err.Error()
					}
				}
			}
			_ = os.WriteFile(filepath.Join(dir, "appcmd.out"), []byte(out+"\n"), 0o644)
		}
	}()
}

func (a *App) e2eInfo() string {
	a.mu.Lock()
	w, tray := a.main, a.tray != nil
	compact := a.lay.cur
	a.mu.Unlock()
	if w == nil {
		return "main=nil"
	}
	b := w.Bounds()
	return fmt.Sprintf("visible=%v flyout=%v layout=%v tray=%v onTop=%v bounds=%d,%d,%dx%d policy=%s",
		w.IsVisible(), w.IsFlyout(), compact, tray, w.IsAlwaysOnTop(), b.X, b.Y, b.Width, b.Height, mygo.App.ActivationPolicy())
}

func (a *App) e2eGroupInfo() string {
	g := a.group
	g.mu.Lock()
	active, split := g.active, g.split
	g.mu.Unlock()
	out := fmt.Sprintf("active=%d split=%d dock=%q", active, split, a.badges.compute2())
	for _, m := range g.members {
		u := ""
		if m.page != nil {
			u = m.page.URL()
		}
		out += fmt.Sprintf(" | %s badge=%q url=%s", m.cfg.ID, a.badges.pageLabel(m.page), u)
	}
	return out
}

// compute2 is the merged badge.
func (c *badgeController) compute2() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.compute()
}

func (a *App) e2eTabsInfo() string {
	t := a.tabs
	if t == nil {
		return "tabs=nil"
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := fmt.Sprintf("active=%d n=%d closed=%d", t.active, len(t.list), len(t.closed))
	for _, tb := range t.list {
		out += fmt.Sprintf(" | pin=%v loaded=%v icon=%v title=%q url=%s", tb.pinned, tb.view != nil, tb.icon != nil, tb.title, tb.url)
	}
	return out
}
