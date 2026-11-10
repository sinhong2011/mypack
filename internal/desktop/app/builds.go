package app

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/components"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
	"mypack.dev/mypack/internal/domain"
)

type buildsState struct {
	filter   int
	expanded map[string]bool
}

func (a *App) buildsView(c *ui.Context) {
	p := design.Of(c)
	b := &a.builds
	if b.expanded == nil {
		b.expanded = map[string]bool{}
	}
	page(c, design.ContentMaxW, func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			lobeui.Segmented(c.Key("filter"), &b.filter, []string{T("builds.filter.all"), T("builds.filter.active"), T("builds.filter.failed")}, lobeui.Middle)
			ui.Spacer(c)
			components.Caption(c, T("builds.count", "count", len(a.jobs)))
		})
		var list []domain.BuildJob
		for _, j := range a.jobs {
			switch b.filter {
			case 1:
				if j.Status != domain.JobRunning && j.Status != domain.JobQueued && j.Status != domain.JobAwaitingApproval {
					continue
				}
			case 2:
				if j.Status != domain.JobFailed {
					continue
				}
			}
			list = append(list, j)
		}
		if len(list) == 0 {
			components.EmptyState(c, lobeui.IconBuilds(), T("builds.empty.title"), T("builds.empty.body"), nil)
			return
		}
		ui.Column(c).Background(p.Surface).Border(1, p.Border).Radius(design.RadiusCard-2).Dividers(1, p.Divider).Clip().Children(func() {
			for _, j := range list {
				a.jobRow(c.Key(j.ID), j, true)
			}
		})
	})
}

func jobTone(j domain.BuildJob) (*ui.SVG, components.Tone, string) {
	switch j.Status {
	case domain.JobSucceeded:
		return lobeui.IconSuccess(), components.ToneSuccess, T("status.built")
	case domain.JobFailed:
		return lobeui.IconFailed(), components.ToneDanger, T("status.failed")
	case domain.JobCancelled:
		return lobeui.IconStop(), components.ToneNeutral, T("status.cancelled")
	case domain.JobAwaitingApproval:
		return lobeui.IconDownload(), components.ToneWarning, T("stage.awaitingApproval")
	}
	return lobeui.IconLoader(), components.ToneAccent, stageLabel(j)
}

// jobRow is one build history row; expandable with details and actions.
func (a *App) jobRow(c *ui.Context, j domain.BuildJob, showProject bool) {
	p := design.Of(c)
	b := &a.builds
	if b.expanded == nil {
		b.expanded = map[string]bool{}
	}
	open := b.expanded[j.ID]
	ui.Column(c).Children(func() {
		row := ui.ButtonBase(c).Label(j.Snapshot.App.Name).Row().Justify(ui.Start).Height(design.Row).PaddingX(14).Gap(12).AlignItems(ui.Center).
			Cursor(ui.CursorPointer).Expanded(open)
		if row.Hovered() || open {
			row.Background(p.Hover)
		}
		row.Children(func() {
			icon, tone, label := jobTone(j)
			fg, _ := toneColor(p, tone)
			if j.Status == domain.JobRunning || j.Status == domain.JobQueued {
				components.Spinner(c, 16, fg)
			} else {
				ui.Icon(c, icon).FontSize(16).TextColor(fg)
			}
			if showProject {
				var png []byte
				if pr := a.project(j.ProjectID); pr != nil {
					png = pr.IconPNG
				}
				components.AppIcon(c, png, j.Snapshot.App.Name, 22)
			}
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(1).Children(func() {
				name := j.Snapshot.App.Name
				if !showProject {
					name = label
				}
				ui.Text(c, name).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine()
				var ts []string
				for _, t := range j.Targets {
					ts = append(ts, targetLabel(t))
				}
				sub := strings.Join(ts, ", ")
				if showProject {
					sub = label + " · " + sub
				}
				ui.Text(c, sub).FontSize(design.FontSmall).TextColor(p.TextTertiary).SingleLine()
			})
			if j.StartedAt != nil && j.FinishedAt != nil {
				components.Caption(c, duration(j.FinishedAt.Sub(*j.StartedAt))).Width(64).TextAlign(ui.End).FontFeatures("tnum")
			}
			components.Caption(c, ago(j.CreatedAt)).Width(110).TextAlign(ui.End)
			components.Caption(c, actorLabel(j.Actor)).Width(48).TextAlign(ui.End)
			ch := ui.Icon(c, lobeui.IconChevronRt()).FontSize(13).TextColor(p.TextTertiary)
			if open {
				ch.Rotate(90)
			}
		})
		if row.Clicked() {
			b.expanded[j.ID] = !open
			if !open {
				if _, ok := a.logs[j.ID]; !ok {
					a.logs[j.ID] = nil
					id := j.ID
					go func() {
						lines, err := a.svc.JobLog(context.Background(), id)
						if err == nil {
							a.win.Update(func() { a.logs[id] = lines })
						}
					}()
				}
			}
		}
		if open {
			a.jobDetails(c, j)
		}
	})
}

