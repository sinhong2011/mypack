package icons

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Icon Composer documents (.icon folders, macOS 26+) hold layered artwork
// that macOS draws in the icon appearance the user picked (Default, Dark,
// Clear, Tinted). Only Apple's tools read them: actool compiles one into
// Assets.car (plus a .icns for older systems) and ictool renders a PNG. Both
// ship with Xcode 26 or later, so these helpers work on a Mac with Xcode.

// IconComposerName is the name a compiled Icon Composer icon gets in a
// bundle (Info.plist CFBundleIconName, AppIcon.icns).
const IconComposerName = "AppIcon"

// maxIconComposerBytes caps the size of a .icon folder that is copied.
const maxIconComposerBytes = 64 << 20

// IsIconComposer reports whether path is an Icon Composer document: a
// folder named *.icon holding icon.json.
func IsIconComposer(path string) bool {
	if !strings.EqualFold(filepath.Ext(path), ".icon") {
		return false
	}
	st, err := os.Stat(filepath.Join(path, "icon.json"))
	return err == nil && st.Mode().IsRegular()
}

// IconComposerAvailable reports whether this computer can compile Icon
// Composer documents (macOS with Xcode 26+).
func IconComposerAvailable() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.Command("xcrun", "--find", "actool").Output()
	return err == nil
}

// CompiledIcon is what CompileIconComposer writes.
type CompiledIcon struct {
	Car  string // Assets.car: the icon in every appearance (macOS 26+)
	ICNS string // AppIcon.icns: the default appearance, for older systems
}

// CompileIconComposer compiles the Icon Composer document src with actool
// into outDir, as an icon named IconComposerName.
func CompileIconComposer(ctx context.Context, src, outDir string) (CompiledIcon, error) {
	var out CompiledIcon
	if !IsIconComposer(src) {
		return out, fmt.Errorf("icons: %s is not an Icon Composer document", src)
	}
	if runtime.GOOS != "darwin" {
		return out, errors.New("icons: Icon Composer icons can only be compiled on a Mac")
	}
	// actool names the icon after the document and ignores symlinks to it,
	// so it compiles a copy named AppIcon.icon.
	work, err := os.MkdirTemp("", "mypack-icon-")
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(work)
	doc := filepath.Join(work, IconComposerName+".icon")
	if err := copyDocument(src, doc); err != nil {
		return out, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return out, err
	}
	cmd := exec.CommandContext(ctx, "xcrun", "actool", doc,
		"--compile", outDir,
		"--app-icon", IconComposerName,
		"--enable-on-demand-resources", "NO",
		"--development-region", "en",
		"--target-device", "mac",
		"--platform", "macosx",
		"--minimum-deployment-target", "11.0",
		"--output-partial-info-plist", filepath.Join(work, "partial.plist"),
		"--errors", "--warnings",
		"--output-format", "human-readable-text")
	var log bytes.Buffer
	cmd.Stdout, cmd.Stderr = &log, &log
	runErr := cmd.Run()
	out = CompiledIcon{Car: filepath.Join(outDir, "Assets.car"), ICNS: filepath.Join(outDir, IconComposerName+".icns")}
	if runErr != nil || !nonEmpty(out.Car) {
		msg := strings.TrimSpace(actoolErrors(log.String()))
		if runErr != nil && msg == "" {
			msg = runErr.Error()
		}
		if msg == "" {
			msg = "actool produced no Assets.car"
		}
		return CompiledIcon{}, fmt.Errorf("icons: compiling %s: %s", filepath.Base(src), msg)
	}
	if !nonEmpty(out.ICNS) {
		out.ICNS = ""
	}
	return out, nil
}

// IconComposerPNG renders the default appearance of the Icon Composer
// document src as a normalized 1024 PNG. It uses ictool when it is found,
// else the largest image in the .icns that actool makes.
func IconComposerPNG(ctx context.Context, src string) ([]byte, error) {
	if !IsIconComposer(src) {
		return nil, fmt.Errorf("icons: %s is not an Icon Composer document", src)
	}
	if runtime.GOOS != "darwin" {
		return nil, errors.New("icons: Icon Composer icons can only be read on a Mac")
	}
	work, err := os.MkdirTemp("", "mypack-icon-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	if tool := ictoolPath(); tool != "" {
		png := filepath.Join(work, "icon.png")
		cmd := exec.CommandContext(ctx, tool, src, "--export-image", "--output-file", png,
			"--platform", "macOS", "--rendition", "Default",
			"--width", "1024", "--height", "1024", "--scale", "1")
		if cmd.Run() == nil {
			if b, err := os.ReadFile(png); err == nil {
				if n, err := Normalize(b); err == nil {
					return n, nil
				}
			}
		}
	}
	c, err := CompileIconComposer(ctx, src, filepath.Join(work, "out"))
	if err != nil {
		return nil, err
	}
	if c.ICNS == "" {
		return nil, errors.New("icons: actool made no .icns to preview")
	}
	b, err := os.ReadFile(c.ICNS)
	if err != nil {
		return nil, err
	}
	p, err := ICNSLargestPNG(b)
	if err != nil {
		return nil, err
	}
	return Normalize(p)
}

// ictoolPath finds Icon Composer's command-line tool: inside Xcode, or in a
// standalone Icon Composer app.
func ictoolPath() string {
	const rel = "Icon Composer.app/Contents/Executables/ictool"
	var dirs []string
	if b, err := exec.Command("xcode-select", "-p").Output(); err == nil {
		// …/Xcode.app/Contents/Developer → …/Xcode.app/Contents/Applications
		dirs = append(dirs, filepath.Join(filepath.Dir(strings.TrimSpace(string(b))), "Applications"))
	}
	dirs = append(dirs, "/Applications/Xcode.app/Contents/Applications", "/Applications")
	for _, d := range dirs {
		p := filepath.Join(d, rel)
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// copyDocument copies the .icon folder src to dst. Symbolic links are
// skipped, and the copy is capped at maxIconComposerBytes.
func copyDocument(src, dst string) error {
	var total int64
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case !d.Type().IsRegular():
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if total += int64(len(b)); total > maxIconComposerBytes {
			return errors.New("icons: the .icon document is larger than 64 MB")
		}
		return os.WriteFile(target, b, 0o644)
	})
}

// actoolErrors keeps actool's error and warning lines.
func actoolErrors(log string) string {
	var keep []string
	for _, l := range strings.Split(log, "\n") {
		if strings.Contains(l, ": error:") || strings.Contains(l, ": warning:") {
			keep = append(keep, strings.TrimSpace(l))
		}
	}
	return strings.Join(keep, "\n")
}

func nonEmpty(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Size() > 0
}
