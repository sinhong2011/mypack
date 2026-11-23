package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
)

// Preset icons: the built-in presets (Discover, the empty Library) show
// each site's real icon, resolved lazily through FetchMetadata (at most
// three at a time, cancelled when the screen goes), cached on disk for a
// week so they appear at once next time. Nothing third-party is bundled;
// offline, a preset shows its generated letter icon.

const presetTTL = 7 * 24 * time.Hour

type presetIcons struct {
	mu     sync.Mutex
	icons  map[string][]byte // host → PNG; empty = fall back to the letter icon
	busy   map[string]bool
	sem    chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
}

func presetCacheDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "mypack", "preset-icons")
}

func (pi *presetIcons) init() {
	if pi.icons == nil {
		pi.icons, pi.busy, pi.sem = map[string][]byte{}, map[string]bool{}, make(chan struct{}, 3)
	}
}

// cachedPresetIcon reads host's icon from the disk cache while it is fresh.
func cachedPresetIcon(dir, host string) []byte {
	if dir == "" {
		return nil
	}
	path := filepath.Join(dir, host+".png")
	if st, err := os.Stat(path); err != nil || time.Since(st.ModTime()) >= presetTTL {
		return nil
	}
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return b
	}
	return nil
}

// presetIcon returns the icon for preset t, or nil while it is being
// resolved (show a skeleton), or empty bytes for the letter fallback.
func (a *App) presetIcon(t template) []byte {
	pi := &a.presets
	host := hostOf(t.url)
	pi.mu.Lock()
	defer pi.mu.Unlock()
	pi.init()
	if b, ok := pi.icons[host]; ok {
		return b
	}
	if pi.busy[host] {
		return nil
	}
	if pi.ctx == nil || pi.ctx.Err() != nil {
		pi.ctx, pi.cancel = context.WithCancel(context.Background())
	}
	pi.busy[host] = true
	ctx, url := pi.ctx, t.url
	// Everything slow runs here, off the UI thread: the disk cache, the
	// network, and normalizing and decoding the icon (kit.PrepareIcon), so
	// opening Discover never stalls on its 60-odd tiles.
	go func() {
		done := func(icon []byte) {
			kit.PrepareIcon(icon)
			pi.mu.Lock()
			delete(pi.busy, host)
			pi.icons[host] = icon
			pi.mu.Unlock()
			if a.win != nil {
				a.win.Invalidate()
			}
		}
		dir := presetCacheDir()
		if b := cachedPresetIcon(dir, host); b != nil {
			done(b)
			return
		}
		select {
		case pi.sem <- struct{}{}:
		case <-ctx.Done():
			pi.mu.Lock()
			delete(pi.busy, host)
			pi.mu.Unlock()
			return
		}
		fctx, cancel := context.WithTimeout(ctx, 12*time.Second)
		m, err := a.svc.FetchMetadata(fctx, url)
		cancel()
		<-pi.sem
		if ctx.Err() != nil {
			pi.mu.Lock()
			delete(pi.busy, host)
			pi.mu.Unlock()
			return // left the screen: try again next time
		}
		icon := []byte{}
		if err == nil && !m.Fallback && len(m.IconPNG) > 0 {
			icon = m.IconPNG
			if dir != "" && os.MkdirAll(dir, 0o755) == nil {
				_ = os.WriteFile(filepath.Join(dir, host+".png"), icon, 0o644)
			}
		}
		done(icon)
	}()
	return nil
}

// stopPresetFetches cancels pending icon lookups (leaving the screen).
func (a *App) stopPresetFetches() {
	pi := &a.presets
	pi.mu.Lock()
	if pi.cancel != nil {
		pi.cancel()
	}
	pi.mu.Unlock()
}

// presetIconView shows a preset's icon: a shimmer while it resolves, then
// the icon fading in with a small pop.
func (a *App) presetIconView(c *ui.Context, t template, size float32, shadow bool) {
	icon := a.presetIcon(t)
	if icon == nil {
		kit.Shimmer(c.Key("sh"+t.url), size, size, design.AppIconRadius(size))
		return
	}
	var png []byte
	if len(icon) > 0 {
		png = icon
	}
	kit.AppIconOpts(c, png, t.name, size, kit.IconOpts{Shadow: shadow})
}
