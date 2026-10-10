package lsfrt

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// ctrFields are a config's container settings, ending in a comma.
const ctrFields = `"pluginDir":"/p","image":"img@sha256:0000000000000000000000000000000000000000000000000000000000000000",` +
	`"claudeVersion":"1.0.0","proxy":{"listen":"127.0.0.1:0","credentialsFile":"/c"},`

const minimalConfig = `{"stateDir":"/tmp/x",` + ctrFields + `"defaultRole":"worker",
	"roles":{"worker":{"tools":"default","clone":"kept"},
	         "critic":{"tools":"Read,Grep","clone":"kept"}},
	"roleRules":[{"agentType":"phronithm:critic","role":"critic"},{"label":"^critique-","role":"critic"}],
	"skills":[{"label":"^critique-","skill":"phronithm:critique"}],
	"guardTimeout":"90m"}`

func TestLoadConfig(t *testing.T) {
	c, err := LoadConfig(writeConfig(t, minimalConfig))
	if err != nil {
		t.Fatal(err)
	}
	if c.GuardTimeout.D() != 90*time.Minute || c.Concurrency != 4 || c.KillGrace.D() != 10*time.Second || len(c.Backoff) != 3 {
		t.Errorf("defaults not applied: %+v", c)
	}
	for _, tc := range []struct {
		o    Opts
		role string
	}{
		{Opts{Label: "implement:u1"}, "worker"},
		{Opts{Label: "plan:u1", AgentType: "phronithm:critic"}, "critic"},
		{Opts{Label: "critique-code:r1:u1"}, "critic"},
	} {
		if name, _ := c.roleFor(tc.o); name != tc.role {
			t.Errorf("%+v: role %s, want %s", tc.o, name, tc.role)
		}
	}
	if s := c.skillFor("critique-plan:r1:u1"); s != "phronithm:critique" {
		t.Errorf("skillFor: %q", s)
	}
	if s := c.skillFor("implement:u1"); s != "" {
		t.Errorf("skillFor: %q", s)
	}
}

