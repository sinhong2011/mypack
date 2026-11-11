package appservice

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// webProject describes a JavaScript web project folder (one with a
// package.json), so a dropped project resolves to its built output rather
// than its unbuilt source (a Vite root index.html loads /src/main.tsx and
// would package as a blank page).
type webProject struct {
	Name      string // productName / name from package.json
	Framework string // "Next.js", "Vite", "Nuxt", …
	BuildCmd  string // how to build it
	OutDir    string // conventional output folder (relative)
	Static    bool   // false: needs a server (SSR), cannot be bundled
}

// outputDirs are checked in order for a built index.html.
var outputDirs = []string{"dist", "build", "out", ".output/public", "www", "_site", "public"}

// resolveWebProject returns the folder to package for dir. ok=false means
// dir is not a JS project (use it as is). An error explains what to do when
// the project is not built yet or cannot be packaged statically.
func resolveWebProject(dir string) (site string, proj webProject, ok bool, err error) {
	b, rerr := os.ReadFile(filepath.Join(dir, "package.json"))
	if rerr != nil {
		return dir, proj, false, nil
	}
	var pkg struct {
		Name            string            `json:"name"`
		ProductName     string            `json:"productName"`
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	_ = json.Unmarshal(b, &pkg)
	proj = detectFramework(dir, pkg.Dependencies, pkg.DevDependencies)
	proj.Name = firstNonEmpty(pkg.ProductName, prettyPkgName(pkg.Name))

	for _, d := range outputDirs {
		if d == "public" && proj.Framework != "" && proj.Framework != "Static site" {
			continue // a framework's public/ holds unbuilt templates
		}
		if fileExists(filepath.Join(dir, d, "index.html")) {
			return filepath.Join(dir, d), proj, true, nil
		}
	}
	if !proj.Static {
		return "", proj, true, newErr(CodeInvalidInput,
			proj.Framework+" with server-side rendering can't be packaged as a static app.",
			proj.BuildCmd)
	}
	return "", proj, true, newErr(CodeInvalidInput,
		"This "+proj.Framework+" project hasn't been built yet.",
		"Run `"+proj.BuildCmd+"` in the project folder, then drop the project (or its "+proj.OutDir+" folder) here again.")
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
		return webProject{Framework: "Angular", BuildCmd: "npm run build", OutDir: "dist/<project>/browser", Static: true}
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
