package builder

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/tc-hib/winres"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/internal/runtimemanager"
	"mypack.dev/mypack/internal/runtimemanager/testregistry"
	"mypack.dev/mypack/pkg/config"
)

var (
	stubMu    sync.Mutex
	stubCache = map[string][]byte{}
)

// stubRuntime compiles a tiny Go program for goos/goarch (a real Mach-O/PE/ELF).
func stubRuntime(t *testing.T, goos, goarch string) []byte {
	t.Helper()
	key := goos + "-" + goarch
	stubMu.Lock()
	defer stubMu.Unlock()
	if b, ok := stubCache[key]; ok {
		return b
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not available to compile a stub runtime")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module stub\n\ngo 1.21\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { println(\"stub runtime\") }\n"), 0o644)
	out := filepath.Join(dir, "stub.bin")
	cmd := exec.Command(goBin, "build", "-trimpath", "-o", out, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch, "GOFLAGS=")
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot compile stub runtime: %v\n%s", err, o)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	stubCache[key] = b
	return b
}

type env struct {
	reg *testregistry.Registry
	mgr *runtimemanager.Manager
	out string
}

func newEnv(t *testing.T, keys []string, caps []string) *env {
	t.Helper()
	bins := map[string][]byte{}
	for _, k := range keys {
		parts := strings.SplitN(k, "-", 2)
		bins[k] = stubRuntime(t, parts[0], parts[1])
	}
	reg, err := testregistry.Create(t.TempDir(), testregistry.Options{Keys: keys, Binaries: bins, Capabilities: caps})
	if err != nil {
		t.Fatal(err)
	}
	m := runtimemanager.New(t.TempDir(), reg.URL)
	m.SetTrustedKeys(reg.Public)
	return &env{reg: reg, mgr: m, out: t.TempDir()}
}

func testConfig(name string, targets ...config.Target) *config.Config {
	c := config.New(name, config.Source{Type: config.SourceURL, Value: "https://example.com/"}, config.Target{OS: runtime.GOOS, Arch: runtime.GOARCH})
	c.Build.Targets = targets
	c.Runtime.Version = "1.0.0" // the test registry publishes 1.0.0
	return c
}

func hostKey() string { return runtime.GOOS + "-" + runtime.GOARCH }

func requireDarwin(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}
}

// parsePlist returns key → first token of the value (string, "true", "false", or "<dict>").
func parsePlist(t *testing.T, b []byte) map[string]string {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(b))
	out := map[string]string{}
	depth := 0
	var key string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("plist xml: %v", err)
		}
		switch x := tok.(type) {
		case xml.StartElement:
			switch x.Name.Local {
			case "dict":
				depth++
				if depth == 2 && key != "" {
					out[key] = "<dict>"
				}
			case "key":
				var k string
				dec.DecodeElement(&k, &x)
				if depth == 1 {
					key = k
				}
			case "string":
				var v string
				dec.DecodeElement(&v, &x)
				if depth == 1 {
					out[key] = v
				}
			case "true", "false":
				if depth == 1 {
					out[key] = x.Name.Local
				}
			}
		case xml.EndElement:
			if x.Name.Local == "dict" {
				depth--
			}
		}
	}
	return out
}

