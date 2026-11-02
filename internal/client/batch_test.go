package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func TestBatchClientPreservesReadCorrelationAndErrors(t *testing.T) {
	block := clientBlock(t)
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/query" {
			t.Error("wrong batch route")
		}
		var query struct{ Queries []ReadQuery }
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			t.Error(err)
		}
		if len(query.Queries) != 2 {
			t.Error("client changed batch")
		}
		data, err := json.Marshal(AccountState{Account: block.Account, Balance: 10, Representative: block.Representative})
		if err != nil {
			t.Error(err)
		}
		json.NewEncoder(out).Encode([]ReadResult{{ID: "account", Data: data}, {ID: "missing", Error: &Error{Code: "not_found", Message: "missing"}}})
	}))
	defer server.Close()
	c, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := c.Batch(context.Background(), []ReadQuery{{ID: "account", Operation: ReadAccount, Address: block.Account}, {ID: "missing", Operation: ReadBlock, Hash: protocol.Digest([]byte("missing"))}})
	if err != nil {
		t.Fatal(err)
	}
	var state AccountState
	if err := rows[0].Decode(&state); err != nil || state.Balance != 10 {
		t.Fatal("typed batch decoding failed")
	}
	var rpcError *Error
	if err := rows[1].Decode(&state); !errors.As(err, &rpcError) || rpcError.Code != "not_found" {
		t.Fatal("per-query error was lost")
	}
}
func TestBatchClientRejectsMisorderedAndOversizedResponses(t *testing.T) {
	b := clientBlock(t)
	c := branchClient(t, 200, `[{"id":"other","data":{}}]`)
	if _, err := c.Batch(context.Background(), []ReadQuery{{ID: "account", Operation: ReadAccount, Address: b.Account}}); err == nil {
		t.Fatal("misordered batch accepted")
	}
	if _, err := c.Batch(context.Background(), nil); err == nil {
		t.Fatal("empty batch sent")
	}
	if _, err := c.Batch(context.Background(), []ReadQuery{{ID: "x"}, {ID: "x"}}); err == nil {
		t.Fatal("duplicate batch IDs sent")
	}
}
