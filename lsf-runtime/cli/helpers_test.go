package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/internal/testcfg"
	"github.com/duaneg/phronithm/lsf-runtime/orchestrator/orchtest"
	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

// The test binary is three programs. With LSF_CLI_FAKE_TABLE set and a
// claude command line, it is a fake claude answering from the table (see
// fakeClaude). With LSF_CLI_AS_MAIN=1 it is the CLI, so launch's re-exec
// of itself starts a driver. Otherwise it runs the tests.
func TestMain(m *testing.M) {
	if os.Getenv("LSF_CLI_FAKE_TABLE") != "" && (len(os.Args) == 2 && os.Args[1] == "--version" || slices.Contains(os.Args, "--session-id")) {
		os.Exit(fakeClaude())
	}
	if os.Getenv("LSF_CLI_AS_MAIN") == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

// answer is one fake-claude table entry: the first whose Contains the
// prompt holds answers, after running Shell (bash, in the call's cwd). In
// Return, @branch and @head stand for the cwd's branch and HEAD.
type answer struct {
	Contains string `json:"contains"`
	Return   any    `json:"return"`
	Shell    string `json:"shell,omitempty"`
}

// fakeClaude answers a call from LSF_CLI_FAKE_TABLE and records it in
// LSF_CLI_FAKE_CALLS as a file named for the matched entry.
func fakeClaude() int {
	if len(os.Args) == 2 {
		fmt.Println(testcfg.ClaudeVersion + " (Claude Code)")
		return 0
	}
	session := "fake"
	for i, a := range os.Args {
		if a == "--session-id" && i+1 < len(os.Args) {
			session = os.Args[i+1]
		}
	}
	prompt, _ := io.ReadAll(os.Stdin)
	b, err := os.ReadFile(os.Getenv("LSF_CLI_FAKE_TABLE"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake claude:", err)
		return 3
	}
	var table []struct {
		Contains string          `json:"contains"`
		Return   json.RawMessage `json:"return"`
		Shell    string          `json:"shell"`
	}
	if err := json.Unmarshal(b, &table); err != nil {
		fmt.Fprintln(os.Stderr, "fake claude:", err)
		return 3
	}
	fmt.Printf(`{"type":"system","subtype":"init","session_id":%q,"tools":["Read"],"slash_commands":[]}`+"\n", session)
	for i, e := range table {
		if !strings.Contains(string(prompt), e.Contains) {
			continue
		}
		if dir := os.Getenv("LSF_CLI_FAKE_CALLS"); dir != "" {
			f, _ := os.CreateTemp(dir, fmt.Sprintf("call-%d-", i))
			f.Close()
		}
		if e.Shell != "" {
			if out, err := exec.Command("bash", "-c", e.Shell).CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "fake claude: shell: %v %s\n", err, out)
				return 3
			}
		}
		v := string(e.Return)
		for tok, args := range map[string][]string{"@branch": {"rev-parse", "--abbrev-ref", "HEAD"}, "@head": {"rev-parse", "HEAD"}} {
			if strings.Contains(v, tok) {
				out, _ := exec.Command("git", args...).Output()
				v = strings.ReplaceAll(v, tok, strings.TrimSpace(string(out)))
			}
		}
		fmt.Printf(`{"type":"result","subtype":"success","is_error":false,"structured_output":%s,"total_cost_usd":0.01}`+"\n", v)
		return 0
	}
	fmt.Println(`{"type":"result","subtype":"success","is_error":true,"api_error_status":400,"total_cost_usd":0}`)
	return 1
}

type M = map[string]any