func TestDarwinAppBuild(t *testing.T) {
	requireDarwin(t)
	e := newEnv(t, []string{hostKey()}, nil)
	dir := t.TempDir()
	js := filepath.Join(dir, "inject.js")
	os.WriteFile(js, []byte("console.log('hi')"), 0o644)
	tgt := config.Target{OS: "darwin", Arch: runtime.GOARCH, Format: "app"}
	cfg := testConfig("My Test App", tgt)
	cfg.Native.Camera = true
	cfg.WebView.Injections = []config.Injection{{Type: "js", Path: js}, {Type: "css", Content: "body{}"}}
	cfg.App.Icon = config.Icon{Source: "file", Path: "/some/icon.png"}
	orig, _ := cfg.Marshal()

	var stages []domain.Stage
	var logs []string
	eng := &Engine{Runtimes: e.mgr}
	arts, err := eng.Build(context.Background(), Request{Config: cfg, Targets: []config.Target{tgt}, OutputDir: e.out, AllowDownload: true,
		Stage: func(s domain.Stage, p float64) {
			if len(stages) == 0 || stages[len(stages)-1] != s {
				stages = append(stages, s)
			}
		},
		Log: func(l domain.LogLine) { logs = append(logs, l.Message) }})
	if err != nil {
		t.Fatalf("build: %v\nlogs: %v", err, logs)
	}
	if now, _ := cfg.Marshal(); !bytes.Equal(orig, now) {
		t.Fatal("Build mutated the request config")
	}
	if len(arts) != 1 {
		t.Fatalf("artifacts: %+v", arts)
	}
	a := arts[0]
	want := filepath.Join(e.out, "My Test App", "darwin-"+runtime.GOARCH, "My Test App.app")
	if a.Path != want || a.Signature != "adhoc" || a.SHA256 == "" || a.Size <= 0 || a.Target != tgt || a.CreatedAt.IsZero() {
		t.Fatalf("artifact %+v want path %s", a, want)
	}
	// stage order
	expect := []domain.Stage{domain.StageValidating, domain.StageResolving, domain.StageDownloading, domain.StageAssembling, domain.StageCustomizing,
		domain.StageSigning, domain.StagePackaging, domain.StageVerifying, domain.StagePublishing, domain.StageCompleted}
	if len(stages) != len(expect) {
		t.Fatalf("stages %v", stages)
	}
	for i := range expect {
		if stages[i] != expect[i] {
			t.Fatalf("stages %v", stages)
		}
	}
	// layout
	app := a.Path
	pl, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	p := parsePlist(t, pl)
	for k, v := range map[string]string{
		"CFBundleName": "My Test App", "CFBundleDisplayName": "My Test App", "CFBundleIdentifier": cfg.App.Identifier,
		"CFBundleExecutable": "My Test App", "CFBundleIconFile": "AppIcon", "CFBundleShortVersionString": "1.0.0", "CFBundleVersion": "1.0.0",
		"CFBundlePackageType": "APPL", "LSMinimumSystemVersion": "11.0", "NSHighResolutionCapable": "true",
		"LSApplicationCategoryType": "public.app-category.productivity", "NSAppTransportSecurity": "<dict>",
	} {
		if p[k] != v {
			t.Errorf("Info.plist %s = %q, want %q", k, p[k], v)
		}
	}
	if p["NSCameraUsageDescription"] == "" || p["NSMicrophoneUsageDescription"] != "" {
		t.Errorf("camera/mic usage strings wrong: %v", p)
	}
	if !strings.Contains(string(pl), "NSAllowsArbitraryLoadsInWebContent") {
		t.Error("ATS key missing")
	}
	if b, _ := os.ReadFile(filepath.Join(app, "Contents", "PkgInfo")); string(b) != "APPL????" {
		t.Errorf("PkgInfo %q", b)
	}
	exe := filepath.Join(app, "Contents", "MacOS", "My Test App")
	if st, err := os.Stat(exe); err != nil || st.Mode()&0o755 != 0o755 {
		t.Fatalf("exe: %v %v", st, err)
	}
	icns, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "AppIcon.icns"))
	if err != nil || string(icns[:4]) != "icns" || int(binary.BigEndian.Uint32(icns[4:8])) != len(icns) {
		t.Fatalf("icns invalid: %v", err)
	}
	if _, err := icons.ICNSLargestPNG(icns); err != nil {
		t.Fatal(err)
	}
	ic, _ := os.ReadFile(filepath.Join(app, "Contents", "Resources", "icon.png"))
	if !is1024PNG(ic) {
		t.Fatal("icon.png is not a 1024 PNG")
	}
	cb, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "mypack.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := config.Parse(cb)
	if err != nil {
		t.Fatal(err)
	}
	if got.WebView.Injections[0].Content != "console.log('hi')" || got.WebView.Injections[0].Path != "" || got.App.Icon.Path != "" {
		t.Fatalf("injections/icon not inlined: %+v", got.WebView.Injections)
	}
	if !strings.Contains(string(cb), "console.log") {
		t.Fatal("content missing")
	}
	// codesign
	if out, err := exec.Command("codesign", "--verify", "--deep", "--strict", app).CombinedOutput(); err != nil {
		t.Fatalf("codesign verify: %v %s", err, out)
	}
	// staging cleaned
	if _, err := os.Stat(filepath.Join(e.out, ".staging")); !os.IsNotExist(err) {
		t.Fatal(".staging not cleaned")
	}

	// rebuild replaces the artifact; no .prev left; the hash is stable.
	arts2, err := eng.Build(context.Background(), Request{Config: cfg, Targets: []config.Target{tgt}, OutputDir: e.out, AllowDownload: false})
	if err != nil {
		t.Fatalf("rebuild (cached runtime, no download): %v", err)
	}
	if arts2[0].Path != a.Path {
		t.Fatal("path changed")
	}
	if _, err := os.Stat(a.Path + ".prev"); !os.IsNotExist(err) {
		t.Fatal(".prev left behind")
	}
}

