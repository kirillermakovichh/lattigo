#!/usr/bin/env python3
"""Measure Go decision outcomes in an isolated copy; working sources are never rewritten."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import runpy

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / ".cache/branches")
    parser.add_argument("--run", help="run matching Go tests in the isolated copy")
    parser.add_argument("--strict", action="store_true", help="reject uncovered outcomes without an exact invariant contract")
    parser.add_argument("--race", action="store_true", help="race-check isolated test processes")
    parser.add_argument("--faults", action="store_true", help="inject I/O failures in the isolated source copy")
    parser.add_argument("--network", action="store_true", help="include actual CLI/network processes")
    args = parser.parse_args()
    out = args.output.resolve()
    marker = out / ".branch-audit-owned"
    if out.exists() and any(out.iterdir()) and not marker.is_file():
        raise RuntimeError("refusing to replace an unowned output directory")
    out.mkdir(parents=True, exist_ok=True)
    marker.write_text("Go Block Lattice branch audit\n")
    hits = out / "hits"
    if hits.exists():
        shutil.rmtree(hits)
    hits.mkdir()
    env = dict(os.environ, GOCACHE=str(ROOT / ".cache/go-build"), GOMODCACHE=str(ROOT / ".cache/go-mod"), BL_BRANCH_OUT=str(hits))
    with tempfile.TemporaryDirectory(prefix="bl-decisions-") as temporary:
        copy = Path(temporary)
        for name in ("internal", "cmd", "testdata", "tests", "client"):
            shutil.copytree(ROOT / name, copy / name)
        shutil.copy2(ROOT / "go.mod", copy / "go.mod")
        inventory = out / "inventory.json"
        subprocess.run(["go", "run", str(ROOT / "scripts/branch_coverage/instrument.go"), str(copy), str(inventory)],
                       cwd=ROOT, env=env, check=True)
        test_command = ["go", "test", "-count=1"]
        if args.race:
            test_command += ["-race"]
        if args.run:
            test_command += ["-run", args.run]
        if args.faults:
            subprocess.run(["go", "run", str(ROOT / "scripts/fault_injection/instrument.go"), str(copy)],
                           cwd=ROOT, env=env, check=True)
            test_command += ["-tags", "faultaudit"]
        subprocess.run([*test_command, "./..."], cwd=copy, env=env, check=True)
        if args.network:
            binaries = copy / "bin"
            binaries.mkdir()
            for name in ("blnode", "blwallet", "blctl"):
                subprocess.run(["go", "build", "-o", str(binaries / name), "./cmd/" + name],
                               cwd=copy, env=env, check=True)
            subprocess.run(["python3", str(copy / "tests/network_smoke.py"), "--binary-dir", str(binaries)],
                           cwd=copy, env=env, check=True)
            subprocess.run(["python3", str(copy / "tests/wallet_workflows.py"),
                            "--binary", str(binaries / "blwallet")], cwd=copy, env=env, check=True)
            for scenario in ("partition", "custody", "network"):
                subprocess.run(["python3", str(copy / "tests/process_resilience.py"),
                                "--scenario", scenario, "--binary-dir", str(binaries)],
                               cwd=copy, env=env, check=True)
            for name in ("blnode", "blwallet", "blctl"):
                for arguments in (["--help"], ["--version"], ["--not-a-valid-flag"]):
                    result = subprocess.run([str(binaries / name), *arguments], env=env,
                                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                    expected = 1 if arguments[0] == "--not-a-valid-flag" else 0
                    if result.returncode != expected:
                        raise RuntimeError(f"unexpected {name} exit: {result.returncode}")
    merged = {}
    for path in hits.glob("*.json"):
        for site, outcomes in json.loads(path.read_text()).items():
            counts = merged.setdefault(site, {})
            for outcome, count in outcomes.items():
                counts[outcome] = counts.get(outcome, 0) + count
    sites = json.loads(inventory.read_text())
    reached = total = 0
    missing = []
    for site in sites:
        counts = merged.get(site["id"], {})
        absent = [name for name in site["outcomes"] if not counts.get(name)]
        total += len(site["outcomes"])
        reached += len(site["outcomes"]) - len(absent)
        if absent:
            missing.append(dict(site, missing=absent))
    report = {"scope": "if/for true-false, range empty-nonempty, switch/type-switch/select cases",
              "sites": len(sites), "outcomes": total, "observed": reached,
              "percent": round(100 * reached / total, 1), "unobserved": missing}
    if args.strict:
        exclusion = runpy.run_path(str(ROOT / "scripts/branch_coverage/contracts.py"))["exclusion"]
        gaps = [(site["id"], outcome) for site in missing for outcome in site["missing"]
                if exclusion(site, outcome) is None]
        if gaps:
            raise RuntimeError("unverified decision outcomes: " + repr(gaps))
        print(f"Strict decision gate passed; {total - reached} invariant-excluded outcomes remain in the raw metric")
    (out / "report.json").write_text(json.dumps(report, indent=2) + "\n")
    print(f"Observed {reached}/{total} outcomes ({report['percent']}%); {out / 'report.json'}")


if __name__ == "__main__":
    main()
