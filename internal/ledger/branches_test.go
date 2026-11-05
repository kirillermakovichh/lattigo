package ledger

import (
	"crypto/ed25519"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func TestLedgerTransitionRejectionBranches(t *testing.T) {
	s, key, other := fixture(t)
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Address(other.Public().(ed25519.PublicKey))
	base := protocol.Block{Version: 1, Network: s.genesis.ID(), Kind: "send", Account: a, Representative: a, Balance: 90, Link: string(b)}
	for name, mutate := range map[string]func(*protocol.Block){
		"network": func(b *protocol.Block) { b.Network = protocol.Digest([]byte("other")) }, "signature": func(b *protocol.Block) { b.Signature = "bad" }, "frontier": func(b *protocol.Block) { b.Previous = protocol.Digest([]byte("missing")) }, "zero send": func(b *protocol.Block) { b.Balance = 100 }, "increase": func(b *protocol.Block) { b.Balance = 101 }, "unknown source": func(b *protocol.Block) { b.Kind = "receive"; b.Link = string(protocol.Digest([]byte("source"))) }, "unchanged delegation": func(b *protocol.Block) { b.Kind = "change"; b.Link = ""; b.Balance = 100 }, "change balance": func(b *protocol.Block) {
			b.Kind = "change"
			b.Link = ""
			b.Representative = protocol.Account(base.Link)
		},
	} {
		t.Run(name, func(t *testing.T) {
			block := base
			mutate(&block)
			if name != "signature" {
				if err := block.Sign(key); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.validate(block); err == nil {
				t.Fatal("invalid transition accepted")
			}
		})
	}
	unopened := base
	unopened.Account = b
	if err := unopened.Sign(other); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(s.validate(unopened), ErrDependency) {
		t.Fatal("unopened sender accepted")
	}
	send := nextRecord(t, s, base, key)
	if err := s.validateRecord(send); err != nil {
		t.Fatal(err)
	}
	s.apply(send)
	if err := s.validate(send.Block); err != nil {
		t.Fatal("confirmed retry rejected")
	}
	wrong := protocol.Block{Version: 1, Network: s.genesis.ID(), Kind: "receive", Account: a, Previous: send.Block.ID(), Representative: a, Balance: 100, Link: string(send.Block.ID())}
	if err := wrong.Sign(key); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(s.validate(wrong), ErrInvalid) {
		t.Fatal("another destination received funds")
	}
	receive := protocol.Block{Version: 1, Network: s.genesis.ID(), Kind: "receive", Account: b, Representative: b, Balance: 11, Link: string(send.Block.ID())}
	if err := receive.Sign(other); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(s.validate(receive), ErrInvalid) {
		t.Fatal("incorrect receive balance accepted")
	}
	receive.Previous = send.Block.ID()
	if err := receive.Sign(other); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(s.validate(receive), ErrDependency) {
		t.Fatal("opening with previous accepted")
	}
	workState, workKey, _ := fixture(t)
	workState.genesis.WorkBits = 8
	workState = newState(workState.genesis)
	base.Network = workState.genesis.ID()
	if err := base.Sign(workKey); err != nil {
		t.Fatal(err)
	}
	for base.CheckWork(8) {
		base.Work++
	}
	if !errors.Is(workState.validate(base), ErrInvalid) {
		t.Fatal("insufficient admission work accepted")
	}
}
func TestVoteAndCertificateRejectionBranches(t *testing.T) {
	s, key, other := fixture(t)
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Address(other.Public().(ed25519.PublicKey))
	record := nextRecord(t, s, protocol.Block{Version: 1, Network: s.genesis.ID(), Kind: "send", Account: a, Representative: a, Balance: 90, Link: string(b)}, key)
	for name, mutate := range map[string]func(*Vote){"network": func(v *Vote) { v.Network = "bad" }, "parent": func(v *Vote) { v.Parent = "bad" }, "height": func(v *Vote) { v.Height = 0 }, "phase": func(v *Vote) { v.Phase = "bad" }, "block": func(v *Vote) { v.Block = "bad" }, "representative": func(v *Vote) { v.Representative = "bad" }} {
		t.Run(name, func(t *testing.T) {
			vote := record.Votes[0]
			mutate(&vote)
			if vote.Verify() == nil {
				t.Fatal("invalid vote verified")
			}
			if _, err := vote.signingBytes(); err == nil {
				t.Fatal("invalid vote encoded")
			}
		})
	}
	vote := record.Votes[0]
	if vote.Sign(nil) == nil || vote.Sign(other) == nil {
		t.Fatal("invalid vote signer accepted")
	}
	for _, signature := range []string{"bad", "00", strings.Repeat("00", 64)} {
		vote := record.Votes[0]
		vote.Signature = signature
		if vote.Verify() == nil {
			t.Fatal("invalid vote signature accepted")
		}
	}
	for name, mutate := range map[string]func(*Record){"height": func(r *Record) { r.Height++ }, "parent": func(r *Record) { r.Previous = protocol.Digest([]byte("other")) }, "no votes": func(r *Record) { r.Votes = nil }, "oversized": func(r *Record) { r.Votes = make([]Vote, protocol.MaxRepresentatives+1) }, "duplicate": func(r *Record) { r.Votes = append(r.Votes, r.Votes[0]) }, "binding": func(r *Record) { r.Votes[0].Round++ }, "signature": func(r *Record) { r.Votes[0].Signature = "bad" }, "weight": func(r *Record) {
		r.Votes[0].Representative = b
		if err := r.Votes[0].Sign(other); err != nil {
			t.Fatal(err)
		}
	}} {
		t.Run(name, func(t *testing.T) {
			r := copyRecord(record)
			mutate(&r)
			if s.validateRecord(r) == nil {
				t.Fatal("invalid record accepted")
			}
		})
	}
	if err := certificate(record, s.genesis.ID(), map[protocol.Account]protocol.Amount{a: 1}, 100); err == nil {
		t.Fatal("minority certificate accepted")
	}
	for _, r := range []Record{{}, {Height: 1, Previous: "bad"}, {Height: 1, Previous: s.tip}} {
		if r.ID() != "" {
			t.Fatal("invalid record has an identity")
		}
	}
	for _, q := range []struct{ w, total protocol.Amount }{{0, 0}, {2, 3}, {101, 100}} {
		if HasQuorum(q.w, q.total) {
			t.Fatal("invalid quorum accepted")
		}
	}
}
func TestStoreClosedAndRestoreTargetBranches(t *testing.T) {
	state, key, other := fixture(t)
	store, err := Open(t.TempDir(), state.genesis)
	if err != nil {
		t.Fatal(err)
	}
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Address(other.Public().(ed25519.PublicKey))
	r := nextRecord(t, state, protocol.Block{Version: 1, Network: state.genesis.ID(), Kind: "send", Account: a, Representative: a, Balance: 90, Link: string(b)}, key)
	if err = store.Commit(r); err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(r); err != nil {
		t.Fatal("retry not idempotent")
	}
	conflict := r
	conflict.Block.Balance--
	if !errors.Is(store.Commit(conflict), ErrConflict) {
		t.Fatal("confirmed history replaced")
	}
	if len(store.History(a, -1, 0)) != 1 || len(store.Records(0, 2000)) != 1 || len(store.Receivables(b, 9999, 2000)) != 0 {
		t.Fatal("page defaults or clamp incorrect")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	_, accountErr := store.Account(a)
	_, blockErr := store.Block(r.Block.ID())
	_, signErr := store.SigningState()
	for _, err := range []error{accountErr, blockErr, signErr, store.Validate(r.Block), store.Commit(r), store.SaveSigningState([]byte("vote")), store.Backup(io.Discard)} {
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("closed operation:%v", err)
		}
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err = os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if Restore(file, strings.NewReader("bad"), state.genesis) == nil {
		t.Fatal("restore over file accepted")
	}
	link := filepath.Join(dir, "link")
	if err = os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if Restore(link, strings.NewReader("bad"), state.genesis) == nil {
		t.Fatal("restore through symlink accepted")
	}
	for _, limit := range []uint64{1, 1<<40 + 1} {
		if _, err := OpenWithOptions(filepath.Join(dir, "unused"), state.genesis, Options{MaxHistoryBytes: limit}); err == nil {
			t.Fatal("invalid history limit accepted")
		}
	}
	if _, err := OpenExisting(link, state.genesis, Options{}); err == nil {
		t.Fatal("inspect through symlink accepted")
	}
}

func TestLedgerFailedPersistenceAndPaginationBranches(t *testing.T) {
	state, key, other := fixture(t)
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Address(other.Public().(ed25519.PublicKey))
	dir := t.TempDir()
	store, err := Open(dir, state.genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	r := nextRecord(t, state, protocol.Block{Version: 1, Network: state.genesis.ID(), Kind: "send", Account: a, Representative: a, Balance: 90, Link: string(b)}, key)
	if len(store.Receivables(a, -1, 0)) != 0 || len(store.Records(0, 0)) != 0 {
		t.Fatal("empty pages invalid")
	}
	if err = store.Commit(r); err != nil {
		t.Fatal(err)
	}
	if len(store.Receivables(a, 0, 1)) != 0 || len(store.Receivables(b, -1, 0)) != 1 || len(store.Receivables(b, 0, 1)) != 1 || len(store.History(b, 0, 2000)) != 0 || len(store.Records(0, 0)) != 1 {
		t.Fatal("pagination filtering or limits failed")
	}
	if err = os.Mkdir(filepath.Join(dir, "signing.json"), 0700); err != nil {
		t.Fatal(err)
	}
	failed := store.SaveSigningState([]byte("signing intent"))
	if failed == nil {
		t.Fatal("signing write failure ignored")
	}
	for _, err := range []error{store.Validate(r.Block), store.Commit(r), store.SaveSigningState([]byte("again")), store.Backup(io.Discard)} {
		if err == nil {
			t.Fatal("failed store resumed critical work")
		}
	}
	limited, err := Open(t.TempDir(), state.genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer limited.Close()
	limited.historyBytes = limited.historyLimit
	if !errors.Is(limited.Commit(r), ErrCapacity) {
		t.Fatal("exact commit byte limit ignored")
	}
	if _, err = Open(t.TempDir(), protocol.Genesis{}); err == nil {
		t.Fatal("invalid genesis opened")
	}
	empty := t.TempDir()
	if _, err = OpenExisting(empty, state.genesis, Options{}); err == nil {
		t.Fatal("missing WAL inspected")
	}
	if err = os.Mkdir(filepath.Join(empty, "ledger.wal"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenExisting(empty, state.genesis, Options{}); err == nil {
		t.Fatal("directory WAL inspected")
	}
}

func TestVoteSigningRejectsInvalidPayloadBeforeSignature(t *testing.T) {
	_, key, _ := fixture(t)
	vote := Vote{Representative: protocol.Address(key.Public().(ed25519.PublicKey))}
	if err := vote.Sign(key); err == nil || vote.Signature != "" {
		t.Fatal("malformed vote signed")
	}
}
