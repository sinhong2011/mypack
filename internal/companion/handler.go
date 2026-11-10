package companion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"sync"
	"time"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// Launcher shows mypack Desktop (optionally on a project).
type Launcher interface {
	OpenManager(projectID string) error
}

// Handler dispatches protocol requests to the application service.
type Handler struct {
	// Svc may be nil: every action except hello then answers HOST_UNAVAILABLE.
	Svc      appservice.Service
	Launcher Launcher
	// Limiter is optional; NewHandler installs 20 req/s.
	Limiter *Limiter
	// MetadataTimeout bounds the favicon/title fetch of createDraft.
	MetadataTimeout time.Duration
}

// NewHandler returns a Handler with defaults.
func NewHandler(svc appservice.Service) *Handler {
	return &Handler{Svc: svc, Launcher: ExecLauncher{}, Limiter: NewLimiter(20, 20), MetadataTimeout: 6 * time.Second}
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

func fail(id, code, details string) Response {
	return Response{RequestID: id, Error: &Error{Code: code, Details: details}}
}

// HandleFrame processes one raw request frame and returns the response.
func (h *Handler) HandleFrame(ctx context.Context, raw []byte) Response {
	if h.Limiter != nil && !h.Limiter.Allow() {
		return fail("", ErrRateLimited, "too many requests")
	}
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return fail("", ErrInvalidRequest, "malformed JSON")
	}
	if len(req.RequestID) > 128 {
		req.RequestID = req.RequestID[:128]
	}
	if req.ProtocolVersion != ProtocolVersion {
		return fail(req.RequestID, ErrProtocolMismatch, "host speaks protocol 1")
	}
	if req.Action == "" {
		return fail(req.RequestID, ErrInvalidRequest, "missing action")
	}
	if req.Action != ActionHello && h.Svc == nil {
		return fail(req.RequestID, ErrHostUnavailable, "application service is not available")
	}
	var (
		res interface{}
		e   *Error
	)
	switch req.Action {
	case ActionHello:
		res = h.hello(ctx)
	case ActionLookup:
		res, e = h.lookup(ctx, req.Payload)
	case ActionCreateDraft:
		res, e = h.createDraft(ctx, req.Payload)
	case ActionOpenManaged:
		res, e = h.openManaged(ctx, req.Payload)
	case ActionOpenManager:
		res, e = h.openManager(ctx, req.Payload)
	default:
		return fail(req.RequestID, ErrInvalidRequest, "unknown action")
	}
	if e != nil {
		return Response{RequestID: req.RequestID, Error: e}
	}
	return Response{RequestID: req.RequestID, OK: true, Result: res}
}

func decode(payload json.RawMessage, v interface{}) *Error {
	if len(payload) == 0 {
		return &Error{Code: ErrInvalidRequest, Details: "missing payload"}
	}
	if err := json.Unmarshal(payload, v); err != nil {
		return &Error{Code: ErrInvalidRequest, Details: "invalid payload"}
	}
	return nil
}

func (h *Handler) hello(ctx context.Context) HelloResult {
	r := HelloResult{HostVersion: HostVersion, ProtocolVersion: ProtocolVersion, Actions: Actions, Locale: "system"}
	if h.Svc != nil {
		if s, err := h.Svc.Settings(ctx); err == nil && s.Language != "" {
			r.Locale = s.Language
		}
	}
	return r
}

