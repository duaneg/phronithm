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
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	lsfrt "github.com/duaneg/phronithm/lsf-runtime/runtime"
)

func init() {
	commands["status"] = command{run: cmdStatus, usage: "status [-state DIR | -repo DIR -name NAME]"}
	commands["watch"] = command{run: cmdWatch, usage: "watch [-state DIR | -repo DIR -name NAME]"}
}

// Driver states.
const (
	driverNone     = "none"
	driverStarting = "starting"
	driverRunning  = "running"
	driverEnded    = "ended"
	driverCrashed  = "crashed"
	driverFailed   = "failed-to-start"
)

// report is status's and watch's output.
type report struct {
	StateDir  string `json:"stateDir"`
	Repo      string `json:"repo"`
	Name      string `json:"name"`
	Driver    string `json:"driver"`
	DriverLog string `json:"driverLog,omitempty"` // driver.log's tail, for crashed and failed-to-start

	PID    int    `json:"pid,omitempty"` // status.json's
	Launch string `json:"launch,omitempty"`
	State  string `json:"state,omitempty"`
	At     string `json:"at,omitempty"`

	Summary *summaryView    `json:"summary,omitempty"` // ended only
	Outcome json.RawMessage `json:"outcome,omitempty"` // ended only
	// Previous is the last launch that ran, when a later one failed to
	// start: its summary and outcome are still the ones to act on.
	Previous *previousView `json:"previous,omitempty"`

	Pause           *pauseView `json:"pause,omitempty"`
	Leftovers       leftovers  `json:"leftovers"`
	XferRefs        []string   `json:"xferRefs"`
	Containers      []string   `json:"containers"`
	ContainersError string     `json:"containersError,omitempty"`
	Errors          []string   `json:"errors,omitempty"` // pieces that could not be read
}

type summaryView struct {
	StopReason string    `json:"stop_reason"`
	Detail     string    `json:"detail,omitempty"`
	Error      string    `json:"error,omitempty"`
	Calls      int       `json:"calls"`
	CostUSD    float64   `json:"cost_usd"`
	Started    time.Time `json:"started"`
	Finished   time.Time `json:"finished"`
}

type previousView struct {
	Launch  string          `json:"launch"`
	Summary *summaryView    `json:"summary"`
	Outcome json.RawMessage `json:"outcome,omitempty"`
}

// readSummary reads summary.json, refusing one that is not launch's:
// Run writes it from the same start time that names the launch, and
// only logs a failed write.
func readSummary(state, launch string) (lsfrt.Summary, error) {
	var sum lsfrt.Summary
	b, err := os.ReadFile(filepath.Join(state, "summary.json"))
	if err == nil {
		err = json.Unmarshal(b, &sum)
	}
	if err != nil {
		return sum, fmt.Errorf("summary.json: %w", err)
	}
	if !strings.HasPrefix(launch, sum.Started.UTC().Format("20060102T150405")+"-") {
		return sum, fmt.Errorf("summary.json is from a launch started %s, not launch %s", sum.Started.UTC().Format(time.RFC3339), launch)
	}
	return sum, nil
}

func viewOf(sum lsfrt.Summary) *summaryView {
	return &summaryView{StopReason: string(sum.StopReason), Detail: sum.Detail, Error: sum.Error, Calls: sum.Calls, CostUSD: sum.CostUSD, Started: sum.Started, Finished: sum.Finished}
}

type pauseView struct {
	Ref string `json:"ref"`
	Set bool   `json:"set"`
}

type leftovers struct {
	Clone   []string `json:"clone"`
	Scratch []string `json:"scratch"`
	Merge   []string `json:"merge"`
	Partial []string `json:"partial"`
}

// statusFile is the runtime's status.json.
type statusFile struct {
	PID    int    `json:"pid"`
	State  string `json:"state"`
	At     string `json:"at"`
	Launch string `json:"launch"`
}

func readStatus(state string) (statusFile, error) {
	var s statusFile
	b, err := os.ReadFile(filepath.Join(state, "status.json"))
	if err == nil {
		err = json.Unmarshal(b, &s)
	}
	return s, err
}

func readDriver(state string) (driverRecord, error) {
	var d driverRecord
	b, err := os.ReadFile(filepath.Join(state, driverFile))
	if err == nil {
		err = json.Unmarshal(b, &d)
	}
	return d, err
}

// launched is what status reads from launch.json.
type launched struct {
	Repo      string          `json:"repo"`
	PodmanBin string          `json:"podmanBin"`
	PluginDir string          `json:"pluginDir"`
	Args      json.RawMessage `json:"args"`
}

func readLaunched(state string) (launched, error) {
	var l launched
	b, err := os.ReadFile(filepath.Join(state, launchFile))
	if err == nil {
		err = json.Unmarshal(b, &l)
	}
	return l, err
}

