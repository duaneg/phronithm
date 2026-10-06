// Command fixturerun runs the ported orchestrator end to end on the spike
// fixture (workflows/lsf/spike/fixture.sh: one unit, add-mean, gated by
// node --test and a linter) under the runtime and a real, pinned claude.
// Every stage is a model call: it costs real money, so it is not part of go
// test. Run from lsf-runtime/: go run ./tools/fixturerun.
//
// It exits non-zero unless the launch returns status complete, no label was
// reused, every call's transcript records the pinned Claude Code version,
// the fixture's main has the add-mean merge, and the fixture's gates pass
// on main. The work dir (repo and state) is kept for inspection.
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

const pin = "2.1.289"

// config is the launch config: the example's roles and skill rules, this
// fixture's repo, state dir and args, and caps sized for one unit (about
// fourteen calls).
func config(examplePath, dir, claude, planSha string, dryRun bool) (lsfrt.Config, error) {
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
	c.ClaudeBin = claude
	c.Args, _ = json.Marshal(map[string]string{
		"name": "stats", "planSha": planSha, "architectureDoc": "docs/architecture/stats.md", "integrationBranch": "main",
	})
	c.Caps = lsfrt.Caps{MaxCalls: 40, MaxCostUSD: 25, MaxWallClock: lsfrt.Duration(time.Hour)}
	c.DryRun = dryRun
	return c, nil
}

// fixture builds the fixture repo at dir/repo and returns its plan SHA.
func fixture(root, dir string) (string, error) {
	out, err := exec.Command("sh", filepath.Join(root, "workflows", "lsf", "spike", "fixture.sh"), filepath.Join(dir, "repo")).Output()
	if err != nil {
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
	root := flag.String("repo", "..", "phronithm repository root")
	claude := flag.String("claude", filepath.Join(home, ".local", "share", "claude", "versions", pin), "claude binary (the pinned version)")
	dirFlag := flag.String("dir", "", "work dir (default: a new temp dir, kept)")
	example := flag.String("config", filepath.Join("testdata", "config.example.json"), "config whose roles and skill rules the launch uses")
	dryRun := flag.Bool("dry-run", false, "stop at the first call without spawning")
	flag.Parse()
	if err := run(*root, *claude, *dirFlag, *example, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "fixturerun:", err)
		os.Exit(1)
	}
}

func run(root, claude, dir, example string, dryRun bool) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if dir == "" {
		if dir, err = os.MkdirTemp("", "lsf-fixturerun-"); err != nil {
			return err
		}
	}
	planSha, err := fixture(root, dir)
	if err != nil {
		return err
	}
	cfg, err := config(example, dir, claude, planSha, dryRun)
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
	json.Unmarshal(sum.Outcome, &out)
	fmt.Printf("status %s\n", out.Status)
	if out.Escalation != nil {
		fmt.Printf("escalation %s\n", out.Escalation)
	}

	var problems []string
	if out.Status != "complete" {
		problems = append(problems, fmt.Sprintf("status %q (stop_reason %s %s %s)", out.Status, sum.StopReason, sum.Detail, sum.Error))
	}
	logText, _ := os.ReadFile(filepath.Join(cfg.StateDir, "log.txt"))
	if bytes.Contains(logText, []byte("label reused")) {
		problems = append(problems, "log.txt reports a reused label")
	}
	tel, _ := os.ReadFile(filepath.Join(cfg.StateDir, "telemetry.jsonl"))
	fmt.Printf("skill-not-loaded events: %d\n", bytes.Count(tel, []byte(`"skill-not-loaded"`)))
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
		if !strings.Contains(log, "merge(stats): add-mean") {
			problems = append(problems, "main has no add-mean merge")
		}
	} else {
		problems = append(problems, "git log: "+err.Error())
	}
	for _, gate := range [][]string{{"node", "--test"}, {"node", "scripts/lint.js"}} {
		cmd := exec.Command(gate[0], gate[1:]...)
		cmd.Dir = cfg.Repo
		if b, err := cmd.CombinedOutput(); err != nil {
			problems = append(problems, fmt.Sprintf("%s on main: %v\n%s", strings.Join(gate, " "), err, b))
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	fmt.Println("fixturerun: complete")
	return nil
}
