package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

const projectCols = `id, revision, config_json, icon_png, icon_url, draft, origin,
	created_at, updated_at, last_build, trashed_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanProject(r rowScanner) (domain.Project, error) {
	var (
		p                  domain.Project
		cfgJSON            string
		icon               []byte
		draft              int
		created, updated   int64
		lastBuild, trashed sql.NullInt64
	)
	if err := r.Scan(&p.ID, &p.Revision, &cfgJSON, &icon, &p.IconURL, &draft, &p.Origin,
		&created, &updated, &lastBuild, &trashed); err != nil {
		return p, err
	}
	var c config.Config
	if err := json.Unmarshal([]byte(cfgJSON), &c); err != nil {
		return p, fmt.Errorf("project %s: corrupt config: %w", p.ID, err)
	}
	p.Config = &c
	p.IconPNG = icon
	p.Draft = draft != 0
	p.CreatedAt, p.UpdatedAt = fromMS(created), fromMS(updated)
	p.LastBuild, p.TrashedAt = ptrFromNull(lastBuild), ptrFromNull(trashed)
	return p, nil
}

// InsertProject stores a new project. ID (if empty), Revision (=1) and the
// timestamps are filled in; the stored project is returned.
func (s *Store) InsertProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	if p.ID == "" {
		p.ID = NewID(PrefixProject)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	p.Revision, p.CreatedAt, p.UpdatedAt = 1, now, now
	if p.Origin == "" {
		p.Origin = "gui"
	}
	cfg, err := json.Marshal(p.Config)
	if err != nil {
		return p, err
	}
	err = s.tx(ctx, true, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO projects(`+projectCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			p.ID, p.Revision, string(cfg), p.IconPNG, p.IconURL, boolInt(p.Draft), p.Origin,
			ms(p.CreatedAt), ms(p.UpdatedAt), msPtr(p.LastBuild), msPtr(p.TrashedAt))
		return err
	})
	return p, err
}

// GetProject returns a project (trashed ones included).
func (s *Store) GetProject(ctx context.Context, id string) (domain.Project, error) {
	p, err := scanProject(s.db.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, notFound("project", id)
	}
	return p, err
}

// ListProjects returns projects by most recently updated. Trashed projects
// are included only when includeTrashed is set.
func (s *Store) ListProjects(ctx context.Context, includeTrashed bool) ([]domain.Project, error) {
	q := `SELECT ` + projectCols + ` FROM projects`
	if !includeTrashed {
		q += ` WHERE trashed_at IS NULL`
	}
	q += ` ORDER BY updated_at DESC, id`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateProject writes p's config, icon and draft flag if the stored
// revision still equals expectedRev, incrementing the revision. When the
// revision has moved on it returns a REVISION_CONFLICT *domain.Error.
func (s *Store) UpdateProject(ctx context.Context, p domain.Project, expectedRev int64) (domain.Project, error) {
	cfg, err := json.Marshal(p.Config)
	if err != nil {
		return p, err
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	err = s.tx(ctx, true, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE projects SET config_json=?, icon_png=?, icon_url=?, draft=?,
			updated_at=?, revision=revision+1 WHERE id=? AND revision=?`,
			string(cfg), p.IconPNG, p.IconURL, boolInt(p.Draft), ms(now), p.ID, expectedRev)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 1 {
			return nil
		}
		var cur int64
		if err := tx.QueryRowContext(ctx, `SELECT revision FROM projects WHERE id=?`, p.ID).Scan(&cur); errors.Is(err, sql.ErrNoRows) {
			return notFound("project", p.ID)
		} else if err != nil {
			return err
		}
		return &domain.Error{
			Code:        "REVISION_CONFLICT",
			Message:     "The project was changed by someone else since you loaded it.",
			Cause:       fmt.Sprintf("expected revision %d, current revision is %d", expectedRev, cur),
			Remediation: "Reload the project, review the changes and try again.",
		}
	})
	if err != nil {
		return p, err
	}
	return s.GetProject(ctx, p.ID)
}

// SetTrashed soft-deletes (t != nil) or restores (t == nil) a project.
func (s *Store) SetTrashed(ctx context.Context, id string, t *time.Time) error {
	return s.tx(ctx, true, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE projects SET trashed_at=? WHERE id=?`, msPtr(t), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return notFound("project", id)
		}
		return nil
	})
}

// MarkBuilt records a successful build: sets last_build and clears the draft
// flag. It does not bump the revision (a build is not an edit).
func (s *Store) MarkBuilt(ctx context.Context, id string, t time.Time) error {
	return s.tx(ctx, true, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE projects SET last_build=?, draft=0 WHERE id=?`, ms(t), id)
		return err
	})
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
