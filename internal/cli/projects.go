package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// projectView is the JSON shape of a project in CLI output.
func projectView(p domain.Project) map[string]any {
	return map[string]any{
		"id": p.ID, "name": p.Config.App.Name, "identifier": p.Config.App.Identifier,
		"source": p.Config.App.Source, "revision": p.Revision, "draft": p.Draft, "origin": p.Origin,
		"updatedAt": p.UpdatedAt, "lastBuild": p.LastBuild, "config": p.Config,
	}
}

func (a *app) cmdCreate(args []string) int {
	var asJSON bool
	fs := a.flagSet("create", &asJSON)
	name := fs.String("name", "", "app name")
	pos, err := parse(fs, args)
	if err != nil {
		return a.usageErr(asJSON, "%v", err)
	}
	if len(pos) != 1 {
		return a.usageErr(asJSON, "usage: mypack create <url|path> [--name NAME] [--json]")
	}
	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	ctx, stop := signalContext()
	defer stop()
	if !asJSON {
		fmt.Fprintf(a.errw, "Creating project from %s ...\n", pos[0])
	}
	p, err := svc.CreateProject(ctx, appservice.CreateRequest{Input: pos[0], Name: *name, Actor: "cli"})
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	if asJSON {
		a.okJSON(map[string]any{"project": projectView(p)})
		return ExitOK
	}
	fmt.Fprintf(a.out, "%s  %s  (draft, revision %d)\n", p.ID, p.Config.App.Name, p.Revision)
	fmt.Fprintf(a.errw, "Next: mypack build %s\n", p.ID)
	return ExitOK
}

func (a *app) cmdProject(args []string) int {
	if len(args) == 0 {
		return a.usageErr(a.globalJSON, "usage: mypack project list|get|validate|export")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return a.projectList(rest)
	case "get":
		return a.projectGet(rest)
	case "validate":
		return a.projectValidate(rest)
	case "export":
		return a.projectExport(rest)
	}
	return a.usageErr(a.globalJSON, "unknown project command %q", sub)
}

func (a *app) projectList(args []string) int {
	var asJSON bool
	fs := a.flagSet("project list", &asJSON)
	if _, err := parse(fs, args); err != nil {
		return a.usageErr(asJSON, "%v", err)
	}
	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	ps, err := svc.ListProjects(context.Background())
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	if asJSON {
		views := make([]map[string]any, 0, len(ps))
		for _, p := range ps {
			v := projectView(p)
			delete(v, "config") // list stays compact; use `project get` for the config
			views = append(views, v)
		}
		a.okJSON(map[string]any{"projects": views})
		return ExitOK
	}
	rows := make([][]string, 0, len(ps))
	for _, p := range ps {
		state := "built"
		if p.Draft {
			state = "draft"
		}
		rows = append(rows, []string{p.ID, p.Config.App.Name, p.Config.App.Source.Value, fmt.Sprint(p.Revision), state})
	}
	a.table([]string{"ID", "NAME", "SOURCE", "REV", "STATE"}, rows)
	return ExitOK
}

func (a *app) projectGet(args []string) int {
	var asJSON bool
	fs := a.flagSet("project get", &asJSON)
	pos, err := parse(fs, args)
	if err != nil || len(pos) != 1 {
		return a.usageErr(asJSON, "usage: mypack project get <id> [--json]")
	}
	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	p, err := svc.GetProject(context.Background(), pos[0])
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	if asJSON {
		a.okJSON(map[string]any{"project": projectView(p)})
		return ExitOK
	}
	b, _ := json.MarshalIndent(p.Config, "", "  ")
	fmt.Fprintf(a.out, "# %s revision %d (%s)\n%s\n", p.ID, p.Revision, p.Origin, b)
	return ExitOK
}

func (a *app) projectValidate(args []string) int {
	var asJSON bool
	fs := a.flagSet("project validate", &asJSON)
	pos, err := parse(fs, args)
	if err != nil || len(pos) != 1 {
		return a.usageErr(asJSON, "usage: mypack project validate <path|id> [--json]")
	}
	var cfg *config.Config
	if st, serr := os.Stat(pos[0]); serr == nil && !st.IsDir() {
		c, warns, lerr := appservice.LoadConfigFile(pos[0])
		if lerr != nil {
			return a.fail(lerr, asJSON, map[string]any{"valid": false})
		}
		for _, w := range warns {
			if !asJSON {
				fmt.Fprintf(a.errw, "warning: %s\n", w)
			}
		}
		cfg = c
		host := config.Target{OS: goos(), Arch: goarch()}
		if svc, serr := a.service(); serr == nil {
			host = svc.HostTarget()
		}
		cfg.ApplyDefaults(host)
	} else {
		svc, serr := a.service()
		if serr != nil {
			return a.fail(serr, asJSON, nil)
		}
		p, gerr := svc.GetProject(context.Background(), pos[0])
		if gerr != nil {
			return a.fail(gerr, asJSON, nil)
		}
		cfg = p.Config
	}
	if verr := cfg.Validate(); verr != nil {
		de := &domain.Error{Code: "INVALID_CONFIG", Message: verr.Error(), Remediation: "Fix the listed fields."}
		if ve, ok := verr.(*config.ValidationError); ok {
			b, _ := json.Marshal(ve.Issues)
			de.Cause = string(b)
			if len(ve.Issues) > 0 {
				de.Message = fmt.Sprintf("The configuration is not valid (%d issue(s)).", len(ve.Issues))
			}
		}
		return a.fail(de, asJSON, map[string]any{"valid": false})
	}
	if asJSON {
		a.okJSON(map[string]any{"valid": true, "name": cfg.App.Name})
	} else {
		fmt.Fprintf(a.out, "valid: %s\n", cfg.App.Name)
	}
	return ExitOK
}

