package app

import (
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
)

// resultBar is a finished build, as one floating pill under the stage:
// the app (icon, name, state), a hairline, then its actions: Open (or a
// quiet "Opened" chip that offers Open again), Show in Finder, Rebuild,
// View log and a menu for the rest; under it one caption of size and time.
// A failure uses the same bar: a red state, Retry, View log, Copy
// diagnostic. Return opens (or retries), ⌘R rebuilds, ⌘L shows the log.
func (a *App) resultBar(c *ui.Context, pr *domain.Project, j *domain.BuildJob) {
	p := design.Of(c)
	si := a.successOf(pr, j)
	failed := j.Status == domain.JobFailed
	name := pr.Config.App.Name
	ui.Column(c.Key("result")).AlignItems(ui.Center).Gap(8).Children(func() {
		bar := ui.Row(c.Key("bar")).Height(56).Radius(28).Background(p.Raised).Border(1, p.Border).
			Padding(0, 12, 0, 22).Gap(12).AlignItems(ui.Center).MinWidth(420).MaxWidth(620)
		design.Elevate(c, bar, design.Floating)
		// It springs in from the progress block it replaces.
		bar.Transition(ui.ElementTransition{Animation: &design.SpringSoft, Enter: &ui.Motion{Scale: 0.9}})
		bar.Children(func() {
			ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, name).FontSize(14).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
				if failed {
					// Red is the word "failed" alone.
					ui.Row(c).Gap(3).Children(func() {
						ui.Text(c, T("result.failedPre")).FontSize(12).TextColor(p.TextSecondary).SingleLine()
						ui.Text(c, T("result.failedWord")).FontSize(12).TextColor(p.Danger).SingleLine()
						ui.Text(c, T("result.failedPost", "step", a.failedStep(pr, j))).FontSize(12).TextColor(p.TextSecondary).SingleLine().MinWidth(0)
					})
				} else {
					state, tone := a.resultState(c, si, j)
					ui.Text(c, state).FontSize(12).TextColor(tone).SingleLine()
				}
			})
			ui.Box(c).Width(1).Height(24).Background(p.Divider).Shrink(0)
			ui.Row(c).Gap(4).AlignItems(ui.Center).Shrink(0).Children(func() {
				if failed {
					if kit.Button(c.Key("retry"), T("action.retry"), kit.ButtonOpts{Variant: kit.Primary, Size: 36}).Clicked() || c.Shortcut(0, ui.KeyEnter) {
						a.startBuild(pr.ID, j.Targets)
					}
					a.resultIcon(c, "log", icons.Log, T("build.viewLog"), func() { a.logOpen[j.ID] = !a.logOpen[j.ID] })
					a.resultIcon(c, "diag", icons.Copy, T("error.copyDiagnostic"), func() {
						c.WriteClipboard(a.diagnostic(j))
						a.toast(T("error.copied"), "", 1)
					})
				} else {
					a.resultOpen(c, si)
					if si.art != nil {
						path := si.art.Path
						a.resultIcon(c, "finder", icons.Folder, T("action.showInFinder"), func() { a.reveal(path) })
					}
					a.resultIcon(c, "rebuild", icons.Retry, T("action.rebuild"), func() { a.startBuild(pr.ID, j.Targets) })
					a.resultIcon(c, "log", icons.Log, T("build.viewLog"), func() { a.logOpen[j.ID] = !a.logOpen[j.ID] })
				}
				more := kit.ToolbarIcon(c.Key("more"), icons.More, T("common.more"))
				more.Menu(func(m *ui.Menu) { a.resultMenu(m, pr, si) })
			})
		})
		if c.Shortcut(ui.Cmd, ui.KeyR) {
			a.startBuild(pr.ID, j.Targets)
		}
		if c.Shortcut(ui.Cmd, ui.KeyL) {
			a.logOpen[j.ID] = !a.logOpen[j.ID]
		}
		// One caption under the pill.
		caption := ""
		switch {
		case failed:
			// The failed step says what went wrong, just above.
		case si.art != nil && j.StartedAt != nil && j.FinishedAt != nil:
			caption = T("result.caption", "size", bytesLabel(si.art.Size), "time", duration(j.FinishedAt.Sub(*j.StartedAt)))
		}
		if caption != "" {
			ui.Text(c, caption).FontSize(design.FontCaption).TextColor(p.TextTertiary).FontFeatures("tnum").TextAlign(ui.Center).MaxWidth(560)
		}
	})
}

