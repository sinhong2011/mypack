package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"mypack.dev/mypack/pkg/config"
)

// siteScheme is the custom scheme that serves bundled local content
// (macOS WKURLSchemeHandler / Windows WebView2 resource handler through
// mygo.Protocol).
const siteScheme = "mypack"

// Site is a started local-content server.
type Site struct {
	// StartURL is what the main window loads.
	StartURL string
	// Scheme is the custom scheme (protocol mode), "" in loopback mode.
	Scheme string
	// Origin is the loopback origin ("http://127.0.0.1:port"), "" in protocol mode.
	Origin string
}

// ResolveLocal turns a file/dir source into (root directory, entry file).
// Relative values resolve against the directory holding mypack.json (the
// Builder places the content at Resources/site/ and sets value="site").
func ResolveLocal(cfgDir string, src config.Source) (root, entry string, err error) {
	v := src.Value
	if v == "" {
		return "", "", fmt.Errorf("the local source path is empty")
	}
	p := v
	if !filepath.IsAbs(p) {
		p = filepath.Join(cfgDir, filepath.FromSlash(p))
	}
	st, err := os.Stat(p)
	if err != nil {
		return "", "", fmt.Errorf("the bundled website content %q is missing: %w", v, err)
	}
	switch src.Type {
	case config.SourceFile:
		if st.IsDir() { // value pointed at the directory holding the file
			e := cleanEntry(src.Entry)
			if e == "" {
				e = "index.html"
			}
			return p, e, checkEntry(p, e)
		}
		// Serve the file's directory so relative assets keep working.
		return filepath.Dir(p), filepath.Base(p), nil
	default: // dir
		if !st.IsDir() {
			return "", "", fmt.Errorf("%q is not a directory", v)
		}
		e := cleanEntry(src.Entry)
		if e == "" {
			e = "index.html"
		}
		return p, e, checkEntry(p, e)
	}
}

func cleanEntry(e string) string {
	e = strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(e, `\`, "/")), "/")
	if e == "." {
		return ""
	}
	return e
}

func checkEntry(root, entry string) error {
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(entry))); err != nil {
		return fmt.Errorf("the entry page %q was not found in the bundled website: %w", entry, err)
	}
	return nil
}

func entryURLPath(entry string) string {
	// net/http answers ".../index.html" with a 301 to "./"; a custom scheme
	// handler does not follow redirects (blank page), so never request it.
	if entry == "index.html" {
		return ""
	}
	entry = strings.TrimSuffix(entry, "index.html")
	segs := strings.Split(entry, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// StartSite serves root.
//
// Design choice: on macOS and Windows the content is served in-process
// through MyGo's custom-protocol support (mygo.Protocol + mygo.FileServer):
// no socket is opened, nothing else on the machine (or browser) can reach the
// content, and relative assets, fetch and ES modules work. On Linux custom
// schemes cannot play audio/video in WebKitGTK, so content is served by an
// in-process http server bound to 127.0.0.1 on a random port, behind a random
// 128-bit token path prefix and a Host check (defends against other local
// processes guessing the URL and against DNS rebinding).
// $MYPACK_SITE_MODE=http|protocol forces a mode (testing).
func StartSite(root, entry, goos, forceMode string) (*Site, error) {
	mode := "protocol"
	if goos == "linux" {
		mode = "http"
	}
	if forceMode == "http" || forceMode == "protocol" {
		mode = forceMode
	}
	fsys := os.DirFS(root)
	if mode == "protocol" {
		if err := mygo.Protocol.Handle(siteScheme, siteHandler(fsys)); err != nil {
			return nil, err
		}
		return &Site{Scheme: siteScheme, StartURL: siteScheme + "://localhost/" + entryURLPath(entry)}, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("cannot start the local site server: %w", err)
	}
	var tok [16]byte
	if _, err := rand.Read(tok[:]); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tok[:])
	origin := "http://" + ln.Addr().String()
	srv := &http.Server{Handler: LoopbackHandler(token, ln.Addr().String(), siteHandler(fsys))}
	go func() { _ = srv.Serve(ln) }()
	return &Site{Origin: origin, StartURL: origin + "/" + token + "/" + entryURLPath(entry)}, nil
}

// LoopbackHandler wraps h: only requests for host under /<token>/ get through.
func LoopbackHandler(token, host string, h http.Handler) http.Handler {
	prefix := "/" + token
	inner := http.StripPrefix(prefix, h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.URL.Path != prefix && !strings.HasPrefix(r.URL.Path, prefix+"/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		inner.ServeHTTP(w, r)
	})
}

// siteHandler is mygo.FileServer without redirects: WKURLSchemeHandler and
// WebView2 do not follow the 301s net/http issues for "/index.html" and for
// directories without a trailing slash, which would render a blank page.
func siteHandler(fsys fs.FS) http.Handler {
	inner := mygo.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)
		switch {
		case p == "/index.html" || strings.HasSuffix(p, "/index.html"):
			p = strings.TrimSuffix(p, "index.html")
		default:
			if st, err := fs.Stat(fsys, strings.TrimPrefix(p, "/")); err == nil && st.IsDir() && p != "/" {
				p += "/"
			}
		}
		r2 := r.Clone(r.Context())
		u := *r.URL
		u.Path, u.RawPath = p, ""
		r2.URL = &u
		inner.ServeHTTP(w, r2)
	})
}
