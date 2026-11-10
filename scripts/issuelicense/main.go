// Command issuelicense is a DEV-ONLY license issuer.
//
//	go run ./scripts/issuelicense -email me@example.com [-keys dist/keys] [-devices 3]
//
// On first run it creates dist/keys/license.key and license.pub (base64;
// dist/ is gitignored) and prints the public key to trust, then prints an
// Entitlement Token for the email. To use it:
//
//	export MYPACK_LICENSE_PUBKEY=$(cat dist/keys/license.pub)
//	mypack license activate <token>
//
// The production issuer is a server holding its own private key; this tool's
// key must never ship.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mypack.dev/mypack/internal/licensing"
)

func main() {
	email := flag.String("email", "", "customer email (optional)")
	dir := flag.String("keys", "dist/keys", "key directory")
	devices := flag.Int("devices", 3, "maxDevices")
	epoch := flag.Int("epoch", 0, "revocationEpoch")
	flag.Parse()
	if err := run(*email, *dir, *devices, *epoch); err != nil {
		fmt.Fprintln(os.Stderr, "issuelicense:", err)
		os.Exit(1)
	}
}

func run(email, dir string, devices, epoch int) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	keyPath, pubPath := filepath.Join(dir, "license.key"), filepath.Join(dir, "license.pub")
	var priv ed25519.PrivateKey
	if b, err := os.ReadFile(keyPath); err == nil {
		raw, derr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if derr != nil || len(raw) != ed25519.PrivateKeySize {
			return fmt.Errorf("%s is not a valid private key", keyPath)
		}
		priv = ed25519.PrivateKey(raw)
	} else {
		pub, p, gerr := ed25519.GenerateKey(rand.Reader)
		if gerr != nil {
			return gerr
		}
		priv = p
		if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(priv)+"\n"), 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(pubPath, []byte(licensing.EncodeKey(pub)+"\n"), 0o644); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "created", keyPath, "and", pubPath)
	}
	pub := priv.Public().(ed25519.PublicKey)
	fmt.Fprintln(os.Stderr, "trust with: MYPACK_LICENSE_PUBKEY="+licensing.EncodeKey(pub))

	id := make([]byte, 8)
	_, _ = rand.Read(id)
	tok, err := licensing.Sign(priv, licensing.Payload{
		LicenseID: fmt.Sprintf("lic_%x", id), Edition: licensing.EditionPro, Email: email,
		IssuedAt: time.Now().UTC().Truncate(time.Second), Scope: []string{licensing.ScopeBuilder},
		MaxDevices: devices, RevocationEpoch: epoch,
	})
	if err != nil {
		return err
	}
	fmt.Println(tok)
	return nil
}
