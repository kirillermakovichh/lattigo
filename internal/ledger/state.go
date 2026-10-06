package ledger

import (
	"example.com/go-block-lattice/internal/protocol"
	"fmt"
	"math"
)

type state struct {
	genesis     protocol.Genesis
	accounts    map[protocol.Account]AccountState
	receivables map[protocol.Hash]Receivable
	blocks      map[protocol.Hash]protocol.Block
	records     []Record
	tip         protocol.Hash
}

func newState(g protocol.Genesis) *state {
	s := &state{genesis: g, accounts: make(map[protocol.Account]AccountState), receivables: make(map[protocol.Hash]Receivable), blocks: make(map[protocol.Hash]protocol.Block), tip: g.ID()}
	for _, a := range g.Allocations {
		s.accounts[a.Account] = AccountState{Account: a.Account, Balance: a.Balance, Representative: a.Representative}
	}
	return s
}
func (s *state) weights() map[protocol.Account]protocol.Amount {
	w := make(map[protocol.Account]protocol.Amount)
	for _, a := range s.accounts {
		w[a.Representative] += a.Balance
	}
	for _, r := range s.receivables {
		w[s.accounts[r.From].Representative] += r.Amount
	}
	return w
}
func (s *state) validate(b protocol.Block) error {
	if b.Network != s.genesis.ID() {
		return fmt.Errorf("%w: wrong network", ErrInvalid)
	}
	if e := b.Verify(); e != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, e)
	}
	if !b.CheckWork(s.genesis.WorkBits) {
		return fmt.Errorf("%w: insufficient admission work", ErrInvalid)
	}
	if _, ok := s.blocks[b.ID()]; ok {
		return nil
	}
	a, exists := s.accounts[b.Account]
	if exists && b.Previous != a.Frontier {
		return fmt.Errorf("%w: stale or competing frontier", ErrConflict)
	}
	if !exists && (b.Previous != "" || b.Kind != "receive") {
		return fmt.Errorf("%w: account opening requires receive", ErrDependency)
	}
	switch b.Kind {
	case "send":
		if !exists || b.Balance >= a.Balance {
			return fmt.Errorf("%w: send must reduce balance", ErrInvalid)
		}
	case "receive":
		r, ok := s.receivables[protocol.Hash(b.Link)]
		if !ok {
			return fmt.Errorf("%w: source is not receivable", ErrDependency)
		}
		if r.To != b.Account {
			return fmt.Errorf("%w: wrong receive destination", ErrInvalid)
		}
		if math.MaxUint64-uint64(a.Balance) < uint64(r.Amount) || b.Balance != a.Balance+r.Amount {
			return fmt.Errorf("%w: incorrect receive balance", ErrInvalid)
		}
	case "change":
		if !exists || b.Balance != a.Balance || b.Representative == a.Representative {
			return fmt.Errorf("%w: change must only change representative", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: unsupported block kind", ErrInvalid)
	}
	return nil
}
func (s *state) validateRecord(r Record) error {
	if r.Height != uint64(len(s.records))+1 || r.Previous != s.tip {
		return fmt.Errorf("%w: record height or parent", ErrConflict)
	}
	if _, ok := s.blocks[r.Block.ID()]; ok {
		return fmt.Errorf("%w: block already confirmed", ErrConflict)
	}
	if e := s.validate(r.Block); e != nil {
		return e
	}
	return certificate(r, s.genesis.ID(), s.weights(), s.genesis.Supply)
}
func (s *state) apply(r Record) {
	b := r.Block
	a := s.accounts[b.Account]
	switch b.Kind {
	case "send":
		s.receivables[b.ID()] = Receivable{Source: b.ID(), From: b.Account, To: protocol.Account(b.Link), Amount: a.Balance - b.Balance}
	case "receive":
		delete(s.receivables, protocol.Hash(b.Link))
	}
	s.accounts[b.Account] = AccountState{Account: b.Account, Frontier: b.ID(), Balance: b.Balance, Representative: b.Representative, Height: a.Height + 1}
	s.blocks[b.ID()] = b
	s.records = append(s.records, r)
	s.tip = r.ID()
}
