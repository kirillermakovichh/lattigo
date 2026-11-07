package ledger

import (
	"crypto/ed25519"
	"math/rand/v2"
	"reflect"
	"sort"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func checkReferenceIndexes(t *testing.T, s *state) {
	t.Helper()
	weights := map[protocol.Account]protocol.Amount{}
	owned := map[protocol.Account]protocol.Amount{}
	for a, row := range s.accounts {
		owned[a] = row.Balance
		weights[row.Representative] += row.Balance
	}
	for _, r := range s.receivables {
		owned[r.From] += r.Amount
		weights[s.accounts[r.From].Representative] += r.Amount
	}
	for rep, w := range weights {
		if w == 0 {
			delete(weights, rep)
		}
	}
	if !reflect.DeepEqual(weights, s.weights()) || !reflect.DeepEqual(owned, s.owned) {
		t.Fatal("incremental weight view differs from reference")
	}
	store := &Store{state: s}
	for a := range s.accounts {
		var history []uint64
		for _, r := range s.records {
			if r.Block.Account == a {
				history = append(history, r.Height)
			}
		}
		if !reflect.DeepEqual(history, s.history[a]) {
			t.Fatal("history index differs from scan")
		}
		expected := []Receivable{}
		for _, r := range s.receivables {
			if r.To == a {
				expected = append(expected, r)
			}
		}
		sort.Slice(expected, func(i, j int) bool { return expected[i].Source < expected[j].Source })
		if got := store.Receivables(a, 0, 1000); !reflect.DeepEqual(got, expected) {
			t.Fatal("receivable index differs from scan")
		}
	}
	var total protocol.Amount
	for _, row := range s.accounts {
		total += row.Balance
	}
	for _, row := range s.receivables {
		total += row.Amount
	}
	if total != s.genesis.Supply {
		t.Fatal("supply changed")
	}
}

func TestIncrementalIndexesMatchRandomizedReference(t *testing.T) {
	s, k, l := fixture(t)
	keys := []ed25519.PrivateKey{k, l}
	accounts := []protocol.Account{protocol.Address(k.Public().(ed25519.PublicKey)), protocol.Address(l.Public().(ed25519.PublicKey))}
	rng := rand.New(rand.NewPCG(825, 93))
	for step := 0; step < 500; step++ {
		which := rng.IntN(2)
		a := accounts[which]
		row, exists := s.accounts[a]
		b := protocol.Block{Version: 1, Network: s.network, Account: a, Previous: row.Frontier, Representative: accounts[rng.IntN(2)], Balance: row.Balance}
		sources := []protocol.Hash{}
		for id, r := range s.receivables {
			if r.To == a {
				sources = append(sources, id)
			}
		}
		sort.Slice(sources, func(i, j int) bool { return sources[i] < sources[j] })
		if len(sources) > 0 && (rng.IntN(2) == 0 || !exists || row.Balance == 0) {
			b.Kind = "receive"
			b.Link = string(sources[rng.IntN(len(sources))])
			b.Balance += s.receivables[protocol.Hash(b.Link)].Amount
		} else if exists && row.Balance > 0 {
			b.Kind = "send"
			b.Balance -= protocol.Amount(1 + rng.Uint64N(uint64(row.Balance)))
			b.Link = string(accounts[rng.IntN(2)])
		} else {
			continue
		}
		r := nextRecord(t, s, b, keys[which])
		if err := s.validate(r.Block); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		s.apply(r)
		checkReferenceIndexes(t, s)
	}
	// Exported weight maps remain independent mutable copies.
	for rep := range s.weights() {
		w := s.weights()
		w[rep] = 0
		if s.weights()[rep] == 0 {
			t.Fatal("weight map aliases state")
		}
		break
	}
}
