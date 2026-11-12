package runtimemanager

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Bundled runtimes live next to the app (mygo PathResources/runtimes, or
// Contents/Resources/runtimes in a macOS .app). Each platform installer
// ships the matching archive and a copy of the signed registry.json; see
// scripts/bundle-runtime.sh. MYPACK_BUNDLED_RUNTIMES overrides the lookup.

var archiveNameRe = regexp.MustCompile(`^mypack-runtime-(\d+(?:\.\d+){0,3})-(darwin|windows|linux)-(amd64|arm64|universal)\.tar\.gz$`)

// ParseArchiveName reads version, OS and arch from a runtime archive filename.
func ParseArchiveName(name string) (version, osName, arch string, ok bool) {
	m := archiveNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", "", false
	}
	return m[1], m[2], m[3], true
}

// DefaultBundledDir is the runtime bundle shipped inside this install, or
// "" when this process is not an installer build (go run, tests, archives).
func DefaultBundledDir() string {
	if d := os.Getenv("MYPACK_BUNDLED_RUNTIMES"); d != "" {
		return d
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	for _, p := range []string{
		filepath.Join(dir, "runtimes"),
		filepath.Join(dir, "resources", "runtimes"),
		filepath.Join(dir, "..", "Resources", "runtimes"),
		filepath.Join(dir, "..", "resources", "runtimes"),
	} {
		if isBundleDir(p) {
			return p
		}
	}
	return ""
}

func isBundleDir(p string) bool {
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(p, "registry.json")); err == nil {
		return true
	}
	matches, _ := filepath.Glob(filepath.Join(p, "mypack-runtime-*.tar.gz"))
	return len(matches) > 0
}

// SeedFromDir copies a bundled registry and its archives into the cache so a
// first build on this platform does not need the network. dir missing is a
// no-op. A registry.json that fails verification is skipped; archives are
// still installed from their own checksums.
func (m *Manager) SeedFromDir(dir string) error {
	if dir == "" {
		return nil
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return nil
	}
	raw, rerr := os.ReadFile(filepath.Join(dir, "registry.json"))
	sig, serr := os.ReadFile(filepath.Join(dir, "registry.json.sig"))
	if rerr == nil && serr == nil {
		_ = m.seedManifest(raw, sig)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var first error
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		version, osName, arch, ok := ParseArchiveName(e.Name())
		if !ok {
			continue
		}
		if _, err := m.installFromFile(filepath.Join(dir, e.Name()), version, osName+"-"+arch); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (m *Manager) seedManifest(raw, sig []byte) error {
	keys := m.trusted()
	if len(keys) == 0 {
		return errors.New("no trusted registry keys are configured")
	}
	man, err := VerifyManifest(raw, sig, keys)
	if err != nil {
		return err
	}
	m.stateMu.Lock()
	last := m.readState().LastSequence
	m.stateMu.Unlock()
	if man.Sequence < last {
		return nil
	}
	m.persistManifest(raw, sig, man)
	m.mu.Lock()
	m.memo, m.memoAt, m.memoURL = man, m.now(), m.registryURL
	m.mu.Unlock()
	return nil
}

func splitKey(key string) (osName, arch string, ok bool) {
	i := strings.LastIndex(key, "-")
	if i <= 0 || i == len(key)-1 {
		return "", "", false
	}
	return key[:i], key[i+1:], true
}

func (m *Manager) installFromFile(archive, version, key string) (string, error) {
	if err := checkVK(version, key); err != nil {
		return "", err
	}
	sum, size, err := hashFile(archive)
	if err != nil {
		return "", err
	}
	osName, arch, ok := splitKey(key)
	if !ok {
		return "", newErr("RUNTIME_NOT_FOUND", "Invalid runtime platform "+key, nil, "Use a platform key like darwin-arm64.")
	}
	e := Entry{Version: version, OS: osName, Arch: arch, Filename: filepath.Base(archive), SHA256: sum, Size: size}
	if ce, ok := m.CachedEntry(version, key); ok {
		if !strings.EqualFold(ce.SHA256, sum) || ce.Size != size {
			return "", newErr("RUNTIME_VERIFY_FAILED", "The bundled runtime does not match the registry manifest",
				nil, "Rebuild the installer so it ships a matching archive and registry.json.")
		}
		e = ce
	}
	if exe, err := m.checkLocal(version, key, &e); err == nil {
		touch(m.runtimeDir(version, key))
		return exe, nil
	}
	if err := os.MkdirAll(m.cacheDir, 0o755); err != nil {
		return "", newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot create the runtime cache directory", err, "Check permissions of "+m.cacheDir+".")
	}
	m.cleanStaleTemp()
	tmp, err := os.MkdirTemp(m.cacheDir, ".tmp-")
	if err != nil {
		return "", newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot create a temporary directory in the cache", err, "Check free disk space and permissions of "+m.cacheDir+".")
	}
	defer os.RemoveAll(tmp)
	return m.materialize(e, archive, tmp)
}
