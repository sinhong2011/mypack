package appservice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/storage"
	"mypack.dev/mypack/pkg/config"
)

// InstallAndOpen implements Service.
func (s *Impl) InstallAndOpen(ctx context.Context, req InstallRequest) (inst domain.Installation, err error) {
	defer func() { s.audit(ctx, req.Actor, "install", req.ArtifactID, err) }()

	art, err := s.store.GetArtifact(ctx, req.ArtifactID)
	if err != nil {
		return inst, err
	}
	if _, serr := os.Stat(art.Path); serr != nil {
		return inst, &domain.Error{Code: "ARTIFACT_MISSING", Message: "The built app is no longer on disk.",
			Cause: serr.Error(), Remediation: "Build the project again."}
	}
	job, err := s.store.GetJob(ctx, art.JobID)
	if err != nil {
		return inst, err
	}
	set, err := s.Settings(ctx)
	if err != nil {
		return inst, err
	}
	existing := s.managedInstallationFor(ctx, art.ProjectID)
	replace := req.Replace
	if existing != nil {
		replace = true // re-installing our own managed app is an update, not a conflict
		if s.inst.IsRunning(existing.Path) {
			return inst, appRunning(existing.Name)
		}
	}
	path, err := s.inst.Install(ctx, art.Path, set.InstallDir, replace)
	if err != nil {
		return inst, err
	}

	rec := domain.Installation{}
	if existing != nil {
		rec = *existing
	}
	cfg := job.Snapshot
	rec.ProjectID, rec.ArtifactID, rec.Path = art.ProjectID, art.ID, path
	rec.Name, rec.Identifier, rec.Version = cfg.App.Name, cfg.App.Identifier, cfg.App.Version
	rec.RuntimeVersion = job.RuntimeVersion
	rec.ProfileDir = s.inst.ProfileDir(cfg.App.ProfileID)
	rec.State, rec.UpdateReason = domain.InstallManaged, ""
	rec.LastManagedAt = time.Now().UTC()
	if cfg.App.Source.Type == config.SourceURL {
		rec.SourceURL = cfg.App.Source.Value
	}
	if p, perr := s.store.GetProject(ctx, art.ProjectID); perr == nil {
		rec.IconPNG = p.IconPNG
	}
	inst, err = s.store.SaveInstallation(ctx, rec)
	if err != nil {
		return inst, err
	}
	s.emit(Event{Kind: EventInstallChanged, InstallID: inst.ID, ProjectID: inst.ProjectID})
	if req.Open {
		if lerr := s.inst.Launch(path); lerr != nil {
			return inst, asDomainErr(lerr, "LAUNCH_FAILED")
		}
	}
	return inst, nil
}

func appRunning(name string) *domain.Error {
	return &domain.Error{
		Code:        CodeAppRunning,
		Message:     fmt.Sprintf("%s is running.", name),
		Remediation: fmt.Sprintf("Quit %s and try again.", name),
	}
}

func (s *Impl) managedInstallationFor(ctx context.Context, projectID string) *domain.Installation {
	all, err := s.store.ListInstallations(ctx)
	if err != nil {
		return nil
	}
	for i := range all {
		if all[i].ProjectID == projectID && all[i].State != domain.InstallExternal {
			return &all[i]
		}
	}
	return nil
}

// ListInstallations implements Service, refreshing health on the way:
// a vanished app becomes "missing" and an installation whose project or
// runtime moved on gets an UpdateReason.
func (s *Impl) ListInstallations(ctx context.Context) ([]domain.Installation, error) {
	all, err := s.store.ListInstallations(ctx)
	if err != nil {
		return nil, err
	}
	for i := range all {
		in := &all[i]
		_, statErr := os.Stat(in.Path)
		exists := statErr == nil
		before := *in
		switch {
		case !exists:
			in.State, in.UpdateReason = domain.InstallMissing, ""
		case in.State == domain.InstallMissing:
			if in.ProjectID != "" {
				in.State = domain.InstallManaged
			} else {
				in.State = domain.InstallExternal
			}
		}
		if in.State == domain.InstallManaged {
			in.UpdateReason = s.updateReason(ctx, in)
		}
		if in.State != before.State || in.UpdateReason != before.UpdateReason {
			if _, serr := s.store.SaveInstallation(ctx, *in); serr != nil {
				return nil, serr
			}
		}
	}
	return all, nil
}

