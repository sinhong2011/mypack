package builder

import (
	"archive/zip"
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"mypack.dev/mypack/pkg/config"
)

func checkExec(p string) error {
	st, err := os.Stat(p)
	if err != nil {
		return fmt.Errorf("missing executable %s", filepath.Base(p))
	}
	if !st.Mode().IsRegular() || st.Size() == 0 {
		return fmt.Errorf("%s is not a valid executable file", filepath.Base(p))
	}
	if runtime.GOOS != "windows" && st.Mode()&0o111 == 0 {
		return fmt.Errorf("%s is not executable", filepath.Base(p))
	}
	return nil
}

// checkConfigFile parses mypack.json from the resources directory.
func (j *job) checkConfigFile() error {
	b, err := os.ReadFile(filepath.Join(j.resDir, "mypack.json"))
	if err != nil {
		return errors.New("missing mypack.json")
	}
	c, err := config.Parse(b)
	if err != nil {
		return fmt.Errorf("mypack.json does not parse: %w", err)
	}
	if c.App.Identifier != j.cfg.App.Identifier || c.App.Name != j.cfg.App.Name {
		return errors.New("mypack.json does not match the requested app")
	}
	if c.App.Group != nil {
		for i, m := range c.App.Group.Members {
			if m.Icon != "" && !strings.HasPrefix(m.Icon, "members/") {
				return fmt.Errorf("group member %d icon still references a path", i)
			}
		}
	}
	for i, inj := range c.WebView.Injections {
		if inj.Path != "" {
			return fmt.Errorf("injection %d still references a path", i)
		}
	}
	return nil
}

// checkBinaryArch warns when a Mach-O/ELF binary has the wrong architecture
// (non-native stubs used in tests are tolerated with a warning).
func (j *job) checkBinaryArch() {
	switch j.target.OS {
	case "darwin":
		want := map[string]macho.Cpu{"amd64": macho.CpuAmd64, "arm64": macho.CpuArm64}[j.target.Arch]
		if f, err := macho.OpenFat(j.exePath); err == nil {
			defer f.Close()
			if j.target.Arch == "universal" {
				return
			}
			for _, a := range f.Arches {
				if a.Cpu == want {
					return
				}
			}
			j.warn("The runtime binary does not contain %s code", j.target.Arch)
			return
		}
		if f, err := macho.Open(j.exePath); err == nil {
			defer f.Close()
			if j.target.Arch != "universal" && f.Cpu != want {
				j.warn("The runtime binary is for a different CPU than %s", j.target.Arch)
			}
			return
		}
		j.warn("The runtime binary is not a Mach-O executable")
	case "linux":
		want := map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64}[j.target.Arch]
		f, err := elf.Open(j.exePath)
		if err != nil {
			j.warn("The runtime binary is not an ELF executable")
			return
		}
		defer f.Close()
		if f.Machine != want {
			j.warn("The runtime binary is for a different CPU than %s", j.target.Arch)
		}
	}
}

func (j *job) verifyWindows() error {
	for _, p := range []string{"resources/mypack.json", "resources/icon.png", "resources/icon.ico"} {
		if st, err := os.Stat(filepath.Join(j.root, filepath.FromSlash(p))); err != nil || st.Size() == 0 {
			return fmt.Errorf("missing %s", p)
		}
	}
	if st, err := os.Stat(j.exePath); err != nil || st.Size() == 0 {
		return fmt.Errorf("missing %s.exe", j.name)
	}
	if err := j.checkConfigFile(); err != nil {
		return err
	}
	f, err := pe.Open(j.exePath)
	if err != nil {
		return fmt.Errorf("the executable is not a valid Windows program: %w", err)
	}
	defer f.Close()
	want := map[string]uint16{"amd64": pe.IMAGE_FILE_MACHINE_AMD64, "arm64": pe.IMAGE_FILE_MACHINE_ARM64}[j.target.Arch]
	if f.FileHeader.Machine != want {
		return fmt.Errorf("the executable is built for a different CPU (machine 0x%x, expected %s)", f.FileHeader.Machine, j.target.Arch)
	}
	return nil
}

func (j *job) verifyLinux() error {
	for _, p := range []string{"resources/mypack.json", "resources/icon.png", j.slug + ".desktop"} {
		if st, err := os.Stat(filepath.Join(j.root, filepath.FromSlash(p))); err != nil || st.Size() == 0 {
			return fmt.Errorf("missing %s", p)
		}
	}
	if err := checkExec(j.exePath); err != nil {
		return err
	}
	if err := j.checkConfigFile(); err != nil {
		return err
	}
	j.checkBinaryArch()
	return nil
}

// verifyPackage sanity-checks zip/dmg/deb outputs.
func (j *job) verifyPackage() error {
	switch j.target.Format {
	case "zip":
		zr, err := zip.OpenReader(j.artifact)
		if err != nil {
			return fmt.Errorf("the zip file is invalid: %w", err)
		}
		defer zr.Close()
		want := filepath.Base(j.root) + "/"
		found := false
		for _, f := range zr.File {
			if strings.HasPrefix(f.Name, want) && strings.HasSuffix(f.Name, "mypack.json") {
				found = true
			}
		}
		if !found {
			return errors.New("the zip file does not contain the app")
		}
	case "dmg":
		st, err := os.Stat(j.artifact)
		if err != nil || st.Size() < 4096 {
			return errors.New("the disk image is missing or empty")
		}
	case "deb":
		return verifyDeb(j.artifact)
	}
	return nil
}

func verifyDeb(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(b, []byte("!<arch>\n")) {
		return errors.New("the .deb is not an ar archive")
	}
	var names []string
	for pos := 8; pos+60 <= len(b); {
		name := strings.TrimSpace(string(b[pos : pos+16]))
		var size int
		fmt.Sscanf(strings.TrimSpace(string(b[pos+48:pos+58])), "%d", &size)
		names = append(names, name)
		pos += 60 + size + size%2
	}
	if len(names) != 3 || names[0] != "debian-binary" || names[1] != "control.tar.gz" || names[2] != "data.tar.gz" {
		return fmt.Errorf("the .deb has unexpected members %v", names)
	}
	return nil
}
