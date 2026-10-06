package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
)

// Run is run(runtime): the wave loop. Step 1 of the port parses the args
// only; the loop lands with the stages.
func Run(ctx context.Context, rt Runtime) (json.RawMessage, error) {
	a, err := parseArgs(rt.Args())
	if err != nil {
		return nil, err
	}
	if _, err := newRunCtx(a); err != nil {
		return nil, err
	}
	return nil, errors.New("lsf-implement: the Go wave loop is not ported yet")
}
