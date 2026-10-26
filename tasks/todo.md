# Task tracker

This is the only task-completion checklist. IDs remain stable.
Future tasks must be split before implementation if they exceed a focused session.
Every task also meets the [Definition of Done](../.agents/references/definition-of-done.md).

## BL-001: Import project engineering skills

- [x] Complete

**Dependencies:** None.

**Scope:** .agents/.

**Acceptance:** Project-local skills retain applicable upstream attribution and licenses.

**Verification:** Review skill links and adapted text; git diff --check.

## BL-002: Record product and protocol design baseline

- [x] Complete

**Dependencies:** BL-001.

**Scope:** CONTEXT.md, docs/.

**Acceptance:** Product scope, glossary, architecture, invariants and release requirements distinguish accepted direction from open decisions.

**Verification:** Review cross-document consistency; make docs-check.

## BL-003: Initialize Go tooling and working agreement

- [x] Complete

**Dependencies:** BL-001, BL-002.

**Scope:** go.mod, Makefile, scripts/, .github/, AGENTS.md, tasks/.

**Acceptance:** One Go module; local and CI checks share a baseline; task and handoff locations are canonical.

**Verification:** make check; make build; inspect CI workflow. No application tests are claimed.

## BL-004: Resolve Nano compatibility scope

- [x] Complete

**Dependencies:** BL-002.

**Scope:** docs/research/nano.md, docs/open-questions.md, docs/adr/0002-nano-compatibility.md.

**Acceptance:** Pin Nano code and docs; compare independent-network and compatibility scope; record the chosen scope and consequences.

**Verification:** Trace each adopted rule to a pinned source; review ADR status. Resolve Q-01.

**Implementation evidence:** Independent-network decision accepted in ADR-0002; scoped Nano revision comparison pinned in docs/research/nano.md.


## BL-005: Specify canonical blocks and signing

- [x] Complete

**Dependencies:** BL-004.

**Scope:** docs/protocol.md, docs/open-questions.md, testdata/protocol/.

**Acceptance:** Resolve Q-06 and amount representation from Q-04; specify canonical bytes, domains, algorithms and positive/negative vectors.

**Verification:** Cross-check vectors against an independent implementation; reject noncanonical encodings.

**Implementation evidence:** Canonical/hash vector independently calculated and checked; standard Ed25519 implementation used. No Nano format equivalence claimed.


## BL-006: Model representative safety and liveness

- [x] Complete

**Dependencies:** BL-004.

**Scope:** docs/protocol.md, docs/open-questions.md, internal/consensus/.

**Acceptance:** Resolve Q-02; specify weight, election identity, quorum, final votes and signing recovery assumptions; create an executable model.

**Verification:** Replay partition, re-delegation, equivocation and stale-weight traces. Split model tasks before coding if needed.

**Implementation evidence:** Executable engine and weighted/adversarial scheduled traces implemented. These traces are not exhaustive model checking or a formal proof.


## BL-007: Implement canonical protocol primitives

- [x] Complete

**Dependencies:** BL-005.

**Scope:** internal/protocol/, testdata/protocol/.

**Acceptance:** Types, amount arithmetic, codec, hashes and signatures match the accepted vectors and reject malformed input.

**Verification:** Known-answer tests, overflow tests, fuzz seeds and make check.

**Implementation evidence:** internal/protocol and canonical vector shipped; signature/work/amount tests implemented.


## BL-008: Implement local account transitions

- [x] Complete

**Dependencies:** BL-007.

**Scope:** internal/ledger/.

**Acceptance:** Send, receive/open and representative changes preserve supply and consume receivables once; duplicate and missing-dependency outcomes are explicit.

**Verification:** Deterministic transition and conflict tests through the ledger interface; make check.

**Implementation evidence:** Account transitions, exact-once receivables, dependency rejection and delegated-weight invariants implemented.


## BL-009: Select durable storage contract

- [x] Complete

**Dependencies:** BL-008.

