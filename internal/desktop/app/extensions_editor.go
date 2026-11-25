package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/runtime/userscript"
)

// The script editor: an extension's source in MyGo's code editor (syntax
// highlighting, find, folding, several carets), with what its header says
// summed up above it, its header's problems marked in the text, the
// changes since the last save in the gutter and in a Changes tab, and Save
// (⌘S) that installs the edit in place, keeping the extension's state and
// the apps it runs in.

const newScriptTemplate = `// ==UserScript==
// @name         My Extension
// @namespace    https://mypack.local
// @version      0.1.0
// @description  What it does, in one sentence.
// @match        https://example.com/*
// @grant        none
// @run-at       document-idle
// ==/UserScript==

(function () {
  'use strict';
  // Runs on the pages @match names, in every app you choose.
  console.log('Hello from', GM_info.script.name);
})();
`

type extEditor struct {
	open    bool
	id      string // installed entry id; "" for a new extension
	st      *editor.State
	saved   *editor.Buffer // the text as last saved; the gutter's base
	diff    *editor.DiffState
	tab     int // 0 code, 1 changes
	checked uint64
	meta    userscript.Meta
	probs   []userscript.Problem
	busy    bool
	err     string
	leave   bool // the discard-changes sheet
}

func newEditorState(src string) (*editor.State, *editor.Buffer) {
	buf := editor.NewBuffer(src)
	buf.SetLanguage(editor.LanguageFor("script.user.js"))
	saved := editor.NewBuffer(src)
	saved.SetLanguage(buf.Language())
	return &editor.State{Buffer: buf, DiffBase: saved, Label: T("ext.editor.label"), WrapGuides: []int{100}}, saved
}

// openExtEditor opens an installed extension's source.
func (a *App) openExtEditor(id string) {
	b, err := os.ReadFile(filepath.Join(a.extStore().Dir, filepath.Base(id), "script.user.js"))
	if err != nil {
		a.toastErr(err)
		return
	}
	st, saved := newEditorState(string(b))
	a.ext.edit = extEditor{open: true, id: id, st: st, saved: saved}
	a.ext.detail.open = false
	pageScroll = ui.ScrollState{}
}

// newExtension opens the editor on a template.
func (a *App) newExtension() {
	st, saved := newEditorState(newScriptTemplate)
	saved.Reset("") // nothing saved yet: everything is a change
	a.ext.edit = extEditor{open: true, st: st, saved: saved}
}

func (ed *extEditor) modified() bool {
	return ed.st != nil && ed.st.Buffer.String() != ed.saved.String()
}

// closeExtEditor leaves the editor, asking first when there are changes.
func (a *App) closeExtEditor(force bool) {
	if !force && a.ext.edit.modified() {
		a.ext.edit.leave = true
		return
	}
	a.ext.edit = extEditor{}
}

// recheck parses the header again when the text changed, and marks its
// problems in the editor.
func (ed *extEditor) recheck() {
	v := ed.st.Buffer.Version()
	if v == ed.checked && ed.checked != 0 {
		return
	}
	ed.checked = v
	src := ed.st.Buffer.String()
	ed.probs = userscript.Check(src)
	if m, err := userscript.Parse(src); err == nil {
		ed.meta = m // a broken header keeps the last good summary
	}
	var ds []editor.Diagnostic
	for _, p := range ed.probs {
		line := min(p.Line, ed.st.Buffer.LineCount()-1)
		start := ed.st.Buffer.Offset(editor.Pos{Line: line})
		end := start + len(ed.st.Buffer.Line(line))
		sev := editor.SeverityError
		if p.Warning {
			sev = editor.SeverityWarning
		}
		ds = append(ds, editor.Diagnostic{Start: start, End: end, Severity: sev, Message: p.Message, Source: "MyPack"})
	}
	ed.st.SetDiagnostics(v, ds)
}

