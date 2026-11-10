# MyGo API digest (github.com/egoist/mygo v0.8.3)

Source: `/Users/room1910/go/pkg/mod/github.com/egoist/mygo@v0.8.3` (docs/, ui/, examples/, cmd/mygo).
Signatures below were copied from source/docs; items marked **[docs]** come from prose docs only,
**[os]** is OS/WebKit behaviour not implemented in MyGo source.
Module needs `go 1.27.3` (use `GOTOOLCHAIN=auto`; local go is 1.27.2 and auto-switches). No cgo needed (purego).
Imports: `"github.com/egoist/mygo"` and `"github.com/egoist/mygo/ui"`.

---------------------------------------------------------------------------------------------------
## 1. App lifecycle, identity, paths

```go
func main() {
    mygo.App.SetName("My Pack")            // optional, BEFORE Run (see identity table)
    if !mygo.App.RequestSingleInstanceLock() { return }
    mygo.App.OnSecondInstance(func(args []string, workingDir string) { win.Restore(); win.Focus() })
    mygo.App.OnOpenURL(func(rawURL string) { ... })      // register BEFORE Run
    mygo.App.OnOpenFile(func(path string) { ... })       // register BEFORE Run
    mygo.App.WhenReady(func() {                          // windows/trays/dialogs only valid from here on
        win = mygo.NewWindow(mygo.WindowOptions{...})
    })
    if err := mygo.App.Run(); err != nil { log.Fatal(err) }   // main goroutine ONLY; returns when app quits
}
```

`mygo.App` (type `*Application`) methods (app.go, paths.go, single_instance.go, login.go, deeplink.go, locale.go):

| Method | Notes |
|---|---|
| `Run() error` | native event loop; must be on main goroutine |
| `WhenReady(fn func())` | runs fn once ready (immediately if already ready); `IsReady() bool` |
| `Quit()` | closes windows (OnClose may veto), then quits; Cmd+Q/SIGINT do the same |
| `Exit(code int)` | immediate, no events |
| `Relaunch()` | quit then start again with same args |
| `Name() string` / `SetName(string)` | see identity table; SetName before Run |
| `Version() string` / `SetVersion(string)` | |
| `IsPackaged() bool` | true if running from an `.app` bundle (macOS) / identity ldflags set |
| `Path(PathName) (string, error)` / `SetPath(PathName, string)` | override any dir |
| `RequestSingleInstanceLock() bool` | false in 2nd instance (args forwarded). Lock key = bundle identifier + "\n" + Name; unix socket `mygo-<uid>-<hash>.sock` in `$XDG_RUNTIME_DIR`/tmp |
| `OnSecondInstance(fn func(args []string, workingDir string)) (off func())` | |
| `OnReady`, `OnWindowAllClosed`, `OnQuit`, `OnDidBecomeActive`, `OnDidResignActive` `(fn func())` | each returns `off func()` |
| `OnActivate(fn func(hasVisibleWindows bool))` | Dock click (macOS) |
| `OnBeforeQuit(fn func(e *QuitEvent))`, `OnWillQuit(fn func(e *QuitEvent))` | `e.PreventDefault()` cancels |
| `OnWindowCreated(fn func(w *Window))` | |
| `OnOpenURL(fn func(url string))`, `OnOpenFile(fn func(path string))` | deep links / file assoc |
| `OnNotificationClick(fn func(id string))` | |
| `RegisterURLScheme(scheme string) error` (+ `UnregisterURLScheme`, `IsURLSchemeRegistered`) | runtime registration (Win/Linux); on macOS schemes must be in Info.plist `CFBundleURLTypes` |
| `SetOpenAtLogin(bool) error`, `OpenAtLogin() bool`, `WasOpenedAtLogin() bool` | macOS 13+ needs packaged app |
| `SetMenu(*Menu)`, `Menu() *Menu` | |
| `SetActivationPolicy(mygo.ActivationPolicyAccessory)` / `ActivationPolicy()` | hides Dock icon (menu-bar apps) |
| `SetBadgeCount(n)`, `Focus()`, `IsActive()`, `Hide()`, `Show()`, `IsHidden()`, `ShowAboutPanel(AboutPanelOptions)` | |
| `SetLocale(tag)`, `Locale()`, `Languages()`, `SetLanguages(...)`, `PreferredLanguages()`, `OnLocaleChanged(func(string))` | see §6 |
| `ClearBrowsingData() error` | default session |
| `AddRecentDocument(path) error`, `RecentDocuments()`, `ClearRecentDocuments()` | |
| `SetWindowStateStore(WindowStateStore)` | |
| `App.Dock` (`SetBadge`, `Bounce`, `SetIcon`, `SetMenu`, `Hide`, `Show`), `App.JumpList.SetTasks` | |

Other package-level: `mygo.IsDev() bool`, `mygo.RunOnMain(fn func())`, `mygo.SetFrontend(fs.FS)`,
`mygo.Bind(services ...any)`, `mygo.NewEvent[T](name)` (only for web-frontend bindings), `mygo.Screen` (displays),
`mygo.Power`, `mygo.Secrets` (keychain: `Service(s).Set/Get/Delete`), `mygo.Clipboard`, `mygo.Dialog`, `mygo.Shell`,
`mygo.GlobalShortcut`, `mygo.Theme`, `mygo.Cookies`.

### 1.1 HOW IDENTITY (name / bundle id / data dir) IS DECIDED

`App.Name()` (app.go:340), in priority order:
1. `App.SetName(x)` if called (explicit override; "Call it before Run").
2. `packageInfo()` (deeplink.go:34): **macOS**: the main bundle's `Info.plist`
   (only if `[NSBundle mainBundle].bundlePath` ends with `.app`):
   `CFBundleDisplayName` else `CFBundleName` = Name; `CFBundleShortVersionString` = Version;
   `bundleIdentifier` (`CFBundleIdentifier`) = Identifier.
   If not a bundle (or non-macOS) fall back to ldflags vars (below) when `packageIdentifier != ""`.
3. executable file name (minus `.exe`) / `"MyGo"`.

ldflags (set by `mygo build`/`mygo dev`; unexported package vars, `-X` works on them):
`-X github.com/egoist/mygo.packageName=…  .packageVersion=…  .packageIdentifier=…  .packageURLSchemes=a,b  .packageFileExtensions=md,txt`.
Used for Linux (no metadata) and non-bundled runs. Name/Version/Identifier all go through this same path.

There is **no `SetIdentifier`** and no mygo.json/appfile read at run time. mygo.json/mygo.config.ts is read ONLY by
the `mygo` CLI at build time. (`appfile.go` in the root is just file-retry helpers; it is NOT an "appfile" config.)
At run time the identity therefore comes from: Info.plist (macOS bundle) / ldflags / `SetName`.

What identity drives at run time:

| Thing | Derived from |
|---|---|
| `PathUserData` | `os.UserConfigDir()/<App.Name()>` => `~/Library/Application Support/<Name>`, `~/.config/<Name>`, `%AppData%\<Name>`. **Name, NOT bundle id.** Two apps with the same Name SHARE this dir. |
| `PathCache` | `os.UserCacheDir()/<Name>` (`~/Library/Caches/<Name>`) |
| `PathLogs` | macOS `~/Library/Logs/<Name>`; else `<UserConfigDir>/<Name>/logs` |
| `window-state.json` (`WindowOptions.StateKey`) | `PathUserData` |
| `locale` file (Win/Linux) | `PathUserData` |
| Single-instance lock | `<bundle identifier>\n<Name>` hashed |
| Open-at-login entry | bundle identifier (login.go:63) |
| WKWebView default data store (macOS) | **[os]** WebKit keys `defaultDataStore` on the bundle identifier: `~/Library/WebKit/<CFBundleIdentifier>/…`, caches `~/Library/Caches/<id>`. MyGo has no code to override it. |
| WebView2 user data (Windows) | `<UserConfigDir>\<sanitized Name>\WebView2` (internal/windows/app.go:198); env `WEBVIEW2_USER_DATA_FOLDER` overrides |
| Linux WebKitGTK | `cookies.sqlite` etc in `~/.config/<Name>`; named sessions in `Sessions/<UUID>` under it |
| Notifications, URL scheme registration, file associations | Info.plist (macOS) |

