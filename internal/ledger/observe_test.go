package ledger

import (
	"crypto/ed25519"
	"errors"
	"example.com/go-block-lattice/internal/protocol"
	"testing"
)

func TestDurableObservationAndReplay(t *testing.T) {
	s, _, _, appendRecord := viewFixture(t)
	_, changed, err := s.ObserveRecord(5)
	if !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	appendRecord()
	select {
	case <-changed:
	default:
		t.Fatal("durable transition did not wake reader")
	}
	r, next, err := s.ObserveRecord(5)
	if err != nil || r.Height != 6 {
		t.Fatal(r, err)
	}
	b, same, err := s.ObserveConfirmation(r.Block.ID())
	if err != nil || b.ID() != r.Block.ID() || same != next {
		t.Fatal("inconsistent observation")
	}
	r.Votes[0].Signature = "changed"
	again, _, _ := s.ObserveRecord(5)
	if again.Votes[0].Signature == "changed" {
		t.Fatal("certificate aliases state")
	}
	if err := s.Commit(again); err != nil {
		t.Fatal(err)
	}
	select {
	case <-next:
		t.Fatal("idempotent commit emitted notification")
	default:
	}
	if _, _, err := s.ObserveRecord(7); !errors.Is(err, ErrStaleView) {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-next:
	default:
		t.Fatal("close did not wake reader")
	}
	if _, _, err := s.ObserveRecord(6); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, _, err := s.ObserveConfirmation(b.ID()); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestRejectedAppendDoesNotPublish(t *testing.T) {
	s, _, _, _ := viewFixture(t)
	_, changed, _ := s.ObserveRecord(5)
	r := s.Records(4, 1)[0]
	r.Height++
	if err := s.Commit(r); err == nil {
		t.Fatal("invalid record accepted")
	}
	select {
	case <-changed:
		t.Fatal("rejected append emitted notification")
	default:
	}
}

func TestFailedAppendDoesNotPublish(t *testing.T) {
	state, key, other := fixture(t)
	s, err := Open(t.TempDir(), state.genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Address(other.Public().(ed25519.PublicKey))
	r := nextRecord(t, state, protocol.Block{Version: 1, Network: state.genesis.ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(b)}, key)
	_, changed, _ := s.ObserveRecord(0)
	if err := s.journal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Commit(r); err == nil {
		t.Fatal("closed journal accepted append")
	}
	select {
	case <-changed:
		t.Fatal("failed append emitted notification")
	default:
	}
	if _, _, err := s.ObserveRecord(0); err != s.failed {
		t.Fatal(err)
	}
	if _, _, err := s.ObserveConfirmation(r.Block.ID()); err != s.failed {
		t.Fatal(err)
	}
}
