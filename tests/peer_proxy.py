"""Bounded per-link HTTP proxies with explicit deterministic partition controls."""
import http.client
from http.server import BaseHTTPRequestHandler, HTTPServer
import threading
from urllib.parse import urlsplit

MAX_BYTES = 32 << 20


class PeerProxy:
    def __init__(self, target):
        self.target = urlsplit(target)
        self.blocked = threading.Event()
        self.slow = threading.Event()
        self.closed = threading.Event()
        self.requests = 0
        owner = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def handle_request(self):
                self.connection.settimeout(3)
                owner.requests += 1
                if owner.blocked.is_set():
                    self.send_error(503, "partitioned")
                    return
                if owner.slow.is_set() and owner.closed.wait(2):
                    return
                try:
                    length = int(self.headers.get("Content-Length", "0"))
                    if length < 0 or length > MAX_BYTES or self.headers.get("Transfer-Encoding"):
                        self.send_error(413)
                        return
                    data = self.rfile.read(length)
                    if len(data) != length:
                        self.send_error(400)
                        return
                    headers = {k: v for k, v in self.headers.items()
                               if k.lower() not in {"host", "connection", "content-length"}}
                    upstream = http.client.HTTPConnection(owner.target.hostname, owner.target.port, timeout=3)
                    try:
                        upstream.request(self.command, self.path, data if length else None, headers)
                        response = upstream.getresponse()
                        body = response.read(MAX_BYTES + 1)
                        if len(body) > MAX_BYTES:
                            self.send_error(502)
                            return
                        self.send_response(response.status)
                        for key, value in response.getheaders():
                            if key.lower() not in {"content-length", "connection", "transfer-encoding"}:
                                self.send_header(key, value)
                        self.send_header("Content-Length", str(len(body)))
                        self.end_headers()
                        self.wfile.write(body)
                    finally:
                        upstream.close()
                except (OSError, ValueError, http.client.HTTPException):
                    try:
                        self.send_error(502)
                    except OSError:
                        pass

            do_GET = handle_request
            do_POST = handle_request

        self.server = HTTPServer(("127.0.0.1", 0), Handler)
        self.server.timeout = 3
        self.origin = "http://127.0.0.1:" + str(self.server.server_port)
        self.thread = threading.Thread(target=self.server.serve_forever, kwargs={"poll_interval": 0.05})
        self.thread.start()

    def close(self):
        self.closed.set()
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)
        if self.thread.is_alive():
            raise AssertionError("peer proxy did not terminate")

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()
