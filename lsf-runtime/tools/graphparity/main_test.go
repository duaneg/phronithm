package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestParity runs the parity check when node is installed.
func TestParity(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
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
		t.Fatalf("%d of %d cases differ:\n%s", failed, total, out.String())
	}
	t.Logf("%d cases identical", total)
}
