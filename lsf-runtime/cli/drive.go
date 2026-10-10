package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

func init() {
	commands["drive"] = command{run: cmdDrive, hidden: true, usage: "drive -state DIR -launch FILE"}
}

// cmdDrive is the detached driver launch starts: one lsfrt.Run over the
// composed config. Its output goes to driver.log.
func cmdDrive(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("drive", stderr)
	state := fs.String("state", "", "the state dir")
	file := fs.String("launch", "", "the composed config")
	if err := parse(fs, args); err != nil {
		return fail(stderr, err)
	}
	if *state == "" || *file == "" {
		return fail(stderr, usageError{error: fmt.Errorf("drive: -state and -launch are required")})
	}
	sum, err := drive(*state, *file)
	if err != nil {
		fmt.Fprintln(stderr, err) // the runtime's errors carry their prefix
		return exitError
	}
	b, _ := json.Marshal(sum)
	fmt.Fprintf(stdout, "%s\n", b)
	return exitOK
}

// drive runs the launch whose config is at file. The body first renames
// file to launch.json: the lock is held by then, so a refused launch never
// replaces the last good one.
func drive(state, file string) (lsfrt.Summary, error) {
	// launch resolved claudeBin; the env must not override it.
	os.Unsetenv("LSF_CLAUDE_BIN")
	cfg, err := lsfrt.LoadConfig(file)
	if err != nil {
		return lsfrt.Summary{}, oldConfigHint(err)
	}
	body := func(ctx context.Context, rt *lsfrt.Runtime) (json.RawMessage, error) {
		if err := os.Rename(file, filepath.Join(state, launchFile)); err != nil {
			return nil, fmt.Errorf("launch.json: %w", err)
		}
		// Composed files of launches that gave up waiting on a refused
		// driver. Their drivers cannot reach this body: this one holds the
		// lock until it exits.
		stale, _ := filepath.Glob(filepath.Join(state, "launch.*.json"))
		for _, p := range stale {
			os.Remove(p)
		}
		return orchestrator.Body(ctx, rt)
	}
	return lsfrt.Run(context.Background(), cfg, body)
}

// The CLI's files in a state dir.
const (
	launchFile = "launch.json" // the current launch's composed config
	driverFile = "driver.json" // the current launch's driver: pid, started, launchFile
	driverLog  = "driver.log"  // the drivers' stdout and stderr, appended
)
