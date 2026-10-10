package lsfrt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/internal/testcfg"
)

// wantArgv is the podman argv the design gives a call (lsf-runtime.md,
// Permissions → A call's launch), built independently of containerArgv.
func wantArgv(rt *Runtime, cfg Config, name, dir string, git bool, o Opts, tools string) []string {
	_, port, _ := strings.Cut(strings.TrimPrefix(rt.Proxy().URL, "http://"), ":")
	a := []string{"run", "--rm", "-i", "--init", "--pull=never", "--log-driver=none", "--name", name, "--label", "lsf.state=" + rt.stateHash,
		"--userns=keep-id", "--cap-drop=all", "--security-opt", "no-new-privileges", "--read-only", "--tmpfs", "/tmp:rw,exec",
		"--network", "pasta:--splice-only,-T," + port + egressSplice(rt), "-v", dir + ":" + dir + ":rw"}
	if git {
		a = append(a, "-v", dir+"/.git:"+dir+"/.git:rw")
		for _, g := range []string{"config", "hooks", "info"} {
			p := dir + "/.git/" + g
			a = append(a, "-v", p+":"+p+":ro")
		}
	}
	a = append(a, "-v", cfg.ClaudeBin+":/opt/claude/claude:ro", "-v", cfg.PluginDir+":/opt/plugin:ro", "-w", dir,
		"-e", "HOME=/tmp/home", "-e", "SHELL=/bin/bash", "-e", "DISABLE_AUTOUPDATER=1", "-e", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"-e", "ANTHROPIC_BASE_URL="+rt.Proxy().URL, "-e", "ANTHROPIC_AUTH_TOKEN")
	a = append(a, proxyEnv(rt)...)
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

// egressSplice is the network's egress splice, "" without hosts.
func egressSplice(rt *Runtime) string {
	if p := rt.egressPort(); p != "" {
		return ",-T," + p
	}
	return ""
}

// proxyEnv is the proxy variables a call or setup gets.
func proxyEnv(rt *Runtime) []string {
	var a []string
	if p := rt.egressPort(); p != "" {
		u := "http://127.0.0.1:" + p
		a = append(a, "-e", "HTTPS_PROXY="+u, "-e", "HTTP_PROXY="+u, "-e", "https_proxy="+u, "-e", "http_proxy="+u)
	}
	return append(a, "-e", "NO_PROXY=127.0.0.1,localhost,::1", "-e", "no_proxy=127.0.0.1,localhost,::1")
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
		got := podmanRun(t, stub, runs[i])
		if !slices.Equal(got, w) {
			t.Errorf("run %d argv\n got %q\nwant %q", runs[i], got, w)
		}
		if slices.ContainsFunc(got, func(a string) bool {
			return strings.Contains(strings.ToUpper(a), "HTTPS_PROXY") || strings.Contains(strings.ToUpper(a), "HTTP_PROXY")
		}) ||
			strings.Count(flagValue(got, "--network"), "-T") != 1 {
			t.Errorf("run %d: proxy variables or an egress splice without hosts: %q", runs[i], got)
		}
	}
	runFiles, _ := filepath.Glob(filepath.Join(stub, "podman", "run.*"))
	for _, f := range append(runFiles, filepath.Join(cfg.StateDir, "telemetry.jsonl"), filepath.Join(cfg.StateDir, "log.txt")) {
		if b, _ := os.ReadFile(f); placeholder == "" || strings.Contains(string(b), placeholder) {
			t.Errorf("%s carries the placeholder bearer", f)
		}
	}
	// podman's own environment carries it, for -e ANTHROPIC_AUTH_TOKEN.
	for _, n := range runs {
		if b, _ := os.ReadFile(filepath.Join(stub, "podman", fmt.Sprintf("token.%d", n+1))); string(b) != placeholder { // run 1 is the smoke
			t.Errorf("run %d: podman's ANTHROPIC_AUTH_TOKEN %q, want the placeholder", n, b)
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

// A podman run exiting 125, 126 or 127 before its stream stops the run with
// runtime-error after one attempt, unjournaled; the same exit after the
// stream began is the model's, classified as any other.
func TestLaunchFailureStops(t *testing.T) {
	for _, code := range []string{"125", "126", "127"} {
		t.Run(code, func(t *testing.T) {
			cfg, stub := testConfig(t)
			t.Setenv("STUB_PODMAN_RUN_EXIT", code)
			res, err := call(t, cfg, vOpts)
			s := readSummary(t, cfg)
			if err == nil || s.StopReason != StopRuntimeError || !strings.Contains(s.Detail, "podman run exited "+code+" before the stream") ||
				!strings.Contains(s.Detail, "stub podman run failed") || res.Attempts != 1 {
				t.Errorf("res %+v err %v summary %+v", res, err, s)
			}
			if n := stubCount(t, stub); n != 0 {
				t.Errorf("claude ran %d times", n)
			}
			if b, _ := filepath.Glob(filepath.Join(cfg.StateDir, "journal", "*.json")); len(b) != 0 {
				t.Errorf("journaled %q", b)
			}
		})
	}
	cfg, _ := testConfig(t)
	t.Setenv("STUB_SCRIPT", "exit:125")
	res, err := call(t, cfg, vOpts)
	if err != nil || res.Class != string(classTransient) {
		t.Errorf("an exit 125 after the stream began: res %+v err %v", res, err)
	}
}

// A call killed on the grace-expiry path (a TERM-ignoring container, the
// client SIGKILLed) leaves no container: podman rm -f ends it before Call
// returns, and its processes with it.
func TestGraceExpiryRemovesContainer(t *testing.T) {
	cfg, stub := testConfig(t)
	cfg.GuardTimeout = Duration(time.Second)
	cfg.KillGrace = Duration(300 * time.Millisecond)
	t.Setenv("STUB_PODMAN_DETACH", "1")
	t.Setenv("STUB_SCRIPT", "hang-ignore-term")
	res, err := call(t, cfg, vOpts)
	if err != nil || res.Class != string(classTimeout) {
		t.Fatalf("res %+v err %v", res, err)
	}
	for _, pid := range stubPids(t, stub, 1) {
		if alive(pid) {
			t.Errorf("pid %d outlived Call", pid)
		}
	}
	out, _ := exec.Command(testcfg.Podman(), "ps", "-a", "--filter", "label=lsf.state="+mustHash(t, cfg.StateDir), "--format", "{{.Names}}").Output()
	if len(out) > 0 {
		t.Errorf("containers left: %q", out)
	}
	if countEvents(telemetry(t, cfg), "container-removed") != 1 {
		t.Error("no container-removed event")
	}
}

// A failed removal of a container still there stops the run before
// anything reads the clone: no fetch-back, no journal record, and a
// discarded clone is not removed. One that failed because --rm got there
// first is not a failure.
func TestFailedRemovalLeavesCloneUnread(t *testing.T) {
	for _, label := range []string{vOpts.Label, "critique-code:r1:u1"} {
		t.Run(label, func(t *testing.T) {
			cfg, stub := testConfig(t)
			cfg.GuardTimeout = Duration(time.Second)
			cfg.KillGrace = Duration(200 * time.Millisecond)
			t.Setenv("STUB_PODMAN_DETACH", "1") // the container outlives its client
			t.Setenv("STUB_SCRIPT", "hang-ignore-term")
			t.Setenv("STUB_PODMAN_RM_EXIT", "1")
			t.Cleanup(func() { killStubs(stub) })
			o := vOpts
			o.Label = label
			_, err := call(t, cfg, o)
			s := readSummary(t, cfg)
			if err == nil || s.StopReason != StopRuntimeError || !strings.Contains(s.Detail, "not removed") {
				t.Fatalf("err %v summary %+v", err, s)
			}
			if b, _ := filepath.Glob(filepath.Join(cfg.StateDir, "journal", "*.json")); len(b) != 0 {
				t.Errorf("journaled %q", b)
			}
			if out, _ := exec.Command("git", "-C", cfg.Repo, "branch", "--list", "lsf/*").Output(); len(out) > 0 {
				t.Errorf("fetched back: %s", out)
			}
			sub := "clone"
			if label != vOpts.Label {
				sub = "scratch"
			}
			if _, err := os.Stat(filepath.Join(cfg.StateDir, sub, journalKey(label))); err != nil {
				t.Errorf("clone gone: %v", err)
			}
		})
	}
}

func mustHash(t *testing.T, stateDir string) string {
	t.Helper()
	h, err := stateHashOf(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// A call that redirects its clone's git directory (a commondir naming a
// config it wrote) gets no host git run there: the fetch-back is refused
// and the run stops.
func TestRedirectedGitDirRefused(t *testing.T) {
	cfg, _ := testConfig(t)
	t.Setenv("STUB_SCRIPT", "dirty:.git/commondir")
	_, err := call(t, cfg, vOpts)
	s := readSummary(t, cfg)
	if err == nil || s.StopReason != StopRuntimeError || !strings.Contains(s.Detail, "commondir exists") {
		t.Errorf("err %v summary %+v", err, s)
	}
}

// The launch check refuses, before any body runs and naming the fault: no
// podman, podman not rootless, no pasta, no image, a smoke container that
// fails, and one reporting another claude version.
func TestLaunchChecks(t *testing.T) {
	for _, tc := range []struct {
		name, env, val, want string
		cfg                  func(*Config)
	}{
		{name: "no podman", cfg: func(c *Config) { c.PodmanBin = "/nonexistent/podman" }, want: "podman: "},
		{name: "rootful", env: "STUB_PODMAN_ROOTLESS", val: "false", want: "podman is not rootless"},
		{name: "no pasta", env: "STUB_PODMAN_PASTA", val: "", want: "pasta is missing"},
		{name: "pasta gone", env: "STUB_PODMAN_PASTA", val: "/nonexistent/pasta", want: "/nonexistent/pasta is not an executable"},
		{name: "no image", env: "STUB_PODMAN_NOIMAGE", val: "1", want: "is not present"},
		{name: "smoke fails", env: "STUB_PODMAN_SMOKE_EXIT", val: "1", want: "smoke container failed"},
		{name: "version", cfg: func(c *Config) { c.ClaudeVersion = "2.1.294" }, want: `reported "0.0.0-stub (Claude Code)", not claudeVersion 2.1.294`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := testConfig(t)
			if tc.env != "" {
				t.Setenv(tc.env, tc.val)
			}
			if tc.cfg != nil {
				tc.cfg(&cfg)
			}
			ran := false
			_, err := Run(context.Background(), cfg, func(context.Context, *Runtime) (json.RawMessage, error) { ran = true; return nil, nil })
			if err == nil || ran || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err %v ran %v, want %q", err, ran, tc.want)
			}
		})
	}
	// A passing check removes its smoke repo and records the version.
	cfg, _ := testConfig(t)
	launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) { return nil, nil })
	if _, err := os.Stat(filepath.Join(cfg.StateDir, "smoke")); !os.IsNotExist(err) {
		t.Errorf("smoke repo left: %v", err)
	}
	if countEvents(telemetry(t, cfg), "smoke") != 1 {
		t.Error("no smoke event")
	}
}

// The smoke container is a call's container without the API: --network
// none, the smoke label, the .git binds, and no proxy variables.
func TestSmokeArgv(t *testing.T) {
	cfg, _ := testConfig(t)
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		argv := rt.containerArgv(containerSpec{name: "s", dir: "/d", gitBinds: true, network: "none", labels: []string{labelSmoke}, cmd: []string{"/bin/sh"}})
		if flagValue(argv, "--network") != "none" || !slices.Contains(argv, "lsf.smoke=1") || !slices.Contains(argv, "/d/.git/config:/d/.git/config:ro") {
			t.Errorf("argv %q", argv)
		}
		for _, a := range argv {
			if strings.HasPrefix(a, "ANTHROPIC_") {
				t.Errorf("smoke argv carries %s", a)
			}
		}
		return nil, nil
	})
}

