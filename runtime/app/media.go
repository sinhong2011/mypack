package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo"
	_ "golang.org/x/image/webp" // artwork of many sites is WebP
)

// MediaReport is what the media shim (runtime/inject/media.go) reports of
// its page.
type MediaReport struct {
	State    string   `json:"state"` // playing | paused | none
	Title    string   `json:"title"`
	Artist   string   `json:"artist"`
	Album    string   `json:"album"`
	Artwork  string   `json:"artwork"`  // absolute URL, or ""
	Duration float64  `json:"duration"` // seconds; 0 is unknown
	Position float64  `json:"position"` // seconds
	Rate     float64  `json:"rate"`
	Actions  []string `json:"actions"`
}

// mediaKeysSupported: WebView2 already hands the media keys and the Windows
// media controls to the page, so Mypack stays out of the way there.
func mediaKeysSupported() bool { return runtime.GOOS != "windows" }

// nowPlaying is the part of mygo.NowPlaying the controller uses.
type nowPlaying interface {
	Set(mygo.NowPlayingInfo)
	Clear()
}

// mediaController shows the media of one page, its owner, where the system
// shows media, and sends the media keys' commands back to that page. The
// owner is the page that played last: another page that starts playing
// takes the keys, and when the owner stops reporting, a page still playing
// gets them.
type mediaController struct {
	np    nowPlaying
	send  func(p *mygo.Page, action string, seconds float64)
	fetch func(rawURL string) []byte // artwork, PNG or JPEG; nil on failure
	// async runs a fetch (tests run it inline).
	async func(func())

	mu      sync.Mutex
	pages   map[*mygo.Page]MediaReport
	owner   *mygo.Page
	shown   bool
	artURL  string // the artwork URL artData is of
	artData []byte
	artBusy string // the URL being fetched
}

func newMediaController(np nowPlaying) *mediaController {
	return &mediaController{np: np, pages: map[*mygo.Page]MediaReport{}, async: func(f func()) { go f() }}
}

// report records what page p plays.
func (c *mediaController) report(p *mygo.Page, r MediaReport) {
	c.mu.Lock()
	if r.State != "playing" && r.State != "paused" {
		delete(c.pages, p)
		if c.owner == p {
			c.owner = c.nextOwnerLocked()
		}
	} else {
		c.pages[p] = r
		if r.State == "playing" || c.owner == nil {
			c.owner = p
		}
	}
	fetch := c.applyLocked()
	c.mu.Unlock()
	if fetch != nil {
		c.async(fetch)
	}
}

// drop forgets a closed page.
func (c *mediaController) drop(p *mygo.Page) { c.report(p, MediaReport{}) }

// nextOwnerLocked picks a page still playing, else none: a paused page
// in a background tab should not grab the keys.
func (c *mediaController) nextOwnerLocked() *mygo.Page {
	for p, r := range c.pages {
		if r.State == "playing" {
			return p
		}
	}
	return nil
}

