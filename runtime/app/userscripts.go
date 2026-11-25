package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"mypack.dev/mypack/runtime/inject"
	"mypack.dev/mypack/runtime/userscript"
)

// Limits of the GM_* bridge.
const (
	gmMaxValueBytes  = 1 << 20  // one GM_setValue
	gmMaxTotalValues = 10 << 20 // all values of one script
	gmMaxXhrBody     = 20 << 20 // GM_xmlhttpRequest response
	gmMaxXhrUpload   = 20 << 20
	gmMaxClipboard   = 1 << 20
	gmXhrTimeout     = 60 * time.Second
)

// gmScript is one userscript loaded in this launch.
type gmScript struct {
	id      string
	name    string
	grants  map[string]bool
	connect []string
}

// gmService backs the GM_* API of userscripts. Scripts are known by the
// token the preload passes; page code never sees the tokens.
type gmService struct {
	dir     string // <profile>/userscripts: per-script values of this app
	client  *http.Client
	scripts map[string]*gmScript // by token
	clip    func(text string)

	mu   sync.Mutex
	vals map[string]map[string]json.RawMessage // by script id, loaded lazily
}

// loadUserScripts reads the userscripts that run in this app and returns
// them for the preload; scripts that fail to load are logged and skipped.
func (a *App) loadUserScripts(profile string) []inject.UserScript {
	st := userscript.Store{Dir: userscript.Dir()}
	loaded, errs := st.Load(a.cfg.App.Identifier)
	for _, err := range errs {
		log.Printf("mypack-runtime: userscripts: %v", err)
	}
	// Settings → Extensions: the ones this app turned off for itself.
	prefs := loadExtPrefs(profile)
	a.extRunning = map[string]bool{}
	var scripts []userscript.Script
	for _, s := range loaded {
		if prefs.off(s.ID) {
			log.Printf("mypack-runtime: userscripts: %s is off in this app", s.ID)
			continue
		}
		scripts = append(scripts, s)
		a.extRunning[s.ID] = true
	}
	if len(scripts) == 0 {
		return nil
	}
	gm := &gmService{
		dir:     filepath.Join(profile, "userscripts"),
		client:  &http.Client{Timeout: gmXhrTimeout},
		scripts: map[string]*gmScript{},
		vals:    map[string]map[string]json.RawMessage{},
		clip:    func(s string) { mygo.Clipboard.WriteText(s) },
	}
	out := make([]inject.UserScript, 0, len(scripts))
	for _, s := range scripts {
		tok := newToken()
		gm.scripts[tok] = &gmScript{id: s.ID, name: s.Name, grants: s.Meta.Grants(), connect: s.Meta.Connect}
		us := inject.UserScript{
			Token: tok, Name: s.Name, Namespace: s.Meta.Namespace, Version: s.Meta.Version, Description: s.Meta.Description,
			RunAt: s.Meta.RunAt, NoFrames: s.Meta.NoFrames, Code: s.Code, Requires: s.Requires,
		}
		for _, r := range s.Include {
			us.Include = append(us.Include, inject.URLRule{Source: r.Source, IgnoreCase: r.IgnoreCase})
		}
		for _, r := range s.Exclude {
			us.Exclude = append(us.Exclude, inject.URLRule{Source: r.Source, IgnoreCase: r.IgnoreCase})
		}
		for _, r := range s.Resources {
			us.Resources = append(us.Resources, inject.Resource{Name: r.Name, MIME: r.MIME, Data: r.Data})
		}
		out = append(out, us)
		log.Printf("mypack-runtime: userscripts: loaded %s (%s)", s.ID, s.Meta.Version)
	}
	if a.bridge != nil {
		a.bridge.gm = gm
	}
	return out
}

func newToken() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// script resolves a call: the token must be known and the calling page part
// of the app.
func (b *bridgeService) gmScript(ctx context.Context, token string) (*gmScript, string, error) {
	if b.gm == nil {
		return nil, "", errors.New("no userscripts in this app")
	}
	s := b.gm.scripts[token]
	if s == nil {
		return nil, "", errors.New("unknown userscript")
	}
	_, raw := b.caller(ctx)
	if !b.isApp(raw) {
		return nil, "", errors.New("blocked: the page is not part of the app")
	}
	return s, raw, nil
}

