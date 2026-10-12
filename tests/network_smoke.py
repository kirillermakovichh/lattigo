#!/usr/bin/env python3
"""Exercise packaged CLI workflows against four real local node processes.

All identities, credentials and ledger data are generated in a temporary directory.
Run from any directory: python3 tests/network_smoke.py [--binary-dir bin]
"""

import argparse
import json
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
MAX_RESPONSE = 8 << 20


def rpc(origin, path, body=None, token=None, expected=200):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(
        origin + path,
        data=json.dumps(body).encode() if body is not None else None,
        headers=headers,
    )
    try:
        response = urllib.request.urlopen(request, timeout=3)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        data = response.read(MAX_RESPONSE + 1)
        if len(data) > MAX_RESPONSE:
            raise AssertionError("RPC response exceeded fixture bound")
        if response.status != expected:
            raise AssertionError(f"{path}: HTTP {response.status}: {data.decode()}")
        return json.loads(data)


class Network:
    def __init__(self, directory, binary_dir):
        self.path = Path(directory)
        self.bin = binary_dir
        self.env = os.environ.copy()
        self.env["BL_WALLET_PASSWORD"] = secrets.token_urlsafe(32)
        self.env["BL_RPC_TOKEN"] = secrets.token_hex(32)
        self.token = self.env["BL_RPC_TOKEN"]
        self.processes = {}
        self.logs = {}
        self.reservations = []
        self.rpc_ports = [self.reserve() for _ in range(5)]
        self.peer_ports = [self.reserve() for _ in range(5)]
        self.origins = [f"http://127.0.0.1:{p}" for p in self.rpc_ports]
        self.peers = [f"http://127.0.0.1:{p}" for p in self.peer_ports]

    def reserve(self):
        listener = socket.socket()
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
        self.reservations.append(listener)
        return port

    def release_ports(self):
        for listener in self.reservations:
            listener.close()
        self.reservations.clear()

    def cli(self, binary, *args, expected=0):
        process = subprocess.run(
            [str(self.bin / binary), *map(str, args)],
            env=self.env,
            text=True,
            capture_output=True,
            timeout=65,
        )
        if process.returncode != expected:
            raise AssertionError(
                f"{binary} {args[0]} failed ({process.returncode}): {process.stderr}"
            )
        if process.stdout.strip():
            return json.loads(process.stdout)
        return None

    def wallet(self, name, command, *args):
        return self.cli("blwallet", command, "--wallet", self.path / f"{name}.wallet", *args)

    def configure(self, index, representative=True):
        config = {
            "data_dir": str(self.path / f"data-{index}"),
            "genesis": str(self.path / "genesis.json"),
            "rpc_listen": f"127.0.0.1:{self.rpc_ports[index]}",
            "p2p_listen": f"127.0.0.1:{self.peer_ports[index]}",
            "peers": [p for i, p in enumerate(self.peers[:4]) if i != index],
            "rpc_token": self.token,
            "representative_key": str(self.path / f"rep-{index}.seed") if representative else "",
            "round_timeout_ms": 300,
            "max_pending": 256,
            "max_peers": 8,
            "allow_remote_rpc": False,
        }
        path = self.path / f"node-{index}.json"
        path.write_text(json.dumps(config))
        path.chmod(0o600)
        return path

    def start(self, index):
        log = open(self.path / f"node-{index}.log", "ab")
        self.logs[index] = log
        self.processes[index] = subprocess.Popen(
            [str(self.bin / "blnode"), "--config", str(self.path / f"node-{index}.json")],
            env=self.env,
            stdout=log,
            stderr=subprocess.STDOUT,
        )
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            if self.processes[index].poll() is not None:
                raise AssertionError(f"node {index} exited during startup")
            try:
                rpc(self.origins[index], "/health/live")
                return
            except (OSError, AssertionError):
                time.sleep(0.1)
        raise AssertionError(f"node {index} did not start")

    def stop(self, index):
        process = self.processes.pop(index, None)
        if process is None:
            return
        process.send_signal(signal.SIGTERM)
        try:
            code = process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
            raise AssertionError(f"node {index} did not shut down")
        self.logs.pop(index).close()
        if code:
            raise AssertionError(f"node {index} exit status {code}")

    def converge(self, indexes, height):
        deadline = time.monotonic() + 35
        last = None
        while time.monotonic() < deadline:
            last = [rpc(self.origins[i], "/v1/status") for i in indexes]
            if all(s["height"] == height for s in last) and len({s["tip"] for s in last}) == 1:
                return last[0]
            time.sleep(0.15)
        raise AssertionError(f"nodes did not converge to height {height}: {last}")

    def close(self):
        self.release_ports()
        for index in list(self.processes):
            try:
                self.stop(index)
            except Exception:
                process = self.processes.pop(index, None)
                if process and process.poll() is None:
                    process.kill()
                    process.wait(timeout=5)
        for log in self.logs.values():
            log.close()
        self.logs.clear()

    def diagnostics(self):
        for index in range(5):
            path = self.path / f"node-{index}.log"
            if path.exists():
                print(f"node {index} log:\n{path.read_text()[-12000:]}")


