// Command mypack is the Builder: with a subcommand it runs the CLI (same
// code as mypack-cli); without one it opens the native GUI.
//
//	mypack                 open the GUI
//	mypack build …         CLI (see `mypack help`)
//	MYPACK_FAKE=1 mypack   GUI against the in-memory demo service
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/egoist/mygo"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/cli"
	"mypack.dev/mypack/internal/desktop/app"
	"mypack.dev/mypack/internal/desktop/fakesvc"
)

func main() {
	useAppVersion()
	if isCLI(os.Args[1:]) {
		os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
	}
	var svc appservice.Service
	if os.Getenv("MYPACK_FAKE") == "1" {
		svc = fakesvc.New()
	} else {
		s, err := appservice.New(appservice.Options{
			DBPath:   os.Getenv("MYPACK_DB"),
			CacheDir: os.Getenv("MYPACK_CACHE_DIR"),
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "mypack:", err)
			os.Exit(1)
		}
		svc = s
	}
	defer svc.Close()
	if err := app.Run(svc); err != nil {
		fmt.Fprintln(os.Stderr, "mypack:", err)
		os.Exit(1)
	}
}

// isCLI reports whether the arguments ask for the CLI. macOS may pass
// "-psn_…" when launched from Finder; that, and no args, mean GUI.
func isCLI(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "-psn_") || strings.HasPrefix(a, "-NS") || strings.HasPrefix(a, "-Apple") {
			continue
		}
		if a == "--show-project" || strings.HasPrefix(a, "mypack://") {
			return false // GUI launch intents (browser companion, deep links)
		}
		return true
	}
	return false
}

// useAppVersion takes the version mygo build links into the app (mygo.json,
// set from the release tag) when no -ldflags -X stamped one: the installers
// are built by mygo build, which takes no ldflags of ours.
func useAppVersion() {
	if !strings.HasSuffix(cli.Version, "-dev") {
		return
	}
	if v := mygo.App.Version(); v != "" && !strings.HasSuffix(v, "-dev") {
		cli.Version, app.Version = v, v
	}
}
