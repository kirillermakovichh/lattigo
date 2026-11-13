package ledger

import (
	"crypto/ed25519"
	"math/rand/v2"
	"reflect"
	"sort"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func TestRandomizedCertifiedReplayPreservesIndexes(t *testing.T) {
	initial, k, l := fixture(t)
	g := initial.genesis
	keys := []ed25519.PrivateKey{k, l}
	accounts := []protocol.Account{protocol.Address(k.Public().(ed25519.PublicKey)), protocol.Address(l.Public().(ed25519.PublicKey))}
	directory := t.TempDir()
	store, err := Open(directory, g)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	rng := rand.New(rand.NewPCG(117, 4096))
	restarts := 0
	for step := 0; step < 150; step++ {
		which := rng.IntN(2)
		a := accounts[which]
		row, exists := store.state.accounts[a]
		b := protocol.Block{Version: 1, Network: g.ID(), Account: a, Previous: row.Frontier, Representative: accounts[0], Balance: row.Balance}
		sources := []protocol.Hash{}
		for id, r := range store.state.receivables {
			if r.To == a {
				sources = append(sources, id)
			}
		}
		sort.Slice(sources, func(i, j int) bool { return sources[i] < sources[j] })
		if len(sources) > 0 && (rng.IntN(2) == 0 || !exists || row.Balance == 0) {
			b.Kind = "receive"
			b.Link = string(sources[rng.IntN(len(sources))])
			b.Balance += store.state.receivables[protocol.Hash(b.Link)].Amount
		} else if exists && row.Balance > 0 {
			b.Kind = "send"
			b.Balance -= protocol.Amount(1 + rng.Uint64N(uint64(row.Balance)))
			b.Link = string(accounts[rng.IntN(2)])
		} else {
			continue
		}
		r := nextRecord(t, store.state, b, keys[which])
		r.Votes[0].Representative = accounts[0]
		if err := r.Votes[0].Sign(k); err != nil {
			t.Fatal(err)
		}
		if err := store.Commit(r); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		checkReferenceIndexes(t, store.state)
		if rng.IntN(5) == 0 || step == 149 {
			status := store.Status()
			history := store.History(accounts[0], 0, 1000)
			receivables := store.Receivables(accounts[1], 0, 1000)
			weights := store.Weights()
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = OpenExisting(directory, g, Options{})
			if err != nil {
				t.Fatal(err)
			}
			restarts++
			if status != store.Status() || !reflect.DeepEqual(history, store.History(accounts[0], 0, 1000)) || !reflect.DeepEqual(receivables, store.Receivables(accounts[1], 0, 1000)) || !reflect.DeepEqual(weights, store.Weights()) {
				t.Fatal("certified replay changed confirmed query results")
			}
			checkReferenceIndexes(t, store.state)
		}
	}
	if restarts < 10 {
		t.Fatal("fixture did not exercise enough reopen boundaries")
	}
}
