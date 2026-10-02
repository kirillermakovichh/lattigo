# Node runtime contracts

Use docs/architecture.md and accepted ADRs as authority.
- Ledger owns account frontiers, pending transfers and confirmed state.
- Consensus owns elections, vote deduplication and durable signing intent.
- Define the weight view and dependency rules used by every election.
- Persist anti-equivocation signing state before publishing a vote.
- Bound decoding, queues, connections, retained bytes and expensive verification.
- Distinguish admission, durable storage and confirmation in RPC responses.
- Recover and verify schema before readiness. Acquire an exclusive data lock.
- Stop admission, drain accepted writes and wait for workers before closing storage.
- Storage failure stops signing and confirmation. Never silently replace durable state.
- Verify crashes, overload, cancellation, replay and conflicting votes through maintained interfaces.
