package app

import (
	"path/filepath"
	"testing"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

func TestPins(t *testing.T) {
	t.Setenv("MYPACK_UI_STATE", filepath.Join(t.TempDir(), "ui-state.json"))
	a := &App{}
	draft := domain.Project{ID: "prj-1", Config: config.New("Notion", config.Source{Type: config.SourceURL, Value: "https://notion.so"}, config.Target{})}
	other := domain.Project{ID: "prj-2", Config: config.New("Linear", config.Source{Type: config.SourceURL, Value: "https://linear.app"}, config.Target{})}
	a.projects = []domain.Project{draft, other}
	imported := domain.Installation{ID: "inst-x", Name: "ChatGPT", State: domain.InstallExternal}
	a.installs = []domain.Installation{imported}

	a.ui.togglePin("prj-2")
	a.ui.togglePin("prj-1")
	a.ui.togglePin(installPinKey(&imported))
	names := func() (out []string) {
		for _, it := range a.pinnedItems() {
			out = append(out, it.name())
		}
		return out
	}
	if got := names(); len(got) != 3 || got[0] != "Linear" || got[1] != "Notion" || got[2] != "ChatGPT" {
		t.Fatalf("pinned %v, want pin order Linear Notion ChatGPT", got)
	}
	// The draft gets installed: still pinned, now as the installed app.
	a.installs = append(a.installs, domain.Installation{ID: "inst-1", ProjectID: "prj-1", Name: "Notion"})
	if it := a.pinnedItems()[1]; it.install == nil || it.install.ID != "inst-1" {
		t.Errorf("installed draft pin = %+v, want the installation", it)
	}
	// Unpin, and a pin whose app is gone is skipped.
	a.ui.togglePin("prj-2")
	a.projects = a.projects[:1]
	a.ui.togglePin("prj-gone")
	if got := names(); len(got) != 2 || got[0] != "Notion" || got[1] != "ChatGPT" {
		t.Errorf("after unpin %v, want Notion ChatGPT", got)
	}
	// Saved across restarts.
	if got := loadUIState().Pinned; len(got) != 3 {
		t.Errorf("saved pins %v", got)
	}
}
