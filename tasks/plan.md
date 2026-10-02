# Implementation plan

The destination is the complete product in docs/product.md. Phases order work;
none is a substitute for the release requirements in docs/quality.md.
Completion status exists only in [todo.md](todo.md).

## Sequence

1. Repository foundation: local skills, maintained contracts, tooling and handoff.
2. Protocol decisions: compatibility, cryptography, amounts/genesis and executable voting model.
3. A verified payment slice: canonical blocks, account transitions, durable state and recovery.
4. Representative confirmation: vote persistence, weight rules, elections and dependency finality.
5. A networked slice: bounded peer exchange, verified bootstrap and multi-process payments.
6. Product workflows: local wallet, versioned RPC, administration and operator visibility.
7. Release readiness: overload evidence, migrations, recovery drills, review and packaged binaries.

## Dependencies and checkpoints

Q-01 and Q-06 precede codecs and signing. Q-04 precedes genesis and supply invariants.
Q-02 precedes voting code. Q-05 precedes durable ledger. Q-03 precedes public admission.
After local payment/recovery, demonstrate a restart preserving balances and receivables.
After confirmation, reproduce replay, equivocation and partition traces.
After networking, demonstrate end-to-end transfer, conflict and restart across processes.
Before release, satisfy every quality and operations requirement with actual evidence.

## Task discipline

Bootstrap tasks are fully specified in todo.md. Later scope is decomposed into bounded
feature slices there. Before starting a future task, expand it if it would touch more than
roughly five implementation files or cannot be verified in one focused session.
Do not generate all planned packages at once. Do not write protocol behavior from an
unresolved proposal. Keep independent work progressing without guessing economic policy.

## Risks

- Weight/quorum rules can violate safety even with valid signatures: model first.
- Crash or backup rollback can cause representative equivocation: durable signing design first.
- Feeless admission can exhaust resources: measured limits and adversarial workloads.
- Bootstrap can import false finality: specify validation or explicit trust anchors.
- Product scope can drift into premature layers: implement behavior slices through small interfaces.

The initial request authorizes repository setup and documentation. This plan records
subsequent development order; no ledger or consensus implementation is claimed complete.

## Authorized implementation run

The user now authorizes full implementation with three subagents on separate branches.
Use docs/implementation-contract.md as the frozen cross-agent contract.

- Core branch: protocol primitives, ledger transitions, durable storage and certificates.
- Runtime branch: P2P, RPC, node lifecycle and operator endpoints.
- Wallet branch: encrypted local custody, RPC client and command-line workflows.
- Integration owner: weighted two-phase confirmation, shared decisions, packaging and end-to-end checks.

Merge coherent blocks with feature history preserved. Compile and verify at integration
boundaries rather than after every edit. Fix integration-blocking defects immediately.
Public deployment and external independent review are not implied by writing code.
