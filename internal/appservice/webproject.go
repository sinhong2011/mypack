package appservice

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"mypack.dev/mypack/pkg/config"
)

// webProject describes a JavaScript web project folder (one with a
// package.json). mypack builds it (after approval) and packages the output,
// since a Vite root index.html loads /src/main.tsx and would package as a
// blank page.
type webProject struct {
	Name           string // productName / name from package.json
	Framework      string // "Next.js", "Vite", "Nuxt", …
	BuildCmd       string // how to build it (for messages)
	OutDir         string // output folder relative to the project
	Static         bool   // false: needs a server (SSR), cannot be bundled
	PackageManager string // npm | pnpm | yarn | bun
	Script         string // package.json script that builds it; "" = none
}

// outputDirs are checked in order for a built index.html.
var outputDirs = []string{"dist", "build", "out", ".output/public", "www", "_site", "public"}

// resolveWebProject inspects dir. ok=false means dir is not a JS project (use
// it as is). For a project, site is the folder that holds (or will hold) the
// built output and proj.Script is the build script to run (empty when the
// project has no usable script and is already built). An error explains why
// the project cannot be packaged statically (Next.js SSR) or has nothing to
// build and nothing built.
func resolveWebProject(dir string) (site string, proj webProject, ok bool, err error) {
	b, rerr := os.ReadFile(filepath.Join(dir, "package.json"))
	if rerr != nil {
		return dir, proj, false, nil
	}
	var pkg struct {
		Name            string            `json:"name"`
		ProductName     string            `json:"productName"`
		PackageManager  string            `json:"packageManager"`
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	_ = json.Unmarshal(b, &pkg)
	proj = detectFramework(dir, pkg.Dependencies, pkg.DevDependencies)
	proj.Name = firstNonEmpty(pkg.ProductName, prettyPkgName(pkg.Name))
	proj.PackageManager = detectPackageManager(dir, pkg.PackageManager)
	if proj.Framework == "Angular" {
		if out := angularOutDir(dir); out != "" {
			proj.OutDir = out
		}
	}
	if _, has := pkg.Scripts["build"]; has {
		proj.Script = "build"
	}
	if proj.Framework == "Nuxt" {
		if _, has := pkg.Scripts["generate"]; has {
			proj.Script = "generate"
		}
	}

	if !proj.Static {
		return "", proj, true, newErr(CodeInvalidInput,
			proj.Framework+" with server-side rendering can't be packaged as a static app.",
			proj.BuildCmd)
	}
	// An existing build in another conventional folder wins over the guess.
	if !fileExists(filepath.Join(dir, filepath.FromSlash(proj.OutDir), "index.html")) {
		for _, d := range outputDirs {
			if d == "public" && proj.Framework != "" && proj.Framework != "Static site" {
				continue // a framework's public/ holds unbuilt templates
			}
			if fileExists(filepath.Join(dir, filepath.FromSlash(d), "index.html")) {
				proj.OutDir = d
				break
			}
		}
	}
	site = filepath.Join(dir, filepath.FromSlash(proj.OutDir))
	if proj.Script == "" && !fileExists(filepath.Join(site, "index.html")) {
		return "", proj, true, newErr(CodeInvalidInput,
			"This "+proj.Framework+" project has no \"build\" script and no built output.",
			"Add a build script to package.json (or build the project yourself), then drop it here again.")
	}
	return site, proj, true, nil
}

// detectPackageManager prefers lockfiles, then package.json's packageManager.
func detectPackageManager(dir, declared string) string {
	for _, c := range []struct{ file, pm string }{
		{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lock", "bun"}, {"bun.lockb", "bun"},
		{"package-lock.json", "npm"}, {"npm-shrinkwrap.json", "npm"},
	} {
		if fileExists(filepath.Join(dir, c.file)) {
			return c.pm
		}
	}
	if name, _, _ := strings.Cut(declared, "@"); name == "pnpm" || name == "yarn" || name == "bun" || name == "npm" {
		return name
	}
	return "npm"
}

// angularOutDir reads angular.json's build outputPath when it is simple.
// The newer "application" builder emits to <outputPath>/browser.
func angularOutDir(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, "angular.json"))
	if err != nil {
		return ""
	}
	var ng struct {
		DefaultProject string `json:"defaultProject"`
		Projects       map[string]struct {
			Architect struct {
				Build struct {
					Builder string `json:"builder"`
					Options struct {
						OutputPath json.RawMessage `json:"outputPath"`
					} `json:"options"`
				} `json:"build"`
			} `json:"architect"`
		} `json:"projects"`
	}
	if json.Unmarshal(raw, &ng) != nil || len(ng.Projects) == 0 {
		return ""
	}
	name := ng.DefaultProject
	if _, ok := ng.Projects[name]; !ok {
		names := make([]string, 0, len(ng.Projects))
		for n := range ng.Projects {
			names = append(names, n)
		}
		sort.Strings(names)
		name = names[0]
	}
	bld := ng.Projects[name].Architect.Build
	app := strings.HasSuffix(bld.Builder, ":application") || strings.HasSuffix(bld.Builder, ":browser-esbuild")
	var out string
	if json.Unmarshal(bld.Options.OutputPath, &out) == nil && out != "" {
		if app {
			out = path.Join(out, "browser")
		}
	} else {
		var obj struct {
			Base    string  `json:"base"`
			Browser *string `json:"browser"`
		}
		if json.Unmarshal(bld.Options.OutputPath, &obj) != nil || obj.Base == "" {
			return ""
		}
		out = obj.Base
		if br := "browser"; obj.Browser != nil {
			if *obj.Browser != "" {
				out = path.Join(out, *obj.Browser)
			}
		} else {
			out = path.Join(out, br)
		}
	}
	out = path.Clean(strings.ReplaceAll(out, "\\", "/"))
	if !config.SafeRelPath(out) {
		return ""
	}
	return out
}

