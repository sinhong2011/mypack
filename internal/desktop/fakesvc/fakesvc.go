// Package fakesvc is an in-memory appservice.Service used for GUI demos,
// screenshots and automation tests (MYPACK_FAKE=1). It simulates metadata
// fetches, staged builds with measurable download progress, first-time
// runtime approval, installations, updates and failures — without touching
// the network or building anything.
//
// Deterministic hooks for demos:
//   - a URL whose host contains "offline" or ends in ".invalid" fails the
//     metadata fetch (Fallback=true);
//   - a project whose name or URL contains "fail" fails at the signing stage;
//   - MYPACK_FAKE_SEED=empty starts with an empty library;
//   - MYPACK_FAKE_FAST=1 shortens every simulated delay.
package fakesvc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/sampleicons"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/internal/licensing"
	"mypack.dev/mypack/pkg/config"
)

const runtimeVersion = config.DefaultRuntimeVersion

// Service implements appservice.Service in memory.
type Service struct {
	mu        sync.Mutex
	projects  map[string]*domain.Project
	jobs      []*domain.BuildJob
	installs  []*domain.Installation
	runtimes  map[string]*domain.RuntimeInfo // key "version/os-arch"
	settings  appservice.Settings
	activity  []domain.Activity
	cancels   map[string]context.CancelFunc
	approvals map[string]bool
	nextID    int
	host      config.Target
	speed     float64

	licenseToken string // verified token, "" = Free

	subMu sync.Mutex
	subs  map[int]chan appservice.Event
	subN  int
}

var _ appservice.Service = (*Service)(nil)

// New returns a seeded fake service.
func New() *Service {
	s := &Service{
		projects:  map[string]*domain.Project{},
		runtimes:  map[string]*domain.RuntimeInfo{},
		cancels:   map[string]context.CancelFunc{},
		approvals: map[string]bool{},
		subs:      map[int]chan appservice.Event{},
		host:      config.Target{OS: runtime.GOOS, Arch: runtime.GOARCH},
		speed:     1,
	}
	if os.Getenv("MYPACK_FAKE_FAST") == "1" {
		s.speed = 0.15
	}
	home, _ := os.UserHomeDir()
	s.settings = appservice.Settings{
		Language: "system", Appearance: "system",
		OutputDir:       filepath.Join(home, "mypack", "Builds"),
		InstallDir:      filepath.Join(home, "Applications"),
		RegistryURL:     "https://runtimes.mypack.dev/registry.json",
		ShowMenuBarIcon: true, CheckUpdates: true, LibraryView: "grid",
	}
	for _, k := range []string{"darwin-arm64", "darwin-amd64", "windows-amd64", "windows-arm64", "linux-amd64", "linux-arm64"} {
		size := int64(9_800_000)
		if strings.HasPrefix(k, "windows") {
			size = 8_400_000
		} else if strings.HasPrefix(k, "linux") {
			size = 7_900_000
		}
		s.runtimes[runtimeVersion+"/"+k] = &domain.RuntimeInfo{
			Version: runtimeVersion, Key: k, Size: size, Source: "runtimes.mypack.dev",
		}
	}
	if ap := os.Getenv("MYPACK_FAKE_APPEARANCE"); ap != "" {
		s.settings.Appearance = ap
	}
	if l := os.Getenv("MYPACK_FAKE_LANG"); l != "" {
		s.settings.Language = l
	}
	if os.Getenv("MYPACK_FAKE_SEED") != "empty" {
		s.seed()
	}
	return s
}

func (s *Service) delay(d time.Duration) time.Duration { return time.Duration(float64(d) * s.speed) }

func (s *Service) sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(s.delay(d))
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (s *Service) id(prefix string) string {
	s.nextID++
	return fmt.Sprintf("%s_%04d", prefix, s.nextID)
}

// ---- events ---------------------------------------------------------------

func (s *Service) Subscribe(fn func(appservice.Event)) func() {
	ch := make(chan appservice.Event, 256)
	s.subMu.Lock()
	id := s.subN
	s.subN++
	s.subs[id] = ch
	s.subMu.Unlock()
	go func() {
		for e := range ch {
			fn(e)
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.subMu.Lock()
			if c, ok := s.subs[id]; ok {
				delete(s.subs, id)
				close(c)
			}
			s.subMu.Unlock()
		})
	}
}

func (s *Service) publish(e appservice.Event) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for _, ch := range s.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

func (s *Service) Close() error {
	s.subMu.Lock()
	for id, ch := range s.subs {
		close(ch)
		delete(s.subs, id)
	}
	s.subMu.Unlock()
	return nil
}

func (s *Service) logActivity(actor, action, target, result string) {
	s.activity = append([]domain.Activity{{ID: s.id("act"), Time: time.Now(), Actor: actor, Action: action, Target: target, Result: result}}, s.activity...)
}

// ---- projects ---------------------------------------------------------------

func clone(p *domain.Project) domain.Project {
	out := *p
	if p.Config != nil {
		out.Config = p.Config.Clone()
	}
	return out
}

func notFound(what string) error {
	return &domain.Error{Code: appservice.CodeNotFound, Message: what + " was not found."}
}