**Scope:** docs/architecture.md, docs/open-questions.md, docs/adr/.

**Acceptance:** Resolve Q-05 with atomicity, locking, schema, corruption, backup and migration comparison.

**Verification:** Focused durability experiment and recorded decision; no unsupported durability claims.

**Implementation evidence:** ADR-0004 compares standard-library WAL trade-offs with embedded databases; capacity and restore policy explicit.


## BL-010: Persist a payment across restart

- [x] Complete

**Dependencies:** BL-009.

**Scope:** internal/storage/, internal/ledger/.

**Acceptance:** A complete transfer survives restart; account frontiers, receivables and delegation indexes change atomically.

**Verification:** Process-kill and interrupted-transaction tests; reopen and verify conservation; make check.

**Implementation evidence:** WAL replay, torn tail, corruption, fsync, locking, checksummed signing and real restart/backup implemented.


## BL-011: Prevent conflicting representative signatures

- [x] Complete

**Dependencies:** BL-006, BL-010.

**Scope:** internal/consensus/, internal/storage/.

**Acceptance:** Persist signing intent before publication; restart cannot produce a prohibited conflicting vote; backup rollback policy is explicit.

**Verification:** Crash at each signing boundary and replay conflicting requests; make check.

**Implementation evidence:** Durable signing envelope/marker and restart/equivocation regressions implemented; supported restore is observer-only.


## BL-012: Confirm account-chain dependencies

- [x] Complete

**Dependencies:** BL-011.

**Scope:** internal/consensus/, internal/ledger/.

**Acceptance:** Implement accepted vote/weight/quorum rules; finality includes required dependencies and cannot be replaced.

**Verification:** Model trace conformance, replay, partitions and dependency tests; make check.

**Implementation evidence:** Two-phase certificates, lock persistence, late proofs, round catchup and dependency validation implemented.


## BL-013: Define bounded network admission

- [x] Complete

**Dependencies:** BL-005, BL-006.

**Scope:** docs/protocol.md, docs/architecture.md, docs/open-questions.md.

**Acceptance:** Resolve Q-03; specify framing, versioning, peer identity, queue limits and overload/fairness behavior.

**Verification:** Malformed-input and flood workload design; resource budget review before transport coding.

**Implementation evidence:** Configured peers, proof-of-work and bounded HTTP/queue byte/item budgets implemented; sustained economic fairness remains BL-020 qualification.


## BL-014: Exchange blocks between bounded peers

- [x] Complete

**Dependencies:** BL-012, BL-013.

**Scope:** internal/p2p/, internal/node/.

**Acceptance:** Nodes exchange valid blocks/votes with bounded workers and queues; readiness and shutdown reflect actual resource state.

**Verification:** Local multi-process exchange, slow-peer, overload and joined-shutdown scenarios; make check.

**Implementation evidence:** P2P, node lifecycle, bounded gossip and joined shutdown implemented; real HTTP and process scenarios added.


## BL-015: Bootstrap verified state

- [x] Complete

**Dependencies:** BL-014.

**Scope:** internal/p2p/, internal/ledger/, docs/operations.md.

**Acceptance:** Resolve Q-07; fetch bounded dependencies and verify state/finality under explicit trust rules.

**Verification:** Malicious frontier, missing dependency, interrupted bootstrap and fresh-node catch-up tests.

**Implementation evidence:** Certified replay from explicit genesis implemented; restored observer catches up after further confirmations.


## BL-016: Expose versioned payment RPC

- [x] Complete

**Dependencies:** BL-012.

**Scope:** internal/rpc/, internal/node/.

**Acceptance:** Submission, lookup and history distinguish admission/durability/confirmation; pagination, errors, limits and admin access are specified.

**Verification:** Contract tests for uncertain submission, retries, malformed requests and authorization; make check.

**Implementation evidence:** Versioned raw JSON RPC, bearer writes/admin, bounded pagination, explicit statuses and retry semantics implemented.


## BL-017: Create and restore local wallet identities

