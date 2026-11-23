package app

import (
	"encoding/json"
	"strings"

	_ "embed"
)

//go:embed settings.html
var settingsTemplate string

func settingsHTML(boot settingsBoot) string {
	b, err := json.Marshal(boot)
	if err != nil {
		b = []byte("{}")
	}
	return strings.Replace(settingsTemplate, "{{BOOT}}", string(b), 1)
}
