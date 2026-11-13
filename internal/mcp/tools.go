package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// tool is one MCP tool: metadata, JSON Schema and handler.
type tool struct {
	Name, Title, Description string
	InputSchema              map[string]any
	Annotations              map[string]any
	Handler                  func(ctx context.Context, args json.RawMessage) (any, error)
}

// ---- schema helpers ---------------------------------------------------------

type obj = map[string]any

func schema(required []string, props obj) obj {
	s := obj{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) obj     { return obj{"type": "string", "description": desc} }
func boolean(desc string) obj { return obj{"type": "boolean", "description": desc} }

var targetsSchema = obj{
	"type":        "array",
	"description": "Build targets. Omit to use the project's own targets.",
	"items": obj{
		"type": "object",
		"properties": obj{
			"os":     obj{"type": "string", "enum": []string{"darwin", "windows", "linux"}},
			"arch":   obj{"type": "string", "enum": []string{"amd64", "arm64", "universal"}},
			"format": obj{"type": "string", "enum": []string{"app", "portable", "dmg", "zip", "deb", "appimage", "exe"}},
		},
		"required": []string{"os", "arch"},
	},
}

func annotations(readOnly, destructive, idempotent bool) obj {
	return obj{"readOnlyHint": readOnly, "destructiveHint": destructive, "idempotentHint": idempotent, "openWorldHint": false}
}

// decode strictly decodes tool arguments.
func decode(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return argError(err.Error())
	}
	return nil
}

// ---- tool catalogue -----------------------------------------------------------

func buildTools(s *Server) []tool {
	return []tool{
		{
			Name: "project_list", Title: "List projects",
			Description: "List the user's mypack projects (apps) with id, name, source, revision and draft status.",
			InputSchema: schema(nil, obj{}), Annotations: annotations(true, false, true),
			Handler: s.projectList,
		},
		{
			Name: "project_get", Title: "Get project",
			Description: "Get one project's full configuration (mypack.json) and current revision. Pass this revision to project_update.",
			InputSchema: schema([]string{"id"}, obj{"id": str("Project ID, e.g. prj_ab12cd34")}),
			Annotations: annotations(true, false, true), Handler: s.projectGet,
		},
		{
			Name: "project_create", Title: "Create draft project",
			Description: "Create a draft project from a website address. Fetches the site's title and icon. Nothing is built or installed. Only http(s) websites are accepted.",
			InputSchema: schema([]string{"url"}, obj{
				"url":  str("Website address, e.g. github.com or https://github.com"),
				"name": str("Optional app name; defaults to the site's name."),
			}),
			Annotations: annotations(false, false, false), Handler: s.projectCreate,
		},
		{
			Name: "project_update", Title: "Update project",
			Description: "Modify a project's configuration with a JSON merge patch (RFC 7386) applied to the mypack.json config. " +
				"Requires the project's current revision (optimistic concurrency). Returns a summary of what changed. " +
				"Use dryRun to validate and preview without saving. App identity (identifier, profileId), local file sources, " +
				"injected scripts, proxy and certificate settings cannot be changed by agents; the user must edit them in mypack.",
			InputSchema: schema([]string{"id", "revision", "patch"}, obj{
				"id":       str("Project ID"),
				"revision": obj{"type": "integer", "description": "Revision the patch was based on (from project_get)."},
				"patch":    obj{"type": "object", "description": "JSON merge patch over the config, e.g. {\"window\":{\"width\":1400}}. null deletes a key."},
				"dryRun":   boolean("Validate and return the diff without saving."),
			}),
			Annotations: annotations(false, false, false), Handler: s.projectUpdate,
		},
		{
			Name: "project_validate", Title: "Validate project",
			Description: "Validate a stored project (id) or a candidate config object. Returns field-level issues.",
			InputSchema: schema(nil, obj{
				"id":     str("Project ID to validate."),
				"config": obj{"type": "object", "description": "A mypack.json config object to validate instead."},
			}),
			Annotations: annotations(true, false, true), Handler: s.projectValidate,
		},
		{
			Name: "runtime_status", Title: "Runtime status",
			Description: "List available and cached runtimes (the prebuilt binaries apps are assembled from) and whether each is downloaded and verified.",
			InputSchema: schema(nil, obj{}), Annotations: annotations(true, false, true),
			Handler: s.runtimeStatus,
		},
		{
			Name: "build_preflight", Title: "Build preflight",
			Description: "Check what a build needs without side effects: runtime cache status, download size, target support and output location.",
			InputSchema: schema([]string{"projectId"}, obj{"projectId": str("Project ID"), "targets": targetsSchema}),
			Annotations: annotations(true, false, true), Handler: s.buildPreflight,
		},
		{
			Name: "build_start", Title: "Start build",
			Description: "Queue a build of the project's current configuration. Returns a job ID; poll build_status. " +
				"If a runtime must be downloaded, the job waits for the user's approval in the MyPack app and the result status is approval_required. " +
				"For a JavaScript project app (source.build set), mypack must first run the project's own install/build script; " +
				"this is NEVER run on an agent's request: the result is approval_required with job.approval.command (the exact command) " +
				"and the user must approve it in the MyPack app or with `mypack build <project> --run-build`. Then poll build_status.",
			InputSchema: schema([]string{"projectId"}, obj{"projectId": str("Project ID"), "targets": targetsSchema}),
			Annotations: annotations(false, false, false), Handler: s.buildStart,
		},
		{
			Name: "build_status", Title: "Build status",
			Description: "Get a build job's status, stage, progress, error, artifacts and the tail of its log.",
			InputSchema: schema([]string{"jobId"}, obj{
				"jobId":   str("Job ID, e.g. job_ab12cd34"),
				"logTail": obj{"type": "integer", "description": "Number of trailing log lines to include (default 20, max 200)."},
			}),
			Annotations: annotations(true, false, true), Handler: s.buildStatus,
		},
		{
			Name: "build_cancel", Title: "Cancel build",
			Description: "Cancel a queued, waiting or running build. Previous build results are left untouched.",
			InputSchema: schema([]string{"jobId"}, obj{"jobId": str("Job ID")}),
			Annotations: annotations(false, false, true), Handler: s.buildCancel,
		},
		{
			Name: "artifact_list", Title: "List artifacts",
			Description: "List built artifacts (path, target, size, sha256, signature) of a project, newest first.",
			InputSchema: schema([]string{"projectId"}, obj{"projectId": str("Project ID")}),
			Annotations: annotations(true, false, true), Handler: s.artifactList,
		},
	}
}

