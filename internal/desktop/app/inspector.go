package app

import (
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
)

// Preview + inspector: the left column is the live preview (flexible);
// the right is a fixed inspector of settings that scrolls on its own, set
// off by a hairline, with its primary action pinned in a footer. Below
// ~900 pt the inspector moves under the preview in one column. Tab order
// follows: the preview's controls, then the inspector's, then its footer.

const (
	inspectorW      float32 = 364
	inspectorNarrow float32 = 900
)

func (a *App) previewInspector(c *ui.Context, preview, inspector, footer func()) {
	p := design.Of(c)
	narrow := a.toolbarW > 0 && a.toolbarW < inspectorNarrow-design.SidebarWidth
	if narrow {
		ui.Scroll(c).Grow(1).Basis(0).ScrollbarInsets(design.TitleBarH, 0, 0).TrackScroll(&pageScroll).Children(func() {
			ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(design.TitleBarH+12, 24, 32, 24).Children(func() {
				ui.Column(c).FillWidth().MaxWidth(560).Gap(24).Children(func() {
					preview()
					ui.Box(c).Height(1).FillWidth().Background(p.Divider)
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
		ui.Scroll(c).Grow(1).Basis(0).MinWidth(0).ScrollbarInsets(design.TitleBarH, 0, 0).TrackScroll(&pageScroll).Children(func() {
			ui.Column(c).FillWidth().MinHeightPercent(100).AlignItems(ui.Center).Padding(design.TitleBarH+12, 32, 32, 32).Children(func() {
				ui.Column(c).FillWidth().MaxWidth(620).Gap(0).Children(preview)
			})
		})
		// The inspector: a hairline, its own scroll, a pinned footer.
		ui.Column(c).Width(inspectorW).Shrink(0).BorderWidth(0, 0, 0, 1).BorderColor(p.Divider).Children(func() {
			ui.Scroll(c.Key("inspector:"+string(a.section)+a.projectID)).Grow(1).Basis(0).ScrollbarInsets(design.TitleBarH, 0, 0).Children(func() {
				ui.Column(c).FillWidth().Gap(22).Padding(design.TitleBarH+8, 20, 24, 20).Children(inspector)
			})
			if footer != nil {
				ui.Column(c).Gap(10).Padding(14, 20, 18, 20).BorderWidth(1, 0, 0, 0).BorderColor(p.Divider).Children(footer)
			}
		})
	})
}
