package appservice

import (
	"context"

	"mypack.dev/mypack/internal/builder"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/internal/installer"
	"mypack.dev/mypack/internal/metadata"
	"mypack.dev/mypack/internal/runtimemanager"
	"mypack.dev/mypack/internal/storage"
	"mypack.dev/mypack/pkg/config"
)

// The interfaces below are the seams between the service and the engine
// packages. They default to the real packages and are replaced by fakes in
// tests, so service logic can be exercised without a runtime or a network.

// Engine builds artifacts from a config snapshot (builder.Engine).
type Engine interface {
	Build(ctx context.Context, req builder.Request) ([]domain.Artifact, error)
}

// Runtimes manages the runtime cache (runtimemanager.Manager).
type Runtimes interface {
	SetRegistryURL(u string)
	List(ctx context.Context) ([]domain.RuntimeInfo, error)
	Status(ctx context.Context, version, key string) (domain.RuntimeInfo, error)
	Ensure(ctx context.Context, version, key string, progress func(done, total int64)) (string, error)
	Verify(ctx context.Context, version, key string) error
	Delete(version, key string) error
	CacheSize() int64
}

// Installer places and manages apps on this machine (package installer).
type Installer interface {
	DefaultInstallDir() string
	ProfileDir(profileID string) string
	Install(ctx context.Context, artifactPath, installDir string, replace bool) (string, error)
	Replace(ctx context.Context, installedPath, newArtifactPath string) (backupPath string, err error)
	IsRunning(installedPath string) bool
	Launch(path string) error
	Reveal(path string) error
	OpenURL(url string) error
	Uninstall(installedPath string) error
	ClearProfile(profileID string) error
	ReadAppInfo(path string) (installer.AppInfo, error)
}

// Icons produces and normalizes app icons (package icons).
type Icons interface {
	Default(name string) []byte
	Normalize(data []byte) ([]byte, error)
	// IconComposerPNG renders an Icon Composer document (.icon folder).
	IconComposerPNG(ctx context.Context, path string) ([]byte, error)
}

// MetadataFetcher resolves website metadata (metadata.Fetch).
type MetadataFetcher func(ctx context.Context, rawURL string) (metadata.Result, error)

// IconFetcher asks a third-party favicon service for a website's icon
// (metadata.FetchIconFrom).
type IconFetcher func(ctx context.Context, rawURL string, p metadata.IconProvider) ([]byte, error)

// CapabilityFunc evaluates host -> target support (builder.Capabilities).
type CapabilityFunc func(host, target config.Target) builder.Capability

// Options configures New. Zero values select production defaults; the
// injectable fields exist for tests.
type Options struct {
	DBPath      string // "" = <UserConfigDir>/mypack/mypack.db
	CacheDir    string // runtime cache; "" = platform default
	BundledDir  string // installer-shipped runtimes; "" = DefaultBundledDir
	RegistryURL string // overrides the stored/default registry when set
	OutputDir   string // default build output directory
	InstallDir  string // default install directory

	// Concurrency bounds simultaneous builds (default 2).
	Concurrency int

	Store        *storage.Store // optional, already-open store (not closed by Close)
	Engine       Engine
	Runtimes     Runtimes
	Installer    Installer
	Icons        Icons
	Metadata     MetadataFetcher
	IconFrom     IconFetcher
	Capabilities CapabilityFunc
	Host         *config.Target // overrides builder.HostTarget()

	// RoutesFile is where the link-routing table of installed apps is kept.
	// "" = $MYPACK_ROUTES_FILE, else routes.json beside a custom DBPath,
	// else <UserConfigDir>/mypack-apps/routes.json; "-" disables writing.
	RoutesFile string

	// LoginItem starts MyPack at login (OpenAtLogin). Only the GUI sets it;
	// without one, OpenAtLogin and SetOpenAtLogin return UNSUPPORTED.
	LoginItem LoginItem
}

// ---- production adapters --------------------------------------------------

type realInstaller struct{}

func (realInstaller) DefaultInstallDir() string    { return installer.DefaultInstallDir() }
func (realInstaller) ProfileDir(id string) string  { return installer.ProfileDir(id) }
func (realInstaller) IsRunning(p string) bool      { return installer.IsRunning(p) }
func (realInstaller) Launch(p string) error        { return installer.Launch(p) }
func (realInstaller) Reveal(p string) error        { return installer.Reveal(p) }
func (realInstaller) OpenURL(u string) error       { return installer.OpenURL(u) }
func (realInstaller) Uninstall(p string) error     { return installer.Uninstall(p) }
func (realInstaller) ClearProfile(id string) error { return installer.ClearProfile(id) }
func (realInstaller) ReadAppInfo(p string) (installer.AppInfo, error) {
	return installer.ReadAppInfo(p)
}
func (realInstaller) Install(ctx context.Context, a, d string, replace bool) (string, error) {
	return installer.Install(ctx, a, d, replace)
}
func (realInstaller) Replace(ctx context.Context, installed, next string) (string, error) {
	return installer.Replace(ctx, installed, next)
}

type realIcons struct{}

func (realIcons) Default(name string) []byte         { return icons.Default(name) }
func (realIcons) Normalize(b []byte) ([]byte, error) { return icons.Normalize(b) }
func (realIcons) IconComposerPNG(ctx context.Context, p string) ([]byte, error) {
	return icons.IconComposerPNG(ctx, p)
}

// Compile-time checks that the real packages satisfy the seams.
var (
	_ Engine   = (*builder.Engine)(nil)
	_ Runtimes = (*runtimemanager.Manager)(nil)
)
