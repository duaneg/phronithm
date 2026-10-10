package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/duaneg/phronithm/lsf-runtime/orchestrator"
	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

func init() {
	commands["launch"] = command{run: cmdLaunch, usage: "launch -args FILE|- [-state DIR | -repo DIR] [-name NAME] [-config FILE] [-plugin-dir DIR] [-dry-run]"}
}

// Bounds on launch's wait for the driver's launch checks.
var (
	launchWait = 5 * time.Minute
	launchPoll = 200 * time.Millisecond
)

// lockRetries bounds the starts of a driver refused because a status or
// watch probe held the lock.
const lockRetries = 4

func cmdLaunch(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("launch", stderr)
	var a addr
	a.flags(fs)
	argsPath := fs.String("args", "", "the orchestrator's args, a JSON object (- for stdin)")
	host := fs.String("config", "", "the host config (default "+hostConfigPath()+")")
	plugin := fs.String("plugin-dir", "", "the plugin root (beats the host config's pluginDir)")
	dry := fs.Bool("dry-run", false, "run the launch checks and stop at the first call, in a scratch state dir")
	if err := parse(fs, args); err != nil {
		return fail(stderr, err)
	}
	if *argsPath == "" {
		return fail(stderr, usageError{error: errors.New("launch: -args is required")})
	}
	raw, err := readArgs(*argsPath)
	if err != nil {
		return fail(stderr, err)
	}
	name, err := argsName(raw, a.name)
	if err != nil {
		return fail(stderr, err)
	}
	if err := checkGit(); err != nil {
		return fail(stderr, err)
	}
	repo, err := repoTop(a.repo)
	if err != nil {
		return fail(stderr, err)
	}
	var state string
	if a.state != "" {
		if state, err = filepath.Abs(a.state); err == nil {
			err = (&addr{name: name, repo: a.repo}).agrees(state)
		}
	} else {
		state, err = defaultStateDir(repo, name)
	}
	if err != nil {
		return fail(stderr, err)
	}
	in := launchIn{hostFile: *host, pluginDir: *plugin, repo: repo, stateDir: state, args: raw}
	if *dry {
		return dryRun(in, stdout, stderr)
	}
	return start(in, stdout, stderr)
}

// readArgs reads the args file, or stdin for "-", compacted.
func readArgs(path string) (json.RawMessage, error) {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("args: %w", err)
	}
	var out bytes.Buffer
	if err := json.Compact(&out, b); err != nil {
		return nil, fmt.Errorf("args: %w", err)
	}
	return out.Bytes(), nil
}

// driverRecord is driver.json: the current launch's driver.
type driverRecord struct {
	PID        int       `json:"pid"`
	Started    time.Time `json:"started"`
	State      string    `json:"state"`      // the -state the driver was given
	LaunchFile string    `json:"launchFile"` // its composed config, renamed to launch.json once it holds the lock
}

// launchLock takes <state>/launch.lock, held by a launch until it
// returns and by a dry run until it ends.
func launchLock(state string) (*os.File, error) {
	ll, err := os.OpenFile(filepath.Join(state, "launch.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(ll.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		ll.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("another launch or dry run is in progress on %s; see status", state)
		}
		return nil, fmt.Errorf("launch.lock: %w", err)
	}
	return ll, nil
}