def scenario(network):
    accounts = []
    for i in range(4):
        name = f"rep-{i}"
        created = network.wallet(name, "create")
        accounts.append(created["account"])
        network.wallet(name, "export-key", "--out", network.path / f"rep-{i}.seed")
    destination = network.wallet("recipient", "create")["account"]
    allocations = [{"account": a, "balance": "2500", "representative": a} for a in accounts]
    allocation_file = network.path / "allocations.json"
    allocation_file.write_text(json.dumps(allocations))
    genesis = network.cli(
        "blctl", "genesis", "--name", "generated-network-smoke", "--allocations", allocation_file,
        "--work-bits", "0", "--out", network.path / "genesis.json",
    )
    for i in range(4):
        network.configure(i)
    network.release_ports()
    for i in range(4):
        network.start(i)
    network.converge(range(4), 0)
    common = ["--rpc", network.origins[0], "--expected-network", genesis["network"], "--wait", "--timeout", "50s"]
    sent = network.wallet("rep-0", "send", "--to", destination, "--amount", "25", *common)
    assert sent["status"] == "confirmed", sent
    network.converge(range(4), 1)
    received = network.wallet(
        "recipient", "receive", "--source", sent["hash"], "--representative", accounts[0], *common,
    )
    assert received["status"] == "confirmed", received
    delegated = network.wallet("recipient", "delegate", "--representative", accounts[1], *common)
    assert delegated["status"] == "confirmed", delegated
    network.converge(range(4), 3)
    state = rpc(network.origins[0], "/v1/account?address=" + destination)
    assert state["balance"] == "25" and state["representative"] == accounts[1], state
    # Idempotent retry uses exactly the original signed block, never a replacement.
    duplicate = network.cli(
        "blwallet", "submit", "--file", network.path / "rep-0.wallet.pending-0.json",
        "--rpc", network.origins[0],
    )
    assert duplicate["status"] == "confirmed" and duplicate["hash"] == sent["hash"], duplicate
    assert rpc(network.origins[0], "/v1/status")["height"] == 3
    rpc(network.origins[0], "/v1/blocks", body={}, expected=401)
    # Three live representatives still hold more than two thirds after delegation.
    network.stop(3)
    second = network.wallet("rep-0", "send", "--to", destination, "--amount", "5", *common)
    assert second["status"] == "confirmed", second
    network.converge(range(3), 4)
    network.start(3)
    tip = network.converge(range(4), 4)
    backup = network.path / "ledger.backup"
    network.cli("blctl", "backup", "--rpc", network.origins[0], "--out", backup)
    restored = network.cli(
        "blctl", "restore", "--genesis", network.path / "genesis.json", "--backup", backup,
        "--data-dir", network.path / "data-4",
    )
    assert restored["status"] == "restored_observer_only", restored
    inspected = network.cli(
        "blctl", "verify", "--genesis", network.path / "genesis.json", "--data-dir", network.path / "data-4",
    )
    assert inspected["tip"] == tip["tip"] and inspected["height"] == 4, inspected
    network.configure(4, representative=False)
    network.start(4)
    network.converge(range(5), 4)
    # A restored observer can independently verify and catch up another certified transition.
    final_receive = network.wallet("recipient", "receive", "--source", second["hash"], *common)
    assert final_receive["status"] == "confirmed", final_receive
    final = network.converge(range(5), 5)
    account = rpc(network.origins[4], "/v1/account?address=" + destination)
    assert account["balance"] == "30", account
    print(json.dumps({"status": "passed", "representatives": 4, "restored_observers": 1,
                      "confirmed_height": final["height"], "recipient_balance": account["balance"],
                      "scenarios": ["send", "receive", "delegate", "idempotent_retry", "unauthorized_write",
                                    "quorum_with_one_offline", "restart_catchup", "backup", "observer_restore"]}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary-dir", type=Path, help="use existing binaries instead of building")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="bl-network-smoke-") as directory:
        binary_dir = args.binary_dir.resolve() if args.binary_dir else Path(directory) / "bin"
        if not args.binary_dir:
            binary_dir.mkdir()
            env = os.environ.copy()
            env.setdefault("GOCACHE", str(ROOT / ".cache/go-build"))
            for binary in ("blnode", "blwallet", "blctl"):
                subprocess.run(["go", "build", "-o", str(binary_dir / binary), "./cmd/" + binary],
                               cwd=ROOT, env=env, check=True, timeout=90)
        network = Network(directory, binary_dir)
        try:
            scenario(network)
        except BaseException:
            network.diagnostics()
            raise
        finally:
            network.close()


if __name__ == "__main__":
    main()
