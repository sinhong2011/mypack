// Package builder assembles generated apps from a prebuilt runtime plus
// injected configuration (spec §6). Platform assemblers live in this package.
//
// Pipeline per target: validating → resolving → downloading → assembling →
// customizing → signing → packaging → verifying → publishing. All work happens
// in a staging directory below <OutputDir>/.staging; the final artifact is
// published with an atomic rename, replacing a previous artifact only after
// everything succeeded.
package builder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/internal/runtimemanager"
	"mypack.dev/mypack/pkg/config"
)

// Engine runs build pipelines. Safe for concurrent builds.
type Engine struct {
	Runtimes *runtimemanager.Manager
}

// Request is one build of an immutable config snapshot.
type Request struct {
	Config        *config.Config // snapshot; Engine must not mutate it
	IconPNG       []byte         // 1024 PNG; nil => icons.Default(name)
	Targets       []config.Target
	OutputDir     string // artifacts published to <OutputDir>/<Name>/<os-arch>/
	AllowDownload bool   // false + missing runtime => RUNTIME_DOWNLOAD_REQUIRED
	// Stage is called when the pipeline enters a stage; progress 0..1 or -1.
	Stage func(stage domain.Stage, progress float64)
	Log   func(line domain.LogLine)
}

// Build runs the pipeline for every target. Artifacts are returned without
// ID/JobID/ProjectID (the service fills those). A target failure does not
// hide other targets' successes: it returns the artifacts that succeeded
// plus a *domain.Error describing the first failure.
func (e *Engine) Build(ctx context.Context, req Request) ([]domain.Artifact, error) {
	var arts []domain.Artifact
	var first error
	if req.Config == nil {
		return nil, &domain.Error{Code: "INVALID_CONFIG", Message: "No configuration to build", Remediation: "Provide a project configuration."}
	}
	if len(req.Targets) == 0 {
		return nil, &domain.Error{Code: "INVALID_CONFIG", Field: "build.targets", Message: "At least one build target is required", Remediation: "Add a target such as macOS app or Windows portable."}
	}
	if req.OutputDir == "" {
		return nil, &domain.Error{Code: "INVALID_CONFIG", Field: "outputDir", Message: "No output directory given", Remediation: "Choose where to save the built app."}
	}
	for _, t := range req.Targets {
		if ctx.Err() != nil {
			if first == nil {
				first = cancelled()
			}
			break
		}
		a, err := e.buildTarget(ctx, req, t)
		if err != nil {
			if first == nil {
				first = err
			}
			if isCancelled(err) {
				break
			}
			continue
		}
		arts = append(arts, a)
	}
	if first == nil && req.Stage != nil {
		req.Stage(domain.StageCompleted, 1)
	}
	return arts, first
}

func cancelled() *domain.Error {
	return &domain.Error{Code: "BUILD_CANCELLED", Message: "The build was cancelled", Remediation: "Start the build again when you are ready."}
}

func isCancelled(err error) bool {
	var de *domain.Error
	return errors.As(err, &de) && de.Code == "BUILD_CANCELLED"
}

func berr(code, msg string, cause error, remediation string) *domain.Error {
	d := &domain.Error{Code: code, Message: msg, Remediation: remediation}
	if cause != nil {
		d.Cause = cause.Error()
	}
	return d
}

// job is the state of one target build.
type job struct {
	e      *Engine
	ctx    context.Context
	req    Request
	cfg    *config.Config
	target config.Target
	stage  domain.Stage

	name    string // filesystem-safe app name (bundle/dir/exe base)
	slug    string // lowercase slug for linux
	staging string // per-target staging dir
	runtime string // path of the runtime executable in the cache
	entry   runtimemanager.Entry
	hasEnt  bool
	iconPNG []byte
	iconCar bool // Contents/Resources/Assets.car holds an Icon Composer icon

	root     string // primary tree root: <Name>.app or <Name>/
	exePath  string
	resDir   string
	artifact string // path (inside staging) of the finished artifact
	signing  string // adhoc | unsigned
	warnings []string
	lastProg float64
}

