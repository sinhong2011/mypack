package metadata

import (
	"context"
	"os"
	"time"
	"testing"
)

// TestProbeManual fetches MYPACK_PROBE_URL for diagnosis (skipped otherwise).
func TestProbeManual(t *testing.T) {
	u := os.Getenv("MYPACK_PROBE_URL")
	if u == "" {
		t.Skip("set MYPACK_PROBE_URL")
	}
	start := time.Now()
	r, err := Fetch(context.Background(), u)
	t.Logf("took %v", time.Since(start))
	t.Logf("err=%v url=%s title=%q icon=%q pngBytes=%d fallback=%v warning=%q theme=%q", err, r.URL, r.Title, r.IconURL, len(r.IconPNG), r.Fallback, r.Warning, r.ThemeColor)
}
