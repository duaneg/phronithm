package lsfrt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
)

// Live tests run each call in a real rootless podman container, with the
// stub claude as claudeBin (bash in the image runs it). They run when
// podman is on PATH and LSF_TEST_IMAGE names an image by digest (one built
// from container/Containerfile), and skip otherwise; LSF_LIVE_REQUIRE=1
// makes a skip a failure.

// liveConfig is testConfig on the real podman and LSF_TEST_IMAGE. The stub
// claude's mode reaches it through env; its records stay off, as its state
// dir is not mounted.
func liveConfig(t *testing.T) Config {
	t.Helper()
	img := os.Getenv("LSF_TEST_IMAGE")
	podman, err := exec.LookPath("podman")
	if img == "" || err != nil {
		if os.Getenv("LSF_LIVE_REQUIRE") == "1" {
			t.Fatalf("live test required: podman %v, LSF_TEST_IMAGE %q", err, img)
		}
		t.Skip("live: needs podman and LSF_TEST_IMAGE")
	}
	cfg, _ := testConfig(t)
	cfg.PodmanBin, cfg.Image = podman, img
	cfg.Env = []string{"STUB_SCRIPT"}
	t.Cleanup(func() { liveReap(t, cfg) })
	return cfg
}

// liveContainers are the containers carrying cfg's state label.
func liveContainers(t *testing.T, cfg Config) []string {
	t.Helper()
	h, err := stateHashOf(cfg.StateDir)
	if err != nil {
		return nil
	}
	out, err := exec.Command(cfg.PodmanBin, "ps", "-a", "--filter", "label=lsf.state="+h, "--format", "{{.Names}}").Output()
	if err != nil {
		t.Fatalf("podman ps: %v", err)
	}
	return strings.Fields(string(out))
}

func liveReap(t *testing.T, cfg Config) {
	if names := liveContainers(t, cfg); len(names) > 0 {
		exec.Command(cfg.PodmanBin, append([]string{"rm", "-f", "-t", "0"}, names...)...).Run()
	}
}

// spawnedContainers are the container names of the launch's spawn events.
func spawnedContainers(t *testing.T, cfg Config) []string {
	var names []string
	for _, e := range telemetry(t, cfg) {
		if e["kind"] == "spawn" {
			names = append(names, e["container"].(string))
		}
	}
	return names
}

// L1: a commit call in a kept clone returns its value through -i attach
// with --log-driver=none, its commit is fetched back, every file in the
// clone is the invoking user's, and the stream is not in the journal.
func TestLiveCall(t *testing.T) {
	cfg := liveConfig(t)
	t.Setenv("STUB_SCRIPT", "commit:made.txt")
	res, err := call(t, cfg, vOpts)
	var v struct{ Verdict string }
	if err != nil || json.Unmarshal(res.Value, &v) != nil || v.Verdict != "pass" || res.Branch == "" {
		t.Fatalf("res %+v err %v", res, err)
	}
	if out, err := exec.Command("git", "-C", cfg.Repo, "show", res.Branch+":made.txt").Output(); err != nil {
		t.Errorf("commit not fetched back to %s: %v %s", res.Branch, err, out)
	}
	clone := filepath.Join(cfg.StateDir, "clone", journalKey(vOpts.Label))
	uid := uint32(os.Getuid())
	filepath.Walk(clone, func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.Sys().(*syscall.Stat_t).Uid != uid {
			t.Errorf("%s is owned by uid %d", p, fi.Sys().(*syscall.Stat_t).Uid)
		}
		return nil
	})
	names := spawnedContainers(t, cfg)
	if len(names) != 1 {
		t.Fatalf("spawned %q", names)
	}
	if _, err := exec.LookPath("journalctl"); err != nil {
		t.Log("journalctl absent: the journal check is skipped")
	} else if out, _ := exec.Command("journalctl", "--user", "-q", "-o", "cat", "CONTAINER_NAME="+names[0]).Output(); len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("the call's stream is in the journal: %.200s", out)
	}
	if left := liveContainers(t, cfg); len(left) > 0 {
		t.Errorf("containers left: %q", left)
	}
}

// probe reads the probe mode's probe.txt in dir.
func probe(t *testing.T, dir string) map[string][]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "probe.txt"))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string][]string{}
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		k, v, _ := strings.Cut(l, "=")
		if strings.HasPrefix(l, "env:") {
			k, v = "env", strings.TrimPrefix(l, "env:")
		}
		m[k] = append(m[k], v)
	}
	return m
}

// checkProbe asserts a probe's read-only .git binds and writable index.
func checkProbe(t *testing.T, p map[string][]string, dir string) {
	t.Helper()
	resolved, _ := filepath.EvalSymlinks(dir)
	if got := p["cwd"]; len(got) != 1 || (got[0] != dir && got[0] != resolved) {
		t.Errorf("cwd %q, want %s", got, dir)
	}
	if got := strings.Join(p["readonly"], " "); got != ".git/config .git/hooks/x .git/info/x" {
		t.Errorf("read-only %q; writable %q", got, p["writable"])
	}
	if got := p["index"]; len(got) != 1 || got[0] != "writable" {
		t.Errorf("index %q", got)
	}
}