func (s *Service) ListProjects(ctx context.Context) ([]domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Project
	for _, p := range s.projects {
		if p.TrashedAt == nil {
			out = append(out, clone(p))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (s *Service) GetProject(ctx context.Context, id string) (domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok {
		return domain.Project{}, notFound("The project")
	}
	return clone(p), nil
}

func (s *Service) CreateProject(ctx context.Context, req appservice.CreateRequest) (domain.Project, error) {
	in := strings.TrimSpace(req.Input)
	var src config.Source
	if st, err := os.Stat(expandHome(in)); err == nil && in != "" && !strings.Contains(in, "://") {
		path, _ := filepath.Abs(expandHome(in))
		if st.IsDir() {
			src = config.Source{Type: config.SourceDir, Value: path, Entry: "index.html"}
		} else {
			src = config.Source{Type: config.SourceFile, Value: path}
		}
	} else {
		u, err := config.NormalizeURL(in)
		if err != nil {
			return domain.Project{}, &domain.Error{Code: appservice.CodeInvalidInput, Message: err.Error(), Field: "input"}
		}
		src = config.Source{Type: config.SourceURL, Value: u}
	}
	name := strings.TrimSpace(req.Name)
	var iconPNG []byte
	if req.Metadata != nil {
		if name == "" {
			name = req.Metadata.Title
		}
		iconPNG = req.Metadata.IconPNG
	}
	if name == "" {
		if src.Type == config.SourceURL {
			name = titleForHost(config.RegistrableHost(src.Value))
		} else {
			name = strings.TrimSuffix(filepath.Base(src.Value), filepath.Ext(src.Value))
		}
	}
	cfg := config.New(name, src, s.host)
	if iconPNG == nil {
		iconPNG = demoIcon(name)
	}
	s.mu.Lock()
	now := time.Now()
	p := &domain.Project{ID: s.id("prj"), Revision: 1, Config: cfg, IconPNG: iconPNG, Draft: true,
		CreatedAt: now, UpdatedAt: now, Origin: "gui"}
	s.projects[p.ID] = p
	s.logActivity(actorOr(req.Actor), "project.create", p.ID, "ok")
	out := clone(p)
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventProjectChanged, ProjectID: p.ID})
	return out, nil
}

func (s *Service) UpdateProject(ctx context.Context, req appservice.UpdateRequest) (domain.Project, error) {
	s.mu.Lock()
	p, ok := s.projects[req.ID]
	if !ok {
		s.mu.Unlock()
		return domain.Project{}, notFound("The project")
	}
	if req.ExpectedRevision != 0 && req.ExpectedRevision != p.Revision {
		s.mu.Unlock()
		return domain.Project{}, &domain.Error{Code: appservice.CodeRevisionConflict,
			Message:     "This project was changed somewhere else.",
			Remediation: "Reload to see the latest version, or keep your changes to overwrite it."}
	}
	if req.Config == nil {
		s.mu.Unlock()
		return domain.Project{}, &domain.Error{Code: appservice.CodeInvalidInput, Message: "No configuration given."}
	}
	if (req.Config.App.Identifier != p.Config.App.Identifier || req.Config.App.ProfileID != p.Config.App.ProfileID) && !req.ConfirmIdentityChange {
		s.mu.Unlock()
		return domain.Project{}, &domain.Error{Code: appservice.CodeIdentityChange, Field: "app.identifier",
			Message:     "Changing the app identity creates a separate app.",
			Remediation: "Confirm the change to continue."}
	}
	if err := req.Config.Validate(); err != nil {
		s.mu.Unlock()
		var ve *config.ValidationError
		if errors.As(err, &ve) && len(ve.Issues) > 0 {
			return domain.Project{}, &domain.Error{Code: appservice.CodeInvalidConfig, Message: ve.Issues[0].Message, Field: ve.Issues[0].Field}
		}
		return domain.Project{}, err
	}
	p.Config = req.Config.Clone()
	p.Revision++
	p.UpdatedAt = time.Now()
	out := clone(p)
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventProjectChanged, ProjectID: p.ID})
	return out, nil
}

func (s *Service) SetProjectIcon(ctx context.Context, id string, png []byte, actor string) (domain.Project, error) {
	s.mu.Lock()
	p, ok := s.projects[id]
	if !ok {
		s.mu.Unlock()
		return domain.Project{}, notFound("The project")
	}
	if png == nil {
		png = icons.Default(p.Config.App.Name)
		p.Config.App.Icon = config.Icon{Source: "auto"}
	} else {
		if n, err := icons.Normalize(png); err == nil {
			png = n
		}
		p.Config.App.Icon = config.Icon{Source: "file", Path: "icon.png"}
	}
	p.IconPNG = png
	p.Revision++
	p.UpdatedAt = time.Now()
	out := clone(p)
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventProjectChanged, ProjectID: id})
	return out, nil
}

func (s *Service) TrashProject(ctx context.Context, id, actor string) error {
	s.mu.Lock()
	p, ok := s.projects[id]
	if !ok {
		s.mu.Unlock()
		return notFound("The project")
	}
	now := time.Now()
	p.TrashedAt = &now
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventProjectDeleted, ProjectID: id})
	return nil
}

