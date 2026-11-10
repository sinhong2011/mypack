package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/components"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
	"mypack.dev/mypack/internal/domain"
)

func (a *App) runtimesView(c *ui.Context) {
	p := design.Of(c)
	page(c, design.ContentMaxW, func() {
		components.Secondary(c, T("runtimes.intro")).LineHeight(1.45)

		// Storage breakdown.
		st := a.storage
		total := st.RuntimeCache + st.Artifacts + st.Profiles
		components.Card(c).Padding(18).Gap(14).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				components.Heading(c, T("runtimes.storage"))
				ui.Spacer(c)
				ui.Text(c, bytesLabel(total)).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(p.Text).FontFeatures("tnum")
			})
			cols := []struct {
				label string
				n     int64
				color ui.Color
			}{
				{T("runtimes.storage.runtimes"), st.RuntimeCache, p.Accent},
				{T("runtimes.storage.builds"), st.Artifacts, ui.RGB(124, 110, 245)},
				{T("runtimes.storage.data"), st.Profiles, ui.RGB(54, 178, 160)},
			}
			ui.Row(c).Height(8).Radius(4).Clip().Background(p.Hover).Gap(2).AlignItems(ui.Stretch).Children(func() {
				for i, s := range cols {
					if total > 0 && s.n > 0 {
						ui.Box(c.Key(i)).Grow(float32(s.n) / float32(total) * 100).Basis(0).Background(s.color)
					}
				}
				if total == 0 {
					ui.Box(c).Grow(1)
				}
			})
			ui.Row(c).Gap(24).Wrap().Children(func() {
				for i, s := range cols {
					ui.Row(c.Key(i)).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(8, 8).Radius(2).Background(s.color)
						components.Secondary(c, s.label).FontSize(design.FontSmall)
						ui.Text(c, bytesLabel(s.n)).FontSize(design.FontSmall).FontWeight(design.WeightMedium).TextColor(p.Text).FontFeatures("tnum")
					})
				}
			})
			components.Caption(c, T("runtimes.storage.note"))
		})

		// Runtime table.
		ui.Column(c).Gap(8).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				components.Heading(c, T("runtimes.table"))
				ui.Spacer(c)
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, lobeui.IconFolder()).FontSize(13).TextColor(p.TextTertiary)
					components.Mono(c, a.runtimeCacheDir())
				})
			})
			ui.Column(c).Background(p.Surface).Border(1, p.Border).Radius(design.RadiusCard-2).Dividers(1, p.Divider).Children(func() {
				ui.Row(c).Height(34).PaddingX(14).Gap(12).AlignItems(ui.Center).Children(func() {
					head := func(s string, w float32) {
						e := ui.Text(c, s).FontSize(design.FontCaption).FontWeight(design.WeightSemibold).TextColor(p.TextTertiary)
						if w > 0 {
							e.Width(w).Shrink(0)
						} else {
							e.Grow(1).Basis(0)
						}
					}
					head(T("runtimes.col.platform"), 0)
					head(T("runtimes.col.version"), 70)
					head(T("runtimes.col.size"), 80)
					head(T("runtimes.col.status"), 120)
					head(T("runtimes.col.lastUsed"), 110)
					head("", 150)
				})
				for _, rt := range a.runtimes {
					a.runtimeRow(c.Key(rt.Key+rt.Version), rt)
				}
			})
			components.Caption(c, T("runtimes.source", "url", a.settings.RegistryURL))
		})
	})
}

func (a *App) runtimeRow(c *ui.Context, rt domain.RuntimeInfo) {
	p := design.Of(c)
	key := rt.Version + "/" + rt.Key
	busy := a.rtBusy[key]
	parts := strings.SplitN(rt.Key, "-", 2)
	osName, arch := parts[0], ""
	if len(parts) > 1 {
		arch = parts[1]
	}
	ui.Row(c).MinHeight(design.Row).PaddingX(14).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
			icon := lobeui.IconMonitor()
			if osName == a.host.OS && arch == a.host.Arch {
				icon = lobeui.IconLaptop()
			}
			ui.Icon(c, icon).FontSize(16).TextColor(p.TextSecondary)
			ui.Column(c).Gap(1).MinWidth(0).Children(func() {
				ui.Text(c, osLabel(osName)+" · "+archLabel(osName, arch)).FontSize(design.FontBody).TextColor(p.Text).SingleLine()
				if osName == a.host.OS && arch == a.host.Arch {
					components.Caption(c, T("runtimes.thisDevice"))
				}
			})
		})
		ui.Text(c, rt.Version).FontSize(design.FontSmall).TextColor(p.TextSecondary).Width(70).Shrink(0).FontFeatures("tnum")
		size := bytesLabel(rt.Size)
		if rt.Cached && rt.CacheSize > 0 {
			size = bytesLabel(rt.CacheSize)
		}
		ui.Text(c, size).FontSize(design.FontSmall).TextColor(p.TextSecondary).Width(80).Shrink(0).FontFeatures("tnum")
		ui.Row(c).Width(120).Shrink(0).Children(func() {
			switch {
			case busy == "download":
				ui.Column(c).Grow(1).Gap(4).Children(func() {
					components.Caption(c, fmt.Sprintf("%s %d%%", T("runtimes.downloading"), int(a.rtProg[key]*100)))
					lobeui.Progress(c, a.rtProg[key])
				})
			case rt.Cached && rt.Verified:
				components.StatusPill(c, T("runtimes.verified"), components.ToneSuccess)
			case rt.Cached:
				components.StatusPill(c, T("runtimes.cached"), components.ToneWarning)
			default:
				components.StatusPill(c, T("runtimes.notDownloaded"), components.ToneNeutral)
			}
		})
		last := "—"
		if !rt.LastUsed.IsZero() {
			last = ago(rt.LastUsed)
		}
		ui.Text(c, last).FontSize(design.FontSmall).TextColor(p.TextTertiary).Width(110).Shrink(0)
		ui.Row(c).Width(150).Shrink(0).Gap(6).Justify(ui.End).Children(func() {
			if !rt.Cached {
				if components.Button(c.Key("dl"), T("action.download"), components.ButtonOpts{Size: design.ControlSM, Icon: lobeui.IconDownload(), Loading: busy != ""}).Clicked() {
					a.runtimeAction(rt, "download")
				}
				return
			}
			if components.Button(c.Key("v"), T("action.verify"), components.ButtonOpts{Size: design.ControlSM, Loading: busy == "verify", Disabled: busy != ""}).Clicked() {
				a.runtimeAction(rt, "verify")
			}
			if components.Button(c.Key("d"), "", components.ButtonOpts{Size: design.ControlSM, Variant: components.Ghost, Icon: lobeui.IconTrash(),
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