// lockHeld probes the state dir's lock: a non-blocking shared flock fails
// while a driver holds it exclusively. A missing lock file is free.
func lockHeld(state string) (bool, error) {
	f, err := os.Open(filepath.Join(state, "lock"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return true, nil
		}
		return false, fmt.Errorf("lock: %w", err)
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false, nil
}

// isDriver reports whether pid is a live driver on state: its command line
// is drive -state <state>.
func isDriver(pid int, state string) bool {
	if pid <= 0 {
		return false
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	a := strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
	return len(a) >= 4 && a[1] == "drive" && a[2] == "-state" && a[3] == state
}

// driverState applies the status table (lsf-runtime.md, CLI → Status).
func driverState(held bool, d *driverRecord, s *statusFile) string {
	switch {
	case held && d == nil: // a launch not made by this CLI
		if s != nil && s.State == "running" {
			return driverRunning
		}
		return driverStarting
	case d == nil:
		return driverNone
	case held && s != nil && s.PID != d.PID && s.State == "running" && isDriver(s.PID, d.State):
		// An earlier launch's driver won the lock after a later launch,
		// whose driver was then refused, replaced driver.json.
		return driverRunning
	case held && (s == nil || s.PID != d.PID):
		return driverStarting
	case held:
		return driverRunning
	case s != nil && s.PID == d.PID && s.State == "running":
		return driverCrashed
	case s != nil && s.PID == d.PID:
		return driverEnded
	case isDriver(d.PID, d.State): // spawned, the lock not yet taken
		return driverStarting
	default:
		return driverFailed
	}
}

// fallback names the run when it has no launch.json: the address's repo
// and name.
type fallback struct{ repo, name string }

// statusOf builds the report. It never fails: a piece it cannot read is
// left out and named in errors.
func statusOf(state string, fb fallback) report {
	r := report{StateDir: state, Repo: fb.repo, Name: fb.name, XferRefs: []string{}, Containers: []string{}}
	r.Leftovers = leftovers{Clone: []string{}, Scratch: []string{}, Merge: []string{}, Partial: []string{}}
	note := func(err error) {
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			r.Errors = append(r.Errors, err.Error())
		}
	}
	held, err := lockHeld(state)
	note(err)
	var dp *driverRecord
	if d, err := readDriver(state); err == nil {
		dp = &d
	} else {
		note(err)
	}
	var sp *statusFile
	if s, err := readStatus(state); err == nil {
		sp = &s
		r.PID, r.Launch, r.State, r.At = s.PID, s.Launch, s.State, s.At
	} else {
		note(err)
	}
	r.Driver = driverState(held, dp, sp)
	if r.Driver == driverCrashed { // the lock was probed before status.json was read
		if held, _ := lockHeld(state); held {
			r.Driver = driverRunning
		}
	}
	switch r.Driver {
	case driverEnded:
		if sum, err := readSummary(state, sp.Launch); err == nil {
			r.Summary, r.Outcome = viewOf(sum), sum.Outcome
		} else {
			r.Errors = append(r.Errors, err.Error())
		}
	case driverCrashed, driverFailed:
		r.DriverLog = tail(filepath.Join(state, driverLog), 4096)
		// status.json and summary.json are still the last launch that ran.
		if r.Driver == driverFailed && sp != nil && sp.State != "running" {
			if sum, err := readSummary(state, sp.Launch); err == nil && string(sum.StopReason) == sp.State {
				r.Previous = &previousView{Launch: sp.Launch, Summary: viewOf(sum), Outcome: sum.Outcome}
			}
		}
	}

	podman, pauseRef := "podman", ""
	if l, err := readLaunched(state); err == nil {
		r.Repo = l.Repo
		if l.PodmanBin != "" {
			podman = l.PodmanBin
		}
		var a struct {
			Name     string `json:"name"`
			PauseRef string `json:"pauseRef"`
		}
		json.Unmarshal(l.Args, &a)
		r.Name, pauseRef = a.Name, a.PauseRef
	} else {
		note(err)
	}
	if pauseRef == "" && r.Name != "" {
		pauseRef = "refs/lsf/" + r.Name + "/pause"
	}
	if r.Repo != "" && pauseRef != "" {
		set, err := refExists(r.Repo, pauseRef)
		note(err)
		r.Pause = &pauseView{Ref: pauseRef, Set: set}
	}

	r.Leftovers.Clone = entries(filepath.Join(state, "clone"))
	r.Leftovers.Scratch = entries(filepath.Join(state, "scratch"))
	r.Leftovers.Merge = entries(filepath.Join(state, "merge"))
	for _, d := range []string{"clone", "scratch", "merge", "gates"} {
		m, _ := filepath.Glob(filepath.Join(state, d, "*.partial"))
		for _, p := range m {
			rel, _ := filepath.Rel(state, p)
			r.Leftovers.Partial = append(r.Leftovers.Partial, rel)
		}
	}
	if r.Repo != "" {
		refs, err := xferRefs(r.Repo)
		note(err)
		r.XferRefs = append(r.XferRefs, refs...)
	}
	if _, err := os.Stat(state); err == nil {
		names, err := containers(podman, state)
		if err != nil {
			r.ContainersError = err.Error()
		}
		r.Containers = append(r.Containers, names...)
	}
	return r
}

// entries are a directory's entry names; none when it is missing.
func entries(dir string) []string {
	out := []string{}
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

// tail is a file's last n bytes.
func tail(path string, n int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	off := max(fi.Size()-n, 0)
	b, _ := io.ReadAll(io.NewSectionReader(f, off, n))
	return string(b)
}

// toolTimeout bounds each git and podman command status runs.
const toolTimeout = 10 * time.Second

// output runs a git or podman command under toolTimeout.
func output(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	var ee *exec.ExitError
	switch {
	case ctx.Err() != nil:
		err = fmt.Errorf("%s %s: timed out after %v", name, args[0], toolTimeout)
	case errors.As(err, &ee) && len(bytes.TrimSpace(ee.Stderr)) > 0:
		err = &toolError{ee, strings.TrimSpace(string(ee.Stderr))}
	}
	return out, err
}

// toolError is a command's exit with its stderr; it unwraps to the
// *exec.ExitError.
type toolError struct {
	*exec.ExitError
	stderr string
}

func (e *toolError) Error() string { return e.ExitError.Error() + ": " + e.stderr }
func (e *toolError) Unwrap() error { return e.ExitError }

func refExists(repo, ref string) (bool, error) {
	_, err := output("git", "-C", repo, "show-ref", "--verify", "--quiet", ref)
	var ee *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &ee) && ee.ExitCode() == 1:
		return false, nil
	default:
		return false, fmt.Errorf("git show-ref %s: %v", ref, err)
	}
}

