---
status: proposed
---
# Prefer an independent Nano-inspired protocol

Proposal: implement an independent network that borrows block-lattice and representative
voting concepts, rather than a drop-in Nano node. This permits a purpose-built versioned
protocol but requires its own specification, vectors and security review.
The alternative is Nano ledger/wire compatibility, which requires matching actual signatures,
encodings, work rules, voting semantics and supported protocol versions.
The phrase "like Nano" does not resolve this trade-off. Q-01 must be decided before codec implementation.
Until then, no network, address, wallet or interoperability compatibility is promised.
