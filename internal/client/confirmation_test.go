package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestConfirmationClientValidatesSignedBlock(t *testing.T) {
	b := clientBlock(t)
	raw, err := json.Marshal(Confirmation{Hash: b.ID(), Confirmed: true, Block: &b})
	if err != nil {
		t.Fatal(err)
	}
	c := branchClient(t, 200, string(raw))
	if err := c.WaitConfirmed(context.Background(), b.ID()); err != nil {
		t.Fatal(err)
	}
	malformed, err := json.Marshal(Confirmation{Hash: b.ID(), Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	c = branchClient(t, 200, string(malformed))
	if _, err := c.Confirmation(context.Background(), b.ID(), time.Second); err == nil {
		t.Fatal("confirmation without block proof accepted")
	}
}
func TestConfirmationClientFallsBackForOlderNodes(t *testing.T) {
	b := clientBlock(t)
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/confirmation" {
			http.NotFound(out, r)
			return
		}
		json.NewEncoder(out).Encode(b)
	}))
	defer server.Close()
	c, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WaitConfirmed(context.Background(), b.ID()); err != nil {
		t.Fatal(err)
	}
}
