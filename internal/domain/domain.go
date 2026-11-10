// Package domain holds the persistent entities of spec §9C.8:
//
//	Project (editable versioned configuration)
//	├── BuildJob (immutable configuration snapshot)
//	│   └── Artifact (hash, target, signature, output path)
//	├── Installation (OS, path, verified installation ID, current artifact)
//	│   └── ProfileReference (stable per-user data location)
//	└── ExternalImport (source, confidence, read-only metadata)
//
// All machine-facing identifiers and enums are stable English strings.
package domain

import (
	"time"

	"mypack.dev/mypack/pkg/config"
)

// Project is an editable app configuration. Revision increments on every
// write and is used for optimistic concurrency between GUI, CLI and MCP.
type Project struct {
	ID        string         `json:"id"`
	Revision  int64          `json:"revision"`
	Config    *config.Config `json:"config"`
	IconPNG   []byte         `json:"-"` // resolved icon (512x512 PNG), may be nil
	IconURL   string         `json:"iconUrl,omitempty"`
	Draft     bool           `json:"draft"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	LastBuild *time.Time     `json:"lastBuild,omitempty"`
	TrashedAt *time.Time     `json:"trashedAt,omitempty"`
	Origin    string         `json:"origin"` // "gui" | "cli" | "mcp" | "companion" | "import"
}

// JobStatus is the BuildJob lifecycle state.
type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "completed"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
	// JobAwaitingApproval: a headless request needs GUI approval (spec §12.5).
	JobAwaitingApproval JobStatus = "awaiting_approval"
)

// Stage is a pipeline stage (spec §14).
type Stage string

const (
	StageQueued      Stage = "queued"
	StageValidating  Stage = "validating"
	StageResolving   Stage = "resolving"
	StageDownloading Stage = "downloading"
	StageAssembling  Stage = "assembling"
	StageCustomizing Stage = "customizing"
	StageSigning     Stage = "signing"
	StagePackaging   Stage = "packaging"
	StageVerifying   Stage = "verifying"
	StagePublishing  Stage = "publishing"
	StageCompleted   Stage = "completed"
)

// Stages is the ordered pipeline for progress display.
var Stages = []Stage{StageValidating, StageResolving, StageDownloading, StageAssembling,
	StageCustomizing, StageSigning, StagePackaging, StageVerifying, StagePublishing}

// BuildJob records one build of an immutable config snapshot.
type BuildJob struct {
	ID             string         `json:"id"`
	ProjectID      string         `json:"projectId"`
	ProjectRev     int64          `json:"projectRevision"`
	Snapshot       *config.Config `json:"snapshot"`
	ConfigHash     string         `json:"configHash"`
	Targets        []config.Target `json:"targets"`
	RuntimeVersion string         `json:"runtimeVersion"`
	Status         JobStatus      `json:"status"`
	Stage          Stage          `json:"stage"`
	// Progress is 0..1 when the current stage has a trustworthy denominator, else -1.
	Progress   float64    `json:"progress"`
	Actor      string     `json:"actor"` // "gui" | "cli" | "mcp:<client>"
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Error      *Error     `json:"error,omitempty"`
	Warnings   []string   `json:"warnings,omitempty"`
	Artifacts  []Artifact `json:"artifacts,omitempty"`
	Log        []LogLine  `json:"-"`
}

// Artifact is one output of a build.
type Artifact struct {
	ID        string        `json:"id"`
	JobID     string        `json:"jobId"`
	ProjectID string        `json:"projectId"`
	Target    config.Target `json:"target"`
	Path      string        `json:"path"`
	Size      int64         `json:"size"`
	SHA256    string        `json:"sha256"`
	Signature string        `json:"signature"` // "unsigned" | "adhoc" | "signed" | "notarized"
	CreatedAt time.Time     `json:"createdAt"`
}

// InstallState is a managed-installation health state (spec §9C.7).
type InstallState string

const (
	InstallManaged   InstallState = "managed"
	InstallExternal  InstallState = "external_readonly"
	InstallMissing   InstallState = "missing"
	InstallAttention InstallState = "attention"
)

// Installation is an app placed on this machine (≠ a build artifact).
type Installation struct {
	ID             string       `json:"id"`
	ProjectID      string       `json:"projectId,omitempty"`
	ArtifactID     string       `json:"artifactId,omitempty"`
	Name           string       `json:"name"`
	Identifier     string       `json:"identifier"`
	SourceURL      string       `json:"sourceUrl,omitempty"`
	Path           string       `json:"path"`
	Version        string       `json:"version"`
	RuntimeVersion string       `json:"runtimeVersion"`
	ProfileDir     string       `json:"profileDir"`
	State          InstallState `json:"state"`
	UpdateReason   string       `json:"updateReason,omitempty"` // non-empty => update available
	InstalledAt    time.Time    `json:"installedAt"`
	LastManagedAt  time.Time    `json:"lastManagedAt"`
	IconPNG        []byte       `json:"-"`
}

// LogLine is one structured build log entry.
type LogLine struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"` // "debug" | "info" | "warn" | "error"
	Stage   Stage     `json:"stage"`
	Message string    `json:"message"`
}

// Error is a structured, stable error: what happened → why → how to fix (§9.5).
type Error struct {
	Code        string `json:"code"`              // stable, e.g. RUNTIME_VERIFY_FAILED
	Message     string `json:"message"`           // English summary
	Cause       string `json:"cause,omitempty"`   // technical detail
	Remediation string `json:"remediation,omitempty"`
	Field       string `json:"field,omitempty"`
}

func (e *Error) Error() string {
	if e.Cause != "" {
		return e.Code + ": " + e.Message + " (" + e.Cause + ")"
	}
	return e.Code + ": " + e.Message
}

// RuntimeInfo describes a cached or available generated-app runtime.
type RuntimeInfo struct {
	Version   string    `json:"version"`
	Key       string    `json:"key"` // e.g. darwin-arm64
	Cached    bool      `json:"cached"`
	Verified  bool      `json:"verified"`
	Size      int64     `json:"size"`       // download size
	CacheSize int64     `json:"cacheSize"`  // on-disk size when cached
	Path      string    `json:"path,omitempty"`
	LastUsed  time.Time `json:"lastUsed,omitempty"`
	Source    string    `json:"source"` // registry URL
}

// Activity is an audit record (spec §12.6).
type Activity struct {
	ID        string    `json:"id"`
	Time      time.Time `json:"time"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	RequestID string    `json:"requestId,omitempty"`
	Result    string    `json:"result"`
}
