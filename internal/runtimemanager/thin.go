package runtimemanager

import (
	"debug/macho"
	"fmt"
	"io"
	"os"
)

// The macOS installer ships its runtime helpers as universal Mach-O files:
// `mygo build` merges resources/darwin-arm64/bin and resources/darwin-amd64/bin
// into one Contents/Resources/bin. The cache holds one runtime per key, and
// an app built for darwin-arm64 must not carry an x86_64 slice it never runs
// (a universal Full helper is about 37 MB, its arm64 slice about 17 MB). So
// seeding keeps only the slice of the key's architecture.

// machOCPU is the Mach-O CPU type of a single-arch darwin key ("darwin-arm64",
// "darwin-amd64-lite"), and false for every other key (darwin-universal too).
func machOCPU(key string) (macho.Cpu, bool) {
	osName, arch, _, ok := splitKey(key)
	if !ok || osName != "darwin" {
		return 0, false
	}
	switch arch {
	case "arm64":
		return macho.CpuArm64, true
	case "amd64":
		return macho.CpuAmd64, true
	}
	return 0, false
}

// isFat reports whether path is a universal (fat) Mach-O file.
func isFat(path string) bool {
	f, err := macho.OpenFat(path)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// copyHelper copies the bundled helper src to dst for key. A universal
// helper is cut down to the slice of key's architecture; any other file is
// copied as it is. Each slice of a universal file carries its own code
// signature, so the slice stays signed.
func copyHelper(src, dst, key string) error {
	cpu, single := machOCPU(key)
	if !single {
		return copyFile(src, dst, 0o755)
	}
	ff, err := macho.OpenFat(src)
	if err != nil { // a single-arch Mach-O (or not Mach-O at all): as it is
		return copyFile(src, dst, 0o755)
	}
	defer ff.Close()
	for _, a := range ff.Arches {
		if a.Cpu != cpu {
			continue
		}
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, io.NewSectionReader(in, int64(a.Offset), int64(a.Size))); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
	return fmt.Errorf("the bundled runtime helper has no %s slice for %s", cpu, key)
}
