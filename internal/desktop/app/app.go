// Package app is the mypack Builder GUI: window lifecycle, navigation,
// screens, menus and the macOS menu bar item. All state lives on the main
// thread; service calls run on goroutines and land back via Window.Update.
package app

import (
	"context"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/i18n"
	"mypack.dev/mypack/pkg/config"
)

// Version is the Builder version shown in About (set by main via ldflags).
var Version = "0.1.0-dev"

type section string

const (
	secLibrary  section = "library"
	secDiscover section = "discover"
	secBuilds   section = "builds"
	secRuntimes section = "runtimes"
	secCreate   section = "create"
	secProject  section = "project"
	secGallery  section = "gallery" // MYPACK_KIT_GALLERY=1
)

// App is the GUI state.
type App struct {
	settingsQ   string // the inspector's settings search, while it renders
	settingHits int    // settings it matched this frame
	svc         appservice.Service
	win         *mygo.Window
	host        config.Target
	ui          uiState

	section    section
	projectID  string
	projTab    int     // 0 overview, 1 settings
	toolbarW   float32 // the toolbar's width last frame (compact controls when narrow)
	hud        hudState
	status     map[string]*statusState // the build status blocks, per job
	steps      map[string]*stepTrack   // the build step lists, per job
	trayPhase  float64                 // the menu-bar ring's turn while a build's % is unknown
	traySpin   bool                    // the build HUD near the notch
	previewBox ui.Rect                 // the preview column's viewport last frame (the stage fills it)
	autoOpen   map[string]time.Time    // jobs installed & opened automatically, and when

	loaded   bool
	settings appservice.Settings
	projects []domain.Project
	// siteIcons are the icons of sites pasted into a group (by address),
	// fetched with their metadata.
	siteIcons map[string][]byte
	e2eSeeded bool // test builds: Create's extra sites were added
	e2eStudio bool
	installs  []domain.Installation
	jobs      []domain.BuildJob
	runtimes  []domain.RuntimeInfo
	storage   appservice.Storage
	logs      map[string][]domain.LogLine
	logOpen   map[string]bool
	rtBusy    map[string]string // runtime key -> "download"/"verify"/"delete"
	rtProg    map[string]float64

	// watched jobs started from this session, per project
	watch map[string]string
	// successAt is when a watched job finished, for the success burst.
	successAt map[string]time.Time
	scenes    map[string]sceneMark
	presets   presetIcons
	gallery   galleryState
	palette   paletteState

	flyout    *mygo.Window
	setWin    *mygo.Window // Settings, once opened
	flyoutURL string

	lib     libState
	create  createState
	editors map[string]*editorState
	builds  buildsState
	set     settingsState
	dlg     dialogs
	studio  iconStudio
	sync    librarySync

	toasts []toastReq

	reloadMu   sync.Mutex
	reloadWant map[string]bool
	reloadT    *time.Timer

	tray     *mygo.Tray
	quitting bool
}

type toastReq struct {
	title, desc string
	kind        int
	action      string
	fn          func()
	toast       *ui.Toast // shown as is, when set
}

