package wallet

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"example.com/go-block-lattice/internal/client"
)

func TestOutboxCleanupRequiresConfirmedLookup(t *testing.T) {
	path, g, b := ownedOutboxFixture(t)
	var confirmed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		var value any
		switch r.URL.Path {
		case "/v1/genesis":
			value = g
		case "/v1/status":
			value = client.Status{Network: g.ID()}
		case "/v1/block":
			if !confirmed.Load() {
				out.WriteHeader(404)
				value = map[string]any{"error": map[string]string{"code": "not_found"}}
			} else {
				value = b
			}
		default:
			http.NotFound(out, r)
			return
		}
		if err := json.NewEncoder(out).Encode(value); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	args := []string{"outbox", "--wallet", path, "--rpc", server.URL, "--action", "clear"}
	if err := RunCommand(context.Background(), args, io.Discard, io.Discard); err == nil {
		t.Fatal("unconfirmed outbox removed")
	}
	if _, err := InspectOutbox(path, 0); err != nil {
		t.Fatal("failed cleanup lost durable intent")
	}
	confirmed.Store(true)
	if err := RunCommand(context.Background(), args, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectOutbox(path, 0); !os.IsNotExist(err) {
		t.Fatal("confirmed outbox not removed")
	}
}
