# Definition of Done

Every completed task meets its acceptance criteria and these applicable checks:

- Observable behavior and affected invariants are verified, including failure paths.
- Go changes pass formatting, vet, tests and race detection.
- Docs-only changes pass local link and consistency checks; no artificial behavior tests.
- Review deterministic validation, checked arithmetic and untrusted input where affected.
- Document resource bounds, ownership, recovery and compatibility where affected.
- Update authoritative contracts, task status and session handoff.
- Review the diff for secrets, unrelated edits and generated artifacts.
- Commit coherent verified increments. Never present bootstrap as release readiness.
- Product releases require the evidence in docs/quality.md.
