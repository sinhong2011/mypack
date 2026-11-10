package app

import (
	"runtime"
	"strings"
	"testing"
)

func TestUserAgent(t *testing.T) {
	if got := userAgent("Custom/1.0"); got != "Custom/1.0" {
		t.Fatalf("configured UA not kept: %q", got)
	}
	got := userAgent("")
	if runtime.GOOS == "darwin" && (!strings.Contains(got, "Version/") || !strings.Contains(got, "Safari/")) {
		t.Fatalf("darwin default must look like Safari: %q", got)
	}
}
