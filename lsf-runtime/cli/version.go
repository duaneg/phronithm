package main

import (
	"fmt"
	"io"
)

// version and commit are set by the release build's -ldflags -X.
var (
	version = "dev"
	commit  = "unknown"
)

func init() {
	commands["version"] = command{run: cmdVersion, usage: "version"}
}

// cmdVersion prints "<version> <commit>".
func cmdVersion(args []string, stdout, stderr io.Writer) int {
	if err := parse(newFlags("version", stderr), args); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintln(stdout, version, commit)
	return exitOK
}