**Per-app isolation recipe for generated bundles** (what our runtime must do):
1. Give every generated `.app` its own `CFBundleIdentifier` (=> own WebKit default store, own single-instance lock, own login item).
2. Give every app a unique `CFBundleName`/`CFBundleDisplayName` OR call `mygo.App.SetName(uniqueName)` from the runtime's `main()` before `Run` (=> own `PathUserData`/cache/logs).
3. The runtime `main()` may read its own config (e.g. JSON in `Contents/Resources`, found via `mygo.App.Path(mygo.PathResources)`) and call `SetName`/`SetVersion`/`SetPath` before `Run`; identifier can only come from Info.plist/ldflags.
4. Optional belt-and-braces for web data: `PageOptions.Session: mygo.NewSession("<appid>")` (persistent; macOS 14+ `dataStoreForIdentifier` UUID from the name; **before macOS 14 sessions are in-memory only**) – still no arbitrary directory API.
5. `mygo.App.SetPath(mygo.PathUserData, dir)` moves MyGo's own files (window state, locale) only; it does not move WebKit's store.

### 1.2 Paths
```go
dir, err := mygo.App.Path(mygo.PathUserData)   // created on first use
```
`PathName` consts: `PathHome PathAppData PathUserData PathCache PathLogs PathResources PathTemp PathExe PathDesktop PathDocuments PathDownloads PathMusic PathPictures PathVideos`.
`PathResources` = `Contents/Resources` in a macOS bundle, exe dir elsewhere, `./resources` under go run/test.

### 1.3 Deep links / file types
Declare `urlSchemes`, `fileAssociations` in mygo.json (CLI writes `CFBundleURLTypes`/`CFBundleDocumentTypes`). On Win/Linux a URL starts a new
process: use single instance lock; URLs reach `OnOpenURL` in first instance. Launch args containing URLs of
`packageURLSchemes` are recognised via ldflags var (deeplink.go) so generated bundles on macOS only need Info.plist.

---------------------------------------------------------------------------------------------------
## 2. Windows

```go
func NewWindow(opts WindowOptions) *Window   // after ready (waits if called off-main); panics on bad BackgroundColor
```
`WindowOptions` (window.go:62; ALL fields):
```go
Title string; URL string                    // URL "/" = app frontend; https://... loads remote
Width, Height int                           // default 800x600 (DIPs)
UseContentSize bool                         // Width/Height = page area
X, Y int                                    // centered when both 0
MinWidth, MinHeight, MaxWidth, MaxHeight int
FitContent bool                             // size to page content (not native UI)
Hidden bool                                 // use with win.OnReadyToShow(win.Show)
Frameless bool                              // CSS `--app-region: drag` for drag areas
TitleBarStyle TitleBarStyle                 // mygo.TitleBarDefault | TitleBarHidden | TitleBarHiddenInset
TrafficLightPosition *Point                 // macOS
TitleBarHeight int                          // Linux/Windows hidden title bar
HideWindowButtons bool
DisableResize, DisableMove, DisableMinimize, DisableMaximize, DisableClose, DisableFullScreen, DisableShadow bool
AlwaysOnTop, FullScreen, Maximized bool
SkipTaskbar bool; AutoHideMenuBar bool      // Linux/Windows
Transparent bool
BackgroundColor string                      // CSS: "#1e1e1e", "rgb(..)", "light-dark(#f5f5f7, #1e1e1e)"
Vibrancy Vibrancy; VibrancyTint string
CornerRadius int                            // frameless, macOS
Opacity float64
Parent *Window; Modal bool                  // child/sheet
Panel bool                                  // launcher-style floating panel
Page PageOptions                            // web page config (§3)
StateKey string                             // remember pos/size/maximized in window-state.json (PathUserData)
Content Content                             // native UI: ui.View(fn); URL/Page ignored
```
There is **no HideOnClose option**. Idiom (docs/menus.md):
```go
var quitting bool
mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })
win.OnClose(func(e *mygo.CloseEvent) { if !quitting { e.PreventDefault(); win.Hide() } })
```
Vibrancy consts: `VibrancyNone ""`, `VibrancyWindow Content Sidebar Header Titlebar UnderWindow UnderPage Menu Popover HUD Sheet Tooltip Selection FullScreenUI`,
`VibrancyGlass`, `VibrancyGlassClear` (macOS 26 Liquid Glass; Acrylic on Win11), `VibrancyMica VibrancyAcrylic VibrancyTabbed` (Win11).

`*Window` methods (window.go): `ID() int`, `IsDestroyed()`, `Parent()`, `Close()` (asks OnClose), `Destroy()` (no OnClose), `Show()`, `ShowInactive()`, `Hide()`,
`IsVisible() IsFocused() IsMinimized() IsMaximized() IsFullScreen()`, `Focus() Blur() Minimize() Maximize() Unmaximize() ToggleMaximize() Restore() Center()`,
`SetFullScreen(bool) ToggleFullScreen()`, `SetTitle(string) Title()`, `SetRepresentedFile(path)`, `SetDocumentEdited(bool)`,
`SetBounds(Rectangle) Bounds() Rectangle`, `SetContentBounds/ContentBounds`, `SetSize(w,h) Size()`, `SetContentSize(w,h) ContentSize()`, `SetPosition(x,y) Position()`,
`SetMinimumSize(w,h) SetMaximumSize(w,h)`, `SetResizable/IsResizable`, `SetMovable`, `SetMinimizable`, `SetMaximizable`, `SetClosable`, `SetAlwaysOnTop(bool)/IsAlwaysOnTop`,
`SetBackgroundColor(css string) error`, `SetOpacity(float64)`, `SetHasShadow(bool)`, `SetIgnoreMouseEvents(bool)`, `SetActiveAppearance(bool)`, `SetWindowButtonsTheme(ThemeSource)`,
`SetContentProtection(bool)`, `SetVibrancy(Vibrancy)`, `SetVibrancyTint(css) error`, `SetProgressBar(ProgressBar)`, `FlashFrame(bool)`, `SetSkipTaskbar(bool)`,
`SetVisibleOnAllWorkspaces(bool)`, `SetIcon(png []byte) error`, `SetMenu(*Menu)`, `SetAutoHideMenuBar(bool)`, `NativeHandle() uintptr`,
`Page() *Page` (nil for native-UI windows), `CapturePage() ([]byte, error)` (PNG),
`Update(fn func())`, `Invalidate()` (native UI redraw, §4.12), `NewWebView(WebViewOptions) (*WebView, error)`.
Events (each returns `off func()`): `OnClose(func(*CloseEvent))`, `OnClosed`, `OnFocus`, `OnBlur`, `OnShow`, `OnHide`, `OnReadyToShow`, `OnResize`, `OnMove`,
`OnMaximize`, `OnUnmaximize`, `OnMinimize`, `OnRestore`, `OnEnterFullScreen`, `OnLeaveFullScreen`, `OnFileDrop(func(*FileDropEvent))`.

Flyouts (popover windows anchored to tray/parent): `mygo.NewFlyout(mygo.FlyoutOptions{Tray, Parent, Anchor, Placement, Gap, Width, Height, Focusable, Shadow, Popover, FitContent, Content...}) *Window`.

---------------------------------------------------------------------------------------------------
## 3. WebView windows

```go
win := mygo.NewWindow(mygo.WindowOptions{
    Title: "Gmail", URL: "https://mail.google.com", Width: 1200, Height: 800,
    BackgroundColor: "light-dark(#fff,#1e1e1e)", Hidden: true, StateKey: "main",
    Page: mygo.PageOptions{
        Session:         mygo.NewSession("app-123"),       // isolated persistent store
        UserAgentSuffix: "MyPack/1.0",                      // or UserAgent: "full string"
        ZoomFactor:      1.0,
        PreloadScript:   `/* runs before page scripts, every navigation */`,
        DevTools:        mygo.DevToolsAuto,
    },
})
win.OnReadyToShow(win.Show)
```
Load: `p := win.Page()`: `LoadURL(rawURL string) error` (http(s), custom protocol; scheme-less = app frontend), `LoadFile(path) error` (local HTML; relative resolves vs cwd then exe dir/Resources),
`LoadHTML(html, baseURL string)`, `Reload() ReloadIgnoringCache() Stop() GoBack() GoForward() CanGoBack() CanGoForward() IsLoading() URL()`.