func (s *Service) RestoreProject(ctx context.Context, id, actor string) error {
	s.mu.Lock()
	p, ok := s.projects[id]
	if !ok {
		s.mu.Unlock()
		return notFound("The project")
	}
	p.TrashedAt = nil
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventProjectChanged, ProjectID: id})
	return nil
}

func (s *Service) DuplicateProject(ctx context.Context, id, actor string) (domain.Project, error) {
	s.mu.Lock()
	p, ok := s.projects[id]
	if !ok {
		s.mu.Unlock()
		return domain.Project{}, notFound("The project")
	}
	n := clone(p)
	n.ID = s.id("prj")
	n.Revision = 1
	n.Config.App.Name += " Copy"
	n.Config.App.Identifier += ".copy"
	n.Config.App.ProfileID = n.Config.App.Identifier
	n.Draft = true
	n.LastBuild = nil
	n.CreatedAt, n.UpdatedAt = time.Now(), time.Now()
	s.projects[n.ID] = &n
	out := clone(&n)
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventProjectChanged, ProjectID: n.ID})
	return out, nil
}

func (s *Service) ExportProject(ctx context.Context, id, path string) error {
	p, err := s.GetProject(ctx, id)
	if err != nil {
		return err
	}
	return p.Config.Save(path)
}

// ---- metadata -----------------------------------------------------------------

type site struct{ title, desc, color string }

var knownSites = map[string]site{
	"github.com":          {"GitHub", "Where the world builds software", "#24292f"},
	"notion.so":           {"Notion", "Your connected workspace", "#191919"},
	"chatgpt.com":         {"ChatGPT", "", "#10a37f"},
	"figma.com":           {"Figma", "The collaborative interface design tool", "#a259ff"},
	"linear.app":          {"Linear", "Plan and build products", "#5e6ad2"},
	"music.youtube.com":   {"YouTube Music", "", "#ff0000"},
	"web.whatsapp.com":    {"WhatsApp", "", "#25d366"},
	"mail.google.com":     {"Gmail", "", "#ea4335"},
	"app.slack.com":       {"Slack", "", "#4a154b"},
	"discord.com":         {"Discord", "", "#5865f2"},
	"calendar.google.com": {"Google Calendar", "", "#1a73e8"},
	"x.com":               {"X", "", "#000000"},
}

func titleForHost(host string) string {
	if st, ok := knownSites[host]; ok {
		return st.title
	}
	h := strings.TrimPrefix(host, "www.")
	if i := strings.Index(h, "."); i > 0 {
		h = h[:i]
	}
	if h == "" {
		return "My App"
	}
	return strings.ToUpper(h[:1]) + h[1:]
}

func (s *Service) FetchMetadata(ctx context.Context, rawURL string) (appservice.Metadata, error) {
	u, err := config.NormalizeURL(rawURL)
	if err != nil {
		return appservice.Metadata{}, &domain.Error{Code: appservice.CodeInvalidInput, Message: err.Error(), Field: "url"}
	}
	host := config.RegistrableHost(u)
	if err := s.sleep(ctx, 750*time.Millisecond); err != nil {
		return appservice.Metadata{}, err
	}
	title := titleForHost(host)
	if strings.Contains(host, "offline") || strings.HasSuffix(host, ".invalid") {
		return appservice.Metadata{URL: u, Title: title, Fallback: true, Warning: "network unreachable"}, nil
	}
	st := knownSites[host]
	return appservice.Metadata{URL: u, Title: title, Description: st.desc, ThemeColor: st.color,
		IconURL: "https://" + host + "/favicon.ico", IconPNG: demoIcon(title)}, nil
}

// ---- builds -----------------------------------------------------------------

func (s *Service) runtimeFor(t config.Target) *domain.RuntimeInfo {
	return s.runtimes[runtimeVersion+"/"+t.Key()]
}

func (s *Service) capability(t config.Target) appservice.TargetPreflight {
	tp := appservice.TargetPreflight{Target: t, Supported: true, Level: "portable", Signing: "unsigned"}
	if rt := s.runtimeFor(t); rt != nil {
		tp.Runtime = *rt
	} else {
		tp.Supported = false
		tp.Reason = "No runtime is published for this platform yet."
		return tp
	}
	switch {
	case t.OS == "darwin" && s.host.OS != "darwin":
		tp.Supported = false
		tp.Signing = "unavailable"
		tp.Reason = "Mac apps can only be built on a Mac."
	case t.OS == "darwin":
		tp.Level, tp.Signing = "installer", "adhoc"
		tp.Reason = "Ad-hoc signed: runs on this Mac. Other Macs will ask to confirm on first launch."
	case t.OS == "windows":
		tp.Reason = "Unsigned: Windows SmartScreen may ask to confirm on first launch."
	case t.OS == "linux":
		tp.Reason = "Portable folder with a .desktop entry."
	}
	return tp
}

