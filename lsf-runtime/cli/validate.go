package main

import (
	"io"

	"github.com/duaneg/phronithm/lsf-runtime/validate"
)

func init() {
	commands["validate"] = command{
		run: func(args []string, stdout, stderr io.Writer) int {
			return validate.Command("lsf-runtime validate", args, stdout, stderr)
		},
		usage: "validate [-decisions PATH] PLAN",
	}
}
