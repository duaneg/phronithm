// Command lsf-runtime runs Phase 3 of phronithm:large-scale-feature on the
// host runtime: it launches a detached driver over a run's state dir, and
// reports, pauses, stops, resumes and watches it
// (docs/design/lsf-runtime.md, CLI).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Exit codes.
const (
	exitOK       = 0
	exitError    = 1 // a refusal or error, its message on stderr
	exitUsage    = 2
	exitCrashed  = 3 // watch and status: crashed or failed-to-start
	exitNoLaunch = 4 // watch and status: no launch on the state dir
)

// command is a subcommand: its args after the name, and the streams.
type command struct {
	run    func(args []string, stdout, stderr io.Writer) int
	hidden bool
	usage  string
}

// commands are the subcommands; each file adds its own.
var commands = map[string]command{}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(argv []string, stdout, stderr io.Writer) int {
	if len(argv) == 0 {
		usage(stderr)
		return exitUsage
	}
	c, ok := commands[argv[0]]
	if !ok {
		fmt.Fprintf(stderr, "lsf-runtime: unknown command %q\n", argv[0])
		usage(stderr)
		return exitUsage
	}
	return c.run(argv[1:], stdout, stderr)
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: lsf-runtime <command> [flags]")
	for _, name := range sortedKeys(commands) {
		if c := commands[name]; !c.hidden {
			fmt.Fprintf(w, "  %s\n", c.usage)
		}
	}
	fmt.Fprintln(w, "A run is addressed by -state DIR, or by -repo DIR (default: the current repo) and -name NAME.")
}

// usageError is a bad command line: exit 2. printed marks one the flag
// package has already reported.
type usageError struct {
	error
	printed bool
}

// fail prints err and returns its exit code.
func fail(stderr io.Writer, err error) int {
	var ue usageError
	if errors.As(err, &ue) {
		if !ue.printed {
			fmt.Fprintln(stderr, "lsf-runtime:", err)
		}
		return exitUsage
	}
	fmt.Fprintln(stderr, "lsf-runtime:", err)
	return exitError
}

// newFlags is a subcommand's flag set, its errors returned, not fatal.
func newFlags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

// parse parses a subcommand's flags; positional arguments are refused.
func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return usageError{err, true}
	}
	if fs.NArg() > 0 {
		return usageError{error: fmt.Errorf("%s: unexpected argument %q", fs.Name(), fs.Arg(0))}
	}
	return nil
}

// addr addresses a run: its state dir, or a repo and a name.
type addr struct{ state, repo, name string }

func (a *addr) flags(fs *flag.FlagSet) {
	fs.StringVar(&a.state, "state", "", "the run's state dir")
	fs.StringVar(&a.repo, "repo", "", "the repository (default: the current directory's)")
	fs.StringVar(&a.name, "name", "", "the run's name (args.name)")
}

// stateDir is the addressed state dir: -state, or the default for the
// repo and name.
func (a *addr) stateDir() (string, error) {
	if a.state != "" {
		return filepath.Abs(a.state)
	}
	if a.name == "" {
		return "", usageError{error: errors.New("pass -state, or -name (with -repo outside the repository)")}
	}
	if err := checkName(a.name); err != nil {
		return "", usageError{error: err}
	}
	top, err := repoTop(a.repo)
	if err != nil {
		return "", err
	}
	return defaultStateDir(top, a.name), nil
}

// repoTop is the toplevel of dir's repository ("" for the current
// directory), symlinks resolved.
func repoTop(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("%s is not in a git repository: %v", dir, err)
	}
	return filepath.EvalSymlinks(strings.TrimSpace(string(out)))
}

// defaultStateDir is ${XDG_STATE_HOME:-~/.local/state}/lsf-runtime/<12 hex
// of the repo toplevel's SHA-256>/<name>.
func defaultStateDir(top, name string) string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	sum := sha256.Sum256([]byte(top))
	return filepath.Join(base, "lsf-runtime", hex.EncodeToString(sum[:])[:12], name)
}
