package lsfrt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// spawnOnce runs one attempt the way Call composes it: role and skill from
// the config, prompt on stdin.
func spawnOnce(t *testing.T, ctx context.Context, rt *Runtime, prompt string, o Opts) *attempt {
	t.Helper()
	c := &callState{o: o}
	c.roleName, c.role = rt.cfg.roleFor(o)
	sid := newUUID()
	cs := rt.callContainer(c, placement{dir: rt.cfg.Repo}, sid)
	return rt.spawn(ctx, spawnSpec{
		prompt:     withSkill(rt.cfg.skillFor(o.Label), prompt),
		argv:       rt.containerArgv(cs),
		sessionID:  sid,
		name:       cs.name,
		transcript: filepath.Join(rt.cfg.StateDir, "calls", sid+".jsonl"),
	})
}

func flagValues(argv []string, flag string) []string {
	i := slices.Index(argv, flag)
	if i < 0 {
		return nil
	}
	var vs []string
	for _, a := range argv[i+1:] {
		if strings.HasPrefix(a, "--") {
			break
		}
		vs = append(vs, a)
	}
	return vs
}

func TestArgvCarriesRoleAndSkill(t *testing.T) {
	cfg, stub := testConfig(t)
	t.Setenv("STUB_SCRIPT", "ok")
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		for _, o := range []Opts{
			{Label: "implement:u1", Model: "opus", Effort: "low", Schema: json.RawMessage(verdictSchema)},
			{Label: "critique-code:r1:u1", Model: "opus", Effort: "medium", Schema: json.RawMessage(verdictSchema)},
		} {
			if a := spawnOnce(t, ctx, rt, "review the diff", o); classify(a, o.Schema, "", false).class != classOK {
				t.Fatalf("%s: %+v", o.Label, a)
			}
		}
		return nil, nil
	})
	for n, want := range map[int]struct {
		role   Role
		prompt string
	}{
		1: {cfg.Roles["worker"], "review the diff"},
		2: {cfg.Roles["critic"], "/phronithm:critique review the diff"},
	} {
		argv := stubArgv(t, stub, n)
		for _, f := range []string{"--strict-mcp-config", "-p", "--dangerously-skip-permissions"} {
			if !slices.Contains(argv, f) {
				t.Errorf("call %d: argv %q lacks %s", n, argv, f)
			}
		}
		for flag, vals := range map[string][]string{
			"--tools":       {want.role.Tools},
			"--plugin-dir":  {cfg.PluginDir},
			"--json-schema": {verdictSchema},
		} {
			if got := flagValues(argv, flag); !slices.Equal(got, vals) {
				t.Errorf("call %d: %s = %q, want %q", n, flag, got, vals)
			}
		}
		stdin, _ := os.ReadFile(filepath.Join(stub, "stdin."+string(rune('0'+n))))
		if string(stdin) != want.prompt {
			t.Errorf("call %d: stdin %q, want %q", n, stdin, want.prompt)
		}
	}
}

func TestGuardTimeoutKillsGroup(t *testing.T) {
	for _, mode := range []string{"hang", "hang-ignore-term", "orphan-ignore-term"} {
		t.Run(mode, func(t *testing.T) {
			cfg, stub := testConfig(t)
			cfg.GuardTimeout = Duration(time.Second)
			t.Setenv("STUB_SCRIPT", mode)
			launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
				start := time.Now()
				a := spawnOnce(t, ctx, rt, "x", Opts{Label: "implement:u1", Schema: json.RawMessage(verdictSchema)})
				if d := time.Since(start); d > cfg.GuardTimeout.D()+cfg.KillGrace.D()+time.Second {
					t.Errorf("took %v", d)
				}
				if v := classify(a, json.RawMessage(verdictSchema), "", false); v.class != classTimeout {
					t.Errorf("class %s", v.class)
				}
				return nil, nil
			})
			for _, pid := range stubPids(t, stub, 1) {
				waitFor(t, 2*time.Second, "pid to die", func() bool { return !alive(pid) })
			}
		})
	}
}

func TestSpawnErrorAndChildrenRecord(t *testing.T) {
	cfg, _ := testConfig(t)
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		rt.cfg.PodmanBin = filepath.Join(t.TempDir(), "no-such-podman")
		a := spawnOnce(t, ctx, rt, "x", Opts{Label: "implement:u1", Schema: json.RawMessage(verdictSchema)})
		if v := classify(a, json.RawMessage(verdictSchema), "", false); v.class != classSpawnError {
			t.Errorf("class %s", v.class)
		}
		return nil, nil
	})
	// Nothing is recorded there: containers carry a label.
	if _, err := os.Stat(filepath.Join(cfg.StateDir, "children.json")); !os.IsNotExist(err) {
		t.Errorf("children.json written: %v", err)
	}
}

func TestLaunchExclusion(t *testing.T) {
	cfg, _ := testConfig(t)
	s := launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		_, err := Run(ctx, cfg, func(context.Context, *Runtime) (json.RawMessage, error) {
			t.Error("second launch ran its body")
			return nil, nil
		})
		if err == nil || !strings.Contains(err.Error(), "in use by another launch") {
			t.Errorf("second launch: %v", err)
		}
		return json.RawMessage(`{"status":"complete"}`), nil
	})
	if s.StopReason != StopReturned || string(s.Outcome) != `{"status":"complete"}` {
		t.Errorf("summary %+v", s)
	}
	if got := readSummary(t, cfg); got.StopReason != StopReturned {
		t.Errorf("summary.json %+v", got)
	}
	// The lock is released: a later launch runs.
	launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) { return nil, nil })
}
