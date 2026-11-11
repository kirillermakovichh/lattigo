package client

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProfileDownloadAndValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing authentication")
		}
		w.Write([]byte("profile"))
	}))
	defer server.Close()
	c, _ := New(server.URL, "token")
	var b bytes.Buffer
	if err := c.Profile(context.Background(), "heap", 1, &b); err != nil || b.String() != "profile" {
		t.Fatal(err)
	}
	if err := c.Profile(context.Background(), "unknown", 1, &b); err == nil {
		t.Fatal("invalid kind accepted")
	}
	if err := c.Profile(context.Background(), "cpu", 6, &b); err == nil {
		t.Fatal("duration budget ignored")
	}
}
