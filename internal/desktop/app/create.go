package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/domain"
	mpicons "mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/pkg/config"
)

// createState is the Create screen (spec §9C.1: five visible items).
type createState struct {
	input      string // URL or local path
	lastInput  string
	lastEdit   time.Time
	normalized string // normalized URL for the current input, "" if invalid
	local      string // local file/dir path, if the input is one
	invalidMsg string

	name       string
	nameEdited bool
	account    string // optional account label: "Gmail — Work"

	meta        *appservice.Metadata
	metaFor     string
	fetchingFor string
	fetchSeq    int

	iconPNG    []byte
	iconCustom bool
	iconStyle  *config.IconStyle // from the Icon Studio
	styledKey  string
	styled     []byte

	targets   map[string]config.Target
	otherOpen bool
	advOpen   bool
	cfg       *config.Config // pending advanced settings
	adv       *editorState

	busy           bool
	buildRequested bool
	building       string // the project whose build plays in the preview
	autoInstall    bool   // install & open when the build succeeds
	autoSet        bool   // autoInstall was seeded
	focus          bool
	urlHandle      ui.Handle

	// more are the sites added after the first: with any, the app is a
	// group of all of them (app.group), in this order.
	more       []createSite
	addingAddr bool   // the "paste an address" field is open
	moreAddr   string // its text
	groupIcon  []byte // the grid icon of the group, for groupIconKey
	groupKey   string
}

// createSite is a site added to the app being created: a Library project
// or a pasted address.
type createSite struct {
	url       string
	projectID string
	local     string // a folder or HTML file
	name      string
}

// key identifies the site in the list.
func (x createSite) key() string { return x.url + x.local }

// isGroup reports whether the app being created is a group of sites.
func (s *createState) isGroup() bool { return len(s.more) > 0 }

func (s *createState) reset(host config.Target) {
	*s = createState{}
	t := config.DefaultTarget(host)
	s.targets = map[string]config.Target{t.Key(): t}
}

// buildStarted notes that the build of project id started from Create: the
// view stays as it is (input, name, settings) and the build plays in its
// stage. Nothing is reset.
func (s *createState) buildStarted(id string) { s.building = id }

func (s *createState) setInput(v string) {
	s.input = strings.TrimSpace(v)
	s.lastEdit = time.Time{} // fetch immediately
	s.analyze()
}

// analyze classifies the input without network access.
func (s *createState) analyze() {
	s.lastInput = s.input
	s.normalized, s.local, s.invalidMsg = "", "", ""
	in := strings.TrimSpace(s.input)
	if in == "" {
		return
	}
	if strings.HasPrefix(in, "/") || strings.HasPrefix(in, "~/") || strings.HasPrefix(in, "file://") {
		p := strings.TrimPrefix(in, "file://")
		if strings.HasPrefix(p, "~/") {
			if h, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(h, p[2:])
			}
		}
		if _, err := os.Stat(p); err == nil {
			s.local = p
			return
		}
		s.invalidMsg = T("create.error.noFile")
		return
	}
	u, err := config.NormalizeURL(in)
	if err != nil {
		s.invalidMsg = T("create.error.invalidURL")
		return
	}
	s.normalized = u
}

func (s *createState) ready() bool { return s.normalized != "" || s.local != "" }

func (s *createState) displayName() string {
	if strings.TrimSpace(s.name) != "" {
		return s.name
	}
	if s.isGroup() {
		return T("group.defaultName")
	}
	if s.meta != nil && s.meta.Title != "" {
		return s.meta.Title
	}
	if s.local != "" {
		return strings.TrimSuffix(filepath.Base(s.local), filepath.Ext(s.local))
	}
	return ""
}

