package appservice

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"mypack.dev/mypack/internal/builder"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/storage"
	"mypack.dev/mypack/pkg/config"
)

// Throttles for job progress traffic (spec §14: ≤10 events/s per job).
const (
	progressEmitEvery    = 100 * time.Millisecond
	progressPersistEvery = time.Second
	logFlushEvery        = 500 * time.Millisecond
	logFlushLines        = 50
)

// runningJob is the in-process state of a job this Impl is executing.
type runningJob struct {
	cancel context.CancelFunc
	st     *jobState
}

// jobState is the live copy of a job plus its throttling bookkeeping.
type jobState struct {
	mu          sync.Mutex
	job         domain.BuildJob
	lastEmit    time.Time
	lastPersist time.Time
	logBuf      []domain.LogLine
	// flushMu holds a flush until its lines are stored, so a reader that
	// flushes (JobLog) never sees them neither buffered nor stored.
	flushMu sync.Mutex
}

func (j *jobState) snapshot() domain.BuildJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.job
}

// ---- preflight -------------------------------------------------------------

// Preflight implements Service.
func (s *Impl) Preflight(ctx context.Context, projectID string, targets []config.Target) (Preflight, error) {
	p, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return Preflight{}, err
	}
	if len(targets) == 0 {
		targets = p.Config.Build.Targets
	}
	set, err := s.Settings(ctx)
	if err != nil {
		return Preflight{}, err
	}
	return s.preflight(ctx, p.Config, targets, set.OutputDir)
}

func (s *Impl) preflight(ctx context.Context, cfg *config.Config, targets []config.Target, outputDir string) (Preflight, error) {
	s.syncRegistry(ctx)
	runtimeVersion := cfg.Runtime.Version
	pf := Preflight{OutputDir: outputDir, RuntimeVersion: runtimeVersion}
	seen := map[string]bool{}
	for _, t := range targets {
		capab := s.caps(s.host, t)
		tp := TargetPreflight{Target: t, Supported: capab.Supported, Level: capab.Level,
			Signing: capab.Signing, Reason: capab.Reason}
		if capab.Supported {
			key := builder.RuntimeKey(ctx, s.runtimes, cfg, t) // full or Lite, as the build will
			info, err := s.runtimes.Status(ctx, runtimeVersion, key)
			if err != nil {
				return pf, err
			}
			tp.Runtime = info
			if !info.Cached && !seen[key] {
				pf.DownloadBytes += info.Size
			}
			seen[key] = true
		}
		pf.Targets = append(pf.Targets, tp)
	}
	return pf, nil
}

// ---- starting jobs ---------------------------------------------------------

// StartBuild implements Service.
func (s *Impl) StartBuild(ctx context.Context, req BuildRequest) (job domain.BuildJob, err error) {
	actor := actorOr(req.Actor, "gui")
	defer func() { s.audit(ctx, actor, "build.start", req.ProjectID, err) }()

	p, err := s.store.GetProject(ctx, req.ProjectID)
	if err != nil {
		return job, err
	}
	if p.TrashedAt != nil {
		return job, newErr(CodeNotFound, "The project is in the Trash.", "Restore the project before building it.")
	}
	targets := req.Targets
	if len(targets) == 0 {
		targets = p.Config.Build.Targets
	}
	snap := p.Config.Clone()
	s.fillGroupIcons(ctx, p.ID, snap)
	snap.Build.Targets = targets
	if verr := snap.Validate(); verr != nil {
		return job, asDomainErr(verr, CodeInvalidConfig)
	}
	set, err := s.Settings(ctx)
	if err != nil {
		return job, err
	}
	out := firstNonEmpty(req.OutputDir, set.OutputDir)
	return s.submit(ctx, submission{
		projectID: p.ID, projectRev: p.Revision, snapshot: snap, outputDir: out,
		allowDownload: req.AllowDownload, allowProjectBuild: req.AllowProjectBuild && !isMCPActor(actor),
		actor: actor, kind: "build",
	})
}

type submission struct {
	projectID     string
	projectRev    int64
	snapshot      *config.Config
	outputDir     string
	allowDownload bool
	// allowProjectBuild records approval of the project build up front
	// (CLI --run-build). Never honoured for MCP actors.
	allowProjectBuild bool
	actor             string
	kind              string // build | update | repair
	installID         string
}

