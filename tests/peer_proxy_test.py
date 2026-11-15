#!/usr/bin/env python3
from http.server import BaseHTTPRequestHandler, HTTPServer
import threading
import unittest
import urllib.error
import urllib.request
from peer_proxy import PeerProxy

class ProxyTests(unittest.TestCase):
    def test_partition_and_heal(self):
        class Handler(BaseHTTPRequestHandler):
            def log_message(self,*args): pass
            def do_GET(self):
                self.send_response(200);self.end_headers();self.wfile.write(b"confirmed")
        target=HTTPServer(("127.0.0.1",0),Handler)
        thread=threading.Thread(target=target.serve_forever);thread.start()
        try:
            with PeerProxy("http://127.0.0.1:"+str(target.server_port)) as proxy:
                with urllib.request.urlopen(proxy.origin,timeout=3) as response:
                    self.assertEqual(response.read(),b"confirmed")
                proxy.blocked.set()
                with self.assertRaises(urllib.error.HTTPError) as failure:
                    urllib.request.urlopen(proxy.origin,timeout=3)
                self.assertEqual(failure.exception.code,503)
                failure.exception.close()
                proxy.blocked.clear()
                with urllib.request.urlopen(proxy.origin,timeout=3) as response:
                    self.assertEqual(response.read(),b"confirmed")
            self.assertFalse(proxy.thread.is_alive())
        finally:
            target.shutdown();target.server_close();thread.join()

if __name__ == "__main__":
    unittest.main()
