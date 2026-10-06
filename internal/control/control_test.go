package control

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func TestExplicitGenesisAndPrivateConfiguration(t *testing.T) {
	dir := t.TempDir()
	var out, errout bytes.Buffer
	seed := make([]byte, 32)
	seed[0] = 4
	key := ed25519.NewKeyFromSeed(seed)
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	genesis := filepath.Join(dir, "genesis.json")
	if e := RunControl(context.Background(), []string{"genesis", "--name", "isolated-test", "--account", string(a), "--balance", "1000", "--work-bits", "0", "--out", genesis}, &out, &errout); e != nil {
		t.Fatal(e)
	}
	g, e := protocol.LoadGenesis(genesis)
	if e != nil {
		t.Fatal(e)
	}
	if g.Supply != 1000 || g.Allocations[0].Representative != a {
		t.Fatal("incorrect genesis")
	}
	config := filepath.Join(dir, "node.json")
	out.Reset()
	if e = RunControl(context.Background(), []string{"config", "--genesis", "genesis.json", "--out", config}, &out, &errout); e != nil {
		t.Fatal(e)
	}
	var v struct {
		Token string `json:"rpc_token"`
	}
	data, e := os.ReadFile(config)
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(data, &v); e != nil {
		t.Fatal(e)
	}
	if len(v.Token) < 32 || bytes.Contains(out.Bytes(), []byte(v.Token)) {
		t.Fatal("missing or exposed RPC secret")
	}
	st, e := os.Stat(config)
	if e != nil {
		t.Fatal(e)
	}
	if st.Mode().Perm() != 0600 {
		t.Fatal("config permissions")
	}
	if e = RunControl(context.Background(), []string{"config", "--out", config}, &out, &errout); e == nil {
		t.Fatal("configuration overwritten")
	}
}
