package licensing

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// FileName is the license file inside the mypack config directory.
const FileName = "license.json"

// stored is the on-disk record. Only the signed token matters; everything
// else is informational.
type stored struct {
	Token       string    `json:"token"`
	ActivatedAt time.Time `json:"activatedAt"`
}

// Store persists the entitlement token under Dir (0700) as license.json (0600).
type Store struct{ Dir string }

// DefaultDir is <UserConfigDir>/mypack.
func DefaultDir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "mypack"), nil
}

func (s Store) path() string { return filepath.Join(s.Dir, FileName) }

// Load returns the stored token, or "" if none is stored.
func (s Store) Load() (string, error) {
	b, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var st stored
	if err := json.Unmarshal(b, &st); err != nil {
		return "", err
	}
	return st.Token, nil
}

// Save writes the token atomically with owner-only permissions.
func (s Store) Save(token string) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(stored{Token: token, ActivatedAt: time.Now().UTC()}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, ".license-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path())
}

// Clear removes the stored token (no error if absent).
func (s Store) Clear() error {
	if err := os.Remove(s.path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
