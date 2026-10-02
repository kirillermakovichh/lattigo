---
status: accepted
---
# Account-chain payment architecture

The user selected a Nano-like blockchain written in Go on 2026-11-15.
Use account-owned signed chains connected by send/receive dependencies rather than a
single mined chain. Representatives participate in confirmation without custody of delegated funds.
This choice exposes per-account concurrency but requires explicit cross-account dependency
rules, conflict elections and weight accounting. It does not settle Nano compatibility,
quorum parameters, cryptography, issuance or public-network launch policy.
The product target includes reliability and operations; development increments are not an MVP release target.
