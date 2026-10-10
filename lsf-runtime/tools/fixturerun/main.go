// Command fixturerun runs the ported orchestrator end to end on the spike
// fixture (workflows/lsf/spike/fixture.sh: one unit, add-mean, gated by
// node --test and a linter) under the runtime and a real, pinned claude,
// each call in its own podman container. Every stage is a model call: it
// costs real money, so it is not part of go test. Run from lsf-runtime/:
//
//	go run ./tools/fixturerun -image <name@sha256:digest>
//
// The image needs node for the fixture: the reference Containerfile
// (container/Containerfile) extended with nodejs and npm, for example
//
//	FROM localhost/lsf-ref@sha256:<the reference image's digest>
//	RUN apt-get update && apt-get install -y --no-install-recommends nodejs npm && rm -rf /var/lib/apt/lists/*
//
// built with podman build and named by its digest (podman image inspect
// --format '{{.Digest}}'). Calls reach the API through the credential
// proxy, with the live session's credentials file; the plan has no hosts.
//
// It exits non-zero unless the launch returns status complete, no label was
// reused, every call's transcript records the pinned Claude Code version,
// the fixture's main has the add-mean merge, the fixture's gates pass on
// main, and no container of the launch is left. The work dir (repo and
// state) is kept for inspection.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

const pin = "2.1.294"

// options are a run's inputs.
type options struct {
	root, dir, example string
	claude, version    string // the binary and its pinned version
	podman, image      string
	credentials        string // the live session's credentials file
	dryRun             bool
}

// config is the launch config: the example's roles and skill rules, this
// fixture's repo, state dir and args, the plugin at root, the container,
// the proxy, and caps sized for one unit (about fourteen calls).
func config(o options, planSha string) (lsfrt.Config, error) {
	examplePath, root, dir := o.example, o.root, o.dir
	b, err := os.ReadFile(examplePath)
	if err != nil {
		return lsfrt.Config{}, err
	}
	var c lsfrt.Config
	if err := json.Unmarshal(b, &c); err != nil {
		return lsfrt.Config{}, fmt.Errorf("%s: %w", examplePath, err)
	}
	c.StateDir = filepath.Join(dir, "state")
	c.Repo = filepath.Join(dir, "repo")
	c.ClaudeBin, c.ClaudeVersion = o.claude, o.version
	c.PodmanBin, c.Image = o.podman, o.image
	c.Env = nil
	c.Proxy = &lsfrt.ProxyConfig{Listen: "127.0.0.1:0", CredentialsFile: o.credentials}
	c.PluginDir = root
	c.Args, _ = json.Marshal(map[string]string{
		"name": "stats", "planSha": planSha, "architectureDoc": "docs/architecture/stats.md", "integrationBranch": "main",
	})
	c.Caps = lsfrt.Caps{MaxCalls: 40, MaxCostUSD: 25, MaxWallClock: lsfrt.Duration(time.Hour)}
	c.DryRun = o.dryRun
	return c, nil
}

// fixture builds the fixture repo at dir/repo and returns its plan SHA.
func fixture(root, dir string) (string, error) {
	out, err := exec.Command("sh", filepath.Join(root, "workflows", "lsf", "spike", "fixture.sh"), filepath.Join(dir, "repo")).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("fixture.sh: %w: %s", err, ee.Stderr)
		}
		return "", fmt.Errorf("fixture.sh: %w", err)
	}
	lines := strings.Fields(string(out))
	if len(lines) == 0 {
		return "", errors.New("fixture.sh printed no plan SHA")
	}
	return lines[len(lines)-1], nil
}

// versions are the Claude Code versions the calls' transcripts record.
func versions(stateDir string) (map[string]int, error) {
	files, err := filepath.Glob(filepath.Join(stateDir, "calls", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			return nil, err
		}
		v := "(no init)"
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var e struct {
				Type    string `json:"type"`
				Subtype string `json:"subtype"`
				Version string `json:"claude_code_version"`
			}
			if json.Unmarshal(sc.Bytes(), &e) == nil && e.Type == "system" && e.Subtype == "init" {
				v = e.Version
				break
			}
		}
		fh.Close()
		out[v]++
	}
	return out, nil
}