// Run starts the GUI on the main goroutine and returns when the app quits.
func Run(svc appservice.Service) error {
	a := &App{
		svc: svc, host: svc.HostTarget(), ui: loadUIState(),
		logs: map[string][]domain.LogLine{}, logOpen: map[string]bool{},
		rtBusy: map[string]string{}, rtProg: map[string]float64{},
		watch: map[string]string{}, successAt: map[string]time.Time{}, scenes: map[string]sceneMark{}, editors: map[string]*editorState{},
		reloadWant: map[string]bool{},
	}
	a.section = section(a.ui.Section)
	switch a.section {
	case secLibrary, secDiscover, secBuilds, secRuntimes:
	case secProject:
		a.projectID = a.ui.ProjectID
		if a.projectID == "" {
			a.section = secLibrary
		}
	default:
		a.section = secLibrary
	}
	a.set.pane = settingsPane(a.ui.SettingsPane)
	a.create.reset(a.host)
	if galleryEnabled() {
		a.section = secGallery
	}
	if id := showProjectArg(os.Args[1:]); id != "" {
		a.section, a.projectID = secProject, id
	}

	// Load settings synchronously so the first frame has the right
	// language and appearance.
	if st, err := svc.Settings(context.Background()); err == nil {
		a.settings = st
	}
	i18n.SetLocale(a.settings.Language)
	applyAppearance(a.settings.Appearance)

	mygo.App.SetName("Mypack") // the display name; data folders keep their paths
	mygo.App.SetVersion(Version)
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	mygo.App.OnSecondInstance(func(args []string, wd string) {
		if id := showProjectArg(args); id != "" && a.win != nil {
			a.win.Update(func() { a.openProject(id, 0) })
		}
		a.showWindow()
	})
	mygo.App.OnActivate(func(visible bool) {
		if !visible {
			a.showWindow()
		}
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnBeforeQuit(a.beforeQuit)
	mygo.App.OnOpenURL(func(u string) { a.openExternalInput(u) })
	mygo.App.OnOpenFile(func(p string) { a.openExternalInput(p) })
	a.installMenu()

	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:                "Mypack",
			Width:                1200,
			Height:               780,
			MinWidth:             880,
			MinHeight:            600,
			TitleBarStyle:        mygo.TitleBarHidden,
			TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
			BackgroundColor:      "light-dark(#efefeb, #141415)",
			StateKey:             "main",
			Content:              ui.View(a.view),
		})
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if !a.quitting {
				e.PreventDefault()
				a.win.Hide()
			}
		})
		kit.Invalidate = func() {
			if a.win != nil {
				a.win.Invalidate()
			}
			a.redrawSettings()
		}
		svc.Subscribe(a.onEvent)
		a.reload("projects", "installs", "jobs", "runtimes", "storage")
		a.startLibrarySync()
		a.loadCatalog()
		a.watchFront()
		a.syncTray()
		if isFake() && os.Getenv("MYPACK_FAKE_FLYOUT") == "1" {
			// Demo/screenshots: open the menu bar flyout once loaded.
			time.AfterFunc(7*time.Second, func() { mygo.RunOnMain(a.toggleFlyout) })
		}
	})
	return mygo.App.Run()
}

func applyAppearance(s string) {
	switch s {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

func (a *App) showWindow() {
	if a.win == nil {
		return
	}
	a.win.Show()
	a.win.Restore()
	a.win.Focus()
}

func (a *App) beforeQuit(e *mygo.QuitEvent) {
	running := 0
	for _, j := range a.jobs {
		if j.Status == domain.JobRunning || j.Status == domain.JobQueued {
			running++
		}
	}
	if running == 0 || a.quitting {
		a.quitting = true
		return
	}
	e.PreventDefault()
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.win, Type: mygo.MessageWarning,
			Title:   T("quit.running.title", "count", running),
			Message: T("quit.running.title", "count", running),
			Detail:  T("quit.running.detail"),
			Buttons: []string{T("quit.running.quit"), T("common.cancel")}, DefaultButton: 1, CancelButton: 1,
			DestructiveButtons: []int{0}})
		if err != nil || res.Button != 0 {
			return
		}
		for _, j := range a.jobsSnapshot() {
			if j.Status == domain.JobRunning || j.Status == domain.JobQueued {
				_ = a.svc.CancelBuild(context.Background(), j.ID, "gui")
			}
		}
		mygo.RunOnMain(func() {
			a.quitting = true
			mygo.App.Quit()
		})
	}()
}

func (a *App) jobsSnapshot() []domain.BuildJob {
	var out []domain.BuildJob
	done := make(chan struct{})
	mygo.RunOnMain(func() { out = append(out, a.jobs...); close(done) })
	<-done
	return out
}

// ---- async plumbing -------------------------------------------------------------

// do runs work off the main thread, then done on the main thread with a redraw.
func (a *App) do(work func(ctx context.Context) error, done func(err error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err := work(ctx)
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			if done != nil {
				done(err)
			}
		})
	}()
}

// toast queues a toast (shown on the next frame).
func (a *App) toast(title, desc string, kind int) {
	a.toasts = append(a.toasts, toastReq{title: title, desc: desc, kind: kind})
	if a.win != nil {
		a.win.Invalidate()
	}
	a.redrawSettings()
}

func (a *App) toastAction(title, action string, fn func()) {
	a.toasts = append(a.toasts, toastReq{title: title, action: action, fn: fn})
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) toastErr(err error) {
	if err == nil {
		return
	}
	e := errInfo(err)
	a.toast(friendly(err), e.Remediation, 2)
}

// reload re-reads parts of the state, coalescing bursts of events.
func (a *App) reload(kinds ...string) {
	a.reloadMu.Lock()
	for _, k := range kinds {
		a.reloadWant[k] = true
	}
	if a.reloadT == nil {
		a.reloadT = time.AfterFunc(60*time.Millisecond, a.flushReload)
	}
	a.reloadMu.Unlock()
}

