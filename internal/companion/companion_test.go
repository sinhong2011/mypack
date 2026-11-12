package companion

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

type fakeSvc struct {
	appservice.Service // nil: any unexpected method panics
	projects           []domain.Project
	installs           []domain.Installation
	lang               string
	created            []appservice.CreateRequest
	opened             []string
	openErr            error
}

func (f *fakeSvc) Settings(context.Context) (appservice.Settings, error) {
	return appservice.Settings{Language: f.lang}, nil
}
func (f *fakeSvc) ListProjects(context.Context) ([]domain.Project, error) { return f.projects, nil }
func (f *fakeSvc) ListInstallations(context.Context) ([]domain.Installation, error) {
	return f.installs, nil
}
func (f *fakeSvc) GetProject(_ context.Context, id string) (domain.Project, error) {
	for _, p := range f.projects {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Project{}, &domain.Error{Code: appservice.CodeNotFound, Message: "no project"}
}
func (f *fakeSvc) FetchMetadata(_ context.Context, u string) (appservice.Metadata, error) {
	return appservice.Metadata{URL: u, Title: "Fetched", Fallback: true}, nil
}
func (f *fakeSvc) CreateProject(_ context.Context, r appservice.CreateRequest) (domain.Project, error) {
	f.created = append(f.created, r)
	name := r.Name
	if name == "" {
		name = r.Metadata.Title
	}
	cfg := &config.Config{}
	cfg.App.Name = name
	p := domain.Project{ID: "p-new", Draft: true, Config: cfg}
	f.projects = append(f.projects, p)
	return p, nil
}
func (f *fakeSvc) OpenInstallation(_ context.Context, id string) error {
	f.opened = append(f.opened, id)
	return f.openErr
}

type fakeLauncher struct{ got []string }

func (l *fakeLauncher) OpenManager(id string) error { l.got = append(l.got, id); return nil }

func proj(id, name, src string, safe ...string) domain.Project {
	cfg := &config.Config{}
	cfg.App.Name, cfg.App.ProfileID = name, "prof-"+id
	cfg.App.Source = config.Source{Type: config.SourceURL, Value: src}
	cfg.WebView.Navigation.SafeDomains = safe
	return domain.Project{ID: id, Config: cfg}
}

func newH(f *fakeSvc) (*Handler, *fakeLauncher) {
	l := &fakeLauncher{}
	return &Handler{Svc: f, Launcher: l, Limiter: NewLimiter(1000, 1000)}, l
}

func req(action string, payload interface{}) []byte {
	b, _ := json.Marshal(payload)
	r, _ := json.Marshal(Request{ProtocolVersion: 1, RequestID: "r1", Action: action, Payload: b})
	return r
}

func frame(b []byte) []byte {
	out := make([]byte, 4, 4+len(b))
	binary.LittleEndian.PutUint32(out, uint32(len(b)))
	return append(out, b...)
}

func TestHello(t *testing.T) {
	h, _ := newH(&fakeSvc{lang: "ja"})
	r := h.HandleFrame(context.Background(), req(ActionHello, map[string]string{}))
	if !r.OK || r.RequestID != "r1" {
		t.Fatalf("%+v", r)
	}
	hr := r.Result.(HelloResult)
	if hr.Locale != "ja" || hr.ProtocolVersion != 1 || len(hr.Actions) != 5 {
		t.Fatalf("%+v", hr)
	}
}

func TestErrors(t *testing.T) {
	h, _ := newH(&fakeSvc{})
	ctx := context.Background()
	cases := []struct {
		name string
		raw  []byte
		code string
	}{
		{"badjson", []byte("{nope"), ErrInvalidRequest},
		{"noaction", []byte(`{"protocolVersion":1,"requestId":"x"}`), ErrInvalidRequest},
		{"unknown", req("deleteEverything", map[string]string{}), ErrInvalidRequest},
		{"version", []byte(`{"protocolVersion":2,"requestId":"x","action":"hello"}`), ErrProtocolMismatch},
		{"jsurl", req(ActionCreateDraft, CreateDraftPayload{URL: "javascript:alert(1)"}), ErrUnsupportedURL},
		{"fileurl", req(ActionLookup, LookupPayload{URL: "file:///etc/passwd"}), ErrUnsupportedURL},
		{"chrome", req(ActionLookup, LookupPayload{URL: "chrome://settings"}), ErrUnsupportedURL},
		{"creds", req(ActionLookup, LookupPayload{URL: "https://a:b@example.com/"}), ErrUnsupportedURL},
		{"long", req(ActionCreateDraft, CreateDraftPayload{URL: "https://example.com/" + strings.Repeat("a", 2100)}), ErrUnsupportedURL},
		{"nopayload", []byte(`{"protocolVersion":1,"requestId":"x","action":"lookup"}`), ErrInvalidRequest},
		{"badid", req(ActionOpenManaged, OpenManagedPayload{InstallationID: "--args x"}), ErrInvalidRequest},
		{"missingproject", req(ActionOpenManager, OpenManagerPayload{ProjectID: "nope"}), ErrAppNotFound},
	}
	for _, c := range cases {
		r := h.HandleFrame(ctx, c.raw)
		if r.OK || r.Error == nil || r.Error.Code != c.code {
			t.Errorf("%s: %+v", c.name, r)
		}
	}
}

func TestNoServiceIsHostUnavailable(t *testing.T) {
	h := NewHandler(nil)
	if r := h.HandleFrame(context.Background(), req(ActionHello, struct{}{})); !r.OK {
		t.Fatal("hello must work without a service")
	}
	r := h.HandleFrame(context.Background(), req(ActionLookup, LookupPayload{URL: "https://example.com"}))
	if r.Error == nil || r.Error.Code != ErrHostUnavailable {
		t.Fatalf("%+v", r)
	}
}

func TestLookupMatching(t *testing.T) {
	f := &fakeSvc{
		projects: []domain.Project{
			proj("p1", "Mail", "https://mail.example.com/", "mail.example.com", "accounts.example.com"),
			proj("p2", "Other", "https://other.test/", "other.test"),
			proj("p3", "example.com display name trap", "https://unrelated.test/", "unrelated.test"),
		},
		installs: []domain.Installation{{ID: "i1", ProjectID: "p1", Name: "Mail", State: domain.InstallManaged}},
	}
	h, _ := newH(f)
	look := func(u string) LookupResult {
		r := h.HandleFrame(context.Background(), req(ActionLookup, LookupPayload{URL: u, Title: "example.com"}))
		if !r.OK {
			t.Fatalf("%s: %+v", u, r)
		}
		return r.Result.(LookupResult)
	}
	if r := look("https://mail.example.com/inbox?x=1"); r.Kind != MatchUnique || r.Matches[0].InstallationID != "i1" || r.Matches[0].ProfileID != "prof-p1" {
		t.Fatalf("%+v", r)
	}
	if r := look("https://MAIL.example.com./"); r.Kind != MatchUnique {
		t.Fatalf("case/dot: %+v", r)
	}
	if r := look("https://accounts.example.com/"); r.Kind != MatchUnique {
		t.Fatalf("safe domain: %+v", r)
	}
	if r := look("https://evil.mail.example.com.attacker.test/"); r.Kind != MatchNone {
		t.Fatalf("spoof: %+v", r)
	}
	if r := look("https://example.com/"); r.Kind != MatchNone { // title/name is never used
		t.Fatalf("parent domain: %+v", r)
	}
	if r := look("https://notmail.example.com/"); r.Kind != MatchNone {
		t.Fatalf("sibling: %+v", r)
	}
	r := look("https://other.test/")
	if r.Kind != MatchUnique || r.Matches[0].ProjectID != "p2" || r.Matches[0].InstallationID != "" {
		t.Fatalf("project-only: %+v", r)
	}
	// multiple: a second project on the same host
	f.projects = append(f.projects, proj("p4", "Mail 2", "https://mail.example.com/", "mail.example.com"))
	if r := look("https://mail.example.com/"); r.Kind != MatchMultiple || len(r.Matches) != 2 {
		t.Fatalf("multi: %+v", r)
	}
}

func TestCreateDraft(t *testing.T) {
	f := &fakeSvc{}
	h, _ := newH(f)
	r := h.HandleFrame(context.Background(), req(ActionCreateDraft, CreateDraftPayload{
		URL: "https://example.com/a#frag", Title: "  <b>Hello</b>\n\tWorld​ " + strings.Repeat("x", 300)}))
	if !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(f.created) != 1 {
		t.Fatal("no create")
	}
	c := f.created[0]
	if c.Actor != "companion" || c.Input != "https://example.com/a" {
		t.Fatalf("%+v", c)
	}
	if strings.ContainsAny(c.Name, "<>\n\t​") || len([]rune(c.Name)) > 200 || !strings.HasPrefix(c.Name, "bHello/b World x") {
		t.Fatalf("title not sanitized: %q", c.Name)
	}
	if res := r.Result.(CreateDraftResult); res.ProjectID != "p-new" || !res.Draft {
		t.Fatalf("%+v", res)
	}
}

func TestOpenManagedAndManager(t *testing.T) {
	f := &fakeSvc{projects: []domain.Project{proj("p1", "A", "https://a.test/")}}
	h, l := newH(f)
	ctx := context.Background()
	if r := h.HandleFrame(ctx, req(ActionOpenManaged, OpenManagedPayload{InstallationID: "i1"})); !r.OK || f.opened[0] != "i1" {
		t.Fatalf("%+v", r)
	}
	f.openErr = &domain.Error{Code: appservice.CodeNotFound}
	if r := h.HandleFrame(ctx, req(ActionOpenManaged, OpenManagedPayload{InstallationID: "i2"})); r.Error == nil || r.Error.Code != ErrAppNotFound {
		t.Fatalf("%+v", r)
	}
	if r := h.HandleFrame(ctx, req(ActionOpenManager, OpenManagerPayload{ProjectID: "p1"})); !r.OK || l.got[0] != "p1" {
		t.Fatalf("%+v", r)
	}
	if r := h.HandleFrame(ctx, req(ActionOpenManager, OpenManagerPayload{})); !r.OK || l.got[1] != "" {
		t.Fatalf("%+v", r)
	}
}

func TestRateLimit(t *testing.T) {
	now := time.Unix(1000, 0)
	l := NewLimiter(20, 20)
	l.now = func() time.Time { return now }
	ok := 0
	for i := 0; i < 50; i++ {
		if l.Allow() {
			ok++
		}
	}
	if ok != 20 {
		t.Fatalf("burst %d", ok)
	}
	now = now.Add(500 * time.Millisecond)
	if !l.Allow() {
		t.Fatal("should refill")
	}
	h := NewHandler(nil)
	h.Limiter = l
	l.tokens = 0
	if r := h.HandleFrame(context.Background(), req(ActionHello, struct{}{})); r.Error == nil || r.Error.Code != ErrRateLimited {
		t.Fatalf("%+v", r)
	}
}

func TestFraming(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, map[string]string{"a": "b"}, 1024); err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(buf.Bytes()[:4]) != uint32(buf.Len()-4) {
		t.Fatal("bad header")
	}
	got, err := ReadFrame(&buf, 1024)
	if err != nil || string(got) != `{"a":"b"}` {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := ReadFrame(&buf, 1024); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
	if _, err := ReadFrame(bytes.NewReader([]byte{1, 0}), 1024); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("partial header: %v", err)
	}
	if _, err := ReadFrame(bytes.NewReader(frame([]byte("abcdef"))[:7]), 1024); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("partial body: %v", err)
	}
	big := make([]byte, 4)
	binary.LittleEndian.PutUint32(big, MaxRequestBytes+1)
	if _, err := ReadFrame(bytes.NewReader(big), MaxRequestBytes); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("%v", err)
	}
	if err := WriteFrame(io.Discard, strings.Repeat("x", MaxResponseBytes), MaxResponseBytes); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("%v", err)
	}
}

