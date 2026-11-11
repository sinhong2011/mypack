package config

import (
	"errors"
	"testing"
)

func TestProjectBuildValidation(t *testing.T) {
	host := Target{OS: "darwin", Arch: "arm64", Format: "app"}
	mk := func(b *ProjectBuild, project string) *Config {
		c := New("App", Source{Type: SourceDir, Value: "/p/dist", Entry: "index.html", Project: project, Build: b}, host)
		return c
	}
	ok := &ProjectBuild{PackageManager: "pnpm", Script: "build:prod", OutDir: "dist/web", Framework: "Vite"}
	if err := mk(ok, "/p").Validate(); err != nil {
		t.Fatalf("valid build rejected: %v", err)
	}
	if err := mk(nil, "").Validate(); err != nil {
		t.Fatalf("plain dir rejected: %v", err)
	}
	bad := map[string]*ProjectBuild{
		"app.source.build.packageManager": {PackageManager: "make", Script: "build", OutDir: "dist"},
		"app.source.build.script":         {PackageManager: "npm", Script: "build && rm -rf ~", OutDir: "dist"},
		"app.source.build.outDir":         {PackageManager: "npm", Script: "build", OutDir: "../dist"},
	}
	for field, b := range bad {
		err := mk(b, "/p").Validate()
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Issues[0].Field != field {
			t.Errorf("%s: %v", field, err)
		}
	}
	for _, s := range []string{"-rf", "--prefix=/x", "a b", "a;b", ""} {
		if err := mk(&ProjectBuild{PackageManager: "npm", Script: s, OutDir: "dist"}, "/p").Validate(); err == nil {
			t.Errorf("script %q accepted", s)
		}
	}
	for _, o := range []string{"/abs", "a/../../b", `..\x`, "C:\\x", ""} {
		if err := mk(&ProjectBuild{PackageManager: "npm", Script: "build", OutDir: o}, "/p").Validate(); err == nil {
			t.Errorf("outDir %q accepted", o)
		}
	}
	if err := mk(ok, "").Validate(); err == nil {
		t.Error("build without project accepted")
	}
	if err := mk(ok, "relative/dir").Validate(); err == nil {
		t.Error("relative project accepted")
	}
	// Old configs (no project/build keys) still parse; new keys round-trip.
	c, err := Parse([]byte(`{"schemaVersion":1,"app":{"name":"A","source":{"type":"dir","value":"/x"}}}`))
	if err != nil || c.App.Source.Build != nil || c.App.Source.Project != "" {
		t.Fatalf("old config: %v %+v", err, c)
	}
	b, _ := mk(ok, "/p").Marshal()
	c2, err := Parse(b)
	if err != nil || c2.App.Source.Build == nil || c2.App.Source.Build.Script != "build:prod" {
		t.Fatalf("round trip: %v", err)
	}
}
