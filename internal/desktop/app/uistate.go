package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// uiState is GUI-only presentation state persisted across restarts in
// <UserConfigDir>/mypack/ui-state.json (owned by the desktop package; not
// project data, so it doesn't go through the application service).
type uiState struct {
	Section    string `json:"section"`
	ProjectID  string `json:"projectId,omitempty"`
	LibraryTab int    `json:"libraryTab"` // 0 My Apps, 1 Projects
	// LibrarySort is the Library's order: name, modified or status.
	LibrarySort string `json:"librarySort,omitempty"`
	// Pinned is the apps pinned to the sidebar, in pin order: project IDs,
	// or installation IDs for apps with no project (pins.go).
	Pinned []string `json:"pinned,omitempty"`
	// LauncherOff turns off the quick launcher and its global shortcut
	// (launcher.go).
	LauncherOff  bool            `json:"launcherOff,omitempty"`
	SettingsOpen map[string]bool `json:"advancedOpen,omitempty"`
	SettingsPane string          `json:"settingsPane,omitempty"` // Settings' category on show
	// AutoInstall is, per project, whether a successful build for this
	// device is installed and opened at once; AutoInstallDefault (nil =
	// on) seeds new projects.
	AutoInstall        map[string]bool `json:"autoInstall,omitempty"`
	AutoInstallDefault *bool           `json:"autoInstallDefault,omitempty"`
	// ShowHUD (nil = on) shows build progress near the notch while
	// MyPack's window is hidden or minimised.
	ShowHUD *bool `json:"showHud,omitempty"`
	// LookDefaults are the Look & Behaviour choices last made on Create,
	// which seed the next new app (lookdefaults.go).
	LookDefaults *lookDefaults `json:"lookDefaults,omitempty"`
	// LastUpdateCheck is when MyPack last asked for a new release, which
	// spaces automatic checks a day apart; UpdateNotified is the newest
	// version a toast told of, so each is told once.
	LastUpdateCheck time.Time `json:"lastUpdateCheck,omitempty"`
	UpdateNotified  string    `json:"updateNotified,omitempty"`
	// Star is when to ask for a star on GitHub (starask.go).
	Star starAsk `json:"star"`
}

// autoInstallDefault is the default for new projects (on unless turned off).
func (u *uiState) autoInstallDefault() bool {
	return u.AutoInstallDefault == nil || *u.AutoInstallDefault
}

// autoInstall reports whether project id installs and opens after a build.
func (u *uiState) autoInstall(id string) bool {
	if v, ok := u.AutoInstall[id]; ok {
		return v
	}
	return u.autoInstallDefault()
}

func (u *uiState) setAutoInstall(id string, on bool) {
	if u.AutoInstall == nil {
		u.AutoInstall = map[string]bool{}
	}
	u.AutoInstall[id] = on
	u.save()
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
