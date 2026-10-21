package node

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

func TestNodeConfigurationRejectionBranches(t *testing.T) {
	t.Setenv("BL_RPC_TOKEN", "")
	base := Config{Genesis: "genesis.json"}
	base.defaults()
	for name, mutate := range map[string]func(*Config){"genesis": func(c *Config) { c.Genesis = "" }, "data": func(c *Config) { c.DataDir = "" }, "timeout low": func(c *Config) { c.RoundTimeoutMS = 99 }, "timeout high": func(c *Config) { c.RoundTimeoutMS = 60001 }, "pending low": func(c *Config) { c.MaxPending = -1 }, "pending high": func(c *Config) { c.MaxPending = 65537 }, "peers low": func(c *Config) { c.MaxPeers = -1 }, "peers high": func(c *Config) { c.MaxPeers = 129 }, "too many peers": func(c *Config) { c.MaxPeers = 1; c.Peers = []string{"http://a.invalid", "http://b.invalid"} }, "listener": func(c *Config) { c.P2PListen = "bad" }, "hostname": func(c *Config) { c.P2PListen = "bad.invalid:7071" }, "empty port": func(c *Config) { c.P2PListen = "127.0.0.1:" }, "remote": func(c *Config) { c.RPCListen = "0.0.0.0:7070" }, "remote token": func(c *Config) { c.RPCListen = "0.0.0.0:7070"; c.AllowRemoteRPC = true }, "token": func(c *Config) { c.RPCToken = "short" }, "token newline": func(c *Config) { c.RPCToken = strings.Repeat("x", 32) + "\n" }, "same listeners": func(c *Config) { c.P2PListen = c.RPCListen }, "peer origin": func(c *Config) { c.Peers = []string{"file:///tmp/path"} }, "history": func(c *Config) { c.HistoryLimitBytes = 1<<40 + 1 }} {
		t.Run(name, func(t *testing.T) {
			c := base
			mutate(&c)
			if c.Validate() == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
	valid := base
	valid.RPCListen = "0.0.0.0:7070"
	valid.AllowRemoteRPC = true
	valid.RPCToken = strings.Repeat("x", 32)
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BL_RPC_TOKEN", strings.Repeat("e", 32))
	valid.defaults()
	if valid.RPCToken != strings.Repeat("e", 32) {
		t.Fatal("environment token not applied")
	}
	dir := t.TempDir()
	for _, name := range []string{"missing", "directory", "large", "bad"} {
		path := filepath.Join(dir, name)
		switch name {
		case "directory":
			os.Mkdir(path, 0700)
		case "large":
			os.WriteFile(path, make([]byte, (1<<20)+1), 0600)
		case "bad":
			os.WriteFile(path, []byte("{"), 0600)
		}
		if _, err := LoadConfig(path); err == nil {
			t.Fatalf("bad configuration%s accepted", name)
		}
	}
	for _, content := range []string{"bad", strings.Repeat("0", 257)} {
		path := filepath.Join(dir, "key")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadRepresentative(path); err == nil {
			t.Fatal("invalid representative key accepted")
		}
	}
	if _, err := loadRepresentative(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing representative accepted")
	}
	if _, err := loadRepresentative(dir); err == nil {
		t.Fatal("directory key accepted")
	}
	key := filepath.Join(dir, "key")
	if err := os.WriteFile(key, []byte(strings.Repeat("0", 64)), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chmod(key, 0644)
	if _, err := loadRepresentative(key); err == nil {
		t.Fatal("exposed representative accepted")
	}
}
func TestNodeStartupFailureReleasesDataLock(t *testing.T) {
	t.Setenv("BL_RPC_TOKEN", "")
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "startup", Supply: 100, Allocations: []protocol.Allocation{{Account: a, Balance: 100, Representative: a}}}
	for _, name := range []string{"configuration", "genesis", "representative", "storage", "consensus", "rpc listener", "peer listener", "canceled"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			manifest := filepath.Join(dir, "genesis.json")
			data, err := json.Marshal(g)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(manifest, data, 0600); err != nil {
				t.Fatal(err)
			}
			cfg := Config{Genesis: manifest, DataDir: filepath.Join(dir, "data"), RPCListen: "127.0.0.1:0", P2PListen: "[::1]:0"}
			cfg.defaults()
			switch name {
			case "configuration":
				cfg.RoundTimeoutMS = -1
			case "genesis":
				cfg.Genesis = filepath.Join(dir, "missing")
			case "representative":
				cfg.RepresentativeKey = filepath.Join(dir, "missing")
			case "storage":
				if err = os.WriteFile(cfg.DataDir, []byte("file"), 0600); err != nil {
					t.Fatal(err)
				}
			case "consensus":
				store, err := ledger.Open(cfg.DataDir, g)
				if err != nil {
					t.Fatal(err)
				}
				if err = store.SaveSigningState([]byte("{")); err != nil {
					t.Fatal(err)
				}
				store.Close()
			case "rpc listener", "peer listener":
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				if name == "rpc listener" {
					cfg.RPCListen = listener.Addr().String()
				} else {
					cfg.P2PListen = listener.Addr().String()
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err = Run(ctx, cfg)
			if name == "canceled" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("startup failure ignored")
			}
			if name == "consensus" || name == "rpc listener" || name == "peer listener" || name == "canceled" {
				store, err := ledger.OpenExisting(cfg.DataDir, g, ledger.Options{})
				if err != nil {
					t.Fatalf("startup leaked data lock:%v", err)
				}
				store.Close()
			}
		})
	}
}
