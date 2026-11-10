package appservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mypack.dev/mypack/internal/builder"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/metadata"
	"mypack.dev/mypack/internal/runtimemanager"
	"mypack.dev/mypack/internal/storage"
	"mypack.dev/mypack/pkg/config"
)

// DefaultRegistryURL is the placeholder production registry.
// DefaultRegistryURL is overridable at link time (-X ...appservice.DefaultRegistryURL=...).
var DefaultRegistryURL = "https://registry.mypack.dev/runtimes/registry.json"

// Auditor is an optional capability of the service: frontends that perform
// read-only calls (the MCP server) record them in the activity log through it.
type Auditor interface {
	RecordActivity(ctx context.Context, a domain.Activity) error
}

// Impl is the production Service.
type Impl struct {
	opts     Options
	store    *storage.Store
	ownStore bool

	engine   Engine
	runtimes Runtimes
	inst     Installer
	icons    Icons
	fetch    MetadataFetcher
	caps     CapabilityFunc
	host     config.Target

	bus *bus

	ctx    context.Context // lifetime of background work
	cancel context.CancelFunc
	wg     sync.WaitGroup
	sem    chan struct{} // bounds concurrent builds

	mu      sync.Mutex
	running map[string]*runningJob

	regMu   sync.Mutex
	lastReg string
}

var (
	_ Service = (*Impl)(nil)
	_ Auditor = (*Impl)(nil)
)

// New opens the database and wires the engine packages (or the fakes in opts).
func New(opts Options) (*Impl, error) {
	s := &Impl{opts: opts, bus: newBus(), running: map[string]*runningJob{}}
	s.ctx, s.cancel = context.WithCancel(context.Background())

	if opts.Store != nil {
		s.store = opts.Store
	} else {
		st, err := storage.Open(opts.DBPath)
		if err != nil {
			s.cancel()
			return nil, err
		}
		s.store, s.ownStore = st, true
	}
	n := opts.Concurrency
	if n <= 0 {
		n = 2
	}
	s.sem = make(chan struct{}, n)

	s.inst = opts.Installer
	if s.inst == nil {
		s.inst = realInstaller{}
	}
	s.icons = opts.Icons
	if s.icons == nil {
		s.icons = realIcons{}
	}
	s.fetch = opts.Metadata
	if s.fetch == nil {
		s.fetch = metadata.Fetch
	}
	s.caps = opts.Capabilities
	if s.caps == nil {
		s.caps = builder.Capabilities
	}
	if opts.Host != nil {
		s.host = *opts.Host
	} else {
		s.host = builder.HostTarget()
	}

	set, err := s.Settings(s.ctx)
	if err != nil {
		s.Close()
		return nil, err
	}
	s.runtimes = opts.Runtimes
	if s.runtimes == nil {
		s.runtimes = runtimemanager.New(opts.CacheDir, set.RegistryURL)
	}
	s.lastReg = set.RegistryURL
	s.engine = opts.Engine
	if s.engine == nil {
		mgr, ok := s.runtimes.(*runtimemanager.Manager)
		if !ok {
			s.Close()
			return nil, errors.New("appservice: Options.Engine is required when Options.Runtimes is not a *runtimemanager.Manager")
		}
		s.engine = &builder.Engine{Runtimes: mgr}
	}
	return s, nil
}

// Close cancels running builds, waits briefly for them and closes the store.
func (s *Impl) Close() error {
	s.cancel()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
	}
	s.bus.close()
	if s.ownStore {
		return s.store.Close()
	}
	return nil
}

// Store exposes the underlying store (diagnostics / tests).
func (s *Impl) Store() *storage.Store { return s.store }

// WatchExternal polls the database change counter and, when another process
// (CLI, MCP, GUI) has written, emits generic project/job/installation change
// events so subscribers re-read state. Spec §12.9 calls IPC notification-only;
// polling is the v1 transport. It blocks until ctx is done, so run it in a
// goroutine. interval <= 0 means 1s.
func (s *Impl) WatchExternal(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.ctx.Done():
			return
		case <-t.C:
			if changed, err := s.store.ExternalChange(ctx); err == nil && changed {
				s.bus.publish(Event{Kind: EventProjectChanged})
				s.bus.publish(Event{Kind: EventJobChanged})
				s.bus.publish(Event{Kind: EventInstallChanged})
			}
		}
	}
}