// saveBlocker says why the edit cannot be saved, or "".
func (a *App) saveBlocker() string {
	ed := &a.ext.edit
	if userscript.HasErrors(ed.probs) {
		return T("ext.editor.fixProblems")
	}
	id := ed.meta.ID()
	if ed.id != "" && id != ed.id {
		return T("ext.editor.renamed")
	}
	if ed.id == "" && a.installedByID(id) != nil {
		return T("ext.editor.taken", "name", ed.meta.Name)
	}
	return ""
}

func (a *App) saveExtEditor() {
	ed := &a.ext.edit
	if ed.busy || !ed.modified() || a.saveBlocker() != "" {
		return
	}
	ed.busy, ed.err = true, ""
	src := ed.st.Buffer.String()
	st := a.extStore()
	o := userscript.InstallOptions{}
	if e := a.installedByID(ed.id); e != nil {
		o.SourceURL, o.Edited = e.SourceURL, true
		o.Fetch = st.LocalFetcher(e.ID, nil)
	}
	go func() {
		e, _, err := st.Install(context.Background(), []byte(src), o)
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			ed := &a.ext.edit
			ed.busy = false
			if err != nil {
				ed.err = err.Error()
				return
			}
			isNew := ed.id == ""
			ed.id = e.ID
			ed.saved.Reset(src)
			ed.st.Buffer.MarkSaved()
			ed.diff = nil
			a.reloadInstalledExtensions()
			if isNew {
				a.toast(T("ext.installed.toast", "name", e.Name), T("ext.restartHint"), 1)
			} else {
				a.toast(T("ext.editor.saved", "name", e.Name), T("ext.restartHint"), 1)
			}
		})
	}()
}

// extEditorToolbar: Revert and Save, at the right of the toolbar.
func (a *App) extEditorToolbar(c *ui.Context) {
	ed := &a.ext.edit
	mod := ed.modified()
	if ed.id != "" && mod {
		if kit.Button(c.Key("revert"), T("ext.editor.revert"), kit.ButtonOpts{Variant: kit.Ghost, Size: design.ToolbarControl}).Clicked() {
			ed.st.Buffer.Set(ed.saved.String())
		}
	}
	block := a.saveBlocker()
	label := T("ext.editor.save")
	if ed.id == "" {
		label = T("ext.editor.install")
	}
	b := kit.Button(c.Key("save"), label, kit.ButtonOpts{Variant: kit.Primary, Size: design.ToolbarControl, Icon: icons.Check,
		Loading: ed.busy, Disabled: !mod || block != "", Hotkey: "mod+s"})
	if block != "" {
		b.Tooltip(block)
	}
	if b.Clicked() || c.Shortcut(ui.Cmd, ui.KeyS) {
		a.saveExtEditor()
	}
}

func (a *App) extEditorTitle() (string, string) {
	ed := &a.ext.edit
	name := ed.meta.Name
	if name == "" {
		name = T("ext.editor.untitled")
	}
	sub := T("ext.editor.saved.state")
	switch {
	case ed.id == "":
		sub = T("ext.editor.new")
	case ed.modified():
		sub = T("ext.editor.unsaved")
	}
	return name, sub
}

