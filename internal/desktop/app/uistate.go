package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// uiState is GUI-only presentation state persisted across restarts in
// <UserConfigDir>/mypack/ui-state.json (owned by the desktop package; not
// project data, so it doesn't go through the application service).
type uiState struct {
	Section      string          `json:"section"`
	ProjectID    string          `json:"projectId,omitempty"`
	LibraryTab   int             `json:"libraryTab"` // 0 My Apps, 1 Projects
	SettingsOpen map[string]bool `json:"advancedOpen,omitempty"`
}

func uiStatePath() string {
	if p := os.Getenv("MYPACK_UI_STATE"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "mypack", "ui-state.json")
}

func loadUIState() uiState {
	s := uiState{Section: string(secLibrary), LibraryTab: -1}
	path := uiStatePath()
	if path == "" {
		return s
	}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	if s.SettingsOpen == nil {
		s.SettingsOpen = map[string]bool{}
	}
	return s
}

func (s uiState) save() {
	path := uiStatePath()
	if path == "" {
		return
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}