`PageOptions` fields (page.go:296): `PreloadScript string`, `PreloadAllFrames bool`, `TrustedOrigins []string` (origins that may call bound Go; remote pages untrusted by default),
`DevTools DevTools` (`DevToolsAuto` = on in dev builds, `DevToolsEnabled`, `DevToolsDisabled`), `ZoomFactor float64`, `UserAgent string`, `SafariTabFocus`, `PictureInPicture`,
`SwipeNavigation`, `PinchZoom`, `UserAgentSuffix string`, `ContentRules []*ContentRules`, `Session *Session`, `Untrusted bool`, `BackgroundThrottling` (`ThrottlingOn|ThrottlingOff`),
`Autoplay` (`AutoplayUserGesture|AutoplayAllow`), `SessionState []byte`.

Page methods (verified in source):
```go
SetZoomFactor(f float64); ZoomFactor() float64
SetUserAgent(ua string); UserAgent() string
Eval(code string) (any, error); EvalContext(ctx, code); mygo.EvalAs[T](p *Page, code string) (T, error)   // *EvalError on throw
OnWillNavigate(fn func(e *NavigateEvent)) (off func())     // POLICY HOOK; e.PreventDefault() cancels; e.URL, e.UserInitiated
OnDidNavigate(fn func(url string)); OnDOMReady(fn func()); OnDidFinishLoad(fn func()); OnDidFailLoad(fn func(*LoadError))
OnPageTitleUpdated(fn func(*TitleEvent)); OnRenderProcessGone(fn func(reason string)); OnContextMenu(fn func(*ContextMenuEvent))
SetWindowOpenHandler(fn func(req WindowOpenRequest) *WindowOptions)   // popups; nil return = deny
SetPermissionHandler(fn func(req PermissionRequest) bool)             // camera/mic/geo/notifications
SetAuthHandler(fn func(req AuthRequest) AuthResponse)                 // HTTP basic/digest/NTLM, server or proxy
SetCertificateErrorHandler(fn func(e CertificateError) bool)
OnWillDownload(fn func(e *DownloadEvent)); OnDownloadDone(fn func(d *Download))
FindInPage(text string, opts FindOptions) (FindResult, error); StopFindInPage()
SetContentRules(rules ...*ContentRules)
OpenDevTools() CloseDevTools() ToggleDevTools() IsDevToolsOpened()
Print(); PrintToPDF(PDFOptions) ([]byte, error); SessionState() ([]byte, error)
IsAudible() SetAudioMuted(bool) IsAudioMuted() OnAudibleChanged; ThemeColor() OnThemeColorChanged; CaptureState/SetCaptureState/OnCaptureStateChanged; TogglePictureInPicture()
Window() *Window; WebView() *WebView
```
Types: `NavigateEvent{Preventable; URL string; UserInitiated bool}`; `WindowOpenRequest{URL, FrameName string; Width, Height int; UserInitiated bool}`;
`PermissionRequest{Permissions []Permission; Origin string}` with `PermissionCamera|Microphone|Geolocation|Notifications`;
`DownloadEvent{Preventable; URL, SuggestedName, Path string}` (set `Path`, or `PreventDefault()`); `Download{URL, Path string; Err error}`;
`FindOptions{MatchCase, Backward, FindNext bool}`; `FindResult{Matches, Active int}`;
`AuthRequest{Host string; Port int; Realm, Scheme string; IsProxy bool}`, `AuthResponse{Username, Password string}` (empty = cancel).

Recipes:
```go
// Keep window on its site, send other http(s) links to the default browser
win.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
    u, err := url.Parse(e.URL)
    if err == nil && e.UserInitiated && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "mail.google.com" {
        e.PreventDefault(); go mygo.Shell.OpenExternal(e.URL)
    }
})                                  // NOT called for LoadURL(), nor (mostly) for iframes
// target=_blank / window.open: default = http(s) opens in default browser, other schemes denied
win.Page().SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
    if !req.UserInitiated { return nil }
    go mygo.Shell.OpenExternal(req.URL); return nil      // or return &mygo.WindowOptions{Width: 900, Height: 700}
})   // popup windows inherit the OPENER's session regardless of returned options
win.Page().SetPermissionHandler(func(r mygo.PermissionRequest) bool { return r.Origin == "https://meet.example.com" })
win.Page().OnWillDownload(func(e *mygo.DownloadEvent) { e.Path = filepath.Join(downloads, e.SuggestedName) })
win.Page().SetAuthHandler(func(r mygo.AuthRequest) mygo.AuthResponse { return mygo.AuthResponse{Username: u, Password: p} })
```
Without a download listener files go to `PathDownloads`, uniquely numbered. macOS camera/mic need `NSCameraUsageDescription` / `NSMicrophoneUsageDescription` in Info.plist or macOS kills the app.

**Injecting JS/CSS**: there is no AddUserScript/InsertCSS API. Use `PageOptions.PreloadScript` (runs before page scripts on every document; `window.mygo` available;
`PreloadAllFrames` for iframes) and `Page.Eval` for one-shots. CSS: preload script that appends a `<style>` on `DOMContentLoaded`, or `Eval`
`document.head.append(Object.assign(document.createElement('style'),{textContent:'…'}))` from `OnDOMReady`. `ContentRules` (WebKit content-blocker JSON, `mygo.CompileContentRules(id, json) (*ContentRules, error)`) can
`css-display-none` selectors and block loads.

**Sessions / isolation** (app.md "Sessions"):
```go
s := mygo.NewSession("work")            // persistent; same name => same *Session; callable before Run
p := mygo.NewEphemeralSession()         // incognito: memory only, new each call
mygo.DefaultSession()                   // == mygo.Cookies / App.ClearBrowsingData target
s.Cookies.List(mygo.CookieFilter{}) / .Set(mygo.Cookie{Name,Value,Domain,Path,Secure,HTTPOnly,SameSite,Expires}) / .Delete(c) / .Clear()
s.ClearData(mygo.SiteDataAll)           // also SiteDataCookies|LocalStorage|IndexedDB|Caches|ServiceWorkers
```
Persistence: macOS 14+ `WKWebsiteDataStore dataStoreForIdentifier:` (UUID from the name; earlier macOS = in memory only, logs a warning); Linux `Sessions/<UUID>` in user data dir;
Windows WebView2 profile `mygo-<UUID>`. **No API to choose an arbitrary on-disk directory for web data** on macOS (WebKit decides); isolation = bundle id (default store) and/or named session.
`mygo.CookiesModule`: `Cookies.List(CookieFilter{URL,Name,Domain,Path}) ([]Cookie, error)`, `Set`, `Delete`, `Clear`.

**Proxy**: no per-webview/per-session proxy setting (grep found none). Webviews follow the OS proxy (macOS System Settings, WinHTTP, GIO). Go-side HTTP:
`import "github.com/egoist/mygo/netproxy"`; `client := &http.Client{Transport: netproxy.Transport()}` or `t.Proxy = netproxy.Proxy` (honours HTTP(S)_PROXY/ALL_PROXY/NO_PROXY env first, then system settings).
HTTP auth (incl. proxy auth) via `SetAuthHandler`. Env var set before launch (HTTPS_PROXY) affects Go-side clients only, not WKWebView.

WebView inside native UI: `win.NewWebView(mygo.WebViewOptions{URL, Page PageOptions, BackgroundColor string, Transparent bool, ScrollToContainer bool}) (*mygo.WebView, error)`;
`v.Page()` gives the same `*Page` API (so all hooks above work); place it with `ui.WebView(c, v).Grow(1).Radius(12)`; paints under overlays (popover/toast/modal draw OVER it).
Window must have `Content`. `WebView` methods are goroutine-safe. See `examples/embed-webview`.

---------------------------------------------------------------------------------------------------
## 4. Native UI (package ui)

