package licensing

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func mk(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func payload() Payload {
	return Payload{LicenseID: "lic_1", Edition: "pro", Email: "a@b.c", IssuedAt: time.Now().UTC().Truncate(time.Second),
		Scope: []string{"builder"}, MaxDevices: 3}
}

func TestVerifyRoundTrip(t *testing.T) {
	pub, priv := mk(t)
	tok, err := Sign(priv, payload())
	if err != nil {
		t.Fatal(err)
	}
	p, err := Verify("  "+tok+"\n", []ed25519.PublicKey{pub})
	if err != nil || p.LicenseID != "lic_1" || p.Email != "a@b.c" || p.MaxDevices != 3 {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestVerifyRejects(t *testing.T) {
	pub, priv := mk(t)
	otherPub, _ := mk(t)
	good, _ := Sign(priv, payload())
	keys := []ed25519.PublicKey{pub}

	if _, err := Verify(good, []ed25519.PublicKey{otherPub}); !errors.Is(err, ErrBadSignature) {
		t.Errorf("wrong key: %v", err)
	}
	// Tampered payload.
	body, sig, _ := strings.Cut(good, ".")
	forged, _ := Sign(priv, Payload{LicenseID: "x", Edition: "pro", Scope: []string{"builder"}})
	fb, _, _ := strings.Cut(forged, ".")
	if _, err := Verify(fb+"."+sig, keys); !errors.Is(err, ErrBadSignature) {
		t.Errorf("tamper: %v", err)
	}
	_ = body
	for _, bad := range []string{"", "abc", "a.b", "a.b.c", "!!!.???"} {
		if _, err := Verify(bad, keys); !errors.Is(err, ErrMalformed) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if _, err := Verify(good, []ed25519.PublicKey{}); !errors.Is(err, ErrNoKeys) {
		t.Errorf("no keys: %v", err)
	}
	p := payload()
	p.Edition = "team"
	tok, _ := Sign(priv, p)
	if _, err := Verify(tok, keys); !errors.Is(err, ErrUnsupported) {
		t.Errorf("edition: %v", err)
	}
	p = payload()
	p.Scope = []string{"cloud"}
	tok, _ = Sign(priv, p)
	if _, err := Verify(tok, keys); !errors.Is(err, ErrUnsupported) {
		t.Errorf("scope: %v", err)
	}
}

func TestRevocationEpoch(t *testing.T) {
	pub, priv := mk(t)
	old := minRevocationEpoch
	defer func() { minRevocationEpoch = old }()
	minRevocationEpoch = "2"
	p := payload()
	p.RevocationEpoch = 1
	tok, _ := Sign(priv, p)
	if _, err := Verify(tok, []ed25519.PublicKey{pub}); !errors.Is(err, ErrRevoked) {
		t.Errorf("revoked: %v", err)
	}
	p.RevocationEpoch = 2
	tok, _ = Sign(priv, p)
	if _, err := Verify(tok, []ed25519.PublicKey{pub}); err != nil {
		t.Errorf("ok: %v", err)
	}
}

func TestKeysEnv(t *testing.T) {
	pub, priv := mk(t)
	t.Setenv(EnvPubKey, "bogus,"+EncodeKey(pub))
	tok, _ := Sign(priv, payload())
	if _, err := Verify(tok, nil); err != nil {
		t.Fatal(err)
	}
}

func TestStore(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "mypack")}
	if tok, err := s.Load(); tok != "" || err != nil {
		t.Fatalf("empty: %q %v", tok, err)
	}
	if err := s.Save("tok.sig"); err != nil {
		t.Fatal(err)
	}
	if tok, _ := s.Load(); tok != "tok.sig" {
		t.Errorf("load %q", tok)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(filepath.Join(s.Dir, FileName))
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Errorf("perm %v %v", st, err)
		}
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if tok, _ := s.Load(); tok != "" {
		t.Error("not cleared")
	}
}