func (e *Engine) buildTarget(ctx context.Context, req Request, target config.Target) (domain.Artifact, error) {
	cfg := req.Config.Clone()
	j := &job{e: e, ctx: ctx, req: req, cfg: cfg, target: target, signing: "unsigned"}
	j.name = config.ExecutableName(cfg.App.Name)
	j.slug = config.Slug(cfg.App.Name)
	if j.slug == "" {
		j.slug = "app"
	}
	art, err := j.execute()
	if err != nil {
		j.log("error", "Build failed for %s: %v", target.Key(), err)
		return domain.Artifact{}, err
	}
	return art, nil
}

func (j *job) log(level, format string, args ...any) {
	if j.req.Log == nil {
		return
	}
	j.req.Log(domain.LogLine{Time: time.Now().UTC(), Level: level, Stage: j.stage, Message: fmt.Sprintf(format, args...)})
}

func (j *job) warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	j.warnings = append(j.warnings, msg)
	j.log("warn", "%s", msg)
}

// enter starts a stage, honouring cancellation.
func (j *job) enter(s domain.Stage, progress float64) error {
	if err := j.check(); err != nil {
		return err
	}
	j.stage = s
	j.lastProg = progress
	if j.req.Stage != nil {
		j.req.Stage(s, progress)
	}
	return nil
}

func (j *job) progress(p float64) {
	if j.req.Stage != nil && (p-j.lastProg >= 0.01 || p >= 1) {
		j.lastProg = p
		j.req.Stage(j.stage, p)
	}
}

func (j *job) check() error {
	if j.ctx.Err() != nil {
		return cancelled()
	}
	return nil
}

// wrap converts errors raised after a cancellation into BUILD_CANCELLED.
func (j *job) wrap(err error, code, msg, remediation string) error {
	if err == nil {
		return nil
	}
	if j.ctx.Err() != nil {
		return cancelled()
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return de
	}
	var xe *domainErr
	if errors.As(err, &xe) {
		return xe.toDomain()
	}
	return berr(code, msg, err, remediation)
}

func (j *job) execute() (domain.Artifact, error) {
	// ---- validating
	if err := j.enter(domain.StageValidating, -1); err != nil {
		return domain.Artifact{}, err
	}
	if err := j.validate(); err != nil {
		return domain.Artifact{}, err
	}

	// ---- resolving
	if err := j.enter(domain.StageResolving, -1); err != nil {
		return domain.Artifact{}, err
	}
	if err := j.resolve(); err != nil {
		return domain.Artifact{}, err
	}

	// ---- downloading
	if err := j.enter(domain.StageDownloading, 0); err != nil {
		return domain.Artifact{}, err
	}
	if err := j.download(); err != nil {
		return domain.Artifact{}, err
	}

	// staging
	stagingBase := filepath.Join(j.req.OutputDir, ".staging")
	if err := os.MkdirAll(stagingBase, 0o755); err != nil {
		return domain.Artifact{}, berr("BUILD_FAILED", "Cannot create the output directory", err, "Check that "+j.req.OutputDir+" exists and is writable.")
	}
	st, err := os.MkdirTemp(stagingBase, j.name+"-"+j.target.Key()+"-")
	if err != nil {
		return domain.Artifact{}, berr("BUILD_FAILED", "Cannot create a staging directory", err, "Check free disk space and permissions of "+j.req.OutputDir+".")
	}
	j.staging = st
	defer func() {
		_ = os.RemoveAll(st)
		_ = os.Remove(stagingBase) // only if empty
	}()

	// ---- assembling
	if err := j.enter(domain.StageAssembling, -1); err != nil {
		return domain.Artifact{}, err
	}
	if err := j.assemble(); err != nil {
		return domain.Artifact{}, err
	}

	// ---- customizing
	if err := j.enter(domain.StageCustomizing, -1); err != nil {
		return domain.Artifact{}, err
	}
	if err := j.customize(); err != nil {
		return domain.Artifact{}, err
	}

	// ---- signing
	if err := j.enter(domain.StageSigning, -1); err != nil {
		return domain.Artifact{}, err
	}
	if err := j.sign(); err != nil {
		return domain.Artifact{}, err
	}

	// ---- packaging
	if err := j.enter(domain.StagePackaging, -1); err != nil {
		return domain.Artifact{}, err
	}
	if err := j.pack(); err != nil {
		return domain.Artifact{}, err
	}

	// ---- verifying
	if err := j.enter(domain.StageVerifying, -1); err != nil {
		return domain.Artifact{}, err
	}
	if err := j.verify(); err != nil {
		return domain.Artifact{}, err
	}
	sum, size, err := hashArtifact(j.ctx, j.artifact)
	if err != nil {
		return domain.Artifact{}, j.wrap(err, "BUILD_VERIFY_FAILED", "Cannot hash the built artifact", "Check free disk space and try again.")
	}

	// ---- publishing
	if err := j.enter(domain.StagePublishing, -1); err != nil {
		return domain.Artifact{}, err
	}
	final, err := j.publish()
	if err != nil {
		return domain.Artifact{}, err
	}
	j.log("info", "Built %s", final)
	return domain.Artifact{Target: j.target, Path: final, Size: size, SHA256: sum, Signature: j.signing, CreatedAt: time.Now().UTC()}, nil
}

