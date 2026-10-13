---
status: accepted
---
# Use explicit standard-library formats and bounded journal storage

Use standard Ed25519 and SHA-256 with independent domains rather than reproduce
Nano-specific cryptographic formats. Use exact uint64 raw units instead of uint128;
network allocation, maximum supply and overflow behavior are explicit in genesis.
Wallet encryption uses PBKDF2-HMAC-SHA256 and AES-256-GCM with versioned parameters.
The Go runtime has no third-party module dependencies.

Choose a checksummed append-only WAL with certified replay, fsync and exclusive Flock
rather than embed SQLite or bbolt. This keeps the durability format auditable with the
standard library but requires complete replay and indexed resident history. Bound that
history explicitly; do not pretend pagination or WAL alone provides unlimited scaling.
A database-backed/pruned implementation would need a reviewed migration and equivalent
certificate, backup and crash guarantees. The current supported storage platforms are
Linux and macOS. Unsupported schemas fail closed rather than silently migrate.

Use MIT for the project source. Imported development-skill notices remain separate;
those skills are not runtime dependencies. A real remote is needed before choosing a
public module path. No public network or funded distribution is created by this decision.
