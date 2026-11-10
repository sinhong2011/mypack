package metadata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mypack.dev/mypack/internal/icons"
)

func pngOf(t *testing.T, name string) []byte {
	t.Helper()
	return icons.Default(name) // 1024px PNG
}

func TestFetchFullFlow(t *testing.T) {
	big := pngOf(t, "Acme")
	small, _ := icons.Resize(big, 64)
	mux := http.NewServeMux()
	var srvURL string
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if !strings.Contains(r.UserAgent(), "Safari") {
			t.Errorf("unexpected UA %q", r.UserAgent())
		}
		fmt.Fprint(w, `<!doctype html><html><head>
<title>Dashboard | Acme Inc</title>
<meta property="og:site_name" content="Acme">
<meta name="description" content="Acme is great">
<meta name="theme-color" content="#ABC">
<link rel="icon" href="/small.png" sizes="32x32">
<link rel="apple-touch-icon" href="/touch.png" sizes="180x180">
<link rel="manifest" href="/site.webmanifest">
</head><body>hi</body></html>`)
	})
	mux.HandleFunc("/touch.png", func(w http.ResponseWriter, r *http.Request) { w.Write(big) })
	mux.HandleFunc("/small.png", func(w http.ResponseWriter, r *http.Request) { w.Write(small) })
	mux.HandleFunc("/site.webmanifest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"name":"Acme Manifest","icons":[{"src":"/m512.png","sizes":"512x512"}]}`)
	})
	mux.HandleFunc("/m512.png", func(w http.ResponseWriter, r *http.Request) { w.Write(big) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	res, err := Fetch(context.Background(), srvURL)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fallback {
		t.Fatalf("unexpected fallback: %s", res.Warning)
	}
	if res.Title != "Acme" || res.Description != "Acme is great" || res.ThemeColor != "#aabbcc" {
		t.Fatalf("%+v", res)
	}
	if !strings.HasSuffix(res.IconURL, "/m512.png") { // largest >=180 wins
		t.Fatalf("icon url %q", res.IconURL)
	}
	img, err := icons.Decode(res.IconPNG)
	if err != nil || img.Bounds().Dx() != 512 {
		t.Fatalf("icon: %v %v", err, img)
	}
	if res.URL != srvURL+"/" {
		t.Fatalf("url %q", res.URL)
	}
}

func TestFetchTitleFromTitleTagAndFaviconICO(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			ico, _ := icons.ICO(icons.Default("q"))
			w.Write(ico)
			return
		}
		fmt.Fprint(w, `<html><head><title>  Inbox (3) — foo@bar.com — 127.0.0.1  </title></head></html>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	res, err := Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "127.0.0.1" {
		t.Fatalf("title %q", res.Title)
	}
	if res.IconPNG == nil || !strings.HasSuffix(res.IconURL, "/favicon.ico") {
		t.Fatalf("expected favicon.ico fallback, got %q", res.IconURL)
	}
}

func TestFetchNetworkFailureFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens any more
	res, err := Fetch(context.Background(), url)
	if err != nil {
		t.Fatalf("network failure must not return error: %v", err)
	}
	if !res.Fallback || res.Warning == "" || res.Title == "" {
		t.Fatalf("%+v", res)
	}
}

func TestFetchHTTPErrorFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(403)
			fmt.Fprint(w, "<title>Just a moment...</title>")
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	res, _ := Fetch(context.Background(), srv.URL)
	if !res.Fallback || res.Title == "Just a moment..." {
		t.Fatalf("%+v", res)
	}
}

func TestInvalidURL(t *testing.T) {
	for _, in := range []string{"", "javascript:alert(1)", "ftp://x.com", "file:///etc/passwd"} {
		if _, err := Fetch(context.Background(), in); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}

func TestRedirectCapAndScheme(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		http.Redirect(w, r, fmt.Sprintf("/r%d", n), http.StatusFound)
	}))
	defer srv.Close()
	res, err := Fetch(context.Background(), srv.URL)
	if err != nil || !res.Fallback {
		t.Fatalf("redirect loop should fall back: %+v %v", res, err)
	}
	if n > maxRedirects+2 {
		t.Fatalf("followed too many redirects: %d", n)
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "ftp://example.com/x", http.StatusFound)
	}))
	defer srv2.Close()
	res, err = Fetch(context.Background(), srv2.URL)
	if err != nil || !res.Fallback || !strings.Contains(res.Warning, "scheme") {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestRedirectFollowedFinalURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/new/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<title>X</title><meta name="application-name" content="NewApp">`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	res, _ := Fetch(context.Background(), srv.URL+"/old")
	if res.Title != "NewApp" || !strings.HasSuffix(res.URL, "/new/") {
		t.Fatalf("%+v", res)
	}
}

func TestBudget(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(20 * time.Second):
		}
	}))
	defer srv.Close()
	start := time.Now()
	res, err := Fetch(context.Background(), srv.URL)
	if err != nil || !res.Fallback {
		t.Fatalf("%+v %v", res, err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Fetch(ctx, "https://example.com"); err == nil {
		t.Fatal("expected ctx error")
	}
}

func TestCleanTitleTag(t *testing.T) {
	cases := []struct{ title, host, want string }{
		{"Home | Foo", "foo.com", "Foo"},
		{"GitHub: Let's build from here", "github.com", "GitHub Let's build from here"},
		{"Inbox (3) - me@x.com - Gmail", "mail.google.com", "Gmail"},
		{"How do I parse X - Stack Overflow", "stackoverflow.com", "Stack Overflow"},
		{"Dashboard - Acme", "example.com", "Acme"},
		{"Linear", "linear.app", "Linear"},
		{"Notion – The all-in-one workspace", "www.notion.so", "Notion"},
		{"Just a title", "weird.example", "Just a title"},
		{"a/b\\c", "x.com", "abc"},
	}
	for _, c := range cases {
		if got := cleanTitleTag(c.title, c.host); got != c.want {
			t.Errorf("cleanTitleTag(%q,%q)=%q want %q", c.title, c.host, got, c.want)
		}
	}
}

func TestGuessTitleFromHost(t *testing.T) {
	cases := map[string]string{
		"github.com": "GitHub", "www.youtube.com": "YouTube", "chatgpt.com": "ChatGPT",
		"mail.google.com": "Gmail", "x.com": "X", "my-notes.example.org": "Example",
		"foo.co.uk": "Foo", "my-site.io": "My Site", "localhost": "Localhost", "127.0.0.1": "127.0.0.1",
		"app.linear.app": "Linear", "web.whatsapp.com": "WhatsApp",
	}
	for in, want := range cases {
		if got := GuessTitleFromHost(in); got != want {
			t.Errorf("GuessTitleFromHost(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNormalizeColor(t *testing.T) {
	cases := map[string]string{"#fff": "#ffffff", "#1A2B3C": "#1a2b3c", "#11223344": "#112233", "rgb(255, 0, 128)": "#ff0080",
		"white": "#ffffff", "nonsense": "", "": "", "#12": ""}
	for in, want := range cases {
		if got := normalizeColor(in); got != want {
			t.Errorf("normalizeColor(%q)=%q want %q", in, got, want)
		}
	}
}

func TestIconCandidateOrder(t *testing.T) {
	doc := parseHTML([]byte(`<head>
<link rel="icon" href="/a.ico">
<link rel="icon" type="image/svg+xml" href="/a.svg">
<link rel="mask-icon" href="/m.svg">
<link rel="apple-touch-icon" href="/t.png">
<link rel="icon" sizes="192x192" href="/c192.png">
</head>`))
	base := mustURL("https://x.com/")
	c := iconCandidates(doc, nil, base)
	if len(c) < 3 || !strings.HasSuffix(c[0].u, "/c192.png") || !strings.HasSuffix(c[1].u, "/t.png") {
		t.Fatalf("%+v", c)
	}
	for _, x := range c {
		if strings.HasSuffix(x.u, ".svg") {
			t.Fatal("svg must be skipped")
		}
	}
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
