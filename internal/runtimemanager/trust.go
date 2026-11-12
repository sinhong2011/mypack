package runtimemanager

import (
	"crypto/ed25519"
	_ "embed"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// productionKeyB64 is the public key of the production registry (the
// "runtimes" GitHub Release). Every build trusts it, including installers made
// with mygo build, which take no ldflags. Its private key is only in the
// REGISTRY_SIGNING_KEY Actions secret. To rotate, add the new key here (one per
// line) and keep the old one until no published manifest is signed with it.
//
//go:embed registry.pub
var productionKeyB64 string

// trustedKeysB64 is a comma-separated list of base64 Ed25519 public keys (raw
// 32-byte keys). Release builds set it at link time (key rotation = list the
// new key next to the old one):
//
//	go build -ldflags "-X mypack.dev/mypack/internal/runtimemanager.trustedKeysB64=KEY1,KEY2"
var trustedKeysB64 string

// Key sources (in this order, all merged):
//
//  0. productionKeyB64 (embedded, above)
//  1. trustedKeysB64 (ldflags, above)
//  2. env MYPACK_TRUSTED_KEY: one or more comma-separated base64 keys. Intended
//     for CI and self-hosted registries. Anyone who controls the environment of
//     the Builder process can already run arbitrary code, so this does not
//     widen the attack surface.
//  3. MYPACK_DEV_TRUST=1: also trust the dev key in dist/keys/registry.pub
//     (looked up from the working directory upwards, or the file named by
//     MYPACK_DEV_PUBKEY_FILE). Never set in production.
//  4. keys registered with AddTrustedKey (tests/embedding).
const (
	envTrustedKey  = "MYPACK_TRUSTED_KEY"
	envDevTrust    = "MYPACK_DEV_TRUST"
	envDevPubFile  = "MYPACK_DEV_PUBKEY_FILE"
	devPubRelative = "dist/keys/registry.pub"
)

var (
	extraMu   sync.Mutex
	extraKeys []ed25519.PublicKey
)

// AddTrustedKey adds a process-wide trusted registry public key.
func AddTrustedKey(k ed25519.PublicKey) {
	if len(k) != ed25519.PublicKeySize {
		return
	}
	extraMu.Lock()
	defer extraMu.Unlock()
	extraKeys = append(extraKeys, append(ed25519.PublicKey(nil), k...))
}

// ResetTrustedKeys removes keys added with AddTrustedKey (tests).
func ResetTrustedKeys() {
	extraMu.Lock()
	defer extraMu.Unlock()
	extraKeys = nil
}

func parseKeys(list string) []ed25519.PublicKey {
	var out []ed25519.PublicKey
	for _, s := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == ' ' }) {
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
		if err != nil || len(b) != ed25519.PublicKeySize {
			continue
		}
		out = append(out, ed25519.PublicKey(b))
	}
	return out
}

// TrustedKeys returns every key currently trusted to sign registry manifests.
func TrustedKeys() []ed25519.PublicKey {
	var keys []ed25519.PublicKey
	keys = append(keys, parseKeys(productionKeyB64)...)
	keys = append(keys, parseKeys(trustedKeysB64)...)
	keys = append(keys, parseKeys(os.Getenv(envTrustedKey))...)
	if os.Getenv(envDevTrust) == "1" {
		if p := findDevPub(); p != "" {
			if b, err := os.ReadFile(p); err == nil {
				keys = append(keys, parseKeys(string(b))...)
			}
		}
	}
	extraMu.Lock()
	keys = append(keys, extraKeys...)
	extraMu.Unlock()
	return keys
}

func findDevPub() string {
	if p := os.Getenv(envDevPubFile); p != "" {
		return p
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 8; i++ {
		p := filepath.Join(dir, filepath.FromSlash(devPubRelative))
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