// L2: inside the container the cwd is the clone at its host path, the uid
// is the invoking user's, .git/config, hooks and info are read-only while
// the index is writable, the environment is exactly the specified one plus
// the image's, and the API is reached through the proxy, which swaps the
// placeholder for the live token.
func TestLiveProbe(t *testing.T) {
	cfg := liveConfig(t)
	var mu sync.Mutex
	var seen []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		w.WriteHeader(299)
	}))
	defer up.Close()
	cfg.Proxy.Upstream = up.URL
	cfg.Env = append(cfg.Env, "LSF_TEST_PASS")
	t.Setenv("LSF_TEST_PASS", "passed")
	t.Setenv("LSF_TEST_CANARY", "host-only")
	t.Setenv("STUB_SCRIPT", "probe")
	var placeholder string
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		placeholder = rt.Proxy().Placeholder
		_, err := rt.Call(ctx, "x", vOpts)
		return nil, err
	})
	p := probe(t, filepath.Join(cfg.StateDir, "clone", journalKey(vOpts.Label)))
	checkProbe(t, p, filepath.Join(cfg.StateDir, "clone", journalKey(vOpts.Label)))
	if got := p["uid"]; len(got) != 1 || got[0] != strconv.Itoa(os.Getuid()) {
		t.Errorf("uid %q", got)
	}
	if got := p["status"]; len(got) != 1 || got[0] != "299" {
		t.Errorf("status through the proxy %q", got)
	}
	mu.Lock()
	if len(seen) != 1 || seen[0] != "Bearer test-live-token" {
		t.Errorf("upstream saw %q", seen)
	}
	mu.Unlock()
	env := map[string]string{}
	for _, kv := range p["env"] {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}
	want := map[string]string{
		"HOME": "/tmp/home", "SHELL": "/bin/bash", "DISABLE_AUTOUPDATER": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"ANTHROPIC_AUTH_TOKEN": placeholder, "NO_PROXY": "127.0.0.1", "no_proxy": "127.0.0.1",
		"STUB_SCRIPT": "probe", "LSF_TEST_PASS": "passed",
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("env %s=%q, want %q", k, env[k], v)
		}
	}
	if !strings.HasPrefix(env["ANTHROPIC_BASE_URL"], "http://127.0.0.1:") {
		t.Errorf("ANTHROPIC_BASE_URL %q", env["ANTHROPIC_BASE_URL"])
	}
	// The image's own (PATH, HOSTNAME, container, TERM) and the shell's
	// (PWD, SHLVL, _, OLDPWD) are allowed; nothing else of the host's.
	allowed := map[string]bool{"PATH": true, "HOSTNAME": true, "container": true, "TERM": true, "PWD": true, "SHLVL": true, "_": true, "OLDPWD": true, "ANTHROPIC_BASE_URL": true}
	for k := range env {
		if _, ok := want[k]; !ok && !allowed[k] {
			t.Errorf("unexpected variable %s in the container", k)
		}
	}
	if _, ok := env["LSF_TEST_CANARY"]; ok {
		t.Error("a host-only variable reached the container")
	}
}

// L7: the conflict call writes the merge clone mid-merge (its index change
// is visible on the host) while its .git/config, hooks and info stay
// read-only.
func TestLiveMergeClone(t *testing.T) {
	cfg := liveConfig(t)
	var merge string
	s := launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		var err error
		if merge, err = rt.MergeClone(ctx, "m"); err != nil {
			t.Fatal(err)
		}
		g := func(args ...string) {
			t.Helper()
			cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-C", merge}, args...)...)
			if out, err := cmd.CombinedOutput(); err != nil && args[0] != "merge" {
				t.Fatalf("git %v: %v %s", args, err, out)
			}
		}
		g("checkout", "-q", "-b", "side")
		os.WriteFile(filepath.Join(merge, "f.txt"), []byte("side\n"), 0o644)
		g("add", "f.txt")
		g("commit", "-q", "-m", "side")
		g("checkout", "-q", "main")
		os.WriteFile(filepath.Join(merge, "f.txt"), []byte("main\n"), 0o644)
		g("add", "f.txt")
		g("commit", "-q", "-m", "main")
		g("merge", "-q", "side")
		if _, err := os.Stat(filepath.Join(merge, ".git", "MERGE_HEAD")); err != nil {
			t.Fatal("no merge in progress")
		}
		for _, c := range []struct{ label, mode string }{{"merge-conflict:w1:a", "stage:f.txt"}, {"merge-conflict:w1:b", "probe"}} {
			l, mode := c.label, c.mode
			t.Setenv("STUB_SCRIPT", mode) // the stub has no records to count calls by
			if _, err := rt.Call(ctx, "resolve", Opts{Label: l, Dir: merge, Schema: json.RawMessage(verdictSchema)}); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	if s.StopReason != StopReturned {
		t.Fatalf("summary %+v", s)
	}
	out, err := exec.Command("git", "-C", merge, "diff", "--name-only", "--diff-filter=U").Output()
	if err != nil || strings.TrimSpace(string(out)) != "" {
		t.Errorf("unmerged after the conflict call: %q %v", out, err)
	}
	if b, _ := exec.Command("git", "-C", merge, "show", ":f.txt").Output(); string(b) != "resolved\n" {
		t.Errorf("staged f.txt %q", b)
	}
	checkProbe(t, probe(t, merge), merge)
}
