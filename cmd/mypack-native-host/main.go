// Command mypack-native-host is the Native Messaging host for the optional
// mypack Companion browser extension (spec 09G). It speaks length-prefixed
// JSON on stdio only and never opens a network listener.
//
//	mypack-native-host                       serve one browser connection (launched by the browser)
//	mypack-native-host install   [--extension-id ID ...] [--browser chrome|edge|all]
//	mypack-native-host uninstall [--browser chrome|edge|all]
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/companion"
)

type idList []string

func (l *idList) String() string { return strings.Join(*l, ",") }
func (l *idList) Set(v string) error {
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			*l = append(*l, p)
		}
	}
	return nil
}

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "install":
			os.Exit(install(args[1:], os.Stdout, os.Stderr))
		case "uninstall":
			os.Exit(uninstall(args[1:], os.Stdout, os.Stderr))
		case "-h", "--help", "help":
			usage(os.Stdout)
			return
		}
	}
	// Browsers pass the caller origin (and on Windows a parent window
	// handle) as arguments; they are ignored: the manifest allowlist is the
	// origin check.
	os.Exit(serve(os.Stdin, os.Stdout))
}

func usage(w io.Writer) {
	fmt.Fprint(w, `mypack-native-host - Native Messaging host for the mypack Companion extension

Usage:
  mypack-native-host install   --extension-id ID [--extension-id ID ...] [--browser chrome|edge|all]
  mypack-native-host uninstall [--browser chrome|edge|all]

Run without arguments only from a browser (stdio Native Messaging).
`)
}

func serve(in io.Reader, out io.Writer) int {
	svc, err := appservice.New(appservice.Options{DBPath: os.Getenv("MYPACK_DB"), CacheDir: os.Getenv("MYPACK_CACHE_DIR")})
	var h *companion.Handler
	if err != nil {
		fmt.Fprintln(os.Stderr, "mypack-native-host: cannot open mypack:", err)
		h = companion.NewHandler(nil) // hello still works; the rest is HOST_UNAVAILABLE
	} else {
		defer svc.Close()
		h = companion.NewHandler(svc)
	}
	if err := companion.Serve(context.Background(), in, out, h); err != nil {
		fmt.Fprintln(os.Stderr, "mypack-native-host:", err)
		return 1
	}
	return 0
}

func install(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var ids idList
	fs.Var(&ids, "extension-id", "allowed Chromium extension ID (repeatable, comma separated)")
	browser := fs.String("browser", "all", "chrome|edge|all")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err == nil {
		exe, err = filepath.Abs(exe)
	}
	if err != nil {
		fmt.Fprintln(stderr, "cannot resolve own path:", err)
		return 1
	}
	m, err := companion.BuildManifest(exe, ids)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	if runtime.GOOS == "windows" {
		return installWindows(m, *browser, stdout, stderr)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, "cannot find home directory:", err)
		return 1
	}
	dirs, err := companion.ManifestDirs(runtime.GOOS, home, *browser)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	for _, b := range sortedKeys(dirs) {
		p, err := companion.WriteManifest(dirs[b], m)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", b, err)
			return 1
		}
		fmt.Fprintf(stdout, "%s: wrote %s\n", b, p)
	}
	return 0
}

func installWindows(m companion.Manifest, browser string, stdout, stderr io.Writer) int {
	cfg, err := os.UserConfigDir()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	p, err := companion.WriteManifest(filepath.Join(cfg, "mypack", "native-messaging"), m)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, "Wrote manifest:", p)
	fmt.Fprintln(stdout, "Register it for the current user by running:")
	for _, b := range []string{"chrome", "edge"} {
		if browser == "all" || browser == "" || browser == b {
			fmt.Fprintf(stdout, "  reg add \"%s\" /ve /t REG_SZ /d \"%s\" /f\n", companion.RegistryKeys[b], p)
		}
	}
	return 0
}

func uninstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(stderr)
	browser := fs.String("browser", "all", "chrome|edge|all")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if runtime.GOOS == "windows" {
		cfg, _ := os.UserConfigDir()
		_, removed, _ := companion.RemoveManifest(filepath.Join(cfg, "mypack", "native-messaging"))
		fmt.Fprintln(stdout, "manifest removed:", removed)
		for _, b := range []string{"chrome", "edge"} {
			if *browser == "all" || *browser == b {
				fmt.Fprintf(stdout, "  reg delete \"%s\" /f\n", companion.RegistryKeys[b])
			}
		}
		return 0
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	dirs, err := companion.ManifestDirs(runtime.GOOS, home, *browser)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	for _, b := range sortedKeys(dirs) {
		p, removed, err := companion.RemoveManifest(dirs[b])
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", b, err)
			return 1
		}
		if removed {
			fmt.Fprintf(stdout, "%s: removed %s\n", b, p)
		} else {
			fmt.Fprintf(stdout, "%s: nothing to remove\n", b)
		}
	}
	return 0
}

func sortedKeys(m map[string]string) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	for i := range k {
		for j := i + 1; j < len(k); j++ {
			if k[j] < k[i] {
				k[i], k[j] = k[j], k[i]
			}
		}
	}
	return k
}
