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
from process_harness import Network, rpc, build_binaries

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
    request = network.wallet("recipient", "request", "--network", genesis["network"], "--amount", "25")
    sent = network.wallet("rep-0", "send", "--uri", request["uri"], *common)
    assert sent["status"] == "confirmed", sent
    network.converge(range(4), 1)
    received = network.wallet(
        "recipient", "receive-all", "--limit", "1", "--representative", accounts[0],
        "--rpc", network.origins[0], "--expected-network", genesis["network"], "--timeout", "50s",
    )
    assert len(received["completed"]) == 1 and received["completed"][0]["status"] == "confirmed", received
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
    invoice = network.path / "payment-request.json"
    network.wallet("recipient", "request", "--network", genesis["network"], "--amount", "5",
                   "--format", "json", "--out", invoice)
    second = network.wallet("rep-0", "send", "--request-file", invoice, *common)
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
                                    "quorum_with_one_offline", "restart_catchup", "backup", "observer_restore", "payment_uri", "request_file", "receive_all"]}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary-dir", type=Path, help="use existing binaries instead of building")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="bl-network-smoke-") as directory:
        binary_dir = args.binary_dir.resolve() if args.binary_dir else Path(directory) / "bin"
        if not args.binary_dir:
            build_binaries(binary_dir)
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
