//go:build faultaudit

package consensus

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/faultprobe"
	"example.com/go-block-lattice/internal/ledger"
)

func TestDrivePersistsEachSigningBoundaryBeforePublishing(t *testing.T) {
	// Preload two external prevotes; the local prevote creates quorum. The next
	// writes persist validity, the lock, and finally the local precommit.
	for _, nth := range []int{1, 2, 3, 4} {
		t.Run(string(rune('0'+nth)), func(t *testing.T) {
			f := makeFixture(4)
			e, s := openEngine(t, f, 0)
			b := sendBlock(t, f)
			p := Proposal{Network: f.genesis.ID(), Height: 1, Parent: s.Status().Tip, Block: b, ValidRound: -1, Representative: f.accounts[0]}
			if err := p.Sign(f.keys[0]); err != nil {
				t.Fatal(err)
			}
			e.proposals[0] = p
			for i := 1; i < 3; i++ {
				v := ledger.Vote{Network: f.genesis.ID(), Height: 1, Parent: s.Status().Tip, Phase: "prevote", Block: b.ID(), Representative: f.accounts[i]}
				if err := v.Sign(f.keys[i]); err != nil {
					t.Fatal(err)
				}
				e.remember(v)
			}
			faultprobe.Arm("journal.go/AtomicWrite/os.Rename/1", nth)
			defer faultprobe.Arm("", 0)
			err := e.drive()
			if !faultprobe.Reached() || !errors.Is(err, faultprobe.Injected) || e.failure == nil {
				t.Fatalf("signing boundary %d: %v", nth, err)
			}
			for _, m := range e.outgoing {
				if m.Vote != nil && m.Vote.Phase == "precommit" {
					t.Fatal("failed signing sequence published a precommit")
				}
			}
		})
	}
}

func TestDrivePropagatesCertifiedAppendFailure(t *testing.T) {
	f := makeFixture(4)
	e, s := openEngine(t, f, 3)
	b := sendBlock(t, f)
	e.cfg.PrivateKey = nil
	e.proposals[0] = Proposal{Network: f.genesis.ID(), Height: 1, Parent: s.Status().Tip, Block: b, ValidRound: -1, Representative: f.accounts[0]}
	for i := 0; i < 3; i++ {
		v := ledger.Vote{Network: f.genesis.ID(), Height: 1, Parent: s.Status().Tip, Phase: "precommit", Block: b.ID(), Representative: f.accounts[i]}
		if err := v.Sign(f.keys[i]); err != nil {
			t.Fatal(err)
		}
		e.remember(v)
	}
	faultprobe.Arm("journal.go/Append/j.file.Write/1", 1)
	defer faultprobe.Arm("", 0)
	err := e.drive()
	if !faultprobe.Reached() || !errors.Is(err, faultprobe.Injected) || e.failure == nil {
		t.Fatalf("certified append failure lost: %v", err)
	}
	if s.Status().Height != 0 {
		t.Fatal("failed certified append advanced ledger")
	}
}

func TestRunningEngineStopsWhenTimeoutCannotPersist(t *testing.T) {
	f := makeFixture(4)
	e, s := openEngine(t, f, 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	e.deadline = time.Now().Add(-time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := e.Run(ctx); !errors.Is(err, ledger.ErrClosed) {
		t.Fatalf("timeout failure: %v", err)
	}
}

func TestConsensusSigningAndEncodingFailures(t *testing.T) {
	for _, scenario := range []string{"initial-encoding", "vote-signature", "proposal-signature"} {
		t.Run(scenario, func(t *testing.T) {
			f := makeFixture(4)
			e, s := openEngine(t, f, 0)
			b := sendBlock(t, f)
			e.pending[b.ID()] = b
			var err error
			switch scenario {
			case "initial-encoding":
				faultprobe.Arm("engine.go/persist/json.Marshal/1", 1)
				_, err = New(Config{Store: s, PrivateKey: f.keys[0]})
			case "vote-signature":
				faultprobe.Arm("voting.go/signVote/v.Sign/1", 1)
				err = e.signVote("prevote", "")
			case "proposal-signature":
				faultprobe.Arm("voting.go/propose/p.Sign/1", 1)
				err = e.drive()
			}
			defer faultprobe.Arm("", 0)
			if !faultprobe.Reached() || !errors.Is(err, faultprobe.Injected) {
				t.Fatalf("signing/encoding failure: %v", err)
			}
			if len(e.outgoing) != 0 {
				t.Fatal("failed dependency published consensus data")
			}
		})
	}
}

func TestConsensusRejectsExhaustedHeight(t *testing.T) {
	f := makeFixture(4)
	_, s := openEngine(t, f, 0)
	faultprobe.Arm("engine.go/New/status-height-limit", 1)
	defer faultprobe.Arm("", 0)
	if _, err := New(Config{Store: s, PrivateKey: f.keys[0]}); !faultprobe.Reached() || err == nil || !strings.Contains(err.Error(), "height exhausted") {
		t.Fatalf("height limit: %v", err)
	}
}
