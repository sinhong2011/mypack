package cli

import (
	"fmt"
	"runtime"
)

func (a *app) cmdVersion(args []string) int {
	var asJSON bool
	fs := a.flagSet("version", &asJSON)
	if _, err := parse(fs, args); err != nil {
		return a.usageErr(asJSON, "%v", err)
	}
	if asJSON {
		a.okJSON(map[string]any{"version": Version, "os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version()})
	} else {
		fmt.Fprintf(a.out, "mypack %s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)
	}
	return ExitOK
}
