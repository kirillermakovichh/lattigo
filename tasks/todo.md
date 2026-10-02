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

- [ ] Complete

**Dependencies:** BL-002.

**Scope:** docs/research/nano.md, docs/open-questions.md, docs/adr/0002-nano-compatibility.md.

**Acceptance:** Pin Nano code and docs; compare independent-network and compatibility scope; record the chosen scope and consequences.

**Verification:** Trace each adopted rule to a pinned source; review ADR status. Resolve Q-01.

## BL-005: Specify canonical blocks and signing

- [ ] Complete

**Dependencies:** BL-004.

**Scope:** docs/protocol.md, docs/open-questions.md, testdata/protocol/.

**Acceptance:** Resolve Q-06 and amount representation from Q-04; specify canonical bytes, domains, algorithms and positive/negative vectors.

**Verification:** Cross-check vectors against an independent implementation; reject noncanonical encodings.

## BL-006: Model representative safety and liveness

- [ ] Complete

**Dependencies:** BL-004.

**Scope:** docs/protocol.md, docs/open-questions.md, internal/consensus/.

**Acceptance:** Resolve Q-02; specify weight, election identity, quorum, final votes and signing recovery assumptions; create an executable model.

**Verification:** Replay partition, re-delegation, equivocation and stale-weight traces. Split model tasks before coding if needed.

## BL-007: Implement canonical protocol primitives

- [ ] Complete

**Dependencies:** BL-005.

**Scope:** internal/protocol/, testdata/protocol/.

**Acceptance:** Types, amount arithmetic, codec, hashes and signatures match the accepted vectors and reject malformed input.

**Verification:** Known-answer tests, overflow tests, fuzz seeds and make check.

## BL-008: Implement local account transitions

- [ ] Complete

**Dependencies:** BL-007.

**Scope:** internal/ledger/.

**Acceptance:** Send, receive/open and representative changes preserve supply and consume receivables once; duplicate and missing-dependency outcomes are explicit.

**Verification:** Deterministic transition and conflict tests through the ledger interface; make check.

## BL-009: Select durable storage contract

- [ ] Complete

**Dependencies:** BL-008.

**Scope:** docs/architecture.md, docs/open-questions.md, docs/adr/.

**Acceptance:** Resolve Q-05 with atomicity, locking, schema, corruption, backup and migration comparison.

**Verification:** Focused durability experiment and recorded decision; no unsupported durability claims.

## BL-010: Persist a payment across restart

- [ ] Complete

**Dependencies:** BL-009.

**Scope:** internal/storage/, internal/ledger/.

**Acceptance:** A complete transfer survives restart; account frontiers, receivables and delegation indexes change atomically.

**Verification:** Process-kill and interrupted-transaction tests; reopen and verify conservation; make check.

## BL-011: Prevent conflicting representative signatures

- [ ] Complete

**Dependencies:** BL-006, BL-010.

**Scope:** internal/consensus/, internal/storage/.

**Acceptance:** Persist signing intent before publication; restart cannot produce a prohibited conflicting vote; backup rollback policy is explicit.

**Verification:** Crash at each signing boundary and replay conflicting requests; make check.

## BL-012: Confirm account-chain dependencies

- [ ] Complete

**Dependencies:** BL-011.

**Scope:** internal/consensus/, internal/ledger/.

**Acceptance:** Implement accepted vote/weight/quorum rules; finality includes required dependencies and cannot be replaced.

**Verification:** Model trace conformance, replay, partitions and dependency tests; make check.

## BL-013: Define bounded network admission

- [ ] Complete

**Dependencies:** BL-005, BL-006.

**Scope:** docs/protocol.md, docs/architecture.md, docs/open-questions.md.

**Acceptance:** Resolve Q-03; specify framing, versioning, peer identity, queue limits and overload/fairness behavior.

**Verification:** Malformed-input and flood workload design; resource budget review before transport coding.

## BL-014: Exchange blocks between bounded peers

- [ ] Complete

**Dependencies:** BL-012, BL-013.

**Scope:** internal/p2p/, internal/node/.

**Acceptance:** Nodes exchange valid blocks/votes with bounded workers and queues; readiness and shutdown reflect actual resource state.

**Verification:** Local multi-process exchange, slow-peer, overload and joined-shutdown scenarios; make check.

## BL-015: Bootstrap verified state

- [ ] Complete

**Dependencies:** BL-014.

**Scope:** internal/p2p/, internal/ledger/, docs/operations.md.

**Acceptance:** Resolve Q-07; fetch bounded dependencies and verify state/finality under explicit trust rules.

**Verification:** Malicious frontier, missing dependency, interrupted bootstrap and fresh-node catch-up tests.

## BL-016: Expose versioned payment RPC

- [ ] Complete

**Dependencies:** BL-012.

**Scope:** internal/rpc/, internal/node/.

**Acceptance:** Submission, lookup and history distinguish admission/durability/confirmation; pagination, errors, limits and admin access are specified.

**Verification:** Contract tests for uncertain submission, retries, malformed requests and authorization; make check.

## BL-017: Create and restore local wallet identities

- [ ] Complete

**Dependencies:** BL-005.

**Scope:** internal/wallet/, cmd/.

**Acceptance:** Encrypted local keys, derivation, backup and restore follow specified formats; secrets stay out of logs.

**Verification:** Known derivation vectors, wrong-password, backup round-trip and failure tests; make check.

## BL-018: Deliver CLI payment workflows

- [ ] Complete

**Dependencies:** BL-016, BL-017.

**Scope:** cmd/, internal/wallet/.

**Acceptance:** Users sign offline, send, receive, delegate and inspect status through node RPC without exposing spending keys to the node.

**Verification:** Packaged CLI workflow tests including timeout/lookup/retry; make check.

## BL-019: Verify backup, migration and operator visibility

- [ ] Complete

**Dependencies:** BL-015, BL-016.

**Scope:** internal/node/, internal/storage/, docs/operations.md.

**Acceptance:** Operators can diagnose sync/quorum/storage states, restore consistent data and perform supported upgrades.

**Verification:** Restore drill, interrupted migration, disk failure and readiness tests; split scenarios into separate tasks before implementation.

## BL-020: Demonstrate adversarial multi-node operation

- [ ] Complete

**Dependencies:** BL-018, BL-019.

**Scope:** tests/, docs/quality.md, docs/open-questions.md.

**Acceptance:** Resolve Q-08; measure agreed workloads, capacity and sustained operation with reproducible fault schedules.

**Verification:** Transfer/conservation, conflicts, partitions, restarts and bounded-resource evidence from packaged processes.

## BL-021: Prepare product release evidence

- [ ] Complete

**Dependencies:** BL-020.

**Scope:** docs/quality.md, docs/operations.md, release/.

**Acceptance:** Resolve Q-04 distribution, Q-09 and Q-10; complete independent review, packaging, license inventory and all release requirements.

**Verification:** Clean install, checksums, upgrade/restore and full workflow evidence; public deployment remains a separate action.
