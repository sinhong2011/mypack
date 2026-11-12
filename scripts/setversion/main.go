// Command setversion sets "version" in mygo.json, from a release tag or a
// version: setversion mygo.json v0.2.0. make installers runs it, so that
// mygo build names and stamps the installers with the release's version.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var semver = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: setversion mygo.json <version>")
		os.Exit(2)
	}
	path, v := os.Args[1], strings.TrimPrefix(os.Args[2], "v")
	if !semver.MatchString(v) {
		fmt.Fprintf(os.Stderr, "setversion: %q is not X.Y.Z or X.Y.Z-pre\n", os.Args[2])
		os.Exit(2)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "setversion:", err)
		os.Exit(1)
	}
	// Replace the one value in place, so the file keeps its order and form.
	re := regexp.MustCompile(`("version"\s*:\s*)"[^"]*"`)
	if len(re.FindAll(b, -1)) != 1 {
		fmt.Fprintf(os.Stderr, "setversion: %s needs exactly one \"version\"\n", path)
		os.Exit(1)
	}
	out := re.ReplaceAll(b, []byte(`${1}"`+v+`"`))
	if !json.Valid(out) {
		fmt.Fprintln(os.Stderr, "setversion: the result is not JSON")
		os.Exit(1)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "setversion:", err)
		os.Exit(1)
	}
	fmt.Printf("%s: version %s\n", path, v)
}
