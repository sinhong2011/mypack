// Package storage persists mypack state in a single SQLite database shared by
// the GUI, CLI and MCP processes (spec §12.9). The driver is modernc.org/sqlite
// (pure Go) so every platform builds with CGO_ENABLED=0.
//
// Concurrency model: WAL journal, busy_timeout 5s, immediate write
// transactions. Project writes use optimistic concurrency on `revision`.
// Every write bumps `change_seq` (settings table) so other processes can
// notice changes by polling; see Store.ExternalChange.
package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"mypack.dev/mypack/internal/domain"
)

// ChangeSeqKey is the settings key holding the cross-process change counter.
const ChangeSeqKey = "change_seq"

// Store is a handle to the database. It is safe for concurrent use.
type Store struct {
	db   *sql.DB
	path string

	mu       sync.Mutex // guards known / external
	known    int64      // last change_seq value this process has observed
	external bool       // an interleaved foreign bump was detected
}

// DefaultPath returns <os.UserConfigDir()>/mypack/mypack.db.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mypack", "mypack.db"), nil
}

// Open opens (creating if needed) the database at path ("" = DefaultPath),
// applies migrations and marks jobs interrupted by a previous crash/exit as
// failed with code INTERRUPTED.
func Open(path string) (*Store, error) {
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Set("_txlock", "immediate")
	dsn := "file:" + filepath.ToSlash(path) + "?" + q.Encode()
	if path == ":memory:" {
		dsn = "file::memory:?cache=shared&" + q.Encode()
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	s := &Store{db: db, path: path}
	ctx := context.Background()
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: migrate: %w", err)
	}
	if err := s.recoverInterrupted(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: recover jobs: %w", err)
	}
	seq, _ := s.ChangeSeq(ctx)
	s.known = seq
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Path returns the database file path.
func (s *Store) Path() string { return s.path }

// DB exposes the underlying handle for tests and diagnostics.
func (s *Store) DB() *sql.DB { return s.db }

// ---- IDs -----------------------------------------------------------------

var b32 = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// NewID returns prefix + "_" + 8 random base32 characters, e.g. "prj_k3x9q2ab".
func NewID(prefix string) string {
	var b [5]byte // 40 bits -> exactly 8 base32 chars
	if _, err := rand.Read(b[:]); err != nil {
		panic("storage: crypto/rand failed: " + err.Error())
	}
	return prefix + "_" + b32.EncodeToString(b[:])
}

// ID prefixes.
const (
	PrefixProject      = "prj"
	PrefixJob          = "job"
	PrefixArtifact     = "art"
	PrefixInstallation = "ins"
	PrefixActivity     = "act"
)

// ---- migrations ----------------------------------------------------------

// migrations are applied in order; the index+1 is the schema version.
// Never edit an applied migration: append a new one.
var migrations = []string{`
CREATE TABLE projects (
  id          TEXT PRIMARY KEY,
  revision    INTEGER NOT NULL DEFAULT 1,
  config_json TEXT NOT NULL,
  icon_png    BLOB,
  icon_url    TEXT NOT NULL DEFAULT '',
  draft       INTEGER NOT NULL DEFAULT 1,
  origin      TEXT NOT NULL DEFAULT 'gui',
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL,
  last_build  INTEGER,
  trashed_at  INTEGER
);
CREATE TABLE jobs (
  id              TEXT PRIMARY KEY,
  project_id      TEXT NOT NULL,
  project_rev     INTEGER NOT NULL,
  snapshot_json   TEXT NOT NULL,
  config_hash     TEXT NOT NULL,
  targets_json    TEXT NOT NULL,
  runtime_version TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL,
  stage           TEXT NOT NULL DEFAULT '',
  progress        REAL NOT NULL DEFAULT -1,
  actor           TEXT NOT NULL DEFAULT '',
  meta_json       TEXT NOT NULL DEFAULT '{}',
  created_at      INTEGER NOT NULL,
  started_at      INTEGER,
  finished_at     INTEGER,
  error_json      TEXT,
  warnings_json   TEXT
);
CREATE INDEX jobs_project ON jobs(project_id, created_at);
CREATE INDEX jobs_status ON jobs(status);
CREATE TABLE job_logs (
  job_id  TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  seq     INTEGER NOT NULL,
  time    INTEGER NOT NULL,
  level   TEXT NOT NULL,
  stage   TEXT NOT NULL,
  message TEXT NOT NULL,
  PRIMARY KEY (job_id, seq)
);
CREATE TABLE artifacts (
  id         TEXT PRIMARY KEY,
  job_id     TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL,
  target_json TEXT NOT NULL,
  path       TEXT NOT NULL,
  size       INTEGER NOT NULL DEFAULT 0,
  sha256     TEXT NOT NULL DEFAULT '',
  signature  TEXT NOT NULL DEFAULT 'unsigned',
  created_at INTEGER NOT NULL
);
CREATE INDEX artifacts_job ON artifacts(job_id);
CREATE INDEX artifacts_project ON artifacts(project_id);
CREATE TABLE installations (
  id              TEXT PRIMARY KEY,
  project_id      TEXT NOT NULL DEFAULT '',
  artifact_id     TEXT NOT NULL DEFAULT '',
  name            TEXT NOT NULL,
  identifier      TEXT NOT NULL DEFAULT '',
  source_url      TEXT NOT NULL DEFAULT '',
  path            TEXT NOT NULL,
  version         TEXT NOT NULL DEFAULT '',
  runtime_version TEXT NOT NULL DEFAULT '',
  profile_dir     TEXT NOT NULL DEFAULT '',
  state           TEXT NOT NULL,
  update_reason   TEXT NOT NULL DEFAULT '',
  installed_at    INTEGER NOT NULL,
  last_managed_at INTEGER NOT NULL,
  icon_png        BLOB
);
CREATE TABLE activity (
  id         TEXT PRIMARY KEY,
  time       INTEGER NOT NULL,
  actor      TEXT NOT NULL,
  action     TEXT NOT NULL,
  target     TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT '',
  result     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX activity_time ON activity(time);
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
INSERT INTO settings(key, value) VALUES ('change_seq', '0');
`}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	var cur int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&cur); err != nil {
		return err
	}
	if cur > len(migrations) {
		return fmt.Errorf("database schema v%d is newer than this mypack (v%d)", cur, len(migrations))
	}
	for v := cur; v < len(migrations); v++ {
		err := s.tx(ctx, false, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, migrations[v]); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
				v+1, time.Now().UnixMilli())
			return err
		})
		if err != nil {
			// Another process may have raced us and applied it already.
			var now int
			_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&now)
			if now > v {
				continue
			}
			return fmt.Errorf("migration %d: %w", v+1, err)
		}
	}
	return nil
}

