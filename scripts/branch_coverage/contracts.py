"""Exact decision exclusions backed by runtime invariants, not observed hit counts.

The raw measurement retains these as unobserved. Strict checking rejects any other
missing outcome. Changes to preconditions must revisit the corresponding contract.
"""
EXCLUDED = {}

# Function context prevents a similarly named condition elsewhere from silently
# inheriting an exclusion when new code is added.
def functions(file, kind, expression):
    if file == "consensus/engine.go": return ["Health"]
    if file == "consensus/recovery.go": return ["validateRecovered"]
    if file == "consensus/voting.go":
        return ["verifyProof" if expression == "p.Proof" else "acceptVote" if expression in {"e.ahead", "other.Votes"} else "drive"]
    if file == "control/control.go": return ["RunControl"]
    if file == "wallet/transaction.go": return ["createPayment"]
    if file == "ledger/state.go":
        return ["newState" if expression == "g.Allocations" else "weights" if expression == "s.accounts" else "representativeCapacity" if expression == "weights" else "validate"]
    if file == "ledger/types.go": return ["certificate"]
    if file == "node/config.go":
        return ["LoadConfig" if expression.startswith("[]*string") else "loadRepresentative" if expression == "seed" else "Validate"]
    if file == "node/run.go": return ["Run"]
    if file == "p2p/network.go":
        return ["ValidatePeer" if kind == "if" and expression.startswith("strings.ContainsAny") else "allow" if expression == "n.rates" else "syncPeer" if expression == "records" else "records"]
    if file == "protocol/genesis.go": return ["Validate" if expression == "g.Allocations" else "ID"]
    if file == "protocol/types.go": return ["ParseAmount"]
    if file == "rpc/server.go":
        return ["history", "receivables"] if expression == "items == nil" else ["ready" if expression == "ok" else "metrics"]
    raise ValueError("missing function context")


def exclude(file, kind, expression, outcomes, reason):
    for function in functions(file, kind, expression):
        for outcome in outcomes:
            key = ("internal/" + file, function, kind, expression, outcome)
            if key in EXCLUDED:
                raise ValueError("duplicate decision contract")
            EXCLUDED[key] = reason


for file in ("consensus/engine.go", "consensus/voting.go"):
    for expression in ("phases", "vs"):
        exclude(file, "range", expression, ["empty"],
                "remember inserts round, phase and vote under the engine lock; pruning removes entire rounds")
for expression in ("e.ahead", "other.Votes"):
    exclude("consensus/voting.go", "range", expression, ["empty"],
            "acceptVote inserts this representative and its phase before collecting future-round evidence")
exclude("consensus/voting.go", "range", "p.Proof", ["empty"],
        "verifyProof rejects a zero-length nonnegative-round proof before iterating")
exclude("consensus/recovery.go", "range", "[]*protocol.Block{s.Locked, s.Valid}", ["empty"],
        "fixed two-element literal; nullable elements do not change its length")
for file in ("ledger/state.go", "protocol/genesis.go"):
    exclude(file, "range", "g.Allocations", ["empty"],
            "Genesis.Validate requires positive supply and 1..4096 allocations before state construction")
exclude("protocol/genesis.go", "range", "a", ["empty"],
        "ID copies the allocations only after Genesis.Validate succeeds")
for expression in ("s.accounts", "weights"):
    exclude("ledger/state.go", "range", expression, ["empty"],
            "positive-supply state never deletes accounts; representativeCapacity adds the target representative before counting")
exclude("ledger/types.go", "range", "r.Votes", ["empty"],
        "certificate rejects empty certificates before summing; rejection tests cover that precondition")
exclude("ledger/state.go", "switch", "b.Kind", ["default"],
        "Block.Verify rejects unknown kinds before transition dispatch")
exclude("control/control.go", "switch", "cmd", ["default"],
        "the first command switch rejects unknown commands; cmd is never reassigned")
exclude("wallet/transaction.go", "switch", "command", ["default"],
        "the flag switch rejects unknown commands and all non-payment commands return before payment dispatch")
for expression in ("[]*string{&cfg.DataDir, &cfg.Genesis, &cfg.RepresentativeKey}",
                   "[]string{c.RPCListen, c.P2PListen}"):
    exclude("node/config.go", "range", expression, ["empty"], "fixed nonempty literal")
exclude("node/config.go", "range", "port", ["empty"],
        "Validate rejects an empty listener port before its numeric and digit checks")
