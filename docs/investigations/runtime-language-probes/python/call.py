import json, os, signal, subprocess, time
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass

@dataclass
class Outcome:
    kind: str            # "ok" | "null" | "timeout"
    output: dict | None

def call(binary: str, env: dict, timeout: float) -> Outcome:
    p = subprocess.Popen([binary, "-p", "x", "--output-format", "stream-json"],
                         stdout=subprocess.PIPE, text=True, env={**os.environ, **env},
                         start_new_session=True)
    try:
        out, _ = p.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        os.killpg(p.pid, signal.SIGKILL)
        p.wait()
        return Outcome("timeout", None)
    for line in out.splitlines():
        ev = json.loads(line)
        if ev.get("type") == "result":
            so = ev.get("structured_output")
            return Outcome("ok", so) if isinstance(so, dict) else Outcome("null", None)
    return Outcome("null", None)

def parallel(binary, n):
    with ThreadPoolExecutor(n) as ex:
        return list(ex.map(lambda _: call(binary, {"STUB_MODE": "ok"}, 10), range(n)))
