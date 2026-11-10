package builder

import (
	"os"
	"path/filepath"
)

// assemble lays out the primary tree (.app bundle or portable directory) and
// copies the runtime executable into it.
func (j *job) assemble() error {
	var err error
	switch j.target.OS {
	case "darwin":
		j.root = filepath.Join(j.staging, j.name+".app")
		j.exePath = filepath.Join(j.root, "Contents", "MacOS", j.name)
		j.resDir = filepath.Join(j.root, "Contents", "Resources")
	case "windows":
		j.root = filepath.Join(j.staging, j.name)
		j.exePath = filepath.Join(j.root, j.name+".exe")
		j.resDir = filepath.Join(j.root, "resources")
	case "linux":
		j.root = filepath.Join(j.staging, j.name)
		j.exePath = filepath.Join(j.root, j.slug)
		j.resDir = filepath.Join(j.root, "resources")
	}
	if err = os.MkdirAll(j.resDir, 0o755); err != nil {
		return j.wrap(err, "BUILD_FAILED", "Cannot create the app folder", "Check free disk space.")
	}
	if err = copyFile(j.ctx, j.runtime, j.exePath, 0o755); err != nil {
		return j.wrap(err, "BUILD_FAILED", "Cannot copy the runtime into the app", "Check free disk space; the cached runtime may be damaged (delete it and retry).")
	}
	j.log("info", "Assembled %s", filepath.Base(j.root))
	return nil
}

// customize writes identity, configuration and icons.
func (j *job) customize() error {
	if err := j.writeResources(); err != nil {
		return j.wrap(err, "BUILD_FAILED", "Cannot write the app configuration", "Check free disk space.")
	}
	var err error
	switch j.target.OS {
	case "darwin":
		err = j.customizeDarwin()
	case "windows":
		err = j.customizeWindows()
	case "linux":
		err = j.customizeLinux()
	}
	return j.wrap(err, "BUILD_FAILED", "Cannot customize the app", "Check free disk space and try again.")
}

func (j *job) sign() error {
	switch j.target.OS {
	case "darwin":
		return j.signDarwin()
	}
	j.signing = "unsigned"
	j.log("info", "No code signing for %s", j.target.OS)
	return nil
}

// pack produces the final artifact (set in j.artifact) from the finished tree.
func (j *job) pack() error {
	var err error
	out := filepath.Join(j.staging, "out")
	if err = os.MkdirAll(out, 0o755); err != nil {
		return j.wrap(err, "PACKAGING_FAILED", "Cannot create a packaging folder", "Check free disk space.")
	}
	switch j.target.Format {
	case "app", "portable":
		j.artifact = j.root
		return nil
	case "zip":
		j.artifact = filepath.Join(out, j.name+".zip")
		err = j.packZip(j.root, j.artifact)
	case "dmg":
		j.artifact = filepath.Join(out, j.name+".dmg")
		err = j.packDMG(j.artifact)
	case "deb":
		j.artifact = filepath.Join(out, j.debFileName())
		err = j.packDeb(j.artifact)
	default:
		err = errUnsupported
	}
	return j.wrap(err, "PACKAGING_FAILED", "Cannot package the app as "+j.target.Format, "Try the portable format, or check free disk space.")
}

// verify checks the primary tree and the package.
func (j *job) verify() error {
	var err error
	switch j.target.OS {
	case "darwin":
		err = j.verifyDarwin()
	case "windows":
		err = j.verifyWindows()
	case "linux":
		err = j.verifyLinux()
	}
	if err == nil {
		err = j.verifyPackage()
	}
	if err != nil {
		return j.wrap(err, "BUILD_VERIFY_FAILED", "The built app failed verification", "Build again; if it keeps failing report a bug.")
	}
	j.log("info", "Verification passed")
	return nil
}
