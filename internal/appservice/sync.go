package appservice

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"mypack.dev/mypack/internal/domain"
)

// Library sync (Pro): the Library is kept in a folder that a file-sync
// service shares between computers (iCloud Drive by default), one .mypack
// file per app under <folder>/apps/<identifier>.mypack. A sync compares
// each app here, its file there, and what both looked like at the last
// sync (a hash per app in the settings store): the side that changed wins;
// when both did, the later edit does. Moving an app to the Trash writes a
// tombstone file, which moves it to the Trash elsewhere. Nothing is ever
// deleted for good, built or installed by a sync.

const syncStateKey = "sync.state"

// SyncResult says what a sync did.
type SyncResult struct {
	Folder  string    `json:"folder"`
	Pushed  int       `json:"pushed"` // apps written to the folder
	Pulled  int       `json:"pulled"` // apps added or changed here
	Trashed int       `json:"trashed"`
	Apps    int       `json:"apps"` // apps in sync
	At      time.Time `json:"at"`
	Errors  []string  `json:"errors,omitempty"`
}

type syncState struct {
	Hashes map[string]string `json:"hashes"` // identifier -> hash at the last sync
	Last   *SyncResult       `json:"last,omitempty"`
}

var syncMu sync.Mutex // one sync at a time per process

// DefaultSyncFolder is iCloud Drive's mypack folder on macOS, else a folder
// in the home directory (point it at Dropbox, OneDrive, Syncthing…).
func DefaultSyncFolder() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		if icloud := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs"); dirExists(icloud) {
			return filepath.Join(icloud, "Mypack")
		}
	}
	return filepath.Join(home, "Mypack Sync")
}

// SyncLibrary implements Service.
func (s *Impl) SyncLibrary(ctx context.Context) (res SyncResult, err error) {
	syncMu.Lock()
	defer syncMu.Unlock()
	set, err := s.Settings(ctx)
	if err != nil {
		return res, err
	}
	if strings.TrimSpace(set.SyncFolder) == "" {
		return res, newErr(CodeInvalidInput, "Library sync is off.", "Turn on Sync in Settings and choose a folder.")
	}
	if st, _ := s.License(ctx); !st.Valid {
		return res, newErr(CodeLicenseLimit, "Library sync is part of mypack Pro.",
			"Enter a license key, or move apps between computers with File > Export Library.")
	}
	folder := expandHome(set.SyncFolder)
	dir := filepath.Join(folder, "apps")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, newErr(CodeInternal, "The sync folder could not be opened.", "Check that "+folder+" exists and is writable.")
	}
	res.Folder = folder

	var state syncState
	_, _ = s.store.GetSetting(ctx, syncStateKey, &state)
	if state.Hashes == nil {
		state.Hashes = map[string]string{}
	}

	local, err := s.projectsByIdentifier(ctx)
	if err != nil {
		return res, err
	}
	remote := map[string]LibraryApp{}
	files, _ := filepath.Glob(filepath.Join(dir, "*"+LibraryExt))
	for _, f := range files {
		lf, rerr := ReadLibraryFile(f)
		if rerr != nil || len(lf.Apps) != 1 || lf.Apps[0].id() == "" {
			continue // a half-synced or foreign file: try again next time
		}
		remote[lf.Apps[0].id()] = lf.Apps[0]
	}

	ids := map[string]bool{}
	for id := range local {
		ids[id] = true
	}
	for id := range remote {
		ids[id] = true
	}
	for id := range ids {
		p, haveL := local[id]
		r, haveR := remote[id]
		var l LibraryApp
		lh, rh := "", ""
		if haveL {
			l = s.libraryApp(p)
			if p.TrashedAt != nil {
				l = LibraryApp{Identifier: id, Deleted: true, UpdatedAt: l.UpdatedAt}
			}
			lh = libraryHash(l)
		}
		if haveR {
			rh = libraryHash(r)
		}
		last := state.Hashes[id]
		switch {
		case lh == rh:
			state.Hashes[id] = lh // in step
		case !haveL && last != "":
			// gone from this database without a trace: leave the folder alone
		case !haveR && l.Deleted:
			// trashed here and never synced: nothing to tell
		case !haveR || (lh != last && (rh == last || l.UpdatedAt.After(r.UpdatedAt))):
			if werr := writeLibraryFile(filepath.Join(dir, syncFileName(id)), LibraryFile{
				Format: LibraryFormat, Version: LibraryVersion, ExportedAt: time.Now().UTC().Truncate(time.Second), Apps: []LibraryApp{l},
			}); werr != nil {
				res.Errors = append(res.Errors, id+": "+werr.Error())
				continue
			}
			state.Hashes[id] = lh
			res.Pushed++
		case r.Deleted:
			if haveL && p.TrashedAt == nil {
				now := time.Now()
				if terr := s.store.SetTrashed(ctx, p.ID, &now); terr != nil {
					res.Errors = append(res.Errors, id+": "+terr.Error())
					continue
				}
				s.emit(Event{Kind: EventProjectDeleted, ProjectID: p.ID})
				res.Trashed++
			}
			state.Hashes[id] = rh
		default: // the folder's copy is newer
			var ex *domain.Project
			if haveL {
				ex = &p
			}
			if _, _, aerr := s.applyLibraryApp(ctx, r, ex, "sync"); aerr != nil {
				res.Errors = append(res.Errors, r.Config.App.Name+": "+friendlyErr(aerr))
				continue
			}
			// The folder's version is the agreed one; should this computer's
			// copy differ (defaults filled in), the next sync pushes it.
			state.Hashes[id] = rh
			res.Pulled++
		}
	}
	for _, h := range state.Hashes {
		if h != "deleted" && h != "" {
			res.Apps++
		}
	}
	res.At = time.Now().UTC()
	state.Last = &res
	if serr := s.store.SetSetting(ctx, syncStateKey, state); serr != nil {
		res.Errors = append(res.Errors, serr.Error())
	}
	return res, nil
}

// syncFileName is the app's file in the sync folder.
func syncFileName(identifier string) string {
	clean := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, identifier)
	return clean + LibraryExt
}

// SyncStatus implements Service: the last sync's result, if any.
func (s *Impl) SyncStatus(ctx context.Context) (*SyncResult, error) {
	var state syncState
	if _, err := s.store.GetSetting(ctx, syncStateKey, &state); err != nil {
		return nil, err
	}
	return state.Last, nil
}
