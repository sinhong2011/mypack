package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/mcp"
)

func (a *app) cmdInstall(args []string) int {
	var asJSON bool
	fs := a.flagSet("install", &asJSON)
	open := fs.Bool("open", false, "launch the app after installing")
	replace := fs.Bool("replace", false, "replace an existing app of the same name")
	pos, err := parse(fs, args)
	if err != nil || len(pos) != 1 {
		return a.usageErr(asJSON, "usage: mypack install <artifact-id> [--open] [--replace] [--json]")
	}
	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	in, err := svc.InstallAndOpen(context.Background(), appservice.InstallRequest{
		ArtifactID: pos[0], Replace: *replace, Open: *open, Actor: "cli"})
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	if asJSON {
		a.okJSON(map[string]any{"installation": in})
	} else {
		fmt.Fprintf(a.out, "installed %s at %s\n", in.Name, in.Path)
	}
	return ExitOK
}

func (a *app) cmdApps(args []string) int {
	var asJSON bool
	fs := a.flagSet("apps", &asJSON)
	pos, err := parse(fs, args)
	if err != nil || len(pos) == 0 || (pos[0] == "list" && len(pos) != 1) || (pos[0] == "locate" && len(pos) != 3) ||
		(pos[0] != "list" && pos[0] != "locate") {
		return a.usageErr(asJSON, "usage: mypack apps list [--json] | apps locate <id> <path> [--json]")
	}
	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	if pos[0] == "locate" {
		in, err := svc.RelocateInstallation(context.Background(), pos[1], pos[2], "cli")
		if err != nil {
			return a.fail(err, asJSON, nil)
		}
		if asJSON {
			a.okJSON(map[string]any{"installation": in})
		} else {
			fmt.Fprintf(a.out, "reconnected %s at %s\n", in.Name, in.Path)
		}
		return ExitOK
	}
	l, err := svc.ListInstallations(context.Background())
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	if asJSON {
		if l == nil {
			l = []domain.Installation{}
		}
		a.okJSON(map[string]any{"apps": l})
		return ExitOK
	}
	rows := make([][]string, 0, len(l))
	for _, in := range l {
		upd := ""
		if in.UpdateReason != "" {
			upd = "update available"
		}
		rows = append(rows, []string{in.ID, in.Name, string(in.State), in.Version, in.Path, upd})
	}
	a.table([]string{"ID", "NAME", "STATE", "VERSION", "PATH", ""}, rows)
	return ExitOK
}

func (a *app) cmdSettings(args []string) int {
	var asJSON bool
	fs := a.flagSet("settings", &asJSON)
	pos, err := parse(fs, args)
	if err != nil || len(pos) == 0 {
		return a.usageErr(asJSON, "usage: mypack settings get [key] | settings set <key> <value>")
	}
	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	ctx := context.Background()
	cur, err := svc.Settings(ctx)
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	m := settingsMap(cur)

	switch pos[0] {
	case "get":
		if len(pos) == 1 {
			if asJSON {
				a.okJSON(map[string]any{"settings": m})
				return ExitOK
			}
			keys := sortedKeys(m)
			for _, k := range keys {
				fmt.Fprintf(a.out, "%s = %v\n", k, m[k])
			}
			return ExitOK
		}
		v, ok := m[pos[1]]
		if !ok {
			return a.fail(&domain.Error{Code: "INVALID_INPUT", Message: "Unknown setting " + pos[1],
				Remediation: "Known settings: " + strings.Join(sortedKeys(m), ", ")}, asJSON, nil)
		}
		if asJSON {
			a.okJSON(map[string]any{"key": pos[1], "value": v})
		} else {
			fmt.Fprintln(a.out, v)
		}
		return ExitOK
	case "set":
		if len(pos) != 3 {
			return a.usageErr(asJSON, "usage: mypack settings set <key> <value>")
		}
		old, ok := m[pos[1]]
		if !ok {
			return a.fail(&domain.Error{Code: "INVALID_INPUT", Message: "Unknown setting " + pos[1],
				Remediation: "Known settings: " + strings.Join(sortedKeys(m), ", ")}, asJSON, nil)
		}
		var val any = pos[2]
		if _, isBool := old.(bool); isBool {
			b, perr := strconv.ParseBool(pos[2])
			if perr != nil {
				return a.fail(&domain.Error{Code: "INVALID_INPUT", Message: pos[1] + " must be true or false"}, asJSON, nil)
			}
			val = b
		}
		m[pos[1]] = val
		b, _ := json.Marshal(m)
		next := cur
		if err := json.Unmarshal(b, &next); err != nil {
			return a.fail(err, asJSON, nil)
		}
		if err := svc.SaveSettings(ctx, next); err != nil {
			return a.fail(err, asJSON, nil)
		}
		if asJSON {
			a.okJSON(map[string]any{"key": pos[1], "value": val})
		} else {
			fmt.Fprintf(a.out, "%s = %v\n", pos[1], val)
		}
		return ExitOK
	}
	return a.usageErr(asJSON, "unknown settings command %q", pos[0])
}

