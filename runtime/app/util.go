package app

import (
	"encoding/base64"
	"encoding/json"
)

func jsonString(s string) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}

func decodeBase64(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
