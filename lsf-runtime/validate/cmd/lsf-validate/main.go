// Command lsf-validate checks a plan document before launch: the load
// stage's checks and the agreement of its `## Graph` block with its entries.
//
//	lsf-validate [-decisions PATH] PLAN
//
// It is validate.Command; the CLI's `lsf-runtime validate` is the same.
// Exit status: 0 with no problems, 1 with problems, 2 on a usage or I/O
// error.
package main

import (
	"io"
	"os"

	"github.com/duaneg/phronithm/lsf-runtime/validate"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return validate.Command("lsf-validate", args, stdout, stderr)
}
