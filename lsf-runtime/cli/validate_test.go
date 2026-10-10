package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// validate is lsf-validate under the CLI: its exit codes, problems on
// stdout and errors on stderr.
func TestValidate(t *testing.T) {
	const td = "../validate/testdata/"
	for _, c := range []struct {
		name   string
		args   []string
		code   int
		stdout string // a prefix
		stderr string // a substring
	}{
		{"ok", []string{"-decisions", td + "ok.decisions.md", td + "ok.plan.md"}, 0, "", "decision log: " + td + "ok.decisions.md"},
		{"problems", []string{td + "cycle.plan.md"}, 1, "cycle [docs-pass]: ", "decision log: none"},
		{"missing", []string{filepath.Join(t.TempDir(), "absent.md")}, 2, "", "lsf-runtime validate: "},
		{"usage", nil, 2, "", "usage: lsf-runtime validate [-decisions PATH] PLAN"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			code := run(append([]string{"validate"}, c.args...), &out, &errb)
			eq(t, code, c.code)
			if !strings.HasPrefix(out.String(), c.stdout) || (c.stdout == "") != (out.Len() == 0) {
				t.Errorf("stdout %q, want prefix %q", out.String(), c.stdout)
			}
			contains(t, errb.String(), c.stderr)
		})
	}
}