func TestRoleRuleIsAnd(t *testing.T) {
	c, err := LoadConfig(writeConfig(t, `{"stateDir":"/x",`+ctrFields+`"defaultRole":"w",
		"roles":{"w":{"tools":"default","clone":"kept"},"c":{"tools":"Read","clone":"kept"}},
		"roleRules":[{"agentType":"phronithm:critic","label":"^critique-","role":"c"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if name, _ := c.roleFor(Opts{Label: "critique-code:r1", AgentType: "other"}); name != "w" {
		t.Errorf("label alone matched an AND rule: %s", name)
	}
	if name, _ := c.roleFor(Opts{Label: "critique-code:r1", AgentType: "phronithm:critic"}); name != "c" {
		t.Errorf("both matchers set but no match: %s", name)
	}
}

func TestLoadConfigRejects(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`{"stateDir":"/x",` + ctrFields + `"defaultRole":"w","roles":{"w":{"tools":"t","clone":"kept"}},"bogus":1}`, "unknown field"},
		{`{"stateDir":"/x",` + ctrFields + `"defaultRole":"w","roles":{}}`, "at least one role"},
		{`{"stateDir":"/x",` + ctrFields + `"defaultRole":"w","roles":{"w":{"tools":"t","clone":"kept"}},"roleRules":[{"label":"x","role":"nope"}]}`, `role "nope" names no role`},
		{`{"stateDir":"/x",` + ctrFields + `"defaultRole":"w","roles":{"w":{"tools":"t","clone":"kept"}},"roleRules":[{"role":"w"}]}`, "set agentType, label or both"},
		{`{"stateDir":"/x",` + ctrFields + `"defaultRole":"w","roles":{"w":{"tools":"t","clone":"kept"}},"roleRules":[{"label":"(","role":"w"}]}`, "roleRules[0].label"},
		{`{"stateDir":"/x",` + ctrFields + `"defaultRole":"q","roles":{"w":{"tools":"t","clone":"kept"}}}`, `defaultRole "q"`},
		{`{"stateDir":"/x",` + ctrFields + `"defaultRole":"w","roles":{"w":{"tools":"t","clone":"kept"}},"guardTimeout":90}`, "duration must be a string"},
		{`{` + ctrFields + `"defaultRole":"w","roles":{"w":{"tools":"t","clone":"kept"}}}`, "stateDir is required"},
		{`{"stateDir":"/x",` + ctrFields + `"defaultRole":"w","roles":{"w":{"tools":"t"}}}`, `roles.w: clone must be "kept" or "discarded", not ""`},
		{`{"stateDir":"/x",` + ctrFields + `"defaultRole":"w","roles":{"w":{"tools":"t","clone":"fetched"}}}`, `not "fetched"`},
	} {
		_, err := LoadConfig(writeConfig(t, tc.body))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err %v, want it to contain %q", tc.body, err, tc.want)
		}
	}
}

func TestLaunchClaudeBinEnv(t *testing.T) {
	t.Setenv("LSF_CLAUDE_BIN", "/opt/claude")
	c, err := LoadConfig(writeConfig(t, minimalConfig))
	if err != nil {
		t.Fatal(err)
	}
	if c.ClaudeBin != "/opt/claude" {
		t.Errorf("ClaudeBin %q", c.ClaudeBin)
	}
}

func TestLoadConfigRejectsDurationsAndTrailing(t *testing.T) {
	base := `"stateDir":"/x",` + ctrFields + `"defaultRole":"w","roles":{"w":{"tools":"t","clone":"kept"}}`
	for _, tc := range []struct{ body, want string }{
		{`{` + base + `,"guardTimeout":"-1s"}`, "must not be negative"},
		{`{` + base + `,"backoff":["1s","-1s"]}`, "must not be negative"},
		{`{` + base + `,"backoff":[]}`, "backoff: omit it"},
		{`{` + base + `} {}`, "trailing data"},
		{`{` + base + `,"skills":[{"label":"","skill":"s"}]}`, "skills[0].label is required"},
	} {
		_, err := LoadConfig(writeConfig(t, tc.body))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err %v, want %q", tc.body, err, tc.want)
		}
	}
}

func TestExampleConfigLoads(t *testing.T) {
	t.Setenv("LSF_CLAUDE_BIN", "")
	c, err := LoadConfig(filepath.Join("..", "testdata", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	for label, want := range map[string]string{
		"lens-review:r1:u1": "reviewer", "critique-code:r2:u1": "reviewer", "critique-design:r1:u1": "reviewer",
		"review-gate:w2": "reviewer", "review-gate:w2#2": "reviewer", "obligations": "reviewer",
		"implement:u1": "writer", "plan:critique-x": "writer", "merge-conflict:w1:review-gate": "writer",
		"replan:w1": "writer", "direct-edits": "writer", "review-gate:w2x": "writer",
	} {
		if name, r := c.roleFor(Opts{Label: label}); name != want || r.Clone != map[string]string{"writer": CloneKept, "reviewer": CloneDiscarded}[want] {
			t.Errorf("%s: role %s, clone %q; want %s", label, name, r.Clone, want)
		}
	}
	if name, _ := c.roleFor(Opts{Label: "critique-plan:r1:u1", AgentType: "phronithm:critic"}); name != "reviewer" {
		t.Errorf("critic role %s", name)
	}
	if c.PluginDir != "/path/to/phronithm" {
		t.Errorf("pluginDir %q", c.PluginDir)
	}
	if s := c.skillFor("critique-code:r2:u1"); s != "phronithm:critique" {
		t.Errorf("skill %q", s)
	}
}

// A launch whose repo and state dir are under the temp dir runs: no
// container sees the host's /tmp, so the refusal of the Bash-sandbox design
// is gone.
func TestLaunchUnderTempDir(t *testing.T) {
	cfg, stub := testConfig(t)
	base, err := os.MkdirTemp(os.TempDir(), "lsf-tmp-launch-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	repo := filepath.Join(base, "repo")
	if err := os.Rename(cfg.Repo, repo); err != nil {
		t.Fatal(err)
	}
	cfg.Repo, cfg.StateDir = repo, filepath.Join(base, "state")
	t.Setenv("STUB_SCRIPT", "commit:x")
	res, err := call(t, cfg, vOpts)
	if err != nil || res.Value == nil || stubCount(t, stub) != 1 {
		t.Fatalf("res %+v err %v", res, err)
	}
}

// pluginDir is made absolute.
func TestRelativePluginDir(t *testing.T) {
	rel, err := LoadConfig(writeConfig(t, strings.Replace(`{"stateDir":"/x",`+ctrFields+`"defaultRole":"w","roles":{"w":{"tools":"t","clone":"kept"}}}`, `"/p"`, `"phr"`, 1)))
	if err != nil || !filepath.IsAbs(rel.PluginDir) {
		t.Errorf("relative pluginDir: %q %v", rel.PluginDir, err)
	}
}

// The container's settings are checked: an image by digest only, the
// required fields, env names, a 127.0.0.1 proxy, and roles without the
// permission fields of the pre-container design.
func TestLoadConfigRejectsContainer(t *testing.T) {
	roles := `"defaultRole":"w","roles":{"w":{"tools":"t","clone":"kept"}}`
	ok := `{"stateDir":"/x",` + ctrFields + roles + `}`
	if _, err := LoadConfig(writeConfig(t, ok)); err != nil {
		t.Fatal(err)
	}
	digest := "img@sha256:" + strings.Repeat("0", 64)
	for _, tc := range []struct{ from, to, want string }{
		{`"image":"` + digest + `"`, `"image":"img:latest"`, "name it by digest"},
		{`"image":"` + digest + `"`, `"image":"img@sha256:abc"`, "name it by digest"},
		{`"image":"` + digest + `",`, ``, "image is required"},
		{`"claudeVersion":"1.0.0",`, ``, "claudeVersion is required"},
		{`"pluginDir":"/p",`, ``, "pluginDir is required"},
		{`"proxy":{"listen":"127.0.0.1:0","credentialsFile":"/c"},`, ``, "proxy is required"},
		{`127.0.0.1:0`, `0.0.0.0:0`, `the host must be 127.0.0.1, not "0.0.0.0"`},
		{`127.0.0.1:0`, `localhost:0`, `not "localhost"`},
		{`127.0.0.1:0`, `[::1]:0`, `not "::1"`},
		{`"stateDir":"/x"`, `"stateDir":"/x:y"`, "cannot be a bind mount's source"},
		{`"pluginDir":"/p"`, `"pluginDir":"/p:q"`, "cannot be a bind mount's source"},
		{`"image":"img@`, `"image":"-v/:/h@`, "name it by digest"},
		{`"claudeVersion"`, `"env":["1BAD"],"claudeVersion"`, `"1BAD" is not a variable name`},
		{`"claudeVersion"`, `"env":["GOOD","HTTPS_PROXY"],"claudeVersion"`, "HTTPS_PROXY is set by the runtime"},
		{`"claudeVersion"`, `"env":["LSF_GATE_SEQ"],"claudeVersion"`, "LSF_GATE_SEQ is set by the runtime"},
		{`"claudeVersion"`, `"env":["ANTHROPIC_API_KEY"],"claudeVersion"`, "ANTHROPIC_API_KEY is set by the runtime"},
		{`"claudeVersion"`, `"env":["CLAUDE_CODE_OAUTH_TOKEN"],"claudeVersion"`, "CLAUDE_CODE_OAUTH_TOKEN is set by the runtime"},
		{`"claudeVersion"`, `"env":["PATH"],"claudeVersion"`, "PATH is set by the runtime"},
		{`"clone":"kept"`, `"clone":"kept","permissionMode":"dontAsk"`, `unknown field "permissionMode"`},
		{`"clone":"kept"`, `"clone":"kept","allow":["Read"]`, `unknown field "allow"`},
		{`"clone":"kept"`, `"clone":"kept","deny":["Read"]`, `unknown field "deny"`},
	} {
		body := strings.Replace(ok, tc.from, tc.to, 1)
		if body == ok {
			t.Fatalf("%q not in the config", tc.from)
		}
		_, err := LoadConfig(writeConfig(t, body))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err %v, want %q", body, err, tc.want)
		}
	}
	c, err := LoadConfig(writeConfig(t, strings.Replace(ok, `"claudeVersion"`, `"env":["GOPATH","my_var"],"claudeVersion"`, 1)))
	if err != nil || !slices.Equal(c.Env, []string{"GOPATH", "my_var"}) || c.PodmanBin != "podman" {
		t.Errorf("env %q podmanBin %q: %v", c.Env, c.PodmanBin, err)
	}
}
