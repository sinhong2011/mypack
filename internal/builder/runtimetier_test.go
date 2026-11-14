package builder

import (
	"context"
	"errors"
	"testing"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// fakeStatus answers Status from a table: missing keys are not in the registry.
type fakeStatus map[string]domain.RuntimeInfo

func (f fakeStatus) Status(_ context.Context, version, key string) (domain.RuntimeInfo, error) {
	if ri, ok := f[key]; ok {
		return ri, nil
	}
	return domain.RuntimeInfo{}, errors.New("not listed")
}

func TestRuntimeKey(t *testing.T) {
	host := config.Target{OS: "darwin", Arch: "arm64", Format: "app"}
	cfg := config.New("Docs", config.Source{Type: config.SourceURL, Value: "https://docs.example.com"}, host)
	cfg.ApplyDefaults(host)
	cached := domain.RuntimeInfo{Cached: true, Verified: true}
	listed := domain.RuntimeInfo{Size: 3}
	ctx := context.Background()

	cases := []struct {
		name string
		st   fakeStatus
		want string
	}{
		{"lite cached", fakeStatus{"darwin-arm64": cached, "darwin-arm64-lite": cached}, "darwin-arm64-lite"},
		{"only full cached: no new download", fakeStatus{"darwin-arm64": cached, "darwin-arm64-lite": listed}, "darwin-arm64"},
		{"nothing cached: smaller download", fakeStatus{"darwin-arm64": listed, "darwin-arm64-lite": listed}, "darwin-arm64-lite"},
		{"registry has no lite", fakeStatus{"darwin-arm64": listed}, "darwin-arm64"},
	}
	for _, c := range cases {
		if got := RuntimeKey(ctx, c.st, cfg, host); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}

	tabs := cfg.Clone()
	tabs.Window.Tabs = true
	if got := RuntimeKey(ctx, fakeStatus{"darwin-arm64-lite": cached}, tabs, host); got != "darwin-arm64" {
		t.Errorf("tabs need the full runtime, got %s", got)
	}
	old := cfg.Clone()
	old.Runtime.Version = "1.0.9"
	if got := RuntimeKey(ctx, fakeStatus{"darwin-arm64-lite": cached}, old, host); got != "darwin-arm64" {
		t.Errorf("1.0.9 has no lite tier, got %s", got)
	}
}
