package storage

import "encoding/json"

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic("storage: marshal: " + err.Error())
	}
	return string(b)
}
