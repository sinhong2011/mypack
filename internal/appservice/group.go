package appservice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/pkg/config"
)

// GroupRequest makes a group app from websites: Library projects and/or
// addresses, in rail order.
type GroupRequest struct {
	Name  string
	Sites []GroupSite
	Actor string
}

// GroupSite is one website of a new group: a Library project, an address
// (its name and icon are fetched), or a local folder or HTML file (Path).
type GroupSite struct {
	ProjectID string
	URL       string
	Path      string
}

// CreateGroup creates a group project: one app whose window shows the
// given website projects side by side in a rail, each with its own
// logins. The members' icons are kept beside the database (the Builder
// bundles them); the group's icon is a grid of them. It never builds.
func (s *Impl) CreateGroup(ctx context.Context, req GroupRequest) (p domain.Project, err error) {
	actor := actorOr(req.Actor, "gui")
	defer func() { s.audit(ctx, actor, "project.group", name0(req.Name), err) }()
	name := cleanName(req.Name)
	if name == "" {
		return p, &domain.Error{Code: CodeInvalidInput, Message: "Give the group a name.", Field: "name"}
	}
	if qerr := s.checkGroupQuota(ctx); qerr != nil {
		return p, qerr
	}
	if n := len(req.Sites); n < config.MinGroupMembers || n > config.MaxGroupMembers {
		return p, &domain.Error{Code: CodeInvalidInput, Field: "projectIds",
			Message: fmt.Sprintf("Pick %d to %d websites for a group.", config.MinGroupMembers, config.MaxGroupMembers)}
	}
	var (
		members []config.GroupMember
		pngs    [][]byte
		taken   = map[string]bool{}
		seen    = map[string]bool{}
	)
	for _, site := range req.Sites {
		var (
			m   config.GroupMember
			png []byte
		)
		if site.Path != "" {
			lm, lerr := s.localMember(site.Path)
			if lerr != nil {
				return p, lerr
			}
			m, png = lm, s.icons.Default(lm.Name)
			if seen[m.Local] {
				continue
			}
			seen[m.Local] = true
			m.ID = config.MemberID(m.Name, taken)
			taken[m.ID] = true
			members = append(members, m)
			pngs = append(pngs, png)
			continue
		}
		if site.ProjectID != "" {
			mp, gerr := s.store.GetProject(ctx, site.ProjectID)
			if gerr != nil {
				return p, gerr
			}
			c := mp.Config
			if c == nil || c.IsGroup() {
				return p, &domain.Error{Code: CodeInvalidInput, Field: "sites",
					Message: fmt.Sprintf("%s can't join a group: it is a group itself.", projectName(mp))}
			}
			if c.App.Source.Type == config.SourceURL {
				m = config.GroupMember{Name: c.App.Name, URL: c.App.Source.Value, ProjectID: mp.ID}
			} else {
				// A local app joins with its folder (a project's built output).
				lm, lerr := s.localMember(c.App.Source.Value)
				if lerr != nil {
					return p, lerr
				}
				if c.App.Source.Type == config.SourceDir && c.App.Source.Entry != "" {
					lm.Entry = c.App.Source.Entry
				}
				lm.Name, lm.ProjectID = c.App.Name, mp.ID
				m = lm
			}
			png = mp.IconPNG
		} else {
			u, nerr := config.NormalizeURL(strings.TrimSpace(site.URL))
			if nerr != nil {
				return p, &domain.Error{Code: CodeInvalidInput, Field: "sites", Message: fmt.Sprintf("%q is not a website address.", site.URL),
					Remediation: "Enter addresses such as slack.com."}
			}
			mctx, cancel := context.WithTimeout(ctx, 12*time.Second)
			md, ferr := s.FetchMetadata(mctx, u)
			cancel()
			title := ""
			if ferr == nil {
				title, png = md.Title, md.IconPNG
			}
			m = config.GroupMember{Name: firstNonEmpty(cleanName(title), cleanName(titleFromHost(u))), URL: u}
		}
		if seen[m.URL+m.Local] {
			continue
		}
		seen[m.URL+m.Local] = true
		m.ID = config.MemberID(m.Name, taken)
		taken[m.ID] = true
		members = append(members, m)
		pngs = append(pngs, png)
	}
	if len(members) < config.MinGroupMembers {
		return p, &domain.Error{Code: CodeInvalidInput, Field: "projectIds",
			Message: fmt.Sprintf("Add at least %d different websites.", config.MinGroupMembers)}
	}
	// The project's source: its first website, else its first folder.
	input := ""
	for _, m := range members {
		if !m.IsLocal() {
			input = m.URL
			break
		}
	}
	if input == "" {
		input = members[0].Local
	}
	md := &Metadata{URL: input, Title: name, IconPNG: icons.Group(pngs)}
	p, err = s.CreateProject(ctx, CreateRequest{Input: input, Name: name, Metadata: md, Actor: actor})
	if err != nil {
		return p, err
	}
	for i := range members {
		members[i].Icon = s.saveMemberIcon(p.ID, members[i].ID, pngs[i])
	}
	cfg := p.Config.Clone()
	cfg.App.Group = &config.GroupConfig{Members: members}
	cfg.App.Source.Project, cfg.App.Source.Build = "", nil // a group never runs a project build
	if cfg.App.Icon.Source == "default" && len(md.IconPNG) > 0 {
		cfg.App.Icon.Source = "auto"
	}
	return s.UpdateProject(ctx, UpdateRequest{ID: p.ID, ExpectedRevision: p.Revision, Config: cfg, Actor: actor})
}

