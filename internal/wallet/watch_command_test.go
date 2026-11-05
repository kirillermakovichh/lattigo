package wallet

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/event"
	"example.com/go-block-lattice/internal/protocol"
)

func TestWatchExplicitAddressNeedsNoWallet(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	network := protocol.Digest([]byte("watch"))
	b := protocol.Block{Version: 1, Network: network, Kind: "send", Account: a, Representative: a, Balance: 1, Link: string(a)}
	if err := b.Sign(key); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status" {
			_ = json.NewEncoder(w).Encode(client.Status{Network: network})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		data, _ := json.Marshal(map[string]any{"height": 1, "block": b})
		_, _ = fmt.Fprintf(w, "id: %s\nevent: confirmed\ndata: %s\n\n", event.ID(network, 1), data)
	}))
	defer server.Close()
	var out bytes.Buffer
	err := RunCommand(context.Background(), []string{"watch", "--rpc", server.URL, "--wallet", t.TempDir() + "/absent", "--address", string(a), "--count", "1"}, &out, &out)
	if err != nil || !bytes.Contains(out.Bytes(), []byte(event.ID(network, 1))) {
		t.Fatal(out.String(), err)
	}
}
