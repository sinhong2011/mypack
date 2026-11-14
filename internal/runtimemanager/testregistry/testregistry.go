// Package testregistry builds small signed runtime registries on disk for
// tests (of this repo's packages). It is not used in production code.
package testregistry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manifest and Entry mirror runtimemanager's registry types (duplicated to
// avoid an import cycle with runtimemanager's own tests).
type Manifest struct {
	SchemaVersion int       `json:"schemaVersion"`
	Sequence      int64     `json:"sequence"`
	Published     time.Time `json:"published"`
	Expires       time.Time `json:"expires"`
	Runtimes      []Entry   `json:"runtimes"`
}

// Entry is one runtime archive.
type Entry struct {
	Version      string   `json:"version"`
	OS           string   `json:"os"`
	Arch         string   `json:"arch"`
	Filename     string   `json:"filename"`
	URL          string   `json:"url"`
	SHA256       string   `json:"sha256"`
	Size         int64    `json:"size"`
	Capabilities []string `json:"capabilities,omitempty"`
	Tier         string   `json:"tier,omitempty"`
}

// Key is os-arch, os-arch-lite for a lite entry.
func (e Entry) Key() string {
	if e.Tier == "lite" {
		return e.OS + "-" + e.Arch + "-lite"
	}
	return e.OS + "-" + e.Arch
}

func exeName(key string) string {
	if len(key) > 8 && key[:8] == "windows-" {
		return "mypack-runtime.exe"
	}
	return "mypack-runtime"
}

// Options configures Create. Zero values are sensible.
type Options struct {
	Version      string            // default "1.0.0"
	Keys         []string          // os-arch keys, default darwin-arm64
	Binaries     map[string][]byte // per key; default a tiny shell stub
	Sequence     int64             // default 1
	Expires      time.Time         // default +24h
	Capabilities []string
	PrivateKey   ed25519.PrivateKey // default: freshly generated
	Now          time.Time          // default time.Now()
}

// Registry is a created registry.
type Registry struct {
	Dir         string
	URL         string // file:// URL of registry.json
	Public      ed25519.PublicKey
	Private     ed25519.PrivateKey
	Manifest    Manifest
	ManifestRaw []byte
}

// StubBinary is the default fake runtime content.
func StubBinary(key string) []byte {
	return []byte("#!/bin/sh\n# fake mypack runtime for " + key + "\nexit 0\n")
}

// Pack builds a .tar.gz with one executable file.
func Pack(name string, content []byte) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(content)
	_ = tw.Close()
	_ = zw.Close()
	return buf.Bytes()
}

// Create writes archives, registry.json and registry.json.sig into dir.
func Create(dir string, o Options) (*Registry, error) {
	if o.Version == "" {
		o.Version = "1.0.0"
	}
	if len(o.Keys) == 0 {
		o.Keys = []string{"darwin-arm64"}
	}
	if o.Sequence == 0 {
		o.Sequence = 1
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if o.Expires.IsZero() {
		o.Expires = o.Now.Add(24 * time.Hour)
	}
	priv := o.PrivateKey
	if priv == nil {
		var err error
		_, priv, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
	}
	pub := priv.Public().(ed25519.PublicKey)
	m := Manifest{SchemaVersion: 1, Sequence: o.Sequence, Published: o.Now.UTC().Truncate(time.Second), Expires: o.Expires.UTC().Truncate(time.Second)}
	if err := os.MkdirAll(filepath.Join(dir, o.Version), 0o755); err != nil {
		return nil, err
	}
	for _, key := range o.Keys {
		tier := ""
		if strings.HasSuffix(key, "-lite") {
			tier = "lite"
		}
		goos, arch := split(strings.TrimSuffix(key, "-lite"))
		bin := StubBinary(key)
		if b, ok := o.Binaries[key]; ok {
			bin = b
		}
		arc := Pack(exeName(key), bin)
		fn := fmt.Sprintf("mypack-runtime-%s-%s.tar.gz", o.Version, key)
		if err := os.WriteFile(filepath.Join(dir, o.Version, fn), arc, 0o644); err != nil {
			return nil, err
		}
		sum := sha256.Sum256(arc)
		m.Runtimes = append(m.Runtimes, Entry{Version: o.Version, OS: goos, Arch: arch, Filename: fn,
			URL: o.Version + "/" + fn, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(arc)), Capabilities: o.Capabilities, Tier: tier})
	}
	r := &Registry{Dir: dir, Public: pub, Private: priv, Manifest: m}
	if err := r.Write(m); err != nil {
		return nil, err
	}
	return r, nil
}

// Write (re)writes registry.json + .sig for manifest m.
func (r *Registry) Write(m Manifest) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	r.Manifest, r.ManifestRaw = m, raw
	if err := os.WriteFile(filepath.Join(r.Dir, "registry.json"), raw, 0o644); err != nil {
		return err
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(r.Private, raw))
	if err := os.WriteFile(filepath.Join(r.Dir, "registry.json.sig"), []byte(sig), 0o644); err != nil {
		return err
	}
	r.URL = "file://" + filepath.ToSlash(filepath.Join(r.Dir, "registry.json"))
	if len(r.URL) > 8 && r.URL[7] != '/' { // windows drive paths
		r.URL = "file:///" + r.URL[7:]
	}
	return nil
}

func split(key string) (string, string) {
	for i := 0; i < len(key); i++ {
		if key[i] == '-' {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}
