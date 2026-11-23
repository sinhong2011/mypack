package runtimemanager

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"mypack.dev/mypack/internal/domain"
)

// Dev source mode: for working on the runtime itself. With
// MYPACK_DEV_RUNTIME set, every runtime this computer's platform needs (any
// version, full or lite) is built from a MyPack source checkout with
// `go build`, the way scripts/release-runtimes.sh builds a release, instead
// of coming from the registry. Builds then carry the runtime code of that
// checkout (say main, before it is published).
//
//	MYPACK_DEV_RUNTIME=1               the checkout around the current folder
//	MYPACK_DEV_RUNTIME=/path/to/mypack that checkout (another branch's worktree)
//
// Dev builds go to <cache>/_dev/<key>/, never into the verified runtime
// cache, so leaving dev mode can never leave a source build posing as a
// published version. Other platforms (cross builds) use the registry.

// EnvDevRuntime turns dev source mode on.
const EnvDevRuntime = "MYPACK_DEV_RUNTIME"

// SourceDev is RuntimeInfo.Source for a runtime built from source.
const SourceDev = "source"

const runtimePkg = "./runtime/cmd/mypack-runtime"

type devSource struct {
	dir string     // the checkout
	mu  sync.Mutex // one go build at a time
}

// FindCheckout returns the MyPack checkout at or above dir, or "".
func FindCheckout(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	for d := dir; d != "" && d != filepath.Dir(d); d = filepath.Dir(d) {
		b, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err != nil {
			continue
		}
		if bytes.Contains(b, []byte("module mypack.dev/mypack\n")) || bytes.HasPrefix(b, []byte("module mypack.dev/mypack\r")) {
			if st, err := os.Stat(filepath.Join(d, filepath.FromSlash(runtimePkg))); err == nil && st.IsDir() {
				return d
			}
		}
	}
	return ""
}

// devSourceFromEnv reads MYPACK_DEV_RUNTIME ("" when off or not found).
func devSourceFromEnv() string {
	v := strings.TrimSpace(os.Getenv(EnvDevRuntime))
	switch strings.ToLower(v) {
	case "", "0", "false", "off", "no":
		return ""
	case "1", "true", "on", "yes":
		wd, _ := os.Getwd()
		return FindCheckout(wd)
	}
	return FindCheckout(v)
}

// SetDevSource turns dev source mode on for the checkout dir ("" turns it
// off). It reports whether dir is a MyPack checkout.
func (m *Manager) SetDevSource(dir string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if dir == "" {
		m.dev = nil
		return true
	}
	root := FindCheckout(dir)
	if root == "" {
		m.dev = nil
		return false
	}
	m.dev = &devSource{dir: root}
	return true
}

// DevSource is the checkout runtimes are built from, or "".
func (m *Manager) DevSource() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dev == nil {
		return ""
	}
	return m.dev.dir
}

// devFor reports the dev source when key is this computer's platform.
func (m *Manager) devFor(key string) *devSource {
	m.mu.Lock()
	d := m.dev
	m.mu.Unlock()
	if d == nil {
		return nil
	}
	osName, arch, _, ok := splitKey(key)
	if !ok || osName != runtime.GOOS || arch != runtime.GOARCH {
		return nil
	}
	return d
}

func (m *Manager) devPath(key string) string {
	return filepath.Join(m.cacheDir, "_dev", key, ExecutableName(key))
}

// devEntry stands for a source build in the builder: no capabilities
// listed, so none are enforced (the source has them all).
func devEntry(version, key string) Entry {
	osName, arch, tier, _ := splitKey(key)
	return Entry{Version: version, OS: osName, Arch: arch, Tier: tier, Filename: ExecutableName(key)}
}

// devBuild compiles the runtime of key from the checkout.
func (m *Manager) devBuild(ctx context.Context, d *devSource, key string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := m.devPath(key)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	tags := "mygo_noinspector"
	if IsLiteKey(key) {
		tags += " mypack_lite"
	}
	ldflags := "-s -w -X github.com/egoist/mygo.production=1"
	if runtime.GOOS == "windows" {
		ldflags += " -H=windowsgui"
	}
	tmp := out + ".tmp"
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-tags", tags, "-ldflags", ldflags, "-o", tmp, runtimePkg)
	cmd.Dir = d.dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 2000 {
			msg = msg[len(msg)-2000:]
		}
		return "", newErr("RUNTIME_NOT_FOUND", "Could not build the runtime from source", fmt.Errorf("%v: %s", err, msg),
			"Fix the build in "+d.dir+", or unset "+EnvDevRuntime+" to use published runtimes.")
	}
	if err := os.Rename(tmp, out); err != nil {
		return "", err
	}
	return out, nil
}

// devInfo is the status of a source build.
func (m *Manager) devInfo(d *devSource, version, key string) domain.RuntimeInfo {
	ri := domain.RuntimeInfo{Version: version, Key: key, Cached: true, Verified: true, Source: SourceDev, Path: m.devPath(key)}
	if st, err := os.Stat(ri.Path); err == nil {
		ri.CacheSize = st.Size()
	}
	return ri
}
