package app

import (
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
)

// Preview + inspector: the left column is the live preview (flexible);
// the right is an inspector of settings (inspectorWidth) that scrolls on its own, set
// off by a hairline, with its primary action pinned in a footer. Below
// ~900 pt the inspector moves under the preview in one column. Tab order
// follows: the preview's controls, then the inspector's, then its footer.
//
// The inspector's left edge is a drag handle. A double-click restores the
// default width. The chosen width is kept in ui-state.json.

const (
	inspectorNarrow     float32 = 960 // window width below which it is one column
	inspectorMin        float32 = 400
	inspectorDefaultMax float32 = 480 // cap of the automatic width
	inspectorMax        float32 = 720 // farthest a drag may go
	inspectorPreviewMin float32 = 460 // preview keeps at least this beside it
	inspectorResizeGrip float32 = 6
)

// inspectorWidth is the settings column: a remembered drag, else ~38 % of
// the content area between inspectorMin and inspectorDefaultMax.
func (a *App) inspectorWidth() float32 {
	if a.ui.InspectorWidth > 0 {
		return a.clampInspectorWidth(a.ui.InspectorWidth)
	}
	return a.clampInspectorWidth(min(max(a.toolbarW*0.38, inspectorMin), inspectorDefaultMax))
}

func (a *App) clampInspectorWidth(w float32) float32 {
	maxW := inspectorMax
	if a.toolbarW > 0 {
		maxW = min(maxW, max(a.toolbarW-inspectorPreviewMin, inspectorMin))
	}
	return min(max(w, inspectorMin), maxW)
}

func (a *App) setInspectorWidth(w float32) {
	a.ui.InspectorWidth = a.clampInspectorWidth(w)
}

// head, when given, is pinned above the inspector's scroll (its search).
// inspectorNarrow reports whether preview and inspector stack in one
// column (no room for both side by side).
func (a *App) inspectorNarrow() bool {
	return a.toolbarW > 0 && (a.toolbarW < inspectorNarrow-design.SidebarWidth-design.CanvasInset || a.toolbarW-a.inspectorWidth() < inspectorPreviewMin)
}

func (a *App) previewInspector(c *ui.Context, preview, inspector, footer func(), head ...func()) {
	p := design.Of(c)
	// One column when the preview would get less than ~460 pt beside the
	// inspector (or the window is under ~960 pt): never clip the inspector.
	narrow := a.inspectorNarrow()
	if narrow {
		ui.Scroll(c).Grow(1).Basis(0).ScrollbarInsets(design.TitleBarH, 0, 0).TrackScroll(&pageScroll).Children(func() {
			ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(design.TitleBarH+12, 24, 32, 24).Children(func() {
				ui.Column(c).FillWidth().MaxWidth(560).Gap(24).Children(func() {
					preview()
					ui.Box(c).Height(1).FillWidth().Background(p.Divider)
					for _, h := range head {
						h()
					}
					inspector()
					if footer != nil {
						ui.Column(c).Gap(10).Children(footer)
					}
				})
			})
		})
		return
	}
	ui.Row(c).Absolute().Fill().AlignItems(ui.Stretch).Children(func() {
		// The preview: centred in the space left of the inspector.
		sc := ui.Scroll(c).Grow(1).Basis(0).MinWidth(0).ScrollbarInsets(design.TitleBarH, 0, 0).TrackScroll(&pageScroll)
		a.previewBox = sc.Bounds()
		sc.Children(func() {
			ui.Column(c).FillWidth().MinHeightPercent(100).AlignItems(ui.Center).Padding(design.TitleBarH+12, 32, 32, 32).Children(func() {
				ui.Column(c).FillWidth().MaxWidth(720).Grow(1).Gap(0).AlignItems(ui.Center).Children(preview)
			})
		})
		// The inspector: a hairline, its own scroll, a pinned footer.
		ui.Column(c).Width(a.inspectorWidth()).Shrink(0).BorderWidth(0, 0, 0, 1).BorderColor(p.Divider).Children(func() {
			a.inspectorResize(c)
			top, inset := design.TitleBarH+12, design.TitleBarH
			if len(head) > 0 {
				// Pinned under the title bar, level with the preview's first row.
				ui.Column(c.Key("head")).Padding(design.TitleBarH+12, 20, 12, 20).Children(head[0])
				top, inset = 4, 0
			}
			ui.Scroll(c.Key("inspector:"+string(a.section)+a.projectID)).Grow(1).Basis(0).ScrollbarInsets(inset, 0, 0).Children(func() {
				// Its first heading lines up with the preview's first row.
				ui.Column(c).FillWidth().Gap(22).Padding(top, 20, 24, 20).Children(inspector)
			})
			if footer != nil {
				ui.Column(c).Gap(10).Padding(14, 20, 18, 20).BorderWidth(1, 0, 0, 0).BorderColor(p.Divider).Children(footer)
			}
		})
	})
}

// inspectorResize is the settings column's left edge: drag to resize,
// double-click to restore the default width.
func (a *App) inspectorResize(c *ui.Context) {
	p := design.Of(c)
	edge := ui.Box(c.Key("insp-resize")).Absolute().Left(-inspectorResizeGrip / 2).Top(0).Width(inspectorResizeGrip).FillHeight().
		Cursor(ui.CursorResizeEW).Role(ui.RoleSeparator).Label(T("inspector.resize"))
	if edge.Hovered() || a.inspDrag {
		edge.Background(p.Accent)
	}
	dx, _, held := edge.Dragged()
	if held && dx != 0 {
		a.setInspectorWidth(a.inspectorWidth() - dx)
	}
	if edge.DoubleClicked() {
		a.ui.InspectorWidth = 0
		a.ui.save()
	} else if a.inspDrag && !held {
		a.ui.save()
	}
	a.inspDrag = held
}
