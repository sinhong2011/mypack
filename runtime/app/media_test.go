package app

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

type fakeNP struct {
	sets   []mygo.NowPlayingInfo
	clears int
}

func (f *fakeNP) Set(i mygo.NowPlayingInfo) { f.sets = append(f.sets, i) }
func (f *fakeNP) Clear()                    { f.clears++ }
func (f *fakeNP) last() mygo.NowPlayingInfo { return f.sets[len(f.sets)-1] }

type sent struct {
	p      *mygo.Page
	action string
	secs   float64
}

func newTestMedia() (*mediaController, *fakeNP, *[]sent) {
	np := &fakeNP{}
	c := newMediaController(np)
	c.async = func(f func()) { f() }
	var out []sent
	c.send = func(p *mygo.Page, a string, s float64) { out = append(out, sent{p, a, s}) }
	return c, np, &out
}

func TestMediaShowsOwnerAndCommands(t *testing.T) {
	c, np, out := newTestMedia()
	c.fetch = func(u string) []byte { return []byte("img:" + u) }
	p1, p2 := new(mygo.Page), new(mygo.Page)

	c.report(p1, MediaReport{State: "playing", Title: "Song", Artist: "Band", Artwork: "https://x/a.jpg",
		Duration: 200, Position: 10, Actions: []string{"play", "pause", "nexttrack", "previoustrack", "seekto"}})
	got := np.last()
	if got.Title != "Song" || got.Artist != "Band" || got.State != mygo.PlaybackPlaying || got.Duration != 200*time.Second || got.Position != 10*time.Second {
		t.Fatalf("info = %+v", got)
	}
	if string(got.Artwork) != "img:https://x/a.jpg" {
		t.Fatalf("artwork = %q", got.Artwork)
	}
	want := mygo.MediaPlay | mygo.MediaPause | mygo.MediaNext | mygo.MediaPrevious | mygo.MediaSeek
	if got.Commands != want {
		t.Fatalf("commands = %b, want %b", got.Commands, want)
	}

	c.command(mygo.MediaNext, 0)
	c.command(mygo.MediaSeek, 42*time.Second)
	if len(*out) != 2 || (*out)[0] != (sent{p1, "nexttrack", 0}) || (*out)[1] != (sent{p1, "seekto", 42}) {
		t.Fatalf("sent = %+v", *out)
	}

	// A paused page elsewhere does not take the keys; a playing one does.
	c.report(p2, MediaReport{State: "paused", Title: "Other"})
	if np.last().Title != "Song" {
		t.Fatalf("paused page took the keys: %+v", np.last())
	}
	c.report(p2, MediaReport{State: "playing", Title: "Other"})
	if np.last().Title != "Other" {
		t.Fatalf("playing page did not take the keys: %+v", np.last())
	}
	c.command(mygo.MediaPause, 0)
	if (*out)[2].p != p2 {
		t.Fatal("command went to the old owner")
	}

	// The owner closes: the other page still playing gets the keys back.
	c.drop(p2)
	if np.last().Title != "Song" {
		t.Fatalf("after drop: %+v", np.last())
	}
	c.report(p1, MediaReport{State: "none"})
	if np.clears != 1 {
		t.Fatalf("clears = %d", np.clears)
	}
	c.command(mygo.MediaPlay, 0)
	if len(*out) != 3 {
		t.Fatal("a command reached a page after Clear")
	}
}

func TestMediaPausedOwnerKeepsKeys(t *testing.T) {
	c, np, _ := newTestMedia()
	p := new(mygo.Page)
	c.report(p, MediaReport{State: "playing", Title: "A"})
	c.report(p, MediaReport{State: "paused", Title: "A"})
	if np.last().State != mygo.PlaybackPaused || np.clears != 0 {
		t.Fatalf("pausing should keep the controls: %+v clears=%d", np.last(), np.clears)
	}
	// No seek without a duration, and bad numbers are ignored.
	c.report(p, MediaReport{State: "playing", Title: "Live", Duration: 0, Rate: -3, Position: -1, Actions: []string{"seekto"}})
	got := np.last()
	if got.Commands&mygo.MediaSeek != 0 || got.Rate != 1 || got.Position != 0 {
		t.Fatalf("live stream info = %+v", got)
	}
}

func TestBridgeSetMediaChecksApp(t *testing.T) {
	c, np, _ := newTestMedia()
	page := new(mygo.Page)
	url := "https://evil.example.org/"
	b := &bridgeService{
		media:  c,
		caller: func(context.Context) (*mygo.Page, string) { return page, url },
		isApp:  func(raw string) bool { return raw == "https://music.example.com/" },
	}
	b.SetMedia(context.Background(), MediaReport{State: "playing", Title: "x"})
	if len(np.sets) != 0 {
		t.Fatal("a page outside the app showed its media")
	}
	url = "https://music.example.com/"
	b.SetMedia(context.Background(), MediaReport{State: "playing", Title: "x"})
	if len(np.sets) != 1 {
		t.Fatal("the app's page was ignored")
	}
	(&bridgeService{}).SetMedia(context.Background(), MediaReport{}) // no controller: no panic
}

func TestArtworkImage(t *testing.T) {
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	out, err := artworkImage(buf.Bytes())
	if err != nil || !bytes.Equal(out, buf.Bytes()) {
		t.Fatalf("png should pass through: %v", err)
	}
	if _, err := artworkImage([]byte("<html>")); err == nil {
		t.Fatal("non-image accepted")
	}
	if fetchArtwork("file:///etc/passwd") != nil || fetchArtwork("javascript:1") != nil {
		t.Fatal("non-http artwork fetched")
	}
}
