package appservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// App definitions travel as .mypack files: one JSON document holding one
// or more apps, each with its config and the pictures a config only points
// at (the icon, the image beneath an Icon Studio style, a group's site
// icons). Apps keep their identity (identifier, profile id), so the same
// app imported on another computer is the same app, and importing again
// finds it. The same file, one per app, is what Library sync keeps in its
// folder (sync.go).

const (
	LibraryFormat  = "mypack.library"
	LibraryVersion = 1
	LibraryExt     = ".mypack"

	maxLibraryFile = 64 << 20
)

// LibraryFile is a .mypack document.
type LibraryFile struct {
	Format     string       `json:"format"`
	Version    int          `json:"version"`
	ExportedAt time.Time    `json:"exportedAt"`
	Apps       []LibraryApp `json:"apps"`
}

// LibraryApp is one app of a .mypack file.
type LibraryApp struct {
	Config      *config.Config    `json:"config,omitempty"`
	Icon        []byte            `json:"icon,omitempty"`        // PNG, as the Library shows it
	IconBase    []byte            `json:"iconBase,omitempty"`    // PNG beneath app.icon.style
	MemberIcons map[string][]byte `json:"memberIcons,omitempty"` // group site id -> PNG
	UpdatedAt   time.Time         `json:"updatedAt"`
	// Identifier names the app of a sync tombstone (Deleted, no config).
	Identifier string `json:"identifier,omitempty"`
	Deleted    bool   `json:"deleted,omitempty"`
}

func (a LibraryApp) id() string {
	if a.Config != nil {
		return a.Config.App.Identifier
	}
	return a.Identifier
}

// ImportOptions control ImportLibrary.
type ImportOptions struct {
	// Replace overwrites apps already in the Library (same identifier);
	// otherwise they are skipped.
	Replace bool
	Actor   string
}

// ImportResult says what an import did.
type ImportResult struct {
	Added    []string `json:"added"`    // project ids
	Updated  []string `json:"updated"`  // project ids
	Skipped  []string `json:"skipped"`  // app names already in the Library
	Refused  []string `json:"refused"`  // "Name: why"
	Warnings []string `json:"warnings"` // e.g. a local folder missing here
}

// libraryApp packs project p for a .mypack file.
func (s *Impl) libraryApp(p domain.Project) LibraryApp {
	cfg := p.Config.Clone()
	app := LibraryApp{Icon: p.IconPNG, UpdatedAt: p.UpdatedAt.UTC().Truncate(time.Second)}
	if p.TrashedAt != nil && p.TrashedAt.After(p.UpdatedAt) {
		app.UpdatedAt = p.TrashedAt.UTC().Truncate(time.Second)
	}
	if cfg.App.Icon.Style != nil {
		if b := s.iconBase(p); len(b) > 0 && !bytesEqual(b, p.IconPNG) {
			app.IconBase = b
		}
	}
	if cfg.App.Group != nil {
		for i := range cfg.App.Group.Members {
			m := &cfg.App.Group.Members[i]
			if m.Icon == "" {
				continue
			}
			if b, err := os.ReadFile(m.Icon); err == nil {
				if app.MemberIcons == nil {
					app.MemberIcons = map[string][]byte{}
				}
				app.MemberIcons[m.ID] = b
			}
			m.Icon = "" // a path on this computer; the bytes travel instead
		}
	}
	cfg.App.Icon.Path = "" // the icon travels as Icon
	if cfg.App.Icon.Source == "file" {
		cfg.App.Icon.Source = "default"
	}
	app.Config = cfg
	return app
}

func bytesEqual(a, b []byte) bool { return string(a) == string(b) }

// ExportLibrary implements Service: writes the projects ids (all of the
// Library when empty) to a .mypack file at path.
func (s *Impl) ExportLibrary(ctx context.Context, ids []string, path string) (n int, err error) {
	defer func() { s.audit(ctx, "gui", "library.export", fmt.Sprint(len(ids)), err) }()
	var ps []domain.Project
	if len(ids) == 0 {
		if ps, err = s.store.ListProjects(ctx, false); err != nil {
			return 0, err
		}
	} else {
		for _, id := range ids {
			p, gerr := s.store.GetProject(ctx, id)
			if gerr != nil {
				return 0, gerr
			}
			ps = append(ps, p)
		}
	}
	if len(ps) == 0 {
		return 0, newErr(CodeInvalidInput, "There are no apps to export.", "Add an app to your Library first.")
	}
	f := LibraryFile{Format: LibraryFormat, Version: LibraryVersion, ExportedAt: time.Now().UTC().Truncate(time.Second)}
	for _, p := range ps {
		f.Apps = append(f.Apps, s.libraryApp(p))
	}
	if err = writeLibraryFile(path, f); err != nil {
		return 0, newErr(CodeInternal, "The file could not be saved.", "Check that the folder exists and is writable.")
	}
	return len(f.Apps), nil
}

func writeLibraryFile(path string, f LibraryFile) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadLibraryFile parses a .mypack file.
func ReadLibraryFile(path string) (LibraryFile, error) {
	var f LibraryFile
	fh, err := os.Open(path)
	if err != nil {
		return f, err
	}
	defer fh.Close()
	b, err := io.ReadAll(io.LimitReader(fh, maxLibraryFile+1))
	if err != nil {
		return f, err
	}
	if len(b) > maxLibraryFile {
		return f, errors.New("the file is larger than 64 MB")
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, fmt.Errorf("not a mypack file: %w", err)
	}
	if f.Format != LibraryFormat {
		return f, errors.New("not a mypack file")
	}
	if f.Version > LibraryVersion {
		return f, fmt.Errorf("made by a newer mypack (format %d)", f.Version)
	}
	return f, nil
}

