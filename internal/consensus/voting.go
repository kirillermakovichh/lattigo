package consensus

import (
	"errors"
	"fmt"
	"sort"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

func (e *Engine) voteKey(phase string) string {
	return fmt.Sprintf("%d/%d/%s", e.state.Height, e.state.Round, phase)
}
func (e *Engine) hasSigned(phase string) bool { _, ok := e.state.Signed[e.voteKey(phase)]; return ok }
func (e *Engine) canSign() bool {
	return len(e.cfg.PrivateKey) > 0 && e.weights[e.state.Representative] > 0
}
func (e *Engine) signVote(phase string, hash protocol.Hash) error {
	if !e.canSign() {
		return nil
	}
	key := e.voteKey(phase)
	if old, ok := e.state.Signed[key]; ok {
		if old.Block != hash {
			return errors.New("refusing conflicting local vote")
		}
		return nil
	}
	status := e.cfg.Store.Status()
	v := ledger.Vote{Network: status.Network, Height: e.state.Height, Round: e.state.Round, Phase: phase, Parent: status.Tip, Block: hash, Representative: e.state.Representative}
	if err := v.Sign(e.cfg.PrivateKey); err != nil {
		return err
	}
	e.state.Signed[key] = v
	// Durability must precede even local processing: local processing can commit.
	if err := e.persist(); err != nil {
		return err
	}
	e.remember(v)
	e.emit(Message{Type: "vote", Vote: &v})
	return nil
}
func (e *Engine) remember(v ledger.Vote) {
	if e.votes[v.Round] == nil {
		e.votes[v.Round] = map[string]map[protocol.Account]ledger.Vote{}
	}
	if e.votes[v.Round][v.Phase] == nil {
		e.votes[v.Round][v.Phase] = map[protocol.Account]ledger.Vote{}
	}
	e.votes[v.Round][v.Phase][v.Representative] = v
}
func (e *Engine) acceptVote(v ledger.Vote) error {
	status := e.cfg.Store.Status()
	if v.Network != status.Network || v.Height != e.state.Height || v.Parent != status.Tip || uint64(v.Round)+2 < uint64(e.state.Round) {
		return ErrStale
	}
	if e.weights[v.Representative] == 0 {
		return errors.New("representative has no confirmed weight")
	}
	if err := v.Verify(); err != nil {
		return err
	}
	if uint64(v.Round) > uint64(e.state.Round)+32 {
		// Keep only the latest authenticated future vote per representative.
		if old, ok := e.ahead[v.Representative]; !ok || v.Round >= old.Round {
			e.ahead[v.Representative] = v
		}
		var w protocol.Amount
		for rep, other := range e.ahead {
			if other.Round == v.Round {
				w += e.weights[rep]
			}
		}
		if uint64(w) > uint64(status.Supply)/3 {
			if err := e.advanceRound(v.Round); err != nil {
				return err
			}
			for _, other := range e.ahead {
				if other.Round == v.Round {
					e.remember(other)
				}
			}
			e.ahead = map[protocol.Account]ledger.Vote{}
		}
		return nil
	}
	if phases := e.votes[v.Round]; phases != nil {
		if old, ok := phases[v.Phase][v.Representative]; ok {
			if old.Block != v.Block {
				return errors.New("representative equivocation")
			}
			return nil
		}
	}
	e.remember(v)
	e.emit(Message{Type: "vote", Vote: &v})
	return nil
}
func (e *Engine) proof(round uint32, phase string, hash protocol.Hash) ([]ledger.Vote, bool) {
	var weight protocol.Amount
	var vs []ledger.Vote
	for rep, v := range e.votes[round][phase] {
		if v.Block == hash {
			weight += e.weights[rep]
			vs = append(vs, v)
		}
	}
	if !quorum(weight, e.cfg.Store.Status().Supply) {
		return nil, false
	}
	sort.Slice(vs, func(i, j int) bool { return vs[i].Representative < vs[j].Representative })
	return vs, true
}
func (e *Engine) verifyProof(p Proposal) error {
	if p.ValidRound < 0 {
		if p.ValidRound != -1 || len(p.Proof) != 0 {
			return errors.New("unexpected prevote proof")
		}
		return nil
	}
	if p.ValidRound >= int64(p.Round) || len(p.Proof) == 0 || len(p.Proof) > 16384 {
		return errors.New("invalid proposal valid round or certificate size")
	}
	seen := map[protocol.Account]bool{}
	var weight protocol.Amount
	for _, v := range p.Proof {
		if v.Network != p.Network || v.Height != p.Height || int64(v.Round) != p.ValidRound || v.Parent != p.Parent || v.Block != p.Block.ID() || v.Phase != "prevote" {
			return errors.New("proposal proof binding mismatch")
		}
		if seen[v.Representative] || e.weights[v.Representative] == 0 {
			return errors.New("invalid proposal proof representative")
		}
		if err := v.Verify(); err != nil {
			return err
		}
		seen[v.Representative] = true
		weight += e.weights[v.Representative]
	}
	if !quorum(weight, e.cfg.Store.Status().Supply) {
		return errors.New("insufficient proposal proof")
	}
	return nil
}
func (e *Engine) acceptProposal(p Proposal) error {
	status := e.cfg.Store.Status()
	if p.Network != status.Network || p.Height != e.state.Height || p.Parent != status.Tip || uint64(p.Round) > uint64(e.state.Round)+32 || uint64(p.Round)+2 < uint64(e.state.Round) {
		return ErrStale
	}
	if p.Representative != proposer(p.Height, p.Round, e.weights) {
		return errors.New("unexpected proposer")
	}
	if err := p.Verify(); err != nil {
		return err
	}
	if err := e.cfg.Store.Validate(p.Block); err != nil {
		return err
	}
	if err := e.verifyProof(p); err != nil {
		return err
	}
	if old, ok := e.proposals[p.Round]; ok {
		if old.Block.ID() != p.Block.ID() {
			return errors.New("proposer equivocation")
		}
		return nil
	}
	e.proposals[p.Round] = p
	// The proposal carries a valid candidate even when the local mempool is full.
	e.emit(Message{Type: "proposal", Proposal: &p})
	return nil
}
func (e *Engine) propose() error {
	if !e.canSign() || proposer(e.state.Height, e.state.Round, e.weights) != e.state.Representative {
		return nil
	}
	if _, ok := e.proposals[e.state.Round]; ok {
		return nil
	}
	var candidate protocol.Block
	if e.state.Valid != nil {
		candidate = *e.state.Valid
	} else {
		ids := make([]protocol.Hash, 0, len(e.pending))
		for id := range e.pending {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			b := e.pending[id]
			if err := e.cfg.Store.Validate(b); err != nil {
				delete(e.pending, id)
				continue
			}
			candidate = b
			break
		}
	}
	if candidate.ID() == "" {
		return nil
	}
	status := e.cfg.Store.Status()
	p := Proposal{Network: status.Network, Height: e.state.Height, Round: e.state.Round, Parent: status.Tip, Block: candidate, ValidRound: e.state.ValidRound, Proof: e.state.Proof, Representative: e.state.Representative}
	if err := p.Sign(e.cfg.PrivateKey); err != nil {
		return err
	}
	e.state.Proposal = &p
	if err := e.persist(); err != nil {
		return err
	}
	e.proposals[p.Round] = p
	e.emit(Message{Type: "proposal", Proposal: &p})
	return nil
}
func (e *Engine) drive() error {
	// Bound work per admission/tick even with a single-representative network.
	for step := 0; step < 64; step++ {
		height := e.state.Height
		if err := e.propose(); err != nil {
			return err
		}
		// A late old-round quorum still informs later re-proposals; it does not
		// authorize signing a precommit for a round that has already been left.
		for oldRound, p := range e.proposals {
			if oldRound <= e.state.Round && int64(oldRound) > e.state.ValidRound {
				if proof, ok := e.proof(oldRound, "prevote", p.Block.ID()); ok {
					b := p.Block
					e.state.Valid = &b
					e.state.ValidRound = int64(oldRound)
					e.state.Proof = proof
					if err := e.persist(); err != nil {
						return err
					}
				}
			}
		}
		r := e.state.Round
		if p, ok := e.proposals[r]; ok && !e.hasSigned("prevote") {
			hash := protocol.Hash("")
			if e.state.Locked == nil || e.state.Locked.ID() == p.Block.ID() || p.ValidRound >= e.state.LockedRound {
				hash = p.Block.ID()
			}
			if err := e.signVote("prevote", hash); err != nil {
				return err
			}
		}
		// Process nonnil prevote quorums only with the full validated candidate.
		if p, ok := e.proposals[r]; ok {
			if proof, ok := e.proof(r, "prevote", p.Block.ID()); ok {
				if e.state.ValidRound < int64(r) {
					b := p.Block
					e.state.Valid = &b
					e.state.ValidRound = int64(r)
					e.state.Proof = proof
					if err := e.persist(); err != nil {
						return err
					}
				}
				if !e.hasSigned("precommit") && e.canSign() {
					b := p.Block
					e.state.Locked = &b
					e.state.LockedRound = int64(r)
					if err := e.persist(); err != nil {
						return err
					}
					if err := e.signVote("precommit", p.Block.ID()); err != nil {
						return err
					}
				}
			}
		}
		if _, ok := e.proof(r, "prevote", ""); ok && !e.hasSigned("precommit") {
			if err := e.signVote("precommit", ""); err != nil {
				return err
			}
		}
		// Commit certificates from retained earlier rounds are also valid.
		for round, p := range e.proposals {
			if vs, ok := e.proof(round, "precommit", p.Block.ID()); ok {
				record := ledger.Record{Height: e.state.Height, Round: round, Previous: e.cfg.Store.Status().Tip, Block: p.Block, Votes: vs}
				if err := e.commit(record, true); err != nil {
					return err
				}
				break
			}
		}
		if height != e.state.Height {
			continue
		}
		if _, ok := e.proof(r, "precommit", ""); ok {
			if err := e.roundNext(); err != nil {
				return err
			}
			continue
		}
		// A quorum already in a future round permits catching up without waiting
		// for every local timeout. Never discard locks on a round change.
		var highest uint32
		for round, phases := range e.votes {
			if round <= r {
				continue
			}
			seen := map[protocol.Account]bool{}
			var weight protocol.Amount
			for _, vs := range phases {
				for rep := range vs {
					if !seen[rep] {
						weight += e.weights[rep]
						seen[rep] = true
					}
				}
			}
			if uint64(weight) > uint64(e.cfg.Store.Status().Supply)/3 && round > highest {
				highest = round
			}
		}
		if highest > r {
			if err := e.advanceRound(highest); err != nil {
				return err
			}
			continue
		}
		break
	}
	return nil
}
func (e *Engine) commit(r ledger.Record, broadcast bool) error {
	status := e.cfg.Store.Status()
	if r.Height <= status.Height {
		known := e.cfg.Store.Records(r.Height-1, 1)
		if len(known) == 1 && known[0].ID() == r.ID() {
			return nil
		}
		return ledger.ErrConflict
	}
	if r.Height != status.Height+1 {
		return ledger.ErrDependency
	}
	if err := e.cfg.Store.Commit(r); err != nil {
		if !errors.Is(err, ledger.ErrInvalid) && !errors.Is(err, ledger.ErrConflict) && !errors.Is(err, ledger.ErrDependency) {
			e.failure = fmt.Errorf("ledger commit failed: %w", err)
		}
		return err
	}
	if broadcast {
		e.emit(Message{Type: "commit", Record: &r})
	}
	oldRep := e.state.Representative
	e.state = signingState{Version: 1, Network: status.Network, Representative: oldRep, Height: r.Height + 1, LockedRound: -1, ValidRound: -1, Signed: map[string]ledger.Vote{}}
	e.weights = e.cfg.Store.Weights()
	e.ahead = map[protocol.Account]ledger.Vote{}
	e.votes = map[uint32]map[string]map[protocol.Account]ledger.Vote{}
	e.proposals = map[uint32]Proposal{}
	for id, b := range e.pending {
		if id == r.Block.ID() || e.cfg.Store.Validate(b) != nil {
			delete(e.pending, id)
		}
	}
	e.resetDeadline()
	return e.persist()
}
