package config

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandUserPath expands a leading ~ to the user's home directory.
// "~/Downloads" and "~\Downloads" become the Downloads folder in $HOME;
// a bare "~" becomes $HOME. Other paths are cleaned and otherwise unchanged.
// filepath.Join would treat "~/Downloads"[1:] as an absolute "/Downloads"
// and drop the home directory, so this trims the separator first.
func ExpandUserPath(p string) string {
	home, _ := os.UserHomeDir()
	return expandUserPath(p, home)
}

func expandUserPath(p, home string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, `~\`) {
		return filepath.Clean(p)
	}
	if home == "" {
		return p
	}
	rest := strings.TrimLeft(p[1:], `/\`)
	if rest == "" {
		return home
	}
	return filepath.Join(home, rest)
}

// ResolveDownloadDir is the folder files downloaded in a generated app are
// saved to: the configured path (with ~ expanded), else the system
// Downloads folder, else ~/Downloads, else tmp. The first candidate that
// can be created is used. Relative configured paths are resolved against
// home.
func ResolveDownloadDir(configured, home, osDownloads, tmp string, mkdir func(string) error) string {
	var candidates []string
	if d := expandUserPath(configured, home); d != "" {
		if !filepath.IsAbs(d) && home != "" {
			d = filepath.Join(home, d)
		}
		if filepath.IsAbs(d) {
			candidates = append(candidates, d)
		}
	}
	if osDownloads != "" {
		candidates = append(candidates, osDownloads)
	}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, "Downloads"))
	}
	if tmp != "" {
		candidates = append(candidates, tmp)
	}
	if mkdir == nil {
		mkdir = func(string) error { return nil }
	}
	for _, d := range candidates {
		if mkdir(d) == nil {
			return d
		}
	}
	return ""
}

// SafeDownloadName is a file name that can sit in the download folder:
// the last path element of name, or "download" when name is empty or a
// traversal. Separators are treated as / on every OS so a Unix path such
// as /tmp/a.bin is not read as a Windows UNC volume.
func SafeDownloadName(name string) string {
	name = strings.ReplaceAll(strings.TrimSpace(name), `\`, "/")
	name = strings.Trim(name, "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" || name == "." || name == ".." {
		return "download"
	}
	return name
}