// recoverInterrupted fails jobs left queued/running by a dead process.
func (s *Store) recoverInterrupted(ctx context.Context) error {
	e := &domain.Error{
		Code:        "INTERRUPTED",
		Message:     "The build was interrupted because mypack quit or crashed.",
		Remediation: "Start the build again.",
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE status IN (?, ?)`,
		string(domain.JobQueued), string(domain.JobRunning)).Scan(&n); err != nil || n == 0 {
		return err // nothing to recover: do not bump the change counter
	}
	return s.tx(ctx, true, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE jobs SET status=?, error_json=?, finished_at=?
			WHERE status IN (?, ?)`,
			string(domain.JobFailed), mustJSON(e), time.Now().UnixMilli(),
			string(domain.JobQueued), string(domain.JobRunning))
		return err
	})
}

// ---- helpers -------------------------------------------------------------

// tx runs fn in an immediate transaction. When bump is true the change
// counter is incremented inside the same transaction.
func (s *Store) tx(ctx context.Context, bump bool, fn func(*sql.Tx) error) error {
	t, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	var seq int64
	if err := fn(t); err != nil {
		_ = t.Rollback()
		return err
	}
	if bump {
		if err := t.QueryRowContext(ctx, `UPDATE settings SET value = CAST(CAST(value AS INTEGER)+1 AS TEXT)
			WHERE key=? RETURNING CAST(value AS INTEGER)`, ChangeSeqKey).Scan(&seq); err != nil {
			_ = t.Rollback()
			return err
		}
	}
	if err := t.Commit(); err != nil {
		return err
	}
	if bump {
		s.noteOwnBump(seq)
	}
	return nil
}

// noteOwnBump records our own counter increment, flagging an external write
// if the counter jumped by more than one since we last looked.
func (s *Store) noteOwnBump(seq int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if seq != s.known+1 {
		s.external = true
	}
	if seq > s.known {
		s.known = seq
	}
}

// ChangeSeq returns the current cross-process change counter.
func (s *Store) ChangeSeq(ctx context.Context) (int64, error) {
	var v int64
	err := s.db.QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM settings WHERE key=?`, ChangeSeqKey).Scan(&v)
	return v, err
}

// ExternalChange reports (and clears) whether another process has written
// since the last call. Intended to be polled about once a second.
func (s *Store) ExternalChange(ctx context.Context) (bool, error) {
	seq, err := s.ChangeSeq(ctx)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := s.external || seq != s.known
	s.external = false
	s.known = seq
	return changed, nil
}

func ms(t time.Time) int64 { return t.UnixMilli() }
func fromMS(v int64) time.Time {
	return time.UnixMilli(v).UTC()
}
func msPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UnixMilli()
}
func ptrFromNull(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := fromMS(v.Int64)
	return &t
}

func notFound(what, id string) error {
	return &domain.Error{
		Code:        "NOT_FOUND",
		Message:     fmt.Sprintf("%s %q was not found", what, id),
		Remediation: "Check the ID and try again.",
	}
}

// IsNotFound reports whether err is a NOT_FOUND domain error.
func IsNotFound(err error) bool {
	var de *domain.Error
	return errors.As(err, &de) && de.Code == "NOT_FOUND"
}
