package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// JobMeta is service-level bookkeeping persisted next to a job so any process
// can resume it (for example after GUI approval).
type JobMeta struct {
	Kind           string `json:"kind"` // "build" | "update" | "repair"
	OutputDir      string `json:"outputDir,omitempty"`
	AllowDownload  bool   `json:"allowDownload,omitempty"`
	InstallationID string `json:"installationId,omitempty"`
}

const jobCols = `id, project_id, project_rev, snapshot_json, config_hash, targets_json, runtime_version,
	status, stage, progress, actor, created_at, started_at, finished_at, error_json, warnings_json`

func scanJob(r rowScanner) (domain.BuildJob, error) {
	var (
		j                  domain.BuildJob
		snap, targets      string
		status, stage      string
		created            int64
		started, finished  sql.NullInt64
		errJSON, warnsJSON sql.NullString
	)
	if err := r.Scan(&j.ID, &j.ProjectID, &j.ProjectRev, &snap, &j.ConfigHash, &targets, &j.RuntimeVersion,
		&status, &stage, &j.Progress, &j.Actor, &created, &started, &finished, &errJSON, &warnsJSON); err != nil {
		return j, err
	}
	j.Status, j.Stage = domain.JobStatus(status), domain.Stage(stage)
	j.CreatedAt = fromMS(created)
	j.StartedAt, j.FinishedAt = ptrFromNull(started), ptrFromNull(finished)
	var c config.Config
	if err := json.Unmarshal([]byte(snap), &c); err == nil {
		j.Snapshot = &c
	}
	_ = json.Unmarshal([]byte(targets), &j.Targets)
	if errJSON.Valid && errJSON.String != "" && errJSON.String != "null" {
		var e domain.Error
		if json.Unmarshal([]byte(errJSON.String), &e) == nil {
			j.Error = &e
		}
	}
	if warnsJSON.Valid && warnsJSON.String != "" {
		_ = json.Unmarshal([]byte(warnsJSON.String), &j.Warnings)
	}
	return j, nil
}

func nullableJSON(v any, isNil bool) any {
	if isNil {
		return nil
	}
	return mustJSON(v)
}

// InsertJob stores a new job. ID and CreatedAt are filled when empty.
func (s *Store) InsertJob(ctx context.Context, j domain.BuildJob, meta JobMeta) (domain.BuildJob, error) {
	if j.ID == "" {
		j.ID = NewID(PrefixJob)
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now().UTC().Truncate(time.Millisecond)
	}
	err := s.tx(ctx, true, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO jobs(id, project_id, project_rev, snapshot_json, config_hash,
			targets_json, runtime_version, status, stage, progress, actor, meta_json, created_at, started_at,
			finished_at, error_json, warnings_json) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			j.ID, j.ProjectID, j.ProjectRev, mustJSON(j.Snapshot), j.ConfigHash, mustJSON(j.Targets),
			j.RuntimeVersion, string(j.Status), string(j.Stage), j.Progress, j.Actor, mustJSON(meta),
			ms(j.CreatedAt), msPtr(j.StartedAt), msPtr(j.FinishedAt),
			nullableJSON(j.Error, j.Error == nil), nullableJSON(j.Warnings, len(j.Warnings) == 0))
		return err
	})
	return j, err
}

// SaveJobState persists the mutable parts of a job (status, stage, progress,
// timestamps, error, warnings).
func (s *Store) SaveJobState(ctx context.Context, j domain.BuildJob) error {
	return s.tx(ctx, true, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE jobs SET status=?, stage=?, progress=?, started_at=?, finished_at=?,
			error_json=?, warnings_json=? WHERE id=?`,
			string(j.Status), string(j.Stage), j.Progress, msPtr(j.StartedAt), msPtr(j.FinishedAt),
			nullableJSON(j.Error, j.Error == nil), nullableJSON(j.Warnings, len(j.Warnings) == 0), j.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return notFound("job", j.ID)
		}
		return nil
	})
}

// SetJobMeta replaces a job's meta (used when approval flips AllowDownload).
func (s *Store) SetJobMeta(ctx context.Context, id string, meta JobMeta) error {
	return s.tx(ctx, true, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE jobs SET meta_json=? WHERE id=?`, mustJSON(meta), id)
		return err
	})
}

// GetJobMeta returns a job's meta.
func (s *Store) GetJobMeta(ctx context.Context, id string) (JobMeta, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT meta_json FROM jobs WHERE id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return JobMeta{}, notFound("job", id)
	}
	var m JobMeta
	if err == nil {
		_ = json.Unmarshal([]byte(raw), &m)
	}
	return m, err
}

