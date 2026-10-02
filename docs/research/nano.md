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
