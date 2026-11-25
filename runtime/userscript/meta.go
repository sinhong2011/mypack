// Package userscript is MyPack's userscript store: scripts in the
// Greasemonkey / Tampermonkey format (a "// ==UserScript==" header) that every
// generated app with runtime 1.0.12+ runs on the pages they match.
//
// The store is one folder shared by all apps, next to routes.json:
//
//	<UserConfigDir>/mypack-apps/userscripts/
//	  index.json               the installed scripts, their state and scope
//	  <id>/script.user.js      the script
//	  <id>/require-<n>.js      @require files, downloaded at install time
//	  <id>/resource-<n>        @resource files, downloaded at install time
//
// The Builder (CLI, later the GUI) writes it; the runtime only reads it at
// launch. Nothing is fetched by a running app.
package userscript

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

// Meta is a parsed userscript header.
type Meta struct {
	Name         string     `json:"name"`
	Namespace    string     `json:"namespace,omitempty"`
	Version      string     `json:"version,omitempty"`
	Description  string     `json:"description,omitempty"`
	Author       string     `json:"author,omitempty"`
	Match        []string   `json:"match,omitempty"`
	Include      []string   `json:"include,omitempty"`
	Exclude      []string   `json:"exclude,omitempty"`
	ExcludeMatch []string   `json:"excludeMatch,omitempty"`
	RunAt        string     `json:"runAt,omitempty"` // document-start | document-end | document-idle
	Grant        []string   `json:"grant,omitempty"`
	Connect      []string   `json:"connect,omitempty"`
	Require      []string   `json:"require,omitempty"`
	Resource     []Resource `json:"resource,omitempty"`
	NoFrames     bool       `json:"noframes,omitempty"`
	UpdateURL    string     `json:"updateURL,omitempty"`
	DownloadURL  string     `json:"downloadURL,omitempty"`
	Homepage     string     `json:"homepage,omitempty"`
}

// Resource is one @resource line.
type Resource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ErrNoHeader means the source has no "// ==UserScript==" block.
var ErrNoHeader = errors.New("not a userscript: no // ==UserScript== header")

var metaLine = regexp.MustCompile(`^//\s*@([A-Za-z][\w:.-]*)(?:\s+(.*?))?\s*$`)

// Parse reads the header of a userscript.
func Parse(src string) (Meta, error) {
	var m Meta
	in, done := false, false
	sc := bufio.NewScanner(strings.NewReader(src))
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !in {
			if strings.HasPrefix(line, "//") && strings.TrimSpace(strings.TrimPrefix(line, "//")) == "==UserScript==" {
				in = true
			}
			continue
		}
		if strings.HasPrefix(line, "//") && strings.TrimSpace(strings.TrimPrefix(line, "//")) == "==/UserScript==" {
			done = true
			break
		}
		g := metaLine.FindStringSubmatch(line)
		if g == nil {
			continue
		}
		key, val := g[1], strings.TrimSpace(g[2])
		if strings.Contains(key, ":") { // localized (@name:zh-TW): the base value wins
			continue
		}
		switch key {
		case "name":
			m.Name = val
		case "namespace":
			m.Namespace = val
		case "version":
			m.Version = val
		case "description":
			m.Description = val
		case "author":
			m.Author = val
		case "match":
			add(&m.Match, val)
		case "include":
			add(&m.Include, val)
		case "exclude":
			add(&m.Exclude, val)
		case "exclude-match":
			add(&m.ExcludeMatch, val)
		case "run-at":
			m.RunAt = val
		case "grant":
			add(&m.Grant, val)
		case "connect":
			add(&m.Connect, val)
		case "require":
			add(&m.Require, val)
		case "resource":
			if f := strings.Fields(val); len(f) == 2 {
				m.Resource = append(m.Resource, Resource{Name: f[0], URL: f[1]})
			}
		case "noframes":
			m.NoFrames = true
		case "updateURL":
			m.UpdateURL = val
		case "downloadURL":
			m.DownloadURL = val
		case "homepage", "homepageURL", "website", "source":
			if m.Homepage == "" && (strings.HasPrefix(val, "https://") || strings.HasPrefix(val, "http://")) {
				m.Homepage = val
			}
		}
	}
	if !in || !done {
		return Meta{}, ErrNoHeader
	}
	if m.Name == "" {
		return Meta{}, errors.New("the userscript header has no @name")
	}
	switch m.RunAt {
	case "document-start", "document-end", "document-idle":
	case "document-body":
		m.RunAt = "document-end"
	default:
		m.RunAt = "document-idle"
	}
	return m, nil
}

func add(list *[]string, v string) {
	if v != "" {
		*list = append(*list, v)
	}
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// ID is the stable store id of a script: its name as a slug plus a hash of
// namespace and name, so an update of the same script replaces it.
func (m Meta) ID() string {
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(m.Name), "-"), "-")
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	if slug == "" {
		slug = "script"
	}
	h := sha256.Sum256([]byte(m.Namespace + "\x00" + m.Name))
	return slug + "-" + hex.EncodeToString(h[:4])
}

// Grants reports which privileged API families the script asked for, by
// normalized name: "values", "xmlhttprequest", "setclipboard".
func (m Meta) Grants() map[string]bool {
	out := map[string]bool{}
	for _, g := range m.Grant {
		n := strings.ToLower(g)
		n = strings.TrimPrefix(strings.TrimPrefix(n, "gm_"), "gm.")
		switch n {
		case "getvalue", "setvalue", "deletevalue", "listvalues":
			out["values"] = true
		case "xmlhttprequest":
			out["xmlhttprequest"] = true
		case "setclipboard":
			out["setclipboard"] = true
		}
	}
	return out
}
