package ledger

import (
	"crypto/ed25519"
	"errors"
	"example.com/go-block-lattice/internal/protocol"
	"math"
	"testing"
)

func fixture(t *testing.T) (*state, ed25519.PrivateKey, ed25519.PrivateKey) {
	t.Helper()
	k := ed25519.NewKeyFromSeed(make([]byte, 32))
	seed := make([]byte, 32)
	seed[0] = 1
	l := ed25519.NewKeyFromSeed(seed)
	a := protocol.Address(k.Public().(ed25519.PublicKey))
	return newState(protocol.Genesis{Version: 1, Name: "test", Supply: 100, Allocations: []protocol.Allocation{{Account: a, Balance: 100, Representative: a}}}), k, l
}
func nextRecord(t *testing.T, s *state, b protocol.Block, key ed25519.PrivateKey) Record {
	t.Helper()
	if e := b.Sign(key); e != nil {
		t.Fatal(e)
	}
	r := Record{Height: uint64(len(s.records)) + 1, Previous: s.tip, Block: b}
	v := Vote{Network: s.genesis.ID(), Height: r.Height, Phase: "precommit", Parent: r.Previous, Block: b.ID(), Representative: protocol.Address(key.Public().(ed25519.PublicKey))}
	if e := v.Sign(key); e != nil {
		t.Fatal(e)
	}
	r.Votes = []Vote{v}
	return r
}
func TestTransferConservesSupplyAndDelegatedWeight(t *testing.T) {
	s, k, l := fixture(t)
	a := protocol.Address(k.Public().(ed25519.PublicKey))
	b := protocol.Address(l.Public().(ed25519.PublicKey))
	send := nextRecord(t, s, protocol.Block{Version: 1, Network: s.genesis.ID(), Kind: "send", Account: a, Representative: a, Balance: 60, Link: string(b)}, k)
	if e := s.validateRecord(send); e != nil {
		t.Fatal(e)
	}
	s.apply(send)
	if s.weights()[a] != 100 || s.receivables[send.Block.ID()].Amount != 40 {
		t.Fatal("pending weight lost")
	}
	receive := nextRecord(t, s, protocol.Block{Version: 1, Network: s.genesis.ID(), Kind: "receive", Account: b, Representative: a, Balance: 40, Link: string(send.Block.ID())}, l)
	receive.Votes = nil
	v := Vote{Network: s.genesis.ID(), Height: receive.Height, Phase: "precommit", Parent: receive.Previous, Block: receive.Block.ID(), Representative: a}
	if e := v.Sign(k); e != nil {
		t.Fatal(e)
	}
	receive.Votes = []Vote{v}
	if e := s.validateRecord(receive); e != nil {
		t.Fatal(e)
	}
	s.apply(receive)
	if s.weights()[a] != 100 || len(s.receivables) != 0 {
		t.Fatal("supply or delegation changed")
	}
	if e := s.validateRecord(receive); !errors.Is(e, ErrConflict) {
		t.Fatalf("duplicate record: %v", e)
	}
}
func TestCertificateRejectsDuplicationAndBinding(t *testing.T) {
	s, k, l := fixture(t)
	a := protocol.Address(k.Public().(ed25519.PublicKey))
	b := protocol.Address(l.Public().(ed25519.PublicKey))
	r := nextRecord(t, s, protocol.Block{Version: 1, Network: s.genesis.ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(b)}, k)
	r.Votes = append(r.Votes, r.Votes[0])
	if s.validateRecord(r) == nil {
		t.Fatal("duplicate votes accepted")
	}
	r.Votes = r.Votes[:1]
	r.Votes[0].Round++
	if s.validateRecord(r) == nil {
		t.Fatal("wrong round accepted")
	}
	if !HasQuorum(protocol.Amount(math.MaxUint64), protocol.Amount(math.MaxUint64)) || HasQuorum(2, 3) || !HasQuorum(3, 3) {
		t.Fatal("quorum rounding")
	}
}

func TestSelfReceivePreservesDelegation(t *testing.T) {
	s, key, other := fixture(t)
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Address(other.Public().(ed25519.PublicKey))
	send := nextRecord(t, s, protocol.Block{Version: 1, Network: s.genesis.ID(), Kind: "send", Account: a, Representative: a, Balance: 60, Link: string(a)}, key)
	if err := s.validateRecord(send); err != nil {
		t.Fatal(err)
	}
	s.apply(send)
	recv := nextRecord(t, s, protocol.Block{Version: 1, Network: s.genesis.ID(), Kind: "receive", Account: a, Previous: send.Block.ID(), Representative: b, Balance: 100, Link: string(send.Block.ID())}, key)
	if err := s.validateRecord(recv); err != nil {
		t.Fatal(err)
	}
	s.apply(recv)
	if s.weights()[b] != 100 || s.weights()[a] != 0 {
		t.Fatal("self receive duplicated voting weight")
	}
}
