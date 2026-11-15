package main

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func TestConfigurationPreflightSuccessAndFailure(t *testing.T) {
	t.Setenv("BL_RPC_TOKEN", "")
	dir := t.TempDir()
	account := protocol.Address(ed25519.NewKeyFromSeed(make([]byte, 32)).Public().(ed25519.PublicKey))
	genesis := protocol.Genesis{Version: 1, Name: "preflight", Supply: 100, Allocations: []protocol.Allocation{{Account: account, Representative: account, Balance: 100}}}
	raw, err := json.Marshal(genesis)
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "genesis.json")
	if err := os.WriteFile(manifest, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "node.json")
	if err := os.WriteFile(cfg, []byte(`{"genesis":"genesis.json","data_dir":"data"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--config", cfg, "--validate-config"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "data")); !os.IsNotExist(err) {
		t.Fatal("preflight created runtime state")
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--config", cfg, "--validate-config"}); err == nil {
		t.Fatal("missing startup input accepted")
	}
}