func TestServePipe(t *testing.T) {
	h, _ := newH(&fakeSvc{lang: "fr"})
	in := &bytes.Buffer{}
	in.Write(frame(req(ActionHello, struct{}{})))
	in.Write(frame([]byte("garbage")))
	in.Write(frame(req(ActionLookup, LookupPayload{URL: "https://nowhere.test"})))
	var out bytes.Buffer
	if err := Serve(context.Background(), in, &out, h); err != nil {
		t.Fatal(err)
	}
	var codes []string
	for {
		b, err := ReadFrame(&out, MaxResponseBytes)
		if err != nil {
			break
		}
		var r Response
		if json.Unmarshal(b, &r) != nil {
			t.Fatal("bad json")
		}
		if r.OK {
			codes = append(codes, "ok")
		} else {
			codes = append(codes, r.Error.Code)
		}
	}
	if strings.Join(codes, ",") != "ok,INVALID_REQUEST,ok" {
		t.Fatalf("%v", codes)
	}
	// oversized request: error frame then stop
	over := make([]byte, 4)
	binary.LittleEndian.PutUint32(over, MaxRequestBytes+1)
	out.Reset()
	if err := Serve(context.Background(), bytes.NewReader(over), &out, h); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("%v", err)
	}
	b, _ := ReadFrame(&out, MaxResponseBytes)
	if !strings.Contains(string(b), ErrInvalidRequest) {
		t.Fatalf("%s", b)
	}
}

