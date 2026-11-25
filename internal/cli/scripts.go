package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/runtime/userscript"
)

// scriptsStore is the userscript store; tests set MYPACK_USERSCRIPTS_DIR.
func scriptsStore() userscript.Store { return userscript.Store{Dir: userscript.Dir()} }

const scriptsUsage = `usage: mypack scripts list
       mypack scripts install <url|file.user.js> [--app IDENTIFIER]...
       mypack scripts update <id>
       mypack scripts remove <id>
       mypack scripts enable|disable <id>
       mypack scripts scope <id> (--app IDENTIFIER... | --all)`

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// mypack scripts …: userscripts every app with runtime 1.0.12+ runs.
func (a *app) cmdScripts(args []string) int {
	var asJSON bool
	fs := a.flagSet("scripts", &asJSON)
	var apps multiFlag
	fs.Var(&apps, "app", "app identifier the script runs in (repeatable; default all apps)")
	all := fs.Bool("all", false, "run in all apps")
	pos, err := parse(fs, args)
	if err != nil || len(pos) == 0 {
		return a.usageErr(asJSON, "%s", scriptsUsage)
	}
	st := scriptsStore()
	ctx, cancel := signalContext()
	defer cancel()
	sub, rest := pos[0], pos[1:]
	one := func() (string, bool) {
		if len(rest) != 1 {
			return "", false
		}
		return rest[0], true
	}
	switch sub {
	case "list":
		ix, err := st.Read()
		if err != nil {
			return a.fail(err, asJSON, nil)
		}
		if asJSON {
			a.okJSON(map[string]any{"dir": st.Dir, "scripts": ix.Scripts})
			return ExitOK
		}
		if len(ix.Scripts) == 0 {
			fmt.Fprintf(a.out, "no userscripts installed (%s)\n", st.Dir)
			return ExitOK
		}
		rows := make([][]string, 0, len(ix.Scripts))
		for _, e := range ix.Scripts {
			state, scope := "on", "all apps"
			if !e.Enabled {
				state = "off"
			}
			if len(e.Apps) > 0 {
				scope = strings.Join(e.Apps, ", ")
			}
			rows = append(rows, []string{e.ID, e.Name, e.Version, state, scope})
		}
		a.table([]string{"ID", "NAME", "VERSION", "STATE", "APPS"}, rows)
		return ExitOK

	case "install":
		src, ok := one()
		if !ok {
			return a.usageErr(asJSON, "%s", scriptsUsage)
		}
		return a.installScript(ctx, st, src, apps, asJSON)

	case "update":
		id, ok := one()
		if !ok {
			return a.usageErr(asJSON, "%s", scriptsUsage)
		}
		ix, err := st.Read()
		if err != nil {
			return a.fail(err, asJSON, nil)
		}
		for _, e := range ix.Scripts {
			if e.ID == id {
				if e.SourceURL == "" {
					return a.fail(&domain.Error{Code: "INVALID_INPUT", Message: "This script was installed from a file.",
						Remediation: "Install the new file with `mypack scripts install <file>`."}, asJSON, nil)
				}
				return a.installScript(ctx, st, e.SourceURL, nil, asJSON)
			}
		}
		return a.fail(scriptNotFound(id), asJSON, nil)

	case "remove":
		id, ok := one()
		if !ok {
			return a.usageErr(asJSON, "%s", scriptsUsage)
		}
		if err := st.Remove(id); err != nil {
			return a.fail(scriptErr(id, err), asJSON, nil)
		}
		if asJSON {
			a.okJSON(map[string]any{"removed": id})
		} else {
			fmt.Fprintf(a.out, "removed %s\n", id)
		}
		return ExitOK

	case "enable", "disable", "scope":
		id, ok := one()
		if !ok {
			return a.usageErr(asJSON, "%s", scriptsUsage)
		}
		var e userscript.Entry
		switch sub {
		case "scope":
			if (len(apps) == 0) == !*all {
				return a.usageErr(asJSON, "scope needs --app IDENTIFIER (repeatable) or --all")
			}
			e, err = st.SetApps(id, []string(apps))
		default:
			e, err = st.SetEnabled(id, sub == "enable")
		}
		if err != nil {
			return a.fail(scriptErr(id, err), asJSON, nil)
		}
		if asJSON {
			a.okJSON(map[string]any{"script": e})
		} else {
			fmt.Fprintf(a.out, "%s: %s\n", id, describeScope(e))
		}
		fmt.Fprintln(a.errw, "mypack: running apps pick this up the next time they start")
		return ExitOK
	}
	return a.usageErr(asJSON, "%s", scriptsUsage)
}

func (a *app) installScript(ctx context.Context, st userscript.Store, src string, apps multiFlag, asJSON bool) int {
	var data []byte
	var sourceURL string
	if u, err := url.Parse(src); err == nil && (u.Scheme == "https" || u.Scheme == "http") {
		b, _, err := userscript.HTTPFetcher(nil)(ctx, src)
		if err != nil {
			return a.fail(&domain.Error{Code: "NETWORK_ERROR", Message: err.Error()}, asJSON, nil)
		}
		data, sourceURL = b, src
	} else {
		b, err := os.ReadFile(src)
		if err != nil {
			return a.fail(&domain.Error{Code: "INVALID_INPUT", Message: err.Error()}, asJSON, nil)
		}
		data = b
	}
	var scope []string
	if len(apps) > 0 {
		scope = apps
	}
	e, m, err := st.Install(ctx, data, userscript.InstallOptions{SourceURL: sourceURL, Apps: scope})
	if err != nil {
		return a.fail(&domain.Error{Code: "INVALID_INPUT", Message: err.Error()}, asJSON, nil)
	}
	if asJSON {
		a.okJSON(map[string]any{"script": e, "meta": m})
		return ExitOK
	}
	fmt.Fprintf(a.out, "installed %s %s as %s (%s)\n", e.Name, e.Version, e.ID, describeScope(e))
	if len(m.Match)+len(m.Include) > 0 {
		fmt.Fprintf(a.out, "  runs on: %s\n", strings.Join(append(append([]string{}, m.Match...), m.Include...), ", "))
	}
	if g := m.Grants(); g["xmlhttprequest"] {
		c := strings.Join(m.Connect, ", ")
		if c == "" {
			c = "the page's own site only"
		}
		fmt.Fprintf(a.out, "  can make requests to: %s\n", c)
	}
	fmt.Fprintln(a.errw, "mypack: apps with runtime 1.0.12 or later run it the next time they start")
	return ExitOK
}

func describeScope(e userscript.Entry) string {
	state := "on"
	if !e.Enabled {
		state = "off"
	}
	if len(e.Apps) == 0 {
		return state + ", all apps"
	}
	return state + ", apps: " + strings.Join(e.Apps, ", ")
}

func scriptNotFound(id string) error {
	return &domain.Error{Code: "NOT_FOUND", Message: fmt.Sprintf("No userscript %q is installed.", id),
		Remediation: "Run `mypack scripts list` to see the ids."}
}

func scriptErr(id string, err error) error {
	if errors.Is(err, userscript.ErrNotFound) {
		return scriptNotFound(id)
	}
	return err
}
