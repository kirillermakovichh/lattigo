#!/usr/bin/env python3
"""Real-process quorum, interruption and bounded-resource scenarios."""
import argparse
import json
import http.client
import socket
from urllib.parse import urlsplit
from pathlib import Path
import tempfile
import subprocess
import time

from peer_proxy import PeerProxy
from process_harness import Network, build_binaries, rpc


class Cluster:
    def __init__(self, directory, binaries):
        self.network = Network(directory, binaries)
        self.proxies = {}
        self.accounts = []
        self.destination = None
        self.genesis = None

    def __enter__(self):
        n = self.network
        try:
            for i in range(4):
                self.accounts.append(n.wallet(f"rep-{i}", "create")["account"])
                n.wallet(f"rep-{i}", "export-key", "--out", n.path / f"rep-{i}.seed")
            self.destination = n.wallet("recipient", "create")["account"]
            allocations = n.path / "allocations.json"
            allocations.write_text(json.dumps([{"account": a, "balance": "2500", "representative": a} for a in self.accounts]))
            self.genesis = n.cli("blctl", "genesis", "--name", "process-resilience", "--allocations", allocations,
                                 "--work-bits", "0", "--out", n.path / "genesis.json")
            for src in range(4):
                peers = []
                for dst in range(4):
                    if src != dst:
                        proxy = PeerProxy(n.peers[dst])
                        self.proxies[src, dst] = proxy
                        peers.append(proxy.origin)
                path = n.configure(src)
                cfg = json.loads(path.read_text())
                cfg["peers"] = peers
                path.write_text(json.dumps(cfg))
            n.release_ports()
            for i in range(4):
                n.start(i)
            n.converge(range(4), 0)
            return self
        except BaseException:
            self.__exit__(None, None, None)
            raise

    def __exit__(self, kind, value, traceback):
        if kind is not None:
            self.network.diagnostics()
        self.network.close()
        for proxy in self.proxies.values():
            proxy.close()
        self.proxies.clear()

    def partition(self, isolated):
        isolated = set(isolated)
        for (src, dst), proxy in self.proxies.items():
            if (src in isolated) != (dst in isolated):
                proxy.blocked.set()
            else:
                proxy.blocked.clear()

    def heal(self):
        for proxy in self.proxies.values():
            proxy.blocked.clear()

    def send(self, amount="1", node=0, wait=False, timeout="10s"):
        args = ["--to", self.destination, "--amount", amount, "--rpc", self.network.origins[node],
                "--expected-network", self.genesis["network"], "--timeout", timeout]
        if wait:
            args.append("--wait")
        return self.network.wallet("rep-0", "send", *args)


def partition_scenario(c):
    n = c.network
    c.partition({0, 1})
    sent = c.send("10")
    assert sent["status"] == "admitted", sent
    time.sleep(2)  # Multiple bounded consensus rounds with no >2/3 component.
    for origin in n.origins[:4]:
        assert rpc(origin, "/v1/status")["height"] == 0
        rpc(origin, "/v1/block?id=" + sent["hash"], expected=404)
    readiness = rpc(n.origins[0], "/health/ready", expected=503)
    assert not readiness["ready"] and not readiness["quorum_observed"], readiness
    preserved = rpc(n.origins[0], "/v1/account?address=" + c.accounts[0])
    assert preserved["balance"] == "2500", preserved
    n.cli("blctl", "ready", "--rpc", n.origins[0], expected=1)
    c.heal()
    n.converge(range(4), 1)
    assert rpc(n.origins[0], "/health/ready")["ready"]
    for origin in n.origins[:4]:
        assert rpc(origin, "/v1/block?id=" + sent["hash"])["balance"] == "2490"
    print("partition/heal: no minority confirmation; all representatives converged")


