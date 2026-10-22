package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type unavailableWriter struct{}

func (unavailableWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestWalletEnvelopeAndFilesystemBranches(t *testing.T) {
	dir := t.TempDir()
	pass := []byte("private test passphrase")
	seed := bytes.Repeat([]byte{4}, 32)
	data, err := encrypt(seed, pass)
	if err != nil {
		t.Fatal(err)
	}
	var original envelope
	if err = json.Unmarshal(data, &original); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*envelope){"version": func(e *envelope) { e.Version++ }, "kdf": func(e *envelope) { e.KDF = "unknown" }, "iterations": func(e *envelope) { e.Iterations++ }, "salt length": func(e *envelope) { e.Salt = "00" }, "salt encoding": func(e *envelope) { e.Salt = "xyz" }, "nonce length": func(e *envelope) { e.Nonce = "00" }, "nonce encoding": func(e *envelope) { e.Nonce = "xyz" }, "ciphertext length": func(e *envelope) { e.Ciphertext = "00" }, "ciphertext encoding": func(e *envelope) { e.Ciphertext = "xyz" }} {
		t.Run(name, func(t *testing.T) {
			bad := original
			mutate(&bad)
			raw, err := json.Marshal(bad)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = decrypt(raw, pass); err == nil {
				t.Fatal("invalid wallet format accepted")
			}
		})
	}
	for _, raw := range [][]byte{[]byte("{"), append(append([]byte{}, data...), []byte("{}")...)} {
		if _, err := decrypt(raw, pass); err == nil {
			t.Fatal("malformed envelope accepted")
		}
	}
	if _, err = Import(filepath.Join(dir, "invalid"), nil, pass); err == nil {
		t.Fatal("invalid seed accepted")
	}
	if _, err = Import(filepath.Join(dir, "invalid"), seed, []byte("short")); err == nil {
		t.Fatal("short password accepted")
	}
	var closed *Wallet
	closed.Close()
	if _, err = closed.PrivateKey(0); err == nil {
		t.Fatal("nil wallet derived key")
	}
	if closed.ExportAccountSeed(filepath.Join(dir, "export"), 0) == nil {
		t.Fatal("nil wallet exported key")
	}
	missing := filepath.Join(dir, "missing")
	if _, err = Open(missing, pass); err == nil {
		t.Fatal("missing wallet accepted")
	}
	if Backup(missing, filepath.Join(dir, "backup"), pass) == nil {
		t.Fatal("missing wallet backed up")
	}
	bad := filepath.Join(dir, "bad.json")
	if err = os.WriteFile(bad, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(bad, pass); err == nil {
		t.Fatal("invalid wallet opened")
	}
	if Backup(bad, filepath.Join(dir, "backup"), pass) == nil {
		t.Fatal("invalid wallet backed up")
	}
	if _, err = ReadSecret(dir); err == nil {
		t.Fatal("directory secret accepted")
	}
	large := filepath.Join(dir, "large")
	if err = os.WriteFile(large, make([]byte, maxFileSize+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadSecret(large); err == nil {
		t.Fatal("oversized secret accepted")
	}
	if WritePrivate(filepath.Join(bad, "child"), []byte("x")) == nil {
		t.Fatal("file used as parent")
	}
	lock := filepath.Join(dir, "wallet.account-0.lock")
	if err = os.WriteFile(lock, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = lockAccount(filepath.Join(dir, "wallet"), 0); err == nil {
		t.Fatal("exposed lock accepted")
	}
}
func TestWalletCommandArgumentAndOutputBranches(t *testing.T) {
	t.Setenv("BL_WALLET_PASSWORD", "")
	t.Setenv("BL_RPC_TOKEN", "")
	t.Setenv("BL_RPC", "")
	if envDefault("BL_RPC", "fallback") != "fallback" {
		t.Fatal("environment fallback lost")
	}
	t.Setenv("BL_RPC", "http://configured.invalid")
	if envDefault("BL_RPC", "") != "http://configured.invalid" {
		t.Fatal("configured environment ignored")
	}
	for _, args := range [][]string{{"unknown"}, {"create", "--unknown"}, {"create", "positional"}, {"create", "--timeout", "0s"}, {"create"}, {"create", "--password-file", "missing"}, {"restore", "--backup", "missing"}, {"sign"}, {"accounts", "--count", "0"}} {
		if RunCommand(context.Background(), args, io.Discard, io.Discard) == nil {
			t.Errorf("invalid command accepted:%v", args)
		}
	}
	if RunCommand(context.Background(), []string{"help"}, unavailableWriter{}, io.Discard) == nil {
		t.Fatal("help output failure ignored")
	}
	if RunCommand(context.Background(), []string{"create", "--help"}, io.Discard, io.Discard) != nil {
		t.Fatal("command help failed")
	}
	dir := t.TempDir()
	walletPath := filepath.Join(dir, "wallet.json")
	t.Setenv("BL_WALLET_PASSWORD", "private test passphrase")
	w, err := Create(walletPath, []byte(os.Getenv("BL_WALLET_PASSWORD")))
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	for _, args := range [][]string{{"restore"}, {"backup"}, {"export-key"}, {"accounts", "--count", "1001"}, {"accounts", "--index", "18446744073709551615", "--count", "2"}, {"sign", "--balance", "bad"}, {"sign", "--balance", "1", "--work-bits", "25"}, {"submit"}, {"account", "--address", "bad"}, {"history", "--limit", "0"}, {"receivables", "--offset", "-1"}} {
		args = append(args, "--wallet", walletPath)
		if RunCommand(context.Background(), args, io.Discard, io.Discard) == nil {
			t.Errorf("invalid wallet flags accepted:%v", args)
		}
	}
	if RunCommand(context.Background(), []string{"accounts", "--wallet", walletPath}, unavailableWriter{}, io.Discard) == nil {
		t.Fatal("account output failure ignored")
	}
	if _, err = (&Wallet{closed: true}).PrivateKey(math.MaxUint64); err == nil {
		t.Fatal("closed wallet derived key")
	}
	if _, err = loadBlock(dir); err == nil {
		t.Fatal("directory block input accepted")
	}
	for _, raw := range []string{"{", "{}{}", "{}"} {
		p := filepath.Join(dir, "block.json")
		if err = os.WriteFile(p, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = loadBlock(p); err == nil {
			t.Fatal("invalid block input accepted")
		}
	}
}

func TestTokenFileClientOutcomes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	flags := commandFlags{rpc: "http://local.invalid", tokenFile: path}
	if _, err := flags.client(); err == nil {
		t.Fatal("missing token accepted")
	}
	if err := os.WriteFile(path, []byte("private-test-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := flags.client(); err != nil {
		t.Fatal(err)
	}
}