### 4.1 Model
Immediate-mode: the view func is re-run every frame on the main thread; state lives in YOUR struct; widgets take pointers to state and write edits back immediately.
```go
type app struct { win *mygo.Window; n int; name string }
func (a *app) view(c *ui.Context) {
    ui.Column(c).Fill().Padding(16).Gap(8).Children(func() {
        ui.Text(c, fmt.Sprint(a.n)).FontSize(40).Bold()
        if ui.PrimaryButton(c, "Increment").Clicked() { a.n++ }
        if ui.TextInput(c, &a.name).Placeholder("Name").Label("Name").Changed() { ... }
    })
}
a.win = mygo.NewWindow(mygo.WindowOptions{Title: "X", Width: 400, Height: 300, Content: ui.View(a.view)})
```
`func ui.View(view func(c *Context)) *Content`. The view first runs AFTER `NewWindow` returns (so `a.win` is usable inside it).
`ui.Element` values are valid for ONE build pass only: never store them in app state/goroutines (use `ui.Handle` for persistent identity). `mygo vet` flags violations.
Event query vs callback: `el.Clicked() bool` inline, or `el.OnClick(fn func())` (runs after the view finishes building; use when the action mutates the collection being iterated).
`Changed() bool`, `Submitted() bool` (Enter in single-line input), `OnChange(fn)`, `OnSubmit(fn)`.
Keys: `ui.Checkbox(c.Key(item.ID), &item.Done, label)` / `Element.Key(any)`; any container with a key makes a new identity scope. Give keys to list rows / anything whose order can change.

### 4.2 Layout (all `ui.X(c)` return `ui.Element`; chain methods; DIPs)
Containers: `Column(c)` (stretches children; `Box(c)` alias-ish), `Row(c)` (vertically centered), `Grid(c)` (`.Columns(3)`, `.ColumnTracks(ui.Fixed(220), ui.Fr(1), ui.FitContent())`, `.RowSpan/.ColumnSpan`),
`Scroll(c)`, `ScrollHorizontal(c)`, `ScrollBoth(c)`, `Spacer(c)` (takes free space), `Divider(c)`, `Themed(c, *Theme)`.
```go
func (e Element) Children(fn func()) Element        // build children inside
Fill() FillWidth() FillHeight() Grow(f float32) Shrink Basis Width(v) Height(v) Size(w,h) MinWidth MaxWidth MinHeight MaxHeight WidthPercent ... AspectRatio
Gap(v) GapX GapY Padding(v ...float32) /*1,2 or 4 values*/ PaddingX PaddingY Margin(v ...float32) /*ui.Auto allowed*/ MarginX MarginY
Justify(a Align) AlignItems(a Align) AlignSelf Center() Wrap() Reverse()   // ui.Start ui.Center ui.End ui.Stretch ui.Baseline ui.SpaceBetween ui.SpaceAround ui.SpaceEvenly
Absolute() Top/Right/Bottom/Left(v) Attach(at, self AnchorPoint) ZIndex(n) Clip() Hidden(bool) Invisible() Dividers(width, color)
```
Style: `Background(Color) TextColor(Color) FontSize(float32) Bold() Italic() Underline() SingleLine() Radius(r ...float32) Border(width float32, c Color) Shadow(x,y,blur,spread float32, c Color) Opacity(float32)
Translate(x,y) Scale(x,y) Rotate(deg)`. Colors: `ui.RGB(r,g,b uint8)`, `ui.RGBA(r,g,b uint8, alpha float32)`, `ui.Hex("#7c3aed")`, `ui.Transparent`.

### 4.3 Text, buttons
```go
func Text(c *Context, s string) Element;  func Textf(c, format string, args ...any) Element
func RichText(c *Context, spans ...Span) Element;  func Link(c *Context, label, url string) Element
func Button(c *Context, label string) Element;  func PrimaryButton(c *Context, label string) Element
func Toggle(c, on *bool, label string) Element;  func ToggleGroup(c, fn func()) Element
func Badge(c, text string) Element;  func Avatar(c, name string, image *Bitmap) Element
```
Icon-only button needs `.Label("Delete").Tooltip("Delete")`:
`ui.Button(c, "").Label("Delete").Tooltip("Delete").Children(func(){ ui.Icon(c, trash) }).Clicked()`.
Button: `Disabled(bool)`, `AccessDisabled(bool)`. Default-button shortcut: `ui.PrimaryButton(c,"OK").Clicked() || c.Shortcut(0, ui.KeyEnter)`.

### 4.4 Text input
```go
func TextInput(c *Context, value *string) Element    // single line; value written live as user types
func TextArea(c *Context, value *string) Element
func SearchField(c, query *string) Element; func Autocomplete(c, value *string, suggestions []string) Element; func TokenField(c, tokens *[]string, suggestions []string)
func NumberInput(c, value *float64, lo, hi, step float64) Element; func DecimalInput(c, value *string, lo, hi, step string)
func TextInputBuffer/TextAreaBuffer(c, *TextBuffer)   // indexed storage, buffer.Set("")
```
Modifiers: `.Placeholder(s)`, `.Label(s)` (a11y name), `.Password()`, `.AutoFocus()`, `.ReadOnly(bool)`, `.Disabled(bool)`, `.MaxLength(n)`, `.Lines(min,max)` (TextArea auto-grow), `.OnPaste(func(text string) bool)`,
`.OnTextKey(func(k ui.TextKey) bool)` (`k.Key`, `k.Mods`, `k.Text()`, `k.SetText(s)`), `.Changed()`, `.Submitted()`, `.TextSelection()`, `.SetTextSelection(a,b)`, `.Composing()`.
READ: the bound string is always current: `if ui.TextInput(c, &a.q).Changed() { search(a.q) }`.
WRITE: assign the string (`a.q = "x"`) -> editor replaced; undo history is kept. To start a fresh editing session change the key: `ui.TextInput(c.Key(a.session), &a.s)`.
Focus from elsewhere: `.Bind(&a.editor)` with a `ui.Handle` field, later `a.editor.Focus()`. IME: handled natively (composition, press-and-hold); `Composing()` reports it; keys typed during composition submit nothing.

### 4.5 Choice controls
```go
func Switch(c *Context, on *bool) Element                       // no text; give .Label("Wi-Fi") or wrap in ui.Field
func Checkbox(c, checked *bool, label string) Element;  CheckboxState(c, *CheckState, label)
func RadioGroup(c, fn func()) Element;  ui.Radio(c, &chosen, value, label)
func Select(c *Context, selected *string, options []string) Element      // dropdown; .Changed()
func Combobox(c, selected *string, options []string) Element
func Segmented(c *Context, selected *int, labels ...string) Element      // index
func Tabs(c, selected *int, labels ...string) Element
func Slider(c, value *float64, lo, hi float64) Element; StepSlider(.., step); RangeSlider; Stepper; Rating(c, *int, max int)
func ColorPicker/ColorWell(c, *Color); Calendar/DateInput/TimeInput(c, *time.Time)
func Form(c, fn); Field(c, label string, fn func()) Element; Fieldset(c, legend string, fn)
```
```go
ui.Select(c, &a.plan, []string{"Free", "Pro"}).Label("Plan")
ui.Segmented(c, &a.view, "List", "Grid").Label("View")
ui.Form(c, func(){ ui.Field(c, "Sound", func(){ ui.Switch(c, &a.sound) }) })
```

### 4.6 Navigation, sidebar, split, router
```go
func Sidebar(c *Context, selected *string, fn func()) Element           // source list; .Width(220); .Changed()
func SidebarSection(c, title string, open *bool, fn func()) Element     // open nil = always open
func SidebarItem(c, id string, icon *SVG, label string) Element         // .Children(func(){ ui.Badge(c, "3") })
func Split(c, size *float32, first, second func()) Element              // first-pane width in DIPs; SplitVertical; SplitPercent; SplitVerticalPercent; .SplitReset(220)
func Toolbar(c, fn) ; Breadcrumbs(c, items []string, chosen *int); Accordion/AccordionItem; Collapsible; Tree/TreeItem
ui.NewRouter("/notes") -> *ui.Router (store in app struct; .Path(), .Push(path)); ui.BackButton(c, r), ui.ForwardButton(c, r); c.Router()
```
Idiom (examples/embed-webview): `ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func(){ ui.Sidebar(c,&a.tab,func(){...}).Width(180); ui.Column(c).Grow(1).MinWidth(0).Children(...) })`.

