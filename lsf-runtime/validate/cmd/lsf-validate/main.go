// Command lsf-validate checks a plan document before launch: the load
// stage's checks and the agreement of its `## Graph` block with its entries.
//
//	lsf-validate [-decisions PATH] PLAN
//
// Without -decisions, a PLAN named <name>-breakdown.md reads the sibling
// <name>-decisions.md when it exists. Each problem is printed on stdout as
// "<kind> [<referent>]: <statement>". Exit status: 0 with no problems, 1
// with problems, 2 on a usage or I/O error.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/duaneg/phronithm/lsf-runtime/validate"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = "usage: lsf-validate [-decisions PATH] PLAN"

func run(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("lsf-validate", flag.ContinueOnError)
	fl.SetOutput(stderr)
	fl.Usage = func() { fmt.Fprintln(stderr, usage) }
	decisionsPath := fl.String("decisions", "", "the decision log (default: the plan's sibling <name>-decisions.md, if any)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() != 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	planPath := fl.Arg(0)
	planText, err := readFile(planPath)
	if err != nil {
		fmt.Fprintln(stderr, "lsf-validate:", err)
		return 2
	}
	logPath := *decisionsPath
	if logPath == "" {
		if name, ok := strings.CutSuffix(planPath, "-breakdown.md"); ok {
			sibling := name + "-decisions.md"
			if _, err := os.Stat(sibling); err == nil || !errors.Is(err, fs.ErrNotExist) {
				logPath = sibling
			}
		}
	}
	var decisions *string
	if logPath == "" {
		fmt.Fprintln(stderr, "decision log: none")
	} else {
		text, err := readFile(logPath)
		if err != nil {
			fmt.Fprintln(stderr, "lsf-validate:", err)
			return 2
		}
		decisions = &text
		fmt.Fprintln(stderr, "decision log:", logPath)
	}
	problems := validate.Check(planText, decisions)
	for _, p := range problems {
		fmt.Fprintf(stdout, "%s [%s]: %s\n", p.Kind, p.Referent, p.Statement)
	}
	if len(problems) > 0 {
		return 1
	}
	return 0
}

// readFile reads a regular file; a directory is an error.
func readFile(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	b, err := os.ReadFile(path)
	return string(b), err
}
