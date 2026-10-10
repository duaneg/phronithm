package testcfg

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func podman(t *testing.T, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(Podman(), args...)
	cmd.Env = append(os.Environ(), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

// TestStubRun checks run's bind substitution, -e overlay, -w, the record
// of its argv and its refusal of an unknown flag.
func TestStubRun(t *testing.T) {
	c := New(t)
	src := t.TempDir()
	script := filepath.Join(src, "show")
	os.WriteFile(script, []byte("#!/bin/sh\necho \"cwd=$(pwd -P) arg=$1 FOO=$FOO KEEP=$KEEP\"\ncat\n"), 0o755)
	work := t.TempDir()
	argv := []string{"run", "--rm", "-i", "--init", "--pull=never", "--log-driver=none", "--name", "c1", "--label", "lsf.state=abc",
		"--network", "pasta:--splice-only,-T,1234", "-v", src + ":/opt/bin:ro", "-v", work + ":" + work + ":rw", "-w", work,
		"-e", "FOO=bar", "-e", "KEEP", "img@sha256:00", "/opt/bin/show", "/opt/bin/x"}
	cmd := exec.Command(Podman(), argv...)
	cmd.Env = append(os.Environ(), "KEEP=kept", "FOO=host")
	cmd.Stdin = strings.NewReader("stdin-line\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	wantWork, _ := filepath.EvalSymlinks(work)
	if want := "cwd=" + wantWork + " arg=" + src + "/x FOO=bar KEEP=kept\nstdin-line\n"; string(out) != want {
		t.Errorf("run output %q, want %q", out, want)
	}
	rec, _ := os.ReadFile(filepath.Join(c.State, "run.1"))
	if got := strings.Split(strings.TrimSuffix(string(rec), "\x00"), "\x00"); strings.Join(got, " ") != strings.Join(argv, " ") {
		t.Errorf("recorded %q", got)
	}
	if _, stderr, code := podman(t, nil, "run", "--name", "c2", "--bogus", "img", "true"); code != 125 || !strings.Contains(stderr, "unknown flag") {
		t.Errorf("unknown flag: exit %d %q", code, stderr)
	}
	if _, _, code := podman(t, []string{"STUB_PODMAN_RUN_EXIT=127"}, "run", "--name", "c3", "img", "true"); code != 127 {
		t.Errorf("STUB_PODMAN_RUN_EXIT: exit %d", code)
	}
	if out, _, code := podman(t, nil, "run", "--name", "s", "--label", "lsf.smoke=1", "-v", Claude()+":/opt/claude/claude:ro", "img", "/bin/sh", "-c", "x"); code != 0 || out != "lsf-smoke-claude 0.0.0-stub (Claude Code)\n" {
		t.Errorf("smoke: exit %d %q", code, out)
	}
}

// TestStubDetached checks that a detached run survives SIGTERM to its
// client and that ps lists it by label until rm -f ends it.
func TestStubDetached(t *testing.T) {
	c := New(t)
	cmd := exec.Command(Podman(), "run", "--name", "d1", "--label", "lsf.state=s1", "img", "sh", "-c", "trap '' TERM; sleep 300")
	cmd.Env = append(os.Environ(), "STUB_PODMAN_DETACH=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { podman(t, nil, "rm", "-f", "--ignore", "d1"); cmd.Wait() })
	rec := filepath.Join(c.State, "name.d1")
	deadline := time.Now().Add(5 * time.Second)
	var b []byte
	for {
		var err error
		if b, err = os.ReadFile(rec); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no record")
		}
		time.Sleep(20 * time.Millisecond)
	}
	inner, err := strconv.Atoi(strings.SplitN(string(b), "\n", 2)[0])
	if err != nil || inner <= 1 {
		t.Fatalf("record %q", b)
	}
	if _, stderr, code := podman(t, nil, "run", "--name", "d1", "img", "true"); code != 125 || !strings.Contains(stderr, "already in use") {
		t.Errorf("a live name reused: exit %d %q", code, stderr)
	}
	// The record is written once setsid is forked, maybe before it has
	// left the client's process group: wait until it leads its session.
	for deadline := time.Now().Add(5 * time.Second); sessionOf(inner) != inner; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the detached command never led its own session")
		}
	}
	syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	time.Sleep(200 * time.Millisecond)
	if syscall.Kill(inner, 0) != nil {
		t.Fatal("the detached command died with its client's SIGTERM")
	}
	if out, _, _ := podman(t, nil, "ps", "-a", "--filter", "label=lsf.state=s1", "--format", "{{.Names}}"); out != "d1\n" {
		t.Errorf("ps by label: %q", out)
	}
	if out, _, _ := podman(t, nil, "ps", "-a", "--filter", "label=lsf.state=other", "--format", "{{.Names}}"); out != "" {
		t.Errorf("ps by another label: %q", out)
	}
	if _, stderr, code := podman(t, nil, "rm", "-f", "-t", "0", "--ignore", "d1", "gone"); code != 0 {
		t.Fatalf("rm: %d %s", code, stderr)
	}
	cmd.Wait()
	if syscall.Kill(inner, 0) == nil {
		t.Error("rm -f left the command running")
	}
	if _, _, code := podman(t, nil, "rm", "-f", "gone"); code != 1 {
		t.Errorf("rm without --ignore of a missing name: exit %d", code)
	}
	if out, _, _ := podman(t, nil, "ps", "-a", "--filter", "label=lsf.state=s1", "--format", "{{.Names}}"); out != "" {
		t.Errorf("ps after rm: %q", out)
	}
}

func TestStubInfo(t *testing.T) {
	New(t)
	if out, _, _ := podman(t, nil, "info", "--format", "{{.Host.Security.Rootless}}"); out != "true\n" {
		t.Errorf("rootless %q", out)
	}
	if out, _, _ := podman(t, []string{"STUB_PODMAN_PASTA="}, "info", "--format", "{{.Host.Pasta.Executable}}"); out != "\n" {
		t.Errorf("pasta unset %q", out)
	}
	if _, _, code := podman(t, []string{"STUB_PODMAN_NOIMAGE=1"}, "image", "exists", Image); code != 1 {
		t.Errorf("image exists with NOIMAGE: %d", code)
	}
}

// sessionOf is a process's session id from /proc/<pid>/stat, or -1.
func sessionOf(pid int) int {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return -1
	}
	f := strings.Fields(string(b[bytes.LastIndexByte(b, ')')+1:])) // state ppid pgrp session ...
	if len(f) < 4 {
		return -1
	}
	n, err := strconv.Atoi(f[3])
	if err != nil {
		return -1
	}
	return n
}