// updateReason explains why an installed app is out of date ("" = current).
func (s *Impl) updateReason(ctx context.Context, in *domain.Installation) string {
	if in.ProjectID == "" || in.ArtifactID == "" {
		return ""
	}
	p, err := s.store.GetProject(ctx, in.ProjectID)
	if err != nil || p.TrashedAt != nil {
		return ""
	}
	art, err := s.store.GetArtifact(ctx, in.ArtifactID)
	if err != nil {
		return ""
	}
	job, err := s.store.GetJob(ctx, art.JobID)
	if err != nil {
		return ""
	}
	var reasons []string
	// Compare like with like: the snapshot may have been built for a subset of targets.
	cur := p.Config.Clone()
	cur.Build.Targets = job.Targets
	if configHash(cur) != job.ConfigHash {
		reasons = append(reasons, "The app's settings changed since it was built.")
	}
	if in.RuntimeVersion != "" && p.Config.Runtime.Version != in.RuntimeVersion {
		reasons = append(reasons, fmt.Sprintf("Runtime %s is available (installed: %s).", p.Config.Runtime.Version, in.RuntimeVersion))
	}
	return strings.Join(reasons, " ")
}

// OpenInstallation implements Service.
func (s *Impl) OpenInstallation(ctx context.Context, id string) (err error) {
	defer func() { s.audit(ctx, "gui", "installation.open", id, err) }()
	in, err := s.store.GetInstallation(ctx, id)
	if err != nil {
		return err
	}
	if _, serr := os.Stat(in.Path); serr != nil {
		return &domain.Error{Code: "APP_MISSING", Message: in.Name + " is no longer at " + in.Path,
			Remediation: "Repair or reinstall the app."}
	}
	return s.inst.Launch(in.Path)
}

// UpdateInstallation implements Service.
func (s *Impl) UpdateInstallation(ctx context.Context, id, actor string) (job domain.BuildJob, err error) {
	defer func() { s.audit(ctx, actor, "installation.update", id, err) }()
	return s.rebuildInstallation(ctx, id, actor, "update")
}

// RepairInstallation implements Service.
func (s *Impl) RepairInstallation(ctx context.Context, id, actor string) (job domain.BuildJob, err error) {
	defer func() { s.audit(ctx, actor, "installation.repair", id, err) }()
	return s.rebuildInstallation(ctx, id, actor, "repair")
}

// rebuildInstallation queues a build whose result replaces the installed app.
// Update uses the project's current config; repair re-assembles from the
// snapshot and runtime the installed copy was built with. Profile data is
// never touched by either.
func (s *Impl) rebuildInstallation(ctx context.Context, id, actor, kind string) (domain.BuildJob, error) {
	in, err := s.store.GetInstallation(ctx, id)
	if err != nil {
		return domain.BuildJob{}, err
	}
	if in.ProjectID == "" || in.State == domain.InstallExternal {
		return domain.BuildJob{}, &domain.Error{Code: CodeNotManaged,
			Message:     in.Name + " was not created by mypack, so it cannot be " + kind + "d.",
			Remediation: "Create a project for this website and install the result."}
	}
	p, err := s.store.GetProject(ctx, in.ProjectID)
	if err != nil {
		return domain.BuildJob{}, err
	}
	if s.inst.IsRunning(in.Path) {
		return domain.BuildJob{}, appRunning(in.Name)
	}

	// The installed artifact's target decides what to rebuild.
	target := config.DefaultTarget(s.host)
	var prevJob *domain.BuildJob
	if art, aerr := s.store.GetArtifact(ctx, in.ArtifactID); aerr == nil {
		target = art.Target
		if j, jerr := s.store.GetJob(ctx, art.JobID); jerr == nil {
			prevJob = &j
		}
	}
	var snap *config.Config
	if kind == "repair" && prevJob != nil && prevJob.Snapshot != nil {
		snap = prevJob.Snapshot.Clone()
	} else {
		snap = p.Config.Clone()
	}
	if kind == "repair" && in.RuntimeVersion != "" {
		snap.Runtime.Version = in.RuntimeVersion
	}
	snap.Build.Targets = []config.Target{target}
	set, err := s.Settings(ctx)
	if err != nil {
		return domain.BuildJob{}, err
	}
	return s.submit(ctx, submission{
		projectID: p.ID, projectRev: p.Revision, snapshot: snap, outputDir: set.OutputDir,
		actor: actorOr(actor, "gui"), kind: kind, installID: in.ID,
	})
}