func (s *Service) Preflight(ctx context.Context, projectID string, targets []config.Target) (appservice.Preflight, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[projectID]
	if !ok && projectID != "" {
		return appservice.Preflight{}, notFound("The project")
	}
	if len(targets) == 0 && p != nil {
		targets = p.Config.Build.Targets
	}
	if len(targets) == 0 {
		targets = []config.Target{config.DefaultTarget(s.host)}
	}
	pf := appservice.Preflight{OutputDir: s.settings.OutputDir, RuntimeVersion: runtimeVersion}
	for _, t := range targets {
		tp := s.capability(t)
		if tp.Supported && !tp.Runtime.Cached {
			pf.DownloadBytes += tp.Runtime.Size
		}
		pf.Targets = append(pf.Targets, tp)
	}
	return pf, nil
}

func (s *Service) StartBuild(ctx context.Context, req appservice.BuildRequest) (domain.BuildJob, error) {
	s.mu.Lock()
	p, ok := s.projects[req.ProjectID]
	if !ok {
		s.mu.Unlock()
		return domain.BuildJob{}, notFound("The project")
	}
	targets := req.Targets
	if len(targets) == 0 {
		targets = p.Config.Build.Targets
	}
	if len(targets) == 0 {
		targets = []config.Target{config.DefaultTarget(s.host)}
	}
	for _, t := range targets {
		if tp := s.capability(t); !tp.Supported {
			s.mu.Unlock()
			return domain.BuildJob{}, &domain.Error{Code: appservice.CodeTargetUnsupp, Message: tp.Reason}
		}
	}
	job := &domain.BuildJob{ID: s.id("job"), ProjectID: p.ID, ProjectRev: p.Revision, Snapshot: p.Config.Clone(),
		Targets: targets, RuntimeVersion: runtimeVersion, Status: domain.JobQueued, Stage: domain.StageQueued,
		Progress: -1, Actor: actorOr(req.Actor), CreatedAt: time.Now()}
	needDownload := false
	for _, t := range targets {
		if rt := s.runtimeFor(t); rt != nil && !rt.Cached {
			needDownload = true
		}
	}
	s.jobs = append([]*domain.BuildJob{job}, s.jobs...)
	if needDownload && !req.AllowDownload {
		job.Status = domain.JobAwaitingApproval
		job.Error = &domain.Error{Code: appservice.CodeDownloadRequired,
			Message:     "A runtime must be downloaded before this build can continue.",
			Remediation: "Approve the download to continue."}
		out := *job
		s.mu.Unlock()
		s.publish(appservice.Event{Kind: appservice.EventJobChanged, JobID: job.ID, ProjectID: p.ID, Job: &out})
		s.publish(appservice.Event{Kind: appservice.EventApprovalNeeded, JobID: job.ID, ProjectID: p.ID, Job: &out})
		return out, nil
	}
	out := *job
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventJobChanged, JobID: job.ID, ProjectID: p.ID, Job: &out})
	go s.run(job.ID)
	return out, nil
}

func (s *Service) ApproveJob(ctx context.Context, jobID, actor string) error {
	s.mu.Lock()
	j := s.job(jobID)
	if j == nil {
		s.mu.Unlock()
		return notFound("The build")
	}
	if j.Status != domain.JobAwaitingApproval {
		s.mu.Unlock()
		return nil
	}
	j.Status = domain.JobQueued
	j.Error = nil
	out := *j
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventJobChanged, JobID: jobID, ProjectID: j.ProjectID, Job: &out})
	go s.run(jobID)
	return nil
}

func (s *Service) job(id string) *domain.BuildJob {
	for _, j := range s.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

func (s *Service) CancelBuild(ctx context.Context, jobID, actor string) error {
	s.mu.Lock()
	j := s.job(jobID)
	if j == nil {
		s.mu.Unlock()
		return notFound("The build")
	}
	if cancel := s.cancels[jobID]; cancel != nil {
		cancel()
		s.mu.Unlock()
		return nil
	}
	if j.Status == domain.JobAwaitingApproval || j.Status == domain.JobQueued {
		now := time.Now()
		j.Status, j.FinishedAt = domain.JobCancelled, &now
		j.Error = &domain.Error{Code: appservice.CodeCancelled, Message: "The build was cancelled."}
	}
	out := *j
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventJobChanged, JobID: jobID, ProjectID: j.ProjectID, Job: &out})
	return nil
}

func (s *Service) GetJob(ctx context.Context, jobID string) (domain.BuildJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.job(jobID)
	if j == nil {
		return domain.BuildJob{}, notFound("The build")
	}
	return *j, nil
}

func (s *Service) JobLog(ctx context.Context, jobID string) ([]domain.LogLine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.job(jobID)
	if j == nil {
		return nil, notFound("The build")
	}
	return append([]domain.LogLine(nil), j.Log...), nil
}

