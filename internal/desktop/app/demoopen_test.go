package app

import (
	"testing"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

func TestDemoFromInstallUsesProject(t *testing.T) {
	a := &App{
		projects: []domain.Project{{
			ID: "p1",
			Config: &config.Config{
				App:    config.AppConfig{Name: "Linear", Source: config.Source{Value: "https://linear.app/"}},
				Window: config.WindowConfig{Width: 1200, Height: 800},
			},
			IconPNG: []byte("icon"),
		}},
		installs: []domain.Installation{{
			ID: "i1", ProjectID: "p1", Name: "Linear", SourceURL: "https://linear.app/",
			Path: "/no/such/Linear.app",
		}},
	}
	d := a.demoFromInstall("i1")
	if d.name != "Linear" || d.url != "https://linear.app/" || d.key != "inst:i1" {
		t.Fatalf("%+v", d)
	}
	if d.width != 1200 || d.height != 800 {
		t.Fatalf("window size %+v", d)
	}
	if string(d.icon) != "icon" {
		t.Fatalf("icon %q", d.icon)
	}
	if d.cfg == nil || d.cfg.App.Name != "Linear" {
		t.Fatalf("cfg %+v", d.cfg)
	}
}

func TestDemoDownloadDirUsesMyPackDefault(t *testing.T) {
	a := &App{}
	d := demoLaunch{cfg: &config.Config{}}
	if a.demoDownloadDir(d) != "" {
		t.Fatal("empty")
	}
	a.settings.DownloadDir = "~/Inbox"
	if a.demoDownloadDir(d) != "~/Inbox" {
		t.Fatal(a.demoDownloadDir(d))
	}
	d.cfg.Network.DownloadDirectory = "~/Linear"
	if a.demoDownloadDir(d) != "~/Linear" {
		t.Fatal("project folder should win")
	}
}

func TestDemoFromArtifactUsesSnapshot(t *testing.T) {
	cfg := config.New("Notion", config.Source{Type: config.SourceURL, Value: "https://www.notion.so/"}, config.Target{OS: "linux", Arch: "amd64"})
	a := &App{
		jobs: []domain.BuildJob{{
			ProjectID: "p2",
			Snapshot:  cfg,
			Artifacts: []domain.Artifact{{ID: "a1", Path: "/tmp/notion"}},
		}},
	}
	d := a.demoFromArtifact("a1")
	if d.name != "Notion" || d.url != "https://www.notion.so/" || d.key != "art:a1" {
		t.Fatalf("%+v", d)
	}
}

func TestOpenSiteAppEmpty(t *testing.T) {
	if err := openSiteApp(""); err != nil {
		t.Fatal(err)
	}
	if err := openSiteApp("   "); err != nil {
		t.Fatal(err)
	}
}