// GMValues returns all values of the calling script.
func (b *bridgeService) GMValues(ctx context.Context, token string) (map[string]json.RawMessage, error) {
	s, _, err := b.gmScript(ctx, token)
	if err != nil {
		return nil, err
	}
	if !s.grants["values"] {
		return map[string]json.RawMessage{}, nil
	}
	b.gm.mu.Lock()
	defer b.gm.mu.Unlock()
	return b.gm.values(s.id), nil
}

// GMSetValue stores one value (JSON text) of the calling script.
func (b *bridgeService) GMSetValue(ctx context.Context, token, key, value string) error {
	s, _, err := b.gmScript(ctx, token)
	if err != nil {
		return err
	}
	if !s.grants["values"] {
		return errors.New("GM_setValue needs @grant GM_setValue")
	}
	if len(value) > gmMaxValueBytes || !json.Valid([]byte(value)) {
		return errors.New("the value is not JSON or larger than 1 MB")
	}
	b.gm.mu.Lock()
	defer b.gm.mu.Unlock()
	v := b.gm.values(s.id)
	v[key] = json.RawMessage(value)
	total := 0
	for k, x := range v {
		total += len(k) + len(x)
	}
	if total > gmMaxTotalValues {
		delete(v, key)
		return errors.New("the script's values are larger than 10 MB")
	}
	return b.gm.save(s.id)
}

// GMDeleteValue removes one value of the calling script.
func (b *bridgeService) GMDeleteValue(ctx context.Context, token, key string) error {
	s, _, err := b.gmScript(ctx, token)
	if err != nil {
		return err
	}
	if !s.grants["values"] {
		return errors.New("GM_deleteValue needs @grant GM_deleteValue")
	}
	b.gm.mu.Lock()
	defer b.gm.mu.Unlock()
	delete(b.gm.values(s.id), key)
	return b.gm.save(s.id)
}

// values returns the (loaded) values of script id; mu must be held.
func (g *gmService) values(id string) map[string]json.RawMessage {
	if v, ok := g.vals[id]; ok {
		return v
	}
	v := map[string]json.RawMessage{}
	if b, err := os.ReadFile(g.valuesPath(id)); err == nil {
		if err := json.Unmarshal(b, &v); err != nil {
			log.Printf("mypack-runtime: userscripts: values of %s: %v", id, err)
			v = map[string]json.RawMessage{}
		}
	}
	g.vals[id] = v
	return v
}

func (g *gmService) valuesPath(id string) string {
	return filepath.Join(g.dir, filepath.Base(id)+".json")
}