func (a *app) projectExport(args []string) int {
	var asJSON bool
	fs := a.flagSet("project export", &asJSON)
	pos, err := parse(fs, args)
	if err != nil || len(pos) != 2 {
		return a.usageErr(asJSON, "usage: mypack project export <id> <path> [--json]")
	}
	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	path, _ := filepath.Abs(pos[1])
	if err := svc.ExportProject(context.Background(), pos[0], path); err != nil {
		return a.fail(err, asJSON, nil)
	}
	if asJSON {
		a.okJSON(map[string]any{"path": path})
	} else {
		fmt.Fprintf(a.out, "wrote %s\n", path)
	}
	return ExitOK
}

func (a *app) cmdImportPake(args []string) int {
	var asJSON bool
	fs := a.flagSet("import-pake", &asJSON)
	outPath := fs.String("out", "", "also write the converted mypack.json here")
	pos, err := parse(fs, args)
	if err != nil || len(pos) != 1 {
		return a.usageErr(asJSON, "usage: mypack import-pake <pake.json> [--out mypack.json] [--json]")
	}
	b, err := os.ReadFile(pos[0])
	if err != nil {
		return a.fail(&domain.Error{Code: "INVALID_INPUT", Message: "Cannot read " + pos[0], Cause: err.Error()}, asJSON, nil)
	}
	if !config.IsPakeJSON(b) {
		return a.fail(&domain.Error{Code: "INVALID_INPUT", Message: pos[0] + " does not look like a Pake config.",
			Remediation: "Use `mypack create` for mypack.json files."}, asJSON, nil)
	}
	_, warnings, err := config.ImportPake(b)
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	svc, err := a.service()
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	p, err := svc.CreateProject(context.Background(), appservice.CreateRequest{Input: pos[0], Actor: "cli"})
	if err != nil {
		return a.fail(err, asJSON, nil)
	}
	if *outPath != "" {
		if err := svc.ExportProject(context.Background(), p.ID, *outPath); err != nil {
			return a.fail(err, asJSON, nil)
		}
	}
	if warnings == nil {
		warnings = []string{}
	}
	if asJSON {
		a.okJSON(map[string]any{"project": projectView(p), "warnings": warnings})
		return ExitOK
	}
	fmt.Fprintf(a.out, "%s  %s  (imported from Pake)\n", p.ID, p.Config.App.Name)
	for _, w := range warnings {
		fmt.Fprintf(a.errw, "warning: %s\n", w)
	}
	return ExitOK
}

// ensureProjectForFile returns the project backing a mypack.json given to
// `build`. The file is the source of truth: the CLI keeps one project per
// file identity (origin "cli"), updating it when the file changed, so
// repeated builds do not pile up duplicates.
func (a *app) ensureProjectForFile(ctx context.Context, svc appservice.Service, path string) (domain.Project, error) {
	cfg, _, err := appservice.LoadConfigFile(path)
	if err != nil {
		return domain.Project{}, err
	}
	cfg.ApplyDefaults(svc.HostTarget())
	if verr := cfg.Validate(); verr != nil {
		var ve *config.ValidationError
		if asVE(verr, &ve) {
			b, _ := json.Marshal(ve.Issues)
			return domain.Project{}, &domain.Error{Code: "INVALID_CONFIG", Message: "The configuration is not valid.",
				Cause: string(b), Remediation: "Fix the listed fields."}
		}
		return domain.Project{}, verr
	}
	projects, err := svc.ListProjects(ctx)
	if err != nil {
		return domain.Project{}, err
	}
	for _, p := range projects {
		if p.Origin == "cli" && p.Config.App.Identifier == cfg.App.Identifier {
			if sameConfig(p.Config, cfg) {
				return p, nil
			}
			return svc.UpdateProject(ctx, appservice.UpdateRequest{
				ID: p.ID, ExpectedRevision: p.Revision, Config: cfg, Actor: "cli"})
		}
	}
	return svc.CreateProject(ctx, appservice.CreateRequest{Input: path, Actor: "cli"})
}

func asVE(err error, target **config.ValidationError) bool {
	ve, ok := err.(*config.ValidationError)
	if ok {
		*target = ve
	}
	return ok
}

func sameConfig(a, b *config.Config) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func looksLikeConfigFile(arg string) bool {
	if strings.HasPrefix(arg, "prj_") {
		return false
	}
	if st, err := os.Stat(arg); err == nil && !st.IsDir() {
		return true
	}
	return strings.HasSuffix(strings.ToLower(arg), ".json")
}