exclude("node/config.go", "if", "digit < '0' || digit > '9'", ["true"],
        "strconv.ParseUint(port, 10, 16) has already rejected every non-ASCII decimal digit and sign")
exclude("node/config.go", "range", "seed", ["empty"],
        "loadRepresentative requires exactly ed25519.SeedSize decoded bytes before deriving and clearing")
exclude("node/run.go", "range", "[]*http.Server{rpcServer, peerServer}", ["empty"],
        "fixed two-server shutdown group")
exclude("node/run.go", "select", "", ["default"],
        "four workers each attempt at most one send into a four-slot channel; the owner can only remove entries")
exclude("p2p/network.go", "if", 'strings.ContainsAny(u.Host, "\\r\\n")', ["true"],
        "url.Parse rejects control bytes in the URL before returning a host; origin rejection tests cover them")
exclude("p2p/network.go", "if", "records == nil", ["true"],
        "Store.Records always returns an allocated slice, including empty pages")
exclude("p2p/network.go", "if", "len(encoded) > maxResponse-len(raw)-overhead", ["true"],
        "maximum certificate fits 16MiB; Store.Records caps aggregate pages at 28MiB, below the 32MiB response limit")
exclude("p2p/network.go", "if", "len(raw) == 1", ["false", "true"],
        "enclosing transport-overflow guard excluded by the certified-page size contract")
exclude("p2p/network.go", "range", "records", ["empty"],
        "syncPeer rejects zero-length record responses before entering the replay loop")
exclude("p2p/network.go", "range", "n.rates", ["empty"],
        "rate-map pruning runs only with >=4096 entries while holding the only map mutation lock")
exclude("protocol/types.go", "range", "s", ["empty"],
        "ParseAmount rejects the empty string before scanning digits")
for expression in ("items == nil",):
    exclude("rpc/server.go", "if", expression, ["true"],
            "History and Receivables return allocated empty slices; array-shape contract is tested")
exclude("rpc/server.go", "if", "ok", ["false"],
        "ready asserts the fixed bool field supplied by Engine.Health; schema contract is tested")
exclude("rpc/server.go", "type_switch", "value := health[name].(type)", ["default"],
        "all four Engine.Health metric fields have tested int, uint32 or Amount types")
for expression in ('[]string{"pending", "round", "observed_voting_weight", "quorum_weight"}',
                   '[]string{"peers", "connected", "dropped", "received", "queued_bytes"}'):
    exclude("rpc/server.go", "range", expression, ["empty"], "fixed nonempty metric-name literal")


def exclusion(site, outcome):
    return EXCLUDED.get((site["file"], site["function"], site["kind"], site["expression"], outcome))

# Additive product contracts retain the raw unobserved outcomes. These guards
# are structural consequences of the preceding validation in the same function.
for file, function, kind, expression, outcome, reason in [
 ("internal/client/batch.go", "validReadID", "range", "id", "empty", "the preceding length guard returns for an empty ID"),
 ("internal/client/batch.go", "Batch", "range", "queries", "empty", "Batch rejects zero queries before validating IDs"),
 ("internal/client/batch.go", "Batch", "range", "rows", "empty", "row count must equal the already nonempty query count"),
 ("internal/client/options.go", "WithUserAgent", "range", "agent", "empty", "the preceding byte-length guard rejects empty agents"),
 ("internal/rpc/batch.go", "validQueryID", "range", "id", "empty", "the preceding length guard rejects empty IDs"),
 ("internal/rpc/batch.go", "batchRead", "range", "request.Queries", "empty", "batchRead rejects empty query arrays immediately after decoding"),
 ("internal/p2p/dedup.go", "remember", "range", "c.entries", "empty", "pruning runs only with at least maxBurstEntries under the cache mutex"),
 ("internal/ledger/state.go", "weights", "range", "s.delegated", "empty", "positive supply remains delegated to at least one representative after every checked transition"),
 ("internal/ledger/state.go", "apply", "range", "s.delegated", "empty", "positive supply remains delegated to at least one representative; apply updates never remove nonzero total weight"),
]:
 EXCLUDED[(file,function,kind,expression,outcome)] = reason
EXCLUDED[("internal/rpc/server.go", "metrics", "range", '[]string{"peers", "connected", "dropped", "received", "queued_bytes", "queued_messages", "retrying", "deduplicated", "queue_byte_budget", "queue_item_budget"}', "empty")] = "fixed nonempty aggregate metric-name literal"

EXCLUDED[("internal/wallet/profile.go", "validName", "range", "name", "empty")] = "validName rejects empty names in the preceding length guard"