// IsLibraryFile reports whether path looks like a .mypack file.
func IsLibraryFile(path string) bool { return strings.EqualFold(filepath.Ext(path), LibraryExt) }

// ImportLibrary implements Service.
func (s *Impl) ImportLibrary(ctx context.Context, path string, opts ImportOptions) (res ImportResult, err error) {
	actor := actorOr(opts.Actor, "gui")
	defer func() { s.audit(ctx, actor, "library.import", filepath.Base(path), err) }()
	f, rerr := ReadLibraryFile(expandHome(path))
	if rerr != nil {
		return res, newErr(CodeInvalidInput, "That file could not be imported: "+rerr.Error()+".", "Choose a .mypack file exported by mypack.")
	}
	byID, err := s.projectsByIdentifier(ctx)
	if err != nil {
		return res, err
	}
	for _, app := range f.Apps {
		if app.Deleted || app.Config == nil {
			continue
		}
		name := app.Config.App.Name
		cur, exists := byID[app.id()]
		if exists && !opts.Replace {
			res.Skipped = append(res.Skipped, name)
			continue
		}
		var ex *domain.Project
		if exists {
			ex = &cur
		}
		p, warn, aerr := s.applyLibraryApp(ctx, app, ex, actor)
		if aerr != nil {
			res.Refused = append(res.Refused, name+": "+friendlyErr(aerr))
			continue
		}
		res.Warnings = append(res.Warnings, warn...)
		if exists {
			res.Updated = append(res.Updated, p.ID)
		} else {
			res.Added = append(res.Added, p.ID)
		}
		byID[app.id()] = p
	}
	return res, nil
}

func friendlyErr(err error) string {
	var de *domain.Error
	if errors.As(err, &de) {
		return de.Message
	}
	return err.Error()
}

// projectsByIdentifier maps app identifiers to projects (trashed ones too).
func (s *Impl) projectsByIdentifier(ctx context.Context) (map[string]domain.Project, error) {
	ps, err := s.store.ListProjects(ctx, true)
	if err != nil {
		return nil, err
	}
	out := map[string]domain.Project{}
	for _, p := range ps {
		if p.Config == nil {
			continue
		}
		id := p.Config.App.Identifier
		if o, dup := out[id]; dup && o.TrashedAt == nil {
			continue // keep the one in the Library over one in the Trash
		}
		out[id] = p
	}
	return out, nil
}

// applyLibraryApp creates a project from app, or replaces ex with it (a
// trashed ex comes back to the Library). It returns warnings about things
// that will not work on this computer.
func (s *Impl) applyLibraryApp(ctx context.Context, app LibraryApp, ex *domain.Project, actor string) (domain.Project, []string, error) {
	cfg := app.Config.Clone()
	name := cfg.App.Name
	cfg.ApplyDefaults(s.host)
	if verr := cfg.Validate(); verr != nil {
		return domain.Project{}, nil, asDomainErr(verr, CodeInvalidConfig)
	}
	var warn []string
	switch cfg.App.Source.Type {
	case config.SourceDir, config.SourceFile:
		if _, err := os.Stat(cfg.App.Source.Value); err != nil {
			warn = append(warn, fmt.Sprintf("%s: its folder %s is not on this computer.", name, cfg.App.Source.Value))
		}
	}
	icon := app.Icon
	if len(icon) == 0 {
		icon = s.icons.Default(name)
	}
	var p domain.Project
	var err error
	if ex == nil {
		p, err = s.store.InsertProject(ctx, domain.Project{Config: cfg, IconPNG: icon, Draft: true, Origin: originOf(actor)})
		if err != nil {
			return p, nil, err
		}
	} else {
		cur := *ex
		cur.Config, cur.IconPNG = cfg, icon
		if p, err = s.store.UpdateProject(ctx, cur, cur.Revision); err != nil {
			return p, nil, err
		}
		if cur.TrashedAt != nil {
			if err = s.store.SetTrashed(ctx, cur.ID, nil); err != nil {
				return p, nil, err
			}
			p.TrashedAt = nil
		}
	}
	if cfg.App.Icon.Style != nil {
		base := app.IconBase
		if len(base) == 0 {
			base = icon
		}
		s.saveIconBase(p.ID, base)
	}
	// A group's site icons go beside the database, as CreateGroup keeps them.
	if cfg.App.Group != nil && len(app.MemberIcons) > 0 {
		changed := false
		for i := range cfg.App.Group.Members {
			m := &cfg.App.Group.Members[i]
			if b := app.MemberIcons[m.ID]; len(b) > 0 {
				if f := s.saveMemberIcon(p.ID, m.ID, b); f != "" {
					m.Icon, changed = f, true
				}
			}
		}
		if changed {
			p.Config = cfg
			if up, uerr := s.store.UpdateProject(ctx, p, p.Revision); uerr == nil {
				p = up
			}
		}
	}
	s.emit(Event{Kind: EventProjectChanged, ProjectID: p.ID})
	return p, warn, nil
}

// libraryHash is what an app looks like for sync: its config and pictures,
// not when it was saved. A tombstone hashes to "deleted".
func libraryHash(app LibraryApp) string {
	if app.Deleted {
		return "deleted"
	}
	c := struct {
		Config      *config.Config
		Icon, Base  []byte
		MemberIcons map[string][]byte
	}{app.Config, app.Icon, app.IconBase, app.MemberIcons}
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:12])
}
