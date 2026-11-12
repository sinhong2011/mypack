package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Manager is the package manager that installed Mypack, if any.
type Manager string

const (
	ManagerNone       Manager = ""
	ManagerHomebrew   Manager = "Homebrew"
	ManagerWinget     Manager = "winget"
	ManagerChocolatey Manager = "Chocolatey"
	ManagerScoop      Manager = "Scoop"
	ManagerApt        Manager = "APT"
	ManagerDnf        Manager = "DNF"
	ManagerZypper     Manager = "zypper"
	ManagerAUR        Manager = "AUR"
	// ManagerLinuxPackage is a Linux package (in /opt/mypack) whose manager
	// could not be told.
	ManagerLinuxPackage Manager = "package manager"
)

// Source is how this copy of Mypack is updated.
type Source struct {
	Manager Manager // ManagerNone: Mypack updates itself, when it can
	Command string  // the command that updates it; empty when unknown
}

// Managed reports whether a package manager updates Mypack.
func (s Source) Managed() bool { return s.Manager != ManagerNone }

// Env is what Detect looks at; the zero fields take the real system's.
type Env struct {
	GOOS   string
	Exe    string                // the running executable, symlinks resolved
	Getenv func(string) string   // os.Getenv
	Exists func(string) bool     // whether a path exists
	Glob   func(string) []string // filepath.Glob
	Home   string                // the user's home directory
}

// DetectSource looks at where the running Mypack is installed.
func DetectSource() Source {
	exe, _ := os.Executable()
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	home, _ := os.UserHomeDir()
	return Detect(Env{GOOS: runtime.GOOS, Exe: exe, Home: home})
}

// Detect works out the Source from e. Package managers are told by the
// path of the app or the records they keep:
//
//   - Homebrew: the cask's folder, <prefix>/Caskroom/mypack, or the app
//     running from it;
//   - Scoop: the app in ~\scoop\apps\mypack (or $SCOOP\apps\mypack);
//   - Chocolatey: %ChocolateyInstall%\lib\mypack;
//   - winget: %LOCALAPPDATA%\Microsoft\WinGet\Packages\Mypack.Mypack_*, or
//     the app running from WinGet\Packages;
//   - Linux packages: the app in /opt/mypack or /usr, with the dpkg,
//     pacman or RPM tool on the system telling which.
func Detect(e Env) Source {
	e = e.withDefaults()
	switch e.GOOS {
	case "darwin":
		return detectDarwin(e)
	case "windows":
		return detectWindows(e)
	case "linux":
		return detectLinux(e)
	}
	return Source{}
}

func (e Env) withDefaults() Env {
	if e.GOOS == "" {
		e.GOOS = runtime.GOOS
	}
	if e.Getenv == nil {
		e.Getenv = os.Getenv
	}
	if e.Exists == nil {
		e.Exists = func(p string) bool { _, err := os.Stat(p); return err == nil }
	}
	if e.Glob == nil {
		e.Glob = func(p string) []string { m, _ := filepath.Glob(p); return m }
	}
	return e
}

func detectDarwin(e Env) Source {
	brew := Source{Manager: ManagerHomebrew, Command: "brew upgrade --cask mypack"}
	if strings.Contains(e.Exe, "/Caskroom/mypack/") {
		return brew
	}
	prefixes := []string{"/opt/homebrew", "/usr/local"}
	if p := e.Getenv("HOMEBREW_PREFIX"); p != "" {
		prefixes = append([]string{p}, prefixes...)
	}
	for _, p := range prefixes {
		if e.Exists(p + "/Caskroom/mypack") {
			return brew
		}
	}
	return Source{}
}

// winPath lower-cases a Windows path and uses \ throughout, for prefix
// tests that hold whatever the case or separators.
func winPath(p string) string {
	return strings.ToLower(strings.ReplaceAll(p, "/", `\`))
}

func winJoin(parts ...string) string {
	return strings.Join(parts, `\`)
}

func detectWindows(e Env) Source {
	exe := winPath(e.Exe)

	scoopRoots := []string{}
	if s := e.Getenv("SCOOP"); s != "" {
		scoopRoots = append(scoopRoots, s)
	}
	if s := e.Getenv("SCOOP_GLOBAL"); s != "" {
		scoopRoots = append(scoopRoots, s)
	}
	if e.Home != "" {
		scoopRoots = append(scoopRoots, winJoin(e.Home, "scoop"))
	}
	for _, r := range scoopRoots {
		if strings.HasPrefix(exe, winPath(winJoin(r, "apps", "mypack"))+`\`) {
			return Source{Manager: ManagerScoop, Command: "scoop update mypack"}
		}
	}
	if strings.Contains(exe, `\scoop\apps\mypack\`) {
		return Source{Manager: ManagerScoop, Command: "scoop update mypack"}
	}

	choco := e.Getenv("ChocolateyInstall")
	if choco == "" {
		choco = `C:\ProgramData\chocolatey`
	}
	if lib := winJoin(choco, "lib", "mypack"); strings.HasPrefix(exe, winPath(lib)+`\`) || e.Exists(lib) {
		return Source{Manager: ManagerChocolatey, Command: "choco upgrade mypack"}
	}

	winget := Source{Manager: ManagerWinget, Command: "winget upgrade Mypack.Mypack"}
	if strings.Contains(exe, `\microsoft\winget\packages\`) {
		return winget
	}
	if la := e.Getenv("LOCALAPPDATA"); la != "" {
		if len(e.Glob(winJoin(la, "Microsoft", "WinGet", "Packages", "Mypack.Mypack_*"))) > 0 {
			return winget
		}
	}
	return Source{}
}

func detectLinux(e Env) Source {
	if !strings.HasPrefix(e.Exe, "/opt/mypack/") && !strings.HasPrefix(e.Exe, "/usr/") {
		return Source{} // install.sh's copy in the user's home, or an extracted archive
	}
	if e.Exists("/var/lib/dpkg/info/mypack.list") {
		return Source{Manager: ManagerApt, Command: "sudo apt update && sudo apt install --only-upgrade mypack"}
	}
	for _, dir := range e.Glob("/var/lib/pacman/local/mypack*") {
		if name := pacmanName(filepath.Base(dir)); name != "" {
			return Source{Manager: ManagerAUR, Command: "yay -Syu " + name}
		}
	}
	switch {
	case e.Exists("/usr/bin/dnf"):
		return Source{Manager: ManagerDnf, Command: "sudo dnf upgrade mypack"}
	case e.Exists("/usr/bin/zypper"):
		return Source{Manager: ManagerZypper, Command: "sudo zypper update mypack"}
	case e.Exists("/usr/bin/apt"):
		return Source{Manager: ManagerApt, Command: "sudo apt update && sudo apt install --only-upgrade mypack"}
	}
	return Source{Manager: ManagerLinuxPackage}
}

// pacmanName takes the package name from a pacman database entry,
// <name>-<version>-<release>, when it is Mypack's ("mypack" or
// "mypack-bin").
func pacmanName(entry string) string {
	i := strings.LastIndexByte(entry, '-')
	if i <= 0 {
		return ""
	}
	j := strings.LastIndexByte(entry[:i], '-')
	if j <= 0 {
		return ""
	}
	name := entry[:j]
	if name == "mypack" || name == "mypack-bin" || name == "mypack-git" {
		return name
	}
	return ""
}