// resultState is the bar's second line and its colour.
func (a *App) resultState(c *ui.Context, si successInfo, j *domain.BuildJob) (string, ui.Color) {
	pal := design.Of(c)
	switch {
	case si.installing:
		return T("autoInstall.installing"), pal.TextSecondary
	case si.installed && si.auto && !si.opened.IsZero():
		return T("result.installedOpened", "when", agoShort(si.opened)), pal.TextSecondary
	case si.installed:
		return T("result.installed"), pal.TextSecondary
	}
	return T("result.builtNotInstalled"), pal.TextSecondary
}

// resultOpen is Open (installing first when needed), or, once it was
// opened for you, a quiet "✓ Opened" chip that offers Open again.
func (a *App) resultOpen(c *ui.Context, si successInfo) {
	p := design.Of(c)
	open := func() {
		switch {
		case si.installed:
			a.openInstall(si.inst.ID)
		case si.art != nil:
			a.installArtifact(si.art.ID, false)
		}
	}
	if si.installed && si.auto && !si.opened.IsZero() {
		chip := ui.ButtonBase(c.Key("opened")).Label(T("success.openAgain")).Row().Height(36).PaddingX(12).Gap(6).
			AlignItems(ui.Center).Radius(kit.PillRadius(36)).Cursor(ui.CursorPointer).FocusRing(false).Transition(design.Transition(c))
		again := chip.Hovered() || chip.FocusVisible()
		if again {
			chip.Background(p.Hover)
		}
		chip.Children(func() {
			if again {
				ui.Text(c, T("success.openAgain")).FontSize(design.FontBody).FontWeight(design.WeightMedium).TextColor(p.Text).SingleLine()
			} else {
				kit.Glyph(c, icons.Check, 14, p.Success)
				ui.Text(c, T("result.opened")).FontSize(design.FontBody).TextColor(p.TextSecondary).SingleLine()
			}
		})
		if chip.Clicked() || c.Shortcut(0, ui.KeyEnter) {
			open()
		}
		return
	}
	if si.art == nil || si.art.Target.OS != a.host.OS {
		return
	}
	if kit.Button(c.Key("open"), T("action.open"), kit.ButtonOpts{Variant: kit.Primary, Size: 36, Loading: si.installing}).Clicked() ||
		c.Shortcut(0, ui.KeyEnter) {
		open()
	}
}

// resultIcon is one of the bar's 32 pt circular icon buttons.
func (a *App) resultIcon(c *ui.Context, key string, ic *ui.SVG, label string, run func()) {
	if kit.ToolbarIcon(c.Key(key), ic, label).Clicked() {
		run()
	}
}

// resultMenu is the bar's "⋯": the rest of what a result offers.
func (a *App) resultMenu(m *ui.Menu, pr *domain.Project, si successInfo) {
	id := pr.ID
	if m.Item(T("create.viewProject")).Chosen() {
		a.create.reset(a.host)
		a.openProject(id, 0)
	}
	if m.Item(T("success.addAccount")).Chosen() {
		src := pr.Config.App.Source.Value
		a.create.reset(a.host)
		a.create.setInput(src)
		a.nav(secCreate)
	}
	if si.art != nil {
		path := si.art.Path
		if m.Item(T("action.copyPath")).Chosen() {
			go mygo.Clipboard.WriteText(path)
			a.toast(T("common.copied"), "", 1)
		}
		if m.Item(T("result.revealFolder")).Chosen() {
			a.reveal(filepath.Dir(path))
		}
	}
}
