package app

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
)

type buildsState struct {
	filter   int
	expanded map[string]bool
}

func (a *App) buildsView(c *ui.Context) {
	b := &a.builds
	if b.expanded == nil {
		b.expanded = map[string]bool{}
	}
	page(c, func() {
		// The same counted chips as the Library's filters; an empty one
		// is left out, and a filter left empty goes back to All.
		var n [3]int
		for _, j := range a.jobs {
			for f := 0; f < 3; f++ {
				if jobMatches(j, f) {
					n[f]++
				}
			}
		}
		if b.filter != 0 && n[b.filter] == 0 {
			b.filter = 0
		}
		labels := [3]string{T("builds.filter.all"), T("builds.filter.active"), T("builds.filter.failed")}
		if n[1]+n[2] > 0 {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Role(ui.RoleRadioGroup).Label(T("builds.filter.label")).FocusGroup(ui.Horizontal).
				Children(func() {
					for f := 0; f < 3; f++ {
						if f != 0 && n[f] == 0 {
							continue
						}
						if filterChip(c.Key(f), labels[f], n[f], b.filter == f) {
							b.filter = f
						}
					}
				})
		}
		var list []domain.BuildJob
		for _, j := range a.jobs {
			if jobMatches(j, b.filter) {
				list = append(list, j)
			}
		}
		if len(list) == 0 {
			kit.EmptyState(c, icons.Builds, T("builds.empty.title"), T("builds.empty.body"), nil)
			return
		}
		design.Well(c, ui.Column(c).Radius(design.RadiusCard).Clip()).Role(ui.RoleTable).Label(T("nav.builds")).Children(func() {
			kit.TableHeader(c, buildCols(true), 14)
			for i, j := range list {
				a.jobRow(c.Key(j.ID), j, true, i)
			}
		})
	})
}

// jobMatches reports whether a job shows under Builds filter f (All,
// Running, Failed).
func jobMatches(j domain.BuildJob, f int) bool {
	switch f {
	case 1:
		return j.Status == domain.JobRunning || j.Status == domain.JobQueued || j.Status == domain.JobAwaitingApproval
	case 2:
		return j.Status == domain.JobFailed
	}
	return true
}

// buildCols are the Builds table's columns. With the project shown, the
// status icon sits in the Status column beside its word (one place for
// status); in an app's own history the status is the row's name. Who
// started a build is in the Started cell's tooltip, not a column of its own.
func buildCols(showProject bool) []kit.TableColumn {
	var cols []kit.TableColumn
	if showProject {
		cols = []kit.TableColumn{{Title: T("list.col.name")}, {Title: T("list.col.status"), Width: 150}}
	} else {
		cols = []kit.TableColumn{{Width: 18}, {Title: T("list.col.status")}}
	}
	return append(cols,
		kit.TableColumn{Title: T("builds.col.targets"), Width: 150},
		kit.TableColumn{Title: T("builds.col.duration"), Width: 64, End: true},
		kit.TableColumn{Title: T("builds.col.started"), Width: 104, End: true},
		kit.TableColumn{Width: 14})
}

func jobTone(j domain.BuildJob) (*ui.SVG, kit.Tone, string) {
	switch j.Status {
	case domain.JobSucceeded:
		return icons.Success, kit.ToneSuccess, T("status.built")
	case domain.JobFailed:
		return icons.Failed, kit.ToneDanger, T("status.failed")
	case domain.JobCancelled:
		return icons.Cancelled, kit.ToneNeutral, T("status.cancelled")
	case domain.JobAwaitingApproval:
		return icons.Download, kit.ToneWarning, T("stage.awaitingApproval")
	}
	return nil, kit.ToneAccent, stageLabel(j)
}

