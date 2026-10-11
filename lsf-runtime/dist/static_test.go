package dist

import (
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestStaticBuild requires the release binaries to be static: no program
// interpreter and no needed shared library. It cross-builds linux amd64
// and arm64 as CI's build job does, or, with LSF_STATIC_CHECK naming
// binaries (space-separated), checks those instead.
func TestStaticBuild(t *testing.T) {
	bins := strings.Fields(os.Getenv("LSF_STATIC_CHECK"))
	if len(bins) == 0 {
		dir := t.TempDir()
		for _, arch := range []string{"amd64", "arm64"} {
			out := filepath.Join(dir, "lsf-runtime-linux-"+arch)
			cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", out, "../cli")
			cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+arch)
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s: %v\n%s", arch, err, b)
			}
			bins = append(bins, out)
		}
	}
	for _, bin := range bins {
		checkStatic(t, bin)
	}
}

func checkStatic(t *testing.T, bin string) {
	t.Helper()
	f, err := elf.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			t.Errorf("%s: has a program interpreter", bin)
		}
	}
	libs, err := f.ImportedLibraries()
	if err != nil {
		t.Fatalf("%s: %v", bin, err)
	}
	if len(libs) > 0 {
		t.Errorf("%s: needs %v", bin, libs)
	}
}
