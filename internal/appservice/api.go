// Package appservice is the single Application Service shared by the native
// GUI, the headless CLI, the MCP server and the browser-companion host
// (spec §10, §12.1, ADR-007/020). Frontends never touch storage, the build
// engine or the filesystem directly.
//
// Every method is safe to call from any goroutine. Methods that take time
// (metadata fetch, builds, runtime downloads) honour ctx cancellation.
// Mutations take an Actor for the audit log and, where they edit a project,
// an expected revision for optimistic concurrency (REVISION_CONFLICT).
package appservice

import (
	"context"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// Service is the application API.
type Service interface {
	// ---- Projects -------------------------------------------------------

	// ListProjects returns non-trashed projects, most recently updated first.
	ListProjects(ctx context.Context) ([]domain.Project, error)
	GetProject(ctx context.Context, id string) (domain.Project, error)
	// CreateProject creates a draft from a URL, local file/dir, mypack.json
	// or Pake config (req.Input auto-detected). It never builds.
	CreateProject(ctx context.Context, req CreateRequest) (domain.Project, error)
	// UpdateProject replaces the config if expectedRevision matches.
	// Identifier/ProfileID changes require req.ConfirmIdentityChange.
	UpdateProject(ctx context.Context, req UpdateRequest) (domain.Project, error)
	SetProjectIcon(ctx context.Context, id string, png []byte, actor string) (domain.Project, error)
	// ProjectIconImage is the image beneath a styled icon (app.icon.style):
	// the project's IconPNG without the style. Unstyled projects return
	// their IconPNG.
	ProjectIconImage(ctx context.Context, id string) ([]byte, error)
	// CreateGroup creates a group app from website projects (never builds).
	CreateGroup(ctx context.Context, req GroupRequest) (domain.Project, error)
	// TrashProject soft-deletes (undoable via RestoreProject).
	TrashProject(ctx context.Context, id, actor string) error
	RestoreProject(ctx context.Context, id, actor string) error
	DuplicateProject(ctx context.Context, id, actor string) (domain.Project, error)
	ExportProject(ctx context.Context, id, path string) error
	// ExportLibrary writes projects (all when ids is empty) to a .mypack
	// file with their icons; it returns how many.
	ExportLibrary(ctx context.Context, ids []string, path string) (int, error)
	// ImportLibrary adds the apps of a .mypack file, keeping their identity;
	// apps already in the Library are skipped unless opts.Replace.
	ImportLibrary(ctx context.Context, path string, opts ImportOptions) (ImportResult, error)
	// SyncLibrary syncs the Library with Settings.SyncFolder (Pro).
	SyncLibrary(ctx context.Context) (SyncResult, error)
	// SyncStatus is the last sync's result (nil before the first).
	SyncStatus(ctx context.Context) (*SyncResult, error)

	// FetchMetadata resolves a website's title and best icon (≤ 8s, size
	// limited). Never fails hard on network errors: returns a Metadata with
	// Fallback=true so the user can continue (spec §9C.1).
	FetchMetadata(ctx context.Context, rawURL string) (Metadata, error)

	// ---- Builds ---------------------------------------------------------

	// Preflight reports runtime cache status, download sizes, capability
	// limits and output location without side effects.
	Preflight(ctx context.Context, projectID string, targets []config.Target) (Preflight, error)
	// StartBuild snapshots the project and queues a job. If a runtime must
	// be downloaded and opts.AllowDownload is false, the job is returned in
	// JobAwaitingApproval state with error code RUNTIME_DOWNLOAD_REQUIRED.
	StartBuild(ctx context.Context, req BuildRequest) (domain.BuildJob, error)
	CancelBuild(ctx context.Context, jobID, actor string) error
	GetJob(ctx context.Context, jobID string) (domain.BuildJob, error)
	JobLog(ctx context.Context, jobID string) ([]domain.LogLine, error)
	// ListJobs returns recent jobs (projectID "" = all), newest first.
	ListJobs(ctx context.Context, projectID string, limit int) ([]domain.BuildJob, error)
	// Approve resumes a job that is awaiting approval (runtime download or
	// project build; see BuildJob.Approval). MCP actors get APPROVAL_REQUIRED.
	ApproveJob(ctx context.Context, jobID, actor string) error

	// ---- Installations (Library › My Apps) ------------------------------

	// InstallAndOpen copies an artifact to the install location
	// (~/Applications on macOS), records the Installation and launches it.
	// On name conflict with a non-managed app it returns INSTALL_CONFLICT
	// unless req.Replace is set.
	InstallAndOpen(ctx context.Context, req InstallRequest) (domain.Installation, error)
	ListInstallations(ctx context.Context) ([]domain.Installation, error)
	OpenInstallation(ctx context.Context, id string) error
	// OpenArtifact launches a built artifact in place, without installing
	// it (Library › Open on a project that was built but not installed).
	OpenArtifact(ctx context.Context, artifactID string) error
	// UpdateInstallation rebuilds from the project and atomically replaces
	// the installed app, keeping the previous copy and the profile data.
	UpdateInstallation(ctx context.Context, id, actor string) (domain.BuildJob, error)
	// RepairInstallation re-assembles from the verified runtime without
	// touching profile data.
	RepairInstallation(ctx context.Context, id, actor string) (domain.BuildJob, error)
	// Uninstall removes the program; clearData additionally removes the
	// website data (profile) and must be separately confirmed in UI.
	Uninstall(ctx context.Context, id string, clearData bool, actor string) error
	// RelocateInstallation reconnects a missing installation to the app now at
	// newPath (spec §9C.7). The app must be the same one (identifier match and,
	// for managed apps, created by mypack), else NOT_SAME_APP.
	RelocateInstallation(ctx context.Context, id, newPath, actor string) (domain.Installation, error)
	// ForgetInstallation removes the library record only; no files or website
	// data are touched.
	ForgetInstallation(ctx context.Context, id, actor string) error
	// ImportExistingApp registers a third-party app read-only (spec §9C.7).
	ImportExistingApp(ctx context.Context, path, actor string) (domain.Installation, error)

	// ---- License (spec §9E, offline-first, Builder-only) -----------------

	// License reports the local entitlement. It never needs the network.
	License(ctx context.Context) (LicenseStatus, error)
	// ActivateLicense verifies an entitlement token locally and stores it.
	// Failure: LICENSE_INVALID. The Free edition's managed-app allowance is
	// enforced by InstallAndOpen (LICENSE_LIMIT); nothing else is gated.
	ActivateLicense(ctx context.Context, token, actor string) (LicenseStatus, error)
	DeactivateLicense(ctx context.Context, actor string) error

	// ---- Runtimes -------------------------------------------------------

	ListRuntimes(ctx context.Context) ([]domain.RuntimeInfo, error)
	DownloadRuntime(ctx context.Context, version, key string, progress func(done, total int64)) (domain.RuntimeInfo, error)
	VerifyRuntime(ctx context.Context, version, key string) error
	DeleteRuntime(ctx context.Context, version, key string) error

	// ---- Platform helpers ----------------------------------------------

	HostTarget() config.Target
	Reveal(ctx context.Context, path string) error
	OpenURL(ctx context.Context, url string) error
	StorageBreakdown(ctx context.Context) (Storage, error)

	// ---- Settings, activity, events ------------------------------------

	Settings(ctx context.Context) (Settings, error)
	SaveSettings(ctx context.Context, s Settings) error
	Activity(ctx context.Context, limit int) ([]domain.Activity, error)

	// Subscribe registers fn for events; the returned func unsubscribes.
	// fn is called from a background goroutine and must not block.
	Subscribe(fn func(Event)) (unsubscribe func())

	Close() error
}

// CreateRequest creates a project draft.
type CreateRequest struct {
	Input string // URL, local path, mypack.json path or pake.json path
	Name  string // optional override; otherwise metadata title
	// Metadata, when already fetched by the UI, avoids a second fetch.
	Metadata *Metadata
	Actor    string
}

type UpdateRequest struct {
	ID                    string
	ExpectedRevision      int64
	Config                *config.Config
	ConfirmIdentityChange bool
	Actor                 string
}

type BuildRequest struct {
	ProjectID     string
	Targets       []config.Target // empty = project's targets
	AllowDownload bool
	// AllowProjectBuild approves running the project's own build (install +
	// build script) for this and future builds of the same command/folder.
	// Ignored for MCP actors, which can never approve.
	AllowProjectBuild bool
	OutputDir         string // empty = default output dir
	Actor             string
}

type InstallRequest struct {
	ArtifactID string
	Replace    bool
	Open       bool
	Actor      string
}

// Metadata is what we learned about a website.
type Metadata struct {
	URL         string `json:"url"`   // normalized final URL
	Title       string `json:"title"` // suggested app name
	Description string `json:"description,omitempty"`
	IconURL     string `json:"iconUrl,omitempty"`
	IconPNG     []byte `json:"-"` // best icon, decoded and re-encoded as PNG
	ThemeColor  string `json:"themeColor,omitempty"`
	Fallback    bool   `json:"fallback"` // true when fetch failed; values are guesses
	Warning     string `json:"warning,omitempty"`
}

// Preflight summarises what a build will need.
type Preflight struct {
	Targets []TargetPreflight `json:"targets"`
	// DownloadBytes is the total to fetch before building (0 = all cached).
	DownloadBytes  int64  `json:"downloadBytes"`
	OutputDir      string `json:"outputDir"`
	RuntimeVersion string `json:"runtimeVersion"`
}

type TargetPreflight struct {
	Target    config.Target      `json:"target"`
	Runtime   domain.RuntimeInfo `json:"runtime"`
	Supported bool               `json:"supported"`
	// Level: "portable" | "installer" | "distribution" (spec §6).
	Level   string `json:"level"`
	Signing string `json:"signing"`          // "adhoc" | "unsigned" | "unavailable"
	Reason  string `json:"reason,omitempty"` // why unsupported / limitations
}

type Storage struct {
	RuntimeCache int64 `json:"runtimeCache"`
	Artifacts    int64 `json:"artifacts"`
	Profiles     int64 `json:"profiles"`
}

// Settings are global user preferences.
type Settings struct {
	Language        string `json:"language"`   // "system" or a locale ID
	Appearance      string `json:"appearance"` // "system" | "light" | "dark"
	OutputDir       string `json:"outputDir"`
	InstallDir      string `json:"installDir"`
	RegistryURL     string `json:"registryUrl"`
	ShowMenuBarIcon bool   `json:"showMenuBarIcon"`
	CheckUpdates    bool   `json:"checkUpdates"`
	LibraryView     string `json:"libraryView"` // "grid" | "list"
	// SyncFolder is where Library sync keeps the Library (Pro); empty = off.
	SyncFolder string `json:"syncFolder,omitempty"`
}

// EventKind enumerates service events.
type EventKind string

const (
	EventProjectChanged  EventKind = "project.changed"
	EventProjectDeleted  EventKind = "project.deleted"
	EventJobChanged      EventKind = "job.changed"
	EventJobLog          EventKind = "job.log"
	EventInstallChanged  EventKind = "installation.changed"
	EventRuntimeProgress EventKind = "runtime.progress"
	EventRuntimeChanged  EventKind = "runtime.changed"
	EventSettingsChanged EventKind = "settings.changed"
	EventApprovalNeeded  EventKind = "approval.needed"
)

// Event is a change notification. Consumers re-read state from the service;
// events carry IDs and light payloads only.
type Event struct {
	Kind      EventKind        `json:"kind"`
	ProjectID string           `json:"projectId,omitempty"`
	JobID     string           `json:"jobId,omitempty"`
	InstallID string           `json:"installationId,omitempty"`
	Job       *domain.BuildJob `json:"job,omitempty"`
	Log       *domain.LogLine  `json:"log,omitempty"`
	Done      int64            `json:"done,omitempty"`
	Total     int64            `json:"total,omitempty"`
}