func (s *Service) ListJobs(ctx context.Context, projectID string, limit int) ([]domain.BuildJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.BuildJob
	for _, j := range s.jobs {
		if projectID == "" || j.ProjectID == projectID {
			out = append(out, *j)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// update mutates a job under the lock and publishes it.
func (s *Service) update(id string, fn func(j *domain.BuildJob)) {
	s.mu.Lock()
	j := s.job(id)
	if j == nil {
		s.mu.Unlock()
		return
	}
	fn(j)
	out := *j
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventJobChanged, JobID: id, ProjectID: out.ProjectID, Job: &out})
}

func (s *Service) log(id string, stage domain.Stage, level, msg string) {
	line := domain.LogLine{Time: time.Now(), Level: level, Stage: stage, Message: msg}
	s.mu.Lock()
	if j := s.job(id); j != nil {
		j.Log = append(j.Log, line)
	}
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventJobLog, JobID: id, Log: &line})
}

func (s *Service) run(id string) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[id] = cancel
	j := s.job(id)
	snap := j.Snapshot
	targets := j.Targets
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.cancels, id)
		s.mu.Unlock()
		cancel()
	}()
	now := time.Now()
	s.update(id, func(j *domain.BuildJob) { j.Status, j.StartedAt = domain.JobRunning, &now })
	fail := strings.Contains(strings.ToLower(snap.App.Name+snap.App.Source.Value), "fail")

	stage := func(st domain.Stage, d time.Duration, msgs ...string) error {
		s.update(id, func(j *domain.BuildJob) { j.Stage, j.Progress = st, -1 })
		for _, m := range msgs {
			s.log(id, st, "info", m)
		}
		return s.sleep(ctx, d)
	}
	finish := func(status domain.JobStatus, e *domain.Error) {
		end := time.Now()
		s.update(id, func(j *domain.BuildJob) {
			j.Status, j.FinishedAt, j.Error = status, &end, e
			if status == domain.JobSucceeded {
				j.Stage, j.Progress = domain.StageCompleted, 1
			}
		})
	}
	cancelled := func() {
		s.log(id, "", "warn", "Build cancelled by user; previous artifact left untouched.")
		finish(domain.JobCancelled, &domain.Error{Code: appservice.CodeCancelled, Message: "The build was cancelled."})
	}

	if stage(domain.StageValidating, 500*time.Millisecond, "Validating configuration "+snap.App.Identifier, "Source: "+snap.App.Source.Value) != nil {
		cancelled()
		return
	}
	if stage(domain.StageResolving, 400*time.Millisecond, "Resolving runtime "+runtimeVersion) != nil {
		cancelled()
		return
	}
	// Download what's missing, with real (simulated) byte progress.
	for _, t := range targets {
		s.mu.Lock()
		rt := s.runtimeFor(t)
		missing := rt != nil && !rt.Cached
		s.mu.Unlock()
		if !missing {
			s.log(id, domain.StageResolving, "info", "Runtime "+t.Key()+" is cached and verified")
			continue
		}
		s.update(id, func(j *domain.BuildJob) { j.Stage, j.Progress = domain.StageDownloading, 0 })
		s.log(id, domain.StageDownloading, "info", fmt.Sprintf("Downloading runtime %s (%s)", t.Key(), humanBytes(rt.Size)))
		if err := s.download(ctx, rt, func(done, total int64) {
			s.update(id, func(j *domain.BuildJob) { j.Progress = float64(done) / float64(total) })
		}); err != nil {
			cancelled()
			return
		}
		s.log(id, domain.StageDownloading, "info", "Signature and checksum verified")
	}
	if stage(domain.StageAssembling, 900*time.Millisecond, "Assembling app bundle", "Writing Info.plist and mypack.json", "Rendering icons (icns, 16–1024 px)") != nil {
		cancelled()
		return
	}
	if stage(domain.StageCustomizing, 300*time.Millisecond, "Applying window and navigation settings") != nil {
		cancelled()
		return
	}
	if stage(domain.StageSigning, 700*time.Millisecond, "Signing (ad-hoc)") != nil {
		cancelled()
		return
	}
	if fail {
		s.log(id, domain.StageSigning, "error", "codesign: resource fork, Finder information, or similar detritus not allowed")
		finish(domain.JobFailed, &domain.Error{Code: "SIGNING_FAILED",
			Message:     "The app couldn't be signed.",
			Cause:       "codesign reported extended attributes on files inside the bundle.",
			Remediation: "Remove extended attributes from local source files (xattr -cr <folder>) and build again."})
		return
	}
	if stage(domain.StagePackaging, 400*time.Millisecond, "Packaging") != nil {
		cancelled()
		return
	}
	if stage(domain.StageVerifying, 600*time.Millisecond, "Verifying signature and launchability") != nil {
		cancelled()
		return
	}
	if stage(domain.StagePublishing, 250*time.Millisecond, "Publishing to output folder") != nil {
		cancelled()
		return
	}
	var arts []domain.Artifact
	s.mu.Lock()
	out := s.settings.OutputDir
	for _, t := range targets {
		name := config.ExecutableName(snap.App.Name)
		path := filepath.Join(out, name, t.Key(), name)
		if t.OS == "darwin" {
			path += ".app"
		} else if t.OS == "windows" {
			path += ".zip"
		}
		arts = append(arts, domain.Artifact{ID: s.id("art"), JobID: id, ProjectID: j.ProjectID, Target: t, Path: path,
			Size: 11_200_000 + int64(len(snap.App.Name))*1000, SHA256: fakeHash(id + t.Key()), Signature: map[bool]string{true: "adhoc", false: "unsigned"}[t.OS == "darwin"],
			CreatedAt: time.Now()})
	}
	if p := s.projects[j.ProjectID]; p != nil {
		now := time.Now()
		p.LastBuild, p.Draft = &now, false
	}
	s.mu.Unlock()
	s.update(id, func(j *domain.BuildJob) { j.Artifacts = arts })
	s.log(id, domain.StagePublishing, "info", "Done: "+arts[0].Path)
	finish(domain.JobSucceeded, nil)
	s.publish(appservice.Event{Kind: appservice.EventProjectChanged, ProjectID: j.ProjectID})
}

