package validate

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

// Command runs the check as a command named name: args are
// [-decisions PATH] PLAN. Without -decisions, a PLAN named
// <name>-breakdown.md reads the sibling <name>-decisions.md unless it does
// not exist; one that exists but cannot be read is an error. Each problem
// is printed on stdout as "<kind> [<referent>]: <statement>". It returns 0
// with no problems, 1 with problems, 2 on a usage or I/O error.
func Command(name string, args []string, stdout, stderr io.Writer) int {
	usage := "usage: " + name + " [-decisions PATH] PLAN"
	fl := flag.NewFlagSet(name, flag.ContinueOnError)
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
		fmt.Fprintln(stderr, name+":", err)
		return 2
	}
	logPath := *decisionsPath
	if logPath == "" {
		if base, ok := strings.CutSuffix(planPath, "-breakdown.md"); ok {
			sibling := base + "-decisions.md"
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
			fmt.Fprintln(stderr, name+":", err)
			return 2
		}
		decisions = &text
		fmt.Fprintln(stderr, "decision log:", logPath)
	}
	problems := Check(planText, decisions)
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
