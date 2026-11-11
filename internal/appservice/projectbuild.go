package appservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"crypto/sha256"
	"encoding/hex"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// This file implements the "preparing" pipeline stage: building the user's
// JavaScript project (install dependencies, run its build script) before the
// engine packages the output. It runs in the service so the engine API is
// unchanged. Running a project's scripts executes arbitrary code, so it is
// gated by an explicit, fingerprinted approval that only a human frontend
// (GUI/CLI) can grant; an MCP actor never can.

// projectBuildTimeout bounds install+build together.
var projectBuildTimeout = 15 * time.Minute

// extraBinDirs lists where package managers live when the process has a
// minimal PATH (GUI apps on macOS). Replaced in tests.
var extraBinDirs = defaultExtraBinDirs

const maxStaleScan = 50000

// ---- approval state ----------------------------------------------------------

// projectBuildState is the per-project record of what the user approved and
// what mypack last built successfully (both identified by fingerprint).
type projectBuildState struct {
	Approved   string    `json:"approved,omitempty"`
	ApprovedAt time.Time `json:"approvedAt,omitempty"`
	Built      string    `json:"built,omitempty"`
	BuiltAt    time.Time `json:"builtAt,omitempty"`
}

func projectStateKey(projectID string) string { return "projectbuild." + projectID }

// projectFingerprint identifies exactly what the user approves to run.
func projectFingerprint(pm, script, project string) string {
	h := sha256.Sum256([]byte(pm + "\x00" + script + "\x00" + filepath.Clean(project)))
	return hex.EncodeToString(h[:])
}

func (s *Impl) projectState(ctx context.Context, projectID string) projectBuildState {
	var st projectBuildState
	_, _ = s.store.GetSetting(ctx, projectStateKey(projectID), &st)
	return st
}

func (s *Impl) updateProjectState(ctx context.Context, projectID string, mut func(*projectBuildState)) error {
	s.projMu.Lock()
	defer s.projMu.Unlock()
	st := s.projectState(ctx, projectID)
	mut(&st)
	return s.store.SetSetting(ctx, projectStateKey(projectID), st)
}

func (s *Impl) recordProjectApproval(ctx context.Context, projectID, fp string) error {
	return s.updateProjectState(ctx, projectID, func(st *projectBuildState) {
		st.Approved, st.ApprovedAt = fp, time.Now().UTC()
	})
}

// ---- planning ----------------------------------------------------------------

type projectPlan struct {
	Build       *config.ProjectBuild
	Project     string
	OutDir      string // absolute output folder (Source.Value)
	Entry       string
	Fingerprint string
	NeedInstall bool
	Command     string // display string
	Needed      bool   // a build must run
	Reason      string
	Approved    bool
}

