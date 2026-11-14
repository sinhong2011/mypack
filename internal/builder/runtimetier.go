package builder

import (
	"context"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/runtimemanager"
	"mypack.dev/mypack/pkg/config"
)

// RuntimeStatus is the part of the runtime manager RuntimeKey needs.
type RuntimeStatus interface {
	Status(ctx context.Context, version, key string) (domain.RuntimeInfo, error)
}

// RuntimeKey is the runtime cache key a build of cfg for target uses:
// target.Key() for the full runtime, or its "-lite" key when cfg runs on the
// Lite runtime (config.Config.RuntimeTier) and getting it costs no extra
// download. In order:
//
//   - the Lite runtime when it is already cached (an installer bundles it);
//   - else the full runtime when that is cached, so a build never asks for
//     a download it does not need;
//   - else the Lite runtime when the registry lists it (the smaller download);
//   - else the full runtime.
//
// The build preflight and the build itself both call it, so the download a
// build asks approval for is the one it makes.
func RuntimeKey(ctx context.Context, rt RuntimeStatus, cfg *config.Config, target config.Target) string {
	full := target.Key()
	if rt == nil || cfg.RuntimeTier() != config.TierLite {
		return full
	}
	version, lite := cfg.Runtime.Version, runtimemanager.LiteKey(full)
	if st, err := rt.Status(ctx, version, lite); err == nil && st.Cached {
		return lite
	}
	if st, err := rt.Status(ctx, version, full); err == nil && st.Cached {
		return full
	}
	if _, err := rt.Status(ctx, version, lite); err == nil {
		return lite
	}
	return full
}
