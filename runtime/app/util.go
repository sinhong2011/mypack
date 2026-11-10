package app

import "encoding/json"

func jsonString(s string) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}
