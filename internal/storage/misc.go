package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"mypack.dev/mypack/internal/domain"
)

// ---- installations -------------------------------------------------------

const installCols = `id, project_id, artifact_id, name, identifier, source_url, path, version, runtime_version,
	profile_dir, state, update_reason, installed_at, last_managed_at, icon_png`

func scanInstall(r rowScanner) (domain.Installation, error) {
	var i domain.Installation
	var state string
	var installed, managed int64
	if err := r.Scan(&i.ID, &i.ProjectID, &i.ArtifactID, &i.Name, &i.Identifier, &i.SourceURL, &i.Path,
		&i.Version, &i.RuntimeVersion, &i.ProfileDir, &state, &i.UpdateReason, &installed, &managed, &i.IconPNG); err != nil {
		return i, err
	}
	i.State = domain.InstallState(state)
	i.InstalledAt, i.LastManagedAt = fromMS(installed), fromMS(managed)
	return i, nil
}

// SaveInstallation inserts or replaces an installation record (ID filled if empty).
func (s *Store) SaveInstallation(ctx context.Context, i domain.Installation) (domain.Installation, error) {
	if i.ID == "" {
		i.ID = NewID(PrefixInstallation)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	if i.InstalledAt.IsZero() {
		i.InstalledAt = now
	}
	if i.LastManagedAt.IsZero() {
		i.LastManagedAt = now
	}
	err := s.tx(ctx, true, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO installations(`+installCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			i.ID, i.ProjectID, i.ArtifactID, i.Name, i.Identifier, i.SourceURL, i.Path, i.Version, i.RuntimeVersion,
			i.ProfileDir, string(i.State), i.UpdateReason, ms(i.InstalledAt), ms(i.LastManagedAt), i.IconPNG)
		return err
	})
	return i, err
}

// GetInstallation returns one installation.
func (s *Store) GetInstallation(ctx context.Context, id string) (domain.Installation, error) {
	i, err := scanInstall(s.db.QueryRowContext(ctx, `SELECT `+installCols+` FROM installations WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return i, notFound("installation", id)
	}
	return i, err
}

// ListInstallations returns all installations ordered by name.
func (s *Store) ListInstallations(ctx context.Context) ([]domain.Installation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+installCols+` FROM installations ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Installation
	for rows.Next() {
		i, err := scanInstall(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// DeleteInstallation removes an installation record.
func (s *Store) DeleteInstallation(ctx context.Context, id string) error {
	return s.tx(ctx, true, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM installations WHERE id=?`, id)
		return err
	})
}

// ---- activity ------------------------------------------------------------

// AddActivity appends an audit record (ID and Time filled when empty).
func (s *Store) AddActivity(ctx context.Context, a domain.Activity) error {
	if a.ID == "" {
		a.ID = NewID(PrefixActivity)
	}
	if a.Time.IsZero() {
		a.Time = time.Now().UTC()
	}
	return s.tx(ctx, true, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO activity(id, time, actor, action, target, request_id, result)
			VALUES (?,?,?,?,?,?,?)`, a.ID, a.Time.UnixMilli(), a.Actor, a.Action, a.Target, a.RequestID, a.Result)
		return err
	})
}

// ListActivity returns the most recent records, newest first.
func (s *Store) ListActivity(ctx context.Context, limit int) ([]domain.Activity, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, time, actor, action, target, request_id, result
		FROM activity ORDER BY time DESC, rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Activity
	for rows.Next() {
		var a domain.Activity
		var t int64
		if err := rows.Scan(&a.ID, &t, &a.Actor, &a.Action, &a.Target, &a.RequestID, &a.Result); err != nil {
			return nil, err
		}
		a.Time = fromMS(t)
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- settings ------------------------------------------------------------

// GetSetting decodes the JSON value stored under key into v. It reports
// whether the key existed.
func (s *Store) GetSetting(ctx context.Context, key string, v any) (bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), v)
}

// SetSetting stores v as JSON under key. The reserved change counter key
// cannot be written this way.
func (s *Store) SetSetting(ctx context.Context, key string, v any) error {
	if key == ChangeSeqKey {
		return errors.New("storage: change_seq is reserved")
	}
	return s.tx(ctx, true, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, mustJSON(v))
		return err
	})
}
