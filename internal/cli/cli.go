// Package cli implements the mypack command line (spec §11): the same
// Application Service the GUI uses, driven from a terminal or an agent.
//
// Output contract:
//   - With --json, stdout receives exactly one JSON object
//     {"ok":true,...} or {"ok":false,"error":{code,message,remediation}} and
//     nothing else; human-readable logs and progress go to stderr (and
//     progress is suppressed entirely).
//   - Without --json, results go to stdout, progress and diagnostics to stderr.
//   - The CLI never prompts: a missing runtime without --yes exits 6 with
//     RUNTIME_DOWNLOAD_REQUIRED.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/domain"
)

// Version is the CLI/application version (overridable with -ldflags -X).
var Version = "0.1.0-dev"

// Stable exit codes (ARCHITECTURE.md "Error contract").
const (
	ExitOK          = 0
	ExitGeneric     = 1
	ExitUsage       = 2
	ExitValidation  = 3
	ExitRuntime     = 4 // runtime / network
	ExitBuildFailed = 5
	ExitApproval    = 6 // approval required (e.g. runtime download without --yes)
	ExitCancelled   = 130
)

// Deps are the CLI's external dependencies, replaceable in tests.
type Deps struct {
	// NewService opens the Application Service (default: appservice.New with
	// MYPACK_DB / MYPACK_CACHE_DIR / MYPACK_REGISTRY from the environment).
	NewService func() (appservice.Service, error)
	Stdin      io.Reader
}

// Run executes the CLI and returns the process exit code. It is exported so
// the GUI binary can delegate when started with arguments.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunWith(Deps{}, args, stdout, stderr)
}

// RunWith is Run with explicit dependencies.
func RunWith(d Deps, args []string, stdout, stderr io.Writer) int {
	if d.NewService == nil {
		d.NewService = defaultService
	}
	if d.Stdin == nil {
		d.Stdin = os.Stdin
	}
	a := &app{d: d, out: stdout, errw: stderr}
	defer a.close()

	// A leading --json applies to the whole invocation.
	for len(args) > 0 && (args[0] == "--json" || args[0] == "-json") {
		a.globalJSON = true
		args = args[1:]
	}
	if len(args) == 0 {
		a.usage(stderr)
		return ExitUsage
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "help", "-h", "--help":
		a.usage(stdout)
		return ExitOK
	case "version", "--version", "-v":
		return a.cmdVersion(rest)
	case "create":
		return a.cmdCreate(rest)
	case "build":
		return a.cmdBuild(rest)
	case "project":
		return a.cmdProject(rest)
	case "runtime":
		return a.cmdRuntime(rest)
	case "import-config", "import-pake": // import-pake: compatibility alias
		return a.cmdImportConfig(cmd, rest)
	case "export":
		return a.cmdExport(rest)
	case "import":
		return a.cmdImport(rest)
	case "sync":
		return a.cmdSync(rest)
	case "install":
		return a.cmdInstall(rest)
	case "apps":
		return a.cmdApps(rest)
	case "settings":
		return a.cmdSettings(rest)
	case "license":
		return a.cmdLicense(rest)
	case "mcp":
		return a.cmdMCP(rest)
	}
	fmt.Fprintf(stderr, "mypack: unknown command %q\n\n", cmd)
	a.usage(stderr)
	return ExitUsage
}

func defaultService() (appservice.Service, error) {
	return appservice.New(appservice.Options{
		DBPath:   os.Getenv("MYPACK_DB"),
		CacheDir: os.Getenv("MYPACK_CACHE_DIR"),
	})
}

type app struct {
	d          Deps
	out, errw  io.Writer
	globalJSON bool

	svc appservice.Service
}

const usageText = `mypack - turn websites into desktop apps

Usage:
  mypack create <url|path> [--name NAME] [--json]
  mypack build <project-id|mypack.json> [--target darwin-arm64[,..]] [--format app]
               [--output DIR] [--yes] [--json]
  mypack project list|get <id>|validate <path|id>|export <id> <path>
  mypack runtime list|download [key] [--version V]|verify [key]|clean [--all]
  mypack import-config <mypack.json|pake.json> [--out mypack.json]
  mypack export [<project-id>...] -o <file.mypack>   (all apps when no id)
  mypack import <file.mypack> [--replace]
  mypack sync                                        (Pro: sync with the sync folder)
  mypack install <artifact-id> [--open] [--replace]
  mypack apps list | apps locate <id> <path>
  mypack license status | activate <token|-> | deactivate
  mypack settings get [key] | settings set <key> <value>
  mypack mcp serve [--transport stdio]
  mypack version

Flags:
  --json   print a single JSON object on stdout; logs go to stderr
  --yes    allow downloading a missing runtime (otherwise exit code 6)

Exit codes: 0 ok, 1 error, 2 usage, 3 invalid input/config, 4 runtime or network,
            5 build failed, 6 approval required, 130 cancelled.
`

func (a *app) usage(w io.Writer) { fmt.Fprint(w, usageText) }

