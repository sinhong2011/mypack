package builder

import (
	"runtime"

	"mypack.dev/mypack/internal/runtimemanager"
	"mypack.dev/mypack/pkg/config"
)

// Capability describes what this host can build for a target (spec §6 matrix).
type Capability struct {
	Supported bool
	Level     string // "portable" | "installer" | "distribution"
	Signing   string // "adhoc" | "unsigned" | "unavailable"
	Reason    string
}

// Capabilities evaluates host → target support.
func Capabilities(host, target config.Target) Capability {
	switch target.OS {
	case "darwin":
		if host.OS != "darwin" {
			return Capability{Signing: "unavailable", Reason: "macOS apps can only be built on a Mac (code signing)"}
		}
		switch target.Format {
		case "app", "zip":
			return Capability{Supported: true, Level: "portable", Signing: "adhoc"}
		case "dmg":
			return Capability{Supported: true, Level: "installer", Signing: "adhoc"}
		}
		return Capability{Signing: "unavailable", Reason: "Format " + target.Format + " is not available for macOS; use app, zip or dmg"}
	case "windows":
		switch target.Format {
		case "portable", "zip":
			c := Capability{Supported: true, Level: "portable", Signing: "unsigned"}
			if host.OS != "windows" {
				c.Reason = crossReason(host)
			}
			return c
		case "exe":
			return Capability{Signing: "unavailable", Reason: "Windows installers (exe/msi) are not available yet; use the portable format"}
		}
		return Capability{Signing: "unavailable", Reason: "Format " + target.Format + " is not available for Windows; use portable or zip"}
	case "linux":
		switch target.Format {
		case "portable", "zip":
			c := Capability{Supported: true, Level: "portable", Signing: "unsigned"}
			if host.OS != "linux" {
				c.Reason = crossReason(host)
			}
			return c
		case "deb":
			c := Capability{Supported: true, Level: "installer", Signing: "unsigned"}
			if host.OS != "linux" {
				c.Reason = crossReason(host)
			}
			return c
		case "appimage":
			return Capability{Signing: "unavailable", Reason: "AppImage packaging is not supported yet; use portable or deb"}
		}
		return Capability{Signing: "unavailable", Reason: "Format " + target.Format + " is not available for Linux; use portable, deb or zip"}
	}
	return Capability{Signing: "unavailable", Reason: "Unknown target OS " + target.OS}
}

func crossReason(host config.Target) string {
	name := map[string]string{"darwin": "macOS", "windows": "Windows", "linux": "Linux"}[host.OS]
	if name == "" {
		name = host.OS
	}
	return "Built on " + name + "; not yet tested on the target system"
}

// HostTarget returns the current OS/arch with its default format.
func HostTarget() config.Target {
	return config.DefaultTarget(config.Target{OS: runtime.GOOS, Arch: runtime.GOARCH})
}

// requiredCapabilities maps config features to runtime capability names.
// They are only enforced when the registry entry advertises capabilities.
func requiredCapabilities(c *config.Config) []string {
	var r []string
	if c.Native.Tray || c.Native.StartToTray {
		r = append(r, "tray")
	}
	if c.Native.GlobalShortcut != "" {
		r = append(r, "globalShortcut")
	}
	if c.Native.Camera || c.Native.Microphone {
		r = append(r, "media")
	}
	if c.Native.DragDrop {
		r = append(r, "dragDrop")
	}
	if c.Network.Proxy != "" {
		r = append(r, "proxy")
	}
	if len(c.WebView.Injections) > 0 {
		r = append(r, "injection")
	}
	return r
}

func missingCapabilities(c *config.Config, e runtimemanager.Entry) []string {
	if len(e.Capabilities) == 0 {
		return nil // registry does not declare capabilities: assume full support
	}
	var missing []string
	for _, r := range requiredCapabilities(c) {
		if !e.HasCapability(r) {
			missing = append(missing, r)
		}
	}
	return missing
}
