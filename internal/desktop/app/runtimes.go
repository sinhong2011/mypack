package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
)

func (a *App) runtimesView(c *ui.Context) {
	p := design.Of(c)
	page(c, design.ContentMaxW, func() {
		kit.Secondary(c, T("runtimes.intro")).LineHeight(1.45)

		// Storage, as macOS's Storage settings: a total over a segmented bar.
		st := a.storage
		total := st.RuntimeCache + st.Artifacts + st.Profiles
		kit.Card(c).Padding(18, 20).Gap(14).Children(func() {
			ui.Row(c).AlignItems(ui.Baseline).Gap(8).Children(func() {
				kit.Heading(c, T("runtimes.storage"))
				ui.Spacer(c)
				ui.Text(c, bytesLabel(total)).FontSize(design.FontTitle1).FontWeight(design.WeightBold).TextColor(p.Text).FontFeatures("tnum")
			})
			cols := []struct {
				label string
				n     int64
				color ui.Color
			}{
				{T("runtimes.storage.runtimes"), st.RuntimeCache, p.Accent},
				{T("runtimes.storage.builds"), st.Artifacts, design.Blue[3]},
				{T("runtimes.storage.data"), st.Profiles, p.TextTertiary},
			}
			ui.Row(c).Height(16).Radius(8).Clip().Background(p.Fill).Gap(2).AlignItems(ui.Stretch).Children(func() {
				for i, s := range cols {
					if total > 0 && s.n > 0 {
						ui.Box(c.Key(i)).Grow(float32(s.n) / float32(total) * 100).Basis(0).
							LinearGradient(ui.LinearGradient{From: s.color.Mix(ui.RGB(255, 255, 255), 0.15), To: s.color, Angle: 180})
					}
				}
				if total == 0 {
					ui.Box(c).Grow(1)
				}
			})
			ui.Row(c).Gap(24).Wrap().Children(func() {
				for i, s := range cols {
					ui.Row(c.Key(i)).Gap(7).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(9, 9).Radius(4.5).Background(s.color)
						ui.Text(c, s.label).FontSize(design.FontSmall).TextColor(p.TextSecondary)
						ui.Text(c, bytesLabel(s.n)).FontSize(design.FontSmall).FontWeight(design.WeightSemibold).TextColor(p.Text).FontFeatures("tnum")
					})
				}
			})
			kit.Caption(c, T("runtimes.storage.note"))
		})

		// Runtime table.
		ui.Column(c).Gap(10).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(0, 4).Children(func() {
				ui.Text(c, T("runtimes.table")).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).Role(ui.RoleHeading)
				ui.Spacer(c)
				kit.Glyph(c, icons.Folder, 12, p.TextTertiary)
				kit.Mono(c, a.runtimeCacheDir()).FontSize(design.FontCaption).SingleLine()
			})
			ui.Column(c).Radius(design.RadiusCard).Background(p.Group).Border(0.5, p.GroupBorder).Clip().Role(ui.RoleTable).Label(T("runtimes.table")).Children(func() {
				kit.TableHeader(c, []kit.TableColumn{
					{Title: T("runtimes.col.platform")}, {Title: T("runtimes.col.version"), Width: 64},
					{Title: T("runtimes.col.size"), Width: 76, End: true}, {Title: T("runtimes.col.status"), Width: 130},
					{Title: T("runtimes.col.lastUsed"), Width: 104}, {Width: 150},
				}, 14)
				for i, rt := range a.runtimes {
					a.runtimeRow(c.Key(rt.Key+rt.Version), rt, i)
				}
			})
			kit.Footnote(c, T("runtimes.source", "url", a.settings.RegistryURL)).Padding(0, 4)
		})
	})
}