// createIcon is the new app's icon as it will look (with the Icon
// Studio's style).
func (a *App) createIcon() []byte {
	base := a.createBaseIcon()
	st := a.create.iconStyle
	if st == nil {
		return base
	}
	b, _ := json.Marshal(st)
	key := fmt.Sprintf("%p|%s|%s", firstByte(base), a.create.displayName(), b)
	if a.create.styledKey != key {
		out, err := mpicons.Styled(base, *st, a.create.displayName(), 512)
		if err != nil {
			return base
		}
		a.create.styledKey, a.create.styled = key, out
	}
	return a.create.styled
}

// createBaseIcon is the image beneath the style: a chosen image, the
// group's grid or the website's icon.
func (a *App) createBaseIcon() []byte {
	s := &a.create
	if s.iconPNG != nil {
		return s.iconPNG
	}
	if s.isGroup() {
		return a.createGroupIcon()
	}
	if s.meta != nil && s.metaFor == s.normalized {
		return s.meta.IconPNG
	}
	return nil
}

func nonEmpty(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func localSource(p string) config.Source {
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return config.Source{Type: config.SourceDir, Value: p, Entry: "index.html"}
	}
	return config.Source{Type: config.SourceFile, Value: p}
}

// buildCTA is "Build for This Mac" (or PC / Linux, or N platforms).
func (a *App) buildCTA() string {
	s := &a.create
	if _, here := s.targets[config.DefaultTarget(a.host).Key()]; here && s.autoInstall {
		return T("create.cta.buildOpen")
	}
	if len(s.targets) == 1 {
		for _, t := range s.targets {
			if t.OS == a.host.OS && t.Arch == a.host.Arch {
				return T("create.cta.thisDevice", "device", deviceWord(a.host))
			}
			return T("create.cta.platform", "platform", osLabel(t.OS))
		}
	}
	return T("create.cta.many", "count", len(s.targets))
}

// createDownloadHint tells, before building, that a one-time download will
// be needed (spec §9.1 Prevent errors).
func (a *App) createDownloadHint() string {
	var size int64
	for _, t := range a.create.targets {
		if rt := a.runtimeInfo(t); rt != nil && !rt.Cached {
			size += rt.Size
		}
	}
	if size == 0 {
		return ""
	}
	return T("create.downloadHint", "size", bytesLabel(size))
}

// maybeFetchMetadata debounces typing (400 ms), ignores stale results and
// never overwrites a name the user edited (spec §9.5).
func (a *App) maybeFetchMetadata(c *ui.Context) {
	s := &a.create
	if s.normalized == "" || s.metaFor == s.normalized || s.fetchingFor == s.normalized {
		return
	}
	wait := 400*time.Millisecond - time.Since(s.lastEdit)
	if wait > 0 {
		c.After(wait)
		return
	}
	s.fetchSeq++
	seq, u := s.fetchSeq, s.normalized
	s.fetchingFor = u
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		m, err := a.svc.FetchMetadata(ctx, u)
		a.win.Update(func() {
			if s.fetchSeq != seq || s.normalized != u {
				if s.fetchingFor == u {
					s.fetchingFor = ""
				}
				return // stale
			}
			s.fetchingFor = ""
			s.metaFor = u
			if err != nil {
				m = appservice.Metadata{URL: u, Fallback: true}
			}
			s.meta = &m
		})
	}()
}

// ensureCfg returns the pending settings for the new app, made on first use.
func (a *App) ensureCfg() *config.Config {
	s := &a.create
	if s.cfg == nil {
		src := config.Source{Type: config.SourceURL, Value: s.normalized}
		if s.local != "" {
			src = localSource(s.local)
		}
		s.cfg = config.New(nonEmpty(s.displayName(), "App"), src, a.host)
		s.adv = &editorState{cfg: s.cfg, local: true}
	}
	return s.cfg
}

func (s *createState) toggleTarget(t config.Target) {
	if _, ok := s.targets[t.Key()]; ok {
		delete(s.targets, t.Key())
	} else {
		s.targets[t.Key()] = t
	}
}