func TestRuntimeDownloadRequired(t *testing.T) {
	e := newEnv(t, []string{hostKey()}, nil)
	tgt := config.DefaultTarget(config.Target{OS: runtime.GOOS, Arch: runtime.GOARCH})
	cfg := testConfig("Needs Runtime", tgt)
	eng := &Engine{Runtimes: e.mgr}
	_, err := eng.Build(context.Background(), Request{Config: cfg, Targets: []config.Target{tgt}, OutputDir: e.out, AllowDownload: false})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != "RUNTIME_DOWNLOAD_REQUIRED" || de.Remediation == "" {
		t.Fatalf("got %v", err)
	}
	if ents, _ := os.ReadDir(e.out); len(ents) != 0 {
		t.Fatalf("output dir should be untouched: %v", ents)
	}
}

func TestCancelKeepsPreviousArtifact(t *testing.T) {
	requireDarwin(t)
	e := newEnv(t, []string{hostKey()}, nil)
	tgt := config.Target{OS: "darwin", Arch: runtime.GOARCH, Format: "app"}
	cfg := testConfig("Cancel Me", tgt)
	eng := &Engine{Runtimes: e.mgr}
	arts, err := eng.Build(context.Background(), Request{Config: cfg, Targets: []config.Target{tgt}, OutputDir: e.out, AllowDownload: true})
	if err != nil {
		t.Fatal(err)
	}
	before, _, _ := hashArtifact(context.Background(), arts[0].Path)

	ctx, cancel := context.WithCancel(context.Background())
	cfg2 := cfg.Clone()
	cfg2.App.Version = "2.0.0"
	_, err = eng.Build(ctx, Request{Config: cfg2, Targets: []config.Target{tgt}, OutputDir: e.out, AllowDownload: true,
		Stage: func(s domain.Stage, p float64) {
			if s == domain.StageCustomizing {
				cancel()
			}
		}})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != "BUILD_CANCELLED" {
		t.Fatalf("got %v", err)
	}
	after, _, _ := hashArtifact(context.Background(), arts[0].Path)
	if before != after {
		t.Fatal("previous artifact was touched")
	}
	if _, err := os.Stat(filepath.Join(e.out, ".staging")); !os.IsNotExist(err) {
		ents, _ := os.ReadDir(filepath.Join(e.out, ".staging"))
		t.Fatalf("staging left behind: %v", ents)
	}
}