// applyLocked shows the owner's media, and returns the artwork fetch to
// start, if any, once c.mu is released.
func (c *mediaController) applyLocked() (fetch func()) {
	r, ok := c.pages[c.owner]
	if c.owner == nil || !ok {
		if c.shown {
			c.shown = false
			c.np.Clear()
		}
		return nil
	}
	c.shown = true
	c.np.Set(c.infoLocked(r))
	if r.Artwork != c.artURL && r.Artwork != c.artBusy && c.fetch != nil {
		u := r.Artwork
		c.artBusy = u
		return func() {
			data := []byte(nil)
			if u != "" {
				data = c.fetch(u)
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.artBusy == u {
				c.artBusy = ""
			}
			c.artURL, c.artData = u, data
			if cur, ok := c.pages[c.owner]; ok && cur.Artwork == u && len(data) > 0 {
				c.np.Set(c.infoLocked(cur))
			}
		}
	}
	return nil
}

func (c *mediaController) infoLocked(r MediaReport) mygo.NowPlayingInfo {
	info := mygo.NowPlayingInfo{
		Title: cleanText(r.Title, maxTitle), Artist: cleanText(r.Artist, maxTitle), Album: cleanText(r.Album, maxTitle),
		Duration: seconds(r.Duration), Position: seconds(r.Position), Rate: r.Rate,
		State:    mygo.PlaybackPaused,
		Commands: mygo.MediaPlay | mygo.MediaPause,
	}
	if r.State == "playing" {
		info.State = mygo.PlaybackPlaying
	}
	if info.Rate <= 0 || info.Rate > 16 || math.IsNaN(info.Rate) {
		info.Rate = 1
	}
	if r.Artwork != "" && r.Artwork == c.artURL {
		info.Artwork = c.artData
	}
	for _, a := range r.Actions {
		switch a {
		case "stop":
			info.Commands |= mygo.MediaStop
		case "nexttrack":
			info.Commands |= mygo.MediaNext
		case "previoustrack":
			info.Commands |= mygo.MediaPrevious
		case "seekto":
			if info.Duration > 0 {
				info.Commands |= mygo.MediaSeek
			}
		}
	}
	return info
}

// command sends a command of the media keys to the owner page.
func (c *mediaController) command(cmd mygo.MediaCommand, pos time.Duration) {
	action := map[mygo.MediaCommand]string{
		mygo.MediaPlay: "play", mygo.MediaPause: "pause", mygo.MediaStop: "stop",
		mygo.MediaNext: "nexttrack", mygo.MediaPrevious: "previoustrack", mygo.MediaSeek: "seekto",
	}[cmd]
	c.mu.Lock()
	p := c.owner
	c.mu.Unlock()
	if p == nil || action == "" || c.send == nil {
		return
	}
	c.send(p, action, pos.Seconds())
}

func seconds(s float64) time.Duration {
	if s <= 0 || math.IsNaN(s) || math.IsInf(s, 0) || s > 1e7 {
		return 0
	}
	return time.Duration(s * float64(time.Second))
}

// SetMedia is the media shim's report of what the calling page plays.
func (b *bridgeService) SetMedia(ctx context.Context, r MediaReport) {
	if b.media == nil {
		return
	}
	page, raw := b.caller(ctx)
	if page == nil {
		return
	}
	if r.State != "none" && !b.isApp(raw) {
		r = MediaReport{} // a page outside the app may not take the keys
	}
	b.media.report(page, r)
}

// ---- production wiring --------------------------------------------------------

const maxArtworkBytes = 4 << 20

var artworkClient = &http.Client{Timeout: 8 * time.Second}

// fetchArtwork downloads artwork and returns it as PNG or JPEG, else nil.
func fetchArtwork(raw string) []byte {
	if d := decodeDataImage(raw); d != nil {
		return d
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil
	}
	resp, err := artworkClient.Get(u.String())
	if err != nil {
		log.Printf("mypack-runtime: media: artwork: %v", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxArtworkBytes+1))
	if err != nil || len(data) > maxArtworkBytes {
		return nil
	}
	out, err := artworkImage(data)
	if err != nil {
		log.Printf("mypack-runtime: media: artwork %s: %v", u.Host, err)
		return nil
	}
	return out
}

// artworkImage keeps PNG and JPEG and converts other formats to PNG.
func artworkImage(data []byte) ([]byte, error) {
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	switch format {
	case "png", "jpeg":
		return data, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode %s as png: %w", format, err)
	}
	return buf.Bytes(), nil
}

// newMedia wires the media keys, or returns nil where the webview has them.
func (a *App) newMedia() *mediaController {
	if !mediaKeysSupported() {
		return nil
	}
	c := newMediaController(mygo.NowPlaying)
	c.fetch = fetchArtwork
	c.send = func(p *mygo.Page, action string, secs float64) {
		js, _ := jsonString(action)
		code := "window.__mypackMedia&&window.__mypackMedia.command(" + js + "," + strconv.FormatFloat(secs, 'f', 3, 64) + ")"
		if _, err := p.Eval(code); err != nil {
			log.Printf("mypack-runtime: media: command %s: %v", action, err)
		}
	}
	mygo.NowPlaying.OnCommand(c.command)
	return c
}