func TestResponseTooLargeBecomesError(t *testing.T) {
	f := &fakeSvc{}
	for i := 0; i < 6000; i++ {
		f.projects = append(f.projects, proj("p"+strings.Repeat("x", 100), strings.Repeat("N", 100), "https://big.test/", "big.test"))
	}
	h, _ := newH(f)
	var out bytes.Buffer
	in := bytes.NewBuffer(frame(req(ActionLookup, LookupPayload{URL: "https://big.test"})))
	if err := Serve(context.Background(), in, &out, h); err != nil {
		t.Fatal(err)
	}
	b, _ := ReadFrame(&out, MaxResponseBytes)
	if !strings.Contains(string(b), ErrInvalidRequest) || len(b) > MaxResponseBytes {
		t.Fatalf("%.200s", b)
	}
}

func TestManifest(t *testing.T) {
	if _, err := BuildManifest("relative", []string{"abcdefghijklmnopabcdefghijklmnop"}); err == nil {
		t.Fatal("relative path accepted")
	}
	if _, err := BuildManifest("/x/h", nil); err == nil {
		t.Fatal("no ids accepted")
	}
	if _, err := BuildManifest("/x/h", []string{"short"}); err == nil {
		t.Fatal("bad id accepted")
	}
	hostPath := "/x/h"
	home := "/h"
	if runtime.GOOS == "windows" {
		hostPath = `C:\x\h`
		home = `C:\h`
	}
	m, err := BuildManifest(hostPath, []string{"abcdefghijklmnopabcdefghijklmnop"})
	if err != nil || m.AllowedOrigins[0] != "chrome-extension://abcdefghijklmnopabcdefghijklmnop/" || m.Type != "stdio" || m.Name != HostName {
		t.Fatalf("%+v %v", m, err)
	}
	d, _ := ManifestDirs("darwin", home, "all")
	if d["edge"] != filepath.Join(home, "Library", "Application Support", "Microsoft Edge", "NativeMessagingHosts") {
		t.Fatal(d)
	}
	dir := t.TempDir()
	p, err := WriteManifest(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	if _, removed, err := RemoveManifest(dir); err != nil || !removed {
		t.Fatal(p, err)
	}
	if _, removed, _ := RemoveManifest(dir); removed {
		t.Fatal("second remove")
	}
}