var xferRef = regexp.MustCompile(`^refs/lsf/[^/]+/xfer/w[0-9]+$`)

// xferRefs are the merge transfer refs left in the main repo.
func xferRefs(repo string) ([]string, error) {
	out, err := output("git", "-C", repo, "for-each-ref", "--format=%(refname)", "refs/lsf/")
	if err != nil {
		return nil, fmt.Errorf("git for-each-ref: %v", err)
	}
	var refs []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if xferRef.MatchString(l) {
			refs = append(refs, l)
		}
	}
	return refs, nil
}

// containers are the containers carrying the state dir's label.
func containers(podman, state string) ([]string, error) {
	label, err := lsfrt.StateLabel(state)
	if err != nil {
		return nil, err
	}
	out, err := output(podman, "ps", "-a", "--filter", "label=lsf.state="+label, "--format", "{{.Names}}")
	if err != nil {
		return nil, fmt.Errorf("podman ps: %v", err)
	}
	return strings.Fields(string(out)), nil
}

// exitFor is status's and watch's exit code for a driver state.
func exitFor(driver string) int {
	switch driver {
	case driverCrashed, driverFailed:
		return exitCrashed
	case driverNone:
		return exitNoLaunch
	}
	return exitOK
}

// addressed parses an address-only command line: the state dir and the
// fallback naming the run.
func addressed(name string, args []string, stderr io.Writer) (string, fallback, error) {
	fs := newFlags(name, stderr)
	var a addr
	a.flags(fs)
	if err := parse(fs, args); err != nil {
		return "", fallback{}, err
	}
	state, err := a.stateDir()
	if err != nil {
		return "", fallback{}, err
	}
	fb := fallback{name: a.name}
	if a.repo != "" || a.state == "" {
		fb.repo, _ = repoTop(a.repo)
	}
	return state, fb, nil
}

func printReport(stdout io.Writer, r report) int {
	b, _ := json.MarshalIndent(r, "", "  ")
	fmt.Fprintf(stdout, "%s\n", b)
	return exitFor(r.Driver)
}

func cmdStatus(args []string, stdout, stderr io.Writer) int {
	state, fb, err := addressed("status", args, stderr)
	if err != nil {
		return fail(stderr, err)
	}
	return printReport(stdout, statusOf(state, fb))
}

// cmdWatch blocks until no driver holds the lock, then prints the report.
func cmdWatch(args []string, stdout, stderr io.Writer) int {
	state, fb, err := addressed("watch", args, stderr)
	if err != nil {
		return fail(stderr, err)
	}
	for {
		if err := waitUnlocked(state); err != nil {
			return fail(stderr, err)
		}
		r := statusOf(state, fb)
		if r.Driver != driverStarting && r.Driver != driverRunning {
			return printReport(stdout, r)
		}
		time.Sleep(launchPoll) // a driver not yet holding the lock
	}
}

// waitUnlocked blocks until the lock is free: a blocking shared flock,
// granted when the driver's exclusive one is released at its exit.
func waitUnlocked(state string) error {
	f, err := os.Open(filepath.Join(state, "lock"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EINTR) {
			return fmt.Errorf("lock: %w", err)
		}
	}
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