func (s *Service) download(ctx context.Context, rt *domain.RuntimeInfo, progress func(done, total int64)) error {
	total := rt.Size
	steps := 40
	for i := 1; i <= steps; i++ {
		if err := s.sleep(ctx, 70*time.Millisecond); err != nil {
			return err
		}
		done := total * int64(i) / int64(steps)
		if progress != nil {
			progress(done, total)
		}
		s.publish(appservice.Event{Kind: appservice.EventRuntimeProgress, Done: done, Total: total})
	}
	s.mu.Lock()
	home, _ := os.UserHomeDir()
	rt.Cached, rt.Verified, rt.CacheSize, rt.LastUsed = true, true, rt.Size*23/10, time.Now()
	rt.Path = filepath.Join(home, "Library", "Caches", "mypack", "runtimes", rt.Version, rt.Key)
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventRuntimeChanged})
	return nil
}

// ---- installations ------------------------------------------------------------

func (s *Service) artifact(id string) (domain.Artifact, *domain.BuildJob) {
	for _, j := range s.jobs {
		for _, a := range j.Artifacts {
			if a.ID == id {
				return a, j
			}
		}
	}
	return domain.Artifact{}, nil
}

func (s *Service) InstallAndOpen(ctx context.Context, req appservice.InstallRequest) (domain.Installation, error) {
	if err := s.sleep(ctx, 700*time.Millisecond); err != nil {
		return domain.Installation{}, err
	}
	s.mu.Lock()
	a, j := s.artifact(req.ArtifactID)
	if j == nil {
		s.mu.Unlock()
		return domain.Installation{}, notFound("The build output")
	}
	p := s.projects[a.ProjectID]
	var inst *domain.Installation
	for _, in := range s.installs {
		if in.ProjectID == a.ProjectID {
			inst = in
		}
	}
	name := config.ExecutableName(j.Snapshot.App.Name)
	if inst == nil && s.licenseTokenLocked() == "" && s.managedCountLocked() >= appservice.FreeManagedApps {
		s.mu.Unlock()
		return domain.Installation{}, &domain.Error{Code: appservice.CodeLicenseLimit,
			Message:     "The free edition includes up to 2 managed apps, and you have 2.",
			Remediation: "Manage or uninstall an existing app, export its config, or enter a license key to add more."}
	}
	if inst == nil {
		inst = &domain.Installation{ID: s.id("inst"), ProjectID: a.ProjectID, InstalledAt: time.Now()}
		s.installs = append(s.installs, inst)
	}
	inst.ArtifactID, inst.Name, inst.Identifier = a.ID, j.Snapshot.App.Name, j.Snapshot.App.Identifier
	inst.SourceURL = j.Snapshot.App.Source.Value
	inst.Path = filepath.Join(s.settings.InstallDir, name+".app")
	inst.Version, inst.RuntimeVersion = j.Snapshot.App.Version, runtimeVersion
	inst.ProfileDir = "~/Library/Application Support/mypack-apps/" + j.Snapshot.App.ProfileID
	inst.State, inst.UpdateReason, inst.LastManagedAt = domain.InstallManaged, "", time.Now()
	if p != nil {
		inst.IconPNG = p.IconPNG
	}
	out := *inst
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventInstallChanged, InstallID: out.ID, ProjectID: out.ProjectID})
	return out, nil
}

func (s *Service) ListInstallations(ctx context.Context) ([]domain.Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Installation, 0, len(s.installs))
	for _, in := range s.installs {
		out = append(out, *in)
	}
	return out, nil
}

func (s *Service) inst(id string) *domain.Installation {
	for _, in := range s.installs {
		if in.ID == id {
			return in
		}
	}
	return nil
}

func (s *Service) OpenInstallation(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	in := s.inst(id)
	if in == nil {
		return notFound("The app")
	}
	if in.State == domain.InstallMissing {
		return &domain.Error{Code: appservice.CodeNotFound, Message: "We couldn't find the app where it was last installed.", Remediation: "Locate the app or rebuild it."}
	}
	return nil
}

