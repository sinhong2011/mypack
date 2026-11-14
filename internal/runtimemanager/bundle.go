package runtimemanager

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Bundled runtimes live next to the app (mygo PathResources/runtimes, or
// Contents/Resources/runtimes in a macOS .app). Windows and Linux installers
// ship the matching archive plus registry.json. macOS ships registry.json
// and a Mach-O helper at Contents/Resources/bin/mypack-runtime, which
// `mygo build` signs with the app so Apple notarization passes. See
// scripts/bundle-runtime.sh. MYPACK_BUNDLED_RUNTIMES overrides the lookup.

// bundledHelper is resources/runtimes/bundled.json, written when the
// darwin archive is extracted into a MyGo platform directory.
type bundledHelper struct {
	Version string   `json:"version"`
	Keys    []string `json:"keys"`
}

var archiveNameRe = regexp.MustCompile(`^mypack-runtime-(\d+(?:\.\d+){0,3})-(darwin|windows|linux)-(amd64|arm64|universal)(-lite)?\.tar\.gz$`)

// ParseArchiveName reads version, OS, arch and tier ("" or TierLite) from a
// runtime archive filename: mypack-runtime-1.0.10-darwin-arm64[-lite].tar.gz.
func ParseArchiveName(name string) (version, osName, arch, tier string, ok bool) {
	m := archiveNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", "", "", false
	}
	if m[4] != "" {
		tier = TierLite
	}
	return m[1], m[2], m[3], tier, true
}

// ArchiveName is the archive filename of the runtime version/key.
func ArchiveName(version, key string) string {
	return "mypack-runtime-" + version + "-" + key + ".tar.gz"
}

// BundledHelperName is the file name of the bundled macOS helper for key:
// bin/mypack-runtime, or bin/mypack-runtime-lite for a lite key.
func BundledHelperName(key string) string {
	name := ExecutableName(key)
	if IsLiteKey(key) {
		base, ext := strings.TrimSuffix(name, ".exe"), ""
		if base != name {
			ext = ".exe"
		}
		name = base + liteSuffix + ext
	}
	return name
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

// SeedFromDir copies a bundled registry and its archives (or the macOS
// helper Mach-O) into the cache so a first build on this platform does not
// need the network. dir missing is a no-op. A registry.json that fails
// verification is skipped; archives are still installed from their own
// checksums.
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
		version, osName, arch, tier, ok := ParseArchiveName(e.Name())
		if !ok {
			continue
		}
		key := Entry{OS: osName, Arch: arch, Tier: tier}.Key()
		if _, err := m.installFromFile(filepath.Join(dir, e.Name()), version, key); err != nil && first == nil {
			first = err
		}
	}
	if err := m.seedHelper(dir); err != nil && first == nil {
		first = err
	}
	return first
}

func (m *Manager) seedHelper(dir string) error {
	b, err := os.ReadFile(filepath.Join(dir, "bundled.json"))
	if err != nil {
		return nil
	}
	var spec bundledHelper
	if err := json.Unmarshal(b, &spec); err != nil || spec.Version == "" || len(spec.Keys) == 0 {
		return newErr("RUNTIME_VERIFY_FAILED", "The bundled runtime helper metadata is invalid", err, "Rebuild the installer.")
	}
	var first error
	for _, key := range spec.Keys {
		// One helper per tier: bin/mypack-runtime and bin/mypack-runtime-lite.
		name := BundledHelperName(key)
		src := filepath.Join(filepath.Dir(dir), "bin", name)
		if _, err := os.Stat(src); err != nil {
			if first == nil {
				first = newErr("RUNTIME_VERIFY_FAILED", "The bundled runtime helper is missing", err, "Rebuild the installer so it ships bin/"+name+".")
			}
			continue
		}
		if _, err := m.installFromBinary(src, spec.Version, key); err != nil && first == nil {
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

// splitKey parses "<os>-<arch>[-lite]".
func splitKey(key string) (osName, arch, tier string, ok bool) {
	if IsLiteKey(key) {
		key, tier = strings.TrimSuffix(key, liteSuffix), TierLite
	}
	i := strings.LastIndex(key, "-")
	if i <= 0 || i == len(key)-1 {
		return "", "", "", false
	}
	return key[:i], key[i+1:], tier, true
}

func (m *Manager) installFromFile(archive, version, key string) (string, error) {
	if err := checkVK(version, key); err != nil {
		return "", err
	}
	sum, size, err := hashFile(archive)
	if err != nil {
		return "", err
	}
	osName, arch, tier, ok := splitKey(key)
	if !ok {
		return "", newErr("RUNTIME_NOT_FOUND", "Invalid runtime platform "+key, nil, "Use a platform key like darwin-arm64.")
	}
	e := Entry{Version: version, OS: osName, Arch: arch, Tier: tier, Filename: filepath.Base(archive), SHA256: sum, Size: size}
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

// installFromBinary copies a Mach-O helper (signed with the macOS
// installer) into the cache. The marker records the registry archive's
// sha256 when the bundled registry.json is present, so a later Ensure
// still treats this copy as verified even though codesign changed the
// bytes. The binary hash in the marker is of this file.
func (m *Manager) installFromBinary(src, version, key string) (string, error) {
	if err := checkVK(version, key); err != nil {
		return "", err
	}
	osName, arch, tier, ok := splitKey(key)
	if !ok {
		return "", newErr("RUNTIME_NOT_FOUND", "Invalid runtime platform "+key, nil, "Use a platform key like darwin-arm64.")
	}
	exeName := ExecutableName(key)
	e := Entry{Version: version, OS: osName, Arch: arch, Tier: tier, Filename: exeName}
	if ce, ok := m.CachedEntry(version, key); ok {
		e = ce
	} else {
		sum, size, err := hashFile(src)
		if err != nil {
			return "", err
		}
		e.SHA256, e.Size = sum, size
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
	extracted := filepath.Join(tmp, "x")
	if err := os.Mkdir(extracted, 0o755); err != nil {
		return "", newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot create a temporary directory", err, "")
	}
	dst := filepath.Join(extracted, exeName)
	if err := copyHelper(src, dst, key); err != nil {
		return "", newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot copy the bundled runtime helper", err, "")
	}
	bsum, bsize, err := hashFile(dst)
	if err != nil {
		return "", newErr("RUNTIME_VERIFY_FAILED", "Cannot hash the runtime binary", err, "")
	}
	mk := marker{Version: version, Key: key, SHA256: strings.ToLower(e.SHA256), Size: e.Size, Binary: exeName,
		BinarySHA256: bsum, BinarySize: bsize, VerifiedAt: m.clock().UTC()}
	mb, _ := json.MarshalIndent(mk, "", "  ")
	if err := os.WriteFile(filepath.Join(extracted, markerName), mb, 0o644); err != nil {
		return "", newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot write the verification marker", err, "")
	}
	_ = os.WriteFile(filepath.Join(extracted, lastUsedName), nil, 0o644)
	final := m.runtimeDir(version, key)
	if err := swapDir(extracted, final); err != nil {
		return "", newErr("RUNTIME_DOWNLOAD_FAILED", "Cannot install the runtime into the cache", err, "Check permissions and free disk space of "+m.cacheDir+".")
	}
	return filepath.Join(final, exeName), nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