// submit runs preflight, persists the job and either queues it or parks it
// awaiting approval when a runtime download is needed but not allowed.
func (s *Impl) submit(ctx context.Context, sub submission) (domain.BuildJob, error) {
	snap := sub.snapshot
	pf, err := s.preflight(ctx, snap, snap.Build.Targets, sub.outputDir)
	if err != nil {
		return domain.BuildJob{}, err
	}
	for _, t := range pf.Targets {
		if !t.Supported {
			return domain.BuildJob{}, &domain.Error{
				Code:        CodeTargetUnsupp,
				Message:     fmt.Sprintf("Cannot build %s on this computer.", t.Target.Key()),
				Cause:       t.Reason,
				Remediation: "Choose a target this computer can build, or build on a matching machine.",
				Field:       "build.targets",
			}
		}
	}
	job := domain.BuildJob{
		ProjectID: sub.projectID, ProjectRev: sub.projectRev, Snapshot: snap,
		ConfigHash: configHash(snap), Targets: snap.Build.Targets, RuntimeVersion: snap.Runtime.Version,
		Status: domain.JobQueued, Stage: domain.StageQueued, Progress: -1, Actor: sub.actor,
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	meta := storage.JobMeta{Kind: sub.kind, OutputDir: sub.outputDir, AllowDownload: sub.allowDownload,
		InstallationID: sub.installID}

	if sub.allowProjectBuild {
		if p := s.planProjectBuild(ctx, sub.projectID, snap); p != nil {
			if err := s.recordProjectApproval(ctx, sub.projectID, p.Fingerprint); err != nil {
				return domain.BuildJob{}, err
			}
		}
	}
	pending, pendingBytes := s.pendingApproval(ctx, sub.projectID, snap, pf, sub.allowDownload)
	needsApproval := pending != nil
	if needsApproval {
		job.Status = domain.JobAwaitingApproval
		job.Error = pending
	}
	job, err = s.store.InsertJob(ctx, job, meta)
	if err != nil {
		return job, err
	}
	s.emit(Event{Kind: EventJobChanged, ProjectID: job.ProjectID, JobID: job.ID, Job: &job})
	if needsApproval {
		s.emit(Event{Kind: EventApprovalNeeded, ProjectID: job.ProjectID, JobID: job.ID, Job: &job, Total: pendingBytes})
		return s.withApproval(job), nil
	}
	s.launch(job)
	return job, nil
}

func targetKeys(ts []config.Target) string {
	out := ""
	for i, t := range ts {
		if i > 0 {
			out += ", "
		}
		out += t.Key()
	}
	return out
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// ApproveJob implements Service.
func (s *Impl) ApproveJob(ctx context.Context, jobID, actor string) (err error) {
	defer func() { s.audit(ctx, actor, "job.approve", jobID, err) }()
	if isMCPActor(actor) {
		return newErr(CodeApprovalRequired, "An agent cannot approve this build.",
			"Approve it in the MyPack app or with the mypack CLI.")
	}
	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status != domain.JobAwaitingApproval {
		return newErr("INVALID_STATE", "This build is not waiting for approval.", "Start a new build instead.")
	}
	meta, err := s.store.GetJobMeta(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Error != nil && job.Error.Code == CodeProjectBuildRequired {
		p := s.planProjectBuild(ctx, job.ProjectID, job.Snapshot)
		if p == nil {
			return newErr("INVALID_STATE", "This build no longer needs approval.", "Start a new build instead.")
		}
		if err := s.recordProjectApproval(ctx, job.ProjectID, p.Fingerprint); err != nil {
			return err
		}
	} else {
		meta.AllowDownload = true
		if err := s.store.SetJobMeta(ctx, jobID, meta); err != nil {
			return err
		}
	}
	// Another approval (runtime download) may still be outstanding.
	if pf, perr := s.preflight(ctx, job.Snapshot, job.Snapshot.Build.Targets, meta.OutputDir); perr == nil {
		if pending, total := s.pendingApproval(ctx, job.ProjectID, job.Snapshot, pf, meta.AllowDownload); pending != nil {
			job.Error = pending
			if err := s.store.SaveJobState(ctx, job); err != nil {
				return err
			}
			s.emit(Event{Kind: EventJobChanged, ProjectID: job.ProjectID, JobID: job.ID, Job: &job})
			s.emit(Event{Kind: EventApprovalNeeded, ProjectID: job.ProjectID, JobID: job.ID, Job: &job, Total: total})
			return nil
		}
	}
	job.Status, job.Stage, job.Progress, job.Error = domain.JobQueued, domain.StageQueued, -1, nil
	if err := s.store.SaveJobState(ctx, job); err != nil {
		return err
	}
	s.emit(Event{Kind: EventJobChanged, ProjectID: job.ProjectID, JobID: job.ID, Job: &job})
	s.launch(job)
	return nil
}

// CancelBuild implements Service.
func (s *Impl) CancelBuild(ctx context.Context, jobID, actor string) (err error) {
	defer func() { s.audit(ctx, actor, "build.cancel", jobID, err) }()
	s.mu.Lock()
	r := s.running[jobID]
	s.mu.Unlock()
	if r != nil {
		r.cancel()
		return nil
	}
	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	switch job.Status {
	case domain.JobQueued, domain.JobAwaitingApproval:
		// Not executing here: flip it so no process starts it (runners
		// re-check the status once they obtain a worker slot).
		now := time.Now().UTC()
		job.Status, job.FinishedAt = domain.JobCancelled, &now
		job.Error = newErr(CodeCancelled, "The build was cancelled.", "")
		if err := s.store.SaveJobState(ctx, job); err != nil {
			return err
		}
		s.emit(Event{Kind: EventJobChanged, ProjectID: job.ProjectID, JobID: job.ID, Job: &job})
		return nil
	case domain.JobRunning:
		return newErr("JOB_NOT_LOCAL", "This build is running in another mypack window or process.",
			"Cancel it from the process that started it.")
	}
	return newErr("INVALID_STATE", "The build has already finished.", "")
}

// GetJob implements Service. Jobs running in this process return live state.
func (s *Impl) GetJob(ctx context.Context, jobID string) (domain.BuildJob, error) {
	s.mu.Lock()
	r := s.running[jobID]
	s.mu.Unlock()
	if r != nil {
		return s.withApproval(r.st.snapshot()), nil
	}
	j, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		return j, err
	}
	return s.withApproval(j), nil
}

// JobLog implements Service.
func (s *Impl) JobLog(ctx context.Context, jobID string) ([]domain.LogLine, error) {
	s.mu.Lock()
	r := s.running[jobID]
	s.mu.Unlock()
	if r != nil {
		s.flushLogs(r.st)
	}
	return s.store.JobLog(ctx, jobID)
}

// ListJobs implements Service.
func (s *Impl) ListJobs(ctx context.Context, projectID string, limit int) ([]domain.BuildJob, error) {
	jobs, err := s.store.ListJobs(ctx, projectID, limit)
	for i := range jobs {
		jobs[i] = s.withApproval(jobs[i])
	}
	return jobs, err
}

// ---- execution ---------------------------------------------------------------

// launch registers a cancel func synchronously (so CancelBuild works the
// instant StartBuild returns) and starts the worker goroutine.
func (s *Impl) launch(job domain.BuildJob) {
	ctx, cancel := context.WithCancel(s.ctx)
	st := &jobState{job: job}
	s.mu.Lock()
	s.running[job.ID] = &runningJob{cancel: cancel, st: st}
	s.mu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			cancel()
			s.mu.Lock()
			delete(s.running, job.ID)
			s.mu.Unlock()
		}()
		s.runJob(ctx, st)
	}()
}

