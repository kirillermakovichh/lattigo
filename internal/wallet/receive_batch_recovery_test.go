package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/protocol"
)

func TestReceiveAllRetainsUncertainIntentAndResumesAfterExactRetry(t *testing.T) {
	path, store, handler, recipient := receiveFixture(t)
	var submissions atomic.Int32
	rejected := make(chan protocol.Block, 1)
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/blocks" && submissions.Add(1) == 2 {
			var b protocol.Block
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				t.Error(err)
			}
			rejected <- b
			out.WriteHeader(503)
			json.NewEncoder(out).Encode(map[string]any{"error": map[string]string{"code": "unavailable"}})
			return
		}
		handler.ServeHTTP(out, r)
	}))
	defer server.Close()
	args := []string{"receive-all", "--wallet", path, "--rpc", server.URL, "--limit", "2", "--timeout", "20s"}
	var out bytes.Buffer
	err := RunCommand(context.Background(), args, &out, io.Discard)
	var unknown *client.SubmissionError
	if !errors.As(err, &unknown) {
		t.Fatalf("partial submission uncertainty lost: %v", err)
	}
	attempted := <-rejected
	var result ReceiveBatch
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || len(result.Completed) != 1 || result.Pending == nil || result.Pending.Hash != attempted.ID() || result.Error == "" {
		t.Fatalf("partial receipts: %+v %v", result, err)
	}
	state, err := store.Account(recipient)
	if err != nil || state.Height != 1 || state.Balance == 0 || state.Balance >= 3 {
		t.Fatal("failed batch concealed partial completion")
	}
	entry, err := InspectOutbox(path, 0)
	if err != nil || entry.Block != attempted {
		t.Fatal("failed batch lost original signed intent")
	}
	if err := RunCommand(context.Background(), []string{"outbox", "--action", "retry", "--wait", "--wallet", path, "--rpc", server.URL, "--timeout", "20s"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := RunCommand(context.Background(), args, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	result = ReceiveBatch{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || len(result.Completed) != 0 || result.Pending != nil || result.Error != "" {
		t.Fatal("resumed batch consumed a source twice")
	}
	state, err = store.Account(recipient)
	if err != nil || state.Balance != 3 || state.Height != 2 {
		t.Fatal("resumed batch violated transfer conservation")
	}
}
func TestReceiveAllRejectsInvalidLimitsBeforeOpeningWallet(t *testing.T) {
	for _, limit := range []string{"0", "101"} {
		if err := RunCommand(context.Background(), []string{"receive-all", "--limit", limit, "--wallet", "/definitely/missing/wallet"}, io.Discard, io.Discard); err == nil {
			t.Fatal("invalid batch limit accepted")
		}
	}
}
