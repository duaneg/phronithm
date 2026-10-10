package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/duaneg/phronithm/lsf-runtime/internal/testcfg"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

// compose composes the fixture's launch config into a temp file.
func (f *fixture) compose(pluginDir string) (lsfrt.Config, string, error) {
	f.t.Helper()
	out := filepath.Join(f.t.TempDir(), "launch.1.json")
	cfg, err := compose(launchIn{hostFile: f.host, pluginDir: pluginDir, repo: f.repo, stateDir: filepath.Join(f.t.TempDir(), "state"), args: f.args(), out: out})
	return cfg, out, err
}

func TestComposeLayering(t *testing.T) {
	f := newFixture(t)
	f.writeHost(M{"concurrency": 2, "roles": M{"only": M{"tools": "Read", "clone": "discarded"}}, "defaultRole": "only", "roleRules": []any{}})
	cfg, out, err := f.compose("")
	if err != nil {
		t.Fatal(err)
	}
	eq(t, cfg.Concurrency, 2, "a host key")
	eq(t, len(cfg.Roles), 1, "a host key replaces the default's whole")
	eq(t, cfg.DefaultRole, "only")
	var def lsfrt.Config
	json.Unmarshal(defaults, &def)
	eq(t, len(cfg.Skills), len(def.Skills), "a default key the host leaves alone")
	eq(t, cfg.Repo, f.repo)
	eq(t, cfg.ClaudeVersion, testcfg.ClaudeVersion, "filled from the binary")
	eq(t, cfg.ClaudeBin, testcfg.Claude())
	eq(t, cfg.DryRun, false)
	if fi, err := os.Stat(out); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("composed file %v %v", fi, err)
	}
	var stored map[string]json.RawMessage
	json.Unmarshal([]byte(readFile(t, out)), &stored)
	eq(t, string(stored["claudeBin"]), `"`+testcfg.Claude()+`"`, "the resolved path is stored")
}

func TestComposeRefusesCLIKeys(t *testing.T) {
	for _, k := range cliKeys {
		f := newFixture(t)
		f.writeHost(M{k: "x"})
		_, _, err := f.compose("")
		if err == nil || !strings.Contains(err.Error(), "set by the CLI per launch, remove: "+k) {
			t.Errorf("%s: %v", k, err)
		}
	}
}

func TestComposeOldConfigHint(t *testing.T) {
	for _, c := range []M{
		{"allow": []string{"Bash"}},
		{"deny": []string{}},
		{"roles": M{"writer": M{"tools": "Read", "clone": "kept", "permissionMode": "acceptEdits"}}},
	} {
		f := newFixture(t)
		f.writeHost(c)
		_, out, err := f.compose("")
		if err == nil || !strings.Contains(err.Error(), "unknown field") || !strings.Contains(err.Error(), "predates the container boundary") {
			t.Errorf("%v: %v", c, err)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Errorf("a refused config's file is left: %v", err)
		}
	}
}

func TestComposeMissingHost(t *testing.T) {
	f := newFixture(t)
	os.Remove(f.host)
	_, _, err := f.compose("")
	if err == nil || !strings.Contains(err.Error(), f.host+" does not exist") || !strings.Contains(err.Error(), configDoc) {
		t.Errorf("%v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", "/nonexistent")
	eq(t, hostConfigPath(), "/nonexistent/lsf-runtime/config.json")
}

func TestComposePluginDir(t *testing.T) {
	f := newFixture(t)
	os.MkdirAll(filepath.Join(filepath.Dir(f.host), "plugin"), 0o755)
	f.writeHost(M{"pluginDir": "plugin"})
	cfg, _, err := f.compose("")
	if err != nil {
		t.Fatal(err)
	}
	eq(t, cfg.PluginDir, filepath.Join(filepath.Dir(f.host), "plugin"), "relative to the host file")
	flagDir := t.TempDir()
	cfg, _, err = f.compose(flagDir)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, cfg.PluginDir, flagDir, "-plugin-dir beats the host file")
}