// setState mutates the live job under lock, persists it and emits an event.
func (s *Impl) setState(st *jobState, mutate func(j *domain.BuildJob)) domain.BuildJob {
	st.mu.Lock()
	mutate(&st.job)
	j := st.job
	st.lastEmit, st.lastPersist = time.Now(), time.Now()
	st.mu.Unlock()
	if err := s.store.SaveJobState(context.WithoutCancel(s.ctx), j); err != nil {
		_ = err // the DB is gone (shutdown); nothing useful to do
	}
	s.emit(Event{Kind: EventJobChanged, ProjectID: j.ProjectID, JobID: j.ID, Job: &j})
	return j
}

func (s *Impl) runJob(ctx context.Context, st *jobState) {
	id := st.snapshot().ID
	bg := context.WithoutCancel(ctx)

	// Wait for a worker slot (the job is "queued" meanwhile and cancellable).
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		s.finishCancelled(st, nil)
		return
	}
	// Someone (another process) may have cancelled it while it waited.
	cur, err := s.store.GetJob(bg, id)
	if err != nil || cur.Status != domain.JobQueued {
		return
	}
	meta, err := s.store.GetJobMeta(bg, id)
	if err != nil {
		s.finishFailed(st, nil, err)
		return
	}
	var icon []byte
	if p, perr := s.store.GetProject(bg, cur.ProjectID); perr == nil {
		icon = s.iconBase(p) // the Builder draws app.icon.style over it
	}

	now := time.Now().UTC()
	s.setState(st, func(j *domain.BuildJob) {
		j.Status, j.Stage, j.Progress, j.StartedAt, j.Error = domain.JobRunning, domain.StageValidating, -1, &now, nil
		if j.Snapshot != nil && j.Snapshot.App.Source.Build != nil {
			j.Stage = domain.StagePreparing
		}
	})

	// Periodic log flusher.
	stopFlush := make(chan struct{})
	go func() {
		t := time.NewTicker(logFlushEvery)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.flushLogs(st)
			case <-stopFlush:
				return
			}
		}
	}()
	defer close(stopFlush)
	defer func() { s.flushLogs(st) }()

	s.logLine(st, "info", fmt.Sprintf("Building %s for %s", cur.Snapshot.App.Name, targetKeys(cur.Targets)))

	if cur.Snapshot.App.Source.Build != nil {
		parked, perr := s.prepareProject(ctx, st, cur)
		if ctx.Err() != nil || errors.Is(perr, context.Canceled) {
			s.finishCancelled(st, nil)
			return
		}
		if perr != nil {
			s.finishFailed(st, nil, perr)
			return
		}
		if parked {
			return
		}
	}

	// The project's location and build recipe are local-only: they must not
	// be baked into the generated app's mypack.json.
	engineCfg := cur.Snapshot.Clone()
	engineCfg.App.Source.Project, engineCfg.App.Source.Build = "", nil

	arts, berr := s.engine.Build(ctx, builder.Request{
		Config: engineCfg, IconPNG: icon, Targets: cur.Targets, OutputDir: meta.OutputDir,
		AllowDownload: meta.AllowDownload,
		Stage: func(stage domain.Stage, progress float64) {
			s.onStage(st, stage, progress)
		},
		Log: func(l domain.LogLine) {
			if l.Time.IsZero() {
				l.Time = time.Now()
			}
			s.appendLog(st, l)
		},
	})

	if ctx.Err() != nil || errors.Is(berr, context.Canceled) {
		s.finishCancelled(st, nil)
		return
	}

	// Persist whatever succeeded, even when another target failed.
	var saved []domain.Artifact
	for _, a := range arts {
		a.JobID, a.ProjectID, a.ID = cur.ID, cur.ProjectID, ""
		a.CreatedAt = time.Now().UTC().Truncate(time.Millisecond)
		a, ierr := s.store.InsertArtifact(bg, a)
		if ierr != nil {
			s.finishFailed(st, saved, ierr)
			return
		}
		saved = append(saved, a)
	}
	if berr != nil {
		de := asDomainErr(berr, CodeBuildFailed)
		if de.Code == CodeDownloadRequired {
			// The runtime disappeared/changed between preflight and build:
			// fall back to asking for approval instead of failing.
			s.setState(st, func(j *domain.BuildJob) {
				j.Status, j.Stage, j.Progress, j.Error = domain.JobAwaitingApproval, domain.StageQueued, -1, de
			})
			j := st.snapshot()
			s.emit(Event{Kind: EventApprovalNeeded, ProjectID: j.ProjectID, JobID: j.ID, Job: &j})
			return
		}
		s.finishFailed(st, saved, de)
		return
	}

	if meta.Kind == "update" || meta.Kind == "repair" {
		s.onStage(st, domain.StagePublishing, -1)
		warns, perr := s.applyToInstallation(bg, cur, meta, saved)
		if len(warns) > 0 {
			s.setState(st, func(j *domain.BuildJob) { j.Warnings = append(j.Warnings, warns...) })
		}
		if perr != nil {
			s.finishFailed(st, saved, perr)
			return
		}
	}

	_ = s.store.MarkBuilt(bg, cur.ProjectID, time.Now())
	fin := time.Now().UTC()
	s.setState(st, func(j *domain.BuildJob) {
		j.Status, j.Stage, j.Progress, j.FinishedAt, j.Error, j.Artifacts = domain.JobSucceeded, domain.StageCompleted, 1, &fin, nil, saved
	})
	s.emit(Event{Kind: EventProjectChanged, ProjectID: cur.ProjectID})
	s.audit(bg, cur.Actor, "build.completed", cur.ID, nil)
}

