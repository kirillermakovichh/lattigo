package consensus

import (
	"errors"
	"testing"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

func TestConsensusPersistenceFailuresStopEachDecision(t *testing.T) {
	for _, scenario := range []string{"vote", "proposal", "timeout-prevote", "timeout-precommit", "advance-future", "drive-proposal", "drive-valid", "drive-prevote", "drive-lock", "drive-nil-precommit", "drive-next-round", "drive-future"} {
		t.Run(scenario, func(t *testing.T) {
			f := makeFixture(4)
			e, s := openEngine(t, f, 0)
			b := sendBlock(t, f)
			p := Proposal{Network: f.genesis.ID(), Height: 1, Round: 0, Parent: s.Status().Tip, Block: b, ValidRound: -1, Representative: f.accounts[0]}
			if err := p.Sign(f.keys[0]); err != nil {
				t.Fatal(err)
			}
			remember := func(round uint32, phase string, hash protocol.Hash) {
				for i := 0; i < 3; i++ {
					v := ledger.Vote{Network: f.genesis.ID(), Height: 1, Round: round, Phase: phase, Parent: s.Status().Tip, Block: hash, Representative: f.accounts[i]}
					if err := v.Sign(f.keys[i]); err != nil {
						t.Fatal(err)
					}
					e.remember(v)
				}
			}
			// Set up a real validated decision before making persistence unavailable.
			switch scenario {
			case "proposal", "drive-proposal":
				e.state.Valid = &b
				e.state.ValidRound = 0
				e.state.Round = 4
				remember(0, "prevote", b.ID())
				e.state.Proof, _ = e.proof(0, "prevote", b.ID())
			case "drive-valid", "drive-prevote", "drive-lock":
				e.proposals[0] = p
			case "drive-nil-precommit":
				remember(0, "prevote", "")
			case "drive-next-round":
				remember(0, "precommit", "")
			case "drive-future":
				remember(2, "prevote", "")
				remember(2, "precommit", "")
			}
			if scenario == "drive-valid" || scenario == "drive-lock" {
				remember(0, "prevote", b.ID())
			}
			if scenario == "drive-lock" {
				e.state.Valid = &b
				e.state.ValidRound = 0
				e.state.Signed[e.voteKey("prevote")] = ledger.Vote{Block: b.ID()}
			}
			if scenario == "timeout-precommit" {
				e.state.Signed[e.voteKey("prevote")] = ledger.Vote{}
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			var err error
			switch scenario {
			case "vote":
				err = e.signVote("prevote", "")
			case "proposal":
				err = e.propose()
			case "timeout-prevote", "timeout-precommit":
				err = e.timeout()
			case "advance-future":
				for i := 0; i < 2; i++ {
					v := ledger.Vote{Network: f.genesis.ID(), Height: 1, Round: 100, Phase: "prevote", Parent: s.Status().Tip, Representative: f.accounts[i]}
					if signErr := v.Sign(f.keys[i]); signErr != nil {
						t.Fatal(signErr)
					}
					err = e.acceptVote(v)
				}
			default:
				err = e.drive()
			}
			if err == nil {
				t.Fatal("unavailable persistence accepted decision")
			}
			if !errors.Is(err, ledger.ErrClosed) {
				t.Fatalf("decision error: %v", err)
			}
			for _, m := range e.outgoing {
				if m.Proposal != nil || (scenario != "advance-future" && m.Vote != nil && m.Vote.Representative == e.state.Representative) {
					t.Fatal("local signing intent published without durability")
				}
			}
		})
	}
}
