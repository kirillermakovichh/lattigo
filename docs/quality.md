# Verification and release requirements

Status: required evidence plan. No product-level checks have run because product code is absent.
A green bootstrap check only verifies repository infrastructure.

## Verification layers

| Layer | Evidence |
| --- | --- |
| Codec/crypto | Independent known-answer vectors, canonical round trips, invalid lengths and fuzzing |
| Ledger | Conservation, exact-once receives, invalid signatures, overflow, conflicts and dependency tests |
| Storage | Process-kill recovery, interrupted commits, corruption, full disk, migration and restore |
| Consensus | Executable model, equivocation, replay, stale weight, partitions, quorum loss and recovery |
| P2P | Malformed frames, slow peers, flood fairness, bounded orphan retention and bootstrap verification |
| Wallet/API | Restore, wrong passwords, offline signing, uncertain submission, retries and status semantics |
| Integrated network | Multi-process sends/receives, restart, partitions, conflicting candidates and convergence |
| Operations | Readiness, shutdown deadlines, backup/restore, upgrades and unsupported versions |

A test must demonstrate a contract and fail when that behavior is broken.
Run Go race detection for concurrent behavior. Seed fuzzing with actual protocol vectors.
Use deterministic clocks and network scheduling where consensus traces require replay.
Simulation evidence has limits; it does not constitute a proof of Byzantine safety.

## Adversarial review

Before public release, review unauthorized spending, double consumption, weight reuse,
quorum manipulation, eclipse/bootstrap poisoning, vote replay, signing-state rollback,
resource exhaustion, wallet extraction and unsafe operator defaults.
Specify the attacker capabilities and affected invariant for every finding.
Track fixes with regression evidence; do not replace review with a generic checklist claim.

## Release gates

A release candidate must have a frozen versioned protocol, reproducible genesis,
a compatibility/platform matrix, agreed resource budgets and reproducible measurements.
All product workflows must pass against packaged binaries in a sustained multi-node run.
Critical findings must be resolved. Consensus and cryptographic design need independent review.
Exercise clean installation, migration, restore and interrupted upgrade procedures.
Publish source revision, build environment, checksums, dependency/license inventory,
known limitations, network parameters and operator instructions.
Do not promise a numerical TPS, finality latency or soak duration before its acceptance
threshold and workload are agreed and recorded.
