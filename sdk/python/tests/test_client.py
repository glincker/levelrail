import json
import os
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from levelrail import APIError, Client, read_sse  # noqa: E402

SECRET = "s3cret-value"


class Handler(BaseHTTPRequestHandler):
    seen = []

    def log_message(self, *args):
        pass

    def _send(self, code, body, headers=None, ctype="application/json"):
        raw = body if isinstance(body, bytes) else json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(raw)))
        for k, v in (headers or {}).items():
            self.send_header(k, v)
        self.end_headers()
        self.wfile.write(raw)

    def _handle(self):
        n = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(n).decode() if n else ""
        Handler.seen.append((self.command, self.path, self.headers.get("Authorization"), body))
        path = self.path.split("?")[0]
        if path == "/api/v1/apps" and self.command == "GET":
            if "cursor=c2" in self.path:
                return self._send(200, [{"name": "c"}])
            if "cursor=c1" in self.path:
                return self._send(200, [{"name": "b"}], {"X-Next-Cursor": "c2"})
            return self._send(200, [{"name": "a"}], {"X-Next-Cursor": "c1"})
        if path == "/api/v1/events/changes":
            frames = b'event: ping\ndata: {"heartbeat_ms": 5}\n\ndata: {"topics": ["apps"]}\n\n'
            return self._send(200, frames, ctype="text/event-stream")
        if self.command == "PUT":
            return self._send(400, {"error": "bad request"})
        return self._send(404, {"error": "nope"})

    do_GET = do_POST = do_PUT = _handle


class ClientTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.srv = HTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=cls.srv.serve_forever, daemon=True).start()
        cls.client = Client("http://127.0.0.1:%d" % cls.srv.server_port, "tok-123", timeout=5)

    @classmethod
    def tearDownClass(cls):
        cls.srv.shutdown()
        cls.srv.server_close()

    def test_pagination_follows_cursor(self):
        names = [a["name"] for a in self.client.all_apps(limit=1)]
        self.assertEqual(names, ["a", "b", "c"])
        self.assertEqual(Handler.seen[-1][2], "Bearer tok-123")

    def test_changes_stream(self):
        evs = list(self.client.changes())
        self.assertEqual(evs, [{"heartbeat": True, "heartbeat_ms": 5}, {"topics": ["apps"]}])

    def test_env_values_never_leak(self):
        with self.assertRaises(APIError) as cm:
            self.client.set_env("web", {"KEY": SECRET})
        self.assertNotIn(SECRET, str(cm.exception))
        self.assertNotIn(SECRET, repr(cm.exception))
        self.assertEqual(cm.exception.status, 400)
        self.assertNotIn("tok-123", repr(self.client))

    def test_read_sse_comments_and_multiline(self):
        got = list(read_sse([": hi\n", "event: x\n", "data: 1\n", "data: 2\n", "\n"]))
        self.assertEqual(got, [{"event": "x", "id": "", "data": "1\n2"}])


if __name__ == "__main__":
    unittest.main()
