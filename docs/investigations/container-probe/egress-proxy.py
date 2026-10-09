"""Allow-list egress proxy for the container-probe harness.

Runs on the host. A container reaches it through pasta's loopback splice and
is given HTTPS_PROXY/HTTP_PROXY pointing at it. CONNECT is allowed to port 443
of an allowed host only; plain HTTP is refused. Every request is appended to
<log> as one JSON line with its verdict.

Usage: python3 -I egress-proxy.py <port> <log> <host>[,<host>...]
"""
import json
import select
import socket
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(sys.argv[1])
LOG = sys.argv[2]
ALLOW = {h.strip().lower() for h in sys.argv[3].split(",") if h.strip()}
LOCK = threading.Lock()


def log(**kv):
    with LOCK, open(LOG, "a") as f:
        f.write(json.dumps(kv) + "\n")


class H(BaseHTTPRequestHandler):
    def _refuse(self, code, target):
        log(method=self.command, target=target, verdict="refused")
        self.send_response(code)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def do_CONNECT(self):
        host, _, port = self.path.rpartition(":")
        host = host.strip("[]").lower()
        if host not in ALLOW or port != "443":
            return self._refuse(403, self.path)
        try:
            up = socket.create_connection((host, 443), timeout=30)
        except OSError:
            return self._refuse(502, self.path)
        log(method="CONNECT", target=self.path, verdict="allowed")
        self.send_response(200)
        self.end_headers()
        c = self.connection
        socks = [c, up]
        while True:
            r, _, _ = select.select(socks, [], [], 120)
            if not r:
                break
            done = False
            for s in r:
                data = s.recv(65536)
                if not data:
                    done = True
                    break
                (up if s is c else c).sendall(data)
            if done:
                break
        up.close()

    def _plain(self):
        self._refuse(403, self.path)

    do_GET = do_POST = do_PUT = do_HEAD = do_DELETE = _plain

    def log_message(self, *a):
        pass


ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()
