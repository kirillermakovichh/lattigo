# Runtime architecture

Status: design direction; packages and interfaces will be created with actual behavior.
One Go module and a modular node process avoid premature distributed service boundaries.
The wallet is a separate local executable so spending keys need not enter the node.

## Responsibilities and state ownership

| Planned package | Owns | Allowed effects |
| --- | --- | --- |
| internal/protocol | Canonical types, codecs, hashes, deterministic validation | None |
| internal/ledger | State transition rules, frontiers, receivables and confirmation invariants | Through narrow transactional storage contract |
| internal/consensus | Elections, vote validation, weight view, quorum and signing discipline | Explicit persistence and transport dependencies |
| internal/storage | Durable transactions, indexes, schema, migrations and snapshots | Filesystem and selected database |
| internal/p2p | Peer lifecycle, bounded framing, gossip and dependency retrieval | Network |
| internal/rpc | Versioned requests, response status, authentication and limits | Local/API transport |
| internal/wallet | Key lifecycle, local signing, encrypted backup and recovery | Explicit local key storage |
| internal/node | Configuration, construction, readiness and shutdown | Composition of runtime effects |
| cmd/ | Thin node, wallet and control entrypoints | Process setup and exit |

Names are planned, not a requirement to generate empty folders. Do not introduce an
interface for each row. Introduce a consumer seam when it hides real variation or I/O.

## Dependencies

protocol imports no runtime package. ledger depends on protocol and a storage contract.
consensus consumes ledger views and submits decisions through ledger operations.
p2p and rpc submit commands; neither mutates ledger indexes directly.
node constructs concrete dependencies and owns their lifetimes. Avoid import cycles.

Ledger is the authority for atomic state transitions. A confirmed transition updates
frontiers, receivables, delegation/weight indexes and confirmation metadata consistently.
Consensus cannot modify the balance index. Storage does not invent validation rules.
Weight reads and writes need an explicit consistent view from the protocol specification.

## Concurrency and capacity

Serialize conflicting account mutations. Choose the concrete transaction and locking
strategy after the storage decision. Document ordering when multiple accounts are involved.
Keep remote decoding and expensive signature work outside storage transactions where safe.
Every worker has an owner, cancellation signal, bounded admission and a joined shutdown path.
Set limits for bytes, queued items, peers, concurrent verifications, elections and orphan data.
Critical accepted writes cannot disappear into a dropped channel. Overload returns a defined
retryable outcome; gossip may be shed only under a documented policy.

## Lifecycle and failure

Startup validates configuration, acquires the data lock, opens the supported schema,
recovers durable state and signing records, then starts network work. Readiness requires
storage health and the protocol-defined synchronization condition.
Partial startup unwinds acquired resources. Liveness alone does not mean ready to vote.
Shutdown stops admission, stops producers, drains accepted writes, waits for workers,
flushes durable state and releases the lock. Deadline expiry reports incomplete work.
Storage corruption or failed critical writes stop signing and confirmation.
Recovery never resets a database or overwrites a confirmed frontier automatically.

## Interface outcomes

Expose invalid, missing dependency, conflicting, admitted, durable and confirmed outcomes
without conflating them. Read APIs state which ledger view they use.
Logs and RPC responses do not contain wallet secrets or private signing material.
