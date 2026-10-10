package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

func init() {
	commands["pause"] = command{run: cmdPause, usage: "pause [-lift] [-state DIR | -repo DIR -name NAME]"}
	commands["stop"] = command{run: cmdStop, usage: "stop [-state DIR | -repo DIR -name NAME]"}
	commands["resume"] = command{run: cmdResume, usage: "resume [-state DIR | -repo DIR -name NAME] [-config FILE] [-plugin-dir DIR]"}
}

// cmdPause sets the run's pause ref to the repo's HEAD, or with -lift
// deletes it, and verifies the result. The orchestrator reads it at its
// next wave boundary.
func cmdPause(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("pause", stderr)
	var a addr
	a.flags(fs)
	lift := fs.Bool("lift", false, "delete the pause ref")
	if err := parse(fs, args); err != nil {
		return fail(stderr, err)
	}
	state, err := a.stateDir()
	if err != nil {
		return fail(stderr, err)
	}
	fb := fallback{name: a.name}
	if a.repo != "" || a.state == "" {
		fb.repo, _ = repoTop(a.repo)
	}
	r := statusOf(state, fb)
	if r.Pause == nil {
		return fail(stderr, errors.New("no repo and name for the pause ref: pass -repo and -name, or -state of a launched run"))
	}
	ref := r.Pause.Ref
	git := func(args ...string) error {
		out, err := exec.Command("git", append([]string{"-C", r.Repo}, args...)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("git %s: %v %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if *lift {
		err = git("update-ref", "-d", ref)
	} else {
		err = git("update-ref", ref, "HEAD")
	}
	if err != nil {
		return fail(stderr, err)
	}
	set, err := refExists(r.Repo, ref)
	if err != nil {
		return fail(stderr, err)
	}
	if set == *lift {
		return fail(stderr, fmt.Errorf("%s: set is %t after the update", ref, set))
	}
	if set {
		fmt.Fprintf(stdout, "pause set: %s; the run pauses at its next wave boundary, not at once\n", ref)
	} else {
		fmt.Fprintf(stdout, "pause lifted: %s deleted\n", ref)
	}
	return exitOK
}

// cmdStop sends SIGTERM to the running driver: in-flight calls finish
// and the run stops at the next boundary. A second stop cancels them.
func cmdStop(args []string, stdout, stderr io.Writer) int {
	state, _, err := addressed("stop", args, stderr)
	if err != nil {
		return fail(stderr, err)
	}
	held, err := lockHeld(state)
	if err != nil {
		return fail(stderr, err)
	}
	var dp *driverRecord
	if d, err := readDriver(state); err == nil {
		dp = &d
	}
	var sp *statusFile
	if s, err := readStatus(state); err == nil {
		sp = &s
	}
	switch st := driverState(held, dp, sp); {
	case st == driverStarting:
		// Until status.json says running, the runtime's signal handlers may
		// not be installed, and SIGTERM would kill the driver unsummarised.
		return fail(stderr, errors.New("the driver is starting (launch checks); retry once status says running"))
	case st != driverRunning:
		return fail(stderr, fmt.Errorf("no driver is running on %s (driver %s)", state, st))
	case dp == nil:
		return fail(stderr, fmt.Errorf("the run on %s was not launched by this CLI (no %s); signal its process yourself", state, driverFile))
	case !isDriver(dp.PID, dp.State):
		return fail(stderr, fmt.Errorf("pid %d is not this state dir's driver", dp.PID))
	}
	if err := syscall.Kill(dp.PID, syscall.SIGTERM); err != nil {
		return fail(stderr, fmt.Errorf("SIGTERM to driver %d: %w", dp.PID, err))
	}
	fmt.Fprintf(stdout, "SIGTERM sent to driver %d: in-flight calls finish first, which can take as long as the longest call; stop again to cancel them\n", dp.PID)
	return exitOK
}

// cmdResume replays the last launch: its stored args, re-composed
// against the current host config. It serves a run that ended with no
// outcome to act on; one that returned an outcome is relaunched with
// launch -args from the outcome.
func cmdResume(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("resume", stderr)
	var a addr
	a.flags(fs)
	host := fs.String("config", "", "the host config (default "+hostConfigPath()+")")
	plugin := fs.String("plugin-dir", "", "the plugin root (default: the last launch's)")
	if err := parse(fs, args); err != nil {
		return fail(stderr, err)
	}
	state, err := a.stateDir()
	if err != nil {
		return fail(stderr, err)
	}
	r := statusOf(state, fallback{})
	switch r.Driver {
	case driverStarting, driverRunning:
		return fail(stderr, fmt.Errorf("a driver is running on %s; see status", state))
	}
	l, err := readLaunched(state)
	if err != nil {
		return fail(stderr, fmt.Errorf("nothing to resume on %s (%v): use launch", state, err))
	}
	// A crashed driver wrote no summary; summary.json is then an earlier
	// launch's.
	if r.Driver != driverCrashed {
		var sum lsfrt.Summary
		b, err := os.ReadFile(filepath.Join(state, "summary.json"))
		if err == nil && json.Unmarshal(b, &sum) == nil && sum.StopReason == lsfrt.StopReturned {
			var o struct {
				Status  string `json:"status"`
				PlanSha string `json:"planSha"`
			}
			json.Unmarshal(sum.Outcome, &o)
			return fail(stderr, fmt.Errorf("the last launch returned an outcome (status %s): relaunch with launch -args from its planSha %s, passing its unscribed, as phronithm:lsf-drive's Relaunch says", o.Status, o.PlanSha))
		}
	}
	if err := checkGit(); err != nil {
		return fail(stderr, err)
	}
	if *plugin == "" {
		*plugin = l.PluginDir
	}
	return start(launchIn{hostFile: *host, pluginDir: *plugin, repo: l.Repo, stateDir: state, args: l.Args}, stdout, stderr)
}
