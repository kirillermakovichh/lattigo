#!/usr/bin/env python3
"""Check deterministic release subjects without invoking a compiler."""
import hashlib
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("release", Path(__file__).resolve().parents[1] / "scripts/release.py")
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)

class ProvenanceTests(unittest.TestCase):
    def test_exact_subjects_and_reproducibility(self):
        with tempfile.TemporaryDirectory() as tmp:
            stage = Path(tmp)
            for name in ("blnode", "blwallet", "blctl"):
                (stage / name).write_bytes(name.encode())
            args = (stage, "local", "a" * 40, "b" * 40, 1, "go version", "linux", "amd64")
            first = release.provenance(*args)
            self.assertEqual(first, release.provenance(*args))
            self.assertEqual(first["subjects"][0]["sha256"], hashlib.sha256(b"blnode").hexdigest())
            self.assertEqual(first["source"]["tree"], "b" * 40)

if __name__ == "__main__":
    unittest.main()
