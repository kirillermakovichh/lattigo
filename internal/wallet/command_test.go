package wallet

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
