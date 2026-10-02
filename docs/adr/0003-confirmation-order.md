---
status: accepted
---
# Serialize confirmation to define delegated weights

Account chains remain independently signed. A weighted, rotating-proposer, two-phase
BFT protocol orders confirmed blocks globally. Each height uses the preceding confirmed
ledger's delegated weights, including outgoing receivables, so tokens never vote twice
within a height. More than two thirds of supply must precommit the same candidate.
Round locks and prevote certificates govern re-proposals. Voting state is durable before
publication. This trades Nano-like fully asynchronous elections for a simpler explicit
weight transition contract. Safety assumes less than one third Byzantine voting weight;
liveness requires eventual synchrony, reachable honest quorum and healthy signing storage.
These are protocol assumptions, not claims of an externally reviewed proof.
