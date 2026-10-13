# Nano research entrypoints

Status: source map, not a completed protocol audit. Recorded 2026-11-15.
Repository metadata and README were inspected during project selection. The linked protocol
pages and current C++ implementation still need a pinned source review before specification.

| Source | Investigation |
| --- | --- |
| https://github.com/nanocurrency/nano-node | Reference node implementation, license and versioned behavior |
| https://docs.nano.org/what-is-nano/overview/ | Account chains, asynchronous transfers and representative voting |
| https://docs.nano.org/protocol-design/introduction/ | Protocol terminology and design routes |
| https://docs.nano.org/protocol-design/orv-consensus/ | Voting, quorum, confirmation and implementation-version assumptions |
| https://docs.nano.org/integration-guides/the-basics/ | Integration status, receiving and operational expectations |
| https://nano.org/en/whitepaper | Historical design motivation; not sufficient as current implementation spec |

For each adopted behavior, record the source revision/version, exact rule, test vectors,
local differences and limitations. Inspect the current code when prose and historical papers differ.
Do not substitute generic Ed25519, arbitrary quorum thresholds or Bitcoin reorganization
rules for Nano-specific behavior while claiming compatibility.
Before copying implementation code, review its license and preserve attribution.

## Pinned comparison during implementation

Reference revision: `229e5a040a4a2c59ccc9d0d013fe8118e0c69ec4` (2026-11-14).
Inspected nano/lib/blocks.hpp and the beginning of nano/node/election.cpp at that
revision to compare dependency and election structure. This is a scoped comparison,
not a full audit. Nano exposes qualified-root elections, online representative tracking
and final cementing. This implementation instead uses a globally ordered two-phase
certificate sequence, fixed-supply quorum and standard Ed25519/SHA256 formats.
No Nano implementation code was copied into the runtime. The local independent design
is specified in protocol.md and does not inherit Nano's security claims or compatibility.
