package i18n

import (
	"os/exec"
	"strings"
)

func systemLocale() string {
	out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return "en"
	}
	// ( "zh-Hant-HK", "en-US" )
	for _, line := range strings.Split(string(out), "\n") {
		l := strings.Trim(strings.TrimSpace(line), `",()`)
		if l != "" {
			return l
		}
	}
	return "en"
}
