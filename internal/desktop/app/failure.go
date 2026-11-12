package app

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/domain"
)

// A failure, said once and plainly, where it happened (the failed step of
// a build, an update or repair sheet): what happened in one sentence, up
// to two buttons that fix it, and the details folded away (the code, the
// cause and the last lines of the log, with Copy). No box, no tinted
// fill: red is kept for the ✕ and the word "failed".

// plainError is what happened, in plain words, for the codes we know.
func plainError(e *domain.Error) string {
	if e == nil {
		return T("build.failed.generic")
	}
	switch e.Code {
	case "PROJECT_BUILD_FAILED":
		return T("fail.projectBuild")
	case "NODE_NOT_FOUND":
		return T("fail.node")
	case "PROJECT_OUTPUT_MISSING":
		return T("fail.output")
	case "RUNTIME_DOWNLOAD_FAILED":
		return T("fail.download")
	case "SIGNING_FAILED":
		return T("fail.signing")
	case "INSTALL_CONFLICT":
		return T("fail.conflict")
	case "APP_RUNNING":
		return T("fail.running")
	}
	if e.Message != "" {
		return e.Message
	}
	return T("build.failed.generic")
}

// fix is one button that fixes a failure.
type fix struct {
	label string
	run   func()
}

// fixesFor are the (at most two) fixes for a failure. retry reruns what
// failed; projectID, when set, is the project it belongs to.
func (a *App) fixesFor(e *domain.Error, projectID string, retry func()) []fix {
	var out []fix
	code := ""
	if e != nil {
		code = e.Code
	}
	switch code {
	case "NODE_NOT_FOUND":
		out = append(out, fix{T("projectBuild.getNode"), func() {
			a.do(func(ctx context.Context) error { return a.svc.OpenURL(ctx, "https://nodejs.org") }, a.toastErr)
		}})
	case "PROJECT_OUTPUT_MISSING", "PROJECT_BUILD_FAILED":
		if projectID != "" {
			id := projectID
			out = append(out, fix{T("fail.projectSettings"), func() { a.openProject(id, 1) }})
		}
	case "RUNTIME_DOWNLOAD_FAILED":
		out = append(out, fix{T("fail.openSettings"), func() { a.nav(secSettings) }})
	}
	if retry != nil && len(out) < 2 {
		out = append(out, fix{T("action.retry"), retry})
	}
	return out
}

// failureBody draws a failure at the current indent: what happened, the
// fixes (or the remediation when no button covers it), then Details.
// jobID, when set, gives the log its tail.
func (a *App) failureBody(c *ui.Context, e *domain.Error, jobID string, fixes []fix) {
	p := design.Of(c)
	if e == nil {
		e = &domain.Error{Code: "BUILD_FAILED"}
	}
	ui.Column(c.Key("failure")).FillWidth().Gap(8).Children(func() {
		ui.Text(c, plainError(e)).FontSize(13).TextColor(p.Text).LineHeight(1.35)
		if len(fixes) > 0 {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				for i, f := range fixes {
					if kit.Button(c.Key(i), f.label, kit.ButtonOpts{Size: 26}).Clicked() {
						f.run()
					}
				}
			})
		}
		if e.Remediation != "" && (len(fixes) == 0 || len(fixes) == 1 && fixes[0].label == T("action.retry")) {
			ui.Text(c, e.Remediation).FontSize(design.FontSmall).TextColor(p.TextSecondary).LineHeight(1.35)
		}
		// Details, folded away.
		key := "fail-details:" + jobID + e.Code
		open := a.logOpen[key]
		kit.Disclosure(c.Key("details"), &open, T("fail.details"), "").Margin(0, -8)
		a.logOpen[key] = open
		if !open {
			return
		}
		ui.Column(c.Key("detail")).FillWidth().Gap(8).Children(func() {
			ui.Text(c, e.Code).FontSize(design.FontSmall).TextColor(p.TextSecondary).Font(kit.MonoFont).Selectable()
			cause := strings.TrimSpace(e.Cause)
			tail := a.logTail(jobID, 12)
			if e.Code == "PROJECT_BUILD_FAILED" && cause != "" {
				tail, cause = cause, "" // its last lines are the cause
			}
			if cause != "" {
				ui.Text(c, cause).FontSize(design.FontSmall).TextColor(p.TextSecondary).LineHeight(1.35).Selectable()
			}
			if tail != "" {
				ui.Box(c.Key("tail")).FillWidth().Radius(8).Background(p.Sunken).Children(func() {
					ui.ScrollHorizontal(c).FillWidth().Children(func() {
						ui.Column(c).Padding(10, 40, 10, 12).Gap(2).Selectable().Children(func() {
							for i, l := range strings.Split(tail, "\n") {
								ui.Text(c.Key(i), l).Font(kit.MonoFont).FontSize(11).TextColor(p.TextSecondary).SingleLine()
							}
						})
					})
					ui.Box(c).Absolute().Top(4).Right(4).Children(func() {
						if kit.CopyButton(c.Key("copy"), tail, T("common.copy")) {
							a.toast(T("common.copied"), "", 1)
						}
					})
				})
			}
		})
	})
}

// logTail is the last n lines of a build's log, one per line.
func (a *App) logTail(jobID string, n int) string {
	if jobID == "" {
		return ""
	}
	ls := a.logs[jobID]
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	var b strings.Builder
	for _, l := range ls {
		b.WriteString(l.Message)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