func (s *Impl) onStage(st *jobState, stage domain.Stage, progress float64) {
	now := time.Now()
	st.mu.Lock()
	changed := st.job.Stage != stage
	st.job.Stage, st.job.Progress = stage, progress
	emit := changed || progress >= 1 || now.Sub(st.lastEmit) >= progressEmitEvery
	persist := changed || now.Sub(st.lastPersist) >= progressPersistEvery
	if emit {
		st.lastEmit = now
	}
	if persist {
		st.lastPersist = now
	}
	j := st.job
	st.mu.Unlock()
	if persist {
		_ = s.store.SaveJobState(context.WithoutCancel(s.ctx), j)
	}
	if emit {
		s.emit(Event{Kind: EventJobChanged, ProjectID: j.ProjectID, JobID: j.ID, Job: &j})
	}
}

func (s *Impl) logLine(st *jobState, level, msg string) {
	st.mu.Lock()
	stage := st.job.Stage
	st.mu.Unlock()
	s.appendLog(st, domain.LogLine{Time: time.Now(), Level: level, Stage: stage, Message: msg})
}

// appendLog buffers a line for batched persistence and publishes it at once.
func (s *Impl) appendLog(st *jobState, l domain.LogLine) {
	st.mu.Lock()
	st.logBuf = append(st.logBuf, l)
	flush := len(st.logBuf) >= logFlushLines
	id, pid := st.job.ID, st.job.ProjectID
	st.mu.Unlock()
	s.emit(Event{Kind: EventJobLog, JobID: id, ProjectID: pid, Log: &l})
	if flush {
		s.flushLogs(st)
	}
}

