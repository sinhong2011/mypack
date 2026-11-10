package runtimemanager

import (
	"crypto/ed25519"
	"encoding/base64"
)

func encodeKey(k ed25519.PublicKey) string { return base64.StdEncoding.EncodeToString(k) }
