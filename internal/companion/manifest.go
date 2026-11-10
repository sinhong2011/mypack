package companion

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// Manifest is the Native Messaging host manifest (Chrome/Edge flavour).
type Manifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

var extIDRe = regexp.MustCompile(`^[a-p]{32}$`)

// ValidExtensionID reports whether id looks like a Chromium extension ID.
func ValidExtensionID(id string) bool { return extIDRe.MatchString(id) }

// BuildManifest builds the manifest for an absolute host path.
func BuildManifest(hostPath string, extensionIDs []string) (Manifest, error) {
	if !filepath.IsAbs(hostPath) {
		return Manifest{}, errors.New("host path must be absolute")
	}
	if len(extensionIDs) == 0 {
		return Manifest{}, errors.New("at least one --extension-id is required")
	}
	var origins []string
	for _, id := range extensionIDs {
		if !ValidExtensionID(id) {
			return Manifest{}, fmt.Errorf("invalid extension id %q (expected 32 chars a-p)", id)
		}
		origins = append(origins, "chrome-extension://"+id+"/")
	}
	sort.Strings(origins)
	return Manifest{
		Name:           HostName,
		Description:    "mypack Companion native messaging host",
		Path:           hostPath,
		Type:           "stdio",
		AllowedOrigins: origins,
	}, nil
}

// ManifestDirs returns the per-user NativeMessagingHosts directories for the
// given OS ("darwin", "linux") keyed by browser ("chrome", "edge"). browser is
// "chrome", "edge" or "all".
func ManifestDirs(goos, home, browser string) (map[string]string, error) {
	var chrome, edge string
	switch goos {
	case "darwin":
		base := filepath.Join(home, "Library", "Application Support")
		chrome = filepath.Join(base, "Google", "Chrome", "NativeMessagingHosts")
		edge = filepath.Join(base, "Microsoft Edge", "NativeMessagingHosts")
	case "linux":
		chrome = filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts")
		edge = filepath.Join(home, ".config", "microsoft-edge", "NativeMessagingHosts")
	default:
		return nil, fmt.Errorf("no file-based registration on %s", goos)
	}
	out := map[string]string{}
	switch browser {
	case "chrome":
		out["chrome"] = chrome
	case "edge":
		out["edge"] = edge
	case "all", "":
		out["chrome"], out["edge"] = chrome, edge
	default:
		return nil, fmt.Errorf("unknown browser %q (chrome|edge|all)", browser)
	}
	return out, nil
}

// WriteManifest writes dir/dev.mypack.companion.json (0644, dir 0755).
func WriteManifest(dir string, m Manifest) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, HostName+".json")
	return p, os.WriteFile(p, append(b, '\n'), 0o644)
}

// RemoveManifest deletes the manifest in dir; a missing file is not an error.
func RemoveManifest(dir string) (string, bool, error) {
	p := filepath.Join(dir, HostName+".json")
	err := os.Remove(p)
	if errors.Is(err, os.ErrNotExist) {
		return p, false, nil
	}
	return p, err == nil, err
}

// RegistryKeys are the HKCU keys used on Windows (value (Default) = manifest path).
var RegistryKeys = map[string]string{
	"chrome": `HKCU\Software\Google\Chrome\NativeMessagingHosts\` + HostName,
	"edge":   `HKCU\Software\Microsoft\Edge\NativeMessagingHosts\` + HostName,
}
