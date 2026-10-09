package lsfrt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// wantArgv is the podman argv the design gives a call (lsf-runtime.md,
// Permissions → A call's launch), built independently of containerArgv.
func wantArgv(rt *Runtime, cfg Config, name, dir string, git bool, o Opts, tools string) []string {
	_, port, _ := strings.Cut(strings.TrimPrefix(rt.Proxy().URL, "http://"), ":")
	a := []string{"run", "--rm", "-i", "--init", "--pull=never", "--log-driver=none", "--name", name, "--label", "lsf.state=" + rt.stateHash,
		"--userns=keep-id", "--cap-drop=all", "--security-opt", "no-new-privileges", "--read-only", "--tmpfs", "/tmp:rw,exec",
		"--network", "pasta:--splice-only,-T," + port, "-v", dir + ":" + dir + ":rw"}
	if git {
		for _, g := range []string{"config", "hooks", "info"} {
			p := dir + "/.git/" + g
			a = append(a, "-v", p+":"+p+":ro")
		}
	}
	a = append(a, "-v", cfg.ClaudeBin+":/opt/claude/claude:ro", "-v", cfg.PluginDir+":/opt/plugin:ro", "-w", dir,
		"-e", "HOME=/tmp/home", "-e", "SHELL=/bin/bash", "-e", "DISABLE_AUTOUPDATER=1", "-e", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"-e", "ANTHROPIC_BASE_URL="+rt.Proxy().URL, "-e", "ANTHROPIC_AUTH_TOKEN", "-e", "NO_PROXY=127.0.0.1", "-e", "no_proxy=127.0.0.1")
	for _, e := range cfg.Env {
		a = append(a, "-e", e)
	}
	sid := strings.TrimPrefix(name, "lsf-"+rt.stateHash[:8]+"-")
	a = append(a, cfg.Image, "/opt/claude/claude", "-p", "--output-format", "stream-json", "--verbose", "--session-id", sid)
	if o.Model != "" {
		a = append(a, "--model", o.Model)
	}
	if o.Effort != "" {
		a = append(a, "--effort", o.Effort)
	}
	return append(a, "--strict-mcp-config", "--plugin-dir", "/opt/plugin", "--tools", tools,
		"--dangerously-skip-permissions", "--json-schema", string(o.Schema))
}

func flagValue(argv []string, flag string) string {
	if i := slices.Index(argv, flag); i >= 0 && i+1 < len(argv) {
		return argv[i+1]
	}
	return ""
}

// Each call's podman argv: a writer's kept clone, a reviewer's discarded
// clone, the merge call's merge clone, and the env passthrough. The
// placeholder bearer is in none of the argv, the telemetry or the log.
func TestCallContainerArgv(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.Env = []string{"LSF_TEST_PASS"}
	t.Setenv("LSF_TEST_PASS", "v")
	t.Setenv("STUB_SCRIPT", "ok")
	writer := Opts{Label: "implement:u1", Model: "opus", Effort: "low", Schema: json.RawMessage(verdictSchema)}
	critic := Opts{Label: "critique-code:r1:u1", Schema: json.RawMessage(verdictSchema)}
	var placeholder string
	var want [][]string
	runs := []int{1, 2, 4} // the critic's skill is missing under the stub: it runs twice
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		placeholder = rt.Proxy().Placeholder
		merge, err := rt.MergeClone(ctx, "m")
		if err != nil {
			t.Fatal(err)
		}
		merger := Opts{Label: "merge-conflict:w1:u1", Dir: merge, Schema: json.RawMessage(verdictSchema)}
		for i, o := range []Opts{writer, critic, merger} {
			if _, err := rt.Call(ctx, "x", o); err != nil {
				t.Fatal(err)
			}
			name := flagValue(podmanRun(t, stub, runs[i]), "--name")
			if !strings.HasPrefix(name, "lsf-"+rt.stateHash[:8]+"-") {
				t.Errorf("name %q", name)
			}
			_, role := rt.cfg.roleFor(o)
			var dir string
			switch i {
			case 0:
				dir = filepath.Join(cfg.StateDir, "clone", journalKey(o.Label))
			case 1:
				dir = filepath.Join(cfg.StateDir, "scratch", journalKey(o.Label))
			case 2:
				dir, _ = filepath.EvalSymlinks(merge)
			}
			want = append(want, wantArgv(rt, rt.cfg, name, dir, true, o, role.Tools))
		}
		return nil, nil
	})
	for i, w := range want {
		if got := podmanRun(t, stub, runs[i]); !slices.Equal(got, w) {
			t.Errorf("run %d argv\n got %q\nwant %q", runs[i], got, w)
		}
	}
	for _, f := range []string{filepath.Join(stub, "podman", "run.1"), filepath.Join(stub, "podman", "run.2"), filepath.Join(stub, "podman", "run.3"), filepath.Join(stub, "podman", "run.4"),
		filepath.Join(cfg.StateDir, "telemetry.jsonl"), filepath.Join(cfg.StateDir, "log.txt")} {
		if b, _ := os.ReadFile(f); placeholder == "" || strings.Contains(string(b), placeholder) {
			t.Errorf("%s carries the placeholder bearer", f)
		}
	}
	// The stub claude ran in the clone with the plugin dir substituted.
	if got := flagValues(stubArgv(t, stub, 1), "--plugin-dir"); !slices.Equal(got, []string{cfg.PluginDir}) {
		t.Errorf("--plugin-dir %q", got)
	}
}