### 4.7 Lists, tables, grids
```go
func List(c *Context, s *ListState, n int, row ...func(int)) Element    // virtualized; s may be nil
ui.List(c, nil, len(a.items), func(i int){ ui.Text(c, a.items[i].Name).Padding(6,12) }).Grow(1)
func Table(c, s *ListState, columns []TableColumn, n int, cell func(row, col int)) Element
func GridView(c, s *GridState, n int, minWidth, height float32, item func(i int)) Element   // virtualized thumbnails
```
`ListState` (in your state): `Key` func (stable item identity), `Selected *…`, `FollowEnd` (chat/log), `KeepOffset`, scroll-to methods. `List(...).Gap(n).Padding(n).Justify(ui.End)`; children built via `.Children` show when empty.
Key rows: `ui.Row(c.Key(item.ID))`.

### 4.8 Images and icons
```go
func DecodeBitmap(data []byte) (*Bitmap, error)   // PNG/JPEG/GIF/WebP/BMP bytes; EXIF-rotated. Make ONCE, not in view
func NewBitmap(img image.Image) *Bitmap
func (b *Bitmap) Update(img), ReleasePixels()
func Image(c *Context, src ImageSource) Element    // *Bitmap or *SVG; .Size(64,64).Fit(ui.Contain|ui.Cover).Radius(12)
func ParseSVG(data []byte) (*SVG, error);  func MustParseSVG(data []byte) *SVG
func Icon(c *Context, s *SVG) Element              // colored by text color, sized by FontSize: .FontSize(24).TextColor(c.Theme().Danger); .Rotate(...)
```
```go
//go:embed icons/save.svg
var saveSVG []byte
var save = ui.MustParseSVG(saveSVG)
bmp, _ := ui.DecodeBitmap(pngBytes)        // PNG from file: os.ReadFile -> DecodeBitmap
ui.Image(c, bmp).Size(48, 48).Fit(ui.Contain)
```
Frames from goroutines: `win.Update(func(){ a.video.Update(frame) })`. **Lucide is NOT bundled in mygo**; any Lucide/Heroicons SVG works via ParseSVG, or use Lobe MyGo's `pkg/lucide` (§10).
macOS SF Symbols: `plugins/sfsymbols`. Menu icons: `mygo.NewMenuIcon(png []byte, template bool) (*MenuIcon, error)`, `mygo.NewMenuIconSVG`.

### 4.9 Progress, feedback, overlays
```go
func Progress(c, value float64) Element          // 0..1; negative = indeterminate; .Label("Uploading")
func Spinner(c) Element                          // .Label("Loading").Size(32,32)
func Meter(c, value, lo, hi float64, levels *MeterLevels) Element
c.Toast("Saved")                                 // string
c.ToastAction("Note deleted", "Undo", func(){ a.restore() })   // action button, longer
id := c.AddToast(ui.Toast{ID, Title, Description, Type: "error", Timeout: -1 /*until closed; 0=4s*/, Action: "Undo", OnAction: fn, OnClose: fn}); c.CloseToast(id)
func Modal(c, open *bool, fn func()) Element     // dialog over dimmed window; Escape/outside click sets *open=false; .Dismissed()
func AlertDialog(c, open *bool, title, message string, buttons ...string) int   // last button = default; returns index, ui.AlertDismissed on Esc, else -1
func Popover(c, anchor Element, open *bool, fn func()) Element       // below anchor; flips above; .Dismissed()
func MenuButton(c, label string, build func(m *Menu)) Element; PopoverMenuButton(c, label, build, opts...)
el.Tooltip("text")                                // delay: theme.TooltipDelay
el.ContextMenu(func(m *ui.Menu){ if m.Item("Rename").Shortcut(0, ui.KeyF2).Chosen() {...}; m.Submenu("Move", func(m *ui.Menu){...}); m.Separator()
        m.Item("Pinned").Checked(on).Chosen(); m.Item("Del").Disabled(b).Icon(ic).Chosen() })     // system context menu
func Overlay(c, fn func());  func FindBar(c, open *bool, query *string, matches int, current *int) Element
```
Modal example: `ui.Modal(c, &a.renaming, func(){ ui.Text(c,"Rename").Bold(); ui.TextInput(c,&a.name).AutoFocus(); ui.Row(c).Gap(8).Justify(ui.End).Children(...) })`.
Dialogs/popovers draw INSIDE the window (clipped to it). Native OS dialogs: `mygo.Dialog` (§5).

### 4.10 Theme, colors, animation
```go
t := c.Theme()                                  // *ui.Theme: Dark, Background, Surface, SurfaceHover, SurfacePressed, Border, Text, TextMuted, Accent, AccentHover, AccentPressed, AccentText,
                                                //   InactiveAccent, Danger, Warning, Success, Selection, Focus, FocusRingWidth/Offset, Inverse, InverseText, Scrollbar, ScrollbarWidth,
                                                //   Radius, Spacing (default 4), FontSize, Font, TooltipDelay time.Duration;  t.Space(n), t.Rem(n)
nt := *c.Theme(); nt.Accent = ui.Hex("#7c3aed"); nt.Radius = 8; c.SetTheme(&nt)   // follows system light/dark by default
mygo.Theme.SetSource(mygo.ThemeDark /*ThemeLight|ThemeSystem*/); mygo.Theme.IsDark(); mygo.Theme.OnUpdated(fn)   // app-wide appearance (call before Run OK)
p := c.Preferences()                            // Accent, HighContrast, TextScale, ReduceMotion, Pointer...
ui.Themed(c, &theme).Children(...)              // theme for part of the window
c.Vibrancy() bool; c.Root().Background(ui.Transparent) // for vibrancy windows
c.TitleBar() ui.TitleBar{Left,Right,Height…}; el.DragWindow()                      // with TitleBarHidden
c.SetZoom(1.25); c.Zoom(); c.Size() (w,h); c.WindowActive() bool; c.Locale() string
```
Animation: `el.Animate(key any, target float32, d time.Duration) float32`, `el.AnimateTo(key, target, ui.Snappy|ui.Smooth|ui.Bouncy)`, `el.Spring(...)`, `el.Loop("spin", time.Second, ui.Linear)` ;
transitions via element transition options (docs/ui/transitions.md); `c.After(d)` schedules a frame; `c.AnimationFrame()`.

### 4.11 Keyboard, focus, accessibility
```go
c.Shortcut(ui.Cmd, ui.KeyS) bool                // anywhere in window; ui.Cmd = Command on mac, Ctrl elsewhere; ui.Shift ui.Ctrl ui.Alt ui.Super
el.Shortcut(mods, key)                           // only when el/descendant has focus
c.OnShortcut(ui.Cmd, ui.KeyK, fn); c.OnWindowShortcut(mods, key, fn); handle.OnShortcut(c, mods, key, fn)
ui.KeyEnter ui.KeyEscape ui.KeyUp ui.KeyF2 ...  // keys.go; el.KeyShortcuts(ui.Cmd, ui.KeyS) tells screen readers
var h ui.Handle; el.Bind(&h); h.Focus(); c.Focused(h); c.FocusWithin(h); el.AutoFocus(); el.Focusable(); el.FocusGroup(ui.Horizontal)
el.FocusBind(&a.pane, files); ui.FocusedValue(c, &a.pane)
el.Label("Name") .Description("…") .Role(ui.RoleSwitch) .AccessHidden(true); c.Announce("Saved")   // a11y (VoiceOver/NVDA/Orca)
el.Hovered() Pressed() Clicked() DoubleClicked() RightClicked() Dragged() PointerPosition()
```

### 4.12 State and redrawing from a goroutine (THREADING)
View + state are touched on the MAIN thread only. From any other goroutine:
```go
go func() {
    items, err := fetch()
    win.Update(func() { a.items, a.err = items, err })   // runs fn on main thread, THEN redraws; returns immediately
}()
win.Invalidate()                                          // redraw only (safe from any goroutine; coalesced) – for state you guard yourself
mygo.RunOnMain(func(){ if n != a.n { a.n = n; win.Invalidate() } })   // compare-then-invalidate (no redraw if unchanged)
ctx.Services().Invalidate()                               // ui.Services (persist across builds) – Invalidate callable from any goroutine; clipboard/URL methods run on UI thread
c.Invalidate() / c.After(d)                               // inside the view: another frame now / later
```
`Update` rebuilds the view after EVERY call. Never mutate view state from a goroutine without `Update`. Never keep `ui.Element`/`*ui.Context` build values in goroutines.