func (s *Service) rebuildInstall(id, actor string) (domain.BuildJob, error) {
	s.mu.Lock()
	in := s.inst(id)
	if in == nil {
		s.mu.Unlock()
		return domain.BuildJob{}, notFound("The app")
	}
	if in.State == domain.InstallExternal {
		s.mu.Unlock()
		return domain.BuildJob{}, &domain.Error{Code: appservice.CodeNotManaged, Message: "mypack doesn't manage this app."}
	}
	pid := in.ProjectID
	s.mu.Unlock()
	job, err := s.StartBuild(context.Background(), appservice.BuildRequest{ProjectID: pid, AllowDownload: true, Actor: actor})
	if err != nil {
		return job, err
	}
	go func() {
		for {
			time.Sleep(s.delay(200 * time.Millisecond))
			j, err := s.GetJob(context.Background(), job.ID)
			if err != nil || j.Status == domain.JobFailed || j.Status == domain.JobCancelled {
				return
			}
			if j.Status == domain.JobSucceeded && len(j.Artifacts) > 0 {
				s.mu.Lock()
				if in := s.inst(id); in != nil {
					in.ArtifactID, in.UpdateReason, in.State, in.LastManagedAt = j.Artifacts[0].ID, "", domain.InstallManaged, time.Now()
					in.Version = j.Snapshot.App.Version
				}
				s.mu.Unlock()
				s.publish(appservice.Event{Kind: appservice.EventInstallChanged, InstallID: id})
				return
			}
		}
	}()
	return job, nil
}

func (s *Service) UpdateInstallation(ctx context.Context, id, actor string) (domain.BuildJob, error) {
	return s.rebuildInstall(id, actor)
}

func (s *Service) RepairInstallation(ctx context.Context, id, actor string) (domain.BuildJob, error) {
	return s.rebuildInstall(id, actor)
}

func (s *Service) Uninstall(ctx context.Context, id string, clearData bool, actor string) error {
	s.mu.Lock()
	for i, in := range s.installs {
		if in.ID == id {
			if in.State == domain.InstallExternal {
				s.mu.Unlock()
				return &domain.Error{Code: appservice.CodeNotManaged, Message: "mypack doesn't manage this app."}
			}
			s.installs = append(s.installs[:i], s.installs[i+1:]...)
			s.logActivity(actorOr(actor), "installation.uninstall", id, fmt.Sprintf("clearData=%v", clearData))
			s.mu.Unlock()
			s.publish(appservice.Event{Kind: appservice.EventInstallChanged, InstallID: id})
			return nil
		}
	}
	s.mu.Unlock()
	return notFound("The app")
}

func (s *Service) RelocateInstallation(ctx context.Context, id, newPath, actor string) (domain.Installation, error) {
	s.mu.Lock()
	in := s.inst(id)
	if in == nil {
		s.mu.Unlock()
		return domain.Installation{}, notFound("The app")
	}
	// Demo hook: a chosen app whose name contains "other" is a different app.
	if strings.Contains(strings.ToLower(filepath.Base(newPath)), "other") {
		s.mu.Unlock()
		return domain.Installation{}, &domain.Error{Code: appservice.CodeNotSameApp,
			Message:     "That app is not " + in.Name + " (its identifier does not match).",
			Remediation: "Choose the app this entry was created from, or remove the entry from your library."}
	}
	in.Path = newPath
	if in.State == domain.InstallMissing {
		in.State = domain.InstallManaged
	}
	out := *in
	s.logActivity(actorOr(actor), "installation.relocate", id, "")
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventInstallChanged, InstallID: id, ProjectID: out.ProjectID})
	return out, nil
}

func (s *Service) ForgetInstallation(ctx context.Context, id, actor string) error {
	s.mu.Lock()
	for i, in := range s.installs {
		if in.ID == id {
			s.installs = append(s.installs[:i], s.installs[i+1:]...)
			s.logActivity(actorOr(actor), "installation.forget", id, "")
			s.mu.Unlock()
			s.publish(appservice.Event{Kind: appservice.EventInstallChanged, InstallID: id})
			return nil
		}
	}
	s.mu.Unlock()
	return notFound("The app")
}

// ---- license ---------------------------------------------------------------------

func (s *Service) licenseTokenLocked() string { return s.licenseToken }

func (s *Service) managedCountLocked() int {
	n := 0
	for _, in := range s.installs {
		if in.State == domain.InstallManaged || in.State == domain.InstallAttention {
			n++
		}
	}
	return n
}

func (s *Service) License(ctx context.Context) (appservice.LicenseStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := appservice.LicenseStatus{Edition: licensing.EditionFree, FreeLimit: appservice.FreeManagedApps, ManagedApps: s.managedCountLocked()}
	if s.licenseToken == "" {
		return st, nil
	}
	p, err := licensing.Verify(s.licenseToken, nil)
	if err != nil {
		st.Error = err.Error()
		return st, nil
	}
	st.Edition, st.Valid, st.FreeLimit, st.Email, st.LicenseID = p.Edition, true, 0, p.Email, p.LicenseID
	return st, nil
}

