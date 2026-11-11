// Package installer places artifacts on this machine and manages them:
// Install & Open, atomic update with backup, repair, uninstall, launch,
// reveal (spec §9C.4/9C.5). Profile data is never touched except by
// ClearProfile.
package installer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"mypack.dev/mypack/internal/domain"
)

// Test seams.
var (
	goos      = runtime.GOOS
	runCmd    = func(name string, args ...string) error { return exec.Command(name, args...).Run() }
	startCmd  = func(name string, args ...string) error { return startDetached(name, args...) }
	outputCmd = func(name string, args ...string) ([]byte, error) { return exec.Command(name, args...).Output() }
)

func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func derr(code, msg string, cause error, remediation string) *domain.Error {
	e := &domain.Error{Code: code, Message: msg, Remediation: remediation}
	if cause != nil {
		e.Cause = cause.Error()
	}
	return e
}

func home() string {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return os.TempDir()
	}
	return h
}

// DefaultInstallDir: macOS ~/Applications, Windows %LOCALAPPDATA%\Programs\mypack-apps,
// Linux ~/.local/share/mypack-apps (plus a .desktop file in ~/.local/share/applications).
func DefaultInstallDir() string {
	switch goos {
	case "darwin":
		return filepath.Join(home(), "Applications")
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(home(), "AppData", "Local")
		}
		return filepath.Join(base, "Programs", "mypack-apps")
	default:
		return filepath.Join(dataHome(), "mypack-apps")
	}
}

func dataHome() string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" && filepath.IsAbs(x) {
		return x
	}
	return filepath.Join(home(), ".local", "share")
}

func desktopEntryDir() string { return filepath.Join(dataHome(), "applications") }

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ProfileDir returns the per-user website-data directory for a profile ID:
// macOS ~/Library/Application Support/mypack-apps/<id>, Windows
// %APPDATA%\mypack-apps\<id>, Linux $XDG_DATA_HOME/mypack-apps/<id>.
func ProfileDir(profileID string) string {
	id := sanitizeID(profileID)
	switch goos {
	case "darwin":
		return filepath.Join(home(), "Library", "Application Support", "mypack-apps", id)
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(home(), "AppData", "Roaming")
		}
		return filepath.Join(base, "mypack-apps", id)
	default:
		return filepath.Join(dataHome(), "mypack-apps", id)
	}
}

// sanitizeID makes an ID safe as a single path component.
func sanitizeID(id string) string {
	if safeID.MatchString(id) && !strings.Contains(id, "..") {
		return id
	}
	var b strings.Builder
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := strings.ReplaceAll(b.String(), "..", "_")
	out = strings.TrimLeft(out, ".-")
	if out == "" {
		out = "_"
	}
	return out
}

// ClearProfile deletes a profile directory.
func ClearProfile(profileID string) error {
	if strings.TrimSpace(profileID) == "" {
		return derr("INVALID_CONFIG", "No profile ID given", nil, "Provide the app's profile ID.")
	}
	if !safeID.MatchString(profileID) || strings.Contains(profileID, "..") {
		return derr("INVALID_CONFIG", "Invalid profile ID", fmt.Errorf("%q", profileID), "Profile IDs look like com.example.app.")
	}
	dir := ProfileDir(profileID)
	if err := os.RemoveAll(dir); err != nil {
		return derr("BUILD_FAILED", "Could not delete the app's website data", err, "Close the app and try again.")
	}
	return nil
}

func isApp(p string) bool { return strings.HasSuffix(strings.ToLower(p), ".app") }

