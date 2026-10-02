# Open decisions

Status: unresolved. Owners below are decision roles, not assigned people.
Resolve each item with evidence and an ADR or normative contract before its dependency starts.

| ID | Decision | Blocks | Owner | Required evidence |
| --- | --- | --- | --- | --- |
| Q-01 | Independent protocol or Nano-compatible implementation? | Encoding, signatures, addresses, wire protocol | Product/protocol | Compatibility scope and cost; Nano vectors if compatibility is chosen |
| Q-02 | Voting, weight view, quorum and online-weight rules | Consensus implementation | Protocol | Written assumptions, adversarial traces and executable model |
| Q-03 | Anti-spam and fair admission for feeless payments | Public P2P and capacity promises | Protocol/runtime | Flood scenarios, resource measurements and fairness policy |
| Q-04 | Amount width, supply, genesis and distribution | Ledger genesis and public network | Product/protocol | Exact conservation model and reproducible genesis |
| Q-05 | Database and durability configuration | Durable ledger | Storage | Atomicity, crash tests, migration and backup comparison |
| Q-06 | Signatures, hashes, key derivation and address format | Codec and wallet | Protocol/security | Reviewed libraries, domains and cross-implementation vectors |
| Q-07 | Network bootstrap, trust anchors and representative discovery | Shared testnet | Protocol/operations | Recovery and eclipse scenarios; explicit trust model |
| Q-08 | Supported platforms, node hardware and resource budgets | Release acceptance | Product/operations | Measured workload and deployment matrix |
| Q-09 | Name, ticker, canonical module path and root license | Public distribution | Maintainer | Naming decision, remote destination and license selection |
| Q-10 | Upgrades and governance | Long-lived testnet and public release | Product/protocol | Activation, split, rollback and migration procedures |

Working assumptions for documentation only: an independent network, fixed-supply native
asset, delegated representative voting, local CLI wallet and feeless transfer intent.
These are not approved numeric parameters or compatibility guarantees.
The initialized local Go module path is provisional and must change before public imports.
