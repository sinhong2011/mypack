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

// GroupSite is one website of a new group: a Library project, or an
// address (its name and icon are fetched).
type GroupSite struct {
	ProjectID string
	URL       string
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
		if site.ProjectID != "" {
			mp, gerr := s.store.GetProject(ctx, site.ProjectID)
			if gerr != nil {
				return p, gerr
			}
			c := mp.Config
			if c == nil || c.App.Source.Type != config.SourceURL || c.IsGroup() {
				return p, &domain.Error{Code: CodeInvalidInput, Field: "sites",
					Message: fmt.Sprintf("%s can't join a group: only website apps can.", projectName(mp))}
			}
			m = config.GroupMember{Name: c.App.Name, URL: c.App.Source.Value, ProjectID: mp.ID}
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
		if seen[m.URL] {
			continue
		}
		seen[m.URL] = true
		m.ID = config.MemberID(m.Name, taken)
		taken[m.ID] = true
		members = append(members, m)
		pngs = append(pngs, png)
	}
	if len(members) < config.MinGroupMembers {
		return p, &domain.Error{Code: CodeInvalidInput, Field: "projectIds",
			Message: fmt.Sprintf("Add at least %d different websites.", config.MinGroupMembers)}
	}
	md := &Metadata{URL: members[0].URL, Title: name, IconPNG: icons.Group(pngs)}
	p, err = s.CreateProject(ctx, CreateRequest{Input: members[0].URL, Name: name, Metadata: md, Actor: actor})
	if err != nil {
		return p, err
	}
	for i := range members {
		members[i].Icon = s.saveMemberIcon(p.ID, members[i].ID, pngs[i])
	}
	cfg := p.Config.Clone()
	cfg.App.Group = &config.GroupConfig{Members: members}
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
