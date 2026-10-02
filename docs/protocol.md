# Protocol design contract

Status: account-chain architecture is accepted by ADR-0001. Detailed wire, cryptographic
and voting rules are pending specification. This document states required invariants
and the questions that the normative specification must resolve.

## Ledger invariants

- Only the account owner authorizes a successor block.
- Each successor names its predecessor. An unopened account follows a separately specified opening rule.
- A send reduces balance by a positive exact amount and creates one destination-bound receivable.
- A receive consumes one matching receivable exactly once and increases balance by that amount.
- A representative-only change preserves balance and creates or consumes no receivable.
- Negative values, overflow, underflow and ambiguous block classifications are invalid.
- Total supply is conserved across account balances and outstanding receivables, except any explicitly specified genesis or issuance rule.
- Duplicate submission has the same identifier and no additional economic effect.
- Missing dependencies, invalid blocks and conflicting valid candidates are distinct outcomes.
- No descendant is confirmed while its required predecessor or source dependency remains unconfirmed.
- A confirmed outcome cannot be replaced by a local rollback or a later conflicting candidate.

Before implementation, specify which dependencies permit tentative admission and which
require confirmation. Unconfirmed state cannot be exposed as confirmed balance or weight.

## Block and signing specification checklist

The normative format must define field order, widths, byte order, version, account,
previous, representative, balance, operation/link interpretation and canonical rejection rules.
Specify block hashing, signature algorithm, signing preimage, network domain separation,
address checksum, amount width and text conversion. Publish positive and negative vectors.
Do not assume Go's default JSON encoding is a consensus encoding.

An anti-spam proof, if selected, has a separate validation policy and purpose from consensus.
Decide which fields it covers and whether it affects the block identifier.

## Representative voting

The accepted direction is delegated representative voting. Calling it ORV does not
establish equivalence with Nano's current implementation.
The normative design must resolve:

| Topic | Required contract |
| --- | --- |
| Election key | Which conflicting position is being decided; how opening conflicts are keyed |
| Weight | Confirmed-state source, pending funds, redelegation and account dependency ordering |
| Quorum | Threshold, denominator, rounding, online-weight estimation and initialization |
| Vote identity | Network domain, representative key, candidate, sequence/round and replay bounds |
| Re-voting | Whether tentative choices can change; distinction from irreversible final votes |
| Durability | Signing intent, publication order, crash recovery, backup rollback and key reuse |
| Finality | Dependency ordering, quorum evidence, durable confirmation and conflicting evidence |
| Partition | When progress must stop and conditions under which it resumes |

A static representative set or arbitrary two-thirds threshold must not silently replace
this contract. Safety assumptions and liveness assumptions must be stated separately.
A successful local election is not sufficient evidence for public-network Byzantine safety.

## Synchronization and compatibility

Specify bounded dependency retrieval, orphan expiry, conflict retrieval and peer penalties.
Bootstrap must validate blocks and confirmation evidence or name an explicit trust anchor.
A peer's claimed frontier or snapshot is not proof of finality.
Define genesis, network identifiers, protocol negotiation, unsupported-version errors,
upgrade activation and downgrade behavior before a shared test network.
Nano wire, signatures, addresses and ledger compatibility are undecided; see Q-01.