// ActivateLicense verifies real tokens against MYPACK_LICENSE_PUBKEY, exactly
// like the production service (generate one with scripts/issuelicense).
func (s *Service) ActivateLicense(ctx context.Context, token, actor string) (appservice.LicenseStatus, error) {
	token = strings.Join(strings.Fields(token), "")
	if _, err := licensing.Verify(token, nil); err != nil {
		msg := err.Error()
		if token == "" {
			msg = "paste your license key to activate it"
		}
		return appservice.LicenseStatus{}, &domain.Error{Code: appservice.CodeLicenseInvalid,
			Message: strings.ToUpper(msg[:1]) + msg[1:] + ".", Remediation: "Check that you copied the whole key, or contact support to recover your license."}
	}
	s.mu.Lock()
	s.licenseToken = token
	s.logActivity(actorOr(actor), "license.activate", "", "")
	s.mu.Unlock()
	return s.License(ctx)
}

func (s *Service) DeactivateLicense(ctx context.Context, actor string) error {
	s.mu.Lock()
	s.licenseToken = ""
	s.logActivity(actorOr(actor), "license.deactivate", "", "")
	s.mu.Unlock()
	return nil
}

func (s *Service) ImportExistingApp(ctx context.Context, path, actor string) (domain.Installation, error) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	s.mu.Lock()
	in := &domain.Installation{ID: s.id("inst"), Name: name, Identifier: "com.external." + config.Slug(name),
		Path: path, Version: "—", State: domain.InstallExternal, InstalledAt: time.Now(), LastManagedAt: time.Now(),
		IconPNG: icons.Default(name)}
	s.installs = append(s.installs, in)
	out := *in
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventInstallChanged, InstallID: in.ID})
	return out, nil
}

// ---- runtimes -----------------------------------------------------------------

func (s *Service) ListRuntimes(ctx context.Context) ([]domain.RuntimeInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.RuntimeInfo
	for _, r := range s.runtimes {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (s *Service) DownloadRuntime(ctx context.Context, version, key string, progress func(done, total int64)) (domain.RuntimeInfo, error) {
	s.mu.Lock()
	rt := s.runtimes[version+"/"+key]
	s.mu.Unlock()
	if rt == nil {
		return domain.RuntimeInfo{}, &domain.Error{Code: "RUNTIME_NOT_FOUND", Message: "That runtime isn't published."}
	}
	if err := s.download(ctx, rt, progress); err != nil {
		return domain.RuntimeInfo{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return *rt, nil
}

func (s *Service) VerifyRuntime(ctx context.Context, version, key string) error {
	if err := s.sleep(ctx, 600*time.Millisecond); err != nil {
		return err
	}
	s.mu.Lock()
	rt := s.runtimes[version+"/"+key]
	if rt == nil || !rt.Cached {
		s.mu.Unlock()
		return &domain.Error{Code: "RUNTIME_NOT_FOUND", Message: "That runtime isn't downloaded."}
	}
	rt.Verified = true
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventRuntimeChanged})
	return nil
}

func (s *Service) DeleteRuntime(ctx context.Context, version, key string) error {
	s.mu.Lock()
	rt := s.runtimes[version+"/"+key]
	if rt != nil {
		rt.Cached, rt.Verified, rt.CacheSize, rt.Path = false, false, 0, ""
	}
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventRuntimeChanged})
	return nil
}

// ---- platform, settings -------------------------------------------------------

func (s *Service) HostTarget() config.Target                     { return s.host }
func (s *Service) Reveal(ctx context.Context, path string) error { return nil }
func (s *Service) OpenURL(ctx context.Context, url string) error { return nil }
func (s *Service) Activity(ctx context.Context, limit int) ([]domain.Activity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit > 0 && len(s.activity) > limit {
		return append([]domain.Activity(nil), s.activity[:limit]...), nil
	}
	return append([]domain.Activity(nil), s.activity...), nil
}

func (s *Service) StorageBreakdown(ctx context.Context) (appservice.Storage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var st appservice.Storage
	for _, r := range s.runtimes {
		st.RuntimeCache += r.CacheSize
	}
	for _, j := range s.jobs {
		for _, a := range j.Artifacts {
			st.Artifacts += a.Size
		}
	}
	st.Profiles = int64(len(s.installs)) * 48_000_000
	return st, nil
}

func (s *Service) Settings(ctx context.Context) (appservice.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings, nil
}

func (s *Service) SaveSettings(ctx context.Context, st appservice.Settings) error {
	s.mu.Lock()
	s.settings = st
	s.mu.Unlock()
	s.publish(appservice.Event{Kind: appservice.EventSettingsChanged})
	return nil
}

// ---- helpers ------------------------------------------------------------------

func actorOr(a string) string {
	if a == "" {
		return "gui"
	}
	return a
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

func fakeHash(seed string) string {
	const hex = "0123456789abcdef"
	var b strings.Builder
	h := uint32(2166136261)
	for i := 0; i < 64; i++ {
		for _, r := range seed {
			h ^= uint32(r) + uint32(i)
			h *= 16777619
		}
		b.WriteByte(hex[h%16])
	}
	return b.String()
}

func humanBytes(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.0f KB", float64(n)/1e3)
	}
	return fmt.Sprintf("%d B", n)
}

// demoIcon is a generated, neutral icon for the demo library: a glyph
// icon for well-known demo sites, the letter tile otherwise.
func demoIcon(name string) []byte {
	if sampleicons.Known(name) {
		return sampleicons.Icon(name)
	}
	return icons.Default(name)
}
