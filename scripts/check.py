#!/usr/bin/env python3
"""Run Go checks when actual application packages exist."""
import os
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parents[1]
os.chdir(root)
# Ignore repository metadata, caches, local runtime files and vendored code.
sources = [p for p in root.rglob("*.go")
           if not any(part in {".git", ".cache", ".local", "vendor"}
                      for part in p.relative_to(root).parts)]
if not sources:
    print("No Go packages yet; Go build/vet/test/race checks skipped.")
    sys.exit(0)
if sys.argv[1:] == ["--build"]:
    subprocess.run(["go", "build", "./..."], check=True)
else:
    subprocess.run(["python3", "tests/branch_coverage_tool_test.py"], check=True)
    formatted = subprocess.run(["gofmt", "-l", *map(str, sources)],
                               check=True, capture_output=True, text=True)
    if formatted.stdout:
        print("Go formatting required:\n" + formatted.stdout)
        sys.exit(1)
    for args in (["go", "vet", "./..."], ["go", "test", "./..."],
                 ["go", "test", "-race", "./..."]):
        subprocess.run(args, check=True)