// save writes the values of script id; mu must be held.
func (g *gmService) save(id string) error {
	b, err := json.Marshal(g.vals[id])
	if err != nil {
		return err
	}
	p := g.valuesPath(id)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// GMXhrRequest is what GM_xmlhttpRequest posts.
type GMXhrRequest struct {
	Method           string            `json:"method"`
	URL              string            `json:"url"`
	Headers          map[string]string `json:"headers"`
	Data             string            `json:"data"`
	DataBase64       string            `json:"dataBase64"`
	Timeout          int               `json:"timeout"` // ms
	ResponseType     string            `json:"responseType"`
	OverrideMimeType string            `json:"overrideMimeType"`
}

// GMXhrResponse is what it gets back.
type GMXhrResponse struct {
	Status     int    `json:"status"`
	StatusText string `json:"statusText"`
	Headers    string `json:"headers"`
	FinalURL   string `json:"finalUrl"`
	MIME       string `json:"mime,omitempty"`
	Text       string `json:"text,omitempty"`
	BodyBase64 string `json:"bodyBase64,omitempty"`
}

var gmMethods = map[string]bool{"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "OPTIONS": true}

// GMXhr makes a cross-origin request for a script. The request carries no
// cookies of the app (it is made by the runtime, not the web view), and its
// host must be the page's own host or listed in the script's @connect.
func (b *bridgeService) GMXhr(ctx context.Context, token string, r GMXhrRequest) (GMXhrResponse, error) {
	s, pageURL, err := b.gmScript(ctx, token)
	if err != nil {
		return GMXhrResponse{}, err
	}
	if !s.grants["xmlhttprequest"] {
		return GMXhrResponse{}, errors.New("GM_xmlhttpRequest needs @grant GM_xmlhttpRequest")
	}
	method := strings.ToUpper(r.Method)
	if method == "" {
		method = http.MethodGet
	}
	if !gmMethods[method] {
		return GMXhrResponse{}, fmt.Errorf("method %s is not allowed", method)
	}
	u, err := url.Parse(r.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return GMXhrResponse{}, fmt.Errorf("%q is not an http(s) URL", r.URL)
	}
	if !connectAllowed(s.connect, pageURL, u.Hostname()) {
		return GMXhrResponse{}, fmt.Errorf("connect-denied: %s is not in the script's @connect", u.Hostname())
	}
	var body io.Reader
	switch {
	case r.DataBase64 != "":
		raw, err := base64.StdEncoding.DecodeString(r.DataBase64)
		if err != nil || len(raw) > gmMaxXhrUpload {
			return GMXhrResponse{}, errors.New("the request body is invalid or too large")
		}
		body = bytes.NewReader(raw)
	case r.Data != "":
		if len(r.Data) > gmMaxXhrUpload {
			return GMXhrResponse{}, errors.New("the request body is too large")
		}
		body = strings.NewReader(r.Data)
	}
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(r.Timeout)*time.Millisecond)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return GMXhrResponse{}, err
	}
	for k, v := range r.Headers {
		switch strings.ToLower(k) {
		case "host", "content-length", "connection", "transfer-encoding":
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := b.gm.client.Do(req)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() || errors.Is(err, context.DeadlineExceeded) {
			return GMXhrResponse{}, errors.New("timeout")
		}
		return GMXhrResponse{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, gmMaxXhrBody+1))
	if err != nil {
		return GMXhrResponse{}, err
	}
	if len(data) > gmMaxXhrBody {
		return GMXhrResponse{}, errors.New("the response is larger than 20 MB")
	}
	out := GMXhrResponse{Status: resp.StatusCode, StatusText: strings.TrimSpace(strings.TrimPrefix(resp.Status, fmt.Sprint(resp.StatusCode))),
		Headers: formatHeaders(resp.Header), FinalURL: resp.Request.URL.String()}
	ct := resp.Header.Get("Content-Type")
	if r.OverrideMimeType != "" {
		ct = r.OverrideMimeType
	}
	out.MIME, _, _ = mime.ParseMediaType(ct)
	switch strings.ToLower(r.ResponseType) {
	case "arraybuffer", "blob":
		out.BodyBase64 = base64.StdEncoding.EncodeToString(data)
	default:
		out.Text = string(data)
	}
	return out, nil
}

// connectAllowed: the page's own host (and its subdomains), or a @connect
// entry ("*" allows every host; "self" is the page's host).
func connectAllowed(connect []string, pageURL, host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	under := func(h, d string) bool {
		d = strings.ToLower(strings.TrimSpace(d))
		return d != "" && (h == d || strings.HasSuffix(h, "."+d))
	}
	if p, err := url.Parse(pageURL); err == nil && under(host, p.Hostname()) {
		return true
	}
	for _, c := range connect {
		if c == "*" || under(host, c) {
			return true
		}
	}
	return false
}

func formatHeaders(h http.Header) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		for _, v := range h[k] {
			b.WriteString(strings.ToLower(k))
			b.WriteString(": ")
			b.WriteString(v)
			b.WriteString("\r\n")
		}
	}
	return b.String()
}

// GMSetClipboard copies text for a script.
func (b *bridgeService) GMSetClipboard(ctx context.Context, token, text string) error {
	s, _, err := b.gmScript(ctx, token)
	if err != nil {
		return err
	}
	if !s.grants["setclipboard"] {
		return errors.New("GM_setClipboard needs @grant GM_setClipboard")
	}
	if len(text) > gmMaxClipboard {
		return errors.New("the text is larger than 1 MB")
	}
	b.gm.clip(text)
	return nil
}