// Subscribe implements Service.
func (s *Impl) Subscribe(fn func(Event)) func() { return s.bus.subscribe(fn) }

func (s *Impl) emit(e Event) { s.bus.publish(e) }

// RecordActivity implements Auditor.
func (s *Impl) RecordActivity(ctx context.Context, a domain.Activity) error {
	return s.store.AddActivity(ctx, a)
}

// audit records a write in the activity log; failures are non-fatal.
func (s *Impl) audit(ctx context.Context, actor, action, target string, err error) {
	res := "ok"
	if err != nil {
		res = "error: " + asDomainErr(err, CodeInternal).Code
	}
	_ = s.store.AddActivity(context.WithoutCancel(ctx), domain.Activity{
		Actor: actorOr(actor, "gui"), Action: action, Target: target, Result: res,
	})
}

// Activity implements Service.
func (s *Impl) Activity(ctx context.Context, limit int) ([]domain.Activity, error) {
	return s.store.ListActivity(ctx, limit)
}

// HostTarget implements Service.
func (s *Impl) HostTarget() config.Target { return s.host }

// ---- settings --------------------------------------------------------------

const settingsKey = "settings"

func (s *Impl) defaultSettings() Settings {
	out := s.opts.OutputDir
	if out == "" {
		if h, err := os.UserHomeDir(); err == nil {
			out = filepath.Join(h, "mypack", "Builds")
		}
	}
	in := s.opts.InstallDir
	if in == "" {
		in = s.inst.DefaultInstallDir()
	}
	reg := os.Getenv("MYPACK_REGISTRY")
	if reg == "" {
		reg = DefaultRegistryURL
	}
	return Settings{
		Language: "system", Appearance: "system", OutputDir: out, InstallDir: in,
		RegistryURL: reg, ShowMenuBarIcon: true, CheckUpdates: true, LibraryView: "grid",
	}
}

// Settings implements Service: stored values over defaults. An explicit
// Options.RegistryURL or MYPACK_REGISTRY always wins for this process.
func (s *Impl) Settings(ctx context.Context) (Settings, error) {
	set := s.defaultSettings()
	if _, err := s.store.GetSetting(ctx, settingsKey, &set); err != nil {
		return set, err
	}
	d := s.defaultSettings()
	if set.OutputDir == "" {
		set.OutputDir = d.OutputDir
	}
	if set.InstallDir == "" {
		set.InstallDir = d.InstallDir
	}
	if set.RegistryURL == "" {
		set.RegistryURL = d.RegistryURL
	}
	if env := os.Getenv("MYPACK_REGISTRY"); env != "" {
		set.RegistryURL = env
	}
	if s.opts.RegistryURL != "" {
		set.RegistryURL = s.opts.RegistryURL
	}
	return set, nil
}

// SaveSettings implements Service.
func (s *Impl) SaveSettings(ctx context.Context, set Settings) (err error) {
	defer func() { s.audit(ctx, "gui", "settings.save", "settings", err) }()
	d := s.defaultSettings()
	if set.Language == "" {
		set.Language = d.Language
	}
	if set.Appearance == "" {
		set.Appearance = d.Appearance
	}
	if set.LibraryView == "" {
		set.LibraryView = d.LibraryView
	}
	switch set.Appearance {
	case "system", "light", "dark":
	default:
		return newErr(CodeInvalidInput, "appearance must be system, light or dark", "")
	}
	switch set.LibraryView {
	case "grid", "list":
	default:
		return newErr(CodeInvalidInput, "libraryView must be grid or list", "")
	}
	if set.RegistryURL != "" && !strings.HasPrefix(set.RegistryURL, "https://") &&
		!strings.HasPrefix(set.RegistryURL, "file://") && !strings.HasPrefix(set.RegistryURL, "http://localhost") &&
		!strings.HasPrefix(set.RegistryURL, "http://127.0.0.1") {
		return newErr(CodeInvalidInput, "registryUrl must be https:// or file://", "Use an https:// registry address.")
	}
	if err := s.store.SetSetting(ctx, settingsKey, set); err != nil {
		return err
	}
	s.syncRegistry(ctx)
	s.emit(Event{Kind: EventSettingsChanged})
	return nil
}