// validate checks config and target support.
func (j *job) validate() error {
	c := j.cfg
	if len(c.Build.Targets) == 0 {
		c.Build.Targets = j.req.Targets
	}
	if err := c.Validate(); err != nil {
		var ve *config.ValidationError
		if errors.As(err, &ve) && len(ve.Issues) > 0 {
			return &domain.Error{Code: "INVALID_CONFIG", Message: "The app configuration is invalid", Cause: err.Error(),
				Field: ve.Issues[0].Field, Remediation: "Fix the highlighted setting: " + ve.Issues[0].Message}
		}
		return berr("INVALID_CONFIG", "The app configuration is invalid", err, "Fix the configuration and build again.")
	}
	capb := Capabilities(HostTarget(), j.target)
	if !capb.Supported {
		return &domain.Error{Code: "TARGET_UNSUPPORTED", Message: fmt.Sprintf("Cannot build %s/%s as %s on this computer", j.target.OS, j.target.Arch, j.target.Format),
			Cause: capb.Reason, Remediation: "Choose a supported target, or build on a computer that supports it."}
	}
	if capb.Reason != "" {
		j.warn("%s", capb.Reason)
	}
	// icon
	icon := j.req.IconPNG
	if len(icon) == 0 {
		icon = icons.Default(c.App.Name)
	} else if !is1024PNG(icon) {
		n, err := icons.Normalize(icon)
		if err != nil {
			j.warn("The icon could not be read (%v); using the default icon", err)
			n = icons.Default(c.App.Name)
		}
		icon = n
	}
	// The icon studio's style is drawn over the icon's own image (or, with
	// no image, over the name's letter).
	if st := c.App.Icon.Style; st != nil {
		if st.Glyph == config.GlyphEmoji && !icons.EmojiAvailable() {
			j.warn("Emoji icons can only be drawn on a Mac; the app's initial is used instead")
		}
		styled, err := icons.Styled(j.req.IconPNG, *st, c.App.Name, icons.CanvasSize)
		if err != nil {
			j.warn("The icon style could not be drawn (%v); using the plain icon", err)
		} else {
			icon = styled
		}
	}
	j.iconPNG = icon
	return nil
}

