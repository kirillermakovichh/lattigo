# Runtime architecture

Status: implemented. A Go module contains a modular full-node process, a separate
local wallet and operator CLI. Spending keys never need to enter the node.

| Package | State and responsibilities |
| --- | --- |
| internal/protocol | Exact amounts, genesis, canonical blocks, address/signature/work validation |
| internal/ledger | Confirmed account frontiers, receivables, weight map and certificate transitions |
| internal/storage | Exclusive WAL, checksummed frames, durable signing envelopes and atomic files |
| internal/consensus | Ordered elections, round locks, quorum evidence and signing-before-publication |
| internal/p2p | Configured peers, bounded gossip, rate limits and verified record catchup |
| internal/rpc | Versioned HTTP contracts, bearer authorization, readiness, metrics and staged backup |
| internal/node | Strict config, construction, listener ownership and joined shutdown |
| internal/wallet | Encrypted custody, deterministic keys, offline/online signing and durable outbox |
| internal/client | Bounded RPC client, origin/redirect policy and uncertain submission outcomes |
| internal/control | Genesis/configuration generation, backup, offline verification and restore |
| cmd/ | blnode, blwallet and blctl entrypoints |

## Ownership and ordering

Ledger is the sole authority for atomic confirmed transitions. Consensus consumes a
consistent confirmed view and submits certified records. P2P/RPC never mutate indexes.
The engine serializes commands and voting state with a mutex. Storage serializes durable
updates with its own lock. Transport callbacks run after the engine mutex is released.
Network I/O is outside ledger transitions. The node owns worker and listener lifetimes.

At each global confirmation height, weights come from the preceding confirmed state.
Account balances and pending outgoing funds sum exactly to supply. Protocol.md defines
the ordered confirmation trade-off and weight transitions.

## Durability

WAL1 frames include length and its complement, SHA-256 payload checksum and commit marker.
Records are independently verified on replay and fsynced before publication to readers.
A genuinely partial final frame may be truncated on reopen. Complete malformed frames,
unsupported schema, altered genesis and empty/missing established WAL fail closed.
Representative state has a separate33MiB budget for two quorum proofs, a checksum
envelope and durable signer marker; disappearance
or corruption after initialization fails closed. JSON semantics are validated on recovery.
Directory Flock prevents simultaneous opens. File and directory sync make atomic writes durable.
This storage implementation supports Linux/macOS, not Windows.

Backups contain consistent confirmed journals, not keys or signing authorization.
Restore checks the full journal in staging, refuses nonempty targets and writes an
observer-only marker. It cannot undo external copying of a whole representative identity;
operators must never clone or roll back a signing directory.

## Capacity and failure

The node indexes confirmed history in memory. A serialized-history ceiling defaults to
256MiB; admission reserves one maximum frame before voting near capacity. Configured
ceilings fail closed during replay and new admission. There is no pruning or snapshot
trust shortcut. Operators must size memory and adjust limits before a long-lived network
reaches its ceiling; the serialized limit is not an exact RSS bound.
Protocol representative count, HTTP payloads/parallelism, source rate tables, pending
blocks, round evidence and outgoing queue bytes/items are bounded. See runtime.md.
Overload is reported explicitly; dropped best-effort gossip is repaired by re-broadcast
and certified polling. No accepted critical write is silently dropped.

Startup validates config/genesis/key, locks and replays storage, and recovers signing
state before listening. Partial failure releases acquired resources. Readiness distinguishes
known catchup, missing voting quorum when work is pending, capacity and internal failures.
Shutdown stops admission, drains or closes HTTP connections, joins actual handlers and
workers, and only then releases storage. Expired shutdown deadlines are reported.
