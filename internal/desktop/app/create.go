package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/components"
	"mypack.dev/mypack/internal/desktop/design"
	"mypack.dev/mypack/internal/desktop/lobeui"
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

	meta       *appservice.Metadata
	metaFor    string
	fetchingFor string
	fetchSeq   int

	iconPNG    []byte
	iconCustom bool

	targets    map[string]config.Target
	otherOpen  bool
	advOpen    bool
	cfg        *config.Config // pending advanced settings
	adv        *editorState

	busy           bool
	buildRequested bool
	focus          bool
	urlHandle      ui.Handle
}

func (s *createState) reset(host config.Target) {
	*s = createState{}
	t := config.DefaultTarget(host)
	s.targets = map[string]config.Target{t.Key(): t}
}

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
	if s.meta != nil && s.meta.Title != "" {
		return s.meta.Title
	}
	if s.local != "" {
		return strings.TrimSuffix(filepath.Base(s.local), filepath.Ext(s.local))
	}
	return ""
}

func (a *App) createView(c *ui.Context) {
	p := design.Of(c)
	s := &a.create
	if s.input != s.lastInput {
		s.lastEdit = time.Now()
		s.analyze()
	}
	a.maybeFetchMetadata(c)

	page(c, 600, func() {
		ui.Column(c).Gap(6).Margin(28, 0, 6, 0).Children(func() {
			components.PageTitle(c, T("create.title"))
			components.Secondary(c, T("create.subtitle"))
		})

		// 1. Website URL / local files.
		ui.Column(c).Gap(8).Children(func() {
			r := components.URLHero(c.Key("url"), &s.input, components.URLHeroOpts{
				Placeholder: T("create.url.placeholder"), Label: T("create.url.label"),
				Loading: s.fetchingFor != "" && s.fetchingFor == s.normalized, Invalid: s.invalidMsg != "" && s.input != "",
				AutoFocus: true, Handle: &s.urlHandle,
				OnPaste: func(text string) bool {
					t := strings.TrimSpace(text)
					if t == "" {
						return false
					}
					// Paste replaces the field and fetches at once (auto-detect).
					s.setInput(t)
					return true
				},
			})
			if s.focus {
				s.focus = false
				s.urlHandle.Focus()
			}
			if len(r.Dropped) > 0 {
				s.setInput(r.Dropped[0])
			}
			if r.Submitted && s.ready() {
				s.buildRequested = true
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(0, 4).Children(func() {
				switch {
				case s.invalidMsg != "" && s.input != "" && time.Since(s.lastEdit) > 700*time.Millisecond:
					ui.Text(c, s.invalidMsg).FontSize(design.FontSmall).TextColor(p.Warning)
				case r.DragOver:
					ui.Text(c, T("create.drop.release")).FontSize(design.FontSmall).TextColor(p.AccentText)
				default:
					components.Caption(c, T("create.drop.hint"))
					if components.Button(c.Key("choose"), T("create.chooseFile"), components.ButtonOpts{Variant: components.Plain, Size: 18}).
						FontSize(design.FontSmall).Clicked() {
						a.chooseLocalSource()
					}
				}
			})
		})

		if !s.ready() {
			a.createSuggestions(c)
			return
		}

		// 2–3. Preview: icon + name + source.
		components.Card(c).Padding(16).Gap(0).Children(func() {
			ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
				loading := s.normalized != "" && s.meta == nil && s.metaFor != s.normalized
				if loading && s.iconPNG == nil {
					lobeui.Skeleton(c, 64, 64).Radius(design.AppIconRadius(64))
				} else {
					components.IconPicker(c.Key("icon"), a.createIcon(), s.displayName(), 64, T("create.icon.change"), func(m *ui.Menu) {
						if m.Item(T("create.icon.choose")).Chosen() {
							a.chooseIcon(func(b []byte) { s.iconPNG, s.iconCustom = b, true })
						}
						if m.Item(T("create.icon.reset")).Disabled(!s.iconCustom).Chosen() {
							s.iconPNG, s.iconCustom = nil, false
						}
					})
				}
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					components.Overline(c, T("create.name.label"))
					placeholder := s.displayName()
					if placeholder == "" {
						placeholder = T("create.name.placeholder")
					}
					shown := s.name
					if !s.nameEdited {
						shown = s.displayName()
					}
					in := ui.TextInputBase(c.Key("name"), &shown).FontSize(design.FontTitle2).FontWeight(design.WeightSemibold).
						Placeholder(placeholder).PlaceholderColor(p.TextTertiary).TextColor(p.Text).FocusRing(false).Label(T("create.name.label")).
						Padding(4, 8).Margin(0, -8).Radius(design.RadiusSM)
					if in.Focused() {
						in.Background(p.Well).Border(1, p.Accent).Shadow(0, 0, 0, 3, p.Accent.Alpha(0.14))
					} else if in.Hovered() {
						in.Background(p.Hover)
					}
					if in.Changed() {
						s.name = shown
						s.nameEdited = strings.TrimSpace(shown) != ""
					}
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						icon := lobeui.IconLink()
						src := s.normalized
						if s.local != "" {
							icon = lobeui.IconFolder()
							src = s.local
						} else {
							src = config.RegistrableHost(s.normalized)
						}
						ui.Icon(c, icon).FontSize(12).TextColor(p.TextTertiary)
						components.Caption(c, src).SingleLine().MinWidth(0)
						if loading {
							components.Caption(c, "· "+T("create.fetching"))
						}
					})
				})
			})
			if s.meta != nil && s.meta.Fallback && s.metaFor == s.normalized {
				components.Note(c, lobeui.IconInfo(), T("create.offline"), components.ToneWarning).Margin(14, 0, 0, 0)
			}
		})

		// 4. Target.
		a.createTargets(c)

		// Advanced Settings (collapsed).
		ui.Column(c).Gap(8).Children(func() {
			components.Disclosure(c.Key("adv"), &s.advOpen, T("create.advanced"), T("create.advanced.sub"))
			if s.advOpen {
				if s.cfg == nil {
					src := config.Source{Type: config.SourceURL, Value: s.normalized}
					if s.local != "" {
						src = localSource(s.local)
					}
					s.cfg = config.New(nonEmpty(s.displayName(), "App"), src, a.host)
					s.adv = &editorState{cfg: s.cfg, local: true}
				}
				a.settingsEditor(c, s.adv, nil)
			}
		})

		// 5. Primary CTA.
		components.Hairline(c)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if pf := a.createDownloadHint(); pf != "" {
				ui.Icon(c, lobeui.IconDownload()).FontSize(13).TextColor(p.TextTertiary)
				components.Caption(c, pf).MinWidth(0).Shrink(1)
			}
			ui.Spacer(c)
			if components.Button(c.Key("cancel"), T("common.cancel"), components.ButtonOpts{Variant: components.Ghost}).Clicked() {
				a.nav(secLibrary)
			}
			label := a.buildCTA()
			if components.Button(c.Key("build"), label, components.ButtonOpts{Variant: components.Primary, Size: design.ControlXL,
				Loading: s.busy, Disabled: len(s.targets) == 0, Hotkey: "mod+b"}).Clicked() {
				s.buildRequested = true
			}
		})
	})
	if s.buildRequested {
		s.buildRequested = false
		a.createAndBuild()
	}
}

