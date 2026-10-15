package control

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"example.com/go-block-lattice/internal/ledger"
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

func TestVerifyDoesNotCreateMissingLedger(t *testing.T) {
	dir := t.TempDir()
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	account := protocol.Address(key.Public().(ed25519.PublicKey))
	manifest := filepath.Join(dir, "genesis.json")
	var out, errout bytes.Buffer
	if err := RunControl(context.Background(), []string{"genesis", "--name", "verify-test", "--account", string(account), "--balance", "100", "--work-bits", "0", "--out", manifest}, &out, &errout); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"inspect", "verify"} {
		data := filepath.Join(dir, command)
		if err := RunControl(context.Background(), []string{command, "--genesis", manifest, "--data-dir", data}, &out, &errout); err == nil {
			t.Errorf("%s reported a missing ledger as valid", command)
		}
		if _, err := os.Stat(data); !os.IsNotExist(err) {
			t.Errorf("%s created a data directory", command)
		}
	}
}

func TestVerifyPreservesIncompleteJournal(t *testing.T) {
	dir := t.TempDir()
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	account := protocol.Address(key.Public().(ed25519.PublicKey))
	manifest := filepath.Join(dir, "genesis.json")
	var out, errout bytes.Buffer
	if err := RunControl(context.Background(), []string{"genesis", "--name", "verify-tail", "--account", string(account), "--balance", "100", "--out", manifest}, &out, &errout); err != nil {
		t.Fatal(err)
	}
	g, err := protocol.LoadGenesis(manifest)
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(dir, "data")
	st, err := ledger.Open(data, g)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	wal := filepath.Join(data, "ledger.wal")
	original, err := os.ReadFile(wal)
	if err != nil {
		t.Fatal(err)
	}
	damaged := append(original, 1, 2, 3)
	if err = os.WriteFile(wal, damaged, 0600); err != nil {
		t.Fatal(err)
	}
	if err = RunControl(context.Background(), []string{"verify", "--genesis", manifest, "--data-dir", data}, &out, &errout); err == nil {
		t.Error("verify accepted incomplete journal")
	}
	after, err := os.ReadFile(wal)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(damaged, after) {
		t.Fatal("verify changed the journal")
	}
}

func TestGenesisAllocationInputRejectsBytesBeyondLimit(t *testing.T) {
	dir := t.TempDir()
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	account := protocol.Address(key.Public().(ed25519.PublicKey))
	rows := []protocol.Allocation{{Account: account, Balance: 100, Representative: account}}
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, bytes.Repeat([]byte{' '}, (1<<20)-len(data))...)
	data = append(data, []byte("{}")...)
	source := filepath.Join(dir, "allocations.json")
	if err = os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	if err = RunControl(context.Background(), []string{"genesis", "--name", "bounded-input", "--allocations", source, "--out", filepath.Join(dir, "genesis.json")}, &out, &errout); err == nil {
		t.Fatal("genesis accepted hidden JSON beyond allocation size limit")
	}
}
