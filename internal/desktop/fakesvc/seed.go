package fakesvc

import (
	"path/filepath"
	"time"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// seed fills the library with a realistic mix of states.
func (s *Service) seed() {
	now := time.Now()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	type seedP struct {
		name, url string
		built     bool
		updated   time.Duration
	}
	list := []seedP{
		{"Linear", "https://linear.app/", true, 2 * time.Hour},
		{"YouTube Music", "https://music.youtube.com/", true, 26 * time.Hour},
		{"Notion", "https://www.notion.so/", false, 20 * time.Minute},
		{"Figma", "https://www.figma.com/", true, 3 * 24 * time.Hour},
		{"WhatsApp", "https://web.whatsapp.com/", true, 9 * 24 * time.Hour},
	}
	ids := map[string]string{}
	for _, sp := range list {
		cfg := config.New(sp.name, config.Source{Type: config.SourceURL, Value: sp.url}, s.host)
		p := &domain.Project{ID: s.id("prj"), Revision: 3, Config: cfg, IconPNG: demoIcon(sp.name),
			Draft: !sp.built, CreatedAt: ago(sp.updated + 48*time.Hour), UpdatedAt: ago(sp.updated), Origin: "gui"}
		if sp.built {
			t := ago(sp.updated - time.Minute)
			p.LastBuild = &t
		}
		s.projects[p.ID] = p
		ids[sp.name] = p.ID
		if !sp.built {
			continue
		}
		start := ago(sp.updated)
		end := start.Add(14 * time.Second)
		tgt := config.DefaultTarget(s.host)
		name := config.ExecutableName(sp.name)
		art := domain.Artifact{ID: s.id("art"), ProjectID: p.ID, Target: tgt,
			Path: filepath.Join(s.settings.OutputDir, name, tgt.Key(), name+".app"), Size: 11_400_000,
			SHA256: fakeHash(sp.name), Signature: "adhoc", CreatedAt: end}
		job := &domain.BuildJob{ID: s.id("job"), ProjectID: p.ID, ProjectRev: 3, Snapshot: cfg.Clone(),
			Targets: []config.Target{tgt}, RuntimeVersion: runtimeVersion, Status: domain.JobSucceeded,
			Stage: domain.StageCompleted, Progress: 1, Actor: "gui", CreatedAt: start, StartedAt: &start, FinishedAt: &end}
		art.JobID = job.ID
		job.Artifacts = []domain.Artifact{art}
		job.Log = []domain.LogLine{
			{Time: start, Level: "info", Stage: domain.StageValidating, Message: "Validating configuration " + cfg.App.Identifier},
			{Time: start.Add(2 * time.Second), Level: "info", Stage: domain.StageAssembling, Message: "Assembling app bundle"},
			{Time: start.Add(9 * time.Second), Level: "info", Stage: domain.StageSigning, Message: "Signing (ad-hoc)"},
			{Time: end, Level: "info", Stage: domain.StagePublishing, Message: "Done: " + art.Path},
		}
		s.jobs = append(s.jobs, job)

		var inst *domain.Installation
		switch sp.name {
		case "Linear", "YouTube Music", "WhatsApp":
			inst = &domain.Installation{ID: s.id("inst"), ProjectID: p.ID, ArtifactID: art.ID, Name: sp.name,
				Identifier: cfg.App.Identifier, SourceURL: sp.url, Path: filepath.Join(s.settings.InstallDir, name+".app"),
				Version: cfg.App.Version, RuntimeVersion: runtimeVersion,
				ProfileDir: "~/Library/Application Support/mypack-apps/" + cfg.App.ProfileID,
				State:      domain.InstallManaged, InstalledAt: end, LastManagedAt: end, IconPNG: p.IconPNG}
		}
		if inst != nil {
			if sp.name == "YouTube Music" {
				inst.UpdateReason = "Runtime 1.0.1 is available (security fixes)."
			}
			if sp.name == "WhatsApp" {
				inst.State = domain.InstallMissing
			}
			s.installs = append(s.installs, inst)
		}
	}
	// A failed build in history.
	if pid := ids["Figma"]; pid != "" {
		start := ago(50 * time.Hour)
		end := start.Add(8 * time.Second)
		tgt := config.Target{OS: "windows", Arch: "amd64", Format: "portable"}
		s.jobs = append(s.jobs, &domain.BuildJob{ID: s.id("job"), ProjectID: pid, ProjectRev: 2,
			Snapshot: s.projects[pid].Config.Clone(), Targets: []config.Target{tgt}, RuntimeVersion: runtimeVersion,
			Status: domain.JobFailed, Stage: domain.StageDownloading, Progress: -1, Actor: "cli",
			CreatedAt: start, StartedAt: &start, FinishedAt: &end,
			Error: &domain.Error{Code: "RUNTIME_VERIFY_FAILED", Message: "The downloaded runtime didn't pass verification.",
				Cause: "sha256 mismatch for mypack-runtime-1.0.0-windows-amd64.tar.gz", Remediation: "Retry the download. If it keeps failing, check your network proxy."},
			Log: []domain.LogLine{{Time: end, Level: "error", Stage: domain.StageDownloading, Message: "sha256 mismatch"}}})
	}
	// Newest first.
	for i, j := 0, len(s.jobs)-1; i < j; i, j = i+1, j-1 {
		s.jobs[i], s.jobs[j] = s.jobs[j], s.jobs[i]
	}
	sortJobs(s.jobs)

	// An external, read-only app.
	s.installs = append(s.installs, &domain.Installation{ID: s.id("inst"), Name: "ChatGPT", Identifier: "com.pake.chatgpt",
		SourceURL: "https://chatgpt.com/", Path: "/Applications/ChatGPT.app", Version: "2.9.0", State: domain.InstallExternal,
		InstalledAt: ago(40 * 24 * time.Hour), LastManagedAt: ago(40 * 24 * time.Hour), IconPNG: demoIcon("ChatGPT")})

	// One cached runtime for another platform.
	if rt := s.runtimes[runtimeVersion+"/windows-amd64"]; rt != nil {
		rt.Cached, rt.Verified, rt.CacheSize, rt.LastUsed = true, true, rt.Size*21/10, ago(50*time.Hour)
		rt.Path = "~/Library/Caches/mypack/runtimes/1.0.0/windows-amd64"
	}
}

func sortJobs(js []*domain.BuildJob) {
	for i := 1; i < len(js); i++ {
		for k := i; k > 0 && js[k].CreatedAt.After(js[k-1].CreatedAt); k-- {
			js[k], js[k-1] = js[k-1], js[k]
		}
	}
}
