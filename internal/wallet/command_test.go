package wallet

import (
	"bytes"
	"context"
	"encoding/json"
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