func (j *job) resolve() error {
	if j.e == nil || j.e.Runtimes == nil {
		return berr("RUNTIME_NOT_FOUND", "No runtime manager is configured", nil, "Configure a runtime registry.")
	}
	version, key := j.cfg.Runtime.Version, j.target.Key()
	j.log("info", "Runtime %s for %s", version, key)
	if ent, ok := j.e.Runtimes.CachedEntry(version, key); ok {
		j.entry, j.hasEnt = ent, true
	} else if j.req.AllowDownload {
		ent, err := j.e.Runtimes.Entry(j.ctx, version, key)
		if err != nil {
			return j.wrap(err, "RUNTIME_NOT_FOUND", "The runtime is not available", "Pick another runtime version.")
		}
		j.entry, j.hasEnt = ent, true
	}
	if j.hasEnt {
		if missing := missingCapabilities(j.cfg, j.entry); len(missing) > 0 {
			return &domain.Error{Code: "RUNTIME_CAPABILITY_MISSING", Message: fmt.Sprintf("Runtime %s does not support: %v", version, missing),
				Remediation: "Turn those features off, or choose a newer runtime version."}
		}
	}
	return nil
}

func (j *job) download() error {
	version, key := j.cfg.Runtime.Version, j.target.Key()
	st, err := j.e.Runtimes.Status(j.ctx, version, key)
	cached := err == nil && st.Cached && st.Verified
	if !cached && !j.req.AllowDownload {
		return &domain.Error{Code: "RUNTIME_DOWNLOAD_REQUIRED", Message: fmt.Sprintf("Runtime %s for %s must be downloaded first", version, key),
			Remediation: "Allow the download (approve it in the app, or pass --allow-download)."}
	}
	if !cached {
		j.log("info", "Downloading runtime %s (%s)", version, key)
	}
	exe, err := j.e.Runtimes.Ensure(j.ctx, version, key, func(done, total int64) {
		if total > 0 {
			j.progress(float64(done) / float64(total))
		}
	})
	if err != nil {
		return j.wrap(err, "RUNTIME_DOWNLOAD_FAILED", "Could not get the runtime", "Check your connection and try again.")
	}
	j.progress(1)
	j.runtime = exe
	if !j.hasEnt {
		if ent, ok := j.e.Runtimes.CachedEntry(version, key); ok {
			j.entry, j.hasEnt = ent, true
			if missing := missingCapabilities(j.cfg, ent); len(missing) > 0 {
				return &domain.Error{Code: "RUNTIME_CAPABILITY_MISSING", Message: fmt.Sprintf("Runtime %s does not support: %v", version, missing),
					Remediation: "Turn those features off, or choose a newer runtime version."}
			}
		}
	}
	return nil
}

// publish atomically moves the artifact into <OutputDir>/<Name>/<os-arch>/.
func (j *job) publish() (string, error) {
	destDir := filepath.Join(j.req.OutputDir, j.name, j.target.Key())
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", berr("PUBLISH_FAILED", "Cannot create the output folder", err, "Check permissions of "+destDir+".")
	}
	final := filepath.Join(destDir, filepath.Base(j.artifact))
	if err := j.check(); err != nil {
		return "", err
	}
	prev := final + ".prev"
	hadPrev := false
	if _, err := os.Lstat(final); err == nil {
		_ = os.RemoveAll(prev)
		if err := os.Rename(final, prev); err != nil {
			return "", berr("PUBLISH_FAILED", "Cannot replace the previous build", err, "Close the previous app if it is running, then build again.")
		}
		hadPrev = true
	}
	if err := os.Rename(j.artifact, final); err != nil {
		if hadPrev {
			_ = os.RemoveAll(final)
			_ = os.Rename(prev, final)
		}
		return "", berr("PUBLISH_FAILED", "Cannot move the finished app into place", err, "Check free disk space and permissions of "+destDir+".")
	}
	if hadPrev {
		_ = os.RemoveAll(prev)
	}
	return final, nil
}