- [x] Complete

**Dependencies:** BL-005.

**Scope:** internal/wallet/, cmd/.

**Acceptance:** Encrypted local keys, derivation, backup and restore follow specified formats; secrets stay out of logs.

**Verification:** Known derivation vectors, wrong-password, backup round-trip and failure tests; make check.

**Implementation evidence:** Encrypted deterministic wallet custody, exclusive private file creation and backup/restore implemented.


## BL-018: Deliver CLI payment workflows

- [x] Complete

**Dependencies:** BL-016, BL-017.

**Scope:** cmd/, internal/wallet/.

**Acceptance:** Users sign offline, send, receive, delegate and inspect status through node RPC without exposing spending keys to the node.

**Verification:** Packaged CLI workflow tests including timeout/lookup/retry; make check.

**Implementation evidence:** Wallet/control/node binaries implement online/offline flows; durable outbox preserves uncertain intent.


## BL-019: Verify backup, migration and operator visibility

- [x] Complete

**Dependencies:** BL-015, BL-016.

**Scope:** internal/node/, internal/storage/, docs/operations.md.

**Acceptance:** Operators can diagnose sync/quorum/storage states, restore consistent data and perform supported upgrades.

**Verification:** Restore drill, interrupted migration, disk failure and readiness tests; split scenarios into separate tasks before implementation.

**Implementation evidence:** Readiness/metrics, consistent backups, certified restore and compatible-upgrade procedures implemented. Injected descriptor failure checks fail-closed I/O; real hardware power-loss/full-disk drills remain release qualification.


## BL-020: Demonstrate adversarial multi-node operation

- [ ] Complete

**Dependencies:** BL-018, BL-019.

**Scope:** tests/, docs/quality.md, docs/open-questions.md.

**Acceptance:** Resolve Q-08; measure agreed workloads, capacity and sustained operation with reproducible fault schedules.

**Verification:** Transfer/conservation, conflicts, partitions, restarts and bounded-resource evidence from packaged processes.

**Implementation evidence:** Implemented baseline: adversarial partition/locked restart test and generated-key multi-process scenario. Still pending sustained hostile load, measured hardware budgets and long-duration operation.


## BL-021: Prepare product release evidence

- [ ] Complete

**Dependencies:** BL-020.

**Scope:** docs/quality.md, docs/operations.md, release/.

**Acceptance:** Resolve Q-04 distribution, Q-09 and Q-10; complete independent review, packaging, license inventory and all release requirements.

**Verification:** Clean install, checksums, upgrade/restore and full workflow evidence; public deployment remains a separate action.

**Implementation evidence:** Implemented baseline: reproducible archives, checksums, manifests, version output and license. External independent review, public distribution and public governance/launch decisions remain outstanding.


## BL-022: Test all packages and executable workflows

- [x] Complete

**Dependencies:** BL-019.

**Scope:** cmd/, internal/, tests/network_smoke.py.

**Acceptance:** Reproduce observed behavioral failures, commit each change independently,
and run the full package, concurrency, RPC, wallet, network and packaging checks.

**Verification:** All13 packages pass vet and race tests. Final instrumented and packaged
network scenarios pass. Five fuzz targets pass1350245 inputs. Four platform archives
reproduce byte-for-byte. Combined measured statement coverage is75.7%.

**Implementation evidence:** Twelve dedicated behavioral commits plus separate test
coverage commits.


## BL-023: Audit decision outcomes across the implementation

- [x] Complete

**Dependencies:** BL-022.

**Scope:** cmd/, internal/, scripts/branch_coverage.py.

**Acceptance:** Inventory decision outcomes, exercise reachable residuals, and verify
invariant-excluded and fault-injection-dependent paths without coverage reports in docs.

**Verification:** Isolated decision measurement plus ordinary-source make check and
real-process network workflow. Each observed behavioral correction has a separate commit.

