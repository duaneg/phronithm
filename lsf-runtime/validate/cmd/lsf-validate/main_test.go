package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testdata = "../../testdata/"

func runCmd(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestLoadStageFixtures(t *testing.T) {
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"overlap", []string{testdata + "overlap.plan.md"},
			"unserialised-overlap [docs-pass+parser]: docs-pass and parser have overlapping scope and no dependency path between them\n"},
		{"cycle", []string{testdata + "cycle.plan.md"},
			"cycle [docs-pass]: dependency cycle docs-pass -> wire-up -> docs-pass\n"},
		{"bad clears", []string{"-decisions", testdata + "bad-clears.decisions.md", testdata + "bad-clears.plan.md"},
			"invalid-plan [parser/tool]: decision D1 clears environment gate parser/tool; environment gates are probed, not decided\n"},
		{"bad gates", []string{testdata + "bad-gates.plan.md"},
			"invalid-plan [tests]: gates block: tests: command must be a non-empty string\n" +
				"invalid-plan [tests]: gates block: tests: timeout must be a positive duration such as 45m; got \"soon\"\n"},
		{"disagreement", []string{writeTemp(t, "x.plan.md", strings.Replace(readTestdata(t, "ok.plan.md"), "- **Type**: design", "- **Type**: feature", 1))},
			"plan-disagreement [docs-pass]: entries: docs-pass Type is \"feature\" in its entry, \"design\" in the block\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			code, out, _ := runCmd(c.args...)
			if code != 1 || out != c.want {
				t.Errorf("exit %d\n got %q\nwant %q", code, out, c.want)
			}
		})
	}
}

func TestOK(t *testing.T) {
	code, out, errOut := runCmd("-decisions", testdata+"ok.decisions.md", testdata+"ok.plan.md")
	if code != 0 || out != "" || errOut != "decision log: "+testdata+"ok.decisions.md\n" {
		t.Errorf("exit %d stdout %q stderr %q", code, out, errOut)
	}
}

// The live plan passes both halves of the check.
func TestLivePlan(t *testing.T) {
	path := "../../../../docs/plans/lsf-improvements-breakdown.md"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("plan absent")
	}
	if code, out, errOut := runCmd(path); code != 0 {
		t.Errorf("docs/plans/lsf-improvements-breakdown.md fails the pre-flight check (exit %d): the plan document disagrees with its block or fails the load stage\n%s%s", code, out, errOut)
	}
}

func TestEmptyPlanIsAProblem(t *testing.T) {
	code, out, _ := runCmd(writeTemp(t, "empty.md", ""))
	if code != 1 || out != "invalid-plan [lsf-plan]: no ```yaml lsf-plan block found\n" {
		t.Errorf("exit %d stdout %q", code, out)
	}
}

func TestUsageAndIOErrors(t *testing.T) {
	dir := t.TempDir()
	plan := writeTemp(t, "p.md", readTestdata(t, "ok.plan.md"))
	for _, c := range []struct {
		name string
		args []string
		want string // in stderr
	}{
		{"no plan", nil, "usage:"},
		{"two plans", []string{plan, plan}, "usage:"},
		{"unknown flag", []string{"-x", plan}, "usage:"},
		{"flag after plan", []string{plan, "-decisions", plan}, "usage:"},
		{"missing plan", []string{filepath.Join(dir, "absent.md")}, "no such file"},
		{"plan is a directory", []string{dir}, "is a directory"},
		{"missing decisions", []string{"-decisions", filepath.Join(dir, "absent.md"), plan}, "no such file"},
	} {
		t.Run(c.name, func(t *testing.T) {
			code, out, errOut := runCmd(c.args...)
			if code != 2 || out != "" || !strings.Contains(errOut, c.want) {
				t.Errorf("exit %d stdout %q stderr %q", code, out, errOut)
			}
		})
	}
}

func TestUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads anything")
	}
	dir := t.TempDir()
	plan := filepath.Join(dir, "x-breakdown.md")
	mustWrite(t, plan, readTestdata(t, "ok.plan.md"), 0o644)
	log := filepath.Join(dir, "x-decisions.md")
	mustWrite(t, log, readTestdata(t, "ok.decisions.md"), 0o000)
	if f, err := os.Open(log); err == nil {
		f.Close()
		t.Skip("mode bits not enforced here")
	}
	if code, _, errOut := runCmd(plan); code != 2 || !strings.Contains(errOut, "permission denied") {
		t.Errorf("unreadable sibling: exit %d stderr %q", code, errOut)
	}
	if err := os.Chmod(plan, 0o000); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runCmd("-decisions", testdata+"ok.decisions.md", plan); code != 2 || !strings.Contains(errOut, "permission denied") {
		t.Errorf("unreadable plan: exit %d stderr %q", code, errOut)
	}
}

func TestDecisionLogDerivation(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "x-breakdown.md")
	mustWrite(t, plan, readTestdata(t, "ok.plan.md"), 0o644)
	if code, _, errOut := runCmd(plan); code != 0 || errOut != "decision log: none\n" {
		t.Errorf("no sibling: exit %d stderr %q", code, errOut)
	}
	sibling := filepath.Join(dir, "x-decisions.md")
	mustWrite(t, sibling, readTestdata(t, "bad-clears.decisions.md"), 0o644)
	code, out, errOut := runCmd(plan)
	if code != 1 || errOut != "decision log: "+sibling+"\n" || !strings.Contains(out, "parser/tool") {
		t.Errorf("sibling: exit %d stdout %q stderr %q", code, out, errOut)
	}
	if code, _, errOut := runCmd("-decisions", testdata+"ok.decisions.md", plan); code != 0 || errOut != "decision log: "+testdata+"ok.decisions.md\n" {
		t.Errorf("override: exit %d stderr %q", code, errOut)
	}
	other := filepath.Join(dir, "x.plan.md")
	mustWrite(t, other, readTestdata(t, "ok.plan.md"), 0o644)
	if code, _, errOut := runCmd(other); code != 0 || errOut != "decision log: none\n" {
		t.Errorf("not a breakdown name: exit %d stderr %q", code, errOut)
	}
}

// A derived sibling that exists but is not a readable file is an error.
func TestSiblingIsADirectory(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "x-breakdown.md")
	mustWrite(t, plan, readTestdata(t, "ok.plan.md"), 0o644)
	if err := os.Mkdir(filepath.Join(dir, "x-decisions.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runCmd(plan); code != 2 || !strings.Contains(errOut, "is a directory") {
		t.Errorf("exit %d stderr %q", code, errOut)
	}
}

// A decision log without its block is input, so a problem, not an error.
func TestMalformedDecisionLog(t *testing.T) {
	log := writeTemp(t, "d.md", "# Decisions\n")
	code, out, _ := runCmd("-decisions", log, testdata+"ok.plan.md")
	if code != 1 || out != "invalid-plan [lsf-decisions]: no ```yaml lsf-decisions block found\n" {
		t.Errorf("exit %d stdout %q", code, out)
	}
}

// A relative PLAN resolves against the process's working directory.
func TestRelativePath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(wd, testdata))
	if code, out, _ := runCmd("cycle.plan.md"); code != 1 || !strings.HasPrefix(out, "cycle ") {
		t.Errorf("exit %d stdout %q", code, out)
	}
}

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(testdata + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
