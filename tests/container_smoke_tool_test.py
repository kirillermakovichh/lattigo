#!/usr/bin/env python3
"""Verify owned cleanup without requiring a Docker daemon."""
import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("container_smoke", Path(__file__).with_name("container_smoke.py"))
smoke = importlib.util.module_from_spec(spec)
spec.loader.exec_module(smoke)

class ContainerToolTests(unittest.TestCase):
    def test_cleanup_only_owned_resources(self):
        with patch.object(smoke.subprocess, "run") as run:
            smoke.cleanup("bl-smoke-generated", "bl-smoke-generated:local")
            self.assertEqual(run.call_count, 2)
            self.assertEqual(run.call_args_list[0].args[0], ["docker", "rm", "-f", "bl-smoke-generated"])
            self.assertEqual(run.call_args_list[1].args[0], ["docker", "image", "rm", "bl-smoke-generated:local"])
    def test_empty_server_version_stops_before_build(self):
        with patch.object(smoke.shutil, "which", return_value="docker"), \
             patch.object(smoke, "command", return_value="") as command, \
             patch("sys.argv", ["container_smoke.py"]):
            with self.assertRaises(SystemExit) as stopped:
                smoke.main()
            self.assertEqual(stopped.exception.code, 2)
            self.assertEqual(command.call_count, 1)

    def test_existing_image_is_retained(self):
        with patch.object(smoke.subprocess, "run") as run:
            smoke.cleanup("bl-smoke-generated")
            self.assertEqual(run.call_count, 1)

if __name__ == "__main__":
    unittest.main()
