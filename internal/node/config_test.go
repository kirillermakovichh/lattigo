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
	for _, raw := range []string{`{"genesis":"genesis.json","rpc_listen":"0.0.0.0:7070"}`, `{"genesis":"genesis.json","allow_remote_rpc":true,"rpc_listen":"0.0.0.0:7070","rpc_token":"too-short"}`, `{"genesis":"genesis.json","unknown":true}`, `{"genesis":"genesis.json","history_limit_bytes":1}`, `{"genesis":"genesis.json"} {}`} {
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

func TestListenerPortsAreValidatedBeforeStartup(t *testing.T) {
	for _, listen := range []string{"127.0.0.1:-1", "127.0.0.1:65536", "127.0.0.1:unknown-service", "127.0.0.1:+7070"} {
		cfg := Config{Genesis: "genesis.json", RPCListen: listen, P2PListen: "127.0.0.1:7071"}
		cfg.defaults()
		if err := cfg.Validate(); err == nil {
			t.Errorf("invalid listener accepted:%s", listen)
		}
	}
}
