package appservice

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/installer"
	"mypack.dev/mypack/pkg/config"
	"mypack.dev/mypack/runtime/routing"
)

// Link routing: generated apps read <UserConfigDir>/mypack-apps/routes.json
// (runtime/routing) to open a clicked link in the Mypack app that owns its
// host instead of the browser. The Builder is the only writer. The table is
// rewritten whenever installations change (install, update, repair,
// uninstall, relocate, forget, external changes) and when the service starts.

var routesMu sync.Mutex // one writer at a time, across services in a process

func (s *Impl) routesPath() string {
	switch s.opts.RoutesFile {
	case "-":
		return ""
	case "":
		if os.Getenv(routing.EnvFile) == "" && s.opts.DBPath != "" {
			// A custom database (tests, portable setups) is an isolated
			// world: never touch the user's real routes file from it.
			return filepath.Join(filepath.Dir(s.opts.DBPath), "routes.json")
		}
		return routing.FilePath()
	}
	return s.opts.RoutesFile
}

// scheduleRoutes refreshes the routes file in the background.
func (s *Impl) scheduleRoutes() {
	if s.routesPath() == "" {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		_ = s.SyncRoutes(s.ctx)
	}()
}

// RoutingEntries computes the routes of every installed app that was built
// from a website and is present on disk.
func (s *Impl) RoutingEntries(ctx context.Context) ([]routing.Entry, error) {
	all, err := s.store.ListInstallations(ctx)
	if err != nil {
		return nil, err
	}
	var out []routing.Entry
	for _, in := range all {
		if in.State == domain.InstallExternal || in.State == domain.InstallMissing || in.Path == "" {
			continue
		}
		if _, serr := os.Stat(in.Path); serr != nil {
			continue
		}
		cfg := s.installedConfig(ctx, in)
		if cfg == nil || cfg.App.Source.Type != config.SourceURL {
			continue
		}
		e := routing.Entry{Identifier: in.Identifier, BundlePath: in.Path}
		if e.Identifier == "" {
			e.Identifier = cfg.App.Identifier
		}
		if !cfg.Native.LinkRoutingEnabled() {
			continue // the app opted out of being linked to as well
		}
		var doms []string
		if u, uerr := url.Parse(cfg.App.Source.Value); uerr == nil && u.Hostname() != "" {
			e.SourceHost = u.Hostname()
		} else if h := config.RegistrableHost(cfg.App.Source.Value); h != "" {
			e.SourceHost = h
		}
		doms = append(doms, e.SourceHost)
		doms = append(doms, cfg.WebView.Navigation.SafeDomains...)
		if cfg.IsGroup() { // a group owns every member's site
			for _, m := range cfg.App.Group.Members {
				if h := config.RegistrableHost(m.URL); h != "" {
					doms = append(doms, h)
				}
			}
		}
		e.Hosts = routing.ExpandHosts(doms...)
		if runtime.GOOS != "darwin" {
			e.Executable = installer.Executable(in.Path)
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Identifier < out[j].Identifier })
	return out, nil
}

// installedConfig is the config the installed copy was built from (the build
// snapshot), falling back to the project's current config.
func (s *Impl) installedConfig(ctx context.Context, in domain.Installation) *config.Config {
	if in.ArtifactID != "" {
		if art, err := s.store.GetArtifact(ctx, in.ArtifactID); err == nil {
			if job, jerr := s.store.GetJob(ctx, art.JobID); jerr == nil && job.Snapshot != nil {
				return job.Snapshot
			}
		}
	}
	if in.ProjectID != "" {
		if p, err := s.store.GetProject(ctx, in.ProjectID); err == nil {
			return p.Config
		}
	}
	return nil
}

// SyncRoutes rewrites the routes file from the current installations.
func (s *Impl) SyncRoutes(ctx context.Context) error {
	path := s.routesPath()
	if path == "" {
		return nil
	}
	es, err := s.RoutingEntries(ctx)
	if err != nil {
		return err
	}
	routesMu.Lock()
	defer routesMu.Unlock()
	return routing.Write(path, es)
}