**Implementation evidence:** make check and the strict failure/network gate pass.
Unobserved structural outcomes require exact function-specific invariant contracts;
unknown gaps fail the gate. Faults and numeric boundary overrides exist only in
disposable source copies. Verification reports were removed at the user’s request.


## Authorized product extensions

Ordered work from the accepted eight-direction plan. Each row is a completion task;
its outcome is its acceptance criterion. Later interfaces consume earlier contracts.
Each increment runs focused checks; integration boundaries run make check. No reports
are added to docs. BL-020/BL-021 keep their independent release requirements.

### Wallet

| ID | Complete | Acceptance | Verification |
| --- | --- | --- | --- |
| BL-024 | [x] | Password rotation atomically preserves derived identities | Focused Go tests and race checks |
| BL-025 | [ ] | Password command accepts old and new secrets through files or environment | Focused Go tests and race checks |
| BL-026 | [ ] | Interrupted password replacement retains a readable wallet | Focused Go tests and race checks |
| BL-027 | [ ] | Private address book persists bounded validated contacts | Focused Go tests and race checks |
| BL-028 | [ ] | Contact commands add list and remove named recipients | Focused Go tests and race checks |
| BL-029 | [ ] | Named contacts resolve send destinations with network pins | Focused Go tests and race checks |
| BL-030 | [ ] | Contact import and export preserve explicit merge rules | Focused Go tests and race checks |
| BL-031 | [ ] | Contact corruption and concurrent updates fail safely | Focused Go tests and race checks |
| BL-032 | [ ] | Account labels persist deterministic account indexes | Focused Go tests and race checks |
| BL-033 | [ ] | Account-name flags resolve indexes without ambiguous overrides | Focused Go tests and race checks |
| BL-034 | [ ] | Outbox command inspects exact durable signed intent | Focused Go tests and race checks |
| BL-035 | [ ] | Outbox retry resubmits the original signed block | Focused Go tests and race checks |
| BL-036 | [ ] | Confirmed outbox cleanup checks confirmation under the account lock | Focused Go tests and race checks |
| BL-037 | [ ] | Wallet command workflows document and exercise new custody operations | Focused Go tests and race checks |

### Payments

| ID | Complete | Acceptance | Verification |
| --- | --- | --- | --- |
| BL-038 | [ ] | Canonical payment URIs carry address amount and explicit network | Focused Go tests and race checks |
| BL-039 | [ ] | Request command creates payment URIs for an account | Focused Go tests and race checks |
| BL-040 | [ ] | Request inspection validates untrusted payment URIs | Focused Go tests and race checks |
| BL-041 | [ ] | Send accepts network-pinned payment URIs | Focused Go tests and race checks |
| BL-042 | [ ] | Versioned payment request files reject unknown or oversized input | Focused Go tests and race checks |
| BL-043 | [ ] | Send accepts validated payment request files | Focused Go tests and race checks |
| BL-044 | [ ] | Payment URI adversarial inputs and fuzz seeds are exercised | Seed tests and bounded fuzz campaign |
| BL-045 | [ ] | Receive-all confirms bounded receivables sequentially | Focused Go tests and race checks |
| BL-046 | [ ] | Receive-all resumes safely after partial failure | Focused Go tests and race checks |
| BL-047 | [ ] | Real process payments exercise request and receive-all commands | Owned process scenario and cleanup checks |

### RPC

| ID | Complete | Acceptance | Verification |
| --- | --- | --- | --- |
| BL-048 | [ ] | Cursor tokens bind canonical page scope and network | Focused Go tests and race checks |
| BL-049 | [ ] | Ledger page reads capture one consistent confirmed view | Focused Go tests and race checks |
| BL-050 | [ ] | History page endpoint supports stable continuation | Focused Go tests and race checks |
| BL-051 | [ ] | Receivables page endpoint detects stale continuation | Focused Go tests and race checks |
| BL-052 | [ ] | Client exposes typed cursor page operations | Focused Go tests and race checks |
| BL-053 | [ ] | Cursor misuse and stale snapshot responses are verified | Focused Go tests and race checks |
| BL-054 | [ ] | Bounded batch reads return independent typed outcomes | Focused Go tests and race checks |
| BL-055 | [ ] | Client batches account block and status reads | Focused Go tests and race checks |
| BL-056 | [ ] | Confirmation wait endpoint has bounded deadlines | Focused Go tests and race checks |
| BL-057 | [ ] | Client exposes server-assisted confirmation waits | Focused Go tests and race checks |
| BL-058 | [ ] | Confirmation waits cancel without retaining handlers | Focused Go tests and race checks |
| BL-059 | [ ] | Public Go client exposes reusable payment interfaces | Focused Go tests and race checks |

