package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurationRejectsUnsafeExposure(t *testing.T) {
	t.Setenv("BL_RPC_TOKEN", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "node.json")
	for _, raw := range []string{`{"genesis":"genesis.json","rpc_listen":"0.0.0.0:7070"}`, `{"genesis":"genesis.json","allow_remote_rpc":true,"rpc_listen":"0.0.0.0:7070","rpc_token":"too-short"}`, `{"genesis":"genesis.json","unknown":true}`, `{"genesis":"genesis.json"} {}`} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(path); err == nil {
			t.Errorf("accepted invalid configuration %s", raw)
		}
	}
	if err := os.WriteFile(path, []byte(`{"genesis":"genesis.json"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Genesis != filepath.Join(dir, "genesis.json") {
		t.Fatalf("relative genesis not resolved: %s", cfg.Genesis)
	}
	if cfg.RPCListen != "127.0.0.1:7070" {
		t.Fatal("unsafe default")
	}
}