// planProjectBuild returns nil when snap has no project build configured.
func (s *Impl) planProjectBuild(ctx context.Context, projectID string, snap *config.Config) *projectPlan {
	if snap == nil || snap.App.Source.Build == nil {
		return nil
	}
	src := snap.App.Source
	b := src.Build
	p := &projectPlan{Build: b, Project: src.Project, OutDir: src.Value, Entry: firstNonEmpty(src.Entry, "index.html")}
	p.Fingerprint = projectFingerprint(b.PackageManager, b.Script, src.Project)
	p.NeedInstall = !dirExists(filepath.Join(src.Project, "node_modules"))
	run := b.PackageManager + " run " + b.Script
	p.Command = run
	if p.NeedInstall {
		if args := installArgs(b.PackageManager, src.Project); len(args) > 0 {
			p.Command = b.PackageManager + " " + strings.Join(args, " ") + " && " + run
		}
	}
	st := s.projectState(ctx, projectID)
	p.Approved = st.Approved == p.Fingerprint
	switch {
	case st.Built != p.Fingerprint:
		p.Needed, p.Reason = true, "the project has not been built by mypack yet"
	case !fileExists(filepath.Join(p.OutDir, p.Entry)):
		p.Needed, p.Reason = true, "the build output is missing"
	default:
		if stale, why := sourcesNewerThan(src.Project, p.OutDir, filepath.Join(p.OutDir, p.Entry)); stale {
			p.Needed, p.Reason = true, why
		}
	}
	return p
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// installArgs is the dependency-install command for a package manager.
func installArgs(pm, project string) []string {
	has := func(n string) bool { return fileExists(filepath.Join(project, n)) }
	switch pm {
	case "npm":
		if has("package-lock.json") || has("npm-shrinkwrap.json") {
			return []string{"ci"}
		}
		return []string{"install"}
	case "pnpm":
		if has("pnpm-lock.yaml") {
			return []string{"install", "--frozen-lockfile"}
		}
		return []string{"install"}
	case "yarn":
		if has("yarn.lock") {
			if has(".yarnrc.yml") { // Yarn Berry spells it --immutable
				return []string{"install", "--immutable"}
			}
			return []string{"install", "--frozen-lockfile"}
		}
		return []string{"install"}
	case "bun":
		if has("bun.lock") || has("bun.lockb") {
			return []string{"install", "--frozen-lockfile"}
		}
		return []string{"install"}
	}
	return nil
}

// sourcesNewerThan reports whether any project source file is newer than
// the built entry. Dependency, VCS and output folders are skipped.
func sourcesNewerThan(project, outDir, entry string) (bool, string) {
	st, err := os.Stat(entry)
	if err != nil {
		return true, "the build output is missing"
	}
	built := st.ModTime()
	skip := map[string]bool{"node_modules": true, ".git": true, ".next": true, ".nuxt": true, ".output": true,
		".svelte-kit": true, ".angular": true, ".turbo": true, ".cache": true, ".parcel-cache": true, ".vercel": true}
	if rel, rerr := filepath.Rel(project, outDir); rerr == nil && !strings.HasPrefix(rel, "..") {
		skip[strings.Split(filepath.ToSlash(rel), "/")[0]] = true
	}
	var newer string
	n := 0
	_ = filepath.WalkDir(project, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != project && skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if n++; n > maxStaleScan {
			return filepath.SkipAll
		}
		if info, ierr := d.Info(); ierr == nil && info.ModTime().After(built) {
			newer = path
			return filepath.SkipAll
		}
		return nil
	})
	if newer != "" {
		rel, _ := filepath.Rel(project, newer)
		return true, rel + " changed since the last build"
	}
	return false, ""
}

// ---- approvals ---------------------------------------------------------------

func projectApprovalErr(p *projectPlan) *domain.Error {
	return &domain.Error{
		Code:        CodeProjectBuildRequired,
		Message:     "mypack needs to build your project before packaging it.",
		Cause:       p.Command + "  (in " + p.Project + ")",
		Remediation: "Approve the build in mypack, or run `mypack build <project> --run-build`.",
	}
}

// withApproval fills BuildJob.Approval for jobs awaiting approval. It is
// derived from the stored error so it needs no schema.
func (s *Impl) withApproval(j domain.BuildJob) domain.BuildJob {
	j.Approval = nil
	if j.Status != domain.JobAwaitingApproval || j.Error == nil {
		return j
	}
	switch j.Error.Code {
	case CodeProjectBuildRequired:
		if p := s.planProjectBuild(context.Background(), j.ProjectID, j.Snapshot); p != nil {
			j.Approval = &domain.ApprovalRequest{Kind: domain.ApprovalProjectBuild, Command: p.Command, Path: p.Project,
				Detail: "This runs your project's own build script on this computer, in " + p.Project + "."}
		}
	case CodeDownloadRequired:
		cmd := "Download the mypack runtime"
		if j.Snapshot != nil {
			cmd = fmt.Sprintf("Download mypack runtime %s for %s", j.Snapshot.Runtime.Version, targetKeys(j.Snapshot.Build.Targets))
		}
		j.Approval = &domain.ApprovalRequest{Kind: domain.ApprovalRuntimeDownload, Command: cmd, Detail: j.Error.Message}
	}
	return j
}

