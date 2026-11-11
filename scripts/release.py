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


def provenance(stage, version, commit, tree, epoch, toolchain, goos, goarch):
    """Deterministic build inputs and subjects; this is metadata, not attestation."""
    return {"schema_version": 1, "source": {"commit": commit, "tree": tree},
            "version": version, "source_date_epoch": epoch,
            "builder": {"toolchain": toolchain, "goos": goos, "goarch": goarch,
                        "cgo_enabled": False, "trimpath": True, "buildvcs": False,
                        "ldflags": "-s -w -buildid= -X main.version=" + version + "+" + commit[:12]},
            "subjects": [{"name": name, "sha256": hashlib.sha256((stage / name).read_bytes()).hexdigest()}
                         for name in ("blnode", "blwallet", "blctl")]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--all", action="store_true", help="build Linux/macOS amd64/arm64")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.+_-]{0,63}", args.version):
        parser.error("version must be a safe 1..64 character identifier")
    commit = run("git", "rev-parse", "HEAD")
    tree = run("git", "rev-parse", "HEAD^{tree}")
    if run("git", "status", "--porcelain"):
        parser.error("commit the complete source tree before packaging")
    epoch = int(os.environ.get("SOURCE_DATE_EPOCH", run("git", "show", "-s", "--format=%ct", "HEAD")))
    targets = TARGETS if args.all else ((run("go", "env", "GOOS"), run("go", "env", "GOARCH")),)
    out = ROOT / "dist"
    out.mkdir(exist_ok=True)
    sums = []
    docs = [ROOT / "LICENSE", *(ROOT / "docs" / name for name in
            ("runtime.md", "wallet.md", "protocol.md", "operations.md", "quality.md", "architecture.md")),
            *sorted((ROOT / "docs" / "adr").glob("*.md"))]
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
            (stage / "README.md").write_text(
                f"# Go Block Lattice {args.version}\n\nSource revision: `{commit}`.\n\n"
                "This archive contains blnode, blwallet and blctl. Run each with --version.\n"
                "Generate your own encrypted wallet and explicit network genesis using the\n"
                "[wallet guide](docs/wallet.md), then start ./blnode --config node.json.\n\n"
                "- [Node configuration and HTTP interfaces](docs/runtime.md)\n"
                "- [Protocol and voting assumptions](docs/protocol.md)\n"
                "- [Backup, recovery and compatible upgrades](docs/operations.md)\n"
                "- [Verification scope and release qualification](docs/quality.md)\n\n"
                "The wallet guide's bin/ prefix refers to source builds; these executables\n"
                "are directly in the extracted directory. No keys, funded public network,\n"
                "or deployment credentials are included. Verify the sibling SHA256SUMS file.\n")
            manifest = {"version": args.version, "commit": commit, "source_date_epoch": epoch,
                        "go": run("go", "version"), "goos": goos, "goarch": goarch,
                        "cgo": False, "external_runtime_dependencies": [], "protocol_version": 1}
            (stage / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
            (stage / "provenance.json").write_text(json.dumps(provenance(stage, args.version, commit, tree, epoch,
                manifest["go"], goos, goarch), indent=2, sort_keys=True) + "\n")
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
                            info.mode = 0o755 if source.parent == stage and source.name in {"blnode", "blwallet", "blctl"} else 0o644
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