func git(repo string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
	return string(out), err
}

func main() {
	home, _ := os.UserHomeDir()
	o := options{version: pin}
	flag.StringVar(&o.root, "repo", "..", "phronithm repository root")
	flag.StringVar(&o.claude, "claude", filepath.Join(home, ".local", "share", "claude", "versions", pin), "claude binary (the pinned version)")
	flag.StringVar(&o.dir, "dir", "", "work dir (default: a new dir under the user cache dir, kept)")
	flag.StringVar(&o.example, "config", filepath.Join("testdata", "config.example.json"), "config whose roles and skill rules the launch uses")
	flag.StringVar(&o.podman, "podman", "podman", "podman binary")
	flag.StringVar(&o.image, "image", "", "the calls' image, by digest (required; it needs node)")
	flag.StringVar(&o.credentials, "credentials", filepath.Join(home, ".claude", ".credentials.json"), "the live session's credentials file")
	flag.BoolVar(&o.dryRun, "dry-run", false, "stop at the first call without spawning")
	flag.Parse()
	if o.image == "" {
		fmt.Fprintln(os.Stderr, "fixturerun: -image is required")
		os.Exit(2)
	}
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "fixturerun:", err)
		os.Exit(1)
	}
}

func run(o options) error {
	root, err := filepath.Abs(o.root)
	if err != nil {
		return err
	}
	o.root = root
	dir, claude, dryRun := o.dir, o.claude, o.dryRun
	if dir == "" {
		// Kept for inspection: not os.TempDir, which may be cleared.
		cache, err := os.UserCacheDir()
		if err != nil {
			return fmt.Errorf("no default work dir (%v): pass -dir", err)
		}
		if err := os.MkdirAll(filepath.Join(cache, "lsf-fixturerun"), 0o755); err != nil {
			return err
		}
		if dir, err = os.MkdirTemp(filepath.Join(cache, "lsf-fixturerun"), "run-"); err != nil {
			return err
		}
		o.dir = dir
	}
	// A state dir from an earlier launch would replay its journal and mix
	// its transcripts into the version check.
	if _, err := os.Stat(filepath.Join(dir, "state")); err == nil {
		return fmt.Errorf("%s/state exists: use a fresh -dir", dir)
	}
	// Check the pin before spending anything.
	if !dryRun {
		cmd := exec.Command(claude, "--version")
		cmd.Env = append(os.Environ(), "DISABLE_AUTOUPDATER=1")
		v, err := cmd.Output()
		if err != nil || !strings.HasPrefix(strings.TrimSpace(string(v)), pin+" ") {
			return fmt.Errorf("%s --version: %q %v; want %s", claude, strings.TrimSpace(string(v)), err, pin)
		}
	}
	planSha, err := fixture(root, dir)
	if err != nil {
		return err
	}
	cfg, err := config(o, planSha)
	if err != nil {
		return err
	}
	// The pinned binary must not update itself, and LSF_CLAUDE_BIN would
	// override it.
	os.Setenv("DISABLE_AUTOUPDATER", "1")
	os.Unsetenv("LSF_CLAUDE_BIN")
	fmt.Printf("work dir %s; plan %s; claude %s\n", dir, planSha, claude)

	sum, err := lsfrt.Run(context.Background(), cfg, orchestrator.Body)
	if err != nil {
		return err
	}
	fmt.Printf("stop_reason %s; %d call(s); $%.2f; %s\n", sum.StopReason, sum.Calls, sum.CostUSD, sum.Finished.Sub(sum.Started).Round(time.Second))
	if dryRun {
		if sum.StopReason != lsfrt.StopDryRun {
			return fmt.Errorf("dry run ended %s", sum.StopReason)
		}
		return nil
	}
	var out struct {
		Status     string          `json:"status"`
		Escalation json.RawMessage `json:"escalation"`
	}
	if err := json.Unmarshal(sum.Outcome, &out); err != nil {
		return fmt.Errorf("outcome %q: %w", sum.Outcome, err)
	}
	fmt.Printf("status %s\n", out.Status)
	var problems []string
	if out.Escalation != nil {
		fmt.Printf("escalation %s\n", out.Escalation)
	}

	if out.Status != "complete" {
		problems = append(problems, fmt.Sprintf("status %q (stop_reason %s %s %s)", out.Status, sum.StopReason, sum.Detail, sum.Error))
	}
	logText, err := os.ReadFile(filepath.Join(cfg.StateDir, "log.txt"))
	if err != nil {
		return err
	}
	if bytes.Contains(logText, []byte("label reused")) {
		problems = append(problems, "log.txt reports a reused label")
	}
	tel, err := os.ReadFile(filepath.Join(cfg.StateDir, "telemetry.jsonl"))
	if err != nil {
		return err
	}
	kinds := map[string]int{}
	for _, line := range bytes.Split(bytes.TrimSpace(tel), []byte("\n")) {
		var e struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(line, &e) == nil {
			kinds[e.Kind]++
		}
	}
	fmt.Printf("skill-not-loaded events: %d\n", kinds["skill-not-loaded"])
	if kinds["label-reused"] > 0 {
		problems = append(problems, "telemetry reports a reused label")
	}
	vs, err := versions(cfg.StateDir)
	if err != nil {
		return err
	}
	fmt.Printf("transcript versions: %v\n", vs)
	for v := range vs {
		if v != pin {
			problems = append(problems, fmt.Sprintf("a transcript records version %s, not %s", v, pin))
		}
	}
	if len(vs) == 0 {
		problems = append(problems, "no call transcripts")
	}
	if log, err := git(cfg.Repo, "log", "--oneline", "main"); err == nil {
		fmt.Printf("main:\n%s", log)
	}
	// A merge commit on main, and the unit's deliverable in its tree.
	if merges, err := git(cfg.Repo, "log", "--merges", "--oneline", "main"); err != nil || strings.TrimSpace(merges) == "" {
		problems = append(problems, "main has no merge commit")
	}
	probe := exec.Command("node", "--input-type=module", "-e", `const { mean } = await import('./src/stats.js');
if (mean([1, 2, 3]) !== 2) throw new Error('mean([1, 2, 3]) is ' + mean([1, 2, 3]));
try { mean([]); throw new Error('mean([]) did not throw'); } catch (e) { if (!(e instanceof RangeError)) throw e; }`)
	probe.Dir = cfg.Repo
	if b, err := probe.CombinedOutput(); err != nil {
		problems = append(problems, fmt.Sprintf("mean on main: %v %s", err, b))
	}
	for _, gate := range [][]string{{"node", "--test"}, {"node", "scripts/lint.js"}} {
		cmd := exec.Command(gate[0], gate[1:]...)
		cmd.Dir = cfg.Repo
		if b, err := cmd.CombinedOutput(); err != nil {
			problems = append(problems, fmt.Sprintf("%s on main: %v\n%s", strings.Join(gate, " "), err, b))
		}
	}
	label, err := lsfrt.StateLabel(cfg.StateDir)
	if err != nil {
		return err
	}
	if left, err := exec.Command(o.podman, "ps", "-a", "--filter", "label=lsf.state="+label, "--format", "{{.Names}}").Output(); err != nil {
		problems = append(problems, fmt.Sprintf("podman ps: %v", err))
	} else if names := strings.Fields(string(left)); len(names) > 0 {
		problems = append(problems, fmt.Sprintf("containers left: %v", names))
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	fmt.Println("fixturerun: complete")
	return nil
}
