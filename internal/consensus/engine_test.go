package consensus

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

type fixture struct {
	genesis  protocol.Genesis
	keys     []ed25519.PrivateKey
	accounts []protocol.Account
}

func makeFixture(n int) fixture {
	f := fixture{genesis: protocol.Genesis{Version: 1, Name: "consensus-test", Supply: protocol.Amount(n * 100)}}
	for i := 0; i < n; i++ {
		key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{byte(i + 1)}, 32))
		f.keys = append(f.keys, key)
	}
	sort.Slice(f.keys, func(i, j int) bool {
		return protocol.Address(f.keys[i].Public().(ed25519.PublicKey)) < protocol.Address(f.keys[j].Public().(ed25519.PublicKey))
	})
	for _, key := range f.keys {
		a := protocol.Address(key.Public().(ed25519.PublicKey))
		f.accounts = append(f.accounts, a)
		f.genesis.Allocations = append(f.genesis.Allocations, protocol.Allocation{Account: a, Balance: 100, Representative: a})
	}
	return f
}
func openEngine(t *testing.T, f fixture, i int) (*Engine, *ledger.Store) {
	t.Helper()
	store, err := ledger.Open(t.TempDir(), f.genesis)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	e, err := New(Config{Store: store, PrivateKey: f.keys[i], RoundTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return e, store
}
func sendBlock(t *testing.T, f fixture) protocol.Block {
	t.Helper()
	b := protocol.Block{Version: 1, Network: f.genesis.ID(), Kind: "send", Account: f.accounts[0], Representative: f.accounts[0], Balance: 90, Link: string(f.accounts[1])}
	if err := b.Sign(f.keys[0]); err != nil {
		t.Fatal(err)
	}
	return b
}
func TestWeightedNetworkConfirmsAndConservesWeight(t *testing.T) {
	f := makeFixture(4)
	type packet struct {
		source  int
		message Message
	}
	var queue []packet
	engines := make([]*Engine, 4)
	stores := make([]*ledger.Store, 4)
	for i := range engines {
		engines[i], stores[i] = openEngine(t, f, i)
		source := i
		engines[i].Subscribe(func(m Message) { queue = append(queue, packet{source, m}) })
	}
	b := sendBlock(t, f)
	if _, err := engines[0].Submit(b); err != nil {
		t.Fatal(err)
	}
	deliveries := 0
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for i, e := range engines {
			if i != p.source {
				_ = e.Handle(p.message)
			}
		}
		deliveries++
		if deliveries > 10000 {
			t.Fatal("gossip failed to quiesce")
		}
	}
	for _, s := range stores {
		if s.Status().Height != 1 {
			t.Fatalf("height=%d, want1", s.Status().Height)
		}
		a, err := s.Account(f.accounts[0])
		if err != nil || a.Balance != 90 {
			t.Fatalf("sender: %+v %v", a, err)
		}
		rs := s.Receivables(f.accounts[1], 0, 10)
		if len(rs) != 1 || rs[0].Amount != 10 {
			t.Fatalf("receivables=%+v", rs)
		}
		var sum protocol.Amount
		for _, w := range s.Weights() {
			sum += w
		}
		if sum != f.genesis.Supply {
			t.Fatal("voting weight not conserved")
		}
	}
	// Confirmed retries and replayed block gossip must never start another election.
	if _, err := engines[0].Submit(b); err != nil {
		t.Fatal(err)
	}
	if len(engines[0].Pending()) != 0 || stores[0].Status().Height != 1 {
		t.Fatal("confirmed retry created pending work")
	}
	// Receiving changes the next height's weights, never the preceding certificate.
	recv := protocol.Block{Version: 1, Network: f.genesis.ID(), Kind: "receive", Account: f.accounts[1], Representative: f.accounts[1], Balance: 110, Link: string(b.ID())}
	if err := recv.Sign(f.keys[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := engines[1].Submit(recv); err != nil {
		t.Fatal(err)
	}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for i, e := range engines {
			if i != p.source {
				_ = e.Handle(p.message)
			}
		}
		deliveries++
		if deliveries > 20000 {
			t.Fatal("gossip failed to quiesce")
		}
	}
	for _, s := range stores {
		if s.Status().Height != 2 {
			t.Fatalf("receive not confirmed at height %d", s.Status().Height)
		}
		if s.Weights()[f.accounts[0]] != 90 || s.Weights()[f.accounts[1]] != 110 {
			t.Fatal("weight did not move with received funds")
		}
	}
}
func TestSigningRecordSurvivesRestartAndQuorumLoss(t *testing.T) {
	f := makeFixture(4)
	dir := t.TempDir()
	s, err := ledger.Open(dir, f.genesis)
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(Config{Store: s, PrivateKey: f.keys[0]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Submit(sendBlock(t, f)); err != nil {
		t.Fatal(err)
	}
	if s.Status().Height != 0 {
		t.Fatal("minority must not confirm")
	}
	data, err := s.SigningState()
	if err != nil {
		t.Fatal(err)
	}
	var before signingState
	if err = json.Unmarshal(data, &before); err != nil {
		t.Fatal(err)
	}
	if len(before.Signed) != 1 {
		t.Fatalf("expected durable prevote, got %d", len(before.Signed))
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = ledger.Open(dir, f.genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	e, err = New(Config{Store: s, PrivateKey: f.keys[0]})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.signVote("prevote", protocol.Digest([]byte("conflict"))); err == nil {
		t.Fatal("conflicting vote permitted after restart")
	}
	if s.Status().Height != 0 {
		t.Fatal("restart fabricated confirmation")
	}
}
func TestQuorumArithmetic(t *testing.T) {
	for _, x := range []struct {
		weight, total protocol.Amount
		want          bool
	}{{2, 3, false}, {3, 3, true}, {3, 4, true}, {2, 4, false}, {12297829382473034410, 18446744073709551615, false}, {12297829382473034411, 18446744073709551615, true}} {
		if got := quorum(x.weight, x.total); got != x.want {
			t.Fatalf("quorum(%d,%d)=%v", x.weight, x.total, got)
		}
	}
}

func TestFutureRoundCatchupRequiresHonestWeight(t *testing.T) {
	f := makeFixture(4)
	e, s := openEngine(t, f, 3)
	for i := 0; i < 2; i++ {
		v := ledger.Vote{Network: f.genesis.ID(), Height: 1, Round: 1000, Phase: "prevote", Parent: s.Status().Tip, Representative: f.accounts[i]}
		if err := v.Sign(f.keys[i]); err != nil {
			t.Fatal(err)
		}
		if err := e.Handle(Message{Type: "vote", Vote: &v}); err != nil {
			t.Fatal(err)
		}
		if i == 0 && e.state.Round != 0 {
			t.Fatal("one Byzantine representative forced round advancement")
		}
	}
	if e.state.Round != 1000 {
		t.Fatalf("round=%d, want1000", e.state.Round)
	}
	if len(e.votes) > 1 {
		t.Fatal("round catchup retained unbounded history")
	}
}
func TestLatePrevoteCertificateSurvivesRoundChange(t *testing.T) {
	f := makeFixture(4)
	e, s := openEngine(t, f, 3)
	b := sendBlock(t, f)
	p := Proposal{Network: f.genesis.ID(), Height: 1, Round: 0, Parent: s.Status().Tip, Block: b, ValidRound: -1, Representative: f.accounts[0]}
	if err := p.Sign(f.keys[0]); err != nil {
		t.Fatal(err)
	}
	if err := e.Handle(Message{Type: "proposal", Proposal: &p}); err != nil {
		t.Fatal(err)
	}
	if err := e.roundNext(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		v := ledger.Vote{Network: f.genesis.ID(), Height: 1, Round: 0, Phase: "prevote", Parent: s.Status().Tip, Block: b.ID(), Representative: f.accounts[i]}
		if err := v.Sign(f.keys[i]); err != nil {
			t.Fatal(err)
		}
		if err := e.Handle(Message{Type: "vote", Vote: &v}); err != nil {
			t.Fatal(err)
		}
	}
	if e.state.Valid == nil || e.state.Valid.ID() != b.ID() || e.state.ValidRound != 0 {
		t.Fatal("late prevote certificate was lost")
	}
	if e.state.Locked != nil {
		t.Fatal("late previous-round certificate created an old-round lock")
	}
}

func TestFutureRoundVotesPropagateAcrossRelay(t *testing.T) {
	f := makeFixture(4)
	relay, store := openEngine(t, f, 3)
	var outgoing []Message
	relay.Subscribe(func(m Message) { outgoing = append(outgoing, m) })
	for i := 0; i < 2; i++ {
		vote := ledger.Vote{Network: f.genesis.ID(), Height: 1, Round: 1000, Phase: "prevote", Parent: store.Status().Tip, Representative: f.accounts[i]}
		if err := vote.Sign(f.keys[i]); err != nil {
			t.Fatal(err)
		}
		if err := relay.Handle(Message{Type: "vote", Vote: &vote}); err != nil {
			t.Fatal(err)
		}
		// A repeating packet must not amplify traffic in a cyclic peer topology.
		if err := relay.Handle(Message{Type: "vote", Vote: &vote}); err != nil {
			t.Fatal(err)
		}
	}
	forwarded := map[protocol.Account]int{}
	for _, message := range outgoing {
		if message.Vote != nil && message.Vote.Round == 1000 && message.Vote.Phase == "prevote" {
			forwarded[message.Vote.Representative]++
		}
	}
	for _, rep := range f.accounts[:2] {
		if forwarded[rep] != 1 {
			t.Errorf("relayed future vote %s %d times, want1", rep, forwarded[rep])
		}
	}
}