### Events

| ID | Complete | Acceptance | Verification |
| --- | --- | --- | --- |
| BL-060 | [ ] | Ledger signals only durable confirmed state changes | Focused Go tests and race checks |
| BL-061 | [ ] | SSE streams confirmed records with bounded admission | Focused Go tests and race checks |
| BL-062 | [ ] | SSE replay validates network-bound event identifiers | Focused Go tests and race checks |
| BL-063 | [ ] | Streams bound heartbeat writes and subscriber resources | Focused Go tests and race checks |
| BL-064 | [ ] | Client parses bounded SSE frames and cancellation | Focused Go tests and race checks |
| BL-065 | [ ] | Client reconnects with bounded backoff and resumes delivered events | Focused Go tests and race checks |
| BL-066 | [ ] | Wallet watch prints confirmed events without exposing keys | Focused Go tests and race checks |
| BL-067 | [ ] | Incoming event filters select account transfers | Focused Go tests and race checks |
| BL-068 | [ ] | Slow subscribers and disconnected streams release capacity | Focused Go tests and race checks |
| BL-069 | [ ] | Restarted event streams replay persistent confirmation heights | Focused Go tests and race checks |

### Peers

| ID | Complete | Acceptance | Verification |
| --- | --- | --- | --- |
| BL-070 | [ ] | Peer retry delay uses bounded reproducible jitter | Focused Go tests and race checks |
| BL-071 | [ ] | Peer workers back off failures and reset after success | Focused Go tests and race checks |
| BL-072 | [ ] | Peer diagnostics expose bounded connection and queue state | Focused Go tests and race checks |
| BL-073 | [ ] | Operator peers command queries authenticated diagnostics | Focused Go tests and race checks |
| BL-074 | [ ] | Peer diagnostics record response latency and last success | Focused Go tests and race checks |
| BL-075 | [ ] | Outbound consensus messages receive bounded priority admission | Focused Go tests and race checks |
| BL-076 | [ ] | Per-peer delivery preserves priority and ordinary message fairness | Focused Go tests and race checks |
| BL-077 | [ ] | Outbound burst deduplication retains periodic repair | Focused Go tests and race checks |
| BL-078 | [ ] | Accepted inbound duplicates avoid repeated verification work | Focused Go tests and race checks |
| BL-079 | [ ] | Peer metrics expose queue and retry budgets without identity labels | Focused Go tests and race checks |
| BL-080 | [ ] | Slow-peer fairness and retry recovery are exercised | Focused Go tests and race checks |
| BL-081 | [ ] | Peer capability endpoint declares supported bounded transports | Focused Go tests and race checks |

### Performance

