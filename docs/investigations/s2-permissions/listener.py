"""Probe listener for the s2-permissions harness.

Serves two ports on 127.0.0.1 (and the exfil port on the LAN address too):
the exfil port logs every request and returns 204, and the echo port returns
the request body. Each request is appended to <log> as one JSON line.

Usage: python3 -I listener.py <log> <exfil-port> <echo-port> [<lan-addr>]
"""
import json
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

LOG = sys.argv[1]
LOCK = threading.Lock()


def handler(kind):
    class H(BaseHTTPRequestHandler):
        def _serve(self):
            n = int(self.headers.get("Content-Length") or 0)
            body = self.rfile.read(n) if n else b""
            with LOCK, open(LOG, "a") as f:
                f.write(json.dumps({"kind": kind, "addr": self.server.server_address[0], "method": self.command,
                                    "path": self.path, "body": body.decode("utf-8", "replace")}) + "\n")
            if kind == "echo":
                self.send_response(200)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
            else:
                self.send_response(204)
                self.end_headers()

        do_GET = do_POST = do_PUT = _serve

        def log_message(self, *a):
            pass
    return H


def serve(addr, port, kind):
    ThreadingHTTPServer((addr, port), handler(kind)).serve_forever()


exfil, echo = int(sys.argv[2]), int(sys.argv[3])
binds = [("127.0.0.1", exfil, "exfil"), ("127.0.0.1", echo, "echo")]
if len(sys.argv) > 4:
    binds.append((sys.argv[4], exfil, "exfil"))
for b in binds[1:]:
    threading.Thread(target=serve, args=b, daemon=True).start()
serve(*binds[0])