// start composes the config, starts the detached driver and waits until
// it has passed or failed the launch checks.
func start(in launchIn, stdout, stderr io.Writer) int {
	if err := os.MkdirAll(in.stateDir, 0o700); err != nil {
		return fail(stderr, fmt.Errorf("state dir: %w", err))
	}
	// One launch at a time: two that both passed the probe would each
	// record their driver in driver.json.
	ll, err := launchLock(in.stateDir)
	if err != nil {
		return fail(stderr, err)
	}
	defer ll.Close()
	if held, err := lockHeld(in.stateDir); err != nil {
		return fail(stderr, err)
	} else if held {
		return fail(stderr, fmt.Errorf("a driver is running on %s; see status", in.stateDir))
	}
	in.out = filepath.Join(in.stateDir, fmt.Sprintf("launch.%d.json", os.Getpid()))
	if _, err := compose(in); err != nil {
		return fail(stderr, err)
	}
	keep := false
	defer func() {
		if !keep {
			os.Remove(in.out)
		}
	}()
	prev, _ := os.ReadFile(filepath.Join(in.stateDir, driverFile))
	for attempt := 1; ; attempt++ {
		r, err := spawn(in.stateDir, in.out)
		if err != nil {
			return fail(stderr, err)
		}
		switch {
		case r.started:
			keep = true
			if r.left != "" {
				fmt.Fprintln(stderr, "lsf-runtime: warning:", r.left)
			}
			b, _ := json.Marshal(map[string]any{"stateDir": in.stateDir, "pid": r.pid, "launch": r.launch})
			fmt.Fprintf(stdout, "%s\n", b)
			return exitOK
		case r.left != "":
			keep = true
			fmt.Fprintln(stderr, "lsf-runtime:", r.left)
			return exitError
		}
		// The driver refused to start. A status or watch can hold the lock
		// for an instant; retry that a few times.
		if attempt < lockRetries && strings.Contains(r.log, "is in use by another launch") {
			if held, _ := lockHeld(in.stateDir); !held {
				time.Sleep(time.Duration(attempt) * launchPoll)
				continue
			}
		}
		fmt.Fprintf(stderr, "lsf-runtime: the driver refused to start (%s):\n%s", r.exit, r.log)
		restoreDriver(in.stateDir, prev, stderr)
		return exitError
	}
}

// restoreDriver puts back the driver.json a refused launch replaced, when
// the driver it named won the lock after this launch's probe and is
// running: status and stop then still find it.
func restoreDriver(state string, prev []byte, stderr io.Writer) {
	var d driverRecord
	if json.Unmarshal(prev, &d) != nil {
		return
	}
	s, err := readStatus(state)
	if held, _ := lockHeld(state); !held || err != nil || s.PID != d.PID || !isDriver(d.PID, d.State) {
		return
	}
	if writeAtomic(filepath.Join(state, driverFile), prev) == nil {
		fmt.Fprintf(stderr, "lsf-runtime: driver %d, an earlier launch's, is running on %s; see status\n", d.PID, state)
	}
}

// spawned is one driver start's result: started, left running (left says
// why launch stopped waiting) or refused (log holds its output, exit how
// it ended).
type spawned struct {
	started bool
	pid     int
	launch  string
	left    string
	log     string
	exit    string
}