func (h *Handler) lookup(ctx context.Context, payload json.RawMessage) (interface{}, *Error) {
	var p LookupPayload
	if e := decode(payload, &p); e != nil {
		return nil, e
	}
	_, host, ok := ParseWebURL(p.URL)
	if !ok {
		return nil, &Error{Code: ErrUnsupportedURL, Details: "only http(s) URLs up to 2048 chars are supported"}
	}
	projects, err := h.Svc.ListProjects(ctx)
	if err != nil {
		return nil, &Error{Code: ErrHostUnavailable, Details: "cannot list projects"}
	}
	installs, err := h.Svc.ListInstallations(ctx)
	if err != nil {
		return nil, &Error{Code: ErrHostUnavailable, Details: "cannot list installations"}
	}
	byProject := map[string]domain.Project{}
	for _, pr := range projects {
		byProject[pr.ID] = pr
	}
	matches := []Match{}
	covered := map[string]bool{}
	for _, in := range installs {
		var cfg *config.Config
		if pr, ok := byProject[in.ProjectID]; ok {
			cfg = pr.Config
		}
		src, safe := sourceHostAndSafe(cfg, in.SourceURL)
		if !hostMatches(host, src, safe) {
			continue
		}
		if in.ProjectID != "" {
			covered[in.ProjectID] = true
		}
		profile := ""
		if cfg != nil {
			profile = cfg.App.ProfileID
		}
		matches = append(matches, Match{InstallationID: in.ID, ProjectID: in.ProjectID, Name: in.Name,
			ProfileID: profile, State: string(in.State)})
	}
	for _, pr := range projects {
		if covered[pr.ID] || pr.Config == nil || pr.TrashedAt != nil {
			continue
		}
		src, safe := sourceHostAndSafe(pr.Config, "")
		if !hostMatches(host, src, safe) {
			continue
		}
		st := "project"
		if pr.Draft {
			st = "draft"
		}
		matches = append(matches, Match{ProjectID: pr.ID, Name: pr.Config.App.Name, ProfileID: pr.Config.App.ProfileID, State: st})
	}
	kind := MatchNone
	switch {
	case len(matches) == 1:
		kind = MatchUnique
	case len(matches) > 1:
		kind = MatchMultiple
	}
	return LookupResult{Host: host, Kind: kind, Matches: matches}, nil
}

func sourceHostAndSafe(cfg *config.Config, fallbackURL string) (string, []string) {
	src := fallbackURL
	var safe []string
	if cfg != nil {
		if cfg.App.Source.Type == config.SourceURL {
			src = cfg.App.Source.Value
		} else {
			src = ""
		}
		safe = cfg.WebView.Navigation.SafeDomains
	}
	if src == "" {
		return "", safe
	}
	if _, h, ok := ParseWebURL(src); ok {
		return h, safe
	}
	return "", safe
}

func (h *Handler) createDraft(ctx context.Context, payload json.RawMessage) (interface{}, *Error) {
	var p CreateDraftPayload
	if e := decode(payload, &p); e != nil {
		return nil, e
	}
	u, host, ok := ParseWebURL(p.URL)
	if !ok {
		return nil, &Error{Code: ErrUnsupportedURL, Details: "only http(s) URLs up to 2048 chars are supported"}
	}
	// Strip the fragment; keep path/query as the user's start page.
	u.Fragment = ""
	clean := u.String()
	title := SanitizeTitle(p.Title)

	md := appservice.Metadata{URL: clean, Fallback: true}
	if h.MetadataTimeout > 0 {
		fctx, cancel := context.WithTimeout(ctx, h.MetadataTimeout)
		if m, err := h.Svc.FetchMetadata(fctx, clean); err == nil {
			md = m
		}
		cancel()
	}
	if title != "" {
		md.Title = title
	}
	proj, err := h.Svc.CreateProject(ctx, appservice.CreateRequest{Input: clean, Name: title, Metadata: &md, Actor: ActorCompanion})
	if err != nil {
		return nil, mapErr(err, host)
	}
	name := host
	if proj.Config != nil {
		name = proj.Config.App.Name
	}
	return CreateDraftResult{ProjectID: proj.ID, Name: name, Draft: proj.Draft}, nil
}

func (h *Handler) openManaged(ctx context.Context, payload json.RawMessage) (interface{}, *Error) {
	var p OpenManagedPayload
	if e := decode(payload, &p); e != nil {
		return nil, e
	}
	if !idRe.MatchString(p.InstallationID) {
		return nil, &Error{Code: ErrInvalidRequest, Details: "bad installationId"}
	}
	if err := h.Svc.OpenInstallation(ctx, p.InstallationID); err != nil {
		return nil, mapErr(err, "")
	}
	return map[string]bool{"opened": true}, nil
}

