package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"mypack.dev/mypack/internal/appservice"
)

// mypack export [<id>…] -o file.mypack: apps (all when none) with their icons.
func (a *app) cmdExport(args []string) int {
	var asJSON bool
	fs := a.flagSet("export", &asJSON)
	out := fs.String("o", "", "the .mypack file to write")
	pos, err := parse(fs, args)
	if err != nil || *out == "" {
		return a.usageErr(asJSON, "usage: mypack export [<project-id>...] -o <file.mypack> [--json]")
	}
	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	path, _ := filepath.Abs(*out)
	if !strings.HasSuffix(strings.ToLower(path), appservice.LibraryExt) {
		path += appservice.LibraryExt
	}
	n, err := svc.ExportLibrary(context.Background(), pos, path)
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	if asJSON {
		a.okJSON(map[string]any{"path": path, "apps": n})
	} else {
		fmt.Fprintf(a.out, "wrote %d app(s) to %s\n", n, path)
	}
	return ExitOK
}

// mypack import <file.mypack> [--replace]
func (a *app) cmdImport(args []string) int {
	var asJSON bool
	fs := a.flagSet("import", &asJSON)
	replace := fs.Bool("replace", false, "replace apps already in the Library")
	pos, err := parse(fs, args)
	if err != nil || len(pos) != 1 {
		return a.usageErr(asJSON, "usage: mypack import <file.mypack> [--replace] [--json]")
	}
	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	res, err := svc.ImportLibrary(context.Background(), pos[0], appservice.ImportOptions{Replace: *replace, Actor: "cli"})
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	if asJSON {
		a.okJSON(map[string]any{"result": res})
		return ExitOK
	}
	fmt.Fprintf(a.out, "added %d, replaced %d, skipped %d\n", len(res.Added), len(res.Updated), len(res.Skipped))
	if len(res.Skipped) > 0 {
		fmt.Fprintf(a.out, "already in the Library (use --replace): %s\n", strings.Join(res.Skipped, ", "))
	}
	for _, s := range append(res.Refused, res.Warnings...) {
		fmt.Fprintf(a.errw, "mypack: %s\n", s)
	}
	return ExitOK
}

// mypack sync: sync the Library with the sync folder now (Pro).
func (a *app) cmdSync(args []string) int {
	var asJSON bool
	fs := a.flagSet("sync", &asJSON)
	if _, err := parse(fs, args); err != nil {
		return a.usageErr(asJSON, "usage: mypack sync [--json]")
	}
	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	res, err := svc.SyncLibrary(context.Background())
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	if asJSON {
		a.okJSON(map[string]any{"result": res})
		return ExitOK
	}
	fmt.Fprintf(a.out, "synced %d app(s) with %s: %d sent, %d received, %d moved to the Trash\n", res.Apps, res.Folder, res.Pushed, res.Pulled, res.Trashed)
	for _, e := range res.Errors {
		fmt.Fprintf(a.errw, "mypack: %s\n", e)
	}
	return ExitOK
}
