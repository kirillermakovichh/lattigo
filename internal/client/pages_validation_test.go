package client

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func TestCursorClientRejectsInvalidPageEnvelopes(t *testing.T) {
	b := clientBlock(t)
	a := b.Account
	tip := protocol.Digest([]byte("tip"))
	for _, page := range []Page[json.RawMessage]{{Items: nil, Anchor: Anchor{Tip: tip}}, {Items: []json.RawMessage{}, Anchor: Anchor{Tip: tip}, Next: "no-progress"}, {Items: []json.RawMessage{}, Anchor: Anchor{Tip: "bad"}}, {Items: []json.RawMessage{json.RawMessage(`{}`)}, Anchor: Anchor{Tip: tip}, Next: strings.Repeat("x", 1025)}} {
		raw, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		c := branchClient(t, 200, string(raw))
		if _, err := c.HistoryPage(context.Background(), a, "", 1); err == nil {
			t.Fatal("invalid history page accepted")
		}
	}
	c := branchClient(t, 200, `{}`)
	for _, item := range []struct {
		account protocol.Account
		cursor  string
		limit   int
	}{{"bad", "", 1}, {a, "", -1}, {a, "", 1001}, {a, strings.Repeat("x", 1025), 1}} {
		if _, err := c.HistoryPage(context.Background(), item.account, item.cursor, item.limit); err == nil {
			t.Fatal("invalid page input accepted")
		}
	}
}
