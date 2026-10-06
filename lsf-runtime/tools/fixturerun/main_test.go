package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

// The config keeps the example's roles and skill rules and tightens the caps.
func TestConfig(t *testing.T) {
	c, err := config(filepath.Join("..", "..", "testdata", "config.example.json"), "/w", "/bin/claude", "abc12345", false)
	if err != nil {
		t.Fatal(err)
	}
	var args map[string]string
	json.Unmarshal(c.Args, &args)
	if c.StateDir != "/w/state" || c.Repo != "/w/repo" || c.ClaudeBin != "/bin/claude" || args["planSha"] != "abc12345" || args["integrationBranch"] != "main" {
		t.Errorf("%+v %v", c, args)
	}
	if c.Caps.MaxCalls != 40 || c.Caps.MaxCostUSD != 25 || len(c.Roles) == 0 || len(c.Skills) == 0 || c.DefaultRole == "" {
		t.Errorf("%+v", c)
	}
}

// A dry run builds the fixture and stops at the first call without spawning.
func TestDryRun(t *testing.T) {
	for _, bin := range []string{"git", "node", "sh"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skip(bin + " not installed")
		}
	}
	err := run(filepath.Join("..", "..", ".."), "/nonexistent/claude", t.TempDir(), filepath.Join("..", "..", "testdata", "config.example.json"), true)
	if err != nil {
		t.Fatal(err)
	}
}
