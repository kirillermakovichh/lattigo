# Product requirements

Status: implemented product scope with outstanding external release qualification. Nano-inspired architecture, Go, complete-product target
and organized development are user requirements. The scope below translates that target
into reviewable requirements. Open choices remain in open-questions.md.

## Users and outcomes

Account holders create and restore local wallets, verify destinations, sign transfers,
track receivables and distinguish submission from confirmation.
Node operators join a selected network, synchronize verified state, monitor capacity,
back up data and recover after failure without silently losing confirmed state.
Representatives manage voting keys, accept delegation and vote without conflicting signatures.
Integrators submit signed blocks and query balances, history and confirmation through versioned RPC.

## Product scope

| Capability | Required outcome |
| --- | --- |
| Payments | Exact integer amounts; sends, receives, opening accounts and delegation |
| Consensus | Specified representative voting, conflicts, dependency finality and replay protection |
| Networking | Peer discovery, authenticated identity where required, bounded gossip and verified bootstrap |
| Persistence | Atomic transitions, migrations, corruption detection and crash recovery |
| Wallet | Local custody, encrypted backup, restore, offline signing and explicit confirmation status |
| CLI and RPC | Stable errors, versioning, pagination, idempotent retries and bounded requests |
| Operations | Config validation, readiness, logs, metrics, backup/restore and upgrade guidance |
| Delivery | Reproducible release procedure, checksums, compatibility matrix and release evidence |

The implemented payment policy is feeless transfers with per-block proof-of-work and
bounded admission. Economic fairness under sustained adversarial pressure still requires measurement.
No throughput or finality latency is promised before reproducible measurements.

## User workflows

- Wallet creation produces a recoverable identity; restoration reproduces the same accounts.
- Sending validates amount and network, signs locally, submits and reports a durable block identifier.
- A timeout leaves an uncertain submission outcome; lookup precedes any conflicting retry.
- Receiving consumes a receivable once; opening an account is a defined receive case.
- Delegating changes voting influence without granting spending authority.
- Operators can distinguish catching up, healthy operation, overload and storage failure.

## Product boundaries

The first product is a payment network. Smart contracts, bridges, exchanges and a browser
wallet are outside the current scope. A useful CLI wallet and RPC are part of the product.
No server-side custody is required. A local daemon does not need access to wallet spending keys.
Public network launch, distribution and governance remain explicit decisions.

## Completion

A release is a verified integrated product, not merely implemented packages.
Use quality.md for release evidence and operations.md for operator obligations.
