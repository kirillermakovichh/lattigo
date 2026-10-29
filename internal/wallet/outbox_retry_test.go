package wallet

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/protocol"
)

func ownedOutboxFixture(t *testing.T) (string, protocol.Genesis, protocol.Block) {
	t.Helper()
	pass := []byte("private outbox passphrase")
	t.Setenv("BL_WALLET_PASSWORD", string(pass))
	path := filepath.Join(t.TempDir(), "wallet")
	w, err := Import(path, make([]byte, 32), pass)
	if err != nil {
		t.Fatal(err)
	}
	account, err := keyAccount(w, 0)
	w.Close()
	if err != nil {
		t.Fatal(err)
	}
	g := protocol.Genesis{Version: 1, Name: "outbox-retry", Supply: 100, Allocations: []protocol.Allocation{{Account: account, Representative: account, Balance: 100}}}
	w, err = Open(path, pass)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	b := protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: account, Representative: account, Balance: 99, Link: string(account)}
	if err := signWork(context.Background(), w, 0, &b, 0); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	file, err := outboxFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePrivate(file, raw); err != nil {
		t.Fatal(err)
	}
	return path, g, b
}
func TestOutboxRetryKeepsExactSignedIntent(t *testing.T) {
	path, g, b := ownedOutboxFixture(t)
	posted := make(chan protocol.Block, 1)
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		var value any
		switch r.URL.Path {
		case "/v1/genesis":
			value = g
		case "/v1/status":
			value = client.Status{Network: g.ID()}
		case "/v1/blocks":
			var got protocol.Block
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Error(err)
			}
			posted <- got
			value = client.Submission{Hash: got.ID(), Status: "admitted"}
		default:
			http.NotFound(out, r)
			return
		}
		if err := json.NewEncoder(out).Encode(value); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	if err := RunCommand(context.Background(), []string{"outbox", "--action", "retry", "--wallet", path, "--rpc", server.URL}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	got := <-posted
	if got != b {
		t.Fatal("retry changed signed intent")
	}
	file, err := outboxFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("admission removed unconfirmed outbox")
	}
}
