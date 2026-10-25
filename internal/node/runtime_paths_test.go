package node

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

func runtimeFixture(t *testing.T) (Config, protocol.Genesis) {
	t.Helper()
	t.Setenv("BL_RPC_TOKEN", "")
	dir := t.TempDir()
	seed := make([]byte, 32)
	key := ed25519.NewKeyFromSeed(seed)
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "runtime-failure", Supply: 100, Allocations: []protocol.Allocation{{Account: a, Balance: 100, Representative: a}}}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "genesis.json")
	if err = os.WriteFile(manifest, raw, 0600); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "representative")
	if err = os.WriteFile(keyPath, []byte(hex.EncodeToString(seed)), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Genesis: manifest, DataDir: filepath.Join(dir, "data"), RepresentativeKey: keyPath, RPCListen: "127.0.0.1:0", P2PListen: "[::1]:0", RoundTimeoutMS: 100}
	cfg.defaults()
	return cfg, g
}

func TestNodeStopsAndReleasesLedgerOnRuntimeSigningFailure(t *testing.T) {
	cfg, g := runtimeFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg) }()
	signing := filepath.Join(cfg.DataDir, "signing.json")
	deadline := time.After(time.Second)
	for {
		if _, err := os.Stat(signing); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("node stopped before signing initialization: %v", err)
		case <-deadline:
			t.Fatal("signer did not initialize")
		case <-time.After(time.Millisecond):
		}
	}
	if err := os.Remove(signing); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(signing, 0700); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "signing") {
			t.Fatalf("runtime signing failure: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("runtime failure did not stop node")
	}
	s, err := ledger.OpenExisting(cfg.DataDir, g, ledger.Options{})
	if err != nil {
		t.Fatalf("runtime failure retained ledger lock: %v", err)
	}
	s.Close()
}
