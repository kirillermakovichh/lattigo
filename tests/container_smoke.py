#!/usr/bin/env python3
"""Run an owned unprivileged node container with generated temporary inputs."""
import argparse
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[1]


def command(*args, timeout=60, env=None):
    return subprocess.check_output(args, cwd=ROOT, env=env, text=True, stderr=subprocess.PIPE, timeout=timeout).strip()


def cleanup(container, image=None):
    # Called only for this invocation's random names, never global Docker pruning.
    subprocess.run(["docker", "rm", "-f", container], capture_output=True, timeout=30)
    if image:
        subprocess.run(["docker", "image", "rm", image], capture_output=True, timeout=30)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary-dir", type=Path, default=ROOT / "bin")
    parser.add_argument("--image", help="use an existing image instead of building a temporary one")
    args = parser.parse_args()
    if not shutil.which("docker"):
        parser.exit(2, "Docker CLI unavailable; container smoke was not run.\n")
    try:
        version = command("docker", "info", "--format", "{{.ServerVersion}}", timeout=10)
        if not version:
            parser.exit(2, "Docker daemon unavailable; container smoke was not run.\n")
    except (subprocess.SubprocessError, OSError):
        parser.exit(2, "Docker daemon unavailable; container smoke was not run.\n")
    name = "bl-smoke-" + secrets.token_hex(8)
    image = args.image or name + ":local"
    owned_image = None
    try:
        if not args.image:
            owned_image = image
            command("docker", "build", "-t", image, ".", timeout=300)
        user = command("docker", "image", "inspect", "--format", "{{.Config.User}}", image)
        if user != "65532:65532":
            raise AssertionError("container image must declare UID/GID65532")
        with tempfile.TemporaryDirectory(prefix="bl-container-") as temporary:
            root = Path(temporary)
            config_dir = root / "config"
            config_dir.mkdir(mode=0o755)
            env = dict(os.environ, BL_WALLET_PASSWORD=secrets.token_urlsafe(32))
            account = json.loads(command(str(args.binary_dir.resolve() / "blwallet"), "create", "--wallet", str(root / "wallet"), env=env))["account"]
            command(str(args.binary_dir.resolve() / "blctl"), "genesis", "--name", "container-smoke", "--account", account, "--balance", "100", "--work-bits", "0", "--out", str(config_dir / "genesis.json"))
            cfg = {"data_dir": "/data", "genesis": "/config/genesis.json", "rpc_listen": "0.0.0.0:7070", "p2p_listen": "0.0.0.0:7071", "rpc_token": secrets.token_hex(32), "allow_remote_rpc": True}
            (config_dir / "node.json").write_text(json.dumps(cfg))
            for path in config_dir.iterdir():
                path.chmod(0o444)  # Host parent remains private; container mount is read-only.
            command("docker", "run", "--detach", "--name", name, "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--tmpfs", "/data:rw,noexec,nosuid,size=64m,uid=65532,gid=65532,mode=0700", "--mount", f"type=bind,src={config_dir},dst=/config,readonly", "--publish", "127.0.0.1::7070", image)
            port = command("docker", "port", name, "7070/tcp").splitlines()[0]
            deadline = time.monotonic() + 20
            while True:
                try:
                    with urllib.request.urlopen("http://" + port + "/v1/status", timeout=2) as response:
                        body = response.read((64 << 10) + 1)
                        if len(body) > 64 << 10 or json.loads(body)["height"] != 0:
                            raise AssertionError("unexpected generated ledger state")
                    break
                except OSError:
                    if time.monotonic() >= deadline:
                        raise AssertionError("container node did not become reachable")
                    time.sleep(0.1)
            command("docker", "stop", "--time", "20", name, timeout=30)
            if command("docker", "inspect", "--format", "{{.State.ExitCode}}", name) != "0":
                raise AssertionError("container node did not shut down cleanly")
        print("Container smoke passed: unprivileged startup, confirmed reads and owned shutdown.")
    finally:
        cleanup(name, owned_image)


if __name__ == "__main__":
    main()
