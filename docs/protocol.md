# Independent block-lattice protocol

Status: implemented protocol 1. This is an independent network, not a Nano-compatible
node. See ADR-0001, ADR-0002 and ADR-0003 for the architectural trade-offs.

## Networks, addresses and amounts

A network is identified by SHA-256 of its canonical genesis. Genesis specifies version1,
name (1..128 bytes), a positive uint64 supply, work difficulty0..24, and1..4096 distinct
positive allocations. Allocations must sum exactly to supply without overflow.
Each allocation names an account and representative. Allocations are sorted by account
before hashing; input JSON whitespace and order do not define network identity.
No public allocation, default funded secret or universal network is shipped.

Amounts are unsigned64-bit raw integer units. JSON amounts are canonical decimal
strings: no sign, fraction, leading zeros or numbers outside uint64.
An address is `bl_` followed by lowercase public-key hex (32 bytes), followed by four
checksum bytes in hex. Checksum is SHA256(`BL/address/v1` || public key)[:4].
Keys and signatures use the standard Ed25519 scheme from Go's standard library.
There is no Nano key-derivation or address compatibility.

## Canonical blocks

All integers below are big-endian. Zero previous/link is32 zero bytes.

| Field | Encoding |
| --- | --- |
| Domain | UTF-8 `BL/block/v1` |
| Version | uint8,1 |
| Network |32 decoded hash bytes |
| Kind | uint8: send1, receive2, change3 |
| Account |32 public-key bytes |
| Previous |32 hash bytes, or zeros for the first account block |
| Representative |32 public-key bytes |
| Balance | uint64 resulting account balance |
| Link |32 destination public-key bytes, source block hash, or zeros for change |

Block ID is SHA256 of those canonical bytes. The owner signs the32-byte decoded ID.
Signature and work are excluded from the ID. Signatures are64 bytes encoded lowercase hex.
Work is a uint64 nonce; SHA256(`BL/work/v1` || decoded ID || nonceBE) must have
at least genesis.work_bits leading zero bits. Work is admission cost, not voting weight.
Changing a signature or work does not create a different economic operation.
Unknown versions, kinds, invalid hashes, addresses and signatures are rejected.
See testdata/protocol/send.json for an independently calculated canonical/hash vector.

## Ledger transitions

All admission validation uses confirmed state. There is no speculative balance or weight.
The predecessor must equal the confirmed account frontier. An unopened account may only
receive with an empty predecessor. Genesis accounts start funded with an empty frontier.
A send strictly reduces balance and creates one receivable bound to the destination.
A receive consumes exactly one matching receivable and adds its exact amount.
A change preserves balance and changes representative. Send/receive state blocks also
specify the resulting representative. Every positive-weight representative must have a
valid address. The protocol permits at most16,384 positive-weight representatives;
a transition that exceeds the limit is rejected before voting.

The invariant is supply = sum(account balances) + sum(unconsumed receivables).
Each account delegates balance plus all its outgoing unconsumed receivables to its
current representative. Thus funds in transit keep voting weight, and total weight
remains exactly supply. Receiving moves that weight to the recipient's delegation.
Duplicate confirmed submission is idempotent. Missing dependencies, invalid operations,
stale/conflicting frontiers and capacity exhaustion have distinct outcomes.

## Globally ordered representative confirmation

Account chains remain separate signed histories. Confirmation deliberately uses one
ordered sequence so each height has one unambiguous pre-state weight map.
Height1 uses genesis weights and parent genesisID. Every later height uses the preceding
confirmed ledger and its record ID. A record contains one block and a precommit certificate.
RecordID = SHA256(`BL/record/v1` || heightBE64 || parent32 || blockID32).
Round and certificate are excluded from the record ID.

Representatives with positive weight are sorted lexically by address. Proposer index is
`((height-1) + round) mod representativeCount`, with overflow-safe modular arithmetic.
A proposer signs a domain-separated structured JSON proposal containing network,
height, round, parent, full block, valid_round, prevote proof and representative.
`signature` is empty in the signed JSON. Fixed field order comes from the Proposal type;
proof order is covered by the signature. Unknown input fields are rejected by transport.
This JSON signing format is part of protocol1; changing it requires a protocol change.

Votes use canonical fixed bytes: `BL/vote/v1`, network32, heightBE64, roundBE32,
phase byte(prevote1/precommit2), parent32, candidate32(or zeros for nil), representative32.
Ed25519 signs these bytes. Vote signatures bind phase, network, parent, height and round.
A quorum is strictly more than floor(2*supply/3), calculated without overflow.
A certificate includes each positive-weight representative once, at most16,384 votes.

An honest representative signs at most one candidate per height/round/phase.
On a valid proposal it prevotes that block when unlocked, locked on the same block,
or the proposal carries a valid prevote quorum at or after its lock round. Otherwise
it prevotes nil. A nonnil prevote quorum locks the current-round candidate before
precommitting. A nil quorum leads to nil precommit. A nonnil precommit quorum commits.
Confirmation never bypasses predecessor/source validation or reverses a confirmed record.

Locks persist across rounds. Late retained-round prevote certificates update the valid
candidate for later proposals but cannot authorize an old-round local precommit.
Timeouts advance voting phases and then rounds. Timeouts grow with round up to15 times
the configured base. More than one third authenticated weight observed in a future
round permits catchup while preserving locks. Very-future evidence retains only one
latest vote per representative; ordinary retained round state is bounded.
Dropped gossip is repaired by re-broadcast and verified record polling.

Safety assumes less than one third Byzantine weight at each confirmed height and
non-rollback signing storage. Liveness additionally requires eventual synchrony, an
honest reachable quorum, honest proposer opportunities, available capacity and valid work.
The executable regression/model scenarios are evidence for specific traces, not a formal
proof or external security audit. No online-weight estimate can lower the quorum.

## Durability and synchronization

A vote/proposal's local signing record is durable before publication. Lock updates are
durable before precommit. Committed records are fsynced before the confirmed view changes.
Replay independently checks every record, signature, dependency and quorum.
A restored ledger backup cannot reactivate a representative: restored state is observer-only.
Never clone a live representative or roll its data/signing directory backwards.

Peers must match network and protocol1. Bootstrap replays certified records from genesis;
peer status/frontier claims alone do not establish finality. A different genesis creates
a different network. Unknown protocol/schema versions fail closed. Compatible binary
upgrades preserve protocol1 and WAL1; an incompatible upgrade needs an explicit new
specification and migration, not automatic fallback or a silent change to genesis.
