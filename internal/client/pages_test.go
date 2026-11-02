package client

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func TestCursorClientPreservesOpaqueContinuationAndTypedRows(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	tip := protocol.Digest([]byte("page-tip"))
	source := protocol.Digest([]byte("source"))
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("address") != string(a) || r.URL.Query().Get("cursor") != "opaque-cursor" {
			t.Error("client changed cursor scope")
		}
		if r.URL.Query().Get("limit") != "" {
			t.Error("client overrode a continuation's page size")
		}
		value := Page[Receivable]{Anchor: Anchor{Height: 1, Tip: tip}, Items: []Receivable{{Source: source, From: a, To: a, Amount: 1}}}
		if err := json.NewEncoder(out).Encode(value); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	c, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	page, err := c.ReceivablePage(context.Background(), a, "opaque-cursor", 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].Source != source {
		t.Fatalf("typed page: %+v %v", page, err)
	}
}
