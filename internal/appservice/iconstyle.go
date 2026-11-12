package appservice

import (
	"context"
	"os"
	"path/filepath"
	"reflect"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/icons"
	"mypack.dev/mypack/pkg/config"
)

// The icon studio (app.icon.style). A styled project's IconPNG is the
// finished icon, so the Library, the Dock preview and installed apps all
// show it with no extra step; the image beneath the style (the website's
// icon, or one the user chose) is kept as a file beside the database, so
// the style can be changed or removed later, and builds draw the style
// over it (the Builder applies app.icon.style itself, also for CLI builds).

func (s *Impl) iconBasePath(projectID string) string {
	return filepath.Join(filepath.Dir(s.groupIconRoot()), "icons", projectID+".png")
}

// iconBase is the project's own image: what the style is drawn over.
func (s *Impl) iconBase(p domain.Project) []byte {
	if p.Config == nil || p.Config.App.Icon.Style == nil {
		return p.IconPNG
	}
	if b, err := os.ReadFile(s.iconBasePath(p.ID)); err == nil {
		return b
	}
	return p.IconPNG
}

// saveIconBase keeps base as the project's image beneath its style.
func (s *Impl) saveIconBase(projectID string, base []byte) {
	path := s.iconBasePath(projectID)
	if len(base) == 0 {
		_ = os.Remove(path)
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		_ = os.WriteFile(path, base, 0o644)
	}
}

// restyle sets p.IconPNG from base and p's style (base itself when the
// project has no style), and keeps base for later.
func (s *Impl) restyle(p *domain.Project, base []byte) {
	st := p.Config.App.Icon.Style
	if st == nil {
		if len(base) > 0 {
			p.IconPNG = base
		}
		_ = os.Remove(s.iconBasePath(p.ID))
		return
	}
	s.saveIconBase(p.ID, base)
	if out, err := icons.Styled(base, *st, p.Config.App.Name, icons.CanvasSize); err == nil {
		p.IconPNG = out
	}
}

// styleChanged reports whether going from cur to next changes how the
// styled icon looks (its style, or the name its letter comes from).
func styleChanged(cur, next *config.Config) bool {
	a, b := cur.App.Icon.Style, next.App.Icon.Style
	if !reflect.DeepEqual(a, b) {
		return true
	}
	return b != nil && cur.App.Name != next.App.Name
}

// ProjectIconImage implements Service.
func (s *Impl) ProjectIconImage(ctx context.Context, id string) ([]byte, error) {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.iconBase(p), nil
}