func (a *App) createIcon() []byte {
	s := &a.create
	if s.iconPNG != nil {
		return s.iconPNG
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

func (a *App) createSuggestions(c *ui.Context) {
	p := design.Of(c)
	ui.Column(c).Gap(10).Margin(18, 0, 0, 0).Children(func() {
		components.Overline(c, T("create.try"))
		ui.Row(c).Gap(8).Wrap().Children(func() {
			for i, t := range templates {
				if i >= 6 {
					break
				}
				t := t
				chip := ui.ButtonBase(c.Key(t.url)).Label(t.name).Row().Justify(ui.Start).Height(30).PaddingX(10).Gap(8).AlignItems(ui.Center).
					Radius(15).Border(1, p.Border).Background(p.Surface).Cursor(ui.CursorPointer).Transition(design.Transition(c))
				if chip.Hovered() {
					chip.Border(1, p.BorderStrong).Background(p.Surface.Mix(p.Text, 0.03))
				}
				chip.Children(func() {
					components.Placeholder(c, t.name, 16)
					ui.Text(c, t.name).FontSize(design.FontSmall).FontWeight(design.WeightMedium).TextColor(p.Text)
				})
				if chip.Clicked() {
					a.create.setInput(t.url)
				}
			}
		})
	})
}

func (a *App) createTargets(c *ui.Context) {
	p := design.Of(c)
	s := &a.create
	ui.Column(c).Gap(10).Children(func() {
		components.Overline(c, T("create.target.label"))
		host := config.DefaultTarget(a.host)
		hostIcon := lobeui.IconLaptop()
		ui.Row(c).Gap(8).Wrap().Children(func() {
			_, on := s.targets[host.Key()]
			if components.TargetChip(c.Key("host"), hostIcon, thisDevice(a.host), T("target.recommended"), on, false).Clicked() {
				s.toggleTarget(host)
			}
		})
		if components.Button(c.Key("other"), T("create.otherPlatforms"), components.ButtonOpts{Variant: components.Plain, Size: 22,
			Trailing: map[bool]*ui.SVG{true: lobeui.IconChevronDown(), false: lobeui.IconChevronRt()}[s.otherOpen]}).FontSize(design.FontSmall).AlignSelf(ui.Start).Clicked() {
			s.otherOpen = !s.otherOpen
		}
		if s.otherOpen {
			ui.Column(c).Gap(8).Children(func() {
				ui.Row(c).Gap(8).Wrap().Children(func() {
					for _, t := range otherTargets(a.host) {
						t := t
						_, on := s.targets[t.Key()]
						unsupported := t.OS == "darwin" && a.host.OS != "darwin"
						sub := archLabel(t.OS, t.Arch)
						if unsupported {
							sub = T("target.needsMac")
						}
						if components.TargetChip(c.Key(t.Key()), lobeui.IconMonitor(), osLabel(t.OS), sub, on, unsupported).Clicked() {
							s.toggleTarget(t)
						}
					}
				})
				ui.Text(c, T("create.otherPlatforms.note")).FontSize(design.FontSmall).TextColor(p.TextTertiary).LineHeight(1.4)
			})
		}
	})
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
	var meta *appservice.Metadata
	if s.meta != nil && s.metaFor == s.normalized {
		m := *s.meta
		meta = &m
	}
	var icon []byte
	if s.iconCustom {
		icon = s.iconPNG
	}
	var targets []config.Target
	for _, t := range s.targets {
		targets = append(targets, t)
	}
	var adv *config.Config
	if s.cfg != nil {
		adv = s.cfg.Clone()
	}
	var projectID string
	a.do(func(ctx context.Context) error {
		pr, err := a.svc.CreateProject(ctx, appservice.CreateRequest{Input: input, Name: name, Metadata: meta, Actor: "gui"})
		if err != nil {
			return err
		}
		projectID = pr.ID
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
			if adv.App.Title == "" {
				cfg.App.Title = keep.Title
			}
		}
		cfg.Build.Targets = targets
		if pr, err = a.svc.UpdateProject(ctx, appservice.UpdateRequest{ID: pr.ID, ExpectedRevision: pr.Revision, Config: cfg, Actor: "gui"}); err != nil {
			return err
		}
		j, err := a.svc.StartBuild(ctx, appservice.BuildRequest{ProjectID: pr.ID, Targets: targets, Actor: "gui"})
		if err != nil {
			return err
		}
		a.win.Update(func() {
			a.watch[pr.ID] = j.ID
			a.upsertJob(j)
		})
		return nil
	}, func(err error) {
		s.busy = false
		if err != nil {
			a.toastErr(err)
			if projectID == "" {
				return
			}
		}
		a.reload("projects")
		a.create.reset(a.host)
		a.openProject(projectID, 0)
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
