package consensus

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

func TestConsensusConstructionAndMessageDecisions(t *testing.T) {
	f := makeFixture(4)
	e, s := openEngine(t, f, 3)
	for _, config := range []Config{{}, {Store: s, PrivateKey: make([]byte, 1)}, {Store: s, MaxPending: -1}, {Store: s, MaxPending: 65537}, {Store: s, RoundTimeout: time.Millisecond}, {Store: s, RoundTimeout: time.Minute + 1}} {
		if _, err := New(config); err == nil {
			t.Fatal("invalid consensus configuration accepted")
		}
	}
	for _, kind := range []string{"unknown", "proposal", "vote", "commit", "block"} {
		if err := e.Handle(Message{Type: kind}); err == nil {
			t.Fatal("invalid message accepted")
		}
	}
	e.Subscribe(nil)
	b := sendBlock(t, f)
	p := Proposal{Network: f.genesis.ID(), Height: 1, Round: 0, Parent: s.Status().Tip, Block: b, ValidRound: -1, Representative: f.accounts[0]}
	if err := p.Sign(f.keys[0]); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Proposal){"height": func(p *Proposal) { p.Height++ }, "parent": func(p *Proposal) { p.Parent = protocol.Digest([]byte("wrong")) }, "network": func(p *Proposal) { p.Network = protocol.Digest([]byte("wrong")) }, "future": func(p *Proposal) { p.Round = 33 }, "proposer": func(p *Proposal) { p.Representative = f.accounts[1] }, "signature": func(p *Proposal) { p.Signature = "bad" }, "block": func(p *Proposal) { p.Block.Balance++ }, "proof": func(p *Proposal) { p.ValidRound = 0 }} {
		t.Run(name, func(t *testing.T) {
			bad := p
			mutate(&bad)
			if name == "block" || name == "proof" {
				if err := bad.Sign(f.keys[0]); err != nil {
					t.Fatal(err)
				}
			}
			if e.Handle(Message{Type: "proposal", Proposal: &bad}) == nil {
				t.Fatal("invalid proposal accepted")
			}
		})
	}
	if err := e.Handle(Message{Type: "proposal", Proposal: &p}); err != nil {
		t.Fatal(err)
	}
	if err := e.Handle(Message{Type: "proposal", Proposal: &p}); err != nil {
		t.Fatal(err)
	}
	conflicting := p
	conflicting.Block.Balance = 89
	if err := conflicting.Block.Sign(f.keys[0]); err != nil {
		t.Fatal(err)
	}
	if err := conflicting.Sign(f.keys[0]); err != nil {
		t.Fatal(err)
	}
	if e.Handle(Message{Type: "proposal", Proposal: &conflicting}) == nil {
		t.Fatal("equivocating proposer accepted")
	}
	vote := ledger.Vote{Network: f.genesis.ID(), Height: 1, Parent: s.Status().Tip, Phase: "prevote", Representative: f.accounts[1], Block: b.ID()}
	if err := vote.Sign(f.keys[1]); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ledger.Vote){"network": func(v *ledger.Vote) { v.Network = protocol.Digest([]byte("other")) }, "height": func(v *ledger.Vote) { v.Height++ }, "parent": func(v *ledger.Vote) { v.Parent = protocol.Digest([]byte("other")) }, "signature": func(v *ledger.Vote) { v.Signature = "bad" }, "weight": func(v *ledger.Vote) {
		v.Representative = protocol.Address(ed25519.NewKeyFromSeed(make([]byte, 32)).Public().(ed25519.PublicKey))
	}} {
		t.Run(name, func(t *testing.T) {
			bad := vote
			mutate(&bad)
			if e.Handle(Message{Type: "vote", Vote: &bad}) == nil {
				t.Fatal("invalid vote accepted")
			}
		})
	}
	if err := e.Handle(Message{Type: "vote", Vote: &vote}); err != nil {
		t.Fatal(err)
	}
	if err := e.Handle(Message{Type: "vote", Vote: &vote}); err != nil {
		t.Fatal(err)
	}
	conflictingVote := vote
	conflictingVote.Block = conflicting.Block.ID()
	if err := conflictingVote.Sign(f.keys[1]); err != nil {
		t.Fatal(err)
	}
	if e.Handle(Message{Type: "vote", Vote: &conflictingVote}) == nil {
		t.Fatal("equivocating voter accepted")
	}
	if err := e.commit(ledger.Record{Height: 3}, false); !errors.Is(err, ledger.ErrDependency) {
		t.Fatal("skipped height accepted")
	}
	if err := e.advanceRound(0); err != nil {
		t.Fatal(err)
	}
	e.state.Round = math.MaxUint32
	if e.roundNext() == nil {
		t.Fatal("round counter wrapped")
	}
}
func TestSigningFailureStopsPublicationAndAdmission(t *testing.T) {
	f := makeFixture(4)
	dir := t.TempDir()
	store, err := ledger.Open(dir, f.genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	e, err := New(Config{Store: store, PrivateKey: f.keys[0]})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(dir, "signing.json")); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(dir, "signing.json"), 0700); err != nil {
		t.Fatal(err)
	}
	var signed int
	e.Subscribe(func(m Message) {
		if m.Vote != nil || m.Proposal != nil {
			signed++
		}
	})
	b := sendBlock(t, f)
	if _, err = e.Submit(b); err == nil {
		t.Fatal("signing failure ignored")
	}
	if signed != 0 {
		t.Fatal("undurable signed data published")
	}
	if e.Health()["ready"] != false {
		t.Fatal("failed signer reported ready")
	}
	if _, err = e.Submit(b); err == nil {
		t.Fatal("failed signer admitted new work")
	}
	if e.Handle(Message{Type: "block", Block: &b}) == nil {
		t.Fatal("failed engine handled work")
	}
	if err = e.Run(context.Background()); err == nil {
		t.Fatal("failed engine kept running")
	}
}
func TestRecoveryRejectsInvalidSigningStateBranches(t *testing.T) {
	f := makeFixture(4)
	dir := t.TempDir()
	store, err := ledger.Open(dir, f.genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	e, err := New(Config{Store: store, PrivateKey: f.keys[0]})
	if err != nil {
		t.Fatal(err)
	}
	base := e.state
	for name, mutate := range map[string]func(*signingState){"version": func(s *signingState) { s.Version++ }, "network": func(s *signingState) { s.Network = "wrong" }, "identity": func(s *signingState) { s.Representative = f.accounts[1] }, "height ahead": func(s *signingState) { s.Height = 2 }, "round": func(s *signingState) { s.LockedRound = -2 }, "inconsistent lock": func(s *signingState) { s.LockedRound = 0 }, "unexpected proof": func(s *signingState) { s.Proof = []ledger.Vote{{}} }, "too many votes": func(s *signingState) { s.Signed = map[string]ledger.Vote{"a": {}, "b": {}, "c": {}} }, "vote binding": func(s *signingState) { s.Signed = map[string]ledger.Vote{"bad": {}} }} {
		t.Run(name, func(t *testing.T) {
			state := base
			mutate(&state)
			data, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if err = store.SaveSigningState(data); err != nil {
				t.Fatal(err)
			}
			if _, err = New(Config{Store: store, PrivateKey: f.keys[0]}); err == nil {
				t.Fatal("invalid recovered state accepted")
			}
		})
	}
	if err = store.SaveSigningState([]byte("{")); err != nil {
		t.Fatal(err)
	}
	if _, err = New(Config{Store: store, PrivateKey: f.keys[0]}); err == nil {
		t.Fatal("malformed recovered JSON accepted")
	}
}
func TestConsensusRunnerRejectsOverlapAndStopsOnCancellation(t *testing.T) {
	f := makeFixture(1)
	e, _ := openEngine(t, f, 0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx) }()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		e.mu.Lock()
		running := e.running
		e.mu.Unlock()
		if running {
			break
		}
		select {
		case <-deadline.C:
			cancel()
			t.Fatal("runner did not start")
		case <-time.After(time.Millisecond):
		}
	}
	if e.Run(context.Background()) == nil {
		cancel()
		t.Fatal("overlapping runner accepted")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPrevoteProofAcceptanceAndRejectionBranches(t *testing.T) {
	f := makeFixture(4)
	e, s := openEngine(t, f, 3)
	block := sendBlock(t, f)
	proposal := Proposal{Network: f.genesis.ID(), Height: 1, Round: 1, Parent: s.Status().Tip, Block: block, ValidRound: 0, Representative: f.accounts[1]}
	for i := 0; i < 3; i++ {
		v := ledger.Vote{Network: proposal.Network, Height: 1, Round: 0, Phase: "prevote", Parent: proposal.Parent, Block: block.ID(), Representative: f.accounts[i]}
		if err := v.Sign(f.keys[i]); err != nil {
			t.Fatal(err)
		}
		proposal.Proof = append(proposal.Proof, v)
	}
	if err := proposal.Sign(f.keys[1]); err != nil {
		t.Fatal(err)
	}
	if err := e.Handle(Message{Type: "proposal", Proposal: &proposal}); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Proposal){"negative": func(p *Proposal) { p.ValidRound = -2 }, "unexpected": func(p *Proposal) { p.ValidRound = -1 }, "same round": func(p *Proposal) { p.ValidRound = 1 }, "empty": func(p *Proposal) { p.Proof = nil }, "too large": func(p *Proposal) { p.Proof = make([]ledger.Vote, 16385) }, "binding": func(p *Proposal) { p.Proof[0].Parent = protocol.Digest([]byte("wrong")) }, "duplicate": func(p *Proposal) { p.Proof[1] = p.Proof[0] }, "signature": func(p *Proposal) { p.Proof[0].Signature = "bad" }, "weight": func(p *Proposal) {
		p.Proof[0].Representative = protocol.Address(ed25519.NewKeyFromSeed([]byte("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")).Public().(ed25519.PublicKey))
	}, "minority": func(p *Proposal) { p.Proof = p.Proof[:1] }} {
		t.Run(name, func(t *testing.T) {
			bad := proposal
			bad.Proof = append([]ledger.Vote{}, proposal.Proof...)
			mutate(&bad)
			if err := bad.Sign(f.keys[1]); err != nil {
				t.Fatal(err)
			}
			if e.Handle(Message{Type: "proposal", Proposal: &bad}) == nil {
				t.Fatal("invalid prevote proof accepted")
			}
		})
	}
}
func TestRecoveredLocksProofsVotesAndProposals(t *testing.T) {
	f := makeFixture(4)
	e, s := openEngine(t, f, 0)
	block := sendBlock(t, f)
	base := e.state
	base.Round = 4
	base.Locked = &block
	base.LockedRound = 0
	base.Valid = &block
	base.ValidRound = 0
	for i := 0; i < 3; i++ {
		v := ledger.Vote{Network: f.genesis.ID(), Height: 1, Round: 0, Phase: "prevote", Parent: s.Status().Tip, Block: block.ID(), Representative: f.accounts[i]}
		if err := v.Sign(f.keys[i]); err != nil {
			t.Fatal(err)
		}
		base.Proof = append(base.Proof, v)
	}
	own := ledger.Vote{Network: f.genesis.ID(), Height: 1, Round: 4, Phase: "prevote", Parent: s.Status().Tip, Block: block.ID(), Representative: f.accounts[0]}
	if err := own.Sign(f.keys[0]); err != nil {
		t.Fatal(err)
	}
	base.Signed = map[string]ledger.Vote{"1/4/prevote": own}
	p := Proposal{Network: f.genesis.ID(), Height: 1, Round: 4, Parent: s.Status().Tip, Block: block, ValidRound: 0, Proof: base.Proof, Representative: f.accounts[0]}
	if err := p.Sign(f.keys[0]); err != nil {
		t.Fatal(err)
	}
	base.Proposal = &p
	save := func(state signingState) {
		t.Helper()
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.SaveSigningState(data); err != nil {
			t.Fatal(err)
		}
	}
	save(base)
	if _, err := New(Config{Store: s, PrivateKey: f.keys[0]}); err != nil {
		t.Fatalf("valid recovery:%v", err)
	}
	for name, mutate := range map[string]func(*signingState){
		"lock after valid": func(s *signingState) { s.LockedRound = 1 }, "lock mismatch": func(s *signingState) { b := block; b.Balance--; s.Locked = &b }, "candidate": func(s *signingState) { b := block; b.Signature = "bad"; s.Valid = &b; s.Locked = &b }, "proof": func(s *signingState) { s.Proof = nil }, "round exhaustion": func(s *signingState) { s.Round = math.MaxUint32 }, "vote signature": func(s *signingState) {
			v := own
			v.Signature = "bad"
			s.Signed = map[string]ledger.Vote{"1/4/prevote": v}
		}, "unlocked precommit": func(s *signingState) {
			v := own
			v.Phase = "precommit"
			if err := v.Sign(f.keys[0]); err != nil {
				t.Fatal(err)
			}
			s.Signed = map[string]ledger.Vote{"1/4/precommit": v}
		}, "proposal binding": func(s *signingState) { s.Proposal.Height++ }, "proposal signature": func(s *signingState) { s.Proposal.Signature = "bad" }, "proposal candidate": func(s *signingState) {
			s.Proposal.Block.Balance--
			if err := s.Proposal.Sign(f.keys[0]); err != nil {
				t.Fatal(err)
			}
		}, "proposal proof": func(s *signingState) {
			s.Proposal.ValidRound = -1
			if err := s.Proposal.Sign(f.keys[0]); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			state := base
			copy := p
			state.Proposal = &copy
			mutate(&state)
			save(state)
			if _, err := New(Config{Store: s, PrivateKey: f.keys[0]}); err == nil {
				t.Fatal("invalid recovery accepted")
			}
		})
	}
}
