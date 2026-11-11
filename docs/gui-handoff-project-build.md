# GUI handoff: building the user's JS project

The service (`internal/appservice`) now runs a dropped JavaScript project's own
build before packaging it. Nothing in `internal/desktop` has been changed; this
note is what the GUI has to render.

## What changed in the API

- `CreateProject` for a project folder (package.json) **always succeeds**. The
  project's `config.app.source` is
  `{type:"dir", value:"<project>/<outDir>", entry:"index.html", project:"<abs root>", build:{packageManager, script, outDir, framework}}`.
  Only Next.js with server-side rendering still returns the old `INVALID_INPUT` guidance error.
- New first pipeline stage: `domain.StagePreparing` (`"preparing"`), now first in
  `domain.Stages`. It covers dependency install + the build script. Jobs whose
  source has no `build` (websites, plain folders) never enter it.
- `domain.BuildJob.Approval *ApprovalRequest` (JSON `approval`), set while
  `Status == awaiting_approval` (derived on read; also present in `job.changed`
  and `approval.needed` event payloads):
  ```go
  type ApprovalRequest struct {
      Kind    string // "project_build" | "runtime_download"
      Command string // exact text to display, e.g. "npm install && npm run build"
      Detail  string // one-sentence explanation
      Path    string // project folder the command runs in (project_build only)
  }
  ```
- `BuildJob.Error.Code == "PROJECT_BUILD_REQUIRED"` accompanies a project-build approval
  (`Error.Cause` = `"<command>  (in <path>)"`). Runtime-download approvals keep
  `RUNTIME_DOWNLOAD_REQUIRED` and now also fill `Approval{Kind:"runtime_download", Command:"Download mypack runtime X for T", Detail:<message>}`.
- When both are needed the project build is asked first; after `ApproveJob` the
  job may come back to `awaiting_approval` once more, now for the runtime download.
  Always re-read `job.Approval` after approving.
- `ApproveJob(ctx, jobID, "gui")` records the approval for the fingerprint
  (package manager + script + project path) and resumes. The approval is stored per
  project, so later builds of the same project do not ask again. Changing the
  script, package manager or folder asks again. `ApproveJob` with an `mcp*` actor
  returns `APPROVAL_REQUIRED`.
- `BuildRequest.AllowProjectBuild` approves up front (the CLI's `--run-build`).
  Do not set it silently from the GUI: the card below is the consent.
- mypack skips the project build when its output is up to date and was built by
  mypack before, so a project card normally shows approval only the first time.

## Approval card (job `awaiting_approval`, `approval.kind == "project_build"`)

- Title: **Build your project first?**
- Body: one line, e.g. "To make this app, mypack needs to run your project's build on this computer."
- The command, monospace and copyable, exactly `approval.command`
  (e.g. `npm ci && npm run build`).
- Project path, `approval.path` (with a Reveal button if cheap).
- Buttons: primary **Run & Continue** -> `ApproveJob(jobID, "gui")`; secondary **Cancel** -> `CancelBuild(jobID, "gui")`.
- Show it wherever the runtime-download approval is shown today (same event: `approval.needed`).
  Branch on `approval.kind`: `runtime_download` keeps the existing card.

## Progress (stage `preparing`)

- Add "Preparing" as the first step in the stage list/progress UI (`domain.Stages` order).
  Progress is indeterminate (`-1`) during it.
- Process output arrives as ordinary `job.log` events: `Log.Stage == "preparing"`,
  one trimmed line per event (stdout and stderr merged, ANSI stripped, `$ npm ...`
  lines mark each step). Stream them into the live log view; `JobLog(jobID)` returns them later.
- Cancel works during this stage; the whole process tree is killed and the job ends `cancelled`.
- Timeout is 15 minutes.

## Errors (job `failed`, `job.Error`)

| Code | Message / what to show |
|---|---|
| `NODE_NOT_FOUND` | "Couldn't find npm (or pnpm/yarn/bun) on this computer." Remediation: install Node.js from nodejs.org or `brew install node`. Offer a "Download Node.js" link. |
| `PROJECT_BUILD_FAILED` | Message names the failing command and exit status. `Error.Cause` holds the last ~20 log lines: show them in a code block, with "Show full log". Remediation tells the user to run the command in a terminal. |
| `PROJECT_OUTPUT_MISSING` | The build ran but `<outDir>/index.html` is absent. Remediation: check the framework's output folder / static adapter. |

Use `Error.Message` as the headline and `Error.Remediation` as the hint, as for other errors.
CLI exit codes: approval `6`, these three errors `5`.
