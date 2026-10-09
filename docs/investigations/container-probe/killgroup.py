"""Kill a command as lsf-runtime/runtime/call.go kills a call.

The command leads its own process group (Setpgid). After <delay> seconds, or
when <file> gains a line matching <pattern>: SIGTERM to the group, SIGKILL to
the leader if it is alive after <grace> seconds, then SIGKILL to the group.
stdin, stdout and stderr are inherited.

Usage: python3 -I killgroup.py <delay|file:pattern> <grace> -- <command...>
"""
import os, signal, subprocess, sys, time

when, grace = sys.argv[1], float(sys.argv[2])
cmd = sys.argv[sys.argv.index("--") + 1:]
p = subprocess.Popen(cmd, process_group=0)
t0 = time.time()
if ":" in when:
    path, pat = when.split(":", 1)
    while p.poll() is None:
        try:
            if pat in open(path).read():
                break
        except FileNotFoundError:
            pass
        time.sleep(0.5)
else:
    time.sleep(float(when))
t1 = time.time()
print(f"killgroup: trigger after {t1 - t0:.1f} s", file=sys.stderr)
try:
    os.killpg(p.pid, signal.SIGTERM)
except ProcessLookupError:
    pass
try:
    p.wait(grace)
    how = "exited within grace"
except subprocess.TimeoutExpired:
    p.kill()
    p.wait()
    how = "SIGKILL to leader after grace"
try:
    os.killpg(p.pid, signal.SIGKILL)
except ProcessLookupError:
    pass
print(f"killgroup: {how}, rc {p.returncode}, {time.time() - t1:.1f} s after SIGTERM", file=sys.stderr)