// A dry run reports orphaned containers and leaves them running.
func TestDryRunReportsOrphans(t *testing.T) {
	cfg, stub := testConfig(t)
	pids := orphanLaunch(t, cfg, stub, "hang")
	cfg.DryRun = true
	launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) { return nil, nil })
	if n := countEvents(telemetry(t, cfg), "orphan-found"); n != 3 {
		t.Errorf("%d orphan-found events, want 3", n)
	}
	for _, pid := range pids {
		if !alive(pid) {
			t.Errorf("pid %d killed in a dry run", pid)
		}
	}
	cfg.DryRun = false
	launch(t, cfg, func(context.Context, *Runtime) (json.RawMessage, error) { return nil, nil })
}

// A nested repository an agent commits as a gitlink, with a filter in its
// own config and a .gitmodules ignore=none, is not looked into by the
// runtime's git, so the filter never runs on the host; the fetch-back
// refuses the branch, so it never reaches the main repo or a merge.
func TestNestedRepoFilterNotRun(t *testing.T) {
	cfg, _ := testConfig(t)
	marker := filepath.Join(t.TempDir(), "MARKER")
	t.Setenv("STUB_SCRIPT", "nested:"+marker)
	_, err := call(t, cfg, vOpts)
	s := readSummary(t, cfg)
	if err == nil || s.StopReason != StopRuntimeError || !strings.Contains(s.Detail, "nested repository (gitlink sub") {
		t.Errorf("err %v summary %+v", err, s)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the nested repo's filter ran on the host")
	}
	// The fetch-back refused it: the gitlink never reaches the main repo.
	if out, _ := exec.Command("git", "-C", cfg.Repo, "branch", "--list", "lsf/*").Output(); len(out) > 0 {
		t.Errorf("fetched back: %s", out)
	}
}

