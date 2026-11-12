package fakesvc

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/domain"
)

// Library export, import and sync for the demo: real files, an in-memory
// Library, and a sync that only pretends.

func (s *Service) ExportLibrary(ctx context.Context, ids []string, path string) (int, error) {
	s.mu.Lock()
	f := appservice.LibraryFile{Format: appservice.LibraryFormat, Version: appservice.LibraryVersion, ExportedAt: time.Now().UTC()}
	for _, p := range s.projects {
		if p.TrashedAt != nil {
			continue
		}
		if len(ids) > 0 && !contains(ids, p.ID) {
			continue
		}
		f.Apps = append(f.Apps, appservice.LibraryApp{Config: p.Config.Clone(), Icon: p.IconPNG, UpdatedAt: p.UpdatedAt})
	}
	s.mu.Unlock()
	if len(f.Apps) == 0 {
		return 0, &domain.Error{Code: appservice.CodeInvalidInput, Message: "There are no apps to export."}
	}
	b, _ := json.MarshalIndent(f, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return 0, err
	}
	return len(f.Apps), nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func (s *Service) ImportLibrary(ctx context.Context, path string, opts appservice.ImportOptions) (appservice.ImportResult, error) {
	var res appservice.ImportResult
	f, err := appservice.ReadLibraryFile(path)
	if err != nil {
		return res, &domain.Error{Code: appservice.CodeInvalidInput, Message: "That file could not be imported: " + err.Error() + "."}
	}
	for _, app := range f.Apps {
		if app.Deleted || app.Config == nil {
			continue
		}
		s.mu.Lock()
		var ex *domain.Project
		for _, p := range s.projects {
			if p.Config.App.Identifier == app.Config.App.Identifier {
				ex = p
			}
		}
		switch {
		case ex != nil && !opts.Replace:
			res.Skipped = append(res.Skipped, app.Config.App.Name)
		case ex != nil:
			ex.Config, ex.IconPNG, ex.TrashedAt = app.Config.Clone(), app.Icon, nil
			ex.Revision++
			ex.UpdatedAt = time.Now()
			res.Updated = append(res.Updated, ex.ID)
		default:
			p := &domain.Project{ID: s.id("prj"), Revision: 1, Config: app.Config.Clone(), IconPNG: app.Icon, Draft: true,
				Origin: "import", CreatedAt: time.Now(), UpdatedAt: time.Now()}
			s.projects[p.ID] = p
			res.Added = append(res.Added, p.ID)
		}
		s.mu.Unlock()
	}
	s.publish(appservice.Event{Kind: appservice.EventProjectChanged})
	return res, nil
}

func (s *Service) SyncLibrary(ctx context.Context) (appservice.SyncResult, error) {
	set, _ := s.Settings(ctx)
	if set.SyncFolder == "" {
		return appservice.SyncResult{}, &domain.Error{Code: appservice.CodeInvalidInput, Message: "Library sync is off."}
	}
	s.mu.Lock()
	n := 0
	for _, p := range s.projects {
		if p.TrashedAt == nil {
			n++
		}
	}
	res := appservice.SyncResult{Folder: set.SyncFolder, Apps: n, At: time.Now().UTC()}
	s.lastSync = &res
	s.mu.Unlock()
	return res, nil
}

func (s *Service) SyncStatus(ctx context.Context) (*appservice.SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSync, nil
}