func (a *App) extEditorView(c *ui.Context) {
	ed := &a.ext.edit
	ed.recheck()
	p := design.Of(c)
	ui.Column(c.Key("ext-editor")).Grow(1).Basis(0).MinHeight(0).Gap(10).
		Padding(design.TitleBarH+8, design.PageGutter, 16, design.PageGutter).Children(func() {
		a.extEditorSummary(c)
		if e := a.installedByID(ed.id); e != nil && !e.Edited && a.hubFor(*e) != nil {
			kit.Note(c, icons.Info, T("ext.editor.reviewedNote"), kit.ToneNeutral)
		}
		if ed.err != "" {
			kit.Note(c, icons.Error, ed.err, kit.ToneDanger)
		} else if msg := a.saveBlocker(); msg != "" && ed.modified() {
			kit.Note(c, icons.Warning, msg, kit.ToneWarning)
		}
		// Code | Changes
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			kit.Segmented(c.Key("tab"), &ed.tab, []string{T("ext.editor.code"), T("ext.editor.changes")}, true)
			pos := ed.st.CursorPos()
			ui.Spacer(c)
			var errs, warns int
			for _, pr := range ed.probs {
				if pr.Warning {
					warns++
				} else {
					errs++
				}
			}
			if errs+warns > 0 {
				tone := kit.ToneWarning
				if errs > 0 {
					tone = kit.ToneDanger
				}
				kit.StatusPill(c, T("ext.editor.problems", "n", errs+warns), tone)
			}
			ui.Text(c, T("ext.editor.cursor", "line", pos.Line+1, "col", pos.Col+1)).FontSize(design.FontCaption).TextColor(p.TextTertiary)
			ui.Text(c, "JavaScript").FontSize(design.FontCaption).TextColor(p.TextTertiary)
		})
		frame := ui.Box(c.Key("frame")).Grow(1).Basis(0).MinHeight(0).Radius(design.RadiusCard).Border(0.5, p.GroupBorder).Clip()
		frame.Children(func() {
			if ed.tab == 1 {
				if !ed.modified() {
					kit.EmptyState(c, icons.Check, T("ext.editor.noChanges"), T("ext.editor.noChanges.body"), nil)
					return
				}
				if ed.diff == nil {
					ed.diff = &editor.DiffState{Old: ed.saved, New: ed.st.Buffer, Layout: editor.DiffUnified}
				}
				editor.DiffView(c.Key("diff"), ed.diff).Fill()
				return
			}
			editor.View(c.Key("code"), ed.st).Fill()
		})
	})
	if c.Shortcut(ui.Cmd, ui.KeyF) && ed.tab == 0 {
		ed.st.Run(editor.Find)
	}
	a.extLeaveSheet(c)
}

// extEditorSummary sums up the header as it is typed.
func (a *App) extEditorSummary(c *ui.Context) {
	ed := &a.ext.edit
	m := ed.meta
	p := design.Of(c)
	ui.Row(c).Gap(14).AlignItems(ui.Center).Padding(10, 14).Radius(design.RadiusCard).Background(p.Raised).Border(0.5, p.GroupBorder).Children(func() {
		name := m.Name
		if name == "" {
			name = "?"
		}
		kit.Placeholder(c, name, 34)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).SingleLine()
				if m.Version != "" {
					ui.Text(c, m.Version).FontSize(design.FontCaption).TextColor(p.TextTertiary)
				}
				if e := a.installedByID(ed.id); e != nil && e.Edited {
					kit.StatusPill(c, T("ext.editor.editedPill"), kit.ToneAccent)
				}
			})
			var parts []string
			if len(m.Match)+len(m.Include) > 0 || m.Name != "" {
				parts = append(parts, T("ext.editor.runsOn", "sites", sitesLabel(m.Sites())))
			}
			parts = append(parts, strings.Join(grantLines(m), " · "))
			if m.RunAt != "" {
				parts = append(parts, runAtLabel(m.RunAt))
			}
			ui.Text(c, strings.Join(parts, "   ·   ")).FontSize(design.FontCaption).TextColor(p.TextSecondary).SingleLine()
		})
	})
}

// extLeaveSheet asks before unsaved changes are thrown away.
func (a *App) extLeaveSheet(c *ui.Context) {
	ed := &a.ext.edit
	if !ed.leave {
		return
	}
	kit.Sheet(c.Key("ext-leave"), &ed.leave, 420, func() {
		kit.SheetHeader(c, T("ext.editor.leave.title"), nil)
		kit.SheetBody(c, T("ext.editor.leave.body"))
		ok, cancel := kit.SheetFooter(c, T("common.cancel"), T("ext.editor.leave.discard"), kit.Danger, false, false)
		if cancel {
			ed.leave = false
		}
		if ok {
			a.closeExtEditor(true)
		}
	})
}
