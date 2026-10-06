package consensus

import (
	"errors"
	"fmt"
	"math"

	"example.com/go-block-lattice/internal/protocol"
)

// validateRecovered rejects semantically invalid durable signing state even when
// its storage envelope is intact. A repaired JSON file is not authorization to vote.
func (e *Engine) validateRecovered() error {
	s := e.state
	status := e.cfg.Store.Status()
	if s.LockedRound < -1 || s.ValidRound < -1 || s.LockedRound > int64(s.Round) || s.ValidRound > int64(s.Round) || len(s.Signed) > 2 {
		return errors.New("invalid persisted signing round")
	}
	if (s.Locked == nil) != (s.LockedRound < 0) || (s.Valid == nil) != (s.ValidRound < 0) {
		return errors.New("inconsistent persisted lock or valid candidate")
	}
	for _, b := range []*protocol.Block{s.Locked, s.Valid} {
		if b != nil {
			if err := e.cfg.Store.Validate(*b); err != nil {
				return fmt.Errorf("invalid persisted candidate: %w", err)
			}
		}
	}
	if s.Valid != nil {
		if s.Round == math.MaxUint32 {
			return errors.New("persisted round exhausted")
		}
		p := Proposal{Network: status.Network, Height: s.Height, Round: s.Round + 1, Parent: status.Tip, Block: *s.Valid, ValidRound: s.ValidRound, Proof: s.Proof}
		if err := e.verifyProof(p); err != nil {
			return fmt.Errorf("invalid persisted proof: %w", err)
		}
	} else if len(s.Proof) != 0 {
		return errors.New("unexpected persisted proof")
	}
	for key, v := range s.Signed {
		if v.Network != status.Network || v.Height != s.Height || v.Round != s.Round || v.Parent != status.Tip || v.Representative != s.Representative || key != fmt.Sprintf("%d/%d/%s", s.Height, s.Round, v.Phase) {
			return errors.New("persisted vote binding mismatch")
		}
		if err := v.Verify(); err != nil {
			return fmt.Errorf("invalid persisted vote: %w", err)
		}
		if v.Phase == "precommit" && v.Block != "" && (s.Locked == nil || s.Locked.ID() != v.Block || s.LockedRound != int64(v.Round)) {
			return errors.New("persisted precommit has no matching lock")
		}
	}
	if p := s.Proposal; p != nil {
		if p.Network != status.Network || p.Height != s.Height || p.Round != s.Round || p.Parent != status.Tip || p.Representative != s.Representative || p.Representative != proposer(s.Height, s.Round, e.weights) {
			return errors.New("persisted proposal binding mismatch")
		}
		if err := p.Verify(); err != nil {
			return err
		}
		if err := e.cfg.Store.Validate(p.Block); err != nil {
			return err
		}
		if err := e.verifyProof(*p); err != nil {
			return err
		}
	}
	return nil
}
