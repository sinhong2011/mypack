// Package companion implements the host side of the optional mypack
// Companion browser extension (spec 09G): protocol v1 over Chrome/Edge
// Native Messaging (4-byte little-endian length + JSON on stdio).
//
// The host is deliberately small: it only creates project drafts, looks up
// managed apps by validated origin, opens an installed app and asks Desktop
// to show a project. It never builds, installs, signs or deletes anything.
package companion

import "encoding/json"

const (
	// ProtocolVersion is the only protocol version this host speaks.
	ProtocolVersion = 1
	// HostVersion is reported by hello.
	HostVersion = "0.1.0"
	// HostName is the Native Messaging host name.
	HostName = "dev.mypack.companion"

	MaxRequestBytes  = 64 * 1024
	MaxResponseBytes = 1024 * 1024
	MaxURLLen        = 2048
	MaxTitleRunes    = 200
	// ActorCompanion is the audit actor for everything done by this host.
	ActorCompanion = "companion"
)

// Fixed, non-localized action names.
const (
	ActionHello       = "hello"
	ActionLookup      = "lookup"
	ActionCreateDraft = "createDraft"
	ActionOpenManaged = "openManagedApp"
	ActionOpenManager = "openManager"
)

// Actions lists every action this host supports (returned by hello).
var Actions = []string{ActionHello, ActionLookup, ActionCreateDraft, ActionOpenManaged, ActionOpenManager}

// Stable error codes.
const (
	ErrHostUnavailable  = "HOST_UNAVAILABLE"
	ErrAppNotFound      = "APP_NOT_FOUND"
	ErrAmbiguousMatch   = "AMBIGUOUS_MATCH"
	ErrUnsupportedURL   = "UNSUPPORTED_URL"
	ErrProtocolMismatch = "PROTOCOL_MISMATCH"
	ErrInvalidRequest   = "INVALID_REQUEST"
	ErrRateLimited      = "RATE_LIMITED"
)

// Request is the versioned envelope.
type Request struct {
	ProtocolVersion int             `json:"protocolVersion"`
	RequestID       string          `json:"requestId"`
	Action          string          `json:"action"`
	Payload         json.RawMessage `json:"payload,omitempty"`
}

// Response is the host reply.
type Response struct {
	RequestID string      `json:"requestId"`
	OK        bool        `json:"ok"`
	Result    interface{} `json:"result,omitempty"`
	Error     *Error      `json:"error,omitempty"`
}

// Error carries a stable code; details are English and for diagnostics only
// (the extension localizes user-facing text).
type Error struct {
	Code    string `json:"code"`
	Details string `json:"details,omitempty"`
}

// HelloResult is returned by hello.
type HelloResult struct {
	HostVersion     string   `json:"hostVersion"`
	ProtocolVersion int      `json:"protocolVersion"`
	Actions         []string `json:"actions"`
	// Locale is the Desktop language preference ("system" or a locale ID).
	Locale string `json:"locale"`
}

type LookupPayload struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// Match kinds.
const (
	MatchNone     = "none"
	MatchUnique   = "unique"
	MatchMultiple = "multiple"
)

// Match is one managed installation or project.
type Match struct {
	InstallationID string `json:"installationId,omitempty"`
	ProjectID      string `json:"projectId,omitempty"`
	Name           string `json:"name"`
	ProfileID      string `json:"profileId"`
	// State: an installation state ("managed", "missing", ...) or "draft" /
	// "project" for a project that has no installation.
	State string `json:"state"`
}

type LookupResult struct {
	Host    string  `json:"host"`
	Kind    string  `json:"match"`
	Matches []Match `json:"matches"`
}

type CreateDraftPayload struct {
	URL        string `json:"url"`
	Title      string `json:"title,omitempty"`
	FaviconURL string `json:"faviconUrl,omitempty"`
}

type CreateDraftResult struct {
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
	Draft     bool   `json:"draft"`
}

type OpenManagedPayload struct {
	InstallationID string `json:"installationId"`
}

type OpenManagerPayload struct {
	ProjectID string `json:"projectId,omitempty"`
}
