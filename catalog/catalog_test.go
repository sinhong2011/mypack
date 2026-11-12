package catalog

import (
	"strings"
	"testing"
)

func TestBundled(t *testing.T) {
	c, err := Parse(bundled)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.Count(string(bundled), `"url"`)
	if len(c.Apps) != raw {
		t.Fatalf("discover.json has %d entries but only %d are valid; run the test with -v and fix the file", raw, len(c.Apps))
	}
	per := map[string]int{}
	for _, a := range c.Apps {
		per[a.Category]++
	}
	for _, cat := range Categories {
		if per[cat] == 0 {
			t.Errorf("category %q has no apps", cat)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for name, src := range map[string]string{
		"version": `{"version":2,"apps":[{"name":"A","url":"https://a.example/","category":"work"}]}`,
		"empty":   `{"version":1,"apps":[]}`,
		"json":    `{`,
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	c, err := Parse([]byte(`{"version":1,"apps":[
		{"name":"Good","url":"https://good.example/","category":"work"},
		{"name":"Dup","url":"https://good.example/","category":"work"},
		{"name":"HTTP","url":"http://plain.example/","category":"work"},
		{"name":"JS","url":"javascript:alert(1)","category":"work"},
		{"name":"Creds","url":"https://u:p@x.example/","category":"work"},
		{"name":"Cat","url":"https://cat.example/","category":"games"},
		{"name":"","url":"https://noname.example/","category":"work"}
	]}`))
	if err != nil || len(c.Apps) != 1 || c.Apps[0].Name != "Good" {
		t.Fatalf("got %+v, %v", c.Apps, err)
	}
}
