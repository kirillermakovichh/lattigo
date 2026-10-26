package wallet

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/protocol"
)

func TestWalletCommandFailurePaths(t *testing.T) {
	t.Setenv("BL_WALLET_PASSWORD", "private command passphrase")
	dir := t.TempDir()
	wallet := filepath.Join(dir, "wallet.json")
	w, err := Import(wallet, bytes.Repeat([]byte{3}, 32), []byte(os.Getenv("BL_WALLET_PASSWORD")))
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	badHex := filepath.Join(dir, "bad-hex")
	shortSeed := filepath.Join(dir, "short-seed")
	validSeed := filepath.Join(dir, "seed")
	for path, data := range map[string]string{badHex: "xx", shortSeed: "00", validSeed: hex.EncodeToString(bytes.Repeat([]byte{3}, 32))} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cases := [][]string{
		{"create", "--seed-file", filepath.Join(dir, "missing")},
		{"create", "--seed-file", badHex},
		{"create", "--seed-file", shortSeed},
		{"create", "--seed-file", validSeed, "--wallet", wallet},
		{"create", "--wallet", wallet},
		{"restore", "--backup", filepath.Join(dir, "missing")},
		{"backup", "--wallet", wallet, "--out", wallet},
		{"export-key", "--wallet", wallet, "--out", wallet},
		{"sign", "--wallet", wallet, "--balance", "1", "--kind", "send", "--network", "invalid"},
		{"status", "--rpc", "invalid"},
		{"submit", "--file", filepath.Join(dir, "missing")},
		{"send", "--wallet", filepath.Join(dir, "missing")},
		{"account", "--wallet", filepath.Join(dir, "missing")},
	}
	for _, args := range cases {
		t.Run(args[0]+"/"+args[len(args)-1], func(t *testing.T) {
			if err := RunCommand(context.Background(), args, io.Discard, io.Discard); err == nil {
				t.Fatalf("invalid operation accepted: %v", args)
			}
		})
	}
	closed := &Wallet{closed: true}
	if _, err := keyAccount(closed, 0); err == nil {
		t.Fatal("closed wallet derived account")
	}
	if err := signWork(context.Background(), closed, 0, &protocol.Block{}, 0); err == nil {
		t.Fatal("closed wallet signed")
	}
	t.Setenv("BL_WALLET_PASSWORD", "")
	if err := RunCommand(context.Background(), []string{"backup", "--wallet", wallet, "--out", filepath.Join(dir, "copy")}, io.Discard, io.Discard); err == nil {
		t.Fatal("missing password accepted")
	}
}

func TestSignedOutputRefusesExistingPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing")
	if err := os.WriteFile(path, []byte("retain"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := writeBlock(&out, path, protocol.Block{}); err == nil {
		t.Fatal("existing signed output replaced")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "retain" || out.Len() != 0 {
		t.Fatal("failed output publication modified an existing path or acknowledged success")
	}
}

func TestAccountMetadataRequiresExplicitGenesisAllocation(t *testing.T) {
	if err := verifyAccountState(context.Background(), nil, protocol.Genesis{}, client.AccountState{}); err == nil {
		t.Fatal("unallocated account metadata accepted")
	}
}