// syncRegistry pushes the effective registry URL into the runtime manager if
// it changed (possibly by another process).
func (s *Impl) syncRegistry(ctx context.Context) {
	set, err := s.Settings(ctx)
	if err != nil {
		return
	}
	s.regMu.Lock()
	defer s.regMu.Unlock()
	if set.RegistryURL != s.lastReg {
		s.lastReg = set.RegistryURL
		s.runtimes.SetRegistryURL(set.RegistryURL)
	}
}

// ---- projects ----------------------------------------------------------------

// ListProjects implements Service.
func (s *Impl) ListProjects(ctx context.Context) ([]domain.Project, error) {
	return s.store.ListProjects(ctx, false)
}

// GetProject implements Service.
func (s *Impl) GetProject(ctx context.Context, id string) (domain.Project, error) {
	return s.store.GetProject(ctx, id)
}

// FetchMetadata implements Service.
func (s *Impl) FetchMetadata(ctx context.Context, rawURL string) (Metadata, error) {
	u, err := config.NormalizeURL(rawURL)
	if err != nil {
		return Metadata{}, &domain.Error{Code: CodeInvalidInput, Message: err.Error(), Field: "input",
			Remediation: "Enter a website address such as example.com."}
	}
	r, err := s.fetch(ctx, u)
	if err != nil {
		return Metadata{}, asDomainErr(err, CodeInvalidInput)
	}
	return Metadata{URL: r.URL, Title: r.Title, Description: r.Description, IconURL: r.IconURL,
		IconPNG: r.IconPNG, ThemeColor: r.ThemeColor, Fallback: r.Fallback, Warning: r.Warning}, nil
}

// CreateProject implements Service.
func (s *Impl) CreateProject(ctx context.Context, req CreateRequest) (p domain.Project, err error) {
	actor := actorOr(req.Actor, "gui")
	defer func() { s.audit(ctx, actor, "project.create", req.Input, err) }()

	in := strings.TrimSpace(req.Input)
	if in == "" {
		return p, &domain.Error{Code: CodeInvalidInput, Message: "Enter a website address, file or folder.", Field: "input"}
	}
	kind, path, err := detectInput(in)
	if err != nil {
		return p, err
	}

	var (
		cfg         *config.Config
		icon        []byte
		iconURL     string
		origin      = originOf(actor)
		fromConfig  bool // identity came from the user's file: keep it verbatim
		defaultName string
	)
	switch kind {
	case inputURL:
		u, nerr := config.NormalizeURL(path)
		if nerr != nil {
			return p, &domain.Error{Code: CodeInvalidInput, Message: nerr.Error(), Field: "input",
				Remediation: "Enter a website address such as example.com."}
		}
		md := req.Metadata
		if md == nil {
			m, ferr := s.FetchMetadata(ctx, u)
			if ferr != nil {
				return p, ferr
			}
			md = &m
		}
		name := cleanName(firstNonEmpty(req.Name, md.Title))
		if name == "" {
			name = cleanName(titleFromHost(u))
		}
		cfg = config.New(name, config.Source{Type: config.SourceURL, Value: u}, s.host)
		if len(md.IconPNG) > 0 {
			icon, iconURL = md.IconPNG, md.IconURL
		} else {
			cfg.App.Icon.Source = "default"
		}
	case inputFile:
		defaultName = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		cfg = config.New(cleanName(firstNonEmpty(req.Name, defaultName)),
			config.Source{Type: config.SourceFile, Value: path}, s.host)
		cfg.App.Icon.Source = "default"
	case inputDir:
		entry, eerr := findEntry(path)
		if eerr != nil {
			return p, eerr
		}
		cfg = config.New(cleanName(firstNonEmpty(req.Name, filepath.Base(path))),
			config.Source{Type: config.SourceDir, Value: path, Entry: entry}, s.host)
		cfg.App.Icon.Source = "default"
	case inputConfig:
		c, _, lerr := LoadConfigFile(path)
		if lerr != nil {
			return p, lerr
		}
		cfg, fromConfig = c, true
		origin = originOf(actor)
		if req.Name != "" {
			cfg.App.Name = cleanName(req.Name)
		}
		if config.IsPakeJSON(mustRead(path)) {
			origin = "import"
		}
		cfg.ApplyDefaults(s.host)
		icon, iconURL = s.configIcon(ctx, cfg, req.Metadata)
	}

	if !fromConfig {
		// Generated identities must not collide with another project's, or
		// two apps would silently share one profile directory.
		cfg.App.Identifier, cfg.App.ProfileID = s.uniqueIdentity(ctx, cfg.App.Identifier)
	}
	cfg.ApplyDefaults(s.host)
	if len(icon) == 0 {
		icon = s.icons.Default(cfg.App.Name)
		if cfg.App.Icon.Source == "auto" {
			cfg.App.Icon.Source = "default"
		}
	}
	if verr := cfg.Validate(); verr != nil {
		return p, asDomainErr(verr, CodeInvalidConfig)
	}
	p, err = s.store.InsertProject(ctx, domain.Project{
		Config: cfg, IconPNG: icon, IconURL: iconURL, Draft: true, Origin: origin,
	})
	if err != nil {
		return p, err
	}
	s.emit(Event{Kind: EventProjectChanged, ProjectID: p.ID})
	return p, nil
}