def custody_scenario(c):
    n = c.network
    c.partition({0, 1})
    outbox = n.path / "rep-0.wallet.pending-0.json"
    command = [str(n.bin / "blwallet"), "send", "--wallet", str(n.path / "rep-0.wallet"),
               "--to", c.destination, "--amount", "7", "--rpc", n.origins[0],
               "--expected-network", c.genesis["network"], "--wait", "--timeout", "30s"]
    payment = subprocess.Popen(command, env=n.env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        deadline = time.monotonic() + 10
        while not outbox.exists():
            if payment.poll() is not None or time.monotonic() >= deadline:
                raise AssertionError("wallet did not persist uncertain intent")
            time.sleep(0.05)
        original = outbox.read_bytes()
        assert payment.poll() is None, "wallet did not retain ownership while waiting"
        contender = subprocess.run(command[:-3] + ["--timeout", "3s"], env=n.env,
                                   capture_output=True, text=True, timeout=10)
        assert contender.returncode != 0 and "another payment operation" in contender.stderr, contender.stderr
        assert outbox.read_bytes() == original, "concurrent wallet replaced signed intent"
        n.kill(0)
        payment.communicate(timeout=10)
        assert payment.returncode != 0, "interrupted confirmation was reported as success"
        assert outbox.read_bytes() == original, "node interruption changed durable intent"
        inspected = n.wallet("rep-0", "outbox")
        assert inspected["state"] == "stored", inspected
        n.start(0)
        c.heal()
        retried = n.wallet("rep-0", "outbox", "--action", "retry", "--wait", "--rpc", n.origins[0],
                           "--expected-network", c.genesis["network"], "--timeout", "50s")
        assert retried["hash"] == inspected["hash"] and retried["status"] == "confirmed", retried
        final = n.converge(range(4), 1)
        n.kill(2)
        n.start(2)
        assert n.converge(range(4), 1)["tip"] == final["tip"]
        for origin in n.origins[:4]:
            balances = sum(int(rpc(origin, "/v1/account?address=" + account)["balance"]) for account in c.accounts)
            incoming = rpc(origin, "/v1/receivables?address=" + c.destination)
            assert balances + sum(int(row["amount"]) for row in incoming) == 10000
            assert len(incoming) == 1 and incoming[0]["source"] == inspected["hash"]
        duplicate = n.wallet("rep-0", "outbox", "--action", "retry", "--wait", "--rpc", n.origins[0], "--timeout", "10s")
        assert duplicate["hash"] == inspected["hash"] and rpc(n.origins[0], "/v1/status")["height"] == 1
        assert outbox.read_bytes() == original
        print("custody/restart: concurrent CLI exclusion, exact outbox retry and supply conservation")
    finally:
        if payment.poll() is None:
            payment.kill()
        payment.communicate(timeout=5)


def peer_request(c, index, method, path, body=None):
    target = urlsplit(c.network.peers[index])
    conn = http.client.HTTPConnection(target.hostname, target.port, timeout=5)
    try:
        conn.request(method, path, body, {"X-BL-Network": c.genesis["network"], "X-BL-Version": "1"})
        response = conn.getresponse()
        data = response.read(4097)
        assert len(data) <= 4096, "unbounded peer rejection response"
        return response.status, data
    finally:
        conn.close()


def network_scenario(c):
    n = c.network
    c.proxies[0, 3].slow.set()
    first = c.send("2", wait=True, timeout="50s")
    assert first["status"] == "confirmed", first
    n.converge(range(4), 1)
    for origin in n.origins[:4]:
        peers = rpc(origin, "/v1/admin/peers", token=n.token)
        assert len(peers) == 3
        for peer in peers:
            assert 0 <= peer["queued_bytes"] <= 48 << 20, peer
            assert 0 <= peer["queued_messages"] <= 48, peer
    c.proxies[0, 3].slow.clear()
    # Hold the16 admitted P2P handlers on incomplete bodies. RPC is independent.
    for (src, dst), proxy in c.proxies.items():
        if dst == 3:
            proxy.blocked.set()
    sockets = []
    try:
        for _ in range(16):
            peer = socket.create_connection(("127.0.0.1", n.peer_ports[3]), timeout=3)
            sockets.append(peer)
            peer.sendall(("POST /p2p/message HTTP/1.1\r\nHost: localhost\r\nContent-Length: 1024\r\n"
                          "X-BL-Network: " + c.genesis["network"] + "\r\nX-BL-Version: 1\r\n\r\n{").encode())
        deadline = time.monotonic() + 3
        while True:
            status, _ = peer_request(c, 3, "GET", "/p2p/status")
            if status == 503:
                break
            assert time.monotonic() < deadline, "P2P handler capacity was not enforced"
            time.sleep(0.05)
        assert rpc(n.origins[3], "/v1/status")["height"] == 1
    finally:
        for peer in sockets:
            peer.close()
        c.heal()
    deadline = time.monotonic() + 5
    while peer_request(c, 3, "GET", "/p2p/status")[0] != 200:
        assert time.monotonic() < deadline, "disconnected readers retained P2P capacity"
        time.sleep(0.05)
    status, _ = peer_request(c, 3, "POST", "/p2p/message", b" " * ((20 << 20) + 1))
    assert status == 400, status
    assert rpc(n.origins[3], "/v1/status")["height"] == 1
    # No spending key or password is passed to the explicit-address event consumer.
    watch = [str(n.bin / "blwallet"), "watch", "--address", c.destination, "--incoming",
             "--rpc", n.origins[0], "--expected-network", c.genesis["network"], "--count", "1", "--timeout", "30s"]
    env = {k: v for k, v in n.env.items() if k != "BL_WALLET_PASSWORD"}
    replay = subprocess.run(watch, env=env, capture_output=True, text=True, timeout=35)
    assert replay.returncode == 0, replay.stderr
    delivered = json.loads(replay.stdout)
    assert delivered["height"] == 1 and delivered["block"]["balance"] == "2498", delivered
    consumer = subprocess.Popen(watch + ["--after", delivered["id"]], env=env,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        time.sleep(0.5)
        assert consumer.poll() is None, "watch exited before restart"
        n.kill(0)
        n.start(0)
        second = c.send("3", wait=True, timeout="50s")
        n.converge(range(4), 2)
        stdout, stderr = consumer.communicate(timeout=30)
        assert consumer.returncode == 0, stderr
        resumed = json.loads(stdout)
        assert resumed["height"] == 2 and resumed["id"] == c.genesis["network"] + ":2", resumed
        assert resumed["block"]["balance"] == "2495" and second["status"] == "confirmed", resumed
        print("network bounds/resume: slow peer fairness, handler/byte limits and restart event replay")
    finally:
        if consumer.poll() is None:
            consumer.kill()
        consumer.communicate(timeout=5)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary-dir", type=Path)
    parser.add_argument("--scenario", choices=["partition", "custody", "network"], default="partition")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="bl-process-resilience-") as temporary:
        binaries = args.binary_dir.resolve() if args.binary_dir else build_binaries(Path(temporary) / "bin")
        with Cluster(temporary, binaries) as cluster:
            {"partition": partition_scenario, "custody": custody_scenario, "network": network_scenario}[args.scenario](cluster)


if __name__ == "__main__":
    main()