### 4.13 Windows w/ native UI
`TitleBarStyle: mygo.TitleBarHidden` + `c.TitleBar()`; `Vibrancy`; multiple windows can share `ui.View(fn)` (each gets its own element state). `Window.Page()` nil, `CapturePage()` returns a PNG.
Testing without a window: `tt := ui.NewTester(app.view, 320, 240); tt.Click("Increment"); tt.HasText("1"); tt.Find("Volume"); tt.Key(ui.Cmd, ui.KeyS); tt.Type("x"); tt.RightClick; tt.ChooseMenuItem(...)`.

---------------------------------------------------------------------------------------------------
## 5. Menus, tray, shortcuts, notifications, dialogs, clipboard, shell

### Menus
```go
menu := mygo.NewMenu([]*mygo.MenuItem{
    {Role: mygo.RoleAppMenu},
    {Label: "File", Submenu: []*mygo.MenuItem{
        {Label: "New", Accelerator: "CmdOrCtrl+N", Click: func(item *mygo.MenuItem, win *mygo.Window) { ... }},
        mygo.Separator(),
        {ID: "side", Label: "Sidebar", Type: mygo.MenuItemCheckbox, Checked: true},
        {Role: mygo.RoleClose}}},
    {Role: mygo.RoleEditMenu}, {Role: mygo.RoleWindowMenu},
})
mygo.App.SetMenu(menu)           // default App/Edit/View/Window menu installed if never called
```
`MenuItem{ID, Label, Type(MenuItemNormal|Checkbox|Radio|Separator|Submenu), Role, Accelerator, Disabled, Hidden, Checked, ToolTip, Icon *MenuIcon, Subtitle, Submenu, Click}`.
Roles incl. `RoleAppMenu RoleEditMenu RoleWindowMenu RoleQuit RoleClose RoleToggleDevTools RoleToggleFullScreen RoleOpenRecent`.
`Menu`: `Items() Append(items...) Insert(pos,item) Remove(ids...) ItemByID(id) Popup(win) PopupAt(win,x,y)`. Item setters: `SetEnabled SetVisible SetChecked SetLabel SetAccelerator SetSubtitle SetIcon`. `win.SetMenu(m)` (Win/Linux per-window).
Dock menu: `App.Dock.SetMenu`.

### Tray / menu bar status item
```go
tray, err := mygo.NewTray(mygo.TrayOptions{ID: "main", Icon: pngBytes, IconIsTemplate: true, Title: "", ToolTip: "App", Menu: menu})
tray.SetIcon(png []byte, template bool) error; tray.SetTitle(s) /*macOS text next to icon*/; tray.SetToolTip(s); tray.SetMenu(*Menu); tray.PopUpMenu()
tray.Bounds() mygo.Rectangle; tray.OnClick(fn); tray.OnRightClick(fn); tray.Dark() bool; tray.OnAppearanceChange(fn); tray.Destroy()
```
Menu is shown on click when set; with no menu, clicks reach `OnClick`. Tray does not keep app alive. Menu-bar-only app: `mygo.json` `"macos":{"infoPlist":{"LSUIElement":true}}` (+ `SetActivationPolicy(mygo.ActivationPolicyAccessory)`).
Panel under tray: `mygo.NewFlyout(FlyoutOptions{Tray: tray, Placement: mygo.PlacementBottom, Gap: 4, Width: 280, Height: 230, Focusable: true, Shadow: true, Popover: true, Content: ui.View(v)})` (examples/menubar-flyout)
or a hidden Frameless AlwaysOnTop SkipTaskbar window positioned with `tray.Bounds()` + `mygo.Screen.DisplayNearestPoint(mygo.Point{X,Y}).WorkArea` (examples/menubar-native). Linux trays report no clicks/bounds.

### Global shortcuts
`err := mygo.GlobalShortcut.Register("CmdOrCtrl+Shift+Space", func(){ win.Show(); win.Focus() })`; `Unregister(acc)`, `UnregisterAll()`, `IsRegistered(acc)`. Callback on main thread; fails if taken.

### Notifications
```go
n := mygo.NewNotification(mygo.NotificationOptions{ID, Title, Subtitle, Body, Silent, Group, Image []byte, Sender *mygo.NotificationSender})
n.OnClick(func(){ win.Focus(); n.Close() }); err := n.Show()  // also ShowContext(ctx); returns mygo.ErrNotificationsDenied
mygo.NotificationsSupported() bool; mygo.ClearNotifications(); mygo.App.OnNotificationClick(func(id string){})
```
macOS: packaged apps only (needs a real bundle id); first Show prompts user. No action-button field exists.

### Dialogs (block; call from goroutine / bound method, NOT from the main thread event handler directly)
```go
paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title, DefaultPath, ButtonLabel, Message, Filters: []mygo.FileFilter{{Name:"Images", Extensions: []string{"png","jpg"}}},
    Directory: true, Multiple: true, ShowHiddenFiles, CreateDirectories, TreatPackagesAsDirectories})   // nil on cancel
path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent, Title, DefaultPath, NameFieldLabel, Filters...})   // "" on cancel
res, err := mygo.Dialog.Message(mygo.MessageOptions{Parent, Type: mygo.MessageQuestion /*None|Info|Warning|Error|Question*/, Title, Message, Detail,
    Buttons: []string{"Save","Cancel"}, DefaultButton: 0, CancelButton: 1, DestructiveButtons: []int{..}, CheckboxLabel, CheckboxChecked})  // res.Button, res.CheckboxChecked
mygo.Dialog.Error(title, content string)
```

### Clipboard / shell
```go
mygo.Clipboard.WriteText(s); s := mygo.Clipboard.ReadText(); WriteHTML(html); WriteImage(png []byte); ReadImage() []byte; AvailableFormats(); Clear(); ReadFiles()/WriteFiles(paths...)
mygo.Shell.OpenExternal("https://…")      // default browser (also mailto:, custom schemes)
mygo.Shell.OpenPath(path)                 // default app for file
mygo.Shell.ShowItemInFolder(path)         // Reveal in Finder
mygo.Shell.TrashItem(path); Beep(); FileIcon(path, px int) ([]byte, error) /*PNG*/; TypeIcon(ext, px); InstalledApps(); LaunchApp(app)
```
In view: `c.OpenURL(url)`, `c.ReadClipboard()`, `c.WriteClipboard(s)`.
Other: `mygo.Screen` displays (`DisplayNearestPoint(Point).WorkArea`), `mygo.Power.IsOnBattery()`, `mygo.Secrets.Service("x").Set(key, []byte)`/`Get`/`Delete`, file watcher (`watch.go`), `mygo.Network`.

---------------------------------------------------------------------------------------------------
## 6. i18n

- Catalogs: ARB-style JSON per BCP-47 tag in a dir (`locales/en.json`, `zh-Hans.json`, `pt-BR.json`); config `"i18n": {"dir":"locales","source":"en"}` in mygo.json.
  Messages use ICU MessageFormat: `"filesSelected": "{count, plural, =0 {No files} one {# file} other {# files}}"`; `"@key": {description, placeholders}` metadata in source file.
- `mygo generate` (run by `dev`/`build`) writes `locales/catalog_gen.go` (commit it): typed funcs `locales.FilesSelected(3)`, `locales.In("de").FilesSelected(3)`, `import _ ".../locales"` registers languages.
- Language shown: `mygo.App.Locale()` = best match of `PreferredLanguages()` among catalog languages, else source. `App.SetLocale("de")` (persisted: macOS AppleLanguages; Win/Linux `locale` file in PathUserData; "" = follow system),
  `App.Languages() []string`, `App.SetLanguages("en","zh-Hans")` for apps without catalogs, `App.OnLocaleChanged(func(string))`. Env `MYGO_LOCALE`, `mygo dev -lang de`; pseudo locales `en-XA`, `ar-XB`.
- Native UI re-runs the view each frame so `ui.Text(c, locales.Title())` follows live; re-set one-time strings (window titles, menu labels) in `OnLocaleChanged`. RTL auto (`MYGO_DIRECTION`).
- `c.Locale()`, `c.Region()`, `el.Lang("ja")`; Go formatting via package `github.com/egoist/mygo/i18n` (numbers/dates/currency/CLDR, `i18n.Autonym(tag)`).
- App name per language: catalog keys `mygo.appName`, `mygo.appDescription`; MyGo's own control/menu strings translate automatically; override with `mygo.*` keys. `mygo i18n check` lints catalogs.
- macOS: bundle gets `CFBundleDevelopmentRegion`, `CFBundleLocalizations` and `<lang>.lproj/InfoPlist.strings` per language (CLI writes them).