func (a *App) flushReload() {
	a.reloadMu.Lock()
	want := a.reloadWant
	a.reloadWant = map[string]bool{}
	a.reloadT = nil
	a.reloadMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var (
		projects []domain.Project
		installs []domain.Installation
		jobs     []domain.BuildJob
		runtimes []domain.RuntimeInfo
		storage  appservice.Storage
		settings appservice.Settings
		errs     []error
	)
	var projErr error
	if want["projects"] {
		projects, projErr = a.svc.ListProjects(ctx)
	}
	if want["installs"] {
		p, err := a.svc.ListInstallations(ctx)
		installs, errs = p, append(errs, err)
	}
	if want["jobs"] {
		p, err := a.svc.ListJobs(ctx, "", 200)
		jobs, errs = p, append(errs, err)
	}
	if want["runtimes"] {
		p, err := a.svc.ListRuntimes(ctx)
		runtimes, errs = p, append(errs, err)
	}
	if want["storage"] {
		p, err := a.svc.StorageBreakdown(ctx)
		storage, errs = p, append(errs, err)
	}
	if want["settings"] {
		p, err := a.svc.Settings(ctx)
		settings, errs = p, append(errs, err)
	}
	if a.win == nil {
		return
	}
	a.win.Update(func() {
		for _, err := range errs {
			if err != nil {
				log.Printf("mypack: reload: %v", err)
			}
		}
		if projErr != nil {
			log.Printf("mypack: reload projects: %v", projErr)
		}
		if want["projects"] && projErr == nil {
			a.projects = projects
			a.syncEditors()
		}
		if want["installs"] {
			a.installs = installs
		}
		if want["jobs"] && jobs != nil {
			a.jobs = jobs
		}
		if want["runtimes"] && runtimes != nil {
			a.runtimes = runtimes
		}
		if want["storage"] {
			a.storage = storage
		}
		if want["settings"] && settings.Appearance != "" {
			lang := a.settings.Language
			a.settings = settings
			if lang != settings.Language {
				i18n.SetLocale(settings.Language)
				a.installMenu()
			}
			applyAppearance(settings.Appearance)
		}
		if want["projects"] {
			a.loaded = true
		}
		a.syncTray()
	})
}

// onEvent runs on the service's goroutine.
func (a *App) onEvent(e appservice.Event) {
	switch e.Kind {
	case appservice.EventProjectChanged, appservice.EventProjectDeleted:
		a.reload("projects")
		a.syncSoon()
	case appservice.EventInstallChanged:
		a.reload("installs", "storage")
	case appservice.EventRuntimeChanged:
		a.reload("runtimes", "storage")
	case appservice.EventSettingsChanged:
		a.reload("settings")
	case appservice.EventRuntimeProgress:
		if a.win != nil {
			a.win.Invalidate()
		}
	case appservice.EventJobLog:
		if e.Log == nil || a.win == nil {
			return
		}
		line := *e.Log
		a.win.Update(func() {
			if _, ok := a.logs[e.JobID]; ok {
				a.logs[e.JobID] = append(a.logs[e.JobID], line)
			}
		})
	case appservice.EventJobChanged, appservice.EventApprovalNeeded:
		if e.Job == nil {
			a.reload("jobs")
			return
		}
		j := *e.Job
		if a.win == nil {
			return
		}
		a.win.Update(func() { a.upsertJob(j) })
		if j.Status == domain.JobSucceeded || j.Status == domain.JobFailed || j.Status == domain.JobCancelled {
			a.reload("projects", "installs", "storage", "runtimes")
		}
	}
}