// GetJob returns a job with its artifacts.
func (s *Store) GetJob(ctx context.Context, id string) (domain.BuildJob, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return j, notFound("job", id)
	}
	if err != nil {
		return j, err
	}
	j.Artifacts, err = s.ListArtifactsByJob(ctx, id)
	return j, err
}

// ListJobs returns recent jobs newest first (projectID "" = all), each with
// its artifacts. limit <= 0 means 50.
func (s *Store) ListJobs(ctx context.Context, projectID string, limit int) ([]domain.BuildJob, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT ` + jobCols + ` FROM jobs`
	var args []any
	if projectID != "" {
		q += ` WHERE project_id=?`
		args = append(args, projectID)
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var out []domain.BuildJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Artifacts, err = s.ListArtifactsByJob(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// AppendLogs appends log lines to a job in one transaction. It does not bump
// the change counter (log traffic is high; state changes bump it).
func (s *Store) AppendLogs(ctx context.Context, jobID string, lines []domain.LogLine) error {
	if len(lines) == 0 {
		return nil
	}
	return s.tx(ctx, false, func(tx *sql.Tx) error {
		var seq int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM job_logs WHERE job_id=?`, jobID).Scan(&seq); err != nil {
			return err
		}
		st, err := tx.PrepareContext(ctx, `INSERT INTO job_logs(job_id, seq, time, level, stage, message) VALUES (?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, l := range lines {
			seq++
			if _, err := st.ExecContext(ctx, jobID, seq, ms(l.Time), l.Level, string(l.Stage), l.Message); err != nil {
				return err
			}
		}
		return nil
	})
}

// JobLog returns every log line of a job in order.
func (s *Store) JobLog(ctx context.Context, jobID string) ([]domain.LogLine, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT time, level, stage, message FROM job_logs WHERE job_id=? ORDER BY seq`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.LogLine
	for rows.Next() {
		var l domain.LogLine
		var t int64
		var st string
		if err := rows.Scan(&t, &l.Level, &st, &l.Message); err != nil {
			return nil, err
		}
		l.Time, l.Stage = fromMS(t), domain.Stage(st)
		out = append(out, l)
	}
	return out, rows.Err()
}

// ---- artifacts -----------------------------------------------------------

const artifactCols = `id, job_id, project_id, target_json, path, size, sha256, signature, created_at`

func scanArtifact(r rowScanner) (domain.Artifact, error) {
	var a domain.Artifact
	var target string
	var created int64
	if err := r.Scan(&a.ID, &a.JobID, &a.ProjectID, &target, &a.Path, &a.Size, &a.SHA256, &a.Signature, &created); err != nil {
		return a, err
	}
	_ = json.Unmarshal([]byte(target), &a.Target)
	a.CreatedAt = fromMS(created)
	return a, nil
}

// InsertArtifact stores an artifact; ID and CreatedAt are filled when empty.
func (s *Store) InsertArtifact(ctx context.Context, a domain.Artifact) (domain.Artifact, error) {
	if a.ID == "" {
		a.ID = NewID(PrefixArtifact)
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC().Truncate(time.Millisecond)
	}
	err := s.tx(ctx, true, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO artifacts(`+artifactCols+`) VALUES (?,?,?,?,?,?,?,?,?)`,
			a.ID, a.JobID, a.ProjectID, mustJSON(a.Target), a.Path, a.Size, a.SHA256, a.Signature, ms(a.CreatedAt))
		return err
	})
	return a, err
}

// GetArtifact returns one artifact.
func (s *Store) GetArtifact(ctx context.Context, id string) (domain.Artifact, error) {
	a, err := scanArtifact(s.db.QueryRowContext(ctx, `SELECT `+artifactCols+` FROM artifacts WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, notFound("artifact", id)
	}
	return a, err
}

func (s *Store) queryArtifacts(ctx context.Context, where string, arg any) ([]domain.Artifact, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+artifactCols+` FROM artifacts WHERE `+where+` ORDER BY created_at, id`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Artifact
	for rows.Next() {
		a, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListArtifactsByJob returns a job's artifacts.
func (s *Store) ListArtifactsByJob(ctx context.Context, jobID string) ([]domain.Artifact, error) {
	return s.queryArtifacts(ctx, `job_id=?`, jobID)
}

// ListArtifactsByProject returns a project's artifacts, oldest first.
func (s *Store) ListArtifactsByProject(ctx context.Context, projectID string) ([]domain.Artifact, error) {
	return s.queryArtifacts(ctx, `project_id=?`, projectID)
}

// ListAllArtifacts returns every artifact.
func (s *Store) ListAllArtifacts(ctx context.Context) ([]domain.Artifact, error) {
	return s.queryArtifacts(ctx, `1=?`, 1)
}