// applyToInstallation runs after a successful update/repair build: it swaps
// the new artifact in (keeping a backup until the app launches), refreshes
// the Installation record and returns non-fatal warnings.
func (s *Impl) applyToInstallation(ctx context.Context, job domain.BuildJob, meta storage.JobMeta, arts []domain.Artifact) ([]string, error) {
	if len(arts) == 0 {
		return nil, newErr(CodeBuildFailed, "The build produced no app to install.", "")
	}
	in, err := s.store.GetInstallation(ctx, meta.InstallationID)
	if err != nil {
		return nil, err
	}
	art := arts[0]
	for _, a := range arts {
		if a.Target.OS == s.host.OS && a.Target.Arch == s.host.Arch {
			art = a
		}
	}
	// Re-check: the user may have launched the app while we were building.
	if s.inst.IsRunning(in.Path) {
		return nil, appRunning(in.Name)
	}
	var warns []string
	var backup string
	if _, serr := os.Stat(in.Path); serr != nil {
		// Deleted behind our back: a repair simply installs it again.
		set, _ := s.Settings(ctx)
		newPath, ierr := s.inst.Install(ctx, art.Path, firstNonEmpty(filepath.Dir(in.Path), set.InstallDir), false)
		if ierr != nil {
			return nil, ierr
		}
		in.Path = newPath
	} else {
		backup, err = s.inst.Replace(ctx, in.Path, art.Path)
		if err != nil {
			return nil, err
		}
	}

	cfg := job.Snapshot
	in.ArtifactID, in.Version, in.RuntimeVersion = art.ID, cfg.App.Version, job.RuntimeVersion
	in.Name, in.Identifier = cfg.App.Name, cfg.App.Identifier
	in.State, in.UpdateReason, in.LastManagedAt = domain.InstallManaged, "", time.Now().UTC()
	if _, err := s.store.SaveInstallation(ctx, in); err != nil {
		return warns, err
	}
	s.emit(Event{Kind: EventInstallChanged, InstallID: in.ID, ProjectID: in.ProjectID})

	// Keep the backup until the new copy has launched successfully.
	if lerr := s.inst.Launch(in.Path); lerr != nil {
		if backup != "" {
			warns = append(warns, fmt.Sprintf("The updated app could not be launched (%v); the previous version was kept at %s.", lerr, backup))
		} else {
			warns = append(warns, fmt.Sprintf("The updated app could not be launched: %v", lerr))
		}
	} else if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return warns, nil
}

// Uninstall implements Service.
func (s *Impl) Uninstall(ctx context.Context, id string, clearData bool, actor string) (err error) {
	defer func() { s.audit(ctx, actor, "installation.uninstall", id, err) }()
	in, err := s.store.GetInstallation(ctx, id)
	if err != nil {
		return err
	}
	if in.State == domain.InstallExternal {
		return &domain.Error{Code: CodeNotManaged, Message: in.Name + " was not installed by mypack.",
			Remediation: "Remove it with your system's own tools."}
	}
	if s.inst.IsRunning(in.Path) {
		return appRunning(in.Name)
	}
	if _, serr := os.Stat(in.Path); serr == nil {
		if err = s.inst.Uninstall(in.Path); err != nil {
			return err
		}
	}
	if clearData && in.ProfileDir != "" {
		// Installer.ClearProfile takes the profile ID, which is the last
		// path element of the profile directory.
		if err = s.inst.ClearProfile(filepath.Base(in.ProfileDir)); err != nil {
			return err
		}
	}
	if err = s.store.DeleteInstallation(ctx, id); err != nil {
		return err
	}
	s.emit(Event{Kind: EventInstallChanged, InstallID: id, ProjectID: in.ProjectID})
	return nil
}

