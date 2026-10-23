package consensus

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

func TestRecoveryResetsOlderHeightAndNormalizesUnsignedState(t *testing.T) {
	f := makeFixture(4)
	e, s := openEngine(t, f, 0)
	old := e.state
	old.Signed = nil
	data, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveSigningState(data); err != nil {
		t.Fatal(err)
	}
	restored, err := New(Config{Store: s, PrivateKey: f.keys[0]})
	if err != nil {
		t.Fatal(err)
	}
	if restored.state.Signed == nil {
		t.Fatal("unsigned recovery left a nil vote map")
	}
	b := sendBlock(t, f)
	r := ledger.Record{Height: 1, Previous: s.Status().Tip, Block: b}
	for i := 0; i < 3; i++ {
		v := ledger.Vote{Network: f.genesis.ID(), Height: 1, Parent: r.Previous, Phase: "precommit", Block: b.ID(), Representative: f.accounts[i]}
		if err := v.Sign(f.keys[i]); err != nil {
			t.Fatal(err)
		}
		r.Votes = append(r.Votes, v)
	}
	if err = s.Commit(r); err != nil {
		t.Fatal(err)
	}
	restored, err = New(Config{Store: s, PrivateKey: f.keys[0]})
	if err != nil {
		t.Fatal(err)
	}
	if restored.state.Height != 2 || restored.state.Round != 0 || restored.state.Locked != nil {
		t.Fatal("older signing height was reused")
	}
}

func TestRecoveryRejectsMissingCandidateAndUnavailableStorage(t *testing.T) {
	f := makeFixture(4)
	e, s := openEngine(t, f, 0)
	e.state.ValidRound = 0
	if err := e.validateRecovered(); err == nil {
		t.Fatal("valid round without candidate accepted")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{Store: s, PrivateKey: f.keys[0]}); !errors.Is(err, ledger.ErrClosed) {
		t.Fatalf("closed recovery: %v", err)
	}
}

func TestConstructorDoesNotPublishWhenInitialSigningWriteFails(t *testing.T) {
	f := makeFixture(4)
	dir := t.TempDir()
	s, err := ledger.Open(dir, f.genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}
	if err = os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)
	if _, err = New(Config{Store: s, PrivateKey: f.keys[0]}); err == nil {
		t.Fatal("initial signing persistence failure accepted")
	}
	if _, err = os.Stat(filepath.Join(dir, "signing.json")); !os.IsNotExist(err) {
		t.Fatal("failed initialization published signing state")
	}

}

func TestRoundRetentionAndIdempotentLocalVote(t *testing.T) {
	f := makeFixture(4)
	e, _ := openEngine(t, f, 0)
	if err := e.signVote("prevote", ""); err != nil {
		t.Fatal(err)
	}
	n := len(e.outgoing)
	if err := e.signVote("prevote", ""); err != nil {
		t.Fatal(err)
	}
	if len(e.outgoing) != n {
		t.Fatal("idempotent vote republished")
	}
	e.state.Signed["future"] = ledger.Vote{Round: 5}
	e.proposals[0] = Proposal{}
	if err := e.advanceRound(5); err != nil {
		t.Fatal(err)
	}
	if len(e.proposals) != 0 || len(e.state.Signed) != 1 {
		t.Fatal("round retention discarded current evidence or kept stale proposal")
	}
}

func TestLockedSignerPrevotesNilForUnprovenConflictingCandidate(t *testing.T) {
	f := makeFixture(4)
	e, s := openEngine(t, f, 3)
	locked := sendBlock(t, f)
	other := locked
	other.Balance--
	if err := other.Sign(f.keys[0]); err != nil {
		t.Fatal(err)
	}
	e.state.Round = 1
	e.state.Locked = &locked
	e.state.LockedRound = 0
	p := Proposal{Network: f.genesis.ID(), Height: 1, Round: 1, Parent: s.Status().Tip, Block: other, ValidRound: -1, Representative: f.accounts[1]}
	if err := p.Sign(f.keys[1]); err != nil {
		t.Fatal(err)
	}
	if err := e.Handle(Message{Type: "proposal", Proposal: &p}); err != nil {
		t.Fatal(err)
	}
	vote, ok := e.state.Signed[e.voteKey("prevote")]
	if !ok || vote.Block != "" {
		t.Fatal("locked signer prevoted a conflicting unproven block")
	}
}

func TestFutureEvidenceCountsEachRepresentativeOnce(t *testing.T) {
	f := makeFixture(4)
	e, s := openEngine(t, f, 3)
	send := func(rep int, round uint32, phase string) {
		t.Helper()
		v := ledger.Vote{Network: f.genesis.ID(), Height: 1, Round: round, Phase: phase, Parent: s.Status().Tip, Representative: f.accounts[rep]}
		if err := v.Sign(f.keys[rep]); err != nil {
			t.Fatal(err)
		}
		if err := e.acceptVote(v); err != nil {
			t.Fatal(err)
		}
	}
	send(0, 100, "prevote")
	send(1, 101, "prevote")
	send(1, 101, "precommit")
	if e.state.Round != 0 {
		t.Fatal("unrelated rounds or duplicate representative phases combined")
	}
	send(2, 101, "prevote")
	if e.state.Round != 101 || len(e.ahead) != 0 {
		t.Fatal("matching authenticated future evidence did not advance")
	}
	if len(e.votes[101]["prevote"]) != 2 || len(e.votes[101]["precommit"]) != 1 {
		t.Fatal("future phases not retained")
	}
}

func TestCommitRetainsIndependentPendingAndHonorsBroadcastFlag(t *testing.T) {
	f := makeFixture(4)
	e, s := openEngine(t, f, 3)
	b := sendBlock(t, f)
	independent := protocol.Block{Version: 1, Network: f.genesis.ID(), Kind: "send", Account: f.accounts[1], Representative: f.accounts[1], Balance: 99, Link: string(f.accounts[2])}
	if err := independent.Sign(f.keys[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := e.submit(independent, false); err != nil {
		t.Fatal(err)
	}
	r := ledger.Record{Height: 1, Previous: s.Status().Tip, Block: b}
	for i := 0; i < 3; i++ {
		v := ledger.Vote{Network: f.genesis.ID(), Height: 1, Parent: r.Previous, Phase: "precommit", Block: b.ID(), Representative: f.accounts[i]}
		if err := v.Sign(f.keys[i]); err != nil {
			t.Fatal(err)
		}
		r.Votes = append(r.Votes, v)
	}
	if err := e.commit(r, false); err != nil {
		t.Fatal(err)
	}
	if len(e.pending) != 1 || len(e.outgoing) != 0 {
		t.Fatal("quiet commit lost independent work or broadcast a record")
	}
}