// Install copies artifactPath (a .app bundle or portable dir) into installDir.
// If the destination exists and replace is false it returns a *domain.Error
// with code INSTALL_CONFLICT. When replace is true the old copy is moved to
// a backup first and restored if the copy fails. Returns the installed path.
func Install(ctx context.Context, artifactPath, installDir string, replace bool) (string, error) {
	st, err := os.Stat(artifactPath)
	if err != nil {
		return "", derr("NOT_FOUND", "The app to install was not found", err, "Build the app again.")
	}
	if !st.IsDir() {
		return "", derr("INSTALL_UNSUPPORTED", "Only app bundles and portable folders can be installed", fmt.Errorf("%s is a file", filepath.Base(artifactPath)),
			"Build the 'app' or 'portable' format, or open the "+filepath.Ext(artifactPath)+" file yourself.")
	}
	if installDir == "" {
		installDir = DefaultInstallDir()
	}
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", derr("INSTALL_FAILED", "Cannot create the install folder", err, "Check permissions of "+installDir+".")
	}
	dest := filepath.Join(installDir, filepath.Base(artifactPath))
	if absA, _ := filepath.Abs(artifactPath); absA == mustAbs(dest) {
		return "", derr("INSTALL_CONFLICT", "The app is already in the install folder", nil, "Choose a different install folder.")
	}
	backup := ""
	if _, err := os.Lstat(dest); err == nil {
		if !replace {
			return "", derr("INSTALL_CONFLICT", "An app with this name is already installed", fmt.Errorf("%s exists", dest),
				"Choose Replace to update it, or uninstall the existing one first.")
		}
		if IsRunning(dest) {
			return "", derr("APP_RUNNING", "The app is running", nil, "Quit the app, then install again.")
		}
		backup = uniqueSibling(dest, "mypack-bak")
		if err := os.Rename(dest, backup); err != nil {
			return "", derr("INSTALL_FAILED", "Cannot move the existing app aside", err, "Quit the app and check permissions.")
		}
	}
	if err := copyBundle(ctx, artifactPath, dest); err != nil {
		_ = os.RemoveAll(dest)
		if backup != "" {
			_ = os.Rename(backup, dest)
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", derr("INSTALL_FAILED", "Copying the app failed", err, "Check free disk space and permissions of "+installDir+".")
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	postInstall(dest)
	return dest, nil
}

func mustAbs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

func uniqueSibling(p, tag string) string {
	return fmt.Sprintf("%s.%s-%d", p, tag, time.Now().UnixNano())
}

// copyBundle copies with full fidelity: ditto on macOS (resource forks,
// xattrs, symlinks), a Go copy preserving symlinks and modes elsewhere.
func copyBundle(ctx context.Context, src, dst string) error {
	if goos == "darwin" && runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("ditto"); err == nil {
			cmd := exec.CommandContext(ctx, "ditto", src, dst)
			if out, err := cmd.CombinedOutput(); err != nil {
				_ = os.RemoveAll(dst)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("ditto: %w: %s", err, strings.TrimSpace(string(out)))
			}
			return nil
		}
	}
	return copyTree(ctx, src, dst)
}

func copyTree(ctx context.Context, src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode()&fs.ModeSymlink != 0:
			l, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(l, target)
		case info.Mode().IsRegular():
			return copyFile(ctx, p, target, info.Mode().Perm())
		}
		return nil
	})
}

func copyFile(ctx context.Context, src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	buf := make([]byte, 256<<10)
	for {
		if err := ctx.Err(); err != nil {
			out.Close()
			return err
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return werr
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			out.Close()
			return rerr
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

// postInstall registers Linux desktop entries (best effort).
func postInstall(dest string) {
	if goos != "linux" {
		return
	}
	ents, _ := os.ReadDir(dest)
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".desktop") {
			b, err := os.ReadFile(filepath.Join(dest, e.Name()))
			if err != nil {
				continue
			}
			content := strings.ReplaceAll(string(b), "__INSTALL_DIR__", dest)
			dir := desktopEntryDir()
			if os.MkdirAll(dir, 0o755) == nil {
				_ = os.WriteFile(filepath.Join(dir, e.Name()), []byte(content), 0o644)
			}
		}
	}
}

func removeDesktopEntries(installed string) {
	if goos != "linux" {
		return
	}
	ents, _ := os.ReadDir(installed)
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".desktop") {
			p := filepath.Join(desktopEntryDir(), e.Name())
			if b, err := os.ReadFile(p); err == nil && strings.Contains(string(b), installed) {
				_ = os.Remove(p)
			}
		}
	}
}

