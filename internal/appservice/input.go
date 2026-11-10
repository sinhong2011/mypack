package appservice

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mypack.dev/mypack/pkg/config"
)

// LoadConfigFile reads a mypack.json or a Pake config (auto-detected) and
// returns the config with relative local paths (file/dir source, icon,
// injections) resolved against the file's directory. Defaults and semantic
// validation are left to the caller. Warnings list unmapped Pake fields.
func LoadConfigFile(path string) (*config.Config, []string, error) {
	abs, err := filepath.Abs(expandHome(path))
	if err != nil {
		return nil, nil, err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, nil, newErr(CodeInvalidInput, "Cannot read "+path, "Check that the file exists and is readable.")
	}
	var (
		cfg   *config.Config
		warns []string
	)
	if config.IsPakeJSON(b) {
		cfg, warns, err = config.ImportPake(b)
	} else {
		cfg, err = config.Parse(b)
	}
	if err != nil {
		return nil, nil, asDomainErr(err, CodeInvalidConfig)
	}
	base := filepath.Dir(abs)
	rel := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(base, p)
	}
	if cfg.App.Source.Type == config.SourceFile || cfg.App.Source.Type == config.SourceDir {
		cfg.App.Source.Value = rel(cfg.App.Source.Value)
	}
	cfg.App.Icon.Path = rel(cfg.App.Icon.Path)
	for i := range cfg.WebView.Injections {
		cfg.WebView.Injections[i].Path = rel(cfg.WebView.Injections[i].Path)
	}
	return cfg, warns, nil
}

// expandHome expands a leading "~/".
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[1:])
		}
	}
	return p
}

type inputKind int

const (
	inputURL inputKind = iota
	inputFile
	inputDir
	inputConfig
)

// detectInput classifies a CreateProject input string.
func detectInput(in string) (kind inputKind, path string, err error) {
	low := strings.ToLower(in)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return inputURL, in, nil
	}
	path = expandHome(in)
	if st, serr := os.Stat(path); serr == nil {
		if abs, aerr := filepath.Abs(path); aerr == nil {
			path = abs
		}
		if st.IsDir() {
			return inputDir, path, nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".json":
			return inputConfig, path, nil
		case ".html", ".htm":
			return inputFile, path, nil
		}
		return 0, "", newErr(CodeInvalidInput, "Unsupported file type: "+filepath.Base(path),
			"Use a website address, an .html file, a folder with index.html, or a mypack.json / pake.json file.")
	}
	// Does not exist locally: things that look like paths are an error,
	// anything else is treated as a bare domain ("github.com/foo").
	ext := strings.ToLower(filepath.Ext(in))
	looksLikePath := strings.HasPrefix(in, ".") || strings.HasPrefix(in, "~") ||
		strings.HasPrefix(in, "/") || strings.HasPrefix(in, `\`) ||
		len(in) > 2 && in[1] == ':' && (in[2] == '\\' || in[2] == '/') ||
		ext == ".json" || ext == ".html" || ext == ".htm"
	if looksLikePath {
		return 0, "", newErr(CodeInvalidInput, "No such file or folder: "+in, "Check the path and try again.")
	}
	return inputURL, in, nil
}

// findEntry picks the HTML entry of a directory source.
func findEntry(dir string) (string, error) {
	if st, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !st.IsDir() {
		return "index.html", nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", newErr(CodeInvalidInput, "Cannot read folder "+dir, "Check the folder permissions.")
	}
	var names []string
	for _, e := range ents {
		n := strings.ToLower(e.Name())
		if !e.IsDir() && (strings.HasSuffix(n, ".html") || strings.HasSuffix(n, ".htm")) {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", newErr(CodeInvalidInput, "The folder contains no .html file", "Add an index.html to the folder.")
	}
	sort.Strings(names)
	return names[0], nil
}

// cleanName makes a metadata title safe for config.App.Name.
func cleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 64 {
		s = strings.TrimSpace(string(r[:64]))
	}
	return s
}

// originOf derives Project.Origin from an actor ("mcp:claude" -> "mcp").
func originOf(actor string) string {
	if i := strings.IndexByte(actor, ':'); i > 0 {
		actor = actor[:i]
	}
	switch actor {
	case "gui", "cli", "mcp", "companion", "import":
		return actor
	}
	return "gui"
}

func actorOr(actor, def string) string {
	if strings.TrimSpace(actor) == "" {
		return def
	}
	return actor
}
