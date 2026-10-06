package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestParity runs the parity check when node is installed.
func TestParity(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		if os.Getenv("PROMPTPARITY_REQUIRE_NODE") != "" {
			t.Fatal("node not installed and PROMPTPARITY_REQUIRE_NODE is set")
		}
		t.Skip("node not installed")
	}
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	failed, total, err := run(repo, false, &out)
	if err != nil {
		t.Fatal(err)
	}
	if failed > 0 {
		t.Fatalf("%d failure(s) over %d scenarios:\n%s", failed, total, out.String())
	}
	t.Logf("%d scenarios identical", total)
}
