package builder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/pkg/config"
)

const (
	camUsage = "This app needs access to the camera for websites that request it."
	micUsage = "This app needs access to the microphone for websites that request it."

	// menuBarRuntime is the first runtime with window.mode "menubar".
	menuBarRuntime = "1.0.6"
)

func (j *job) infoPlist() []byte {
	c := j.cfg
	d := map[string]any{
		"CFBundleName":                         j.name,
		"CFBundleDisplayName":                  j.name,
		"CFBundleIdentifier":                   c.App.Identifier,
		"CFBundleExecutable":                   j.name,
		"CFBundleIconFile":                     "AppIcon",
		"CFBundleShortVersionString":           c.App.Version,
		"CFBundleVersion":                      c.App.Version,
		"CFBundlePackageType":                  "APPL",
		"CFBundleInfoDictionaryVersion":        "6.0",
		"LSMinimumSystemVersion":               "11.0",
		"NSHighResolutionCapable":              true,
		"NSPrincipalClass":                     "NSApplication",
		"NSSupportsAutomaticGraphicsSwitching": true,
		"LSApplicationCategoryType":            "public.app-category.productivity",
		"NSAppTransportSecurity":               map[string]any{"NSAllowsArbitraryLoadsInWebContent": true},
		"MyPackManaged":                        true,
		"MyPackRuntimeVersion":                 c.Runtime.Version,
	}
	if j.iconCar {
		// macOS 26+ draws the Icon Composer icon from Assets.car in the
		// appearance the user picked; older systems use AppIcon.icns.
		d["CFBundleIconName"] = icons.IconComposerName
	}
	if c.Native.Camera {
		d["NSCameraUsageDescription"] = camUsage
	}
	if c.Native.Microphone {
		d["NSMicrophoneUsageDescription"] = micUsage
	}
	// A menu-bar app has no Dock icon from launch on. Only runtimes that
	// show the menu-bar panel get it: an older runtime would open a window
	// with no Dock icon and no menu-bar icon to bring it back.
	if c.Window.EffectiveMode() == config.ModeMenuBar && config.RuntimeAtLeast(c.Runtime.Version, menuBarRuntime) {
		d["LSUIElement"] = true
	}
	return writePlist(d)
}

func (j *job) customizeDarwin() error {
	contents := filepath.Join(j.root, "Contents")
	haveICNS, err := j.iconComposer()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), j.infoPlist(), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(contents, "PkgInfo"), []byte("APPL????"), 0o644); err != nil {
		return err
	}
	if haveICNS {
		return nil
	}
	// Full-bleed artwork: macOS 26+ masks icons itself and "jails" icons
	// that carry their own rounded corners/margins on a light plate.
	art, err := icons.MacAppIcon(j.iconPNG)
	if err != nil {
		art = j.iconPNG
	}
	icns, err := icons.ICNS(art)
	if err != nil {
		return fmt.Errorf("icns: %w", err)
	}
	return os.WriteFile(filepath.Join(j.resDir, "AppIcon.icns"), icns, 0o644)
}

// iconComposer compiles an Icon Composer icon (app.icon.path naming a .icon
// document) into Assets.car and AppIcon.icns in the bundle's resources. It
// reports whether AppIcon.icns was written. When the document cannot be
// compiled the app gets the icon's picture instead, with a warning; an icon
// studio style always wins over the document.
func (j *job) iconComposer() (bool, error) {
	ic := j.cfg.App.Icon
	if ic.Source != "file" || !icons.IsIconComposer(ic.Path) {
		return false, nil
	}
	if ic.Style != nil {
		j.warn("The icon is styled in the icon studio, so the Icon Composer icon's light, dark and tinted looks are not used")
		return false, nil
	}
	if !icons.IconComposerAvailable() {
		j.warn("Icon Composer icons need a Mac with Xcode 26 or later; the app gets a plain icon that does not follow the icon appearance")
		return false, nil
	}
	out := filepath.Join(j.staging, "iconcomposer")
	c, err := icons.CompileIconComposer(j.ctx, ic.Path, out)
	if err != nil {
		if j.ctx.Err() != nil {
			return false, j.ctx.Err()
		}
		j.warn("The Icon Composer icon could not be compiled (%v); the app gets a plain icon", err)
		return false, nil
	}
	if err := copyFile(j.ctx, c.Car, filepath.Join(j.resDir, "Assets.car"), 0o644); err != nil {
		return false, err
	}
	j.iconCar = true
	j.log("info", "Compiled the Icon Composer icon %s", filepath.Base(ic.Path))
	if c.ICNS == "" {
		return false, nil
	}
	if err := copyFile(j.ctx, c.ICNS, filepath.Join(j.resDir, "AppIcon.icns"), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func (j *job) signDarwin() error {
	if !have("codesign") {
		j.signing = "unsigned"
		j.warn("codesign was not found; the app is unsigned and macOS may refuse to open it")
		return nil
	}
	if err := j.run("codesign", "--force", "--deep", "--sign", "-", "--timestamp=none", j.root); err != nil {
		return j.wrap(err, "SIGNING_FAILED", "Code signing failed", "Make sure Xcode command line tools are installed (xcode-select --install) and try again.")
	}
	j.signing = "adhoc"
	j.log("info", "Ad-hoc signed %s", filepath.Base(j.root))
	return nil
}

func (j *job) verifyDarwin() error {
	c := j.root
	need := []string{"Contents/Info.plist", "Contents/PkgInfo", "Contents/Resources/AppIcon.icns", "Contents/Resources/icon.png", "Contents/Resources/mypack.json"}
	if j.iconCar {
		need = append(need, "Contents/Resources/Assets.car")
	}
	for _, p := range need {
		if st, err := os.Stat(filepath.Join(c, p)); err != nil || st.Size() == 0 {
			return fmt.Errorf("missing %s", p)
		}
	}
	if err := checkExec(j.exePath); err != nil {
		return err
	}
	if err := j.checkConfigFile(); err != nil {
		return err
	}
	j.checkBinaryArch()
	if j.signing == "adhoc" {
		if err := j.run("codesign", "--verify", "--deep", "--strict", j.root); err != nil {
			return fmt.Errorf("code signature check failed: %w", err)
		}
	}
	return nil
}

func (j *job) packDMG(out string) error {
	if !have("hdiutil") {
		return errors.New("hdiutil is not available; DMG packaging needs macOS")
	}
	dir := filepath.Join(j.staging, "dmgroot")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := j.copyBundle(j.root, filepath.Join(dir, filepath.Base(j.root))); err != nil {
		return err
	}
	if err := os.Symlink("/Applications", filepath.Join(dir, "Applications")); err != nil {
		return err
	}
	if err := j.run("hdiutil", "create", "-volname", j.name, "-srcfolder", dir, "-ov", "-format", "UDZO", out); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// copyBundle copies a bundle with full fidelity (ditto on macOS).
func (j *job) copyBundle(src, dst string) error {
	if j.target.OS == "darwin" && have("ditto") {
		return j.run("ditto", src, dst)
	}
	return copyTree(j.ctx, src, dst)
}

func (j *job) packZip(src, out string) error {
	if j.target.OS == "darwin" && have("ditto") {
		return j.run("ditto", "-c", "-k", "--keepParent", src, out)
	}
	return zipTree(j.ctx, src, out)
}