func mustRead(path string) []byte { b, _ := os.ReadFile(path); return b }

// configIcon resolves the icon of a project loaded from a config file.
func (s *Impl) configIcon(ctx context.Context, cfg *config.Config, md *Metadata) (png []byte, iconURL string) {
	switch cfg.App.Icon.Source {
	case "file":
		if b, err := os.ReadFile(cfg.App.Icon.Path); err == nil {
			if n, nerr := s.icons.Normalize(b); nerr == nil {
				return n, ""
			}
		}
		cfg.App.Icon = config.Icon{Source: "default"}
	case "auto":
		if cfg.App.Source.Type == config.SourceURL {
			if md == nil {
				if m, err := s.FetchMetadata(ctx, cfg.App.Source.Value); err == nil {
					md = &m
				}
			}
			if md != nil && len(md.IconPNG) > 0 {
				return md.IconPNG, md.IconURL
			}
		}
		cfg.App.Icon.Source = "default"
	}
	return nil, ""
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func titleFromHost(u string) string {
	h := config.RegistrableHost(u)
	if h == "" {
		return "App"
	}
	first := strings.Split(h, ".")[0]
	if first == "" {
		return "App"
	}
	return strings.ToUpper(first[:1]) + first[1:]
}

// uniqueIdentity returns an identifier (and equal profile ID) not used by any
// existing project, trashed ones included.
func (s *Impl) uniqueIdentity(ctx context.Context, ident string) (string, string) {
	all, err := s.store.ListProjects(ctx, true)
	if err != nil {
		return ident, ident
	}
	used := map[string]bool{}
	for _, p := range all {
		used[p.Config.App.Identifier] = true
		used[p.Config.App.ProfileID] = true
	}
	cand := ident
	for n := 2; used[cand]; n++ {
		cand = fmt.Sprintf("%s-%d", ident, n)
	}
	return cand, cand
}

// UpdateProject implements Service.
func (s *Impl) UpdateProject(ctx context.Context, req UpdateRequest) (p domain.Project, err error) {
	actor := actorOr(req.Actor, "gui")
	defer func() { s.audit(ctx, actor, "project.update", req.ID, err) }()

	cur, err := s.store.GetProject(ctx, req.ID)
	if err != nil {
		return p, err
	}
	if req.Config == nil {
		return p, &domain.Error{Code: CodeInvalidInput, Message: "config is required", Field: "config"}
	}
	next := req.Config.Clone()
	// Identity is never defaulted or silently re-derived (spec §9C.2).
	if next.App.Identifier == "" {
		next.App.Identifier = cur.Config.App.Identifier
	}
	if next.App.ProfileID == "" {
		next.App.ProfileID = cur.Config.App.ProfileID
	}
	if !req.ConfirmIdentityChange {
		for _, c := range []struct{ field, old, new string }{
			{"app.identifier", cur.Config.App.Identifier, next.App.Identifier},
			{"app.profileId", cur.Config.App.ProfileID, next.App.ProfileID},
		} {
			if c.old != c.new {
				return p, &domain.Error{
					Code:  CodeIdentityChange,
					Field: c.field,
					Message: fmt.Sprintf("Changing %s from %q to %q would make installed copies of this app "+
						"look like a different app and can orphan its website data.", c.field, c.old, c.new),
					Remediation: "Confirm the identity change explicitly, or restore the original value.",
				}
			}
		}
	}
	next.ApplyDefaults(s.host)
	if verr := next.Validate(); verr != nil {
		return p, asDomainErr(verr, CodeInvalidConfig)
	}
	cur.Config = next
	p, err = s.store.UpdateProject(ctx, cur, req.ExpectedRevision)
	if err != nil {
		return p, err
	}
	s.emit(Event{Kind: EventProjectChanged, ProjectID: p.ID})
	return p, nil
}

// SetProjectIcon implements Service.
func (s *Impl) SetProjectIcon(ctx context.Context, id string, png []byte, actor string) (p domain.Project, err error) {
	defer func() { s.audit(ctx, actor, "project.icon", id, err) }()
	norm, err := s.icons.Normalize(png)
	if err != nil {
		return p, &domain.Error{Code: CodeInvalidInput, Message: "That image could not be used as an icon.",
			Cause: err.Error(), Remediation: "Use a PNG, JPEG, GIF or ICO image."}
	}
	for attempt := 0; ; attempt++ {
		cur, gerr := s.store.GetProject(ctx, id)
		if gerr != nil {
			return p, gerr
		}
		cur.IconPNG = norm
		p, err = s.store.UpdateProject(ctx, cur, cur.Revision)
		var de *domain.Error
		if errors.As(err, &de) && de.Code == CodeRevisionConflict && attempt < 3 {
			continue // someone edited between our read and write: retry on the new revision
		}
		break
	}
	if err == nil {
		s.emit(Event{Kind: EventProjectChanged, ProjectID: p.ID})
	}
	return p, err
}

// TrashProject implements Service.
func (s *Impl) TrashProject(ctx context.Context, id, actor string) (err error) {
	defer func() { s.audit(ctx, actor, "project.trash", id, err) }()
	now := time.Now()
	if err = s.store.SetTrashed(ctx, id, &now); err == nil {
		s.emit(Event{Kind: EventProjectDeleted, ProjectID: id})
	}
	return err
}

// RestoreProject implements Service.
func (s *Impl) RestoreProject(ctx context.Context, id, actor string) (err error) {
	defer func() { s.audit(ctx, actor, "project.restore", id, err) }()
	if err = s.store.SetTrashed(ctx, id, nil); err == nil {
		s.emit(Event{Kind: EventProjectChanged, ProjectID: id})
	}
	return err
}

// DuplicateProject implements Service. The copy gets a fresh identity so the
// two apps never share a profile.
func (s *Impl) DuplicateProject(ctx context.Context, id, actor string) (p domain.Project, err error) {
	defer func() { s.audit(ctx, actor, "project.duplicate", id, err) }()
	src, err := s.store.GetProject(ctx, id)
	if err != nil {
		return p, err
	}
	cfg := src.Config.Clone()
	cfg.App.Name = cleanName(cfg.App.Name + " copy")
	cfg.App.Identifier, cfg.App.ProfileID = s.uniqueIdentity(ctx, src.Config.App.Identifier+"-copy")
	p, err = s.store.InsertProject(ctx, domain.Project{
		Config: cfg, IconPNG: src.IconPNG, IconURL: src.IconURL, Draft: true, Origin: originOf(actor),
	})
	if err == nil {
		s.emit(Event{Kind: EventProjectChanged, ProjectID: p.ID})
	}
	return p, err
}

// ExportProject implements Service: writes the project's mypack.json.
func (s *Impl) ExportProject(ctx context.Context, id, path string) (err error) {
	defer func() { s.audit(ctx, "gui", "project.export", id, err) }()
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return err
	}
	return p.Config.Save(path)
}

// configHash is the sha256 of the canonical (struct-ordered) JSON encoding.
func configHash(c *config.Config) string {
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
