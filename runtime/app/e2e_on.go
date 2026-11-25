//go:build e2e && !mypack_lite

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Test builds only: with MYPACK_E2E_DIR set, the shell polls <dir>/cmd for
// one command per file write ("shot <name>", "eval <js>", "nav <url>") and
// writes <dir>/<name>-win.png (native strip), <name>-page.png (web view) and
// <dir>/out.txt (eval results, tint). The automation CLI cannot reach web
// views inside native UI, hence this hook.
func (s *shell) e2eHook() {
	dir := os.Getenv("MYPACK_E2E_DIR")
	if dir == "" {
		return
	}
	go func() {
		cmdf := filepath.Join(dir, "cmd")
		for {
			select {
			case <-s.stop:
				return
			case <-time.After(300 * time.Millisecond):
			}
			b, err := os.ReadFile(cmdf)
			if err != nil || len(b) == 0 {
				continue
			}
			_ = os.Remove(cmdf)
			line := strings.TrimSpace(string(b))
			verb, arg, _ := strings.Cut(line, " ")
			p := s.page.Load()
			out := ""
			switch verb {
			case "shot":
				if png, err := s.win.CapturePage(); err == nil {
					_ = os.WriteFile(filepath.Join(dir, arg+"-win.png"), png, 0o644)
				}
				if v := s.view.Load(); v != nil {
					if png, err := v.CapturePage(); err == nil {
						_ = os.WriteFile(filepath.Join(dir, arg+"-page.png"), png, 0o644)
					}
				}
				s.mu.Lock()
				out = "tint=" + s.lastRGB
				s.mu.Unlock()
			case "eval":
				v, err := p.Eval(arg)
				out = fmt.Sprintf("%v %v", v, err)
			case "nav":
				_ = p.LoadURL(arg)
				out = "nav " + arg
			case "glance":
				// The window's glance: "<phase> <url>", after arg ms; arg
				// "close" or "expand" presses that button first.
				switch arg {
				case "close", "expand":
					act := arg
					s.win.Update(func() {
						if s.gl != nil && act == "close" {
							s.gl.close()
						} else if s.gl != nil {
							s.gl.expandOrOpen()
						}
					})
				default:
					if ms, err := time.ParseDuration(arg + "ms"); err == nil {
						time.Sleep(ms)
					}
				}
				done := make(chan string, 1)
				s.win.Update(func() {
					if s.gl == nil {
						done <- "off"
						return
					}
					gp := s.gl.page
					ww, wh := s.win.Size()
					cw, ch := s.win.ContentSize()
					info := fmt.Sprintf("phase=%d url=%s cover=%v area=%+v card=%+v win=%dx%d content=%dx%d", s.gl.phase, s.gl.url, s.cover != nil, s.gl.area, s.gl.card, ww, wh, cw, ch)
					if gp != nil {
						go func() {
							v, _ := gp.Eval("innerWidth+'x'+innerHeight+' scrollY='+scrollY+' vv='+(visualViewport&&visualViewport.offsetTop)+'/'+(visualViewport&&visualViewport.height)+' doc='+document.documentElement.clientHeight")
							done <- fmt.Sprintf("%s glanceViewport=%v", info, v)
						}()
						return
					}
					done <- info
				})
				out = <-done
			case "tint":
				v, err := p.Eval(sampleJS)
				out = fmt.Sprintf("sample=%v %v", v, err)
			}
			_ = os.WriteFile(filepath.Join(dir, "out.txt"), []byte(out+"\n"), 0o644)
		}
	}()
}
