package wallet

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func outboxFixture(t *testing.T) (string, protocol.Block) {
	t.Helper()
	path, c := profileFixture(t)
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	b := protocol.Block{Version: 1, Network: c.Network, Kind: "send", Account: c.Address, Representative: c.Address, Balance: 1, Link: string(c.Address)}
	if err := b.Sign(key); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	file, err := outboxFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePrivate(file, data); err != nil {
		t.Fatal(err)
	}
	return path, b
}
func TestOutboxInspectionDoesNotClaimAdmission(t *testing.T) {
	path, b := outboxFixture(t)
	t.Setenv("BL_WALLET_PASSWORD", "")
	var out bytes.Buffer
	if err := RunCommand(context.Background(), []string{"outbox", "--wallet", path}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	var entry OutboxEntry
	if err := json.Unmarshal(out.Bytes(), &entry); err != nil || entry.Hash != b.ID() || entry.State != "stored" {
		t.Fatal("outbox inspection changed intent or claimed confirmation")
	}
}
