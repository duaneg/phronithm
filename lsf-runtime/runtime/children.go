package lsfrt

import (
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

// reapOrphans terminates calls a previous launch left running: a SIGKILLed
// launch's children keep running and would race its successor (spike run 4).
// A process whose arguments carry "--session-id <id>" for an id recorded in
// children.json is a call; it leads its own process group, which gets SIGTERM
// and, after KillGrace, SIGKILL. Matching the session id, not the pid, covers
// a child recorded before Start and rules out a reused pid. If ps fails,
// children.json is kept for the next launch. In a dry run, orphans are
// reported, not killed.
func (rt *Runtime) reapOrphans() {
	path := filepath.Join(rt.cfg.StateDir, "children.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var recorded map[string]int
	if err := json.Unmarshal(b, &recorded); err != nil {
		rt.Log("children.json unreadable, treated as empty: " + err.Error())
		rt.event("children-unreadable", map[string]any{"error": err.Error()})
	}
	if len(recorded) > 0 {
		procs, err := processes()
		if err != nil {
			rt.Log("orphan check failed, children.json kept: " + err.Error())
			rt.event("orphan-check-failed", map[string]any{"error": err.Error()})
			return
		}
		for pid, args := range procs {
			for session := range recorded {
				if !strings.Contains(" "+args+" ", " --session-id "+session+" ") {
					continue
				}
				if rt.cfg.DryRun {
					rt.event("orphan-found", map[string]any{"pid": pid, "session": session})
					rt.Log(fmt.Sprintf("dry run: orphaned call pid %d (session %s) left running", pid, session))
					continue
				}
				if err := killGroup(pid, rt.cfg.KillGrace.D()); err != nil {
					rt.Log(fmt.Sprintf("orphaned call pid %d (session %s): %v", pid, session, err))
					continue
				}
				rt.event("orphan-terminated", map[string]any{"pid": pid, "session": session})
				rt.Log(fmt.Sprintf("terminated orphaned call pid %d (session %s) left by a previous launch", pid, session))
			}
		}
	}
	if !rt.cfg.DryRun {
		if err := writeFileAtomic(path, []byte("{}")); err != nil {
			rt.Log("children.json: " + err.Error())
		}
	}
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
	for time.Now().Before(deadline) && syscall.Kill(pid, 0) == nil {
		time.Sleep(50 * time.Millisecond)
	}
	syscall.Kill(target, syscall.SIGKILL)
	return nil
}
