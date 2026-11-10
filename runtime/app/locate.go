package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mypack.dev/mypack/pkg/config"
)

// ConfigCandidates lists mypack.json locations in lookup order
// (ARCHITECTURE.md "Generated-app runtime contract"):
//
//	$MYPACK_CONFIG  ->  <exeDir>/../Resources/mypack.json (macOS bundle)
//	-> <exeDir>/resources/mypack.json (Windows/Linux) -> <exeDir>/mypack.json
//
// When $MYPACK_CONFIG is set it is the only candidate: an explicit override
// that points nowhere must fail loudly rather than fall back silently.
func ConfigCandidates(exeDir string, env string) []string {
	if env != "" {
		return []string{env}
	}
	return []string{
		filepath.Join(exeDir, "..", "Resources", "mypack.json"),
		filepath.Join(exeDir, "resources", "mypack.json"),
		filepath.Join(exeDir, "mypack.json"),
	}
}

// FindConfig returns the first existing candidate.
func FindConfig(exeDir, env string) (string, error) {
	cands := ConfigCandidates(exeDir, env)
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return filepath.Clean(c), nil
		}
	}
	return "", fmt.Errorf("mypack.json was not found.\nLooked in:\n  %s", strings.Join(cands, "\n  "))
}

// LoadConfig locates, parses and validates the configuration. It returns the
// directory holding mypack.json (local sources and icons resolve against it).
func LoadConfig() (*config.Config, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, "", fmt.Errorf("cannot locate the application: %w", err)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	path, err := FindConfig(filepath.Dir(exe), os.Getenv("MYPACK_CONFIG"))
	if err != nil {
		return nil, "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read %s: %w", path, err)
	}
	cfg, err := ParseConfig(b)
	if err != nil {
		return nil, "", fmt.Errorf("%s is not a valid application configuration.\n%v", path, err)
	}
	return cfg, filepath.Dir(path), nil
}

// ParseConfig parses mypack.json and validates it. Issues in build.* are
// ignored: they only matter to the Builder, never to a running app.
func ParseConfig(b []byte) (*config.Config, error) {
	cfg, err := config.Parse(b)
	if err != nil {
		return nil, err
	}
	cfg.ApplyDefaults(config.Target{})
	if err := cfg.Validate(); err != nil {
		var ve *config.ValidationError
		if errors.As(err, &ve) {
			var keep []config.Issue
			for _, is := range ve.Issues {
				if !strings.HasPrefix(is.Field, "build.") {
					keep = append(keep, is)
				}
			}
			if len(keep) == 0 {
				return cfg, nil
			}
			return nil, &config.ValidationError{Issues: keep}
		}
		return nil, err
	}
	return cfg, nil
}