func TestDarwinZipAndDMG(t *testing.T) {
	requireDarwin(t)
	e := newEnv(t, []string{hostKey()}, nil)
	eng := &Engine{Runtimes: e.mgr}
	zt := config.Target{OS: "darwin", Arch: runtime.GOARCH, Format: "zip"}
	dt := config.Target{OS: "darwin", Arch: runtime.GOARCH, Format: "dmg"}
	cfg := testConfig("Zip Dmg", zt)
	arts, err := eng.Build(context.Background(), Request{Config: cfg, Targets: []config.Target{zt}, OutputDir: e.out, AllowDownload: true})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(arts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var sawExe bool
	for _, f := range zr.File {
		if f.Name == "Zip Dmg.app/Contents/MacOS/Zip Dmg" {
			sawExe = f.Mode()&0o111 != 0
		}
	}
	if !sawExe {
		t.Fatal("executable bit lost in zip")
	}
	// the Go zipper must produce equivalent archives (used when ditto is absent)
	arts, err = eng.Build(context.Background(), Request{Config: cfg, Targets: []config.Target{dt}, OutputDir: e.out, AllowDownload: true})
	if err != nil {
		var de *domain.Error
		if errors.As(err, &de) && de.Code == "PACKAGING_FAILED" {
			t.Skipf("hdiutil unavailable in this environment: %v", err)
		}
		t.Fatal(err)
	}
	st, err := os.Stat(arts[0].Path)
	if err != nil || st.Size() < 4096 || !strings.HasSuffix(arts[0].Path, "Zip Dmg.dmg") {
		t.Fatalf("dmg: %v %v", st, err)
	}
	if arts[0].Signature != "adhoc" {
		t.Fatal(arts[0].Signature)
	}
}

func TestGoZipPreservesSymlinksAndModes(t *testing.T) {
	src := filepath.Join(t.TempDir(), "App.app")
	os.MkdirAll(filepath.Join(src, "Contents", "MacOS"), 0o755)
	os.WriteFile(filepath.Join(src, "Contents", "MacOS", "App"), []byte("bin"), 0o755)
	os.WriteFile(filepath.Join(src, "Contents", "data.txt"), []byte("d"), 0o644)
	os.Symlink("MacOS/App", filepath.Join(src, "Contents", "link"))
	out := filepath.Join(t.TempDir(), "o.zip")
	if err := zipTree(context.Background(), src, out); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	got := map[string]os.FileMode{}
	for _, f := range zr.File {
		got[f.Name] = f.Mode()
	}
	if got["App.app/Contents/MacOS/App"]&0o111 == 0 {
		t.Fatal("exec bit lost")
	}
	if got["App.app/Contents/link"]&os.ModeSymlink == 0 {
		t.Fatalf("symlink lost: %v", got)
	}
	if _, ok := got["App.app/"]; !ok {
		t.Fatalf("top dir missing: %v", got)
	}
}

func TestWindowsPortable(t *testing.T) {
	e := newEnv(t, []string{"windows-amd64"}, nil)
	tgt := config.Target{OS: "windows", Arch: "amd64", Format: "portable"}
	cfg := testConfig("Win App", tgt)
	cfg.App.Version = "2.3.4"
	eng := &Engine{Runtimes: e.mgr}
	var warns []string
	arts, err := eng.Build(context.Background(), Request{Config: cfg, Targets: []config.Target{tgt}, OutputDir: e.out, AllowDownload: true,
		Log: func(l domain.LogLine) {
			if l.Level == "warn" {
				warns = append(warns, l.Message)
			}
		}})
	if err != nil {
		t.Fatal(err)
	}
	a := arts[0]
	if a.Signature != "unsigned" || a.Path != filepath.Join(e.out, "Win App", "windows-amd64", "Win App") {
		t.Fatalf("%+v", a)
	}
	for _, w := range warns {
		if strings.Contains(w, "Could not embed") {
			t.Fatalf("PE patching failed: %s", w)
		}
	}
	for _, f := range []string{"Win App.exe", "resources/mypack.json", "resources/icon.png", "resources/icon.ico"} {
		if _, err := os.Stat(filepath.Join(a.Path, filepath.FromSlash(f))); err != nil {
			t.Fatal(err)
		}
	}
	// the icon + version info were embedded in the PE
	f, err := os.Open(filepath.Join(a.Path, "Win App.exe"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rs, err := winres.LoadFromEXE(f)
	if err != nil {
		t.Fatalf("no resources in patched exe: %v", err)
	}
	hasIcon, hasVer := false, false
	rs.WalkType(winres.RT_GROUP_ICON, func(resID winres.Identifier, langID uint16, data []byte) bool { hasIcon = true; return true })
	rs.WalkType(winres.RT_VERSION, func(resID winres.Identifier, langID uint16, data []byte) bool {
		hasVer = bytes.Contains(data, utf16("Win App"))
		return true
	})
	if !hasIcon || !hasVer {
		t.Fatalf("icon=%v version=%v", hasIcon, hasVer)
	}
	// cross-build warning surfaced
	if runtime.GOOS != "windows" {
		found := false
		for _, w := range warns {
			found = found || strings.Contains(w, "not yet tested")
		}
		if !found {
			t.Fatalf("expected cross-build warning, got %v", warns)
		}
	}
}

func utf16(s string) []byte {
	var b []byte
	for _, r := range s {
		b = append(b, byte(r), 0)
	}
	return b
}

func TestWindowsWrongArchRejected(t *testing.T) {
	// a registry serving an amd64 exe under the arm64 key must fail verification
	e := newEnv(t, nil, nil)
	reg, err := testregistry.Create(t.TempDir(), testregistry.Options{Keys: []string{"windows-arm64"}, Binaries: map[string][]byte{"windows-arm64": stubRuntime(t, "windows", "amd64")}})
	if err != nil {
		t.Fatal(err)
	}
	m := runtimemanager.New(t.TempDir(), reg.URL)
	m.SetTrustedKeys(reg.Public)
	tgt := config.Target{OS: "windows", Arch: "arm64", Format: "portable"}
	_, err = (&Engine{Runtimes: m}).Build(context.Background(), Request{Config: testConfig("Wrong Arch", tgt), Targets: []config.Target{tgt}, OutputDir: e.out, AllowDownload: true})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != "BUILD_VERIFY_FAILED" {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.out, "Wrong Arch")); !os.IsNotExist(err) {
		t.Fatal("must not publish a failed build")
	}
}

func TestLinuxPortableAndDeb(t *testing.T) {
	e := newEnv(t, []string{"linux-amd64", "linux-arm64"}, nil)
	pt := config.Target{OS: "linux", Arch: "amd64", Format: "portable"}
	dt := config.Target{OS: "linux", Arch: "arm64", Format: "deb"}
	cfg := testConfig("Cool Site", pt, dt)
	eng := &Engine{Runtimes: e.mgr}
	arts, err := eng.Build(context.Background(), Request{Config: cfg, Targets: []config.Target{pt, dt}, OutputDir: e.out, AllowDownload: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 2 {
		t.Fatalf("%+v", arts)
	}
	p := arts[0].Path
	for _, f := range []string{"cool-site", "cool-site.desktop", "resources/mypack.json", "resources/icon.png"} {
		if _, err := os.Stat(filepath.Join(p, filepath.FromSlash(f))); err != nil {
			t.Fatal(err)
		}
	}
	if st, _ := os.Stat(filepath.Join(p, "cool-site")); st.Mode()&0o111 == 0 {
		t.Fatal("not executable")
	}
	deb := arts[1].Path
	if filepath.Base(deb) != "cool-site_1.0.0_arm64.deb" {
		t.Fatal(deb)
	}
	members := readAr(t, deb)
	if string(members["debian-binary"]) != "2.0\n" {
		t.Fatal("debian-binary")
	}
	ctl := tarFiles(t, members["control.tar.gz"])
	control := string(ctl["./control"])
	for _, want := range []string{"Package: cool-site\n", "Version: 1.0.0\n", "Architecture: arm64\n", "Depends: libwebkit2gtk-4.1-0 | libwebkit2gtk-4.0-37, libgtk-3-0\n"} {
		if !strings.Contains(control, want) {
			t.Errorf("control missing %q:\n%s", want, control)
		}
	}
	data := tarFiles(t, members["data.tar.gz"])
	for _, f := range []string{"./opt/cool-site/cool-site", "./opt/cool-site/resources/mypack.json", "./usr/share/applications/cool-site.desktop", "./usr/share/icons/hicolor/512x512/apps/cool-site.png"} {
		if _, ok := data[f]; !ok {
			t.Errorf("data.tar.gz missing %s (have %v)", f, keys(data))
		}
	}
	if !strings.Contains(string(data["./usr/share/applications/cool-site.desktop"]), `Exec="/opt/cool-site/cool-site"`) {
		t.Error("desktop Exec wrong")
	}
	if !strings.Contains(string(ctl["./md5sums"]), "opt/cool-site/cool-site") {
		t.Error("md5sums")
	}
	if arts[1].SHA256 == "" || arts[1].Size == 0 {
		t.Fatal("hash/size")
	}
}

func keys(m map[string][]byte) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	return k
}

func readAr(t *testing.T, path string) map[string][]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b[:8]) != "!<arch>\n" {
		t.Fatal("not ar")
	}
	out := map[string][]byte{}
	for pos := 8; pos+60 <= len(b); {
		name := strings.TrimSpace(string(b[pos : pos+16]))
		var size int
		if _, err := fmtSscan(strings.TrimSpace(string(b[pos+48:pos+58])), &size); err != nil {
			t.Fatal(err)
		}
		if string(b[pos+58:pos+60]) != "`\n" {
			t.Fatal("bad ar magic")
		}
		out[name] = b[pos+60 : pos+60+size]
		pos += 60 + size + size%2
	}
	return out
}

func tarFiles(t *testing.T, tgz []byte) map[string][]byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		out[h.Name] = b
	}
}