| ID | Complete | Acceptance | Verification |
| --- | --- | --- | --- |
| BL-082 | [ ] | Protocol and certificate benchmarks establish reproducible workloads | Benchmarks and invariant tests |
| BL-083 | [ ] | History and receivable benchmarks separate ledger query costs | Benchmarks and invariant tests |
| BL-084 | [ ] | Ledger caches its immutable network identity | Benchmarks and invariant tests |
| BL-085 | [ ] | Ledger validates block identity without redundant hashing | Benchmarks and invariant tests |
| BL-086 | [ ] | Account history indexes avoid whole-ledger scans | Benchmarks and invariant tests |
| BL-087 | [ ] | Receivable destination indexes avoid unrelated transfers | Benchmarks and invariant tests |
| BL-088 | [ ] | Receivable ordering updates incrementally with confirmed state | Benchmarks and invariant tests |
| BL-089 | [ ] | Page allocations respect requested and byte-bounded limits | Benchmarks and invariant tests |
| BL-090 | [ ] | Confirmed delegation weights update incrementally | Benchmarks and invariant tests |
| BL-091 | [ ] | Consensus health reads one consistent ledger status | Benchmarks and invariant tests |
| BL-092 | [ ] | Work generation reuses fixed-size hashing buffers | Benchmarks and invariant tests |
| BL-093 | [ ] | Index and allocation regressions compare against reference results | Benchmarks and invariant tests |

### Operations

| ID | Complete | Acceptance | Verification |
| --- | --- | --- | --- |
| BL-094 | [ ] | Node validates configuration without creating runtime state | Focused Go tests and race checks |
| BL-095 | [ ] | Operator doctor emits redacted local diagnostics | Focused Go tests and race checks |
| BL-096 | [ ] | Authenticated profiles bound collection time and retained bytes | Focused Go tests and race checks |
| BL-097 | [ ] | Operator profile command writes bounded private downloads | Focused Go tests and race checks |
| BL-098 | [ ] | Container image runs the node as an unprivileged user | Owned process scenario and cleanup checks |
| BL-099 | [ ] | Systemd service declares filesystem and process restrictions | Owned process scenario and cleanup checks |
| BL-100 | [ ] | Release archives include reproducible build provenance | Focused Go tests and race checks |
| BL-101 | [ ] | Client and operator expose readiness probes | Focused Go tests and race checks |
| BL-102 | [ ] | Live doctor checks listeners and runtime capacity explicitly | Focused Go tests and race checks |
| BL-103 | [ ] | Container smoke tooling owns and cleans its generated runtime | Owned process scenario and cleanup checks |

### Resilience

| ID | Complete | Acceptance | Verification |
| --- | --- | --- | --- |
| BL-104 | [ ] | Process harness owns nodes temporary secrets and deadlines | Owned process scenario and cleanup checks |
| BL-105 | [ ] | Controllable peer proxy supports deterministic partitions | Focused Go tests and race checks |
| BL-106 | [ ] | Partitioned representatives wait for quorum and converge after healing | Focused Go tests and race checks |
| BL-107 | [ ] | Killed unconfirmed submissions retain reusable outbox intent | Focused Go tests and race checks |
| BL-108 | [ ] | Confirmed process restarts conserve funds and catch up | Owned process scenario and cleanup checks |
| BL-109 | [ ] | Slow HTTP peers cannot consume unbounded queues or workers | Focused Go tests and race checks |
| BL-110 | [ ] | Oversized process-level peer requests fail within input budgets | Owned process scenario and cleanup checks |
| BL-111 | [ ] | Quorum loss changes readiness while preserving confirmed reads | Focused Go tests and race checks |
| BL-112 | [ ] | Concurrent wallet processes preserve account exclusion | Owned process scenario and cleanup checks |
| BL-113 | [ ] | Process event reconnection resumes without missing confirmed records | Owned process scenario and cleanup checks |
| BL-114 | [ ] | Payment request fuzzing bounds parsing and canonicalization | Seed tests and bounded fuzz campaign |
| BL-115 | [ ] | Cursor fuzzing bounds malformed continuation input | Seed tests and bounded fuzz campaign |
| BL-116 | [ ] | Event parser fuzzing bounds partial and oversized frames | Seed tests and bounded fuzz campaign |
| BL-117 | [ ] | Randomized index checks preserve transfer and weight invariants | Focused Go tests and race checks |
| BL-118 | [ ] | Process interruption checks journal and signing recovery | Owned process scenario and cleanup checks |
| BL-119 | [ ] | Packaged extended workflows pass the complete development gates | Owned process scenario and cleanup checks |
