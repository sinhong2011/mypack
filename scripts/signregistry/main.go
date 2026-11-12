// Command signregistry is the helper behind scripts/release-runtimes.sh.
//
//	signregistry keygen   -keys dist/keys
//	signregistry pack     -bin <file> -key darwin-arm64 -out <archive.tar.gz>
//	signregistry registry -dir dist/registry -keys dist/keys [-capabilities a,b] [-expires-days 365] [-flat] [-merge]
//
// keygen creates an Ed25519 key pair on first use (dist/keys/registry.key,
// registry.pub; base64) and prints the public key. pack writes a reproducible
// tar.gz holding the single runtime binary. registry scans
// <dir>/<version>/mypack-runtime-<version>-<os>-<arch>.tar.gz, writes
// <dir>/registry.json (sequence = previous + 1) and the detached signature
// registry.json.sig (base64 Ed25519 over the exact bytes).
//
// -flat writes each entry's url as the bare file name, for a registry whose
// files sit side by side (a GitHub Release). -merge keeps the entries of the
// previous <dir>/registry.json whose archives are not in <dir>, so a registry
// that holds only the newest archives locally still lists the older ones.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"mypack.dev/mypack/internal/runtimemanager"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = cmdKeygen(os.Args[2:])
	case "pack":
		err = cmdPack(os.Args[2:])
	case "registry":
		err = cmdRegistry(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "signregistry:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: signregistry keygen|pack|registry [flags]")
	os.Exit(2)
}

func cmdKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	dir := fs.String("keys", "dist/keys", "key directory")
	fs.Parse(args)
	_, pub, err := loadOrCreateKey(*dir)
	if err != nil {
		return err
	}
	fmt.Println(base64.StdEncoding.EncodeToString(pub))
	return nil
}

func loadOrCreateKey(dir string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	keyFile, pubFile := filepath.Join(dir, "registry.key"), filepath.Join(dir, "registry.pub")
	if b, err := os.ReadFile(keyFile); err == nil {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if err != nil || len(raw) != ed25519.PrivateKeySize {
			return nil, nil, fmt.Errorf("%s is not a valid private key", keyFile)
		}
		priv := ed25519.PrivateKey(raw)
		pub := priv.Public().(ed25519.PublicKey)
		// keep the .pub file in sync
		if err := os.WriteFile(pubFile, []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644); err != nil {
			return nil, nil, err
		}
		return priv, pub, nil
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(keyFile, []byte(base64.StdEncoding.EncodeToString(priv)+"\n"), 0o600); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(pubFile, []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644); err != nil {
		return nil, nil, err
	}
	fmt.Fprintln(os.Stderr, "generated a new registry signing key in", dir)
	return priv, pub, nil
}

func cmdPack(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ExitOnError)
	bin := fs.String("bin", "", "runtime binary")
	key := fs.String("key", "", "os-arch key")
	out := fs.String("out", "", "output .tar.gz")
	fs.Parse(args)
	if *bin == "" || *key == "" || *out == "" {
		return errors.New("pack needs -bin, -key and -out")
	}
	data, err := os.ReadFile(*bin)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	tmp := *out + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw, _ := gzip.NewWriterLevel(f, gzip.BestCompression) // zero ModTime/Name: reproducible
	tw := tar.NewWriter(zw)
	hdr := &tar.Header{Name: runtimemanager.ExecutableName(*key), Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg,
		ModTime: time.Unix(0, 0), Uid: 0, Gid: 0, Format: tar.FormatPAX}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(data); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, *out)
}

var archiveRe = regexp.MustCompile(`^mypack-runtime-(\d+(?:\.\d+){0,3})-(darwin|windows|linux)-(amd64|arm64|universal)\.tar\.gz$`)

func cmdRegistry(args []string) error {
	fs := flag.NewFlagSet("registry", flag.ExitOnError)
	dir := fs.String("dir", "dist/registry", "registry directory")
	keys := fs.String("keys", "dist/keys", "key directory")
	caps := fs.String("capabilities", "tray,globalShortcut,media,dragDrop,proxy,injection", "capabilities to advertise (comma separated; overrides existing)")
	days := fs.Int("expires-days", 365, "manifest validity in days")
	flat := fs.Bool("flat", false, "write entry urls as bare file names (files side by side, e.g. a GitHub Release)")
	merge := fs.Bool("merge", false, "keep previous entries whose archives are not in -dir")
	fs.Parse(args)

	priv, pub, err := loadOrCreateKey(*keys)
	if err != nil {
		return err
	}
	regPath := filepath.Join(*dir, "registry.json")
	var prev runtimemanager.Manifest
	if b, err := os.ReadFile(regPath); err == nil {
		_ = json.Unmarshal(b, &prev)
	}
	prevCaps := map[string][]string{}
	for _, e := range prev.Runtimes {
		prevCaps[e.Version+"/"+e.Key()] = e.Capabilities
	}
	var capList []string
	for _, c := range strings.Split(*caps, ",") {
		if c = strings.TrimSpace(c); c != "" {
			capList = append(capList, c)
		}
	}

	var entries []runtimemanager.Entry
	vers, err := os.ReadDir(*dir)
	if err != nil {
		return err
	}
	for _, v := range vers {
		if !v.IsDir() {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(*dir, v.Name()))
		for _, f := range files {
			m := archiveRe.FindStringSubmatch(f.Name())
			if m == nil || m[1] != v.Name() {
				continue
			}
			p := filepath.Join(*dir, v.Name(), f.Name())
			sum, size, err := hashFile(p)
			if err != nil {
				return err
			}
			e := runtimemanager.Entry{Version: m[1], OS: m[2], Arch: m[3], Filename: f.Name(), URL: v.Name() + "/" + f.Name(),
				SHA256: sum, Size: size, Capabilities: capList}
			if *flat {
				e.URL = f.Name()
			}
			if len(capList) == 0 {
				e.Capabilities = prevCaps[e.Version+"/"+e.Key()]
			}
			entries = append(entries, e)
		}
	}
	if *merge {
		have := map[string]bool{}
		for _, e := range entries {
			have[e.Version+"/"+e.Key()] = true
		}
		for _, e := range prev.Runtimes {
			if !have[e.Version+"/"+e.Key()] {
				entries = append(entries, e)
			}
		}
	}
	if len(entries) == 0 {
		return fmt.Errorf("no runtime archives found in %s", *dir)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Version != entries[j].Version {
			return entries[i].Version > entries[j].Version
		}
		return entries[i].Key() < entries[j].Key()
	})
	now := time.Now().UTC().Truncate(time.Second)
	m := runtimemanager.Manifest{SchemaVersion: runtimemanager.SchemaVersion, Sequence: prev.Sequence + 1, Published: now,
		Expires: now.Add(time.Duration(*days) * 24 * time.Hour), Runtimes: entries}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, raw))
	if err := writeAtomic(regPath, raw); err != nil {
		return err
	}
	if err := writeAtomic(regPath+".sig", []byte(sig+"\n")); err != nil {
		return err
	}
	// self-check exactly as the Builder will
	if _, err := runtimemanager.VerifyManifest(raw, []byte(sig), []ed25519.PublicKey{pub}); err != nil {
		return fmt.Errorf("self-verification failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (sequence %d, %d runtimes, expires %s)\n", regPath, m.Sequence, len(entries), m.Expires.Format("2006-01-02"))
	fmt.Println(base64.StdEncoding.EncodeToString(pub))
	return nil
}

func hashFile(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

func writeAtomic(p string, b []byte) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
