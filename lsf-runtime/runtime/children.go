package lsfrt

import (
	"encoding/json"
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
// Each process whose arguments carry a session id recorded in children.json
// gets SIGTERM to its process group, then SIGKILL after KillGrace. Matching on
// the session id, not the pid, covers a child recorded before Start and rules
// out a reused pid. In a dry run, orphans are reported, not killed.
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
		for pid, args := range processes() {
			for session := range recorded {
				if !strings.Contains(args, session) {
					continue
				}
				if rt.cfg.DryRun {
					rt.event("orphan-found", map[string]any{"pid": pid, "session": session})
					rt.Log("dry run: orphaned call pid " + strconv.Itoa(pid) + " (session " + session + ") left running")
					continue
				}
				killGroup(pid, rt.cfg.KillGrace.D())
				rt.event("orphan-terminated", map[string]any{"pid": pid, "session": session})
				rt.Log("terminated orphaned call pid " + strconv.Itoa(pid) + " (session " + session + ") left by a previous launch")
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
// Linux and macOS share (macOS has no /proc).
func processes() map[int]string {
	out, err := exec.Command("ps", "-A", "-o", "pid=,args=").Output()
	if err != nil {
		return nil
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
	return m
}

// killGroup sends SIGTERM to pid's group (a call is its group's leader), waits
// up to grace for the leader to exit, then sends SIGKILL to the group.
func killGroup(pid int, grace time.Duration) {
	syscall.Kill(-pid, syscall.SIGTERM)
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) && syscall.Kill(pid, 0) == nil {
		time.Sleep(50 * time.Millisecond)
	}
	syscall.Kill(-pid, syscall.SIGKILL)
}
