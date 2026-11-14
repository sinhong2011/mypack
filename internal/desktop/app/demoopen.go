package app

import (
	"os"
	"os/exec"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/desktop/kit/icons"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/installer"
)

// chromeAppBins are browsers that open a URL as its own window (--app).
var chromeAppBins = []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"}

type demoLaunch struct {
	key, name, url, path string
	icon                 []byte
	width, height        int
}

func (a *App) installByID(id string) *domain.Installation {
	for i := range a.installs {
		if a.installs[i].ID == id {
			return &a.installs[i]
		}
	}
	return nil
}

func (a *App) demoFromInstall(id string) demoLaunch {
	in := a.installByID(id)
	if in == nil {
		return demoLaunch{}
	}
	d := demoLaunch{key: "inst:" + id, name: in.Name, url: in.SourceURL, path: in.Path, icon: in.IconPNG,
		width: 1200, height: 780}
	if pr := a.project(in.ProjectID); pr != nil {
		d.applyProject(pr)
	}
	return d
}

func (a *App) demoFromArtifact(id string) demoLaunch {
	for _, j := range a.jobs {
		for _, art := range j.Artifacts {
			if art.ID != id {
				continue
			}
			d := demoLaunch{key: "art:" + id, path: art.Path, width: 1200, height: 780}
			if j.Snapshot != nil {
				d.name = j.Snapshot.App.Name
				d.url = j.Snapshot.App.Source.Value
				d.width, d.height = j.Snapshot.Window.Width, j.Snapshot.Window.Height
			}
			if pr := a.project(j.ProjectID); pr != nil {
				d.applyProject(pr)
				if d.path == "" {
					d.path = art.Path
				}
			}
			return d
		}
	}
	return demoLaunch{}
}

func (d *demoLaunch) applyProject(pr *domain.Project) {
	if d.name == "" {
		d.name = pr.Config.App.Name
	}
	if d.url == "" {
		d.url = pr.Config.App.Source.Value
	}
	if len(d.icon) == 0 {
		d.icon = pr.IconPNG
	}
	if w, h := pr.Config.Window.Width, pr.Config.Window.Height; w > 0 && h > 0 {
		d.width, d.height = w, h
	}
}

func (a *App) launchDemo(d demoLaunch) {
	if d.name == "" && d.url == "" && d.path == "" {
		a.toast(T("demo.open.missing"), "", 2)
		return
	}
	if d.path != "" {
		if _, err := os.Stat(d.path); err == nil {
			if err := installer.Launch(d.path); err == nil {
				return
			}
			// Fake installs often have a leftover folder with no binary;
			// open the preview instead of failing silently.
		}
	}
	if d.key != "" {
		if w := a.demoApps[d.key]; w != nil && !w.IsDestroyed() {
			w.Show()
			w.Focus()
			_ = openSiteApp(d.url)
			return
		}
	}
	a.openDemoWindow(d)
	if err := openSiteApp(d.url); err != nil && d.url != "" {
		a.toastErr(err)
	}
}

func (a *App) openDemoWindow(d demoLaunch) {
	if a.demoApps == nil {
		a.demoApps = map[string]*mygo.Window{}
	}
	w, h := d.width, d.height
	if w < 480 {
		w = 1000
	}
	if h < 360 {
		h = 700
	}
	state := d
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: state.name, Width: w, Height: h, MinWidth: 480, MinHeight: 360,
		BackgroundColor: "light-dark(#efefeb, #141415)",
		Content:         ui.View(func(c *ui.Context) { a.demoAppView(c, state) }),
	})
	setWindowIcon(win, state.icon)
	if d.key != "" {
		key := d.key
		a.demoApps[key] = win
		win.OnClose(func(*mygo.CloseEvent) { delete(a.demoApps, key) })
	}
}

func (a *App) demoAppView(c *ui.Context, d demoLaunch) {
	p := design.Of(c)
	design.Apply(c)
	ui.Column(c).Fill().Background(p.CanvasBot).Center().Gap(18).Padding(40).Children(func() {
		kit.AppIconOpts(c, d.icon, d.name, 96, kit.IconOpts{Shadow: true})
		kit.LargeTitle(c, d.name)
		if d.url != "" {
			kit.Secondary(c, d.url).SingleLine()
		}
		kit.Caption(c, T("demo.open.body"))
		if d.url != "" {
			if kit.Button(c.Key("open"), T("demo.open.browser"), kit.ButtonOpts{Variant: kit.Primary, Size: design.ControlLG, Icon: icons.Globe}).Clicked() {
				if err := openSiteApp(d.url); err != nil {
					a.toastErr(err)
				}
			}
		}
	})
}

func openSiteApp(raw string) error {
	u := strings.TrimSpace(raw)
	if u == "" {
		return nil
	}
	if !strings.HasPrefix(strings.ToLower(u), "http://") && !strings.HasPrefix(strings.ToLower(u), "https://") {
		u = "https://" + u
	}
	for _, name := range chromeAppBins {
		bin, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		cmd := exec.Command(bin, "--app="+u, "--no-first-run")
		if err := cmd.Start(); err != nil {
			return err
		}
		_ = cmd.Process.Release()
		return nil
	}
	return installer.OpenURL(u)
}
