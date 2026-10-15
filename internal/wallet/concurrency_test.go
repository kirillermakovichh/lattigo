package wallet

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/protocol"
)

func TestConcurrentPaymentsPreserveSingleAccountOutbox(t *testing.T) {
	password := []byte("a private test passphrase")
	t.Setenv("BL_WALLET_PASSWORD", string(password))
	path := filepath.Join(t.TempDir(), "wallet.json")
	wallet, err := Import(path, bytes.Repeat([]byte{1}, 32), password)
	if err != nil {
		t.Fatal(err)
	}
	key, err := wallet.PrivateKey(0)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	wallet.Close()
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	destination := protocol.Address(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{2}, 32)).Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "outbox-test", Supply: 100, Allocations: []protocol.Allocation{{Account: a, Balance: 100, Representative: a}}}
	previous := protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: a, Representative: a, Balance: 90, Link: string(destination)}
	if err = previous.Sign(key); err != nil {
		t.Fatal(err)
	}
	pending := fmt.Sprintf("%s.pending-0.json", path)
	data, err := json.Marshal(previous)
	if err != nil {
		t.Fatal(err)
	}
	if err = WritePrivate(pending, data); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	resume := make(chan struct{})
	var release sync.Once
	unblock := func() { release.Do(func() { close(resume) }) }
	defer unblock()
	var reads, submits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var value any
		switch r.URL.Path {
		case "/v1/block":
			if reads.Add(1) == 1 {
				close(entered)
				select {
				case <-resume:
				case <-r.Context().Done():
					return
				}
			}
			value = previous
		case "/v1/genesis":
			value = g
		case "/v1/status":
			value = client.Status{Network: g.ID(), Supply: 100, Height: 1}
		case "/v1/account":
			value = client.AccountState{Account: a, Frontier: previous.ID(), Balance: 90, Representative: a, Height: 1}
		case "/v1/blocks":
			var b protocol.Block
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				http.Error(w, "bad block", 400)
				return
			}
			submits.Add(1)
			w.WriteHeader(http.StatusAccepted)
			value = client.Submission{Hash: b.ID(), Status: "admitted"}
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(value)
	}))
	defer server.Close()
	command := func(amount string) error {
		return RunCommand(context.Background(), []string{"send", "--wallet", path, "--rpc", server.URL, "--to", string(destination), "--amount", amount, "--timeout", "5s"}, io.Discard, io.Discard)
	}
	done := make(chan error, 1)
	go func() { done <- command("1") }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		unblock()
		t.Fatal("first payment never reached the pending lookup")
	}
	secondErr := command("2")
	unblock()
	if err = <-done; err != nil {
		t.Fatalf("first payment: %v", err)
	}
	if secondErr == nil {
		t.Error("concurrent command replaced another account's signed intent")
	}
	if submits.Load() != 1 {
		t.Errorf("submitted %d competing candidates", submits.Load())
	}
	saved, err := loadBlock(pending)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Balance != 89 {
		t.Fatal("durable outbox lost the first payment")
	}
}
