package app

import (
	"strings"
	"testing"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
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

func TestRuntimeVersionOptionsListsEveryPublishedVersion(t *testing.T) {
	all := []domain.RuntimeInfo{
		{Version: "1.0.8", Key: "darwin-arm64"},
		{Version: "1.0.8", Key: "linux-amd64"},
		{Version: "1.0.10", Key: "darwin-arm64"},
		{Version: "1.0.9", Key: "linux-amd64", Cached: true},
		{Version: "not-a-version", Key: "darwin-arm64"},
	}
	got := runtimeVersionOptions(all, "1.0.6", config.DefaultRuntimeVersion)
	var vers []string
	for _, o := range got {
		vers = append(vers, o.Value)
		if o.Value == config.DefaultRuntimeVersion && !strings.Contains(o.Label, "Recommended") && !strings.Contains(o.Label, T("set.runtime.recommended")) {
			t.Fatalf("recommended %s should be labeled: %+v", config.DefaultRuntimeVersion, o)
		}
		if o.Value != config.DefaultRuntimeVersion && o.Label != o.Value {
			t.Fatalf("non-recommended %s should use the version as the label, got %q", o.Value, o.Label)
		}
	}
	want := []string{config.DefaultRuntimeVersion, "1.0.9", "1.0.8", "1.0.6"}
	if len(vers) != len(want) {
		t.Fatalf("got %v, want %v", vers, want)
	}
	for i, v := range want {
		if vers[i] != v {
			t.Fatalf("got %v, want %v", vers, want)
		}
	}
}

func TestRuntimeVersionOptionsEmptyRegistryStillOffersRecommended(t *testing.T) {
	got := runtimeVersionOptions(nil, "", config.DefaultRuntimeVersion)
	if len(got) != 1 || got[0].Value != config.DefaultRuntimeVersion {
		t.Fatalf("got %+v, want only %s", got, config.DefaultRuntimeVersion)
	}
}

func TestVersionNewer(t *testing.T) {
	if !versionNewer("1.0.10", "1.0.9") || versionNewer("1.0.9", "1.0.10") {
		t.Fatal("1.0.10 should sort before 1.0.9")
	}
	if !versionNewer("1.0.9", "1.0.8") {
		t.Fatal("1.0.9 should sort before 1.0.8")
	}
	if versionNewer("1.0.10", "1.0.10") {
		t.Fatal("equal versions are not newer")
	}
}
