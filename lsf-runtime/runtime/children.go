package lsfrt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// reapOrphans removes what a previous launch left running: a SIGKILLed
// launch's calls keep running and would race its successor (spike run 4).
// A call is a container carrying this state dir's label; each is removed
// with podman rm -f. A failed podman ps, or a container not removed, is
// the launch's error: a live container may still write a clone this launch
// would read. A gate process (RunGates) is recorded in children.json
// under its marker, the "lsf-gate:" argument its shell carries as $0, and
// matched by it, not by pid, which covers one recorded before Start and
// rules out a reused pid; its group gets SIGTERM and, after KillGrace,
// SIGKILL. If ps fails, children.json is kept for the next launch. Another
// entry there is a call recorded by an earlier runtime, logged and dropped.
// In a dry run, orphans are reported, not removed.
func (rt *Runtime) reapOrphans() error {
	if err := rt.reapContainers(); err != nil {
		return err
	}
	path := filepath.Join(rt.cfg.StateDir, "children.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var recorded map[string]int
	if err := json.Unmarshal(b, &recorded); err != nil {
		rt.Log("children.json unreadable, treated as empty: " + err.Error())
		rt.event("children-unreadable", map[string]any{"error": err.Error()})
	}
	for marker, pid := range recorded {
		if !strings.HasPrefix(marker, gateMarkerPrefix) {
			rt.Log(fmt.Sprintf("children.json: %s (pid %d) is not a gate marker; dropped, not killed", marker, pid))
			rt.event("children-dropped", map[string]any{"entry": marker, "pid": pid})
			delete(recorded, marker)
		}
	}
	if len(recorded) > 0 {
		procs, err := processes()
		if err != nil {
			rt.Log("orphan check failed; the recorded gates are carried forward for the next launch: " + err.Error())
			rt.event("orphan-check-failed", map[string]any{"error": err.Error()})
			rt.carryForward(recorded)
			return nil
		}
		for pid, args := range procs {
			for marker := range recorded {
				if !matchesChild(args, marker) {
					continue
				}
				if rt.cfg.DryRun {
					rt.event("orphan-found", map[string]any{"pid": pid, "gate": marker})
					rt.Log(fmt.Sprintf("dry run: orphaned gate pid %d (%s) left running", pid, marker))
					continue
				}
				if err := killGroup(pid, rt.cfg.KillGrace.D()); err != nil {
					rt.Log(fmt.Sprintf("orphaned gate pid %d (%s), carried forward: %v", pid, marker, err))
					rt.carryForward(map[string]int{marker: pid})
					continue
				}
				rt.event("orphan-terminated", map[string]any{"pid": pid, "gate": marker})
				rt.Log(fmt.Sprintf("terminated orphaned gate pid %d (%s) left by a previous launch", pid, marker))
			}
		}
	}
	if !rt.cfg.DryRun {
		rt.childMu.Lock()
		rt.saveChildrenLocked()
		rt.childMu.Unlock()
	}
	return nil
}

// reapContainers removes the containers carrying this state dir's label.
func (rt *Runtime) reapContainers() error {
	ctx, cancel := context.WithTimeout(context.Background(), rmTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, rt.cfg.PodmanBin, "ps", "-a", "--filter", "label="+labelState+"="+rt.stateHash, "--format", "{{.Names}}").Output()
	if err != nil {
		rt.event("orphan-check-failed", map[string]any{"error": err.Error()})
		return fmt.Errorf("orphan check: podman ps: %w%s", err, stderrOf(err))
	}
	for _, name := range strings.Fields(string(out)) {
		if rt.cfg.DryRun {
			rt.event("orphan-found", map[string]any{"container": name})
			rt.Log("dry run: orphaned container " + name + " left running")
			continue
		}
		if err := rt.removeContainer(name); err != nil {
			return fmt.Errorf("orphaned container %s not removed: %w", name, err)
		}
		rt.event("orphan-terminated", map[string]any{"container": name})
		rt.Log("removed orphaned container " + name + " left by a previous launch")
	}
	return nil
}

// matchesChild reports whether a process's arguments carry a gate's marker.
func matchesChild(args, marker string) bool {
	return strings.Contains(" "+args+" ", " "+marker+" ")
}

// carryForward keeps recorded gates in this launch's children.json, which
// every spawn rewrites, so a later launch can still reap them.
func (rt *Runtime) carryForward(recorded map[string]int) {
	rt.childMu.Lock()
	defer rt.childMu.Unlock()
	for s, p := range recorded {
		rt.children[s] = p
	}
	rt.saveChildrenLocked()
}

// processes lists live processes as pid → arguments, through POSIX ps, which
// Linux and macOS share (macOS has no /proc). -ww stops ps truncating long
// argument lists.
func processes() (map[int]string, error) {
	out, err := exec.Command("ps", "-A", "-ww", "-o", "pid=,args=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	m := map[int]string{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		if pid, err := strconv.Atoi(f[0]); err == nil && pid != os.Getpid() {
			m[pid] = strings.Join(f[1:], " ")
		}
	}
	return m, nil
}

// killGroup terminates pid and, when it leads its group (a call does), the
// group: SIGTERM, up to grace for pid to exit, then SIGKILL.
func killGroup(pid int, grace time.Duration) error {
	target := pid
	if pgid, err := syscall.Getpgid(pid); err != nil {
		return err
	} else if pgid == pid {
		target = -pid
	}
	if err := syscall.Kill(target, syscall.SIGTERM); err != nil {
		return err
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) && running(pid) {
		time.Sleep(50 * time.Millisecond)
	}
	syscall.Kill(target, syscall.SIGKILL)
	return nil
}

// running reports whether pid exists and is not a zombie: an orphan's parent
// may be an init that does not reap.
func running(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return err != nil || !strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
}

// children.json holds the markers of running gate commands (pid 0 before
// Start), so a later launch can terminate gates a killed launch left
// running. Calls are found by their containers' label.

func (rt *Runtime) addChild(marker string) {
	rt.childMu.Lock()
	defer rt.childMu.Unlock()
	rt.children[marker] = 0
	rt.saveChildrenLocked()
}

func (rt *Runtime) setChildPid(marker string, pid int) {
	rt.childMu.Lock()
	defer rt.childMu.Unlock()
	rt.children[marker] = pid
	rt.saveChildrenLocked()
}

func (rt *Runtime) removeChild(marker string) {
	rt.childMu.Lock()
	defer rt.childMu.Unlock()
	delete(rt.children, marker)
	rt.saveChildrenLocked()
}

func (rt *Runtime) saveChildrenLocked() {
	b, _ := json.Marshal(rt.children)
	if err := writeFileAtomic(filepath.Join(rt.cfg.StateDir, "children.json"), b); err != nil {
		rt.Log("children.json: " + err.Error())
	}
}