func detectFramework(dir string, deps, dev map[string]string) webProject {
	has := func(n string) bool { _, a := deps[n]; _, b := dev[n]; return a || b }
	switch {
	case has("next"):
		if nextStaticExport(dir) {
			return webProject{Framework: "Next.js", BuildCmd: "next build", OutDir: "out", Static: true}
		}
		return webProject{Framework: "Next.js", OutDir: "out", Static: false,
			BuildCmd: "Use a static export (set output: 'export' in next.config, then run `next build`), or make an app from the deployed website URL instead."}
	case has("nuxt"):
		return webProject{Framework: "Nuxt", BuildCmd: "npx nuxi generate", OutDir: ".output/public", Static: true}
	case has("@sveltejs/kit"):
		return webProject{Framework: "SvelteKit", BuildCmd: "npm run build (with @sveltejs/adapter-static)", OutDir: "build", Static: true}
	case has("astro"):
		return webProject{Framework: "Astro", BuildCmd: "npm run build", OutDir: "dist", Static: true}
	case has("react-scripts"):
		return webProject{Framework: "Create React App", BuildCmd: "npm run build", OutDir: "build", Static: true}
	case has("@angular/core"):
		return webProject{Framework: "Angular", BuildCmd: "npm run build", OutDir: "dist/browser", Static: true}
	case has("vite"):
		return webProject{Framework: "Vite", BuildCmd: "npm run build", OutDir: "dist", Static: true}
	case has("parcel"):
		return webProject{Framework: "Parcel", BuildCmd: "npm run build", OutDir: "dist", Static: true}
	}
	return webProject{Framework: "web", BuildCmd: "npm run build", OutDir: "dist", Static: true}
}

// nextStaticExport reports whether next.config.* asks for a static export.
func nextStaticExport(dir string) bool {
	for _, n := range []string{"next.config.js", "next.config.mjs", "next.config.ts", "next.config.cjs"} {
		if b, err := os.ReadFile(filepath.Join(dir, n)); err == nil {
			s := strings.ReplaceAll(string(b), " ", "")
			if strings.Contains(s, `output:"export"`) || strings.Contains(s, `output:'export'`) {
				return true
			}
		}
	}
	return fileExists(filepath.Join(dir, "out", "index.html"))
}

func prettyPkgName(n string) string {
	if i := strings.LastIndex(n, "/"); i >= 0 {
		n = n[i+1:] // @scope/name
	}
	parts := strings.FieldsFunc(n, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
