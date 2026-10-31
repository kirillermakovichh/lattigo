package wallet

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/consensus"
	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
	"example.com/go-block-lattice/internal/rpc"
)

func receiveFixture(t *testing.T) (string, *ledger.Store, http.Handler, protocol.Account) {
	t.Helper()
	pass := []byte("private batch receive passphrase")
	t.Setenv("BL_WALLET_PASSWORD", string(pass))
	t.Setenv("BL_RPC_TOKEN", "private-batch-receive-rpc-token")
	path := filepath.Join(t.TempDir(), "wallet")
	w, err := Import(path, make([]byte, 32), pass)
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := keyAccount(w, 0)
	w.Close()
	if err != nil {
		t.Fatal(err)
	}
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32))
	payer := protocol.Address(key.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "receive-batch", Supply: 100, Allocations: []protocol.Allocation{{Account: payer, Representative: payer, Balance: 100}}}
	store, err := ledger.Open(t.TempDir(), g)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	engine, err := consensus.New(consensus.Config{Store: store, PrivateKey: key, RoundTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	for _, amount := range []protocol.Amount{1, 2} {
		state, err := store.Account(payer)
		if err != nil {
			t.Fatal(err)
		}
		b := protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: payer, Previous: state.Frontier, Representative: payer, Balance: state.Balance - amount, Link: string(recipient)}
		if err := b.Sign(key); err != nil {
			t.Fatal(err)
		}
		if _, err := engine.Submit(b); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	api, err := rpc.New(rpc.Config{Store: store, Engine: engine, Token: "private-batch-receive-rpc-token"})
	if err != nil {
		t.Fatal(err)
	}
	return path, store, api.Handler(), recipient
}
func TestReceiveAllConfirmsBeforeAdvancingAccount(t *testing.T) {
	path, store, handler, recipient := receiveFixture(t)
	server := httptest.NewServer(handler)
	defer server.Close()
	var out bytes.Buffer
	if err := RunCommand(context.Background(), []string{"receive-all", "--wallet", path, "--rpc", server.URL, "--limit", "2", "--timeout", "20s"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	var result ReceiveBatch
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || len(result.Completed) != 2 || !result.LimitReached || result.Pending != nil {
		t.Fatalf("batch result: %+v %v", result, err)
	}
	for _, receipt := range result.Completed {
		if receipt.Status != "confirmed" {
			t.Fatal("unconfirmed receive counted as completed")
		}
	}
	state, err := store.Account(recipient)
	if err != nil || state.Balance != 3 || state.Height != 2 {
		t.Fatalf("recipient state: %+v %v", state, err)
	}
}