func otherTargets(host config.Target) []config.Target {
	all := []config.Target{
		{OS: "darwin", Arch: "arm64", Format: "app"}, {OS: "darwin", Arch: "amd64", Format: "app"},
		{OS: "windows", Arch: "amd64", Format: "portable"}, {OS: "windows", Arch: "arm64", Format: "portable"},
		{OS: "linux", Arch: "amd64", Format: "portable"}, {OS: "linux", Arch: "arm64", Format: "portable"},
	}
	var out []config.Target
	for _, t := range all {
		if t.OS == host.OS && t.Arch == host.Arch {
			continue
		}
		out = append(out, t)
	}
	return out
}

// createAndBuild: create the draft → apply icon/advanced/targets → build.
func (a *App) createAndBuild() {
	s := &a.create
	if !s.ready() || s.busy || len(s.targets) == 0 {
		if !s.ready() && s.input != "" {
			s.lastEdit = time.Time{}
		}
		return
	}
	s.busy = true
	input := s.normalized
	if s.local != "" {
		input = s.local
	}
	name := ""
	if s.nameEdited {
		name = strings.TrimSpace(s.name)
	}
	if acct := strings.TrimSpace(s.account); acct != "" {
		name = nonEmpty(strings.TrimSpace(s.displayName()), "App") + " — " + acct
	}
	var meta *appservice.Metadata
	if s.meta != nil && s.metaFor == s.normalized {
		m := *s.meta
		meta = &m
	}
	var icon []byte
	if s.iconCustom {
		icon = s.iconPNG
	}
	style := s.iconStyle
	var targets []config.Target
	for _, t := range s.targets {
		targets = append(targets, t)
	}
	var adv *config.Config
	if s.cfg != nil {
		adv = s.cfg.Clone()
	}
	var projectID string
	group := s.isGroup()
	var sites []appservice.GroupSite
	if group {
		sites = s.groupSites()
		if !s.nameEdited {
			name = T("group.defaultName")
		}
	}
	a.do(func(ctx context.Context) error {
		var pr domain.Project
		var err error
		if group {
			pr, err = a.svc.CreateGroup(ctx, appservice.GroupRequest{Name: name, Sites: sites, Actor: "gui"})
		} else {
			pr, err = a.svc.CreateProject(ctx, appservice.CreateRequest{Input: input, Name: name, Metadata: meta, Actor: "gui"})
		}
		if err != nil {
			return err
		}
		projectID = pr.ID
		auto := s.autoInstall
		a.win.Update(func() { a.ui.setAutoInstall(pr.ID, auto) })
		if icon != nil {
			if pr, err = a.svc.SetProjectIcon(ctx, pr.ID, icon, "gui"); err != nil {
				return err
			}
		}
		cfg := pr.Config.Clone()
		if adv != nil {
			keep := cfg.App
			cfg = adv
			cfg.App.Name, cfg.App.Identifier, cfg.App.ProfileID, cfg.App.Source, cfg.App.Icon = keep.Name, keep.Identifier, keep.ProfileID, keep.Source, keep.Icon
			cfg.App.Group = keep.Group // the service made the members (icons, ids)
			if adv.App.Title == "" {
				cfg.App.Title = keep.Title
			}
		}
		cfg.App.Icon.Style = style // the Icon Studio's
		cfg.Build.Targets = targets
		if pr, err = a.svc.UpdateProject(ctx, appservice.UpdateRequest{ID: pr.ID, ExpectedRevision: pr.Revision, Config: cfg, Actor: "gui"}); err != nil {
			return err
		}
		j, err := a.svc.StartBuild(ctx, appservice.BuildRequest{ProjectID: pr.ID, Targets: targets, Actor: "gui"})
		if err != nil {
			return err
		}
		a.win.Update(func() {
			a.flyFrom(cardKey(pr.ID), a.iconAt["create"], pr.IconPNG, pr.Config.App.Name)
			a.watch[pr.ID] = j.ID
			a.upsertJob(j)
		})
		return nil
	}, func(err error) {
		s.busy = false
		var de *domain.Error
		if errors.As(err, &de) && de.Code == appservice.CodeLicenseLimit && group {
			a.dlg.groupLimit = true
			return
		}
		if err != nil {
			a.toastErr(err)
			if projectID == "" {
				return
			}
		}
		a.reload("projects")
		// The build plays in Create's preview, where it started.
		a.create.buildStarted(projectID)
	})
}

