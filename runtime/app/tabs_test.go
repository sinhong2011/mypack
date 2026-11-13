//go:build !mypack_lite

package app

import (
	"testing"

	"mypack.dev/mypack/pkg/config"
)

func testTabs(urls ...string) *tabSet {
	cfg := &config.Config{}
	cfg.WebView.Incognito = true // no state file
	t := &tabSet{a: &App{cfg: cfg}}
	for _, u := range urls {
		t.list = append(t.list, &tab{id: t.newID(), url: u})
	}
	return t
}

func TestTabFind(t *testing.T) {
	ts := testTabs("https://x.example/", "https://x.example/inbox#label/work", "https://x.example/settings")
	for raw, want := range map[string]int{
		"https://x.example":         0,
		"https://x.example/inbox":   1,
		"https://x.example/inbox#a": 1,
		"https://x.example/nope":    -1,
	} {
		if got := ts.find(raw); got != want {
			t.Errorf("find(%q) = %d, want %d", raw, got, want)
		}
	}
}

func TestTabPinOrder(t *testing.T) {
	ts := testTabs("a", "b", "c", "d")
	ts.active = 2 // "c"
	ts.togglePin(3)
	ts.togglePin(2) // "b", now third
	got := ""
	for _, tb := range ts.list {
		got += tb.url
		if tb.pinned {
			got += "*"
		}
	}
	if got != "d*b*ac" {
		t.Fatalf("order = %s", got)
	}
	if ts.list[ts.active].url != "c" {
		t.Fatalf("the shown tab moved: %s", ts.list[ts.active].url)
	}
	ts.togglePin(0) // unpin d: after the pinned ones
	if ts.list[0].url != "b" || ts.list[1].url != "d" || ts.list[1].pinned {
		t.Fatalf("unpin: %s %s", ts.list[0].url, ts.list[1].url)
	}
}
