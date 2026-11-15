#!/usr/bin/env python3
import tempfile
import unittest
from pathlib import Path
from process_harness import Network

class HarnessTests(unittest.TestCase):
    def test_ports_and_secrets_are_owned(self):
        with tempfile.TemporaryDirectory() as temporary:
            with Network(temporary, Path(temporary)) as network:
                self.assertEqual(len(network.reservations), 10)
                self.assertEqual(len(set(network.rpc_ports + network.peer_ports)), 10)
                self.assertGreaterEqual(len(network.token), 32)
            self.assertEqual(network.reservations, [])
            self.assertEqual(network.processes, {})

if __name__ == "__main__":
    unittest.main()