func TestRemovalRacingRm(t *testing.T) {
	cfg, _ := testConfig(t)
	t.Setenv("STUB_SCRIPT", "transient,ok")
	t.Setenv("STUB_PODMAN_RM_EXIT", "1") // fails, but the container has exited and gone
	res, err := call(t, cfg, vOpts)
	if err != nil || res.Value == nil {
		t.Errorf("res %+v err %v", res, err)
	}
}

// With hosts pinned a call's network splices the egress proxy and its env
// carries the proxy variables; without, neither (TestCallContainerArgv).
func TestCallContainerEgressArgv(t *testing.T) {
	cfg, stub := testConfig(t)
	t.Setenv("STUB_SCRIPT", "ok")
	var port string
	var want []string
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		rt.PinHosts([]string{"registry.npmjs.org"})
		port = rt.egressPort()
		o := Opts{Label: "implement:u1", Schema: json.RawMessage(verdictSchema)}
		if _, err := rt.Call(ctx, "x", o); err != nil {
			t.Fatal(err)
		}
		_, role := rt.cfg.roleFor(o)
		want = wantArgv(rt, rt.cfg, flagValue(podmanRun(t, stub, 1), "--name"), filepath.Join(cfg.StateDir, "clone", journalKey(o.Label)), true, o, role.Tools)
		return nil, nil
	})
	got := podmanRun(t, stub, 1)
	if !slices.Equal(got, want) {
		t.Errorf("argv\n got %q\nwant %q", got, want)
	}
	if port == "" || !strings.HasSuffix(flagValue(got, "--network"), ",-T,"+port) || !slices.Contains(got, "HTTPS_PROXY=http://127.0.0.1:"+port) || !slices.Contains(got, "http_proxy=http://127.0.0.1:"+port) {
		t.Errorf("no egress splice or proxy variables for port %q: %q", port, got)
	}
}