// fixture is a repository holding a one-unit plan (unit A, its work on
// branch work-A) on the integration branch, and a host config for the
// stub podman.
type fixture struct {
	t       *testing.T
	repo    string
	planSha string
	workTip string
	host    string // the host config file
	podman  string // the stub podman's records
	xdg     string // XDG_STATE_HOME
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tc := testcfg.New(t)
	f := &fixture{t: t, repo: t.TempDir(), podman: tc.State, xdg: t.TempDir()}
	f.repo, _ = filepath.EvalSymlinks(f.repo)
	t.Setenv("XDG_STATE_HOME", f.xdg)
	t.Setenv("LSF_CLAUDE_BIN", "")
	f.git("init", "-q", "-b", "integration")
	f.git("config", "user.name", "t")
	f.git("config", "user.email", "t@example.invalid")
	f.git("config", "commit.gpgsign", "false")
	f.commit("README.md", "readme\n", "init")
	f.commit("docs/architecture/f.md", "# Architecture\n", "architecture")
	p := orchtest.Graph("", orchtest.Unit("A"))
	var doc strings.Builder
	doc.WriteString("# Plan: f\n\n## Work Units\n\n")
	for _, u := range p.Units {
		doc.WriteString("### " + u.ID + "\n- **Type**: " + u.Type + "\n\n")
	}
	doc.WriteString("## Graph\n\n" + orchtest.ToBlock(p) + "\n\n## Plan Maintenance\n")
	f.planSha = f.commit("docs/plans/f-breakdown.md", doc.String(), "plan")
	f.git("checkout", "-q", "-b", "work-A")
	f.workTip = f.commit("src/A/a.go", "package a\n", "unit A")
	f.git("checkout", "-q", "integration")
	f.host = filepath.Join(t.TempDir(), "config.json")
	f.writeHost(M{})
	return f
}

func (f *fixture) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *fixture) commit(path, content, msg string) string {
	f.t.Helper()
	p := filepath.Join(f.repo, path)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.git("add", path)
	f.git("commit", "-q", "-m", msg)
	return f.git("rev-parse", "HEAD")
}

// writeHost writes the host config: the stub podman and claude, the test
// credentials, and over.
func (f *fixture) writeHost(over M) {
	f.t.Helper()
	creds := filepath.Join(f.t.TempDir(), ".credentials.json")
	b, _ := json.Marshal(M{"claudeAiOauth": M{"accessToken": testcfg.Token}})
	os.WriteFile(creds, b, 0o600)
	h := M{
		"image": testcfg.Image, "podmanBin": testcfg.Podman(), "claudeBin": testcfg.Claude(),
		"pluginDir": f.t.TempDir(), "proxy": M{"listen": "127.0.0.1:0", "credentialsFile": creds},
		"backoff": []string{"10ms"},
	}
	for k, v := range over {
		if v == nil {
			delete(h, k)
		} else {
			h[k] = v
		}
	}
	b, _ = json.MarshalIndent(h, "", "  ")
	if err := os.WriteFile(f.host, b, 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// args is the launch args for the fixture's plan.
func (f *fixture) args() json.RawMessage {
	b, _ := plan.MarshalJS(M{"name": "f", "planSha": f.planSha, "architectureDoc": "docs/architecture/f.md", "integrationBranch": "integration"}, "")
	return b
}

// argsFile writes args to a file and returns its path.
func (f *fixture) argsFile() string {
	p := filepath.Join(f.t.TempDir(), "args.json")
	os.WriteFile(p, f.args(), 0o644)
	return p
}

// stateDir is the fixture run's default state dir.
func (f *fixture) stateDir() string { return defaultStateDir(f.repo, "f") }

// result is a CLI run's output.
type result struct {
	stdout, stderr string
	code           int
}

// cli runs the CLI as its own process (the test binary as main), in the
// fixture repo, with env added.
func (f *fixture) cli(env []string, args ...string) result {
	f.t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Dir = f.repo
	cmd.Env = append(append(os.Environ(), "LSF_CLI_AS_MAIN=1"), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		f.t.Fatal(err)
	}
	return result{out.String(), errb.String(), code}
}

// launch runs launch -args over the fixture's args with the host config.
func (f *fixture) launch(env []string, extra ...string) result {
	f.t.Helper()
	return f.cli(env, append([]string{"launch", "-args", f.argsFile(), "-config", f.host}, extra...)...)
}

// waitFor polls cond every 50 ms for up to d.
func waitFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(d); !cond(); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func eq[T comparable](t *testing.T, got, want T, msg ...any) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v %v", got, want, fmt.Sprint(msg...))
	}
}

func contains(t *testing.T, s, sub string, msg ...any) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("%q does not contain %q %v", s, sub, fmt.Sprint(msg...))
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
