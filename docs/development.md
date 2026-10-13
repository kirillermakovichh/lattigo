# Development workflow

Go1.26+ and Python3 are the local development tools. Shipped runtime binaries use only Go.
The canonical public import path remains pending an actual remote; the local module path
is example.com/go-block-lattice. No public repository or organization is claimed.

## Commands

| Command | Purpose |
| --- | --- |
| make binaries | Build blnode, blwallet and blctl in bin/ |
| make check | Local links, Go formatting, vet, focused package tests and race detection |
| make build | Compile all Go packages |
| python3 tests/network_smoke.py | One actual multi-process CLI/network/recovery scenario |
| make release VERSION=local | Deterministic committed host-platform archive |
| make release-all VERSION=local | Linux/macOS amd64/arm64 archives |

Run compilation and focused checks at coherent implementation/merge boundaries.
Do not repeat full tests after trivial edits. Run the integrated baseline before declaring
a merged feature complete. A failing build or violated payment invariant blocks integration.
Use tests/network_smoke.py --binary-dir bin to exercise already packaged binaries.
Go caches, binaries, archives and runtime data are excluded from commits.

## Change workflow

Select a stable task ID and read its dependencies and authoritative contract.
Use project-local skills as relevant. Implement one coherent behavior slice.
Stage explicit paths, review the diff and commit the completed block with a Conventional
Commit message. Preserve feature commit history when merging branches.
The user authorized three isolated worktrees for parallel work; shared contract changes
are coordinated by the integration owner. Do not manufacture commits to increase counts.
Update the single task tracker and session handoff at meaningful checkpoints.

## Structure

cmd/ holds thin process entrypoints. internal/ separates protocol, ledger, storage,
consensus, node, p2p, rpc, wallet, client and operator control responsibilities.
Package tests live next to implementation. testdata/protocol owns canonical vectors.
tests/ owns cross-process scenarios. scripts/ owns development/release infrastructure.
No generic utility layer or empty package scaffolding is required.

## Dependency and compatibility policy

The runtime uses the Go standard library. Development skills retain upstream notices.
Protocol1 and WAL1 are explicit contracts; unsupported versions fail closed.
Review canonical/signing and persisted formats before changes. Add an ADR for an
incompatible transition. Update the authoritative docs instead of duplicating definitions.
Release artifacts include source revision, toolchain, license and checksums.