// service lazily opens the service; RunWith closes it on return.
func (a *app) service() (appservice.Service, error) {
	if a.svc != nil {
		return a.svc, nil
	}
	s, err := a.d.NewService()
	if err != nil {
		return nil, err
	}
	a.svc = s
	return s, nil
}

func (a *app) close() {
	if a.svc != nil {
		_ = a.svc.Close()
		a.svc = nil
	}
}

// signalContext is cancelled on Ctrl-C / SIGTERM.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// ---- flags ---------------------------------------------------------------------

// flagSet creates a FlagSet that reports errors itself (we format them).
func (a *app) flagSet(name string, jsonFlag *bool) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if jsonFlag != nil {
		fs.BoolVar(jsonFlag, "json", a.globalJSON, "print a single JSON object")
	}
	return fs
}

// parse parses flags that may appear before, between or after positionals.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if len(arg) > 1 && arg[0] == '-' {
			flags = append(flags, arg)
			name := strings.TrimLeft(arg, "-")
			if strings.Contains(name, "=") {
				continue
			}
			if f := fs.Lookup(name); f != nil {
				if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
					continue
				}
				if i+1 < len(args) {
					i++
					flags = append(flags, args[i])
				}
			}
			continue
		}
		pos = append(pos, arg)
	}
	if err := fs.Parse(flags); err != nil {
		return nil, err
	}
	return pos, nil
}

// usageErr reports a usage problem (exit 2), as JSON when requested.
func (a *app) usageErr(asJSON bool, format string, args ...any) int {
	msg := fmt.Sprintf(format, args...)
	if asJSON {
		a.writeJSON(map[string]any{"ok": false, "error": map[string]any{
			"code": "USAGE", "message": msg, "remediation": "Run `mypack help` for usage."}})
	} else {
		fmt.Fprintf(a.errw, "mypack: %s\nRun `mypack help` for usage.\n", msg)
	}
	return ExitUsage
}

// ---- output --------------------------------------------------------------------

func (a *app) writeJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		b = []byte(`{"ok":false,"error":{"code":"INTERNAL","message":"cannot encode output"}}`)
	}
	fmt.Fprintf(a.out, "%s\n", b)
}

// okJSON prints {"ok":true, ...fields}.
func (a *app) okJSON(fields map[string]any) {
	fields["ok"] = true
	a.writeJSON(fields)
}

func (a *app) table(header []string, rows [][]string) {
	tw := tabwriter.NewWriter(a.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	_ = tw.Flush()
}

// fail prints err in the requested format and returns its exit code.
func (a *app) fail(err error, asJSON bool, extra map[string]any) int {
	de := toDomainError(err)
	code := exitCodeFor(de)
	if asJSON {
		out := map[string]any{"ok": false, "error": errorObject(de)}
		for k, v := range extra {
			out[k] = v
		}
		if is := appservice.ConfigIssues(de); len(is) > 0 {
			out["issues"] = is
		}
		a.writeJSON(out)
		return code
	}
	fmt.Fprintf(a.errw, "mypack: %s: %s\n", de.Code, de.Message)
	for _, is := range appservice.ConfigIssues(de) {
		fmt.Fprintf(a.errw, "  - %s: %s\n", is.Field, is.Message)
	}
	if de.Remediation != "" {
		fmt.Fprintf(a.errw, "  %s\n", de.Remediation)
	}
	return code
}

func errorObject(de *domain.Error) map[string]any {
	m := map[string]any{"code": de.Code, "message": de.Message}
	if de.Remediation != "" {
		m["remediation"] = de.Remediation
	}
	if de.Field != "" {
		m["field"] = de.Field
	}
	return m
}

func toDomainError(err error) *domain.Error {
	var de *domain.Error
	if errors.As(err, &de) {
		return de
	}
	if errors.Is(err, context.Canceled) {
		return &domain.Error{Code: "BUILD_CANCELLED", Message: "Cancelled."}
	}
	return &domain.Error{Code: "ERROR", Message: err.Error()}
}

// exitCodeFor maps a stable error code to a stable exit code.
func exitCodeFor(de *domain.Error) int {
	switch de.Code {
	case "INVALID_CONFIG", "INVALID_INPUT", "INVALID_ARGUMENTS", "IDENTITY_CHANGE_REQUIRES_CONFIRMATION",
		"REVISION_CONFLICT", "TARGET_UNSUPPORTED", "USAGE", "LICENSE_INVALID", "NOT_SAME_APP":
		return ExitValidation
	case "RUNTIME_DOWNLOAD_REQUIRED", "APPROVAL_REQUIRED", "PROJECT_BUILD_REQUIRED":
		return ExitApproval
	case "BUILD_CANCELLED":
		return ExitCancelled
	case "BUILD_FAILED", "PROJECT_BUILD_FAILED", "PROJECT_OUTPUT_MISSING", "NODE_NOT_FOUND", "SIGNING_FAILED", "ASSEMBLY_FAILED", "PACKAGING_FAILED", "VERIFY_FAILED", "INTERRUPTED":
		return ExitBuildFailed
	}
	if strings.HasPrefix(de.Code, "RUNTIME_") || strings.HasPrefix(de.Code, "NETWORK") {
		return ExitRuntime
	}
	return ExitGeneric
}