// groupIconRoot is where group members' icons are kept: beside the
// database.
func (s *Impl) groupIconRoot() string {
	if s.opts.DBPath != "" {
		return filepath.Join(filepath.Dir(s.opts.DBPath), "groups")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "mypack", "groups")
}

func projectName(p domain.Project) string {
	if p.Config != nil && p.Config.App.Name != "" {
		return p.Config.App.Name
	}
	return "This app"
}

// saveMemberIcon keeps a member's icon for the Builder; "" when none.
func (s *Impl) saveMemberIcon(projectID, memberID string, png []byte) string {
	if len(png) == 0 {
		return ""
	}
	dir := filepath.Join(s.groupIconRoot(), projectID)
	if os.MkdirAll(dir, 0o755) != nil {
		return ""
	}
	f := filepath.Join(dir, memberID+".png")
	if os.WriteFile(f, png, 0o644) != nil {
		return ""
	}
	return f
}

// fillGroupIcons gives the members of a group about to be built the icon
// of their Library project when they have none (added in Settings), and
// drops icon files that are gone.
func (s *Impl) fillGroupIcons(ctx context.Context, projectID string, cfg *config.Config) {
	if !cfg.IsGroup() {
		return
	}
	for i := range cfg.App.Group.Members {
		m := &cfg.App.Group.Members[i]
		if m.Icon != "" {
			if _, err := os.Stat(m.Icon); err == nil {
				continue
			}
			m.Icon = ""
		}
		if m.ProjectID == "" {
			continue
		}
		if mp, err := s.store.GetProject(ctx, m.ProjectID); err == nil {
			m.Icon = s.saveMemberIcon(projectID, m.ID, mp.IconPNG)
		}
	}
}

func name0(n string) string { return strings.TrimSpace(n) }

// localMember makes a group member of a local folder or HTML file. A
// JavaScript project joins with its built output: building it is up to
// its own app (a group never runs build scripts).
func (s *Impl) localMember(path string) (config.GroupMember, error) {
	p := expandHome(strings.TrimSpace(path))
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	st, err := os.Stat(p)
	if err != nil {
		return config.GroupMember{}, &domain.Error{Code: CodeInvalidInput, Field: "sites", Message: "That folder or file doesn't exist: " + path}
	}
	if !st.IsDir() {
		low := strings.ToLower(p)
		if !strings.HasSuffix(low, ".html") && !strings.HasSuffix(low, ".htm") {
			return config.GroupMember{}, &domain.Error{Code: CodeInvalidInput, Field: "sites", Message: "Choose a folder or an HTML file."}
		}
		return config.GroupMember{Name: cleanName(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))), Local: p}, nil
	}
	site, proj, isProject, perr := resolveWebProject(p)
	if perr != nil {
		return config.GroupMember{}, perr
	}
	name := filepath.Base(p)
	if isProject {
		if proj.Name != "" {
			name = proj.Name
		}
		if !fileExists(filepath.Join(site, "index.html")) {
			return config.GroupMember{}, &domain.Error{Code: CodeInvalidInput, Field: "sites",
				Message:     name + " isn't built yet, so it can't join a group.",
				Remediation: "Build it once (" + firstNonEmpty(proj.BuildCmd, "npm run build") + "), or make it an app of its own first."}
		}
	} else if !fileExists(filepath.Join(site, "index.html")) {
		return config.GroupMember{}, &domain.Error{Code: CodeInvalidInput, Field: "sites", Message: name + " has no index.html.",
			Remediation: "Choose the folder that holds index.html, or the HTML file itself."}
	}
	return config.GroupMember{Name: cleanName(name), Local: site, Entry: "index.html"}, nil
}