// pendingApproval reports what a human still has to approve before the job
// may run: the project build first, then the runtime download.
func (s *Impl) pendingApproval(ctx context.Context, projectID string, snap *config.Config, pf Preflight, allowDownload bool) (*domain.Error, int64) {
	if p := s.planProjectBuild(ctx, projectID, snap); p != nil && p.Needed && !p.Approved {
		return projectApprovalErr(p), 0
	}
	if pf.DownloadBytes > 0 && !allowDownload {
		return &domain.Error{
			Code: CodeDownloadRequired,
			Message: fmt.Sprintf("Building needs the %s runtime for %s (%s download).",
				snap.Runtime.Version, targetKeys(snap.Build.Targets), humanBytes(pf.DownloadBytes)),
			Remediation: "Approve the download in mypack, or re-run with permission to download runtimes (--yes).",
		}, pf.DownloadBytes
	}
	return nil, 0
}

func isMCPActor(actor string) bool { return strings.HasPrefix(strings.ToLower(actor), "mcp") }

// ---- running -----------------------------------------------------------------

// prepareProject runs the project build for job. parked=true means the job
// went back to awaiting_approval (nothing was run).
func (s *Impl) prepareProject(ctx context.Context, st *jobState, job domain.BuildJob) (parked bool, err error) {
	plan := s.planProjectBuild(ctx, job.ProjectID, job.Snapshot)
	if plan == nil {
		return false, nil
	}
	if !dirExists(plan.Project) {
		return false, &domain.Error{Code: CodeProjectBuildFailed, Message: "The project folder no longer exists.",
			Cause: plan.Project, Remediation: "Restore the folder, or create the app from the project again."}
	}
	if rel, rerr := filepath.Rel(plan.Project, plan.OutDir); rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, &domain.Error{Code: CodeInvalidConfig, Message: "The app source is outside its project folder.",
			Cause: plan.OutDir, Field: "app.source.value"}
	}
	if !plan.Needed {
		s.logLine(st, "info", "Project output is up to date; skipping the project build.")
		return false, nil
	}
	s.logLine(st, "info", "Building your project: "+plan.Reason+".")
	if !plan.Approved {
		de := projectApprovalErr(plan)
		s.setState(st, func(j *domain.BuildJob) {
			j.Status, j.Stage, j.Progress, j.Error = domain.JobAwaitingApproval, domain.StageQueued, -1, de
		})
		j := st.snapshot()
		s.emit(Event{Kind: EventApprovalNeeded, ProjectID: j.ProjectID, JobID: j.ID, Job: &j})
		return true, nil
	}

	b := plan.Build
	if !scriptExists(plan.Project, b.Script) {
		return false, &domain.Error{Code: CodeProjectBuildFailed,
			Message:     fmt.Sprintf("package.json has no %q script.", b.Script),
			Remediation: "Pick an existing build script for this project, or add one to package.json."}
	}
	exe, dir, ferr := findTool(b.PackageManager)
	if ferr != nil {
		return false, nodeNotFound(b.PackageManager)
	}
	s.logLine(st, "info", "Using "+exe)

	tctx, cancel := context.WithTimeout(ctx, projectBuildTimeout)
	defer cancel()
	tail := &tailBuf{max: 20}
	emit := func(line string) {
		tail.add(line)
		s.appendLog(st, domain.LogLine{Time: time.Now(), Level: "info", Stage: domain.StagePreparing, Message: line})
	}

	if plan.NeedInstall {
		if args := installArgs(b.PackageManager, plan.Project); len(args) > 0 {
			s.logLine(st, "info", "$ "+b.PackageManager+" "+strings.Join(args, " "))
			if rerr := runStep(tctx, plan.Project, buildEnv(dir, false), exe, args, emit); rerr != nil {
				return false, stepError(ctx, tctx, "Installing dependencies", b.PackageManager+" "+strings.Join(args, " "), plan, tail, rerr)
			}
		}
	}
	args := []string{"run", b.Script}
	s.logLine(st, "info", "$ "+b.PackageManager+" run "+b.Script)
	tail.reset()
	if rerr := runStep(tctx, plan.Project, buildEnv(dir, true), exe, args, emit); rerr != nil {
		return false, stepError(ctx, tctx, "Building your project", b.PackageManager+" run "+b.Script, plan, tail, rerr)
	}
	if !fileExists(filepath.Join(plan.OutDir, plan.Entry)) {
		rel, _ := filepath.Rel(plan.Project, plan.OutDir)
		return false, &domain.Error{Code: CodeProjectOutputMissing,
			Message:     fmt.Sprintf("The build finished but %s was not created.", filepath.ToSlash(filepath.Join(rel, plan.Entry))),
			Cause:       plan.OutDir,
			Remediation: "Check that the framework writes to the expected output folder (" + filepath.ToSlash(rel) + "), or use its static export/adapter."}
	}
	now := time.Now().UTC()
	_ = s.updateProjectState(context.WithoutCancel(ctx), job.ProjectID, func(ps *projectBuildState) {
		ps.Built, ps.BuiltAt = plan.Fingerprint, now
	})
	s.logLine(st, "info", "Project build finished.")
	return false, nil
}

