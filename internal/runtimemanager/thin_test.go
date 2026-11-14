package runtimemanager

import (
	"bytes"
	"context"
	"debug/macho"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mypack.dev/mypack/internal/runtimemanager/testregistry"
)

// thinSlice is a minimal 64-bit Mach-O file (header only) for cpu, padded
// with tag so slices differ.
func thinSlice(cpu macho.Cpu, tag string) []byte {
	var b bytes.Buffer
	for _, v := range []uint32{macho.Magic64, uint32(cpu), 0, uint32(macho.TypeExec), 0, 0, 0, 0} {
		_ = binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString(tag)
	return b.Bytes()
}

// fatFile is a universal Mach-O holding the given slices, as lipo makes it.
func fatFile(slices map[macho.Cpu][]byte, order ...macho.Cpu) []byte {
	const align = 1 << 12
	var hdr bytes.Buffer
	_ = binary.Write(&hdr, binary.BigEndian, uint32(macho.MagicFat))
	_ = binary.Write(&hdr, binary.BigEndian, uint32(len(order)))
	off := uint32(align)
	for _, cpu := range order {
		s := slices[cpu]
		for _, v := range []uint32{uint32(cpu), 0, off, uint32(len(s)), 12} {
			_ = binary.Write(&hdr, binary.BigEndian, v)
		}
		off += (uint32(len(s)) + align - 1) / align * align
	}
	out := make([]byte, off)
	copy(out, hdr.Bytes())
	off = align
	for _, cpu := range order {
		copy(out[off:], slices[cpu])
		off += (uint32(len(slices[cpu])) + align - 1) / align * align
	}
	return out
}

func TestCopyHelperThinsUniversal(t *testing.T) {
	arm, amd := thinSlice(macho.CpuArm64, "arm"), thinSlice(macho.CpuAmd64, "intel")
	dir := t.TempDir()
	src := filepath.Join(dir, "fat")
	if err := os.WriteFile(src, fatFile(map[macho.Cpu][]byte{macho.CpuAmd64: amd, macho.CpuArm64: arm}, macho.CpuAmd64, macho.CpuArm64), 0o755); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string][]byte{"darwin-arm64": arm, "darwin-arm64-lite": arm, "darwin-amd64": amd} {
		dst := filepath.Join(dir, key)
		if err := copyHelper(src, dst, key); err != nil {
			t.Fatal(key, err)
		}
		got, _ := os.ReadFile(dst)
		if !bytes.Equal(got, want) || isFat(dst) {
			t.Fatalf("%s: got %d bytes, want the %d-byte slice", key, len(got), len(want))
		}
	}
	// darwin-universal keeps both slices.
	dst := filepath.Join(dir, "universal")
	if err := copyHelper(src, dst, "darwin-universal"); err != nil || !isFat(dst) {
		t.Fatalf("universal: %v", err)
	}
	// A universal helper without the key's slice is an error.
	only := filepath.Join(dir, "armonly")
	if err := os.WriteFile(only, fatFile(map[macho.Cpu][]byte{macho.CpuArm64: arm}, macho.CpuArm64), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyHelper(only, filepath.Join(dir, "x"), "darwin-amd64"); err == nil {
		t.Fatal("missing slice should fail")
	}
}

// A cache seeded by MyPack 0.5.0 holds the universal helper under a
// single-arch key. It must fail verification and be replaced by the slice
// on the next seed.
func TestSeedReplacesUniversalCache(t *testing.T) {
	reg, err := testregistry.Create(t.TempDir(), testregistry.Options{Keys: []string{"darwin-arm64"}})
	if err != nil {
		t.Fatal(err)
	}
	arm, amd := thinSlice(macho.CpuArm64, "arm"), thinSlice(macho.CpuAmd64, "intel")
	fat := fatFile(map[macho.Cpu][]byte{macho.CpuAmd64: amd, macho.CpuArm64: arm}, macho.CpuAmd64, macho.CpuArm64)
	root := t.TempDir()
	runtimes, bin := filepath.Join(root, "runtimes"), filepath.Join(root, "bin")
	for _, d := range []string{runtimes, bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"registry.json", "registry.json.sig"} {
		b, _ := os.ReadFile(filepath.Join(reg.Dir, name))
		if err := os.WriteFile(filepath.Join(runtimes, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(runtimes, "bundled.json"), []byte(`{"version":"1.0.0","keys":["darwin-arm64"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "mypack-runtime"), fat, 0o755); err != nil {
		t.Fatal(err)
	}
	m := New(t.TempDir(), "http://127.0.0.1:1/registry.json")
	m.SetTrustedKeys(reg.Public)

	// What 0.5.0 left behind: the universal file, with a valid marker.
	if err := m.SeedFromDir(runtimes); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(m.runtimeDir("1.0.0", "darwin-arm64"), "mypack-runtime")
	_ = os.Chmod(exe, 0o755)
	if err := os.WriteFile(exe, fat, 0o755); err != nil {
		t.Fatal(err)
	}
	mk, _ := readMarker(m.runtimeDir("1.0.0", "darwin-arm64"))
	mk.BinarySHA256, mk.BinarySize, _ = hashFile(exe)
	writeTestMarker(t, m.runtimeDir("1.0.0", "darwin-arm64"), mk)
	if _, err := m.checkLocal("1.0.0", "darwin-arm64", nil); err == nil {
		t.Fatal("a universal binary under darwin-arm64 should not verify")
	}

	if err := m.SeedFromDir(runtimes); err != nil {
		t.Fatal(err)
	}
	got, err := m.Ensure(context.Background(), "1.0.0", "darwin-arm64", nil)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(got); !bytes.Equal(b, arm) {
		t.Fatalf("reseeded %d bytes, want the arm64 slice", len(b))
	}
}

func writeTestMarker(t *testing.T, dir string, mk marker) {
	t.Helper()
	b, _ := json.Marshal(mk)
	if err := os.WriteFile(filepath.Join(dir, markerName), b, 0o644); err != nil {
		t.Fatal(err)
	}
}
