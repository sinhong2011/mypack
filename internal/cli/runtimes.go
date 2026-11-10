package cli

import (
	"context"
	"fmt"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

func (a *app) cmdRuntime(args []string) int {
	if len(args) == 0 {
		return a.usageErr(a.globalJSON, "usage: mypack runtime list|download|verify|clean")
	}
	sub, rest := args[0], args[1:]
	var asJSON bool
	fs := a.flagSet("runtime "+sub, &asJSON)
	version := fs.String("version", config.DefaultRuntimeVersion, "runtime version")
	all := fs.Bool("all", false, "clean: also remove runtimes pinned by projects")
	pos, err := parse(fs, rest)
	if err != nil {
		return a.usageErr(asJSON, "%v", err)
	}
	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	ctx, stop := signalContext()
	defer stop()

	switch sub {
	case "list":
		rs, err := svc.ListRuntimes(ctx)
		if err != nil {
			return a.fail(err, asJSON, nil)
		}
		if asJSON {
			a.okJSON(map[string]any{"runtimes": rs})
			return ExitOK
		}
		rows := make([][]string, 0, len(rs))
		for _, r := range rs {
			state := "not downloaded"
			if r.Cached && r.Verified {
				state = "ready"
			} else if r.Cached {
				state = "unverified"
			}
			rows = append(rows, []string{r.Version, r.Key, state, mb(r.Size)})
		}
		a.table([]string{"VERSION", "KEY", "STATE", "DOWNLOAD"}, rows)
		return ExitOK

	case "download":
		key := hostKey(svc.HostTarget())
		if len(pos) > 1 {
			return a.usageErr(asJSON, "usage: mypack runtime download [key] [--version V]")
		}
		if len(pos) == 1 {
			key = pos[0]
		}
		var last int
		info, err := svc.DownloadRuntime(ctx, *version, key, func(done, total int64) {
			if asJSON || total <= 0 {
				return
			}
			if pct := int(done * 100 / total); pct/10 != last/10 {
				last = pct
				fmt.Fprintf(a.errw, "downloading %s %s: %d%%\n", *version, key, pct)
			}
		})
		if err != nil {
			return a.fail(err, asJSON, nil)
		}
		if asJSON {
			a.okJSON(map[string]any{"runtime": info})
		} else {
			fmt.Fprintf(a.out, "%s %s ready\n", info.Version, info.Key)
		}
		return ExitOK

	case "verify":
		targets, err := a.cachedRuntimes(ctx, svc, *version, pos)
		if err != nil {
			return a.fail(err, asJSON, nil)
		}
		var results []map[string]any
		failed := 0
		var firstErr error
		for _, r := range targets {
			verr := svc.VerifyRuntime(ctx, r.Version, r.Key)
			res := map[string]any{"version": r.Version, "key": r.Key, "verified": verr == nil}
			if verr != nil {
				failed++
				if firstErr == nil {
					firstErr = verr
				}
				res["error"] = errorObject(toDomainError(verr))
			}
			results = append(results, res)
			if !asJSON {
				if verr != nil {
					fmt.Fprintf(a.errw, "FAILED %s %s: %v\n", r.Version, r.Key, verr)
				} else {
					fmt.Fprintf(a.out, "ok %s %s\n", r.Version, r.Key)
				}
			}
		}
		if failed > 0 {
			if asJSON {
				a.writeJSON(map[string]any{"ok": false, "results": results, "error": errorObject(toDomainError(firstErr))})
			}
			return ExitRuntime
		}
		if asJSON {
			a.okJSON(map[string]any{"results": results})
		}
		return ExitOK

	case "clean":
		rs, err := svc.ListRuntimes(ctx)
		if err != nil {
			return a.fail(err, asJSON, nil)
		}
		pinned := map[string]bool{}
		if !*all {
			projects, perr := svc.ListProjects(ctx)
			if perr != nil {
				return a.fail(perr, asJSON, nil)
			}
			for _, p := range projects {
				for _, t := range p.Config.Build.Targets {
					pinned[p.Config.Runtime.Version+"/"+t.Key()] = true
				}
			}
		}
		removed := []map[string]string{}
		for _, r := range rs {
			if !r.Cached || pinned[r.Version+"/"+r.Key] {
				continue
			}
			if err := svc.DeleteRuntime(ctx, r.Version, r.Key); err != nil {
				return a.fail(err, asJSON, nil)
			}
			removed = append(removed, map[string]string{"version": r.Version, "key": r.Key})
			if !asJSON {
				fmt.Fprintf(a.out, "removed %s %s\n", r.Version, r.Key)
			}
		}
		if asJSON {
			a.okJSON(map[string]any{"removed": removed})
		} else if len(removed) == 0 {
			fmt.Fprintln(a.out, "nothing to remove")
		}
		return ExitOK
	}
	return a.usageErr(asJSON, "unknown runtime command %q", sub)
}

// cachedRuntimes selects what `runtime verify` checks: the given key, or
// every cached runtime.
func (a *app) cachedRuntimes(ctx context.Context, svc interface {
	ListRuntimes(context.Context) ([]domain.RuntimeInfo, error)
}, version string, keys []string) ([]domain.RuntimeInfo, error) {
	if len(keys) > 0 {
		out := make([]domain.RuntimeInfo, 0, len(keys))
		for _, k := range keys {
			out = append(out, domain.RuntimeInfo{Version: version, Key: k})
		}
		return out, nil
	}
	rs, err := svc.ListRuntimes(ctx)
	if err != nil {
		return nil, err
	}
	var out []domain.RuntimeInfo
	for _, r := range rs {
		if r.Cached {
			out = append(out, r)
		}
	}
	return out, nil
}

func hostKey(t config.Target) string { return t.OS + "-" + t.Arch }

func mb(n int64) string {
	if n <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}
