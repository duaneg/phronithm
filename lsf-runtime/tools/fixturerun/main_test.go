package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/internal/testcfg"
)

// The config keeps the example's roles and skill rules and tightens the caps.
func TestConfig(t *testing.T) {
	c, err := config(options{example: filepath.Join("..", "..", "testdata", "config.example.json"), root: "/p", dir: "/w", claude: "/bin/claude",
		version: pin, podman: "podman", image: "i@sha256:0", credentials: "/c"}, "abc12345")
	if err != nil {
		t.Fatal(err)
	}
	var args map[string]string
	json.Unmarshal(c.Args, &args)
	if c.StateDir != "/w/state" || c.Repo != "/w/repo" || c.ClaudeBin != "/bin/claude" || c.PluginDir != "/p" || args["planSha"] != "abc12345" || args["integrationBranch"] != "main" {
		t.Errorf("%+v %v", c, args)
	}
	if c.ClaudeVersion != pin || c.Image != "i@sha256:0" || c.Proxy == nil || c.Proxy.CredentialsFile != "/c" || c.Proxy.Listen != "127.0.0.1:0" || len(c.Env) != 0 {
		t.Errorf("container settings %+v", c)
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
	tc := testcfg.New(t)
	err := run(options{root: filepath.Join("..", "..", ".."), dir: t.TempDir(), example: filepath.Join("..", "..", "testdata", "config.example.json"),
		claude: testcfg.Claude(), version: testcfg.ClaudeVersion, podman: testcfg.Podman(), image: testcfg.Image, credentials: tc.Credentials, dryRun: true})
	if err != nil {
		t.Fatal(err)
	}
}