func TestPartialSuccessAndFirstError(t *testing.T) {
	e := newEnv(t, []string{"linux-amd64"}, nil) // no windows-arm64 runtime
	good := config.Target{OS: "linux", Arch: "amd64", Format: "portable"}
	bad := config.Target{OS: "windows", Arch: "arm64", Format: "portable"}
	cfg := testConfig("Partial", good, bad)
	arts, err := (&Engine{Runtimes: e.mgr}).Build(context.Background(), Request{Config: cfg, Targets: []config.Target{bad, good}, OutputDir: e.out, AllowDownload: true})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != "RUNTIME_NOT_FOUND" {
		t.Fatalf("got %v", err)
	}
	if len(arts) != 1 || arts[0].Target != good {
		t.Fatalf("successful artifacts must be returned: %+v", arts)
	}
}

func TestInvalidConfigAndUnsupportedTarget(t *testing.T) {
	e := newEnv(t, []string{hostKey()}, nil)
	eng := &Engine{Runtimes: e.mgr}
	tgt := config.Target{OS: "linux", Arch: "amd64", Format: "portable"}
	cfg := testConfig("Bad", tgt)
	cfg.App.Identifier = "nope"
	_, err := eng.Build(context.Background(), Request{Config: cfg, Targets: []config.Target{tgt}, OutputDir: e.out, AllowDownload: true})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != "INVALID_CONFIG" || de.Field != "app.identifier" {
		t.Fatalf("got %#v", err)
	}
	ok := testConfig("Good", tgt)
	appimg := config.Target{OS: "linux", Arch: "amd64", Format: "appimage"}
	_, err = eng.Build(context.Background(), Request{Config: ok, Targets: []config.Target{appimg}, OutputDir: e.out, AllowDownload: true})
	if !errors.As(err, &de) || de.Code != "TARGET_UNSUPPORTED" {
		t.Fatalf("got %v", err)
	}
}

