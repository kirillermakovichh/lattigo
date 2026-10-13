# Decision register

The implementation mandate resolved technical choices below. User-specific public
network identity and external release evidence are not invented by implementation.

| ID | Current decision or remaining work |
| --- | --- |
| Q-01 | Independent protocol; accepted ADR-0002. No Nano interoperability claim. |
| Q-02 | Ordered dynamic-weight, rotating-proposer two-phase confirmation; ADR-0003 and protocol.md. No online-weight quorum reduction. |
| Q-03 | Per-block configurable PoW plus bounded peers, verification/HTTP admission, queues and payloads. Runtime docs specify limits; stronger economic fairness under sustained adversarial load needs measurement. |
| Q-04 | uint64 raw fixed supply, checked positive genesis allocations; operator explicitly chooses distribution and network genesis. No public distribution has been selected. |
| Q-05 | Checksummed fsynced WAL1, exclusive Flock, certified replay, atomic signing envelopes, bounded indexed history and observer-only restore. |
| Q-06 | Standard Ed25519, SHA-256 domains, checksummed addresses, HMAC wallet derivation and PBKDF2/AES-GCM custody; protocol.md and wallet.md. |
| Q-07 | Operator-configured peer origins; no untrusted automatic URL discovery. Certified replay from explicit genesis is the bootstrap trust model. |
| Q-08 | Linux/macOS amd64/arm64. Explicit runtime/history budgets; no measured public throughput or finality guarantee. Long-duration adversarial capacity evidence remains outstanding. |
| Q-09 | Working name Go Block Lattice, MIT source license. Module path stays example.com/go-block-lattice until a real remote is chosen. No public ticker or remote is invented. |
| Q-10 | Protocol1/WAL1; compatible binary upgrades preserve data, unsupported versions fail closed. Incompatible governance/activation needs a separately specified network upgrade. |

External independent review, public-network deployment policy and sustained adversarial
load qualification remain release work. A functioning integrated implementation does
not manufacture this evidence. See quality.md and the task tracker for current status.