func scriptExists(project, script string) bool {
	raw, err := os.ReadFile(filepath.Join(project, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return false
	}
	_, ok := pkg.Scripts[script]
	return ok
}

func nodeNotFound(pm string) *domain.Error {
	rem := "Install Node.js from nodejs.org (or `brew install node`), then try again."
	if pm != "npm" {
		rem = "Install Node.js from nodejs.org (or `brew install node`) and " + pm + " (for example `npm install -g " + pm + "`), then try again."
	}
	return &domain.Error{Code: CodeNodeNotFound, Message: "Couldn't find " + pm + " on this computer.",
		Cause: "searched PATH and common install locations", Remediation: rem}
}

// stepError converts a failed step into the right domain error. Cancellation
// is returned as-is so the caller records a cancelled build.
func stepError(parent, tctx context.Context, what, cmd string, plan *projectPlan, tail *tailBuf, err error) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(err, exec.ErrNotFound) {
		return nodeNotFound(plan.Build.PackageManager)
	}
	msg := fmt.Sprintf("%s failed (%s).", what, cmd)
	switch {
	case tctx.Err() != nil:
		msg = fmt.Sprintf("%s timed out after %s (%s).", what, projectBuildTimeout, cmd)
	default:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			msg = fmt.Sprintf("%s failed (%s exited with status %d).", what, cmd, ee.ExitCode())
		}
	}
	cause := tail.String()
	if cause == "" {
		cause = err.Error()
	}
	return &domain.Error{Code: CodeProjectBuildFailed, Message: msg, Cause: cause,
		Remediation: "Fix the errors above, or run `" + cmd + "` in " + plan.Project + " to see them in your terminal."}
}

// buildEnv is os.Environ() with PATH extended; the build step also gets
// CI=1 and NODE_ENV=production (not the install step: that would skip
// devDependencies).
func buildEnv(binDir string, forBuild bool) []string {
	pathKey := "PATH"
	cur := ""
	var out []string
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		switch {
		case strings.EqualFold(k, "PATH"):
			pathKey, cur = k, v
			continue
		case strings.EqualFold(k, "CI"), forBuild && k == "NODE_ENV":
			continue
		}
		out = append(out, kv)
	}
	parts := []string{binDir}
	if cur != "" {
		parts = append(parts, cur)
	}
	for _, d := range extraBinDirs() {
		if dirExists(d) {
			parts = append(parts, d)
		}
	}
	out = append(out, pathKey+"="+strings.Join(parts, string(os.PathListSeparator)), "CI=1")
	if forBuild {
		out = append(out, "NODE_ENV=production")
	}
	return out
}