func (h *Handler) openManager(ctx context.Context, payload json.RawMessage) (interface{}, *Error) {
	var p OpenManagerPayload
	if len(payload) > 0 {
		if e := decode(payload, &p); e != nil {
			return nil, e
		}
	}
	if p.ProjectID != "" {
		if !idRe.MatchString(p.ProjectID) {
			return nil, &Error{Code: ErrInvalidRequest, Details: "bad projectId"}
		}
		if _, err := h.Svc.GetProject(ctx, p.ProjectID); err != nil {
			return nil, mapErr(err, "")
		}
	}
	if h.Launcher == nil {
		return nil, &Error{Code: ErrHostUnavailable, Details: "no launcher"}
	}
	if err := h.Launcher.OpenManager(p.ProjectID); err != nil {
		return nil, &Error{Code: ErrHostUnavailable, Details: "cannot launch mypack Desktop"}
	}
	return map[string]bool{"opened": true}, nil
}

func mapErr(err error, host string) *Error {
	var de *domain.Error
	if errors.As(err, &de) {
		switch de.Code {
		case appservice.CodeNotFound, "APP_MISSING", appservice.CodeNotManaged:
			return &Error{Code: ErrAppNotFound, Details: de.Message}
		case appservice.CodeInvalidInput, appservice.CodeInvalidConfig:
			return &Error{Code: ErrUnsupportedURL, Details: de.Message}
		}
		return &Error{Code: ErrHostUnavailable, Details: de.Code}
	}
	return &Error{Code: ErrHostUnavailable, Details: "internal error"}
}

// ---- rate limiting ----------------------------------------------------------

// Limiter is a small token bucket.
type Limiter struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
	now    func() time.Time
}

func NewLimiter(perSecond, burst int) *Limiter {
	return &Limiter{rate: float64(perSecond), burst: float64(burst), tokens: float64(burst), now: time.Now}
}

func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.now()
	if l.last.IsZero() {
		l.last = t
	}
	l.tokens += t.Sub(l.last).Seconds() * l.rate
	if l.tokens > l.burst {
		l.tokens = l.burst
	}
	l.last = t
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}

// ---- serving ----------------------------------------------------------------

// Serve reads frames from r until EOF and writes responses to w.
func Serve(ctx context.Context, r io.Reader, w io.Writer, h *Handler) error {
	for {
		raw, err := ReadFrame(r, MaxRequestBytes)
		if err == io.EOF {
			return nil
		}
		if errors.Is(err, ErrFrameTooLarge) {
			// The body was not consumed, so the stream cannot be resynced.
			_ = WriteFrame(w, fail("", ErrInvalidRequest, "message too large"), MaxResponseBytes)
			return err
		}
		if err != nil {
			return err
		}
		resp := h.HandleFrame(ctx, raw)
		if err := WriteFrame(w, resp, MaxResponseBytes); err != nil {
			if errors.Is(err, ErrFrameTooLarge) {
				err = WriteFrame(w, fail(resp.RequestID, ErrInvalidRequest, "response too large"), MaxResponseBytes)
			}
			if err != nil {
				return err
			}
		}
	}
}

// ---- launching Desktop --------------------------------------------------------

// ExecLauncher starts mypack Desktop with the OS launcher.
type ExecLauncher struct{}

func (ExecLauncher) OpenManager(projectID string) error {
	switch runtime.GOOS {
	case "darwin":
		// The registered mypack:// deep link reaches a running instance too
		// (open --args would be dropped if mypack is already open).
		link := "mypack://library"
		if projectID != "" {
			link = "mypack://project/" + projectID
		}
		if err := exec.Command("open", link).Run(); err == nil {
			return nil
		}
		return exec.Command("open", "-b", "dev.mypack.builder").Run()
	case "windows":
		args := []string{}
		if projectID != "" {
			args = append(args, "--show-project", projectID)
		}
		return startDetached(exec.Command("mypack.exe", args...))
	default:
		args := []string{}
		if projectID != "" {
			args = append(args, "--show-project", projectID)
		}
		return startDetached(exec.Command("mypack", args...))
	}
}

func startDetached(c *exec.Cmd) error {
	c.Stdin, c.Stdout, c.Stderr = nil, nil, nil // never inherit the framed stdio
	if err := c.Start(); err != nil {
		return err
	}
	go func() { _ = c.Wait() }()
	return nil
}