// Replace atomically swaps installedPath with newArtifactPath, keeping the
// previous version at a backup path (returned) until the caller confirms
// (DiscardBackup) or rolls back (RestoreBackup). If the app is running it
// returns code APP_RUNNING.
func Replace(ctx context.Context, installedPath, newArtifactPath string) (backupPath string, err error) {
	if _, err := os.Lstat(installedPath); err != nil {
		return "", derr("NOT_FOUND", "The installed app was not found", err, "Install it first.")
	}
	st, err := os.Stat(newArtifactPath)
	if err != nil || !st.IsDir() {
		return "", derr("NOT_FOUND", "The new build was not found", err, "Build the app again.")
	}
	if IsRunning(installedPath) {
		return "", derr("APP_RUNNING", "The app is running", nil, "Quit the app, then update it.")
	}
	// 1. copy next to the target first: the old app stays intact if this fails
	staged := uniqueSibling(installedPath, "mypack-new")
	if err := copyBundle(ctx, newArtifactPath, staged); err != nil {
		_ = os.RemoveAll(staged)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", derr("INSTALL_FAILED", "Copying the new version failed; nothing was changed", err, "Check free disk space and try again.")
	}
	// 2. swap
	backup := uniqueSibling(installedPath, "mypack-prev")
	if err := os.Rename(installedPath, backup); err != nil {
		_ = os.RemoveAll(staged)
		return "", derr("INSTALL_FAILED", "Cannot move the old version aside; nothing was changed", err, "Quit the app and check permissions.")
	}
	if err := os.Rename(staged, installedPath); err != nil {
		_ = os.Rename(backup, installedPath)
		_ = os.RemoveAll(staged)
		return "", derr("INSTALL_FAILED", "Cannot put the new version in place; the old version was restored", err, "Check permissions and try again.")
	}
	postInstall(installedPath)
	return backup, nil
}

// RestoreBackup undoes a Replace: the current installedPath is discarded and
// the backup moved back.
func RestoreBackup(installedPath, backupPath string) error {
	if _, err := os.Lstat(backupPath); err != nil {
		return derr("NOT_FOUND", "The backup no longer exists", err, "")
	}
	tmp := uniqueSibling(installedPath, "mypack-fail")
	if _, err := os.Lstat(installedPath); err == nil {
		if err := os.Rename(installedPath, tmp); err != nil {
			return derr("INSTALL_FAILED", "Cannot restore the previous version", err, "Quit the app and try again.")
		}
	}
	if err := os.Rename(backupPath, installedPath); err != nil {
		_ = os.Rename(tmp, installedPath)
		return derr("INSTALL_FAILED", "Cannot restore the previous version", err, "")
	}
	_ = os.RemoveAll(tmp)
	return nil
}

// DiscardBackup deletes a backup returned by Replace once the caller is happy.
func DiscardBackup(backupPath string) error {
	if !strings.Contains(filepath.Base(backupPath), ".mypack-prev-") {
		return derr("INVALID_CONFIG", "Not a mypack backup path", fmt.Errorf("%s", backupPath), "")
	}
	return os.RemoveAll(backupPath)
}

// IsRunning reports whether an installed app is currently running.
func IsRunning(installedPath string) bool {
	abs := mustAbs(installedPath)
	switch goos {
	case "darwin":
		pat := regexp.QuoteMeta(filepath.Join(abs, "Contents", "MacOS")) + "/"
		return runCmd("pgrep", "-f", pat) == nil
	case "linux":
		return linuxRunning(abs)
	case "windows":
		script := fmt.Sprintf(`$p='%s\*'; if (Get-Process | Where-Object { $_.Path -like $p } | Select-Object -First 1) { exit 0 } else { exit 1 }`, strings.ReplaceAll(abs, "'", "''"))
		return runCmd("powershell", "-NoProfile", "-NonInteractive", "-Command", script) == nil
	}
	return false
}

func linuxRunning(dir string) bool {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	prefix := dir + string(os.PathSeparator)
	for _, e := range ents {
		if len(e.Name()) == 0 || e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		exe, err := os.Readlink(filepath.Join("/proc", e.Name(), "exe"))
		if err == nil && strings.HasPrefix(strings.TrimSuffix(exe, " (deleted)"), prefix) {
			return true
		}
	}
	return false
}

// findExecutable locates the main program of a portable directory.
func findExecutable(dir string) (string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	base := filepath.Base(dir)
	var cands []string
	for _, e := range ents {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".desktop") {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if goos == "windows" {
			if strings.HasSuffix(strings.ToLower(e.Name()), ".exe") {
				cands = append(cands, e.Name())
			}
		} else if info.Mode()&0o111 != 0 {
			cands = append(cands, e.Name())
		}
	}
	for _, c := range cands {
		if strings.TrimSuffix(c, ".exe") == base || strings.EqualFold(strings.TrimSuffix(c, ".exe"), base) {
			return filepath.Join(dir, c), nil
		}
	}
	if len(cands) > 0 {
		return filepath.Join(dir, cands[0]), nil
	}
	return "", errors.New("no executable found in " + dir)
}

