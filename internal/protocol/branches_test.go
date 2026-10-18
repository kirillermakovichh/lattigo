package protocol

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func protocolFixture() (Block, Genesis, ed25519.PrivateKey) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := Address(key.Public().(ed25519.PublicKey))
	g := Genesis{Version: 1, Name: "branches", Supply: 100, Allocations: []Allocation{{Account: a, Balance: 100, Representative: a}}}
	return Block{Version: 1, Network: g.ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(a)}, g, key
}
func TestCanonicalBlockRejectionBranches(t *testing.T) {
	original, _, key := protocolFixture()
	for name, mutate := range map[string]func(*Block){
		"version": func(b *Block) { b.Version = 0 }, "network": func(b *Block) { b.Network = "bad" }, "account": func(b *Block) { b.Account = "bad" }, "representative": func(b *Block) { b.Representative = "bad" }, "previous": func(b *Block) { b.Previous = "bad" }, "kind": func(b *Block) { b.Kind = "mint" }, "destination": func(b *Block) { b.Link = "bad" }, "source": func(b *Block) { b.Kind = "receive"; b.Link = "bad" }, "change link": func(b *Block) { b.Kind = "change" },
	} {
		t.Run(name, func(t *testing.T) {
			b := original
			mutate(&b)
			if _, err := b.Canonical(); err == nil {
				t.Fatal("invalid canonical block accepted")
			}
			if err := b.Sign(key); err == nil {
				t.Fatal("invalid block signed")
			}
			if err := b.Verify(); err == nil {
				t.Fatal("invalid block verified")
			}
			if b.ID() != "" || b.CheckWork(0) {
				t.Fatal("invalid block has usable identity/work")
			}
			if err := b.GenerateWork(context.Background(), 0); err == nil {
				t.Fatal("invalid block mined")
			}
		})
	}
	for _, invalid := range []ed25519.PrivateKey{nil, make([]byte, 63), ed25519.NewKeyFromSeed([]byte(strings.Repeat("x", 32)))} {
		b := original
		if err := b.Sign(invalid); err == nil {
			t.Fatal("invalid or foreign key accepted")
		}
	}
	b := original
	if err := b.Sign(key); err != nil {
		t.Fatal(err)
	}
	for _, sig := range []string{"xyz", "00", strings.ToUpper(b.Signature), strings.Repeat("00", 64)} {
		invalid := b
		invalid.Signature = sig
		if invalid.Verify() == nil {
			t.Fatal("invalid signature accepted")
		}
	}
	if b.CheckWork(25) {
		t.Fatal("unsupported work strength accepted")
	}
	if err := b.GenerateWork(context.Background(), 25); err == nil {
		t.Fatal("unsupported work strength generated")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.GenerateWork(canceled, 0); !errors.Is(err, context.Canceled) {
		t.Fatal("work ignored cancellation")
	}
	b.Kind = "receive"
	b.Link = string(Digest([]byte("source")))
	if err := b.Sign(key); err != nil {
		t.Fatal(err)
	}
	if b.Verify() != nil {
		t.Fatal("valid receive rejected")
	}
	b.Kind = "change"
	b.Link = ""
	if err := b.Sign(key); err != nil {
		t.Fatal(err)
	}
	if err := b.GenerateWork(context.Background(), 9); err != nil {
		t.Fatal(err)
	}
	if !b.CheckWork(9) {
		t.Fatal("non-byte-aligned work invalid")
	}
}
func TestAddressHashAndAmountRejectionBranches(t *testing.T) {
	_, g, _ := protocolFixture()
	a := g.Allocations[0].Account
	if Address(nil) != "" {
		t.Fatal("invalid public key accepted")
	}
	for _, value := range []Account{"bad", Account(strings.ToUpper(string(a))), Account("bl_" + strings.Repeat("z", 72)), Account(string(a[:74]) + "z")} {
		if _, err := value.PublicKey(); err == nil {
			t.Fatal("invalid address accepted")
		}
	}
	for _, hash := range []Hash{"", Hash(strings.Repeat("z", 64)), Hash(strings.ToUpper(string(g.ID())))} {
		if _, err := hash.Bytes(); err == nil {
			t.Fatal("invalid hash accepted")
		}
	}
	for _, value := range []string{"", "01", "-1", "+1", "1x", "18446744073709551616"} {
		if _, err := ParseAmount(value); err == nil {
			t.Fatal("invalid amount accepted")
		}
	}
	for _, raw := range []string{`1`, `null`, `"01"`} {
		var value Amount
		if json.Unmarshal([]byte(raw), &value) == nil {
			t.Fatal("invalid JSON amount accepted")
		}
	}
}
func TestGenesisValidationAndInputBranches(t *testing.T) {
	_, original, _ := protocolFixture()
	for name, mutate := range map[string]func(*Genesis){
		"version": func(g *Genesis) { g.Version = 2 }, "name empty": func(g *Genesis) { g.Name = "" }, "name whitespace": func(g *Genesis) { g.Name = " network " }, "name length": func(g *Genesis) { g.Name = strings.Repeat("x", 129) }, "work": func(g *Genesis) { g.WorkBits = 25 }, "supply": func(g *Genesis) { g.Supply = 0 }, "empty": func(g *Genesis) { g.Allocations = nil }, "too many": func(g *Genesis) { g.Allocations = make([]Allocation, 4097) }, "account": func(g *Genesis) { g.Allocations[0].Account = "bad" }, "representative": func(g *Genesis) { g.Allocations[0].Representative = "bad" }, "zero allocation": func(g *Genesis) { g.Allocations[0].Balance = 0 }, "mismatch": func(g *Genesis) { g.Supply = 99 }, "duplicate": func(g *Genesis) { g.Allocations = append(g.Allocations, g.Allocations[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			g := original
			g.Allocations = append([]Allocation{}, original.Allocations...)
			mutate(&g)
			if g.Validate() == nil || g.ID() != "" {
				t.Fatal("invalid genesis accepted")
			}
		})
	}
	second := Address(ed25519.NewKeyFromSeed([]byte(strings.Repeat("y", 32))).Public().(ed25519.PublicKey))
	overflow := original
	overflow.Allocations = []Allocation{{Account: original.Allocations[0].Account, Balance: Amount(^uint64(0)), Representative: second}, {Account: second, Balance: 1, Representative: second}}
	if overflow.Validate() == nil {
		t.Fatal("overflowing allocation sum accepted")
	}
	valid, _ := json.Marshal(original)
	for name, data := range map[string][]byte{"malformed": []byte(`{`), "unknown": []byte(`{"unknown":1}`), "trailing": append(append([]byte{}, valid...), []byte(`{}`)...), "large": []byte(strings.Repeat(" ", 1<<20+1)), "valid": valid} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "genesis.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadGenesis(path)
			if (err == nil) != (name == "valid") {
				t.Fatalf("load result:%v", err)
			}
		})
	}
	if _, err := LoadGenesis(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("missing file accepted")
	}
	if _, err := LoadGenesis(t.TempDir()); err == nil {
		t.Fatal("directory accepted")
	}
}