// ImportExistingApp implements Service: registers an app read-only.
func (s *Impl) ImportExistingApp(ctx context.Context, path, actor string) (inst domain.Installation, err error) {
	defer func() { s.audit(ctx, actor, "installation.import", path, err) }()
	abs, err := filepath.Abs(expandHome(path))
	if err != nil {
		return inst, err
	}
	if _, serr := os.Stat(abs); serr != nil {
		return inst, newErr(CodeInvalidInput, "No such app: "+path, "Check the path and try again.")
	}
	info, err := s.inst.ReadAppInfo(abs)
	if err != nil {
		return inst, asDomainErr(err, CodeInvalidInput)
	}
	rec := domain.Installation{}
	all, _ := s.store.ListInstallations(ctx)
	for _, e := range all {
		if e.Path == abs {
			rec = e
		}
	}
	rec.Name, rec.Identifier, rec.Version, rec.SourceURL = info.Name, info.Identifier, info.Version, info.SourceURL
	rec.Path, rec.IconPNG = abs, info.IconPNG
	rec.State = domain.InstallExternal // never managed without a project to rebuild from
	rec.LastManagedAt = time.Now().UTC()
	inst, err = s.store.SaveInstallation(ctx, rec)
	if err == nil {
		s.emit(Event{Kind: EventInstallChanged, InstallID: inst.ID})
	}
	return inst, err
}

// ---- runtimes ---------------------------------------------------------------

// ListRuntimes implements Service.
func (s *Impl) ListRuntimes(ctx context.Context) ([]domain.RuntimeInfo, error) {
	s.syncRegistry(ctx)
	return s.runtimes.List(ctx)
}

// DownloadRuntime implements Service.
func (s *Impl) DownloadRuntime(ctx context.Context, version, key string, progress func(done, total int64)) (info domain.RuntimeInfo, err error) {
	defer func() { s.audit(ctx, "gui", "runtime.download", version+"/"+key, err) }()
	s.syncRegistry(ctx)
	_, err = s.runtimes.Ensure(ctx, version, key, func(done, total int64) {
		if progress != nil {
			progress(done, total)
		}
		s.emit(Event{Kind: EventRuntimeProgress, Done: done, Total: total})
	})
	if err != nil {
		return info, err
	}
	info, err = s.runtimes.Status(ctx, version, key)
	s.emit(Event{Kind: EventRuntimeChanged})
	return info, err
}

// VerifyRuntime implements Service.
func (s *Impl) VerifyRuntime(ctx context.Context, version, key string) error {
	s.syncRegistry(ctx)
	return s.runtimes.Verify(ctx, version, key)
}

// DeleteRuntime implements Service.
func (s *Impl) DeleteRuntime(ctx context.Context, version, key string) (err error) {
	defer func() { s.audit(ctx, "gui", "runtime.delete", version+"/"+key, err) }()
	if err = s.runtimes.Delete(version, key); err == nil {
		s.emit(Event{Kind: EventRuntimeChanged})
	}
	return err
}

// ---- platform helpers --------------------------------------------------------

// Reveal implements Service.
func (s *Impl) Reveal(ctx context.Context, path string) error { return s.inst.Reveal(path) }

// OpenURL implements Service. Only http(s) URLs are opened.
func (s *Impl) OpenURL(ctx context.Context, url string) error {
	if _, err := config.NormalizeURL(url); err != nil || !strings.Contains(url, "://") {
		return newErr(CodeInvalidInput, "Only http and https addresses can be opened.", "")
	}
	return s.inst.OpenURL(url)
}

// StorageBreakdown implements Service.
func (s *Impl) StorageBreakdown(ctx context.Context) (Storage, error) {
	var out Storage
	out.RuntimeCache = s.runtimes.CacheSize()

	arts, err := s.store.ListAllArtifacts(ctx)
	if err != nil {
		return out, err
	}
	seen := map[string]bool{}
	for _, a := range arts {
		if !seen[a.Path] {
			seen[a.Path] = true
			out.Artifacts += dirSize(a.Path)
		}
	}
	projects, err := s.store.ListProjects(ctx, true)
	if err != nil {
		return out, err
	}
	profiles := map[string]bool{}
	for _, p := range projects {
		profiles[s.inst.ProfileDir(p.Config.App.ProfileID)] = true
	}
	installs, err := s.store.ListInstallations(ctx)
	if err != nil {
		return out, err
	}
	for _, in := range installs {
		if in.ProfileDir != "" {
			profiles[in.ProfileDir] = true
		}
	}
	for d := range profiles {
		out.Profiles += dirSize(d)
	}
	return out, nil
}

// dirSize sums regular file sizes under root (or the file itself).
func dirSize(root string) int64 {
	var n int64
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if info, ierr := d.Info(); ierr == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}
