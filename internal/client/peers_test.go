package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPeerDiagnosticsAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/admin/peers" || r.Header.Get("Authorization") != "Bearer private" {
			t.Error("peer request contract")
		}
		w.Write([]byte(`[{"url":"http://peer","connected":true,"height":4,"queued_bytes":0,"queued_messages":0,"consecutive_failures":0,"retry_at_unix_nano":0}]`))
	}))
	defer server.Close()
	c, _ := New(server.URL, "private")
	rows, err := c.Peers(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Height != 4 {
		t.Fatal(rows, err)
	}
}
