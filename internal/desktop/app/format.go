package app

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/i18n"
	"mypack.dev/mypack/pkg/config"
)

// T translates a stable message ID in the active locale.
func T(id string, args ...any) string { return i18n.T(id, args...) }

// ago is a locale-aware relative time ("2 hours ago").
func ago(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return T("time.justNow")
	case d < time.Hour:
		return T("time.minutesAgo", "count", int(d.Minutes()))
	case d < 24*time.Hour:
		return T("time.hoursAgo", "count", int(d.Hours()))
	case d < 30*24*time.Hour:
		return T("time.daysAgo", "count", int(d.Hours()/24))
	}
	return t.Format("2006-01-02")
}

// duration formats a build duration ("12s", "1m 04s").
func duration(d time.Duration) string {
	if d < 0 {
		return ""
	}
	s := int(math.Round(d.Seconds()))
	if s < 60 {
		return T("duration.seconds", "s", s)
	}
	return T("duration.minutes", "m", s/60, "s", fmt.Sprintf("%02d", s%60))
}

// bytes formats a size with decimal units (as Finder does).
func bytesLabel(n int64) string {
	f := float64(n)
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.2f GB", f/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.1f MB", f/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.0f KB", f/1e3)
	}
	return fmt.Sprintf("%d B", n)
}

// osLabel names an OS for people.
func osLabel(os string) string {
	switch os {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	}
	return os
}

// archLabel names a CPU family for people.
func archLabel(os, arch string) string {
	switch arch {
	case "arm64":
		if os == "darwin" {
			return T("arch.appleSilicon")
		}
		return "ARM64"
	case "amd64":
		if os == "darwin" {
			return T("arch.intel")
		}
		return "x64"
	case "universal":
		return T("arch.universal")
	}
	return arch
}

// targetLabel is "macOS · Apple Silicon".
func targetLabel(t config.Target) string {
	return osLabel(t.OS) + " · " + archLabel(t.OS, t.Arch)
}

// deviceWord is "Mac" / "PC" / "Linux" for the primary CTA.
func deviceWord(host config.Target) string {
	switch host.OS {
	case "darwin":
		return "Mac"
	case "windows":
		return "PC"
	}
	return "Linux"
}

// thisDevice is "This Mac (Apple Silicon)".
func thisDevice(host config.Target) string {
	return T("target.thisDevice", "device", deviceWord(host), "arch", archLabel(host.OS, host.Arch))
}

// domainOf returns the display domain for a source.
func domainOf(src config.Source) string {
	switch src.Type {
	case config.SourceURL:
		return config.RegistrableHost(src.Value)
	case config.SourceFile, config.SourceDir:
		return shortPath(src.Value)
	}
	return src.Value
}

// addressLabel is what a window's address bar shows for a source: the
// bare host of a website, or "<name> · local" for a file or folder.
func addressLabel(src config.Source, name string) string {
	if src.Type == config.SourceURL {
		if u, err := url.Parse(src.Value); err == nil && u.Hostname() != "" {
			return strings.TrimPrefix(u.Hostname(), "www.")
		}
		return config.RegistrableHost(src.Value)
	}
	if name == "" {
		base := src.Value
		if src.Project != "" {
			base = src.Project
		}
		name = filepath.Base(base)
	}
	return T("address.local", "name", name)
}

func shortPath(p string) string {
	if p == "" {
		return ""
	}
	parts := strings.Split(p, "/")
	if len(parts) > 3 {
		return "…/" + strings.Join(parts[len(parts)-2:], "/")
	}
	return p
}

// errInfo extracts a domain error.
func errInfo(err error) *domain.Error {
	if err == nil {
		return nil
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return de
	}
	return &domain.Error{Code: "INTERNAL", Message: err.Error()}
}

func errCode(err error) string {
	if e := errInfo(err); e != nil {
		return e.Code
	}
	return ""
}

// friendly returns a short human message for an error.
func friendly(err error) string {
	e := errInfo(err)
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}
