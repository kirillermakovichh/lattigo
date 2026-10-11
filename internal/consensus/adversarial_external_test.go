package consensus_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"sort"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/consensus"
	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

// This trace uses only public interfaces. One proposer equivocates; honest nodes
// receive different proposals, two lock, one restarts, and an honest round recovers.
// It is an executable scenario, not an exhaustive BFT proof.
func TestEquivocatingProposerPartitionAndLockedRestart(t *testing.T) {
	keys := make([]ed25519.PrivateKey, 4)
	for i := range keys {
		keys[i] = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{byte(i + 21)}, 32))
	}
	sort.Slice(keys, func(i, j int) bool {
		return protocol.Address(keys[i].Public().(ed25519.PublicKey)) < protocol.Address(keys[j].Public().(ed25519.PublicKey))
	})
	accounts := make([]protocol.Account, 4)
	g := protocol.Genesis{Version: 1, Name: "external-adversarial", Supply: 400}
	for i, key := range keys {
		accounts[i] = protocol.Address(key.Public().(ed25519.PublicKey))
		g.Allocations = append(g.Allocations, protocol.Allocation{Account: accounts[i], Balance: 100, Representative: accounts[i]})
	}
	engines := make([]*consensus.Engine, 4)
	stores := make([]*ledger.Store, 4)
	dirs := make([]string, 4)
	traffic := make(chan consensus.Message, 4096)
	subscribe := func(e *consensus.Engine) {
		e.Subscribe(func(m consensus.Message) {
			select {
			case traffic <- m:
			default:
				t.Error("trace exceeded bounded traffic capacity")
			}
		})
	}
	for i := 1; i < 4; i++ {
		dirs[i] = t.TempDir()
		var err error
		stores[i], err = ledger.Open(dirs[i], g)
		if err != nil {
			t.Fatal(err)
		}
		engines[i], err = consensus.New(consensus.Config{Store: stores[i], PrivateKey: keys[i], RoundTimeout: 100 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		subscribe(engines[i])
	}
	defer func() {
		for _, s := range stores {
			if s != nil {
				s.Close()
			}
		}
	}()
	first := protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: accounts[0], Representative: accounts[0], Balance: 90, Link: string(accounts[1])}
	second := first
	second.Balance = 80
	if err := first.Sign(keys[0]); err != nil {
		t.Fatal(err)
	}
	if err := second.Sign(keys[0]); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 4; i++ {
		b := first
		if i == 3 {
			b = second
		}
		p := consensus.Proposal{Network: g.ID(), Height: 1, Parent: g.ID(), Block: b, ValidRound: -1, Representative: accounts[0]}
		if err := p.Sign(keys[0]); err != nil {
			t.Fatal(err)
		}
		if err := engines[i].Handle(consensus.Message{Type: "proposal", Proposal: &p}); err != nil {
			t.Fatal(err)
		}
	}
	// Byzantine prevote plus two honest prevotes forms a QC only in the partition.
	byzantine := ledger.Vote{Network: g.ID(), Height: 1, Phase: "prevote", Parent: g.ID(), Block: first.ID(), Representative: accounts[0]}
	if err := byzantine.Sign(keys[0]); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		if err := engines[i].Handle(consensus.Message{Type: "vote", Vote: &byzantine}); err != nil {
			t.Fatal(err)
		}
	}
	for deliveries := 0; len(traffic) > 0; deliveries++ {
		if deliveries > 10000 {
			t.Fatal("partition did not quiesce")
		}
		m := <-traffic
		if m.Type == "vote" && m.Vote.Phase == "prevote" {
			for i := 1; i <= 2; i++ {
				_ = engines[i].Handle(m)
			}
		}
	}
	for i := 1; i < 4; i++ {
		if stores[i].Status().Height != 0 {
			t.Fatal("partition manufactured confirmation")
		}
	}
	if err := stores[2].Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	stores[2], err = ledger.Open(dirs[2], g)
	if err != nil {
		t.Fatal(err)
	}
	engines[2], err = consensus.New(consensus.Config{Store: stores[2], PrivateKey: keys[2], RoundTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("normal locked signing recovery rejected: %v", err)
	}
	subscribe(engines[2])
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 3)
	for i := 1; i < 4; i++ {
		e := engines[i]
		go func() { done <- e.Run(ctx) }()
	}
	defer func() {
		cancel()
		for i := 0; i < 3; i++ {
			if err := <-done; err != nil {
				t.Errorf("consensus worker: %v", err)
			}
		}
	}()
	for {
		all := true
		for i := 1; i < 4; i++ {
			st := stores[i].Status()
			if st.Height == 0 {
				all = false
			} else {
				r := stores[i].Records(0, 1)
				if len(r) != 1 || r[0].Block.ID() != first.ID() {
					t.Fatal("locked quorum lost safety after partition")
				}
			}
		}
		if all {
			return
		}
		select {
		case m := <-traffic:
			for i := 1; i < 4; i++ {
				_ = engines[i].Handle(m)
			}
		case <-ctx.Done():
			t.Fatal("honest quorum did not recover after synchrony")
		}
	}
}