// spawn starts the driver in a session of its own, records it in
// driver.json and waits for it to pass or fail the launch checks.
func spawn(state, file string) (spawned, error) {
	logf, err := os.OpenFile(filepath.Join(state, driverLog), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return spawned{}, err
	}
	defer logf.Close()
	fi, err := logf.Stat()
	if err != nil {
		return spawned{}, err
	}
	offset := fi.Size()
	exe, err := os.Executable()
	if err != nil {
		return spawned{}, err
	}
	cmd := exec.Command(exe, "drive", "-state", state, "-launch", file)
	cmd.Stdout, cmd.Stderr = logf, logf // stdin is /dev/null
	cmd.Env = driverEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Before the start, so an interrupt cannot leave the driver unrecorded.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	if err := cmd.Start(); err != nil {
		return spawned{}, fmt.Errorf("driver: %w", err)
	}
	pid := cmd.Process.Pid
	exited := make(chan struct{})
	var exit string
	go func() {
		if err := cmd.Wait(); err != nil {
			exit = err.Error()
		} else {
			exit = "exit status 0"
		}
		close(exited)
	}()
	rec, _ := json.Marshal(driverRecord{PID: pid, Started: time.Now().UTC(), State: state, LaunchFile: file})
	if err := writeAtomic(filepath.Join(state, driverFile), append(rec, '\n')); err != nil {
		return spawned{pid: pid, left: fmt.Sprintf("driver %d left running, but %s: %v", pid, driverFile, err)}, nil
	}

	// Started: wait, briefly, for the body's rename of the composed file
	// to launch.json, so a status right after launch reads this launch's.
	startedAt := func(l string) spawned {
		r := spawned{started: true, pid: pid, launch: l}
		end := time.After(5 * time.Second)
		for {
			if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
				return r
			}
			select {
			case <-time.After(50 * time.Millisecond):
				continue
			case <-sig:
			case <-end:
			}
			break
		}
		r.left = fmt.Sprintf("%s not yet renamed to %s; status may report the previous launch's repo and name until it is", filepath.Base(file), launchFile)
		return r
	}
	tick := time.NewTicker(launchPoll)
	defer tick.Stop()
	deadline := time.After(launchWait)
	ours := func() (string, bool) {
		s, err := readStatus(state)
		return s.Launch, err == nil && s.PID == pid
	}
	for {
		select {
		case <-tick.C:
			if l, ok := ours(); ok {
				return startedAt(l), nil
			}
		case <-exited:
			// A run can start and end within one poll.
			if l, ok := ours(); ok {
				return spawned{started: true, pid: pid, launch: l}, nil
			}
			return spawned{pid: pid, log: logSince(filepath.Join(state, driverLog), offset), exit: exit}, nil
		case <-deadline:
			return spawned{pid: pid, left: fmt.Sprintf("driver %d still starting after %v; see status", pid, launchWait)}, nil
		case <-sig:
			return spawned{pid: pid, left: fmt.Sprintf("interrupted; driver %d left running; see status", pid)}, nil
		}
	}
}

// driverEnv is the driver's environment: launch's, without
// LSF_CLAUDE_BIN (claudeBin is resolved in the composed config), and with
// DISABLE_AUTOUPDATER=1.
func driverEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "LSF_CLAUDE_BIN=") && !strings.HasPrefix(e, "DISABLE_AUTOUPDATER=") {
			env = append(env, e)
		}
	}
	return append(env, "DISABLE_AUTOUPDATER=1")
}

// logSince is a file's content from offset.
func logSince(path string, offset int64) string {
	f, err := os.Open(path)
	if err != nil {
		return err.Error() + "\n"
	}
	defer f.Close()
	b, _ := io.ReadAll(io.NewSectionReader(f, offset, 1<<20))
	return string(b)
}

// dryRun runs the launch in the foreground with dryRun set, in the
// scratch dir <state>/dry-run, so the run's own files stay as they were.
// It holds the run's launch.lock throughout. It exits 0 only when the
// run stopped with dry-run.
func dryRun(in launchIn, stdout, stderr io.Writer) int {
	if err := os.MkdirAll(in.stateDir, 0o700); err != nil {
		return fail(stderr, fmt.Errorf("state dir: %w", err))
	}
	ll, err := launchLock(in.stateDir)
	if err != nil {
		return fail(stderr, err)
	}
	defer ll.Close()
	dir := filepath.Join(in.stateDir, "dry-run")
	if err := os.RemoveAll(dir); err != nil {
		return fail(stderr, err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fail(stderr, err)
	}
	in.stateDir, in.dryRun, in.out = dir, true, filepath.Join(dir, launchFile)
	cfg, err := compose(in)
	if err != nil {
		return fail(stderr, err)
	}
	os.Unsetenv("LSF_CLAUDE_BIN")
	os.Setenv("DISABLE_AUTOUPDATER", "1")
	sum, err := lsfrt.Run(context.Background(), cfg, orchestrator.Body)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	b, _ := json.MarshalIndent(sum, "", "  ")
	fmt.Fprintf(stdout, "%s\n", b)
	if sum.StopReason != lsfrt.StopDryRun {
		fmt.Fprintf(stderr, "lsf-runtime: dry run ended %s, not dry-run\n", sum.StopReason)
		return exitError
	}
	return exitOK
}

// writeAtomic writes a file by temp file and rename.
func writeAtomic(path string, b []byte) error {
	tmp := fmt.Sprintf("%s.tmp.%d", path, os.Getpid())
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