// Executable returns the program inside an installed Windows/Linux app folder
// ("" when it cannot be found, and always on macOS, where the bundle is
// opened by path). Link routing records it so other Mypack apps can hand URLs
// to this one.
func Executable(path string) string {
	if isApp(path) {
		return ""
	}
	exe, err := findExecutable(path)
	if err != nil {
		return ""
	}
	return exe
}

// Launch starts an installed app (open -n on macOS, exec elsewhere).
func Launch(path string) error {
	if _, err := os.Stat(path); err != nil {
		return derr("NOT_FOUND", "The app was not found", err, "It may have been moved or deleted.")
	}
	var err error
	switch goos {
	case "darwin":
		err = startCmd("open", "-n", path)
	case "windows":
		exe := path
		if st, e := os.Stat(path); e == nil && st.IsDir() {
			if exe, err = findExecutable(path); err != nil {
				return derr("LAUNCH_FAILED", "Cannot find the program to start", err, "Reinstall the app.")
			}
		}
		err = startCmd("cmd", "/c", "start", "", exe)
	default:
		exe := path
		if st, e := os.Stat(path); e == nil && st.IsDir() {
			if exe, err = findExecutable(path); err != nil {
				return derr("LAUNCH_FAILED", "Cannot find the program to start", err, "Reinstall the app.")
			}
		}
		err = startCmd(exe)
	}
	if err != nil {
		return derr("LAUNCH_FAILED", "The app could not be started", err, "Try opening it from the file manager.")
	}
	return nil
}

// Reveal shows path in Finder/Explorer/file manager.
func Reveal(path string) error {
	if _, err := os.Lstat(path); err != nil {
		return derr("NOT_FOUND", "The item was not found", err, "It may have been moved or deleted.")
	}
	var err error
	switch goos {
	case "darwin":
		err = runCmd("open", "-R", path)
	case "windows":
		// explorer.exe returns exit status 1 even on success
		_ = runCmd("explorer", "/select,"+path)
	default:
		err = runCmd("xdg-open", filepath.Dir(path))
	}
	if err != nil {
		return derr("REVEAL_FAILED", "Could not open the file manager", err, "Open "+filepath.Dir(path)+" manually.")
	}
	return nil
}

// OpenURL opens an http(s) URL in the default browser.
func OpenURL(url string) error {
	low := strings.ToLower(strings.TrimSpace(url))
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		return derr("INVALID_CONFIG", "Only http and https links can be opened", nil, "")
	}
	var err error
	switch goos {
	case "darwin":
		err = startCmd("open", url)
	case "windows":
		err = startCmd("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		err = startCmd("xdg-open", url)
	}
	if err != nil {
		return derr("LAUNCH_FAILED", "Could not open the browser", err, "Open the link manually: "+url)
	}
	return nil
}

// Uninstall removes the program (to the Trash on macOS, deleted elsewhere).
// Website data (the profile) is untouched; use ClearProfile for that.
func Uninstall(installedPath string) error {
	abs := mustAbs(installedPath)
	if err := safeToRemove(abs); err != nil {
		return err
	}
	if _, err := os.Lstat(abs); err != nil {
		return derr("NOT_FOUND", "The app was not found", err, "It may already have been removed.")
	}
	removeDesktopEntries(abs)
	if goos == "darwin" {
		trash := filepath.Join(home(), ".Trash")
		if err := os.MkdirAll(trash, 0o700); err == nil {
			dest := filepath.Join(trash, filepath.Base(abs))
			if _, err := os.Lstat(dest); err == nil {
				ext := filepath.Ext(dest)
				dest = strings.TrimSuffix(dest, ext) + " " + time.Now().Format("15.04.05.000") + ext
			}
			if err := os.Rename(abs, dest); err == nil {
				return nil
			}
		}
		// cross-volume or permission problem: fall through to deleting
	}
	if err := os.RemoveAll(abs); err != nil {
		return derr("UNINSTALL_FAILED", "Could not remove the app", err, "Quit the app and try again.")
	}
	return nil
}

func safeToRemove(abs string) error {
	clean := filepath.Clean(abs)
	vol := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, vol)
	depth := len(strings.FieldsFunc(rest, func(r rune) bool { return r == filepath.Separator }))
	h := mustAbs(home())
	if depth < 3 || clean == h || clean == mustAbs(DefaultInstallDir()) || clean == mustAbs(filepath.Dir(h)) {
		return derr("INVALID_CONFIG", "Refusing to remove this path", fmt.Errorf("%s", abs), "Only installed app folders can be uninstalled.")
	}
	return nil
}
