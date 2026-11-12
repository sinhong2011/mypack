package appservice

import (
	"context"
	"path/filepath"

	"mypack.dev/mypack/internal/domain"
)

// ---- Locate / Reconnect ---------------------------------------------------------

// RelocateInstallation implements Service: points a missing installation at
// the app's new location after verifying it is the same app.
func (s *Impl) RelocateInstallation(ctx context.Context, id, newPath, actor string) (inst domain.Installation, err error) {
	defer func() { s.audit(ctx, actor, "installation.relocate", id, err) }()
	rec, err := s.store.GetInstallation(ctx, id)
	if err != nil {
		return inst, err
	}
	abs, err := absPath(newPath)
	if err != nil {
		return inst, newErr(CodeInvalidInput, "No such app: "+newPath, "Check the path and try again.")
	}
	info, err := s.inst.ReadAppInfo(abs)
	if err != nil {
		return inst, asDomainErr(err, CodeInvalidInput)
	}
	mismatch := func(why string) error {
		return newErr(CodeNotSameApp, why, "Choose the app this entry was created from, or remove the entry from your library.")
	}
	external := rec.State == domain.InstallExternal || rec.ProjectID == ""
	if !external && !info.Managed {
		return inst, mismatch(rec.Name + " can only be reconnected to an app created by mypack.")
	}
	if info.Identifier != rec.Identifier {
		return inst, mismatch("That app is not " + rec.Name + " (its identifier does not match).")
	}
	all, _ := s.store.ListInstallations(ctx)
	for _, o := range all {
		if o.ID != rec.ID && o.Path == abs {
			return inst, newErr(CodeInvalidInput, "That app is already in your library as "+o.Name+".", "Choose a different app.")
		}
	}
	rec.Path = abs
	if info.Version != "" {
		rec.Version = info.Version
	}
	if !external {
		rec.State = domain.InstallManaged
		rec.UpdateReason = s.updateReason(ctx, &rec)
	}
	inst, err = s.store.SaveInstallation(ctx, rec)
	if err == nil {
		s.emit(Event{Kind: EventInstallChanged, InstallID: inst.ID, ProjectID: inst.ProjectID})
	}
	return inst, err
}

// ForgetInstallation implements Service: drops the library record without
// touching any file or profile data.
func (s *Impl) ForgetInstallation(ctx context.Context, id, actor string) (err error) {
	defer func() { s.audit(ctx, actor, "installation.forget", id, err) }()
	in, err := s.store.GetInstallation(ctx, id)
	if err != nil {
		return err
	}
	if err = s.store.DeleteInstallation(ctx, id); err != nil {
		return err
	}
	s.emit(Event{Kind: EventInstallChanged, InstallID: id, ProjectID: in.ProjectID})
	return nil
}

func absPath(p string) (string, error) { return filepath.Abs(expandHome(p)) }