func (s *Impl) flushLogs(st *jobState) {
	st.flushMu.Lock()
	defer st.flushMu.Unlock()
	st.mu.Lock()
	lines := st.logBuf
	st.logBuf = nil
	id := st.job.ID
	st.mu.Unlock()
	if len(lines) > 0 {
		_ = s.store.AppendLogs(context.WithoutCancel(s.ctx), id, lines)
	}
}

func (s *Impl) finishFailed(st *jobState, arts []domain.Artifact, err error) {
	de := asDomainErr(err, CodeBuildFailed)
	s.logLine(st, "error", de.Message)
	fin := time.Now().UTC()
	j := s.setState(st, func(j *domain.BuildJob) {
		j.Status, j.Progress, j.FinishedAt, j.Error, j.Artifacts = domain.JobFailed, -1, &fin, de, arts
	})
	s.audit(context.WithoutCancel(s.ctx), j.Actor, "build.failed", j.ID, de)
}

func (s *Impl) finishCancelled(st *jobState, arts []domain.Artifact) {
	fin := time.Now().UTC()
	j := s.setState(st, func(j *domain.BuildJob) {
		j.Status, j.Progress, j.FinishedAt, j.Artifacts = domain.JobCancelled, -1, &fin, arts
		j.Error = newErr(CodeCancelled, "The build was cancelled.", "Previous build results were left untouched.")
	})
	s.logLine(st, "warn", "Build cancelled")
	s.flushLogs(st)
	s.audit(context.WithoutCancel(s.ctx), j.Actor, "build.cancelled", j.ID, nil)
}
