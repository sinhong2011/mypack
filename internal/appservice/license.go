package appservice

import (
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/internal/licensing"
)

// Licensing error codes (spec §9E.3: structured entitlement errors shared by
// GUI, CLI and MCP).
const (
	CodeLicenseLimit   = "LICENSE_LIMIT"
	CodeLicenseInvalid = "LICENSE_INVALID"
	CodeNotSameApp     = "NOT_SAME_APP"
)

// FreeManagedApps is the Free edition's managed-app allowance (spec §9E.1,
// a commercial proposal that may change).
const FreeManagedApps = 2

// FreeGroups is how many group apps the Free edition may have in its
// Library; Pro has no limit.
const FreeGroups = 1

// LicenseStatus describes the local entitlement. Secrets (the token) are
// never included.
type LicenseStatus struct {
	Edition     string `json:"edition"` // "free" | "pro"
	Email       string `json:"email,omitempty"`
	LicenseID   string `json:"licenseId,omitempty"`
	ManagedApps int    `json:"managedApps"`
	// FreeLimit is the managed-app cap of the Free edition; 0 means unlimited
	// (Pro).
	FreeLimit int `json:"freeLimit"`
	// Groups is the number of group apps in the Library; FreeGroups their
	// cap in the Free edition (0 = unlimited, Pro).
	Groups     int `json:"groups"`
	FreeGroups int `json:"freeGroups"`
	// Valid is true when a stored token verified. Free users have Valid=false
	// and an empty Error; a stored-but-invalid token leaves Error set and the
	// edition at "free".
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

// licenseKeys returns the issuer keys in effect (nil = built-in + env).
func (s *Impl) licenseKeys() []ed25519.PublicKey { return s.opts.LicenseKeys }

// countManagedApps counts installed apps created by mypack. Missing and
// read-only (imported) apps do not use up the allowance.
func (s *Impl) countManagedApps(ctx context.Context) int {
	all, err := s.ListInstallations(ctx)
	if err != nil {
		return 0
	}
	n := 0
	for _, in := range all {
		if in.State == domain.InstallManaged || in.State == domain.InstallAttention {
			n++
		}
	}
	return n
}

// License implements Service.
func (s *Impl) License(ctx context.Context) (LicenseStatus, error) {
	st := LicenseStatus{Edition: licensing.EditionFree, FreeLimit: FreeManagedApps, ManagedApps: s.countManagedApps(ctx),
		FreeGroups: FreeGroups, Groups: s.countGroups(ctx)}
	tok, err := s.lic.Load()
	if err != nil {
		st.Error = "The saved license could not be read."
		return st, nil
	}
	if tok == "" {
		// Development convenience: a token supplied through the environment
		// (see `make dev-license`). It is verified like any other token, so
		// it grants nothing without a key this build trusts.
		tok = strings.TrimSpace(os.Getenv("MYPACK_LICENSE"))
	}
	if tok == "" {
		return st, nil
	}
	p, verr := licensing.Verify(tok, s.licenseKeys())
	if verr != nil {
		st.Error = verr.Error()
		return st, nil
	}
	st.Edition, st.Valid, st.FreeLimit, st.FreeGroups = p.Edition, true, 0, 0
	st.Email, st.LicenseID = p.Email, p.LicenseID
	return st, nil
}

// ActivateLicense implements Service: verifies the token locally and stores
// it. No network is involved.
func (s *Impl) ActivateLicense(ctx context.Context, token, actor string) (st LicenseStatus, err error) {
	var id string
	defer func() { s.audit(ctx, actor, "license.activate", id, err) }()
	token = strings.Join(strings.Fields(token), "")
	if token == "" {
		return st, newErr(CodeLicenseInvalid, "Paste your license key to activate it.", "Copy the key from your purchase email.")
	}
	p, verr := licensing.Verify(token, s.licenseKeys())
	if verr != nil {
		return st, licenseInvalid(verr)
	}
	id = p.LicenseID
	if err = s.lic.Save(token); err != nil {
		return st, newErr(CodeInternal, "The license could not be saved.", "Check that the mypack settings folder is writable.")
	}
	return s.License(ctx)
}

// DeactivateLicense implements Service: removes the stored token. Existing
// apps keep working; nothing else changes.
func (s *Impl) DeactivateLicense(ctx context.Context, actor string) (err error) {
	defer func() { s.audit(ctx, actor, "license.deactivate", "", err) }()
	return s.lic.Clear()
}

func licenseInvalid(err error) *domain.Error {
	rem := "Check that you copied the whole key, or contact support to recover your license."
	switch {
	case errors.Is(err, licensing.ErrNoKeys):
		rem = "This build of mypack cannot verify license keys. Install an official release."
	case errors.Is(err, licensing.ErrRevoked):
		rem = "Contact support to get a replacement key."
	}
	msg := err.Error()
	return newErr(CodeLicenseInvalid, strings.ToUpper(msg[:1])+msg[1:]+".", rem)
}

// checkManagedQuota is the single place the Free-edition allowance is
// enforced. It only applies to installing a NEW managed app; builds,
// updates, repairs, uninstalls and exports are never gated.
func (s *Impl) checkManagedQuota(ctx context.Context) error {
	st, _ := s.License(ctx)
	if st.Valid || st.ManagedApps < FreeManagedApps {
		return nil
	}
	return newErr(CodeLicenseLimit,
		"The free edition includes up to 2 managed apps, and you have 2.",
		"Manage or uninstall an existing app, export its config, or enter a license key to add more.")
}

// countGroups counts the group apps in the Library (not in the Trash).
func (s *Impl) countGroups(ctx context.Context) int {
	ps, err := s.store.ListProjects(ctx, false)
	if err != nil {
		return 0
	}
	n := 0
	for _, p := range ps {
		if p.Config != nil && p.Config.IsGroup() {
			n++
		}
	}
	return n
}

// checkGroupQuota gates making a new group app: the Free edition has
// FreeGroups of them, Pro is unlimited. Editing, building and installing a
// group are never gated.
func (s *Impl) checkGroupQuota(ctx context.Context) error {
	st, _ := s.License(ctx)
	if st.Valid || st.Groups < FreeGroups {
		return nil
	}
	return newErr(CodeLicenseLimit,
		"The free edition includes 1 group app, and you have one.",
		"Add sites to your existing group, or enter a license key for unlimited groups.")
}

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