// runStep runs exe with explicit args (no shell) in dir, streaming output
// lines to emit. Cancelling ctx kills the whole process group.
func runStep(ctx context.Context, dir string, env []string, exe string, args []string, emit func(string)) error {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir, cmd.Env = dir, env
	setupKill(cmd)
	cmd.WaitDelay = 2 * time.Second
	lw := &lineWriter{emit: emit}
	cmd.Stdout, cmd.Stderr = lw, lw
	err := cmd.Run()
	lw.flush()
	return err
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// lineWriter splits process output into trimmed, ANSI-free lines.
type lineWriter struct {
	mu   sync.Mutex
	buf  []byte
	emit func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.line(string(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) > 8192 { // a never-ending line
		w.line(string(w.buf))
		w.buf = nil
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.line(string(w.buf))
		w.buf = nil
	}
}

func (w *lineWriter) line(s string) {
	s = strings.TrimRight(ansiRe.ReplaceAllString(s, ""), " \t\r")
	if s == "" {
		return
	}
	if len(s) > 2000 {
		s = s[:2000] + "…"
	}
	w.emit(s)
}

// tailBuf keeps the last max lines.
type tailBuf struct {
	mu    sync.Mutex
	max   int
	lines []string
}

func (t *tailBuf) add(l string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lines = append(t.lines, l)
	if len(t.lines) > t.max {
		t.lines = t.lines[len(t.lines)-t.max:]
	}
}
func (t *tailBuf) reset() { t.mu.Lock(); t.lines = nil; t.mu.Unlock() }
func (t *tailBuf) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.Join(t.lines, "\n")
}

// ---- locating tools -----------------------------------------------------------

func defaultExtraBinDirs() []string {
	dirs := []string{"/opt/homebrew/bin", "/usr/local/bin"}
	home, _ := os.UserHomeDir()
	if home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".bun", "bin"),
			filepath.Join(home, ".volta", "bin"),
			filepath.Join(home, ".local", "share", "mise", "shims"),
			filepath.Join(home, ".asdf", "shims"),
		)
		if nvm := newestNvmBin(home); nvm != "" {
			dirs = append(dirs, nvm)
		}
		dirs = append(dirs, filepath.Join(home, ".fnm", "aliases", "default", "bin"))
	}
	if runtime.GOOS == "windows" {
		if pf := os.Getenv("ProgramFiles"); pf != "" {
			dirs = append(dirs, filepath.Join(pf, "nodejs"))
		}
		if ad := os.Getenv("APPDATA"); ad != "" {
			dirs = append(dirs, filepath.Join(ad, "npm"))
		}
	}
	return dirs
}

// newestNvmBin returns ~/.nvm/versions/node/<newest>/bin.
func newestNvmBin(home string) string {
	m, _ := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin"))
	if len(m) == 0 {
		return ""
	}
	ver := func(p string) [3]int {
		name := filepath.Base(filepath.Dir(p))
		var v [3]int
		for i, part := range strings.SplitN(strings.TrimPrefix(name, "v"), ".", 3) {
			v[i], _ = strconv.Atoi(part)
		}
		return v
	}
	sort.Slice(m, func(i, j int) bool {
		a, b := ver(m[i]), ver(m[j])
		for k := 0; k < 3; k++ {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return false
	})
	return m[0]
}

// findTool locates a package-manager executable: PATH first, then the
// well-known install directories. It returns the executable and its folder.
func findTool(name string) (exe, dir string, err error) {
	dirs := filepath.SplitList(os.Getenv("PATH"))
	dirs = append(dirs, extraBinDirs()...)
	names := []string{name}
	if runtime.GOOS == "windows" {
		names = []string{name + ".cmd", name + ".exe", name + ".bat", name}
	}
	for _, d := range dirs {
		if d == "" || !filepath.IsAbs(d) {
			continue
		}
		for _, n := range names {
			p := filepath.Join(d, n)
			if st, serr := os.Stat(p); serr == nil && !st.IsDir() && (runtime.GOOS == "windows" || st.Mode()&0o111 != 0) {
				return p, d, nil
			}
		}
	}
	return "", "", exec.ErrNotFound
}