// Without a repo (the runtime's own tests), a call runs in an empty scratch
// directory, not the launcher's, with no .git binds.
func TestRepoLessContainer(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.Repo = ""
	t.Setenv("STUB_SCRIPT", "ok")
	var want []string
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		if _, err := rt.Call(ctx, "x", vOpts); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(cfg.StateDir, "scratch", journalKey(vOpts.Label))
		want = wantArgv(rt, rt.cfg, flagValue(podmanRun(t, stub, 1), "--name"), dir, false, vOpts, "default")
		return nil, nil
	})
	if got := podmanRun(t, stub, 1); !slices.Equal(got, want) {
		t.Errorf("argv\n got %q\nwant %q", got, want)
	}
	cwd, _ := os.ReadFile(filepath.Join(stub, "cwd.1"))
	resolved, _ := filepath.EvalSymlinks(filepath.Join(cfg.StateDir, "scratch", journalKey(vOpts.Label)))
	if strings.TrimSpace(string(cwd)) != resolved {
		t.Errorf("cwd %q, want %q", cwd, resolved)
	}
}

// A clone's missing .git/hooks and .git/info are made before the launch; a
// clone whose .git/config is gone stops the run with runtime-error and no
// spawn.
func TestGitBindsPrepared(t *testing.T) {
	cfg, stub := testConfig(t)
	t.Setenv("STUB_SCRIPT", "ok,ok")
	s := launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		merge, err := rt.MergeClone(ctx, "m")
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range []string{"hooks", "info"} {
			os.RemoveAll(filepath.Join(merge, ".git", d))
		}
		if _, err := rt.Call(ctx, "x", Opts{Label: "merge-conflict:a", Dir: merge, Schema: json.RawMessage(verdictSchema)}); err != nil {
			t.Fatal(err)
		}
		for _, d := range []string{"hooks", "info"} {
			if fi, err := os.Stat(filepath.Join(merge, ".git", d)); err != nil || !fi.IsDir() {
				t.Errorf(".git/%s not made: %v", d, err)
			}
		}
		os.Remove(filepath.Join(merge, ".git", "config"))
		_, err = rt.Call(ctx, "x", Opts{Label: "merge-conflict:b", Dir: merge, Schema: json.RawMessage(verdictSchema)})
		return nil, err
	})
	if s.StopReason != StopRuntimeError || !strings.Contains(s.Detail, "config is not a regular file") {
		t.Errorf("summary %+v", s)
	}
	if n := stubCount(t, stub); n != 1 {
		t.Errorf("%d spawns, want 1", n)
	}
}

// A dry run logs the podman argv, with placeholders for the clone and the
// session, and spawns nothing.
func TestDryRunArgv(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.DryRun = true
	s := launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		_, err := rt.Call(ctx, "x", vOpts)
		return nil, err
	})
	if s.StopReason != StopDryRun || stubCount(t, stub) != 0 {
		t.Fatalf("summary %+v", s)
	}
	for _, e := range telemetry(t, cfg) {
		if e["kind"] != "dry-run" {
			continue
		}
		var argv []string
		for _, a := range e["argv"].([]any) {
			argv = append(argv, a.(string))
		}
		if argv[0] != "run" || !slices.Contains(argv, "<dir>:<dir>:rw") || !slices.Contains(argv, "<dir>/.git/config:<dir>/.git/config:ro") ||
			flagValue(argv, "--session-id") != "<session-id>" || flagValue(argv, "-w") != "<dir>" {
			t.Errorf("dry-run argv %q", argv)
		}
		return
	}
	t.Error("no dry-run event")
}
