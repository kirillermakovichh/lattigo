# Development workflow

Go 1.26 is the initial local baseline. The module path `example.com/go-block-lattice`
is deliberately provisional; it names no claimed GitHub organization or public repository.
Choose the canonical path before exposing imports to external consumers.

## Commands

- make check: check formatting, vet, tests, race detection and documentation links.
- make build: build existing packages.
- make docs-check: verify relative Markdown links outside .git and generated directories.

The bootstrap has no Go packages. Go targets report that fact instead of claiming tests ran.
Build caches are local to .cache/ and excluded from commits.
CI runs the same baseline; inspect actual CI results after a remote is configured.

## Change workflow

Select one task and inspect its dependencies. Read the relevant authoritative contracts.
Clarify consensus blockers before implementing dependent code. Implement a coherent behavior
slice with failure tests and deterministic vectors where appropriate.
Run applicable checks. Update task status and implementation-progress.md with actual evidence.
Review the staged diff. Commit with a concise Conventional Commit message.
Do not combine unrelated features or accumulate a whole product in one commit.

## Layout

Create cmd/, internal/, tests/ and testdata/ only when they contain real maintained behavior.
Keep package tests alongside source. Put local runtime data in .local/ and binaries in bin/.
Do not create src/, pkg/ or a shared utility package without an explicit need.
Do not commit generated caches, coverage, profiles, keys or production data.

## Dependencies and tooling

Prefer the standard library where it satisfies the contract. Record the purpose and
license of new runtime dependencies. Keep go.mod/go.sum tidy when dependencies exist.
Do not pin a database or cryptographic package before the relevant decision and vectors.
Add specialized lint, fuzz or benchmark tools when real code requires them.
The check script is repository infrastructure, not part of the shipped Go runtime.

## Documentation ownership

Update the contract at its authoritative location in docs/README.md.
Use proposals for unapproved protocol choices. Do not duplicate checklists across docs.
Keep the handoff concise: current state, verification, blockers and next task.
