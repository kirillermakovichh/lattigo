package p2p

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/consensus"
	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

func TestPeerURLsRestrictDialTargets(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:7071", "https://node.example:443/", "http://[::1]:7071"} {
		if _, err := ValidatePeer(raw); err != nil {
			t.Errorf("valid peer %q: %v", raw, err)
		}
	}
	for _, raw := range []string{"file:///tmp/key", "http://user:password@localhost:7071", "http://localhost:7071/secret", "http://localhost:7071?q=1", "http://localhost:7071#fragment", "http://localhost:99999", "localhost:7071"} {
		if _, err := ValidatePeer(raw); err == nil {
			t.Errorf("accepted unsafe peer %q", raw)
		}
	}
}

func TestCertifiedCatchupOverHTTP(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	source := protocol.Address(key.Public().(ed25519.PublicKey))
	destination := protocol.Address(pub)
	genesis := protocol.Genesis{Version: 1, Name: "catchup-test", Supply: 100, Allocations: []protocol.Allocation{{Account: source, Balance: 100, Representative: source}}}
	voterStore, err := ledger.Open(t.TempDir(), genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer voterStore.Close()
	voter, err := consensus.New(consensus.Config{Store: voterStore, PrivateKey: key, RoundTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	network, err := New(Config{Store: voterStore, Engine: voter})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(network.Handler())
	defer server.Close()
	observerStore, err := ledger.Open(t.TempDir(), genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer observerStore.Close()
	observer, err := consensus.New(consensus.Config{Store: observerStore, RoundTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	follower, err := New(Config{Store: observerStore, Engine: observer, Peers: []string{server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = follower.Run(ctx) }()
	defer func() { cancel(); <-done }()
	block := protocol.Block{Version: 1, Network: genesis.ID(), Kind: "send", Account: source, Representative: source, Balance: 90, Link: string(destination)}
	if err := block.Sign(key); err != nil {
		t.Fatal(err)
	}
	if _, err := voter.Submit(block); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for observerStore.Status().Height != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("certified catchup failed: voter=%+v observer=%+v", voterStore.Status(), observerStore.Status())
		}
		time.Sleep(10 * time.Millisecond)
	}
	received := observerStore.Receivables(destination, 0, 10)
	if len(received) != 1 || received[0].Amount != 10 {
		t.Fatalf("catchup lost payment: %+v", received)
	}
}
