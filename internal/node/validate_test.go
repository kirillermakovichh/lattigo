package node

import (
	"crypto/ed25519"
	"encoding/json"
	"example.com/go-block-lattice/internal/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFilesDoesNotCreateRuntimeState(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{Genesis: filepath.Join(dir, "missing.json"), DataDir: filepath.Join(dir, "data")}
	cfg.defaults()
	if err := cfg.ValidateFiles(); err == nil {
		t.Fatal("missing genesis accepted")
	}
	if _, err := os.Stat(cfg.DataDir); !os.IsNotExist(err) {
		t.Fatal("validation created data directory")
	}
}

func TestExtensionValidateFilesCompleteInputs(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{Genesis: filepath.Join(dir, "genesis.json"), DataDir: filepath.Join(dir, "data")}
	cfg.defaults()
	invalid := cfg
	invalid.RPCListen = "bad"
	if err := invalid.ValidateFiles(); err == nil {
		t.Fatal("invalid config")
	}
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "validation", Supply: 1, Allocations: []protocol.Allocation{{Account: a, Representative: a, Balance: 1}}}
	raw, _ := json.Marshal(g)
	if err := os.WriteFile(cfg.Genesis, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateFiles(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg.DataDir); !os.IsNotExist(err) {
		t.Fatal("validation created ledger")
	}
}