// chooseLocalSource opens a file/folder chooser.
func (a *App) chooseLocalSource() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: T("create.chooseFile.title"),
			ButtonLabel: T("create.chooseFile.button"), Directory: false,
			Filters: []mygo.FileFilter{{Name: "HTML", Extensions: []string{"html", "htm"}}}})
		if err != nil || len(paths) == 0 {
			return
		}
		a.win.Update(func() { a.create.setInput(paths[0]) })
	}()
}

// chooseIcon picks a PNG/JPEG/ICNS/ICO file and hands its bytes to set.
func (a *App) chooseIcon(set func([]byte)) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: T("create.icon.choose"),
			Filters: []mygo.FileFilter{{Name: T("create.icon.filter"), Extensions: []string{"png", "jpg", "jpeg", "icns", "ico", "webp"}}}})
		if err != nil || len(paths) == 0 {
			return
		}
		b, err := os.ReadFile(paths[0])
		if err != nil {
			return
		}
		a.win.Update(func() { set(b) })
	}()
}

// projectKind reads a dropped folder's package.json for a short hint of
// what it is ("Vite project · pnpm"); ok is false for anything else.
func projectKind(dir string) (hint string, ok bool) {
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return "", false
	}
	var pkg struct {
		Deps    map[string]string `json:"dependencies"`
		DevDeps map[string]string `json:"devDependencies"`
	}
	_ = json.Unmarshal(b, &pkg)
	has := func(n string) bool { _, a := pkg.Deps[n]; _, b := pkg.DevDeps[n]; return a || b }
	framework := ""
	for _, f := range []struct{ dep, name string }{
		{"next", "Next.js"}, {"nuxt", "Nuxt"}, {"astro", "Astro"}, {"@sveltejs/kit", "SvelteKit"},
		{"@angular/core", "Angular"}, {"react-scripts", "Create React App"}, {"vite", "Vite"},
	} {
		if has(f.dep) {
			framework = f.name
			break
		}
	}
	pm := "npm"
	for _, l := range []struct{ file, pm string }{
		{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lockb", "bun"}, {"bun.lock", "bun"},
	} {
		if _, err := os.Stat(filepath.Join(dir, l.file)); err == nil {
			pm = l.pm
			break
		}
	}
	if framework == "" {
		return T("create.projectKindPlain", "pm", pm), true
	}
	return T("create.projectKind", "framework", framework, "pm", pm), true
}

// chooseLocalFolder opens a folder chooser (a web project or a folder of
// HTML).
func (a *App) chooseLocalFolder() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: T("create.chooseFile.title"),
			ButtonLabel: T("create.chooseFile.button"), Directory: true})
		if err != nil || len(paths) == 0 {
			return
		}
		a.win.Update(func() { a.create.setInput(paths[0]) })
	}()
}

// openSource is the one open panel for Create (⌘O): an HTML file, a web
// project or static site (pick its package.json or index.html: the panel
// can't take folders and files at once), or an app config (.json), which
// goes to the config importer through the same Create flow.
func (a *App) openSource() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: T("create.open.title"),
			Message: T("create.open.message"), ButtonLabel: T("create.chooseFile.button"),
			Filters: []mygo.FileFilter{{Name: T("create.open.filter"), Extensions: []string{"html", "htm", "json"}}}})
		if err != nil || len(paths) == 0 {
			return
		}
		pick := paths[0]
		switch strings.ToLower(filepath.Base(pick)) {
		case "package.json", "index.html":
			pick = filepath.Dir(pick) // the project or site folder
		}
		a.win.Update(func() {
			if a.section != secCreate {
				a.create.reset(a.host)
				a.nav(secCreate)
			}
			a.create.setInput(pick)
		})
	}()
}
