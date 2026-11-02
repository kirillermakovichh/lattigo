package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/protocol"
)

func TestConfirmationWaitObservesDurableBlock(t *testing.T) {
	s, key := branchServer(t)
	g := s.cfg.Store.Genesis()
	a := g.Allocations[0].Account
	b := protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(a)}
	if err := b.Sign(key); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan confirmationResult, 1)
	failed := make(chan error, 1)
	go func() {
		request, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/v1/confirmation?id="+string(b.ID())+"&wait_ms=1000", nil)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			failed <- err
			return
		}
		defer response.Body.Close()
		var result confirmationResult
		err = json.NewDecoder(response.Body).Decode(&result)
		if err != nil {
			failed <- err
			return
		}
		done <- result
	}()
	for len(s.waitSlots) == 0 {
		select {
		case err := <-failed:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal("wait was not admitted")
		case <-time.After(time.Millisecond):
		}
	}
	if _, err := s.cfg.Engine.Submit(b); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if !result.Confirmed || result.Hash != b.ID() || result.Block == nil || result.Block.ID() != b.ID() {
			t.Fatal("wait did not return confirmed block")
		}
	case err := <-failed:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("wait did not finish")
	}
}
func TestConfirmationLookupDoesNotClaimAdmission(t *testing.T) {
	s, _ := branchServer(t)
	id := protocol.Digest([]byte("unknown"))
	out := httptest.NewRecorder()
	s.Handler().ServeHTTP(out, httptest.NewRequest("GET", "/v1/confirmation?id="+string(id), nil))
	var result confirmationResult
	if err := json.Unmarshal(out.Body.Bytes(), &result); err != nil || out.Code != 200 || result.Confirmed || result.Block != nil || result.Hash != id {
		t.Fatal("unknown confirmation lookup fabricated certainty")
	}
}
