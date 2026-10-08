"""Credential-injecting proxy for the s2-permissions A+ route.

Runs on the host, OUTSIDE the sandbox. The sandboxed `claude` is given
ANTHROPIC_BASE_URL=http://127.0.0.1:<port> and a placeholder bearer token;
it never holds the real credential. This proxy strips the placeholder,
injects the real OAuth bearer (read once at startup from the file named by
LSF_INJECT_TOKEN_FILE), ensures the OAuth beta header is present, and
forwards to api.anthropic.com, streaming the response back unbuffered.

A request whose bearer is not the expected placeholder is refused, so a
real token is never forwarded from inside the sandbox by mistake.

Usage: LSF_INJECT_TOKEN_FILE=<path> python3 -I inject-proxy.py <port> <placeholder>
"""
import http.client
import ssl
import sys
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(sys.argv[1])
PLACEHOLDER = sys.argv[2]
UPSTREAM = "api.anthropic.com"
with open(os.environ["LSF_INJECT_TOKEN_FILE"]) as f:
    import json
    _d = json.load(f)
    TOKEN = (_d.get("claudeAiOauth", {}) or {}).get("accessToken") or _d.get("accessToken")
assert TOKEN, "no token in LSF_INJECT_TOKEN_FILE"
CTX = ssl.create_default_context()
OAUTH_BETA = "oauth-2025-04-20"
HOP = {"connection", "proxy-connection", "keep-alive", "transfer-encoding", "te", "trailer", "upgrade", "host"}


class H(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def _reject(self, code, msg):
        self.send_response(code)
        self.send_header("Content-Length", str(len(msg)))
        self.end_headers()
        self.wfile.write(msg.encode())

    def _relay(self):
        n = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(n) if n else b""
        # The sandbox must present the placeholder, nothing else.
        auth = self.headers.get("Authorization", "")
        if auth and auth != f"Bearer {PLACEHOLDER}":
            return self._reject(403, "inject-proxy: unexpected bearer from sandbox\n")
        out = {}
        for k, v in self.headers.items():
            if k.lower() in HOP or k.lower() in ("authorization", "x-api-key", "content-length"):
                continue
            out[k] = v
        out["Authorization"] = f"Bearer {TOKEN}"
        beta = out.get("anthropic-beta", "")
        if OAUTH_BETA not in beta:
            out["anthropic-beta"] = (beta + "," if beta else "") + OAUTH_BETA
        conn = http.client.HTTPSConnection(UPSTREAM, 443, context=CTX, timeout=600)
        try:
            conn.request(self.command, self.path, body=body, headers=out)
            resp = conn.getresponse()
            self.send_response(resp.status)
            for k, v in resp.getheaders():
                if k.lower() in HOP or k.lower() == "content-length":
                    continue
                self.send_header(k, v)
            self.send_header("Transfer-Encoding", "chunked")
            self.end_headers()
            while True:
                chunk = resp.read(4096)
                if not chunk:
                    break
                self.wfile.write(b"%X\r\n%s\r\n" % (len(chunk), chunk))
                self.wfile.flush()
            self.wfile.write(b"0\r\n\r\n")
            self.wfile.flush()
        except Exception as e:
            try:
                self._reject(502, f"inject-proxy upstream error: {e}\n")
            except Exception:
                pass
        finally:
            conn.close()

    do_GET = do_POST = do_PUT = do_DELETE = _relay

    def log_message(self, *a):
        pass


ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()
