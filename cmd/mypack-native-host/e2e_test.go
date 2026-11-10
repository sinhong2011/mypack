package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"mypack.dev/mypack/internal/companion"
)

// TestHostEndToEnd builds the real binary and drives it over framed stdio
// against a temporary database: hello -> createDraft -> lookup.
func TestHostEndToEnd(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "mypack-native-host")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	var in bytes.Buffer
	send := func(id, action string, payload interface{}) {
		pb, _ := json.Marshal(payload)
		if err := companion.WriteFrame(&in, companion.Request{ProtocolVersion: 1, RequestID: id, Action: action, Payload: pb}, 1<<20); err != nil {
			t.Fatal(err)
		}
	}
	send("1", "hello", map[string]string{})
	send("2", "createDraft", map[string]string{"url": "https://e2e-example.invalid/app", "title": "E2E Example"})
	send("3", "lookup", map[string]string{"url": "https://e2e-example.invalid/other/page"})
	send("4", "lookup", map[string]string{"url": "https://different.invalid/"})
	send("5", "createDraft", map[string]string{"url": "javascript:alert(1)"})

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "MYPACK_DB="+filepath.Join(tmp, "mypack.db"), "MYPACK_CACHE_DIR="+filepath.Join(tmp, "cache"))
	cmd.Stdin = &in
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("host: %v\nstderr: %s", err, errb.String())
	}
	var resps []map[string]interface{}
	for {
		b, err := companion.ReadFrame(&out, companion.MaxResponseBytes)
		if err != nil {
			break
		}
		var m map[string]interface{}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		resps = append(resps, m)
	}
	if len(resps) != 5 {
		t.Fatalf("got %d responses: %v\nstderr: %s", len(resps), resps, errb.String())
	}
	for i, id := range []string{"1", "2", "3", "4", "5"} {
		if resps[i]["requestId"] != id {
			t.Fatalf("order/id mismatch %d: %v", i, resps[i])
		}
	}
	if resps[0]["ok"] != true || resps[0]["result"].(map[string]interface{})["protocolVersion"].(float64) != 1 {
		t.Fatalf("hello: %v", resps[0])
	}
	if resps[1]["ok"] != true {
		t.Fatalf("createDraft: %v", resps[1])
	}
	cr := resps[1]["result"].(map[string]interface{})
	if cr["draft"] != true || cr["projectId"] == "" || cr["name"] != "E2E Example" {
		t.Fatalf("createDraft result: %v", cr)
	}
	lr := resps[2]["result"].(map[string]interface{})
	ms := lr["matches"].([]interface{})
	if lr["match"] != "unique" || len(ms) != 1 || ms[0].(map[string]interface{})["projectId"] != cr["projectId"] ||
		ms[0].(map[string]interface{})["state"] != "draft" {
		t.Fatalf("lookup: %v", lr)
	}
	if resps[3]["result"].(map[string]interface{})["match"] != "none" {
		t.Fatalf("lookup none: %v", resps[3])
	}
	if e := resps[4]["error"].(map[string]interface{}); resps[4]["ok"] == true || e["code"] != "UNSUPPORTED_URL" {
		t.Fatalf("bad url: %v", resps[4])
	}
}

func TestInstallManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var o, e bytes.Buffer
	if code := install([]string{"--extension-id", "abcdefghijklmnopabcdefghijklmnop", "--browser", "chrome"}, &o, &e); code != 0 {
		t.Fatalf("%d %s", code, e.String())
	}
	if code := install([]string{"--browser", "chrome"}, &o, &e); code == 0 {
		t.Fatal("missing id accepted")
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		dirs, _ := companion.ManifestDirs(runtime.GOOS, home, "chrome")
		b, err := os.ReadFile(filepath.Join(dirs["chrome"], companion.HostName+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var m companion.Manifest
		if json.Unmarshal(b, &m) != nil || m.Type != "stdio" || !filepath.IsAbs(m.Path) ||
			m.AllowedOrigins[0] != "chrome-extension://abcdefghijklmnopabcdefghijklmnop/" {
			t.Fatalf("%s", b)
		}
	}
}