// jobRow is one build history row (Activity Monitor style); it expands
// into details and actions.
func (a *App) jobRow(c *ui.Context, j domain.BuildJob, showProject bool, i int) {
	p := design.Of(c)
	b := &a.builds
	if b.expanded == nil {
		b.expanded = map[string]bool{}
	}
	open := b.expanded[j.ID]
	ui.Column(c).Children(func() {
		row := ui.ButtonBase(c).Label(j.Snapshot.App.Name).Row().Justify(ui.Start).Height(38).PaddingX(14).Gap(12).AlignItems(ui.Center).
			Cursor(ui.CursorPointer).Expanded(open).Role(ui.RoleRow)
		kit.TableRowBG(c, row, i, row.Hovered(), open)
		row.Children(func() {
			icon, tone, label := jobTone(j)
			fg, _ := kit.ToneColors(p, tone)
			glyph := func(size float32) {
				if j.Status == domain.JobRunning || j.Status == domain.JobQueued {
					kit.BuildingSpinner(c, size-2, p.Accent)
				} else if icon != nil {
					kit.Glyph(c, icon, size, fg)
				}
			}
			if !showProject {
				ui.Row(c).Width(18).Shrink(0).Center().Children(func() { glyph(16) })
			}
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
				if showProject {
					var png []byte
					if pr := a.project(j.ProjectID); pr != nil {
						png = pr.IconPNG
					}
					kit.AppIconOpts(c, png, j.Snapshot.App.Name, 20, kit.IconOpts{})
					ui.Text(c, j.Snapshot.App.Name).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine()
				} else {
					ui.Text(c, label).FontSize(design.FontBody).TextColor(p.Text).SingleLine()
				}
			})
			if showProject {
				ui.Row(c).Width(150).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
					glyph(14)
					ui.Text(c, label).FontSize(design.FontSmall).TextColor(fg).SingleLine().MinWidth(0)
				})
			}
			var ts []string
			for _, t := range j.Targets {
				ts = append(ts, targetLabel(t))
			}
			ui.Text(c, strings.Join(ts, ", ")).FontSize(design.FontSmall).TextColor(p.TextSecondary).SingleLine().Width(150).Shrink(0)
			dur := "—"
			if j.StartedAt != nil && j.FinishedAt != nil {
				dur = duration(j.FinishedAt.Sub(*j.StartedAt))
			}
			ui.Text(c, dur).FontSize(design.FontSmall).TextColor(p.TextSecondary).Width(64).Shrink(0).TextAlign(ui.End).FontFeatures("tnum")
			ui.Text(c, ago(j.CreatedAt)).FontSize(design.FontSmall).TextColor(p.TextSecondary).Width(104).Shrink(0).TextAlign(ui.End).SingleLine().
				Tooltip(T("builds.startedFrom", "actor", actorLabel(j.Actor)))
			ch := kit.Glyph(c, icons.ChevronRt, 10, p.TextTertiary).Width(14)
			turn := float32(0)
			if open {
				turn = 90
			}
			ch.Rotate(ch.AnimateTo("turn", turn, design.Spring))
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

func (a *App) jobDetails(c *ui.Context, j domain.BuildJob) {
	p := design.Of(c)
	ui.Column(c).Padding(10, 14, 16, 44).Gap(12).Background(p.Alt).Children(func() {
		if j.Error != nil {
			a.failureBody(c, j.Error, j.ID, a.fixesFor(j.Error, j.ProjectID, nil))
		}
		for _, art := range j.Artifacts {
			art := art
			ui.Grid(c.Key(art.ID)).ColumnTracks(ui.FitContent(), ui.Fr(1), ui.FitContent()).GapX(14).GapY(6).AlignItems(ui.Center).Children(func() {
				kit.Caption(c, T("builds.artifact"))
				kit.Mono(c, art.Path).SingleLine().MinWidth(0).Selectable()
				if kit.CopyButton(c.Key("cp1"), art.Path, T("action.copyPath")) {
					a.toast(T("common.copied"), "", 1)
				}
				kit.Caption(c, T("builds.checksum"))
				kit.Mono(c, "sha256:"+art.SHA256[:min(16, len(art.SHA256))]+"…").Selectable()
				if kit.CopyButton(c.Key("cp2"), art.SHA256, T("common.copy")) {
					a.toast(T("common.copied"), "", 1)
				}
				kit.Caption(c, T("builds.size"))
				ui.Text(c, bytesLabel(art.Size)+" · "+signatureLabel(art.Signature)).FontSize(design.FontSmall).TextColor(p.Text)
				ui.Box(c)
			})
		}
		ui.Row(c).Gap(8).Children(func() {
			if len(j.Artifacts) > 0 {
				path := j.Artifacts[0].Path
				if kit.Button(c.Key("of"), T("action.openFolder"), kit.ButtonOpts{Size: design.ControlSM, Icon: icons.Folder}).Clicked() {
					a.reveal(path)
				}
				if kit.Button(c.Key("cp"), T("action.copyPath"), kit.ButtonOpts{Size: design.ControlSM, Icon: icons.Copy}).Clicked() {
					c.WriteClipboard(path)
					a.toast(T("common.copied"), "", 1)
				}
			}
			if j.Status == domain.JobFailed || j.Status == domain.JobCancelled {
				if kit.Button(c.Key("rt"), T("action.retry"), kit.ButtonOpts{Size: design.ControlSM, Icon: icons.Retry}).Clicked() {
					a.startBuild(j.ProjectID, j.Targets)
				}
			}
			if j.Status == domain.JobFailed {
				jj := j
				if kit.Button(c.Key("diag"), T("error.copyDiagnostic"), kit.ButtonOpts{Size: design.ControlSM, Variant: kit.Ghost}).Clicked() {
					c.WriteClipboard(a.diagnostic(&jj))
					a.toast(T("error.copied"), "", 1)
				}
			}
			ui.Spacer(c)
			if kit.Button(c.Key("pr"), T("builds.openProject"), kit.ButtonOpts{Size: design.ControlSM, Variant: kit.Ghost}).Clicked() {
				a.watch[j.ProjectID] = j.ID
				a.openProject(j.ProjectID, 0)
			}
		})
		var lines []kit.LogLine
		for _, l := range a.logs[j.ID] {
			lines = append(lines, kit.LogLine{Time: l.Time.Format("15:04:05"), Level: l.Level, Text: l.Message})
		}
		kit.LogViewer(c, lines, 180)
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