func (a *App) runtimeRow(c *ui.Context, rt domain.RuntimeInfo, i int) {
	p := design.Of(c)
	key := rt.Version + "/" + rt.Key
	busy := a.rtBusy[key]
	parts := strings.SplitN(rt.Key, "-", 2)
	osName, arch := parts[0], ""
	if len(parts) > 1 {
		arch = parts[1]
	}
	row := ui.Row(c).MinHeight(design.Row).PaddingX(14).Gap(12).AlignItems(ui.Center).Role(ui.RoleRow)
	kit.TableRowBG(c, row, i, false, false)
	row.Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
			icon := icons.Monitor
			if osName == "darwin" {
				icon = icons.Laptop
			}
			kit.Glyph(c, icon, 17, p.TextSecondary)
			ui.Column(c).Gap(0).MinWidth(0).Children(func() {
				ui.Text(c, osLabel(osName)+" · "+archLabel(osName, arch)).FontSize(design.FontBody).TextColor(p.Text).SingleLine()
				if osName == a.host.OS && arch == a.host.Arch {
					ui.Text(c, T("runtimes.thisDevice")).FontSize(design.FontCaption).TextColor(p.AccentText)
				}
			})
		})
		ui.Text(c, rt.Version).FontSize(design.FontSmall).TextColor(p.TextSecondary).Width(64).Shrink(0).FontFeatures("tnum")
		size := bytesLabel(rt.Size)
		if rt.Cached && rt.CacheSize > 0 {
			size = bytesLabel(rt.CacheSize)
		}
		ui.Text(c, size).FontSize(design.FontSmall).TextColor(p.TextSecondary).Width(76).Shrink(0).TextAlign(ui.End).FontFeatures("tnum")
		ui.Row(c).Width(130).Shrink(0).Children(func() {
			switch {
			case busy == "download":
				ui.Column(c).Grow(1).Gap(4).Children(func() {
					kit.Caption(c, fmt.Sprintf("%s %d%%", T("runtimes.downloading"), int(a.rtProg[key]*100)))
					kit.ProgressBar(c, a.rtProg[key])
				})
			case rt.Cached && rt.Verified:
				kit.StatusDot(c, T("runtimes.verified"), kit.ToneSuccess)
			case rt.Cached:
				kit.StatusDot(c, T("runtimes.cached"), kit.ToneWarning)
			default:
				kit.StatusDot(c, T("runtimes.notDownloaded"), kit.ToneNeutral)
			}
		})
		last := "—"
		if !rt.LastUsed.IsZero() {
			last = ago(rt.LastUsed)
		}
		ui.Text(c, last).FontSize(design.FontSmall).TextColor(p.TextSecondary).Width(104).Shrink(0).SingleLine()
		ui.Row(c).Width(150).Shrink(0).Gap(6).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			if !rt.Cached {
				if kit.Button(c.Key("dl"), T("action.download"), kit.ButtonOpts{Size: design.ControlSM, Icon: icons.Download, Loading: busy != ""}).Clicked() {
					a.runtimeAction(rt, "download")
				}
				return
			}
			if kit.Button(c.Key("v"), T("action.verify"), kit.ButtonOpts{Size: design.ControlSM, Loading: busy == "verify", Disabled: busy != ""}).Clicked() {
				a.runtimeAction(rt, "verify")
			}
			if kit.Button(c.Key("d"), "", kit.ButtonOpts{Size: design.ControlSM, Variant: kit.Ghost, Icon: icons.Trash,
				Tooltip: T("action.delete"), Disabled: busy != ""}).Label(T("action.delete")).Clicked() {
				a.runtimeAction(rt, "delete")
			}
		})
	})
}

func (a *App) runtimeAction(rt domain.RuntimeInfo, what string) {
	key := rt.Version + "/" + rt.Key
	a.rtBusy[key] = what
	a.rtProg[key] = 0
	a.do(func(ctx context.Context) error {
		switch what {
		case "download":
			_, err := a.svc.DownloadRuntime(ctx, rt.Version, rt.Key, func(done, total int64) {
				if total > 0 {
					f := float64(done) / float64(total)
					a.win.Update(func() { a.rtProg[key] = f })
				}
			})
			return err
		case "verify":
			return a.svc.VerifyRuntime(ctx, rt.Version, rt.Key)
		default:
			return a.svc.DeleteRuntime(ctx, rt.Version, rt.Key)
		}
	}, func(err error) {
		delete(a.rtBusy, key)
		if err != nil {
			a.toastErr(err)
		} else if what == "verify" {
			a.toast(T("runtimes.verifiedToast"), "", 1)
		}
		a.reload("runtimes", "storage")
	})
}
