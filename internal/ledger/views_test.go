package ledger

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func viewFixture(t *testing.T) (*Store, protocol.Account, protocol.Account, func()) {
	t.Helper()
	state, key, other := fixture(t)
	store, err := Open(t.TempDir(), state.genesis)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Address(other.Public().(ed25519.PublicKey))
	appendRecord := func() {
		t.Helper()
		account := state.accounts[a]
		r := nextRecord(t, state, protocol.Block{Version: 1, Network: state.genesis.ID(), Kind: "send", Account: a, Previous: account.Frontier, Representative: a, Balance: account.Balance - 1, Link: string(b)}, key)
		if err := store.Commit(r); err != nil {
			t.Fatal(err)
		}
		state.apply(r)
	}
	for i := 0; i < 5; i++ {
		appendRecord()
	}
	return store, a, b, appendRecord
}
func TestHistoryViewContinuesAtOriginalAnchorAfterNewConfirmation(t *testing.T) {
	store, a, _, appendRecord := viewFixture(t)
	first, err := store.HistoryView(a, Anchor{}, 0, 2)
	if err != nil || len(first.Items) != 2 || !first.More || first.Before != 3 {
		t.Fatalf("first view: %+v %v", first, err)
	}
	first.Items[0].Votes[0].Signature = "changed"
	if store.Records(4, 1)[0].Votes[0].Signature == "changed" {
		t.Fatal("view aliases confirmed certificate")
	}
	appendRecord()
	next, err := store.HistoryView(a, first.Anchor, first.Before, 2)
	if err != nil || next.Items[0].Height != 3 || next.Items[1].Height != 2 {
		t.Fatal("new confirmation shifted anchored history")
	}
	last, err := store.HistoryView(a, next.Anchor, next.Before, 2)
	if err != nil || len(last.Items) != 1 || last.More || last.Items[0].Height != 1 {
		t.Fatal("history continuation skipped final record")
	}
}
func TestReceivableViewRejectsChangedSnapshot(t *testing.T) {
	store, _, b, appendRecord := viewFixture(t)
	first, err := store.ReceivableView(b, Anchor{}, "", 2)
	if err != nil || len(first.Items) != 2 || !first.More {
		t.Fatal("receivable page incomplete")
	}
	next, err := store.ReceivableView(b, first.Anchor, first.After, 2)
	if err != nil || len(next.Items) != 2 || next.Items[0].Source <= first.After {
		t.Fatal("receivable continuation repeated a source")
	}
	appendRecord()
	if _, err := store.ReceivableView(b, first.Anchor, first.After, 2); !errors.Is(err, ErrStaleView) {
		t.Fatal("stale receivable snapshot accepted")
	}
}
