package protocol

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCanonicalSigningAndWork(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := Address(key.Public().(ed25519.PublicKey))
	if _, e := a.PublicKey(); e != nil {
		t.Fatal(e)
	}
	b := Block{Version: 1, Network: Digest([]byte("network")), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(a)}
	if e := b.Sign(key); e != nil {
		t.Fatal(e)
	}
	id := b.ID()
	if e := b.GenerateWork(context.Background(), 8); e != nil {
		t.Fatal(e)
	}
	if b.ID() != id || !b.CheckWork(8) || b.Verify() != nil {
		t.Fatal("work/signature identity")
	}
	b.Balance--
	if b.Verify() == nil {
		t.Fatal("tampering accepted")
	}
	if _, e := Account(strings.ToUpper(string(a))).PublicKey(); e == nil {
		t.Fatal("noncanonical address")
	}
}
func TestAmountAndGenesis(t *testing.T) {
	for _, s := range []string{"-1", "01", "18446744073709551616", "1.0", ""} {
		if _, e := ParseAmount(s); e == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	var a Amount
	if json.Unmarshal([]byte(`1`), &a) == nil {
		t.Fatal("numeric JSON accepted")
	}
	if e := json.Unmarshal([]byte(`"18446744073709551615"`), &a); e != nil {
		t.Fatal(e)
	}
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	address := Address(key.Public().(ed25519.PublicKey))
	g := Genesis{Version: 1, Name: "unit", Supply: 100, Allocations: []Allocation{{address, 100, address}}}
	if g.Validate() != nil || g.ID() == "" {
		t.Fatal("genesis")
	}
	g.Allocations = append(g.Allocations, g.Allocations[0])
	if g.Validate() == nil {
		t.Fatal("duplicate allocation")
	}
}

func TestIndependentCanonicalVector(t *testing.T) {
	data, err := os.ReadFile("../../testdata/protocol/send.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		Block        Block  `json:"block"`
		CanonicalHex string `json:"canonical_hex"`
		ID           Hash   `json:"id"`
	}
	if err = json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	encoded, err := vector.Block.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(encoded) != vector.CanonicalHex || vector.Block.ID() != vector.ID {
		t.Fatal("independent canonical vector mismatch")
	}
}

func TestExtensionWorkInputLength(t *testing.T) {
	if workOK(nil, 0, 0) || workOK(make([]byte, 31), 0, 0) {
		t.Fatal("invalid work digest length")
	}
}