func actorLabel(actor string) string {
	switch {
	case actor == "gui":
		return T("actor.app")
	case actor == "cli":
		return "CLI"
	case strings.HasPrefix(actor, "mcp"):
		return T("actor.agent")
	}
	return actor
}

func toneColor(p *design.Palette, t components.Tone) (fg, bg ui.Color) { return components.ToneColors(p, t) }

func (a *App) jobDetails(c *ui.Context, j domain.BuildJob) {
	p := design.Of(c)
	ui.Column(c).Padding(4, 14, 16, 42).Gap(12).Children(func() {
		if j.Error != nil {
			ui.Column(c).Gap(4).Padding(10, 12).Radius(design.RadiusControl).Background(p.DangerSoft).Children(func() {
				ui.Text(c, j.Error.Message).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text)
				if j.Error.Remediation != "" {
					ui.Text(c, j.Error.Remediation).FontSize(design.FontSmall).TextColor(p.TextSecondary)
				}
			})
		}
		for _, art := range j.Artifacts {
			art := art
			ui.Grid(c.Key(art.ID)).ColumnTracks(ui.FitContent(), ui.Fr(1), ui.FitContent()).GapX(14).GapY(6).AlignItems(ui.Center).Children(func() {
				components.Caption(c, T("builds.artifact"))
				components.Mono(c, art.Path).SingleLine().MinWidth(0).Selectable()
				lobeui.CopyButton(c, art.Path)
				components.Caption(c, T("builds.checksum"))
				components.Mono(c, "sha256:"+art.SHA256[:min(16, len(art.SHA256))]+"…").Selectable()
				lobeui.CopyButton(c, art.SHA256)
				components.Caption(c, T("builds.size"))
				ui.Text(c, bytesLabel(art.Size)+" · "+signatureLabel(art.Signature)).FontSize(design.FontSmall).TextColor(p.Text)
				ui.Box(c)
			})
		}
		ui.Row(c).Gap(8).Children(func() {
			if len(j.Artifacts) > 0 {
				path := j.Artifacts[0].Path
				if components.Button(c.Key("of"), T("action.openFolder"), components.ButtonOpts{Size: design.ControlSM, Icon: lobeui.IconFolderOpen()}).Clicked() {
					a.reveal(path)
				}
				if components.Button(c.Key("cp"), T("action.copyPath"), components.ButtonOpts{Size: design.ControlSM, Icon: lobeui.IconCopy()}).Clicked() {
					c.WriteClipboard(path)
					a.toast(T("common.copied"), "", 1)
				}
			}
			if j.Status == domain.JobFailed || j.Status == domain.JobCancelled {
				if components.Button(c.Key("rt"), T("action.retry"), components.ButtonOpts{Size: design.ControlSM, Icon: lobeui.IconRetry()}).Clicked() {
					a.startBuild(j.ProjectID, j.Targets)
				}
			}
			if j.Status == domain.JobFailed {
				jj := j
				if components.Button(c.Key("diag"), T("error.copyDiagnostic"), components.ButtonOpts{Size: design.ControlSM, Variant: components.Ghost}).Clicked() {
					c.WriteClipboard(a.diagnostic(&jj))
					a.toast(T("error.copied"), "", 1)
				}
			}
			ui.Spacer(c)
			if components.Button(c.Key("pr"), T("builds.openProject"), components.ButtonOpts{Size: design.ControlSM, Variant: components.Ghost}).Clicked() {
				a.watch[j.ProjectID] = j.ID
				a.openProject(j.ProjectID, 0)
			}
		})
		var lines []components.LogLine
		for _, l := range a.logs[j.ID] {
			lines = append(lines, components.LogLine{Time: l.Time.Format("15:04:05"), Level: l.Level, Text: l.Message})
		}
		components.LogViewer(c, lines, 180)
	})
}

func signatureLabel(s string) string {
	switch s {
	case "adhoc":
		return T("signature.adhoc")
	case "unsigned":
		return T("signature.unsigned")
	case "signed":
		return T("signature.signed")
	case "notarized":
		return T("signature.notarized")
	}
	return s
}
