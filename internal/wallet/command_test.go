package wallet

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func TestOfflineSigningWorkflow(t *testing.T) {
	t.Setenv("BL_WALLET_PASSWORD", "correct horse battery staple")
	path := filepath.Join(t.TempDir(), "wallet.json")
	var out, errout bytes.Buffer
	if err := RunCommand(context.Background(), []string{"create", "--wallet", path}, &out, &errout); err != nil {
		t.Fatal(err)
	}
	var created struct {
		Account protocol.Account `json:"account"`
	}
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	network := protocol.Digest([]byte("test network"))
	if err := RunCommand(context.Background(), []string{"sign", "--wallet", path, "--kind", "change", "--balance", "42", "--network", string(network), "--representative", string(created.Account), "--work-bits", "0"}, &out, &errout); err != nil {
		t.Fatal(err)
	}
	var b protocol.Block
	if err := json.Unmarshal(out.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if b.Account != created.Account || b.Network != network || b.Balance != 42 {
		t.Fatal("signed different state")
	}
	if err := b.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestSignedBlockFileRejectsBytesBeyondLimit(t *testing.T) {
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{3}, 32))
	account := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Block{Version: 1, Network: protocol.Digest([]byte("bounded file")), Kind: "change", Account: account, Representative: account, Balance: 1}
	if err := b.Sign(key); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, bytes.Repeat([]byte{' '}, (64<<10)-len(data))...)
	data = append(data, []byte("{}")...)
	path := filepath.Join(t.TempDir(), "block.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadBlock(path); err == nil {
		t.Fatal("signed-block loader accepted hidden trailing data beyond64KiB")
	}
}

func TestPrivateFileWalletCommandWorkflow(t *testing.T) {
	dir := t.TempDir()
	password := filepath.Join(dir, "password")
	seed := filepath.Join(dir, "seed")
	if err := os.WriteFile(password, []byte("a private file passphrase\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(seed, []byte(strings.Repeat("01", 32)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	walletPath := filepath.Join(dir, "wallet.json")
	var out, errout bytes.Buffer
	run := func(command string, args ...string) {
		t.Helper()
		out.Reset()
		commandArgs := append([]string{command, "--wallet", walletPath, "--password-file", password}, args...)
		if err := RunCommand(context.Background(), commandArgs, &out, &errout); err != nil {
			t.Fatalf("%s:%v", command, err)
		}
	}
	run("create", "--seed-file", seed)
	var created struct {
		Account protocol.Account `json:"account"`
	}
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	run("accounts", "--count", "2")
	var accounts []struct {
		Index   uint64           `json:"index"`
		Account protocol.Account `json:"account"`
	}
	if err := json.Unmarshal(out.Bytes(), &accounts); err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 || accounts[0].Account != created.Account || accounts[0].Account == accounts[1].Account {
		t.Fatal("account range did not derive distinct stable identities")
	}
	backup := filepath.Join(dir, "backup.json")
	run("backup", "--out", backup)
	restored := filepath.Join(dir, "restored.json")
	walletPath = restored
	run("restore", "--backup", backup)
	keyFile := filepath.Join(dir, "representative.seed")
	run("export-key", "--out", keyFile)
	exported, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := hex.DecodeString(strings.TrimSpace(string(exported)))
	if err != nil || len(decoded) != 32 {
		t.Fatal("invalid representative export")
	}
	if protocol.Address(ed25519.NewKeyFromSeed(decoded).Public().(ed25519.PublicKey)) != created.Account {
		t.Fatal("restored/exported identity changed")
	}
	signed := filepath.Join(dir, "signed.json")
	run("sign", "--kind", "change", "--balance", "1", "--network", string(protocol.Digest([]byte("file-workflow"))), "--representative", string(created.Account), "--work-bits", "0", "--out", signed)
	if _, err = loadBlock(signed); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(password, 0644); err != nil {
		t.Fatal(err)
	}
	if err = RunCommand(context.Background(), []string{"accounts", "--wallet", walletPath, "--password-file", password}, &out, &errout); err == nil {
		t.Fatal("world-readable passphrase was accepted")
	}
}
