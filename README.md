# Go Block Lattice

An independent Nano-inspired payment network written in Go1.26+.
Each account owns a signed chain. Transfers connect account chains through explicit
send/receive operations. Weighted representatives confirm blocks using globally ordered
two-phase voting, making delegated-weight changes deterministic.
This is an independent protocol, not a Nano-compatible node.

## Implemented product

- Signed account chains, exact raw-unit amounts, checked genesis and configurable PoW admission.
- Weighted prevote/precommit confirmation, proposer rotation, durable locks and round recovery.
- Checksummed fsynced ledger, exclusive locking, certified replay and observer-only backup restore.
- Bounded peer gossip, verified catchup, authenticated payment RPC, readiness and metrics.
- Local encrypted wallets, password rotation, named accounts, network-pinned contacts and durable outbox.
- Payment URIs/request files, sequential receive-all, cursor RPC pages, read batches and public Go client.
- Node/wallet/operator CLIs, generated private-network manifests and reproducible binary archives.

## Build and run

```sh
make binaries
bin/blwallet help
bin/blctl help
```

Follow [wallet and private-network setup](docs/wallet.md) to generate your keys and
explicit genesis. Start each node with `bin/blnode --config path/to/node.json`.
See [node configuration and HTTP API](docs/runtime.md) for listeners, peers and tokens.
There are no embedded funded public keys or predefined public network.

## Development and verification

```sh
make check
python3 tests/network_smoke.py --binary-dir bin
```

The integrated scenario starts four representative processes and a restored observer.
It exercises send, receive, delegation, duplicate retries, quorum with one node offline,
restart/catchup, unauthorized requests, backup and subsequent observer synchronization.
[Release packaging](release/README.md) produces local archives and checksums without publishing.

## Limits and project map

Storage supports Linux/macOS. Full history is indexed in memory under a configurable
serialized-history ceiling (default256MiB); pruning is not implemented.
A ledger backup restores an observer, never historical representative signing authority.
Wallets use a trusted RPC node rather than independent light-client verification.
Read [protocol assumptions](docs/protocol.md) and [release evidence](docs/quality.md).

- [Documentation map](docs/README.md)
- [Domain vocabulary](CONTEXT.md)
- [Implementation plan](tasks/plan.md) and [task tracker](tasks/todo.md)
- [Current evidence and handoff](tasks/implementation-progress.md)
- [Contributor rules](AGENTS.md)

MIT licensed. The public module destination, ticker and public-network distribution
remain operator/maintainer choices; no remote repository or deployment has been created.
