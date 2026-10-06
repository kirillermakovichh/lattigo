package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorEnvelopeAndRedirectPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":{"code":"unavailable","message":"syncing"}}`))
			return
		}
		http.Redirect(w, r, "https://example.invalid", 302)
	}))
	defer server.Close()
	c, err := New(server.URL, "secret")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Status(context.Background())
	var e *Error
	if !errors.As(err, &e) || e.Code != "unavailable" {
		t.Fatalf("error contract: %v", err)
	}
	_, err = c.Genesis(context.Background())
	if err == nil {
		t.Fatal("followed RPC redirect")
	}
	if _, err = New(server.URL+"/unexpected/path", ""); err == nil {
		t.Fatal("accepted RPC path")
	}
}