// ---- projects -----------------------------------------------------------------

type projectSummary struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Source    string     `json:"source"`
	Revision  int64      `json:"revision"`
	Draft     bool       `json:"draft"`
	Origin    string     `json:"origin"`
	UpdatedAt time.Time  `json:"updatedAt"`
	LastBuild *time.Time `json:"lastBuild,omitempty"`
}

func summarize(p domain.Project) projectSummary {
	return projectSummary{ID: p.ID, Name: p.Config.App.Name, Source: p.Config.App.Source.Value,
		Revision: p.Revision, Draft: p.Draft, Origin: p.Origin, UpdatedAt: p.UpdatedAt, LastBuild: p.LastBuild}
}

func (s *Server) projectList(ctx context.Context, _ json.RawMessage) (any, error) {
	ps, err := s.svc.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]projectSummary, 0, len(ps))
	for _, p := range ps {
		out = append(out, summarize(p))
	}
	return obj{"projects": out}, nil
}

type idArgs struct {
	ID string `json:"id"`
}

func (s *Server) projectGet(ctx context.Context, raw json.RawMessage) (any, error) {
	var a idArgs
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if a.ID == "" {
		return nil, argError("id is required")
	}
	p, err := s.svc.GetProject(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	return obj{"project": p}, nil
}

func (s *Server) projectCreate(ctx context.Context, raw json.RawMessage) (any, error) {
	var a struct {
		URL  string `json:"url"`
		Name string `json:"name"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	// Agents may only create website projects: never local paths.
	u, err := config.NormalizeURL(a.URL)
	if err != nil {
		return nil, &toolError{Code: "INVALID_INPUT", Message: err.Error(), Field: "url",
			Remediation: "Pass an http(s) website address such as https://example.com."}
	}
	p, err := s.svc.CreateProject(ctx, appservice.CreateRequest{Input: u, Name: a.Name, Actor: s.actor()})
	if err != nil {
		return nil, err
	}
	return obj{"project": p, "note": "Draft created. Nothing was built or installed."}, nil
}

// Paths (dotted, JSON names) that agents may not change; they affect
// identity, local file access, script injection or transport security.
var protectedPrefixes = []string{
	"app.identifier", "app.profileId",
	"app.source.type", "app.source.entry", "app.source.project", "app.source.build",
	"app.icon.path", "app.icon.source",
	"webview.injections", "webview.ignoreCertificateErrors", "webview.devTools",
	"network", "native.trayIcon",
}

func isProtected(path string) bool {
	for _, p := range protectedPrefixes {
		if path == p || strings.HasPrefix(path, p+".") || strings.HasPrefix(path, p+"[") {
			return true
		}
	}
	return false
}

func (s *Server) projectUpdate(ctx context.Context, raw json.RawMessage) (any, error) {
	var a struct {
		ID       string          `json:"id"`
		Revision int64           `json:"revision"`
		Patch    json.RawMessage `json:"patch"`
		DryRun   bool            `json:"dryRun"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if a.ID == "" || a.Revision <= 0 || len(a.Patch) == 0 {
		return nil, argError("id, revision and patch are required")
	}
	p, err := s.svc.GetProject(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	if p.Revision != a.Revision {
		return nil, &toolError{Code: "REVISION_CONFLICT",
			Message:     fmt.Sprintf("The project is at revision %d, not %d: it changed since you read it.", p.Revision, a.Revision),
			Remediation: "Call project_get, review the current configuration and re-apply your change."}
	}
	next, err := applyPatch(p.Config, a.Patch)
	if err != nil {
		return nil, err
	}
	changes, err := diffConfigs(p.Config, next)
	if err != nil {
		return nil, err
	}
	for _, c := range changes {
		if isProtected(c.Path) {
			return nil, &toolError{Code: "APPROVAL_REQUIRED", Field: c.Path,
				Message:     fmt.Sprintf("%s is security-sensitive and cannot be changed by an agent.", c.Path),
				Remediation: "Ask the user to change it in the MyPack app."}
		}
	}
	if len(changes) == 0 {
		return obj{"project": summarize(p), "changes": []change{}, "note": "The patch changed nothing."}, nil
	}
	if a.DryRun {
		cand := next.Clone()
		cand.ApplyDefaults(s.svc.HostTarget())
		res := obj{"dryRun": true, "changes": changes, "valid": true}
		if verr := cand.Validate(); verr != nil {
			res["valid"] = false
			var ve *config.ValidationError
			if asValidation(verr, &ve) {
				res["issues"] = ve.Issues
			}
		}
		return res, nil
	}
	up, err := s.svc.UpdateProject(ctx, appservice.UpdateRequest{
		ID: a.ID, ExpectedRevision: a.Revision, Config: next, Actor: s.actor(),
		// ConfirmIdentityChange stays false: agents never change identity.
	})
	if err != nil {
		return nil, err
	}
	return obj{"project": summarize(up), "changes": changes, "revision": up.Revision}, nil
}

func asValidation(err error, target **config.ValidationError) bool {
	ve, ok := err.(*config.ValidationError)
	if ok {
		*target = ve
	}
	return ok
}

// applyPatch applies a JSON merge patch to cfg and strictly decodes the result.
func applyPatch(cfg *config.Config, patch json.RawMessage) (*config.Config, error) {
	var pv any
	if err := json.Unmarshal(patch, &pv); err != nil {
		return nil, argError("patch is not valid JSON: " + err.Error())
	}
	if _, ok := pv.(map[string]any); !ok {
		return nil, argError("patch must be a JSON object")
	}
	base, _ := json.Marshal(cfg)
	var bv any
	_ = json.Unmarshal(base, &bv)
	merged, _ := json.Marshal(mergePatch(bv, pv))
	var out config.Config
	dec := json.NewDecoder(bytes.NewReader(merged))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return nil, &toolError{Code: "INVALID_CONFIG", Message: "The patched configuration is not valid: " + err.Error(),
			Remediation: "Use only fields from the mypack.json schema (see project_get)."}
	}
	return &out, nil
}

// mergePatch implements RFC 7386.
func mergePatch(target, patch any) any {
	pm, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	tm, ok := target.(map[string]any)
	if !ok {
		tm = map[string]any{}
	}
	for k, v := range pm {
		if v == nil {
			delete(tm, k)
			continue
		}
		tm[k] = mergePatch(tm[k], v)
	}
	return tm
}

type change struct {
	Path   string `json:"path"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

// diffConfigs lists leaf-level differences between two configs. Arrays are
// compared as whole values.
func diffConfigs(a, b *config.Config) ([]change, error) {
	fa, err := flatten(a)
	if err != nil {
		return nil, err
	}
	fb, err := flatten(b)
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for k := range fa {
		keys[k] = true
	}
	for k := range fb {
		keys[k] = true
	}
	var paths []string
	for k := range keys {
		paths = append(paths, k)
	}
	sort.Strings(paths)
	var out []change
	for _, k := range paths {
		av, aok := fa[k]
		bv, bok := fb[k]
		if aok && bok && mustJSON(av) == mustJSON(bv) {
			continue
		}
		c := change{Path: k}
		if aok {
			c.Before = av
		}
		if bok {
			c.After = bv
		}
		out = append(out, c)
	}
	return out, nil
}

func flatten(c *config.Config) (map[string]any, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	out := map[string]any{}
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		if m, ok := v.(map[string]any); ok {
			for k, e := range m {
				p := k
				if prefix != "" {
					p = prefix + "." + k
				}
				walk(p, e)
			}
			return
		}
		out[prefix] = v
	}
	walk("", v)
	return out, nil
}

func (s *Server) projectValidate(ctx context.Context, raw json.RawMessage) (any, error) {
	var a struct {
		ID     string          `json:"id"`
		Config json.RawMessage `json:"config"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	var cfg *config.Config
	switch {
	case a.ID != "" && len(a.Config) == 0:
		p, err := s.svc.GetProject(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		cfg = p.Config.Clone()
	case a.ID == "" && len(a.Config) > 0:
		c, err := config.Parse(a.Config)
		if err != nil {
			return validationResult(err)
		}
		cfg = c
		cfg.ApplyDefaults(s.svc.HostTarget())
	default:
		return nil, argError("pass exactly one of id or config")
	}
	if err := cfg.Validate(); err != nil {
		return validationResult(err)
	}
	return obj{"valid": true, "issues": []config.Issue{}}, nil
}

func validationResult(err error) (any, error) {
	if ve, ok := err.(*config.ValidationError); ok {
		return obj{"valid": false, "issues": ve.Issues}, nil
	}
	return nil, err
}

// ---- runtimes and builds ----------------------------------------------------------

func (s *Server) runtimeStatus(ctx context.Context, _ json.RawMessage) (any, error) {
	rs, err := s.svc.ListRuntimes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rs {
		rs[i].Path = "" // do not leak local paths
	}
	return obj{"runtimes": rs, "host": s.svc.HostTarget()}, nil
}

type buildArgs struct {
	ProjectID string          `json:"projectId"`
	Targets   []config.Target `json:"targets"`
}

func (a *buildArgs) normalize(host config.Target) error {
	if a.ProjectID == "" {
		return argError("projectId is required")
	}
	for i, t := range a.Targets {
		if t.Format == "" {
			a.Targets[i].Format = config.DefaultTarget(config.Target{OS: t.OS, Arch: t.Arch}).Format
		}
	}
	return nil
}

func (s *Server) buildPreflight(ctx context.Context, raw json.RawMessage) (any, error) {
	var a buildArgs
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if err := a.normalize(s.svc.HostTarget()); err != nil {
		return nil, err
	}
	pf, err := s.svc.Preflight(ctx, a.ProjectID, a.Targets)
	if err != nil {
		return nil, err
	}
	for i := range pf.Targets {
		pf.Targets[i].Runtime.Path = ""
	}
	return obj{"preflight": pf, "needsApproval": pf.DownloadBytes > 0}, nil
}

// jobView is a job without its (large) config snapshot.
func jobView(j domain.BuildJob) map[string]any {
	b, _ := json.Marshal(j)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	delete(m, "snapshot")
	return m
}

func (s *Server) buildStart(ctx context.Context, raw json.RawMessage) (any, error) {
	var a buildArgs
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if err := a.normalize(s.svc.HostTarget()); err != nil {
		return nil, err
	}
	job, err := s.svc.StartBuild(ctx, appservice.BuildRequest{
		ProjectID: a.ProjectID, Targets: a.Targets, Actor: s.actor(),
		AllowDownload: false, // an agent can never approve a download (spec §12.5)
	})
	if err != nil {
		return nil, err
	}
	if job.Status == domain.JobAwaitingApproval {
		msg := "approval_required: open mypack to approve the runtime download, then poll build_status."
		if job.Error != nil {
			msg = "approval_required: " + job.Error.Message + " Open mypack to approve it, then poll build_status."
		}
		if job.Approval != nil && job.Approval.Kind == domain.ApprovalProjectBuild {
			msg = "approval_required: the project must be built first by running `" + job.Approval.Command + "` in " +
				job.Approval.Path + ". This cannot be approved by an agent: ask the user to approve it in the MyPack app " +
				"(or run `mypack build " + job.ProjectID + " --run-build`), then poll build_status."
		}
		return obj{"status": "approval_required", "message": msg, "jobId": job.ID, "job": jobView(job)}, nil
	}
	return obj{"status": string(job.Status), "jobId": job.ID, "job": jobView(job)}, nil
}

func (s *Server) buildStatus(ctx context.Context, raw json.RawMessage) (any, error) {
	var a struct {
		JobID   string `json:"jobId"`
		LogTail int    `json:"logTail"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if a.JobID == "" {
		return nil, argError("jobId is required")
	}
	if a.LogTail <= 0 {
		a.LogTail = 20
	}
	if a.LogTail > 200 {
		a.LogTail = 200
	}
	job, err := s.svc.GetJob(ctx, a.JobID)
	if err != nil {
		return nil, err
	}
	lines, err := s.svc.JobLog(ctx, a.JobID)
	if err != nil {
		return nil, err
	}
	if len(lines) > a.LogTail {
		lines = lines[len(lines)-a.LogTail:]
	}
	res := obj{"job": jobView(job), "log": lines}
	if job.Status == domain.JobAwaitingApproval {
		res["status"] = "approval_required"
	} else {
		res["status"] = string(job.Status)
	}
	return res, nil
}

func (s *Server) buildCancel(ctx context.Context, raw json.RawMessage) (any, error) {
	var a struct {
		JobID string `json:"jobId"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if a.JobID == "" {
		return nil, argError("jobId is required")
	}
	if err := s.svc.CancelBuild(ctx, a.JobID, s.actor()); err != nil {
		return nil, err
	}
	return obj{"jobId": a.JobID, "cancelRequested": true}, nil
}

func (s *Server) artifactList(ctx context.Context, raw json.RawMessage) (any, error) {
	var a struct {
		ProjectID string `json:"projectId"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if a.ProjectID == "" {
		return nil, argError("projectId is required")
	}
	if _, err := s.svc.GetProject(ctx, a.ProjectID); err != nil {
		return nil, err
	}
	jobs, err := s.svc.ListJobs(ctx, a.ProjectID, 50)
	if err != nil {
		return nil, err
	}
	arts := []domain.Artifact{}
	for _, j := range jobs { // ListJobs is newest first
		arts = append(arts, j.Artifacts...)
	}
	return obj{"artifacts": arts}, nil
}
