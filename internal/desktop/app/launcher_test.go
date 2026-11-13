package app

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/fakesvc"
)

func launcherApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("MYPACK_UI_STATE", filepath.Join(t.TempDir(), "ui-state.json"))
	svc := fakesvc.New()
	ctx := context.Background()
	a := &App{svc: svc}
	a.projects, _ = svc.ListProjects(ctx)
	a.installs, _ = svc.ListInstallations(ctx)
	a.settings, _ = svc.Settings(ctx)
	a.loaded = true
	return a
}

func names(items []launchItem) (out []string) {
	for _, it := range items {
		out = append(out, it.name)
	}
	return out
}

func TestLauncherItems(t *testing.T) {
	a := launcherApp(t)
	for _, n := range []string{"Workspace", "YouTube Music"} {
		for _, in := range a.installs {
			if in.Name == n {
				a.ui.togglePin(installPinKey(&in))
			}
		}
	}
	// No query: the pins in pin order, then the other apps by name; no drafts.
	got := names(a.launcherItems(""))
	want := []string{"Workspace", "YouTube Music", "ChatGPT", "Linear", "WhatsApp"}
	if len(got) < len(want) {
		t.Fatalf("items %v", got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("no query: %v, want %v first", got, want)
		}
	}
	for _, n := range got {
		if n == "Excalidraw" {
			t.Errorf("a draft listed without a query: %v", got)
		}
	}
	// A query: the best match first; drafts after apps.
	if got := names(a.launcherItems("you")); len(got) == 0 || got[0] != "YouTube Music" {
		t.Errorf("you: %v", got)
	}
	if got := names(a.launcherItems("excal")); len(got) != 1 || got[0] != "Excalidraw" {
		t.Errorf("excal: %v", got)
	}
	// A website address offers to make an app of it.
	if got := a.launcherItems("example.com"); len(got) == 0 || got[len(got)-1].key != "make" {
		t.Errorf("example.com: %v", names(got))
	}
	// A group's subtitle is its size.
	for _, it := range a.launcherItems("work") {
		if it.name == "Workspace" && it.sub != T("group.sites", "n", 3) {
			t.Errorf("Workspace sub %q", it.sub)
		}
	}
}

// TestLauncherView draws the panel, and the arrows move the selection.
func TestLauncherView(t *testing.T) {
	a := launcherApp(t)
	tt := ui.NewTester(a.launcherView, launcherW, 520)
	tt.Frame()
	if !tt.HasText(T("launcher.showInMyPack")) || !tt.HasText("Linear") {
		t.Fatalf("texts %v", tt.Texts())
	}
	a.launcher.input.Focus()
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if a.launcher.sel != 2 {
		t.Errorf("sel %d after two Downs, want 2", a.launcher.sel)
	}
	if dir := os.Getenv("SHOT_DIR"); dir != "" {
		for _, dark := range []bool{false, true} {
			tt.SetDark(dark)
			tt.Frame()
			name := "launcher-light.png"
			if dark {
				name = "launcher-dark.png"
			}
			f, _ := os.Create(filepath.Join(dir, name))
			_ = png.Encode(f, tt.Image())
			f.Close()
		}
	}
}