func (a *App) upsertJob(j domain.BuildJob) {
	prev := -1
	for i := range a.jobs {
		if a.jobs[i].ID == j.ID {
			prev = i
			break
		}
	}
	var old domain.JobStatus
	if prev >= 0 {
		old = a.jobs[prev].Status
		a.jobs[prev] = j
	} else {
		a.jobs = append([]domain.BuildJob{j}, a.jobs...)
		sort.SliceStable(a.jobs, func(x, y int) bool { return a.jobs[x].CreatedAt.After(a.jobs[y].CreatedAt) })
	}
	// Approval from an agent/CLI: surface it.
	if j.Status == domain.JobAwaitingApproval && old != j.Status && j.Actor != "gui" {
		a.toast(T("approval.toast.title"), T("approval.toast.body", "actor", j.Actor), 0)
	}
	if old != j.Status && a.watch[j.ProjectID] == j.ID {
		switch j.Status {
		case domain.JobSucceeded:
			a.successAt[j.ID] = time.Now()
			a.autoInstall(j)
			a.starAfterBuild()
			c := a.win
			if c != nil {
				// announce for screen readers via a toast when not on the page
				if !a.showsBuild(j.ProjectID) {
					a.toast(T("build.done.toast", "name", j.Snapshot.App.Name), "", 1)
				}
			}
		case domain.JobFailed:
			if !a.showsBuild(j.ProjectID) {
				a.toast(T("build.failed.toast", "name", j.Snapshot.App.Name), "", 2)
			}
		}
	}
	a.syncTray()
	a.syncHUD()
}

// ---- lookups --------------------------------------------------------------------

func (a *App) project(id string) *domain.Project {
	for i := range a.projects {
		if a.projects[i].ID == id {
			return &a.projects[i]
		}
	}
	return nil
}

func (a *App) jobByID(id string) *domain.BuildJob {
	for i := range a.jobs {
		if a.jobs[i].ID == id {
			return &a.jobs[i]
		}
	}
	return nil
}

func (a *App) latestJob(projectID string) *domain.BuildJob {
	for i := range a.jobs {
		if a.jobs[i].ProjectID == projectID {
			return &a.jobs[i]
		}
	}
	return nil
}

func (a *App) activeJobs() []domain.BuildJob {
	var out []domain.BuildJob
	for _, j := range a.jobs {
		if j.Status == domain.JobRunning || j.Status == domain.JobQueued || j.Status == domain.JobAwaitingApproval {
			out = append(out, j)
		}
	}
	return out
}

// showsBuild reports whether the screen shows project id's build (its page,
// or Create where it started): its result needs no toast then.
func (a *App) showsBuild(id string) bool {
	return (a.section == secProject && a.projectID == id) || (a.section == secCreate && a.create.building == id)
}

func (a *App) installFor(projectID string) *domain.Installation {
	for i := range a.installs {
		if a.installs[i].ProjectID == projectID && projectID != "" {
			return &a.installs[i]
		}
	}
	return nil
}

func (a *App) runtimeInfo(t config.Target) *domain.RuntimeInfo {
	for i := range a.runtimes {
		if a.runtimes[i].Key == t.Key() {
			return &a.runtimes[i]
		}
	}
	return nil
}

// ---- navigation -----------------------------------------------------------------

func (a *App) nav(s section) {
	if s != a.section {
		a.stopPresetFetches()
	}
	a.travel(s)
	if s == secCreate && a.section != secCreate {
		a.create.focus = true
	}
	a.section = s
	if s != secProject {
		a.projectID = ""
	}
	a.ui.Section = string(s)
	a.ui.ProjectID = a.projectID
	a.ui.save()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) openProject(id string, tab int) {
	a.travel(secProject)
	a.section = secProject
	a.projectID = id
	a.projTab = tab
	a.ui.Section = string(secProject)
	a.ui.ProjectID = id
	a.ui.save()
	if a.win != nil {
		a.win.Invalidate()
	}
}

// openExternalInput handles a URL/file given by the OS (open-with, deep link).
func (a *App) openExternalInput(s string) {
	if a.win == nil {
		return
	}
	if id, ok := strings.CutPrefix(s, "mypack://project/"); ok {
		if validID(id) {
			a.win.Update(func() { a.openProject(id, 0); a.showWindow() })
		}
		return
	}
	if s == "mypack://library" || s == "mypack://" {
		a.win.Update(func() { a.nav(secLibrary); a.showWindow() })
		return
	}
	a.win.Update(func() {
		s = strings.TrimPrefix(s, "mypack://create?url=")
		a.create.reset(a.host)
		a.create.setInput(s)
		a.nav(secCreate)
		a.showWindow()
	})
}

// isFake reports whether we're running against the demo service.
func isFake() bool { return os.Getenv("MYPACK_FAKE") == "1" }

// showProjectArg extracts "--show-project <id>" (browser companion launch).
func showProjectArg(args []string) string {
	for i, v := range args {
		if v == "--show-project" && i+1 < len(args) && validID(args[i+1]) {
			return args[i+1]
		}
	}
	return ""
}

// validID accepts service IDs like "prj_ab12cd34" only.
func validID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}
