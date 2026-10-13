# Verification and release requirements

Status: implemented verification baseline with external release qualification outstanding.
Package checks and the generated-key multi-process scenario exercise actual product code.
The final integrated command results are recorded in tasks/implementation-progress.md.
Specific regression scenarios are evidence, not an exhaustive correctness proof.

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

## Current evidence and outstanding qualification

The suite covers independent block digest vectors, exact amounts, invalid certificates,
transfer/weight conservation, self-receive delegation, restart and backup, torn/corrupt WAL,
lost/corrupt signing state, capacity and byte pagination, encrypted restore, outbox signing,
authorization, peer-origin restrictions, real HTTP catchup and joined handler shutdown.
Consensus scenarios include weighted send/receive, quorum arithmetic, missing quorum,
late certificates, authenticated future-round catchup and an equivocating proposer with
partitioned votes plus a locked representative restart.
The optional network smoke scenario runs four representative binaries and a restored
observer through real CLI flows and certified synchronization.

Not yet evidenced: a long-duration hostile public deployment, exhaustive scheduler/model
exploration, independent external cryptographic/consensus review, hardware/disk-loss drills,
load-based fairness qualification and a public-network distribution/governance ceremony.
No numerical public throughput/finality claim or external-audit claim is made.
Keep BL-020/BL-021 open for those requirements instead of checking them off because the
executables compile. Binaries and reproducible archives are delivered independently of
public-network launch authorization.
