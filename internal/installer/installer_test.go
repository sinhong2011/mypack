package installer

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/icons"
)

func setHome(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("APPDATA", "")
	return h
}

func setGOOS(t *testing.T, g string) {
	t.Helper()
	old := goos
	goos = g
	t.Cleanup(func() { goos = old })
}

func capture(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	oldRun, oldStart := runCmd, startCmd
	runCmd = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}
	startCmd = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}
	t.Cleanup(func() { runCmd, startCmd = oldRun, oldStart })
	return &calls
}

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>Fake &amp; Co</string>
	<key>CFBundleIdentifier</key><string>app.mypack.fake</string>
	<key>CFBundleShortVersionString</key><string>2.1.0</string>
	<key>CFBundleIconFile</key><string>AppIcon</string>
	<key>LSUIElement</key><true/>
	<key>Count</key><integer>3</integer>
	<key>Arr</key><array><string>a</string><dict><key>x</key><string>y</string></dict></array>
</dict>
</plist>`

const managedJSON = `{"schemaVersion":1,"app":{"name":"Fake & Co","identifier":"app.mypack.fake","profileId":"app.mypack.fake","version":"2.1.0","source":{"type":"url","value":"https://example.com/"},"icon":{"source":"auto"}},"runtime":{"version":"1.0.0"},"window":{"width":1200,"height":780},"webview":{"navigation":{}},"build":{"targets":[{"os":"darwin","arch":"arm64","format":"app"}]}}`

// fakeApp creates a minimal .app bundle.
func fakeApp(t *testing.T, dir, name string, managed bool) string {
	t.Helper()
	app := filepath.Join(dir, name+".app")
	must(t, os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755))
	must(t, os.MkdirAll(filepath.Join(app, "Contents", "Resources"), 0o755))
	must(t, os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(infoPlist), 0o644))
	must(t, os.WriteFile(filepath.Join(app, "Contents", "MacOS", name), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	icns, err := icons.ICNS(icons.Default("Fake"))
	must(t, err)
	must(t, os.WriteFile(filepath.Join(app, "Contents", "Resources", "AppIcon.icns"), icns, 0o644))
	must(t, os.Symlink("MacOS/"+name, filepath.Join(app, "Contents", "link")))
	if managed {
		must(t, os.WriteFile(filepath.Join(app, "Contents", "Resources", "mypack.json"), []byte(managedJSON), 0o644))
	}
	return app
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func code(err error) string {
	var de *domain.Error
	if errors.As(err, &de) {
		return de.Code
	}
	return ""
}

func TestInstallConflictReplaceAndFidelity(t *testing.T) {
	setHome(t)
	src := fakeApp(t, t.TempDir(), "Fake", true)
	inst := filepath.Join(t.TempDir(), "Applications") // does not exist yet
	p, err := Install(context.Background(), src, inst, false)
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join(inst, "Fake.app") {
		t.Fatal(p)
	}
	if st, err := os.Stat(filepath.Join(p, "Contents", "MacOS", "Fake")); err != nil || (runtime.GOOS != "windows" && st.Mode()&0o111 == 0) {
		t.Fatalf("exec bit lost: %v %v", st, err)
	}
	if l, err := os.Readlink(filepath.Join(p, "Contents", "link")); err != nil || l != "MacOS/Fake" {
		t.Fatalf("symlink lost: %v %v", l, err)
	}
	if _, err := Install(context.Background(), src, inst, false); code(err) != "INSTALL_CONFLICT" {
		t.Fatalf("want conflict, got %v", err)
	}
	// replace works and leaves no backup
	os.WriteFile(filepath.Join(src, "Contents", "marker"), []byte("v2"), 0o644)
	if _, err := Install(context.Background(), src, inst, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p, "Contents", "marker")); err != nil {
		t.Fatal("not replaced")
	}
	ents, _ := os.ReadDir(inst)
	if len(ents) != 1 {
		t.Fatalf("stray files: %v", ents)
	}
}

func TestInstallReplaceRestoresOnFailure(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("needs unreadable files")
	}
	setHome(t)
	setGOOS(t, "linux") // exercise the portable Go copy path
	old := fakeApp(t, t.TempDir(), "Fake", true)
	inst := t.TempDir()
	if _, err := Install(context.Background(), old, inst, false); err != nil {
		t.Fatal(err)
	}
	bad := fakeApp(t, t.TempDir(), "Fake", true)
	secret := filepath.Join(bad, "Contents", "Resources", "unreadable")
	os.WriteFile(secret, []byte("x"), 0o000)
	defer os.Chmod(secret, 0o600)
	_, err := Install(context.Background(), bad, inst, true)
	if code(err) != "INSTALL_FAILED" {
		t.Fatalf("want INSTALL_FAILED, got %v", err)
	}
	// the old app is back and complete
	if _, err := os.Stat(filepath.Join(inst, "Fake.app", "Contents", "Info.plist")); err != nil {
		t.Fatalf("old app not restored: %v", err)
	}
	ents, _ := os.ReadDir(inst)
	if len(ents) != 1 {
		t.Fatalf("stray backup/partial left: %v", ents)
	}
}

func TestInstallRejectsFilesAndMissing(t *testing.T) {
	f := filepath.Join(t.TempDir(), "App.zip")
	os.WriteFile(f, []byte("z"), 0o644)
	if _, err := Install(context.Background(), f, t.TempDir(), false); code(err) != "INSTALL_UNSUPPORTED" {
		t.Fatalf("%v", err)
	}
	if _, err := Install(context.Background(), "/nonexistent/App.app", t.TempDir(), false); code(err) != "NOT_FOUND" {
		t.Fatalf("%v", err)
	}
}

func TestReplaceBackupRestoreDiscard(t *testing.T) {
	setHome(t)
	capture(t) // IsRunning → runCmd returns nil ("running") — override below
	oldRun := runCmd
	runCmd = func(name string, args ...string) error { return errors.New("not running") }
	defer func() { runCmd = oldRun }()
	setGOOS(t, "darwin")

	inst := t.TempDir()
	v1 := fakeApp(t, t.TempDir(), "Fake", true)
	p, err := Install(context.Background(), v1, inst, false)
	if err != nil {
		t.Fatal(err)
	}
	v2 := fakeApp(t, t.TempDir(), "Fake", true)
	os.WriteFile(filepath.Join(v2, "Contents", "v2"), []byte("2"), 0o644)
	backup, err := Replace(context.Background(), p, v2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p, "Contents", "v2")); err != nil {
		t.Fatal("new version not in place")
	}
	if _, err := os.Stat(filepath.Join(backup, "Contents", "Info.plist")); err != nil {
		t.Fatal("backup missing")
	}
	// roll back
	if err := RestoreBackup(p, backup); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p, "Contents", "v2")); !os.IsNotExist(err) {
		t.Fatal("rollback failed")
	}
	// replace again, then discard
	backup, err = Replace(context.Background(), p, v2)
	if err != nil {
		t.Fatal(err)
	}
	if err := DiscardBackup(backup); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("backup not discarded")
	}
	if err := DiscardBackup(p); err == nil {
		t.Fatal("DiscardBackup must refuse non-backup paths")
	}
	ents, _ := os.ReadDir(inst)
	if len(ents) != 1 {
		t.Fatalf("%v", ents)
	}
}

func TestReplaceRefusesRunningApp(t *testing.T) {
	setHome(t)
	setGOOS(t, "darwin")
	calls := capture(t) // runCmd → nil => pgrep found a process
	inst := t.TempDir()
	p, err := Install(context.Background(), fakeApp(t, t.TempDir(), "Fake", false), inst, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Replace(context.Background(), p, fakeApp(t, t.TempDir(), "Fake", false))
	if code(err) != "APP_RUNNING" {
		t.Fatalf("got %v", err)
	}
	found := false
	for _, c := range *calls {
		if c[0] == "pgrep" && c[1] == "-f" && strings.Contains(filepath.ToSlash(c[2]), "Contents/MacOS/") {
			found = true
		}
	}
	if !found {
		t.Fatalf("pgrep not used: %v", *calls)
	}
}

func TestLaunchRevealOpenURLCommands(t *testing.T) {
	d := t.TempDir()
	app := fakeApp(t, d, "Fake", false)
	for _, tc := range []struct {
		goos   string
		launch string
		reveal string
		url    string
	}{
		{"darwin", "open -n " + app, "open -R " + app, "open https://x.y/"},
		{"windows", "cmd /c start", "explorer /select," + app, "rundll32 url.dll,FileProtocolHandler https://x.y/"},
		{"linux", "", "xdg-open " + d, "xdg-open https://x.y/"},
	} {
		setGOOS(t, tc.goos)
		calls := capture(t)
		if tc.goos == "darwin" {
			if err := Launch(app); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join((*calls)[0], " "); got != tc.launch {
				t.Errorf("%s launch: %q", tc.goos, got)
			}
		}
		*calls = nil
		if err := Reveal(app); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join((*calls)[0], " "); got != tc.reveal {
			t.Errorf("%s reveal: %q want %q", tc.goos, got, tc.reveal)
		}
		*calls = nil
		if err := OpenURL("https://x.y/"); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join((*calls)[0], " "); got != tc.url {
			t.Errorf("%s url: %q", tc.goos, got)
		}
	}
	if err := OpenURL("javascript:alert(1)"); err == nil {
		t.Fatal("must refuse non-http")
	}
	if err := OpenURL("file:///etc/passwd"); err == nil {
		t.Fatal("must refuse file:")
	}
	if err := Launch(filepath.Join(d, "nope.app")); code(err) != "NOT_FOUND" {
		t.Fatal(err)
	}
}

func TestLaunchPortable(t *testing.T) {
	d := t.TempDir()
	w := filepath.Join(d, "Site")
	os.MkdirAll(filepath.Join(w, "resources"), 0o755)
	os.WriteFile(filepath.Join(w, "Site.exe"), []byte("MZ"), 0o755)
	os.WriteFile(filepath.Join(w, "other.exe"), []byte("MZ"), 0o755)
	setGOOS(t, "windows")
	calls := capture(t)
	if err := Launch(w); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join((*calls)[0], "|"); got != "cmd|/c|start||"+filepath.Join(w, "Site.exe") {
		t.Fatal(got)
	}
	// linux
	l := filepath.Join(d, "lin")
	os.MkdirAll(l, 0o755)
	os.WriteFile(filepath.Join(l, "lin"), []byte("x"), 0o755)
	os.WriteFile(filepath.Join(l, "lin.desktop"), []byte("x"), 0o644)
	if runtime.GOOS == "windows" {
		return // the Unix executable bit is not stored, so a Linux binary cannot be recognised
	}
	setGOOS(t, "linux")
	*calls = nil
	if err := Launch(l); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0][0] != filepath.Join(l, "lin") {
		t.Fatal((*calls)[0])
	}
}

func TestUninstallToTrashMac(t *testing.T) {
	h := setHome(t)
	setGOOS(t, "darwin")
	inst := filepath.Join(h, "Applications")
	p, err := Install(context.Background(), fakeApp(t, t.TempDir(), "Fake", false), inst, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("still installed")
	}
	if _, err := os.Stat(filepath.Join(h, ".Trash", "Fake.app", "Contents", "Info.plist")); err != nil {
		t.Fatalf("not in trash: %v", err)
	}
	// second time with the same name gets a unique trash name
	p, _ = Install(context.Background(), fakeApp(t, t.TempDir(), "Fake", false), inst, false)
	if err := Uninstall(p); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(filepath.Join(h, ".Trash"))
	if len(ents) != 2 {
		t.Fatalf("trash: %v", ents)
	}
	if err := Uninstall(p); code(err) != "NOT_FOUND" {
		t.Fatalf("%v", err)
	}
}

func TestUninstallRefusesDangerousPaths(t *testing.T) {
	h := setHome(t)
	for _, p := range []string{"/", h, filepath.Dir(h), DefaultInstallDir(), "/usr"} {
		if err := Uninstall(p); err == nil {
			t.Errorf("Uninstall(%q) must be refused", p)
		}
	}
}

func TestLinuxDesktopEntryLifecycle(t *testing.T) {
	setHome(t)
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	setGOOS(t, "linux")
	src := filepath.Join(t.TempDir(), "cool")
	os.MkdirAll(filepath.Join(src, "resources"), 0o755)
	os.WriteFile(filepath.Join(src, "cool"), []byte("x"), 0o755)
	os.WriteFile(filepath.Join(src, "cool.desktop"), []byte("[Desktop Entry]\nExec=__INSTALL_DIR__/cool\nIcon=__INSTALL_DIR__/resources/icon.png\n"), 0o644)
	inst := filepath.Join(xdg, "mypack-apps")
	if DefaultInstallDir() != inst {
		t.Fatalf("%s != %s", DefaultInstallDir(), inst)
	}
	p, err := Install(context.Background(), src, inst, false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(xdg, "applications", "cool.desktop"))
	if err != nil || !strings.Contains(string(b), "Exec="+p+"/cool") || strings.Contains(string(b), "__INSTALL_DIR__") {
		t.Fatalf("desktop entry: %q %v", b, err)
	}
	if err := Uninstall(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(xdg, "applications", "cool.desktop")); !os.IsNotExist(err) {
		t.Fatal("desktop entry not removed")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("not removed")
	}
}

func TestProfileDirAndClear(t *testing.T) {
	h := setHome(t)
	setGOOS(t, "darwin")
	if got := ProfileDir("app.mypack.x"); got != filepath.Join(h, "Library", "Application Support", "mypack-apps", "app.mypack.x") {
		t.Fatal(got)
	}
	setGOOS(t, "linux")
	t.Setenv("XDG_DATA_HOME", filepath.Join(h, "xdg"))
	if got := ProfileDir("a.b"); got != filepath.Join(h, "xdg", "mypack-apps", "a.b") {
		t.Fatal(got)
	}
	setGOOS(t, "windows")
	t.Setenv("APPDATA", filepath.Join(h, "roam"))
	if got := ProfileDir("a.b"); got != filepath.Join(h, "roam", "mypack-apps", "a.b") {
		t.Fatal(got)
	}
	// traversal in IDs never escapes
	if got := ProfileDir("../../etc"); !strings.HasPrefix(got, filepath.Join(h, "roam", "mypack-apps")+string(os.PathSeparator)) || strings.Contains(got, "..") {
		t.Fatal(got)
	}
	for _, bad := range []string{"", "../x", "a/b", ".."} {
		if err := ClearProfile(bad); err == nil {
			t.Errorf("ClearProfile(%q) must fail", bad)
		}
	}
	dir := ProfileDir("app.mypack.x")
	os.MkdirAll(filepath.Join(dir, "Cookies"), 0o755)
	if err := ClearProfile("app.mypack.x"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("profile not deleted")
	}
	if err := ClearProfile("app.mypack.x"); err != nil {
		t.Fatal("idempotent")
	}
}

func TestDefaultInstallDir(t *testing.T) {
	h := setHome(t)
	setGOOS(t, "darwin")
	if DefaultInstallDir() != filepath.Join(h, "Applications") {
		t.Fatal(DefaultInstallDir())
	}
	setGOOS(t, "windows")
	t.Setenv("LOCALAPPDATA", filepath.Join(h, "local"))
	if DefaultInstallDir() != filepath.Join(h, "local", "Programs", "mypack-apps") {
		t.Fatal(DefaultInstallDir())
	}
}

func TestReadAppInfoManaged(t *testing.T) {
	app := fakeApp(t, t.TempDir(), "Fake", true)
	info, err := ReadAppInfo(app)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "Fake & Co" || info.Identifier != "app.mypack.fake" || info.Version != "2.1.0" || !info.Managed || info.SourceURL != "https://example.com/" {
		t.Fatalf("%+v", info)
	}
	if len(info.IconPNG) == 0 || string(info.IconPNG[1:4]) != "PNG" {
		t.Fatal("icon not extracted from icns")
	}
}

func TestReadAppInfoExternalAndPortable(t *testing.T) {
	app := fakeApp(t, t.TempDir(), "Other", false)
	info, err := ReadAppInfo(app)
	if err != nil || info.Managed || info.SourceURL != "" || info.Identifier != "app.mypack.fake" {
		t.Fatalf("%+v %v", info, err)
	}
	// portable managed dir
	d := filepath.Join(t.TempDir(), "Site")
	os.MkdirAll(filepath.Join(d, "resources"), 0o755)
	os.WriteFile(filepath.Join(d, "resources", "mypack.json"), []byte(managedJSON), 0o644)
	os.WriteFile(filepath.Join(d, "resources", "icon.png"), icons.Default("x"), 0o644)
	info, err = ReadAppInfo(d)
	if err != nil || !info.Managed || info.Name != "Fake & Co" || len(info.IconPNG) == 0 {
		t.Fatalf("%+v %v", info, err)
	}
	if _, err := ReadAppInfo("/nonexistent/x.app"); code(err) != "NOT_FOUND" {
		t.Fatal(err)
	}
	// a plain exe
	exe := filepath.Join(t.TempDir(), "Tool.exe")
	os.WriteFile(exe, []byte("MZ"), 0o755)
	if info, err := ReadAppInfo(exe); err != nil || info.Name != "Tool" || info.Managed {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestParseBinaryPlist(t *testing.T) {
	if _, err := exec.LookPath("plutil"); err != nil {
		t.Skip("plutil not available")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "Info.plist")
	os.WriteFile(p, []byte(infoPlist), 0o644)
	if out, err := exec.Command("plutil", "-convert", "binary1", p).CombinedOutput(); err != nil {
		t.Skipf("plutil: %v %s", err, out)
	}
	raw, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(raw), "bplist00") {
		t.Fatal("not binary")
	}
	m, err := ParsePlist(raw)
	if err != nil {
		t.Fatal(err)
	}
	if m["CFBundleName"] != "Fake & Co" || m["LSUIElement"] != true || m["Count"] != int64(3) {
		t.Fatalf("%v", m)
	}
	arr := m["Arr"].([]any)
	if len(arr) != 2 || arr[1].(map[string]any)["x"] != "y" {
		t.Fatalf("%v", m["Arr"])
	}
	// as a bundle
	app := fakeApp(t, t.TempDir(), "Bin", false)
	b, _ := os.ReadFile(p)
	os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), b, 0o644)
	info, err := ReadAppInfo(app)
	if err != nil || info.Name != "Fake & Co" {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestParsePlistGarbage(t *testing.T) {
	for _, in := range []string{"", "garbage", "bplist00", "bplist00" + strings.Repeat("\xff", 60), "<plist><array/></plist>"} {
		if _, err := ParsePlist([]byte(in)); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}
