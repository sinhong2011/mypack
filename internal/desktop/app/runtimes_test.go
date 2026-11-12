package app

import (
	"testing"

	"mypack.dev/mypack/internal/domain"
)

func TestShownRuntimesHidesOldUndownloadedVersions(t *testing.T) {
	all := []domain.RuntimeInfo{
		{Version: "1.0.9", Key: "darwin-arm64", Cached: true},
		{Version: "1.0.9", Key: "linux-amd64"},
		{Version: "1.0.8", Key: "darwin-arm64", Cached: true},
		{Version: "1.0.8", Key: "linux-amd64"},
	}
	got := shownRuntimes(all, "1.0.9")
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(got), got)
	}
	for _, rt := range got {
		if rt.Version == "1.0.8" && !rt.Cached {
			t.Fatalf("an old runtime that is not on disk must be hidden: %+v", rt)
		}
	}
}