// settingsMap flattens Settings into a name -> value map using JSON names.
func settingsMap(s appservice.Settings) map[string]any {
	b, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// small, fixed set: insertion sort keeps imports minimal
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func (a *app) cmdMCP(args []string) int {
	if len(args) == 0 || args[0] != "serve" {
		return a.usageErr(false, "usage: mypack mcp serve [--transport stdio]")
	}
	fs := a.flagSet("mcp serve", nil)
	transport := fs.String("transport", "stdio", "transport (only stdio is supported)")
	if _, err := parse(fs, args[1:]); err != nil {
		return a.usageErr(false, "%v", err)
	}
	if *transport != "stdio" {
		return a.usageErr(false, "unsupported transport %q: only stdio is available", *transport)
	}
	svc, err := a.service()
	if err != nil {
		fmt.Fprintf(a.errw, "mypack: cannot open state: %v\n", err)
		return ExitGeneric
	}
	ctx, stop := signalContext()
	defer stop()
	fmt.Fprintln(a.errw, "mypack MCP server listening on stdio")
	srv := mcp.NewServer(svc, mcp.ServerInfo{Name: "mypack", Version: Version})
	if err := srv.Serve(ctx, a.d.Stdin, a.out); err != nil && ctx.Err() == nil {
		fmt.Fprintf(a.errw, "mypack: mcp: %v\n", err)
		return ExitGeneric
	}
	return ExitOK
}

func (a *app) cmdLicense(args []string) int {
	var asJSON bool
	fs := a.flagSet("license", &asJSON)
	pos, err := parse(fs, args)
	if err != nil || len(pos) == 0 {
		return a.usageErr(asJSON, "usage: mypack license status | activate <token|-> | deactivate [--json]")
	}
	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	ctx := context.Background()
	var st appservice.LicenseStatus
	switch pos[0] {
	case "status":
		if len(pos) != 1 {
			return a.usageErr(asJSON, "usage: mypack license status [--json]")
		}
		st, err = svc.License(ctx)
	case "activate":
		if len(pos) != 2 {
			return a.usageErr(asJSON, "usage: mypack license activate <token|-> [--json]")
		}
		tok := pos[1]
		if tok == "-" {
			b, rerr := io.ReadAll(io.LimitReader(a.d.Stdin, 64<<10))
			if rerr != nil {
				return a.fail(rerr, asJSON, nil)
			}
			tok = string(b)
		}
		st, err = svc.ActivateLicense(ctx, tok, "cli")
	case "deactivate":
		if len(pos) != 1 {
			return a.usageErr(asJSON, "usage: mypack license deactivate [--json]")
		}
		if err = svc.DeactivateLicense(ctx, "cli"); err == nil {
			st, err = svc.License(ctx)
		}
	default:
		return a.usageErr(asJSON, "usage: mypack license status | activate <token|-> | deactivate [--json]")
	}
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	if asJSON {
		a.okJSON(map[string]any{"license": st})
		return ExitOK
	}
	if st.Valid {
		fmt.Fprintf(a.out, "Edition: Pro (unlimited managed apps)\n")
		if st.Email != "" {
			fmt.Fprintf(a.out, "Licensed to: %s\n", st.Email)
		}
		fmt.Fprintf(a.out, "License ID: %s\n", st.LicenseID)
	} else {
		fmt.Fprintf(a.out, "Edition: Free (%d of %d managed apps)\n", st.ManagedApps, st.FreeLimit)
		if st.Error != "" {
			fmt.Fprintf(a.out, "Saved license is not valid: %s\n", st.Error)
		}
	}
	return ExitOK
}