---------------------------------------------------------------------------------------------------
## 7. Build/packaging CLI

CLI: `go install github.com/egoist/mygo/cmd/mygo@latest`, `go run github.com/egoist/mygo/cmd/mygo@latest <cmd>`, or as go.mod tool: `go tool mygo <cmd>` (native template).
Commands: `mygo init [-template web|native] [-name] [-module] [-mygo checkout] <dir>`, `mygo dev [-skip-dev-command] [-sign id] [-web] [-port] [-lang tag]`,
`mygo build [-platform list] [-debug] [-o dir] [-sign id] [-skip-build-command] [-skip-dmg] [-skip-notarize] [-skip-installer] [-upload]`, `mygo generate [-o file]`, `mygo i18n check`,
`mygo keygen`, `mygo doctor`, `mygo version`, `mygo install-skills`. Platforms: `-platform darwin/universal,darwin/arm64,windows/amd64,linux/amd64,web`.
Env: `CGO_ENABLED` (CLI defaults 0), `MYGO_LDFLAGS`, `MYGO_INSPECTOR=1` (keep inspector in prod), `MYGO_ENV=production`, `MYGO_LOCALE`, `MYGO_CLI_BINARY`, `MYGO_UPDATER_PRIVATE_KEY`, `MYGO_WINDOWS_CERTIFICATE_PASSWORD`.

Config: `mygo.json` (no JS runtime) OR `mygo.config.ts` (`defineConfig` from `mygo-cli`, needs Bun/Node) – never both. Same fields. Read by CLI only.
```json
{ "name": "My App", "slug": "my-app", "identifier": "com.example.myapp", "version": "1.2.0", "copyright": "© 2026", "publisher": "", "icon": "resources/icon.png", "main": ".", "out": "build",
  "resources": ["third_party/licenses"], "resourcesExclude": ["**/*.psd"], "urlSchemes": ["myapp"],
  "fileAssociations": [{"ext":["md"],"name":"Markdown Document","mimeType":"text/markdown","role":"Editor"}],
  "i18n": {"dir":"locales","source":"en"},
  "updates": {"publicKey":"…","github":"you/app"},
  "macos": {"signingIdentity":"Developer ID Application: X (TEAMID)","minimumSystemVersion":"12.0","entitlements":"ent.plist","infoPlist":{"LSUIElement":true,"NSCameraUsageDescription":"…"},
            "icon":"AppIcon.icon","dmgTitle":"","notarize":{"keychainProfile":"notary"},"provisioningProfile":"","helperEntitlements":{}},
  "windows": {"displayName":"","installDirName":"","certificate":"x.pfx","signCommand":"","installerInclude":""},
  "linux": {"maintainer":"","comment":"","categories":["Utility"],"depends":[],"webview":false,"command":"my-app"} }
```
Defaults: name = dir name; slug = kebab(name); identifier = `com.mygo.<name letters/digits>`; version `0.1.0`; icon `resources/icon.png`; out `dist`. `linux.webview:false` for native-UI-only apps.
Frontend fields (web apps): `devUrl devCommand buildCommand frontendDist bindings`.

Icon: square PNG (1024). CLI renders `.icns` -> `Contents/Resources/<icon>.icns` (`CFBundleIconFile`), Windows ICO resource, Linux png. `macos.icon` can be `.icns`, `Assets.car`, or Icon Composer `.icon`
(needs Xcode 26 `actool`; name must be `AppIcon.icon`). `icon-dev.png` for dev builds. Runtime tray/menu icons are plain PNG bytes (embed them).

Outputs in `<out>/<os>-<arch>/`: macOS `My App.app` (signed) + `my-app-0.1.0-arm64.dmg` (`…-0.1.0.dmg` for universal); Windows `My App.exe` + NSIS installer `my-app-setup-0.1.0-amd64.exe`; Linux `my-app`, `.desktop`, `.png`, `.tar.gz`, `.deb`, `PKGBUILD`.
Signing: default ad hoc (`-`); `macos.signingIdentity` Developer ID + hardened runtime; `notarize.keychainProfile` (from `xcrun notarytool store-credentials`); `entitlements` plist; helper executables in resources can have own entitlements.

### 7.1 Anatomy of a mygo macOS bundle (cmd/mygo/bundle.go writeBundle/infoPlist) – replicate this when assembling .app programmatically
```
Foo.app/Contents/
  Info.plist
  PkgInfo                    ("APPL????")
  MacOS/<CFBundleExecutable>          // the Go binary (name == bundle base name by mygo's convention)
  Resources/<icon>.icns, [Assets.car], <lang>.lproj/InfoPlist.strings, + all `resources` files (PathResources)
  [embedded.provisionprofile]
```
Info.plist keys written by MyGo (then `macos.infoPlist` overrides/extends):
`CFBundleName`, `CFBundleDisplayName` (both = name), `CFBundleIdentifier`, `CFBundleVersion` and `CFBundleShortVersionString` (= version), `CFBundleExecutable`, `CFBundlePackageType=APPL`,
`CFBundleInfoDictionaryVersion=6.0`, `LSMinimumSystemVersion` (default 12.0), `NSPrincipalClass=NSApplication`, `NSHighResolutionCapable=true`, `NSSupportsAutomaticGraphicsSwitching=true`,
`CFBundleIconFile` ("AppIcon"-style name of the .icns), `CFBundleIconName` (if Assets.car), `NSHumanReadableCopyright`, `CFBundleDevelopmentRegion` + `CFBundleLocalizations` (if i18n),
`CFBundleDocumentTypes` (file associations: CFBundleTypeName/Role/Extensions/MIMETypes, LSHandlerRank=Default), `CFBundleURLTypes` ([{CFBundleURLName: identifier, CFBundleURLSchemes: [...]}]).
Add yourself when needed: `LSUIElement` (no Dock icon), `NSCameraUsageDescription`, `NSMicrophoneUsageDescription`, `LSApplicationCategoryType`.

**What MyGo reads at runtime from the bundle**: ONLY `[NSBundle mainBundle]` -> `bundlePath` must end in `.app`; `CFBundleDisplayName`/`CFBundleName` (Name), `CFBundleShortVersionString` (Version), `bundleIdentifier` (Identifier);
`Contents/Resources` as `PathResources`; URL schemes/doc types via LaunchServices (so Info.plist must declare them). There is no embedded appfile/config read; nothing else is mandatory.
Our own runtime config can be any file under `Contents/Resources` read via `App.Path(PathResources)`.
Consequences for self-assembled bundles: copy the prebuilt runtime binary to `Contents/MacOS/<CFBundleExecutable>`, write Info.plist with UNIQUE `CFBundleIdentifier` + unique name, then **re-sign**
(`codesign --force --deep --sign - Foo.app`; Apple Silicon refuses unsigned/invalid-signature modified binaries; the signature seals Info.plist+Resources so sign AFTER writing them). Launch via `open Foo.app` or exec Contents/MacOS/…
(`mygo dev` uses a real bundle in `.mygo/dev`; dev app is named `<Name> Dev` with id `<identifier>.dev` to isolate data).
Notifications and open-at-login require this packaged form. Moving/renaming a bundle does not change identity (bundle id is).

---------------------------------------------------------------------------------------------------
## 8. Production build flags & cross compiling

`mygo build` compiles with: `-trimpath -ldflags "-s -w <package flags> -X github.com/egoist/mygo.production=1 [-H=windowsgui on windows] $MYGO_LDFLAGS" [-tags mygo_noinspector]` (tags from GOFLAGS preserved).
`-debug` or `MYGO_INSPECTOR=1` keep the inspector (and the automation plugin). `mygo.IsDev()` is false when `production==1` or `MYGO_ENV=production` (dev builds get web inspector by default via `DevToolsAuto`).
Plain go build equivalent:
```sh
CGO_ENABLED=0 go build -trimpath -tags mygo_noinspector \
  -ldflags "-s -w -X github.com/egoist/mygo.production=1 \
            -X github.com/egoist/mygo.packageName='My App' -X github.com/egoist/mygo.packageIdentifier=com.example.app -X github.com/egoist/mygo.packageVersion=1.0.0" -o app .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w -H=windowsgui -X github.com/egoist/mygo.production=1" -o app.exe .
GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 go build -o app .      # needs GTK3 (+WebKitGTK for webviews) at runtime, loaded via purego
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o app .      # darwin/universal = build both + lipo (CLI does it)
```
No cgo needed for any platform (set `CGO_ENABLED=1` only for third-party cgo deps). Windows apps need the WebView2 Runtime for web windows only. Dev inspector for native UI is in `ui` with build tag `!mygo_noinspector`.

