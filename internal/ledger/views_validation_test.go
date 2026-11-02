package ledger

import (
	"errors"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func TestSnapshotViewsRejectInvalidAndClosedReads(t *testing.T) {
	store, a, b, _ := viewFixture(t)
	for _, limit := range []int{0, 1001} {
		if _, err := store.HistoryView(a, Anchor{}, 0, limit); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid history limit accepted")
		}
		if _, err := store.ReceivableView(b, Anchor{}, "", limit); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid receivable limit accepted")
		}
	}
	if _, err := store.HistoryView("bad", Anchor{}, 0, 1); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid view account accepted")
	}
	if _, err := store.ReceivableView(b, Anchor{}, "bad", 1); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid continuation source accepted")
	}
	for _, anchor := range []Anchor{{Height: 1}, {Height: 9, Tip: protocol.Digest([]byte("tip"))}, {Height: 0, Tip: protocol.Digest([]byte("tip"))}} {
		if _, err := store.HistoryView(a, anchor, 0, 1); err == nil {
			t.Fatal("invalid snapshot anchor accepted")
		}
	}
	first, err := store.HistoryView(a, Anchor{}, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.HistoryView(a, first.Anchor, first.Anchor.Height+1, 1); !errors.Is(err, ErrInvalid) {
		t.Fatal("history escaped its snapshot")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.HistoryView(a, Anchor{}, 0, 1); !errors.Is(err, ErrClosed) {
		t.Fatal("closed history view accepted")
	}
	if _, err := store.ReceivableView(b, Anchor{}, "", 1); !errors.Is(err, ErrClosed) {
		t.Fatal("closed receivable view accepted")
	}
}
