package ledger

import (
	"encoding/json"
	"example.com/go-block-lattice/internal/protocol"
	"fmt"
	"math"
	"sort"
)

type state struct {
	byDestination map[protocol.Account][]protocol.Hash
	history       map[protocol.Account][]uint64
	network       protocol.Hash
	genesis       protocol.Genesis
	accounts      map[protocol.Account]AccountState
	receivables   map[protocol.Hash]Receivable
	blocks        map[protocol.Hash]protocol.Block
	records       []Record
	recordSizes   []uint32
	tip           protocol.Hash
}

func newState(g protocol.Genesis) *state {
	g.Allocations = append([]protocol.Allocation(nil), g.Allocations...)
	network := g.ID()
	s := &state{byDestination: make(map[protocol.Account][]protocol.Hash), history: make(map[protocol.Account][]uint64), network: network, genesis: g, accounts: make(map[protocol.Account]AccountState), receivables: make(map[protocol.Hash]Receivable), blocks: make(map[protocol.Hash]protocol.Block), tip: network}
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
	if b.Network != s.network {
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

	if !s.representativeCapacity(b, a, exists) {
		return fmt.Errorf("%w: maximum active representatives exceeded", ErrInvalid)
	}
	return nil
}

// representativeCapacity evaluates the next weight view without mutating state.
func (s *state) representativeCapacity(b protocol.Block, a AccountState, exists bool) bool {
	weights := s.weights()
	owned := a.Balance
	for _, r := range s.receivables {
		if r.From == b.Account {
			owned += r.Amount
		}
	}
	if exists {
		weights[a.Representative] -= owned
	}
	nextOwned := owned
	if b.Kind == "receive" {
		r := s.receivables[protocol.Hash(b.Link)]
		if r.From != b.Account {
			fromRep := s.accounts[r.From].Representative
			weights[fromRep] -= r.Amount
			nextOwned += r.Amount
		}
	}
	// Sending changes balance to an outgoing receivable, preserving the sender's weight.
	weights[b.Representative] += nextOwned
	count := 0
	for _, w := range weights {
		if w > 0 {
			count++
		}
	}
	return count <= protocol.MaxRepresentatives
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
	return certificate(r, s.network, s.weights(), s.genesis.Supply)
}
func (s *state) apply(r Record, serializedSize ...int) {
	size := 0
	if len(serializedSize) > 0 {
		size = serializedSize[0]
	} else {
		encoded, _ := json.Marshal(r)
		size = len(encoded)
	}
	s.recordSizes = append(s.recordSizes, uint32(size))
	b := r.Block
	id := b.ID()
	a := s.accounts[b.Account]
	switch b.Kind {
	case "send":
		s.receivables[id] = Receivable{Source: id, From: b.Account, To: protocol.Account(b.Link), Amount: a.Balance - b.Balance}
		to := protocol.Account(b.Link)
		keys := s.byDestination[to]
		at := sort.Search(len(keys), func(i int) bool { return keys[i] >= id })
		keys = append(keys, "")
		copy(keys[at+1:], keys[at:])
		keys[at] = id
		s.byDestination[to] = keys
	case "receive":
		delete(s.receivables, protocol.Hash(b.Link))
		keys := s.byDestination[b.Account]
		at := sort.Search(len(keys), func(i int) bool { return keys[i] >= protocol.Hash(b.Link) })
		copy(keys[at:], keys[at+1:])
		keys[len(keys)-1] = ""
		s.byDestination[b.Account] = keys[:len(keys)-1]
		if len(s.byDestination[b.Account]) == 0 {
			delete(s.byDestination, b.Account)
		}
	}
	s.accounts[b.Account] = AccountState{Account: b.Account, Frontier: id, Balance: b.Balance, Representative: b.Representative, Height: a.Height + 1}
	s.blocks[id] = b
	s.records = append(s.records, r)
	s.history[b.Account] = append(s.history[b.Account], r.Height)
	s.tip = r.ID()
}