---------------------------------------------------------------------------------------------------
## 9. Automation plugin (drive/screenshot a running native-UI app)

Enable (test build only; excluded in `mygo_noinspector` builds):
```go
//go:build e2e
package main
import _ "github.com/egoist/mygo/plugins/automation"
```
```sh
go build -tags e2e -o /tmp/app . && MYGO_AUTOMATION=/tmp/app.sock /tmp/app &       # listens ONLY if env var set; socket mode 0600
go install github.com/egoist/mygo/plugins/automation/cmd/mygo-automation@v0.8.3   # CLI (stdlib only)
mygo-automation skills                       # prints full agent guide for the installed version
mygo-automation open --socket /tmp/app.sock  # session (default "default"; --session NAME or $MYGO_AUTOMATION_SESSION)
mygo-automation windows                      # w1 [content] "Title" focused
mygo-automation snapshot -i                  # @e2 [button] "Add" key=add   (-i controls only, --full details/bounds, --json raw)
mygo-automation press @e2 --settle           # or key=add / id=<digits>; prints +/-/= diff
mygo-automation fill key=name "Ada" --settle # replace text; also: focus, act <sel> <action>, click <sel>|<x> <y> [--button right --clicks 2 --mods cmdOrCtrl]
mygo-automation type "text"; mygo-automation key CmdOrCtrl+A Enter; mygo-automation wheel <sel>|x y dx dy; mygo-automation pointer move|down|up x y
mygo-automation menu File "New Window"       # application menu path (no context/tray/Dock menus)
mygo-automation wait --text "Saved" --timeout 5s     # also: wait <sel>, wait --gone, wait (settle)
mygo-automation screenshot /tmp/app.png      # PNG of window, prints "<file> WxH" (device px)
mygo-automation eval 'document.title'        # page windows only (no tree for web pages)
mygo-automation close                        # ends session, app keeps running
```
Give elements stable `Key`s (`ui.Button(c,"Add").Key("add")`, `c.Key("name")` before `ui.TextInput`) and `Label`s to make selectors reliable. Protocol: newline-delimited JSON over unix socket;
methods `windows tree act input menu capture eval idle`. Web windows: use `eval`. Errors: `no_window no_element unsupported timeout failed stale ref`. In-process alternative: `ui.NewTester` in `go test`.

---------------------------------------------------------------------------------------------------
## 10. Lobe MyGo component library

EXISTS: `github.com/lobehub/lobe-mygo` (`go list -m -versions` shows no tagged versions; `@latest` = `v0.0.0-20261109101238-5b3ecde8b184`, go 1.27.3; downloaded to module cache). It requires
`github.com/egoist/mygo v0.8.4-0.20261108082913-9233663736ca` (a pseudo-version NEWER than v0.8.3, so `go get` will bump mygo). Packages: `pkg/{action,field,choice,display,feedback,overlay,nav,layout,code,markdown,theme,motion,lucide}`
(70+ Lobe UI components; `pkg/lucide` = all 2300+ Lucide icons as funcs returning `*ui.SVG`, e.g. `lucide.Save()`, `lucide.Settings()`; `display.Icon(c, svg, display.IconProps{Size, Spin})`).
Usage: call `theme.Apply(c)` first in each view; tokens `theme.TokensOf(c).BgContainer`; `action.Button(c, "Hello", action.ButtonProps{Variant: action.Primary}).Clicked()`. Skeleton app: its `examples/desktop`; gallery: `go run ./catalog`. Docs https://mygo.lobehub.com.

---------------------------------------------------------------------------------------------------
## 11. Gotchas

1. **Threads**: all MyGo functions are goroutine-safe (marshalled to main thread, caller waits), BUT: `App.Run()` must be the main goroutine; listeners (`OnClose`, menu `Click`, shortcuts, `OnWillNavigate`, permission/auth handlers) run ON the main thread – keep them short, never block (no `Dialog.*`, network, sleeps inside them – spawn a goroutine). Bound methods run on their own goroutines.
2. Before `Run` only configuration is allowed (`SetName`, `SetMenu`, `Theme.SetSource`, `Bind`, `OnOpenURL`, `NewSession`, `RegisterURLScheme`, `SetOpenAtLogin`, `Path`). Clipboard, displays, dialogs, notifications, trays, global shortcuts, `Theme.IsDark` before Run PANIC. Windows only after ready (`WhenReady`; from other goroutines `NewWindow` waits).
3. Native UI: view runs on main thread each frame; mutate state only there or via `win.Update(func)`. `ui.Element` and the build-time `*ui.Context` pieces are invalid after the pass; never store them in structs/goroutines (`mygo vet`). `Update` always redraws; use `RunOnMain`+`Invalidate` to avoid redraw churn.
4. Never loop/mutate the collection you are building from `Clicked()` – use `OnClick` or record and apply after the loop. Key stateful widgets in dynamic lists (`c.Key(id)`), keys unique among siblings; duplicate keys panic in tests.
5. Make bitmaps/SVGs ONCE (package var / init), not per frame; `DecodeBitmap` in the view re-uploads each frame.
6. Identity: PathUserData follows App **Name** not bundle id – two generated apps with the same display name share data dir/window-state/locale. WKWebView default store follows bundle id. `SetName` after Run has no effect on already-resolved paths.
7. macOS sessions (`NewSession`) are in-memory only before macOS 14. No API to put WebKit data in a custom folder. No per-session proxy API.
8. `OnWillNavigate` does not fire for `LoadURL`, and only fires for main-frame navigations (Linux <WebKitGTK 2.56 also iframes). During `mygo dev` app pages are `http://localhost` – host-check rather than scheme-check. Popups always inherit the opener's session.
9. Remote pages cannot call bound Go methods unless origin in `PageOptions.TrustedOrigins` (`"*"` trusts all – avoid). `Untrusted: true` for third-party content you host from a custom scheme.
10. WebView `BackgroundColor` flashes white until first paint: create `Hidden: true` then `OnReadyToShow(win.Show)`. Native-UI windows show at first frame automatically.
11. Closing last window quits the app unless an `OnWindowAllClosed` listener exists; macOS convention: keep running + `OnActivate` reopen. Tray alone does not keep the app alive. Hide-on-close = OnClose+PreventDefault+Hide (remember `OnBeforeQuit` flag so Cmd+Q still works).
12. `Dialog.Open/Save/Message` block the calling goroutine; with `Parent: win` they are sheets on macOS.
13. Camera/mic: missing `NS*UsageDescription` in Info.plist => macOS terminates the process.
14. Notifications need a real bundle (packaged). `go run` binaries get none. Single-instance/open-at-login also best from a bundle.
15. Modified/copied bundle binaries must be re-signed (ad hoc OK) after editing Info.plist/Resources. Gatekeeper blocks ad hoc apps on other machines.
16. `Window.Update/Invalidate` do nothing for web-page windows; `Window.Page()` is nil for native-UI windows (nil-check before `win.Page().…`). `NewWebView` errors on non-Content windows.
17. Web view inside native UI is always painted UNDER native overlays; web contents don't get a11y tree entries (single `webview` node).
18. Linux: WebKitGTK DMA-BUF renderer disabled by default (`App.SetDMABUFRenderer`); tray gives no click/bounds events (menu only).
19. `mygo.json` and `mygo.config.ts` are build-time only; do not expect runtime reads. App Name/Version/Identifier for non-bundled launches come from ldflags `packageName/Version/Identifier`.
20. Dev/Prod: `mygo.IsDev()`; `-X …mygo.production=1` removes web inspector default and DevTools; `-tags mygo_noinspector` removes native-UI inspector AND automation plugin.
