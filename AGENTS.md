# Project working rules

Reply in Russian. Use English for code, commit messages and maintained technical docs.
The user authorized repository initialization, documentation, regular local commits and Go development.
The target is a complete Nano-inspired payment network. Incremental delivery does not reduce release requirements.

## Start here

Read README.md, CONTEXT.md, docs/README.md and tasks/implementation-progress.md.
Inspect git status and tasks/todo.md before changing work.
Read relevant contracts and ADRs. Proposed rules are not accepted protocol requirements.

## Organization

- One Go module. Create packages only when behavior requires them.
- cmd/ contains thin entrypoints. internal/ contains private domain packages.
- Avoid utils, common, helpers, generic service layers and empty package scaffolding.
- Keep unit tests beside code. testdata/ contains fixtures; tests/ is for cross-process scenarios.
- docs/ owns maintained contracts. tasks/todo.md is the only completion checklist.
- CONTEXT.md contains domain terms only. docs/adr/ contains durable decisions.
- Never commit keys, seeds, peer identities or runtime databases.

## Agent skills

Use project-local skills from .agents/README.md instead of global duplicates.
Select architecture-foundation for runtime contracts; codebase-design for interfaces;
domain-modeling for vocabulary and decisions; planning-and-task-breakdown for plans;
context-engineering for handoffs; golang-patterns for Go changes.
User instructions override generic skill examples and approval suggestions.
Do not delegate to subagents unless the user explicitly asks.

## Engineering

Keep validation deterministic and independent of I/O, local time and scheduling.
Use exact integer amounts, checked arithmetic and canonical encodings.
Specify signature domains, hashes and vectors before protocol implementation.
Use reviewed cryptographic implementations. Do not invent cryptographic primitives.
Give every goroutine an owner, cancellation and a completion path.
Bound input, queues, work and retained bytes. Document overload behavior.
Define consumer interfaces when needed; do not mirror every concrete struct.
Do not claim Nano compatibility, Byzantine safety or throughput without evidence.
Do not create placeholder daemons, fake success paths or unfinished implementations.

## Verification and commits

Use make check and .agents/references/definition-of-done.md.
Commit coherent verified increments at completed task boundaries and before unrelated work.
Use Conventional Commits: chore, docs, feat, fix, test, refactor.
Stage explicit paths and review git diff --cached before committing.
Never rewrite user commits or discard user changes without authorization.
Record actual checks, task status and next work in the session handoff.
Local commits are already authorized. Remote creation, pushing and public deployment are separate actions.

## Decisions and work tracking

Treat Nano as design evidence, not an implicit compatibility commitment.
Keep open economic, protocol and network choices in docs/open-questions.md.
Resolve consensus-critical choices before dependent implementation.
Continue independent authorized work while choices remain open.
Local tasks use stable IDs in tasks/todo.md; no second issue tracker or duplicate checklist.
Read docs/research/nano.md for source limitations. Keep requirements, proposals and implementation distinct.
