package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mypack.dev/mypack/internal/icons"
)

// withProviders points both providers at srv for the test.
func withProviders(t *testing.T, srv *httptest.Server) {
	t.Helper()
	old := providerBases
	providerBases = map[IconProvider]string{
		IconGoogle:     srv.URL + "/s2/favicons",
		IconDuckDuckGo: srv.URL + "/ip3/",
	}
	t.Cleanup(func() { providerBases = old })
}

func TestFetchIconFromProviders(t *testing.T) {
	icon := pngOf(t, "Acme")
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.String())
		switch {
		// Google: only the registrable domain is known.
		case r.URL.Path == "/s2/favicons" && r.URL.Query().Get("domain") == "acme.example":
			w.Write(icon)
		// DuckDuckGo: the full host is known.
		case r.URL.Path == "/ip3/www.acme.example.ico":
			w.Write(icon)
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write(icon) // Google's generic globe comes with a 404
		}
	}))
	defer srv.Close()
	withProviders(t, srv)

	for _, p := range []IconProvider{IconGoogle, IconDuckDuckGo} {
		asked = nil
		b, err := FetchIconFrom(context.Background(), "https://www.acme.example/dashboard?x=1", p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		img, err := icons.Decode(b)
		if err != nil || img.Bounds().Dx() != img.Bounds().Dy() || img.Bounds().Dx() > iconOutputMax {
			t.Fatalf("%s: bad icon %v %v", p, img.Bounds(), err)
		}
		for _, a := range asked {
			if strings.Contains(a, "dashboard") || strings.Contains(a, "x=1") {
				t.Errorf("%s: sent more than the host: %s", p, a)
			}
		}
	}
}

func TestFetchIconFromNone(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	withProviders(t, srv)
	if _, err := FetchIconFrom(context.Background(), "https://nothing.example/", IconDuckDuckGo); !errors.Is(err, ErrNoProviderIcon) {
		t.Fatalf("got %v", err)
	}
	if _, err := FetchIconFrom(context.Background(), "https://nothing.example/", "bing"); err == nil || errors.Is(err, ErrNoProviderIcon) {
		t.Fatalf("unknown provider: got %v", err)
	}
}
