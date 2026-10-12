#!/usr/bin/env python3
"""Build deterministic local release archives; never uploads or publishes them."""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = (("linux", "amd64"), ("linux", "arm64"), ("darwin", "amd64"), ("darwin", "arm64"))


def run(*args):
    return subprocess.check_output(args, cwd=ROOT, text=True).strip()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--all", action="store_true", help="build Linux/macOS amd64/arm64")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.+_-]{0,63}", args.version):
        parser.error("version must be a safe 1..64 character identifier")
    commit = run("git", "rev-parse", "HEAD")
    if run("git", "status", "--porcelain"):
        parser.error("commit the complete source tree before packaging")
    epoch = int(os.environ.get("SOURCE_DATE_EPOCH", run("git", "show", "-s", "--format=%ct", "HEAD")))
    targets = TARGETS if args.all else ((run("go", "env", "GOOS"), run("go", "env", "GOARCH")),)
    out = ROOT / "dist"
    out.mkdir(exist_ok=True)
    sums = []
    docs = [ROOT / "README.md", ROOT / "LICENSE", *sorted((ROOT / "docs").rglob("*.md"))]
    for goos, goarch in targets:
        if (goos, goarch) not in TARGETS:
            parser.error("only Linux/macOS amd64/arm64 storage platforms are supported")
        stem = f"go-block-lattice-{args.version}-{goos}-{goarch}"
        with tempfile.TemporaryDirectory(prefix="bl-release-") as temporary:
            stage = Path(temporary)
            env = dict(os.environ, CGO_ENABLED="0", GOOS=goos, GOARCH=goarch)
            env.setdefault("GOCACHE", str(ROOT / ".cache" / "go-build"))
            for name in ("blnode", "blwallet", "blctl"):
                subprocess.run(["go", "build", "-trimpath", "-buildvcs=false", "-ldflags",
                                f"-s -w -buildid= -X main.version={args.version}+{commit[:12]}",
                                "-o", str(stage / name), f"./cmd/{name}"], cwd=ROOT, env=env, check=True)
            manifest = {"version": args.version, "commit": commit, "source_date_epoch": epoch,
                        "go": run("go", "version"), "goos": goos, "goarch": goarch,
                        "cgo": False, "external_runtime_dependencies": [], "protocol_version": 1}
            (stage / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
            archive = out / f"{stem}.tar.gz"
            with archive.open("wb") as raw:
                with gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=epoch) as zipped:
                    with tarfile.open(fileobj=zipped, mode="w", format=tarfile.USTAR_FORMAT) as tar:
                        files = [(p, p.name) for p in sorted(stage.iterdir())]
                        files += [(p, str(p.relative_to(ROOT))) for p in docs]
                        for source, name in files:
                            data = source.read_bytes()
                            info = tarfile.TarInfo(f"{stem}/{name}")
                            info.size = len(data)
                            info.mode = 0o755 if source.parent == stage and source.name != "manifest.json" else 0o644
                            info.mtime = epoch
                            info.uid = info.gid = 0
                            info.uname = info.gname = ""
                            tar.addfile(info, io.BytesIO(data))
            sums.append(f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}")
            print(archive.relative_to(ROOT), flush=True)
    checksums = out / f"SHA256SUMS-{args.version}"
    checksums.write_text("\n".join(sorted(sums)) + "\n")
    print(checksums.relative_to(ROOT))


if __name__ == "__main__":
    main()