// fakeVersion writes an executable answering --version with v.
func fakeVersion(t *testing.T, path, v string) {
	t.Helper()
	if err := os.WriteFile(path+".tmp", []byte("#!/bin/sh\necho '"+v+" (Claude Code)'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
}

func TestComposeClaudeVersion(t *testing.T) {
	f := newFixture(t)
	f.writeHost(M{"claudeVersion": testcfg.ClaudeVersion})
	if _, _, err := f.compose(""); err != nil {
		t.Fatalf("a matching claudeVersion: %v", err)
	}
	f.writeHost(M{"claudeVersion": "9.9.9"})
	_, _, err := f.compose("")
	if err == nil || !strings.Contains(err.Error(), "reports version "+testcfg.ClaudeVersion) || !strings.Contains(err.Error(), "claudeVersion is 9.9.9") {
		t.Errorf("mismatch: %v", err)
	}
}

func TestComposeClaudeBinPrecedence(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	fromEnv, fromHost := filepath.Join(dir, "env-claude"), filepath.Join(dir, "host-claude")
	fakeVersion(t, fromEnv, "1.0.0")
	fakeVersion(t, fromHost, "2.0.0")
	f.writeHost(M{"claudeBin": fromHost})
	cfg, _, err := f.compose("")
	if err != nil {
		t.Fatal(err)
	}
	eq(t, cfg.ClaudeBin, fromHost)
	eq(t, cfg.ClaudeVersion, "2.0.0")
	rel, _ := filepath.Rel(filepath.Dir(f.host), fromHost)
	f.writeHost(M{"claudeBin": rel})
	if cfg, _, err = f.compose(""); err != nil {
		t.Fatal(err)
	}
	eq(t, cfg.ClaudeBin, fromHost, "a relative claudeBin with a / is the host file's")
	f.writeHost(M{"claudeBin": fromHost})
	t.Setenv("LSF_CLAUDE_BIN", fromEnv)
	cfg, _, err = f.compose("")
	if err != nil {
		t.Fatal(err)
	}
	eq(t, cfg.ClaudeBin, fromEnv, "LSF_CLAUDE_BIN beats the host file")
	eq(t, cfg.ClaudeVersion, "1.0.0")

	t.Setenv("LSF_CLAUDE_BIN", "")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	os.Symlink(fromEnv, filepath.Join(dir, "claude"))
	f.writeHost(M{"claudeBin": nil})
	cfg, _, err = f.compose("")
	if err != nil {
		t.Fatal(err)
	}
	eq(t, cfg.ClaudeBin, fromEnv, "claude on PATH, its symlink resolved")

	t.Setenv("LSF_CLAUDE_BIN", filepath.Join(dir, "missing"))
	_, _, err = f.compose("")
	if err == nil || !strings.Contains(err.Error(), "from LSF_CLAUDE_BIN") {
		t.Errorf("a resolve error names its source: %v", err)
	}
}

// A host key the CLI reads as a string is refused when it is not one,
// rather than ignored.
func TestComposeWrongTypes(t *testing.T) {
	for _, k := range []string{"claudeVersion", "claudeBin", "pluginDir"} {
		f := newFixture(t)
		f.writeHost(M{k: 5})
		_, _, err := f.compose("")
		if err == nil || !strings.Contains(err.Error(), k+" must be a string") {
			t.Errorf("%s: %v", k, err)
		}
	}
	f := newFixture(t)
	os.WriteFile(f.host, []byte("null"), 0o600)
	_, _, err := f.compose("")
	if err == nil || strings.Contains(err.Error(), "<nil>") || !strings.Contains(err.Error(), "not a JSON object") {
		t.Errorf("null host: %v", err)
	}
	f.writeHost(M{"image": "not-a-digest"})
	_, _, err = f.compose("")
	if err == nil || !strings.Contains(err.Error(), "config composed from "+f.host) || strings.Contains(err.Error(), "launch.1.json") {
		t.Errorf("a load error names the host file, not the removed composed file: %v", err)
	}
}

func TestComposeDryRun(t *testing.T) {
	f := newFixture(t)
	out := filepath.Join(t.TempDir(), "launch.json")
	cfg, err := compose(launchIn{hostFile: f.host, repo: f.repo, stateDir: t.TempDir(), args: f.args(), dryRun: true, out: out})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, cfg.DryRun, true)
}

func TestCheckGit(t *testing.T) {
	if err := checkGit(); err != nil {
		t.Fatalf("this host's git: %v", err)
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for v, want := range map[string]string{
		"git version 2.37.1":                "2.37.1 is older than 2.38 (git merge-tree --write-tree)",
		"git version 1.99.0":                "1.99.0 is older than 2.38",
		"git version 2.38.0":                "",
		"git version 3.0.0.windows.1":       "",
		"git version 2.54.0 (Apple Git-99)": "",
		"something else":                    `cannot parse "something else"`,
	} {
		os.WriteFile(filepath.Join(dir, "git"), []byte("#!/bin/sh\necho '"+v+"'\n"), 0o755)
		err := checkGit()
		switch {
		case want == "" && err != nil:
			t.Errorf("%s: %v", v, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("%s: %v, want %q", v, err, want)
		}
	}
}

func TestNameRule(t *testing.T) {
	for _, n := range []string{"f", "lsf-improvements", "a.b_c-1"} {
		if err := checkName(n); err != nil {
			t.Errorf("%s: %v", n, err)
		}
	}
	for _, n := range []string{"", "-x", ".x", "a/b", "a b", "a..b", "a.lock", "a@{b"} {
		if err := checkName(n); err == nil {
			t.Errorf("%q accepted", n)
		}
	}
	if _, err := argsName(json.RawMessage(`{"name":"f"}`), "g"); err == nil {
		t.Error("-name differing from args.name accepted")
	}
	if _, err := argsName(json.RawMessage(`{"planSha":"x"}`), ""); err == nil {
		t.Error("args without a name accepted")
	}
}

// The plugin defaults are the example config's roles and rules.
func TestDefaultsMatchExample(t *testing.T) {
	var ex, def map[string]any
	json.Unmarshal([]byte(readFile(t, filepath.Join(testcfg.Testdata(), "config.example.json"))), &ex)
	if err := json.Unmarshal(defaults, &def); err != nil {
		t.Fatal(err)
	}
	keys := []string{"defaultRole", "roles", "roleRules", "skills"}
	eq(t, len(def), len(keys), "defaults hold only the label-bound keys")
	for _, k := range keys {
		if !reflect.DeepEqual(ex[k], def[k]) {
			t.Errorf("%s: defaults %v, example %v", k, def[k], ex[k])
		}
	}
}

// smokeArgv is the stub podman's first recorded run: the smoke container.
func smokeArgv(t *testing.T, podmanState string) []string {
	t.Helper()
	return strings.Split(strings.TrimRight(readFile(t, filepath.Join(podmanState, "run.1")), "\x00"), "\x00")
}

// A claude repointed after compose resolved it is not used: the driver
// mounts the resolved file. One overwritten in place is caught by the
// launch check's version comparison.
func TestClaudeBinReplacedMidLaunch(t *testing.T) {
	t.Run("repointed", func(t *testing.T) {
		f := newFixture(t)
		f.git("update-ref", "refs/lsf/f/pause", "HEAD")
		dir := t.TempDir()
		v1, v2, link := filepath.Join(dir, "v1"), filepath.Join(dir, "v2"), filepath.Join(dir, "claude")
		fakeVersion(t, v1, testcfg.ClaudeVersion)
		fakeVersion(t, v2, "2.0.0")
		os.Symlink(v1, link)
		f.writeHost(M{"claudeBin": link})
		cfg, file, err := f.compose("")
		if err != nil {
			t.Fatal(err)
		}
		os.Remove(link)
		os.Symlink(v2, link)
		sum, err := drive(cfg.StateDir, file)
		if err != nil {
			t.Fatal(err)
		}
		eq(t, string(sum.StopReason), "returned", sum.Error)
		argv := strings.Join(smokeArgv(t, f.podman), " ")
		contains(t, argv, "-v "+v1+":/opt/claude/claude:ro")
		if strings.Contains(argv, v2) {
			t.Errorf("the repointed binary is mounted: %s", argv)
		}
	})
	t.Run("overwritten", func(t *testing.T) {
		f := newFixture(t)
		v1 := filepath.Join(t.TempDir(), "v1")
		fakeVersion(t, v1, testcfg.ClaudeVersion)
		f.writeHost(M{"claudeBin": v1})
		cfg, file, err := f.compose("")
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(v1, []byte("#!/bin/sh\necho '2.0.0 (Claude Code)'\n"), 0o755)
		_, err = drive(cfg.StateDir, file)
		if err == nil || !strings.Contains(err.Error(), `claude --version reported "2.0.0 (Claude Code)", not claudeVersion `+testcfg.ClaudeVersion) {
			t.Errorf("%v", err)
		}
	})
}
