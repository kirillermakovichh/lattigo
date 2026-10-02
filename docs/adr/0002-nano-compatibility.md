---
status: accepted
---
# Prefer an independent Nano-inspired protocol

Under the user’s full implementation mandate, implement an independent network that borrows block-lattice and representative
voting concepts, rather than a drop-in Nano node. This permits a purpose-built versioned
protocol but requires its own specification, vectors and security review.
The alternative is Nano ledger/wire compatibility, which requires matching actual signatures,
encodings, work rules, voting semantics and supported protocol versions.
The implementation chooses an independent protocol to avoid an unrequested requirement to reproduce every Nano version-specific rule. Q-01 is resolved.
Until then, no network, address, wallet or interoperability compatibility is promised.
