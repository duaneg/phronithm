package lsfrt

import (
	"os"
	"path/filepath"
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

const minimalConfig = `{"stateDir":"/tmp/x","defaultRole":"worker",
	"roles":{"worker":{"tools":"default","permissionMode":"dontAsk"},
	         "critic":{"tools":"Read,Grep","allow":["Read"],"deny":["Bash(git commit*)"],"permissionMode":"dontAsk"}},
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
	c, err := LoadConfig(writeConfig(t, `{"stateDir":"/x","defaultRole":"w",
		"roles":{"w":{"tools":"default","permissionMode":"dontAsk"},"c":{"tools":"Read","permissionMode":"dontAsk"}},
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
		{`{"stateDir":"/x","defaultRole":"w","roles":{"w":{"tools":"t","permissionMode":"dontAsk"}},"bogus":1}`, "unknown field"},
		{`{"stateDir":"/x","defaultRole":"w","roles":{}}`, "at least one role"},
		{`{"stateDir":"/x","defaultRole":"w","roles":{"w":{"tools":"t","permissionMode":"dontAsk"}},"roleRules":[{"label":"x","role":"nope"}]}`, `role "nope" names no role`},
		{`{"stateDir":"/x","defaultRole":"w","roles":{"w":{"tools":"t","permissionMode":"dontAsk"}},"roleRules":[{"role":"w"}]}`, "set agentType, label or both"},
		{`{"stateDir":"/x","defaultRole":"w","roles":{"w":{"tools":"t","permissionMode":"dontAsk"}},"roleRules":[{"label":"(","role":"w"}]}`, "roleRules[0].label"},
		{`{"stateDir":"/x","defaultRole":"w","roles":{"w":{"tools":"t","permissionMode":""}}}`, "permissionMode is required"},
		{`{"stateDir":"/x","defaultRole":"q","roles":{"w":{"tools":"t","permissionMode":"dontAsk"}}}`, `defaultRole "q"`},
		{`{"stateDir":"/x","defaultRole":"w","roles":{"w":{"tools":"t","permissionMode":"dontAsk"}},"guardTimeout":90}`, "duration must be a string"},
		{`{"defaultRole":"w","roles":{"w":{"tools":"t","permissionMode":"dontAsk"}}}`, "stateDir is required"},
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
