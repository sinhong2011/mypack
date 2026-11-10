// Package licensing verifies offline Entitlement Tokens (spec §9E.2/§9E.3).
//
// A token is base64url(payload JSON) + "." + base64url(Ed25519 signature over
// the payload bytes). The Builder only ever holds issuer PUBLIC keys; the
// private key lives with the (out of scope) license issuer. Verification needs
// no network. Generated apps never use this package.
package licensing

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Edition names.
const (
	EditionFree = "free"
	EditionPro  = "pro"
)

// ScopeBuilder is the only scope in v1: the license unlocks Builder features.
const ScopeBuilder = "builder"

// Payload is the signed claim set.
type Payload struct {
	LicenseID       string    `json:"licenseId"`
	Edition         string    `json:"edition"`
	Email           string    `json:"email,omitempty"`
	IssuedAt        time.Time `json:"issuedAt"`
	Scope           []string  `json:"scope"`
	MaxDevices      int       `json:"maxDevices"`
	RevocationEpoch int       `json:"revocationEpoch"`
}

// Verification errors. They are distinguishable with errors.Is.
var (
	ErrMalformed    = errors.New("the license key is not in the expected format")
	ErrBadSignature = errors.New("the license key's signature could not be verified")
	ErrUnsupported  = errors.New("the license key is not valid for this version of mypack")
	ErrRevoked      = errors.New("the license key has been revoked")
	ErrNoKeys       = errors.New("this build of mypack has no license verification key")
)

// issuerKeysB64 is a comma-separated list of base64 Ed25519 public keys (raw
// 32 bytes). Release builds set it at link time:
//
//	-ldflags "-X mypack.dev/mypack/internal/licensing.issuerKeysB64=KEY1,KEY2"
var issuerKeysB64 string

// minRevocationEpoch is raised at link time to reject tokens issued before a
// revocation round (-X ...licensing.minRevocationEpoch=3).
var minRevocationEpoch = "0"

// EnvPubKey names a dev/CI override: one or more comma-separated base64 keys.
const EnvPubKey = "MYPACK_LICENSE_PUBKEY"

// Keys returns the trusted issuer keys (ldflags + environment).
func Keys() []ed25519.PublicKey {
	return append(ParseKeys(issuerKeysB64), ParseKeys(os.Getenv(EnvPubKey))...)
}

// ParseKeys decodes a separated list of base64 raw Ed25519 public keys,
// skipping malformed entries.
func ParseKeys(list string) []ed25519.PublicKey {
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

// Sign builds a token. It exists for the dev issuer tool and tests; the
// shipped client never has a private key.
func Sign(priv ed25519.PrivateKey, p Payload) (string, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(priv, body)
	return b64.EncodeToString(body) + "." + b64.EncodeToString(sig), nil
}

var b64 = base64.RawURLEncoding

// Verify checks token against keys (nil = Keys()) and returns the payload.
func Verify(token string, keys []ed25519.PublicKey) (Payload, error) {
	var p Payload
	token = strings.Join(strings.Fields(token), "")
	body64, sig64, ok := strings.Cut(token, ".")
	if !ok || body64 == "" || sig64 == "" || strings.Contains(sig64, ".") {
		return p, ErrMalformed
	}
	body, err1 := b64.DecodeString(strings.TrimRight(body64, "="))
	sig, err2 := b64.DecodeString(strings.TrimRight(sig64, "="))
	if err1 != nil || err2 != nil || len(sig) != ed25519.SignatureSize {
		return p, ErrMalformed
	}
	if keys == nil {
		keys = Keys()
	}
	if len(keys) == 0 {
		return p, ErrNoKeys
	}
	valid := false
	for _, k := range keys {
		if ed25519.Verify(k, body, sig) {
			valid = true
			break
		}
	}
	if !valid {
		return p, ErrBadSignature
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return Payload{}, ErrMalformed
	}
	if p.LicenseID == "" || p.Edition != EditionPro {
		return Payload{}, fmt.Errorf("%w (edition %q)", ErrUnsupported, p.Edition)
	}
	hasScope := false
	for _, s := range p.Scope {
		if s == ScopeBuilder {
			hasScope = true
		}
	}
	if !hasScope {
		return Payload{}, ErrUnsupported
	}
	var min int
	fmt.Sscanf(minRevocationEpoch, "%d", &min)
	if p.RevocationEpoch < min {
		return Payload{}, ErrRevoked
	}
	return p, nil
}

// EncodeKey returns the standard-base64 form of a public key (for ldflags/env).
func EncodeKey(k ed25519.PublicKey) string { return base64.StdEncoding.EncodeToString(k) }