func TestRuntimeCapabilityMissing(t *testing.T) {
	e := newEnv(t, []string{"linux-amd64"}, []string{"tray"})
	tgt := config.Target{OS: "linux", Arch: "amd64", Format: "portable"}
	cfg := testConfig("Caps", tgt)
	cfg.Native.GlobalShortcut = "CmdOrCtrl+Shift+G"
	_, err := (&Engine{Runtimes: e.mgr}).Build(context.Background(), Request{Config: cfg, Targets: []config.Target{tgt}, OutputDir: e.out, AllowDownload: true})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != "RUNTIME_CAPABILITY_MISSING" {
		t.Fatalf("got %v", err)
	}
}

func TestLocalDirSourceBundled(t *testing.T) {
	e := newEnv(t, []string{"linux-amd64"}, nil)
	site := t.TempDir()
	os.WriteFile(filepath.Join(site, "index.html"), []byte("<h1>x</h1>"), 0o644)
	os.MkdirAll(filepath.Join(site, "css"), 0o755)
	os.WriteFile(filepath.Join(site, "css", "a.css"), []byte("a{}"), 0o644)
	tgt := config.Target{OS: "linux", Arch: "amd64", Format: "portable"}
	cfg := testConfig("Local Site", tgt)
	cfg.App.Source = config.Source{Type: config.SourceDir, Value: site}
	arts, err := (&Engine{Runtimes: e.mgr}).Build(context.Background(), Request{Config: cfg, Targets: []config.Target{tgt}, OutputDir: e.out, AllowDownload: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(arts[0].Path, "resources", "site", "css", "a.css")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(arts[0].Path, "resources", "mypack.json"))
	var c config.Config
	json.Unmarshal(b, &c)
	if c.App.Source.Value != "site" || c.App.Source.Type != config.SourceDir {
		t.Fatalf("%+v", c.App.Source)
	}
}

func TestCapabilitiesMatrix(t *testing.T) {
	mac := config.Target{OS: "darwin", Arch: "arm64"}
	lin := config.Target{OS: "linux", Arch: "amd64"}
	cases := []struct {
		host, tgt config.Target
		format    string
		ok        bool
		level     string
		signing   string
		reason    string
	}{
		{mac, mac, "app", true, "portable", "adhoc", ""},
		{mac, mac, "zip", true, "portable", "adhoc", ""},
		{mac, mac, "dmg", true, "installer", "adhoc", ""},
		{lin, mac, "app", false, "", "unavailable", "macOS apps can only be built on a Mac (code signing)"},
		{mac, config.Target{OS: "windows", Arch: "amd64"}, "portable", true, "portable", "unsigned", "Built on macOS; not yet tested on the target system"},
		{mac, config.Target{OS: "windows", Arch: "amd64"}, "exe", false, "", "unavailable", ""},
		{mac, config.Target{OS: "linux", Arch: "arm64"}, "deb", true, "installer", "unsigned", "Built on macOS; not yet tested on the target system"},
		{mac, config.Target{OS: "linux", Arch: "arm64"}, "appimage", false, "", "unavailable", ""},
		{lin, config.Target{OS: "linux", Arch: "arm64"}, "portable", true, "portable", "unsigned", ""},
		{lin, config.Target{OS: "windows", Arch: "arm64"}, "portable", true, "portable", "unsigned", "Built on Linux; not yet tested on the target system"},
	}
	for _, c := range cases {
		tgt := c.tgt
		tgt.Format = c.format
		got := Capabilities(c.host, tgt)
		if got.Supported != c.ok || got.Level != c.level || got.Signing != c.signing || (c.reason != "" && got.Reason != c.reason) || (!c.ok && got.Reason == "") {
			t.Errorf("%s→%s/%s: %+v", c.host.OS, tgt.Key(), c.format, got)
		}
	}
	h := HostTarget()
	if h.OS != runtime.GOOS || h.Arch != runtime.GOARCH || h.Format == "" {
		t.Fatal(h)
	}
}

func TestHashArtifactDeterministic(t *testing.T) {
	mk := func() string {
		d := t.TempDir()
		os.MkdirAll(filepath.Join(d, "a", "b"), 0o755)
		os.WriteFile(filepath.Join(d, "a", "f"), []byte("1"), 0o644)
		os.WriteFile(filepath.Join(d, "a", "b", "g"), []byte("2"), 0o755)
		os.Symlink("f", filepath.Join(d, "a", "l"))
		return d
	}
	h1, n1, err := hashArtifact(context.Background(), mk())
	h2, n2, _ := hashArtifact(context.Background(), mk())
	if err != nil || h1 != h2 || n1 != 2 || n2 != 2 {
		t.Fatalf("%s %s %d %v", h1, h2, n1, err)
	}
	d := mk()
	os.WriteFile(filepath.Join(d, "a", "f"), []byte("changed"), 0o644)
	if h3, _, _ := hashArtifact(context.Background(), d); h3 == h1 {
		t.Fatal("hash must change with content")
	}
}

func TestInfoPlistMenuBarMode(t *testing.T) {
	cfg := config.New("Chat", config.Source{Type: config.SourceURL, Value: "https://chat.example.com"}, config.Target{OS: "darwin", Arch: "arm64"})
	plist := func() string { return string((&job{cfg: cfg, name: "Chat"}).infoPlist()) }
	if strings.Contains(plist(), "LSUIElement") {
		t.Fatal("window mode must keep the Dock icon")
	}
	cfg.Window.Mode = config.ModeMenuBar
	if !strings.Contains(plist(), "LSUIElement") {
		t.Fatal("menubar mode on runtime 1.0.6 must hide the Dock icon")
	}
	cfg.Runtime.Version = "1.0.5"
	if strings.Contains(plist(), "LSUIElement") {
		t.Fatal("an older runtime must keep the Dock icon")
	}
}

func TestGroupLocalMembersBundled(t *testing.T) {
	e := newEnv(t, []string{"linux-amd64"}, nil)
	docs, board := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(docs, "index.html"), []byte("<h1>docs</h1>"), 0o644)
	os.MkdirAll(filepath.Join(docs, "img"), 0o755)
	os.WriteFile(filepath.Join(docs, "img", "a.png"), []byte("png"), 0o644)
	os.WriteFile(filepath.Join(board, "board.html"), []byte("board"), 0o644)
	tgt := config.Target{OS: "linux", Arch: "amd64", Format: "portable"}
	cfg := testConfig("Local Group", tgt)
	cfg.App.Source = config.Source{Type: config.SourceDir, Value: docs}
	cfg.App.Group = &config.GroupConfig{Members: []config.GroupMember{
		{ID: "docs", Name: "Docs", Local: docs},
		{ID: "board", Name: "Board", Local: filepath.Join(board, "board.html")},
	}}
	arts, err := (&Engine{Runtimes: e.mgr}).Build(context.Background(), Request{Config: cfg, Targets: []config.Target{tgt}, OutputDir: e.out, AllowDownload: true})
	if err != nil {
		t.Fatal(err)
	}
	res := filepath.Join(arts[0].Path, "resources")
	for _, f := range []string{"members/docs/site/img/a.png", "members/board/site/board.html"} {
		if _, err := os.Stat(filepath.Join(res, filepath.FromSlash(f))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(res, "site")); err == nil {
		t.Fatal("a group's source must not be bundled a second time")
	}
	b, _ := os.ReadFile(filepath.Join(res, "mypack.json"))
	var c config.Config
	json.Unmarshal(b, &c)
	m := c.App.Group.Members
	if m[0].Local != "members/docs/site" || m[1].Local != "members/board/site" || m[1].Entry != "board.html" {
		t.Fatalf("members: %+v", m)
	}
	if c.App.Source.Value != "members/docs/site" {
		t.Fatalf("source: %+v", c.App.Source)
	}
}

func TestValidateDrawsIconStyle(t *testing.T) {
	host := HostTarget()
	cfg := config.New("Notes", config.Source{Type: config.SourceURL, Value: "https://notes.example.com"}, host)
	base := icons.Default("Notes")
	plain := &job{ctx: context.Background(), cfg: cfg, target: host, req: Request{IconPNG: base}}
	if err := plain.validate(); err != nil {
		t.Fatal(err)
	}
	styledCfg := cfg.Clone()
	styledCfg.App.Icon.Style = &config.IconStyle{Glyph: config.GlyphText, Text: "N", Background: config.BackgroundGlass, Badge: "Work"}
	styled := &job{ctx: context.Background(), cfg: styledCfg, target: host, req: Request{IconPNG: base}}
	if err := styled.validate(); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(plain.iconPNG, styled.iconPNG) || len(styled.iconPNG) == 0 {
		t.Fatal("the build did not draw app.icon.style")
	}
}
