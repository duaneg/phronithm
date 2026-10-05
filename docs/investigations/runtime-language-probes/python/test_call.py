import os, time, tempfile, unittest
from call import call, parallel
STUB = os.path.join(os.path.dirname(__file__), "..", "claude")

def alive(pid):
    try: os.kill(pid, 0); return True
    except ProcessLookupError: return False

class T(unittest.TestCase):
    def test_ok(self):
        o = call(STUB, {"STUB_MODE": "ok"}, 10)
        self.assertEqual((o.kind, o.output["n"]), ("ok", 3))
    def test_null(self):
        self.assertEqual(call(STUB, {"STUB_MODE": "nullout"}, 10).kind, "null")
    def test_hang_killed(self):
        f = tempfile.NamedTemporaryFile(delete=False).name
        t = time.monotonic()
        o = call(STUB, {"STUB_MODE": "hang", "STUB_PIDFILE": f}, 1)
        self.assertEqual(o.kind, "timeout"); self.assertLess(time.monotonic() - t, 3)
        time.sleep(0.1); self.assertFalse(alive(int(open(f).read())))
    def test_parallel(self):
        self.assertTrue(all(o.kind == "ok" for o in parallel(STUB, 8)))
unittest.main()
