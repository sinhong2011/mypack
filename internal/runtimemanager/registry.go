package runtimemanager

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// SchemaVersion is the registry manifest schema this code understands.
const SchemaVersion = 1

// Manifest is registry.json.
type Manifest struct {
	SchemaVersion int       `json:"schemaVersion"`
	Sequence      int64     `json:"sequence"`
	Published     time.Time `json:"published"`
	Expires       time.Time `json:"expires"`
	Runtimes      []Entry   `json:"runtimes"`
}

// Entry is one downloadable runtime archive.
type Entry struct {
	Version      string   `json:"version"`
	OS           string   `json:"os"`
	Arch         string   `json:"arch"`
	Filename     string   `json:"filename"`
	URL          string   `json:"url"` // absolute, or relative to the registry URL
	SHA256       string   `json:"sha256"`
	Size         int64    `json:"size"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// Key is the "<os>-<arch>" key of the entry.
func (e Entry) Key() string { return e.OS + "-" + e.Arch }

// HasCapability reports whether the entry advertises capability c.
func (e Entry) HasCapability(c string) bool {
	for _, x := range e.Capabilities {
		if x == c {
			return true
		}
	}
	return false
}

// Find returns the entry for version/key.
func (m *Manifest) Find(version, key string) (Entry, bool) {
	for _, e := range m.Runtimes {
		if e.Version == version && e.Key() == key {
			return e, true
		}
	}
	return Entry{}, false
}

// ExecutableName is the binary inside every runtime archive for an os-arch key.
func ExecutableName(key string) string {
	if strings.HasPrefix(key, "windows-") {
		return "mypack-runtime.exe"
	}
	return "mypack-runtime"
}

// SigURL derives the detached signature location of a registry URL.
func SigURL(registryURL string) string {
	u, err := url.Parse(registryURL)
	if err != nil {
		return registryURL + ".sig"
	}
	u.Path += ".sig"
	u.RawQuery = ""
	return u.String()
}

// VerifyManifest checks the detached signature (base64 of the Ed25519
// signature over the exact manifest bytes) against any of keys and decodes
// the manifest. It does not check expiry or sequence.
func VerifyManifest(raw, sigB64 []byte, keys []ed25519.PublicKey) (*Manifest, error) {
	if len(keys) == 0 {
		return nil, errors.New("no trusted registry keys are configured")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigB64)))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, errors.New("malformed registry signature")
	}
	ok := false
	for _, k := range keys {
		if ed25519.Verify(k, raw, sig) {
			ok = true
			break
		}
	}
	if !ok {
		return nil, errors.New("registry signature does not match any trusted key")
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("invalid registry JSON: %w", err)
	}
	if m.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported registry schemaVersion %d", m.SchemaVersion)
	}
	for _, e := range m.Runtimes {
		if !safeComponent(e.Version) || !safeComponent(e.OS) || !safeComponent(e.Arch) {
			return nil, fmt.Errorf("registry entry has unsafe version/os/arch: %q %q %q", e.Version, e.OS, e.Arch)
		}
	}
	return &m, nil
}

// safeComponent reports whether s can be used as a single path component.
func safeComponent(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_', r == '+':
		default:
			return false
		}
	}
	return !strings.HasPrefix(s, ".")
}
