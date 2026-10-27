//go:build faultaudit

package wallet

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/faultprobe"
	"example.com/go-block-lattice/internal/protocol"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateFileIOFailures(t *testing.T) {
	for _, point := range _auditPoints {
		if point.Function != "writeExclusive" && point.Function != "readPrivateLimit" && point.Function != "lockPrivate" {
			continue
		}
		t.Run(point.ID, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "private")
			if err := os.WriteFile(path, []byte("private"), 0600); err != nil {
				t.Fatal(err)
			}
			_auditArm(point.ID, 1)
			defer _auditArm("", 0)
			var err error
			switch point.Function {
			case "writeExclusive":
				err = WritePrivate(filepath.Join(dir, "new"), []byte("payload"))
			case "readPrivateLimit":
				_, err = ReadSecret(path)
			case "lockPrivate":
				_, err = lockAccount(path, 0)
			}
			if !_auditReached() {
				t.Fatal("fault site not reached")
			}
			if !errors.Is(err, _auditInjected) {
				t.Fatalf("I/O error not propagated: %v", err)
			}
			leftovers, _ := filepath.Glob(filepath.Join(dir, ".bl-private-*"))
			if len(leftovers) != 0 {
				t.Fatal("temporary custody files leaked")
			}
		})
	}
	w := &Wallet{}
	w.Close()
	if _, err := keyAccount(w, 0); err == nil {
		t.Fatal("closed identity derived")
	}
	if err := w.ExportAccountSeed(filepath.Join(t.TempDir(), "seed"), 0); err == nil {
		t.Fatal("closed identity exported")
	}
}

func TestSignedInputDescriptorFailures(t *testing.T) {
	w := &Wallet{}
	defer w.Close()
	key, err := w.PrivateKey(0)
	if err != nil {
		t.Fatal(err)
	}
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Block{Version: 1, Network: protocol.Digest([]byte("input-fault")), Kind: "send", Account: a, Representative: a, Balance: 1, Link: string(a)}
	if err = b.Sign(key); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range _auditPoints {
		if point.Function != "loadBlock" && point.Function != "signWork" {
			continue
		}
		t.Run(point.ID, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "block")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			_auditArm(point.ID, 1)
			defer _auditArm("", 0)
			var err error
			if point.Function == "loadBlock" {
				_, err = loadBlock(path)
			} else {
				copy := b
				err = signWork(context.Background(), w, 0, &copy, 0)
			}
			if !_auditReached() || !errors.Is(err, _auditInjected) {
				t.Fatalf("signed input failure: %v", err)
			}
		})
	}
}

func TestPaymentOutboxAndIdentityIOFailures(t *testing.T) {
	pass := []byte("private fault test passphrase")
	t.Setenv("BL_WALLET_PASSWORD", string(pass))
	t.Setenv("BL_RPC_TOKEN", "")
	for _, site := range []string{"command.go/RunCommand/filepath.EvalSymlinks/1", "command.go/RunCommand/os.Remove/1", "wallet.go/writeExclusive/os.Link/1", "command.go/signWork/b.GenerateWork/1", "command.go/RunCommand/json.MarshalIndent/1"} {
		t.Run(site, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "wallet")
			w, err := Import(path, make([]byte, 32), pass)
			if err != nil {
				t.Fatal(err)
			}
			key, err := w.PrivateKey(0)
			if err != nil {
				t.Fatal(err)
			}
			w.Close()
			a := protocol.Address(key.Public().(ed25519.PublicKey))
			g := protocol.Genesis{Version: 1, Name: "outbox-fault", Supply: 100, Allocations: []protocol.Allocation{{Account: a, Balance: 100, Representative: a}}}
			b := protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(a)}
			if err = b.Sign(key); err != nil {
				t.Fatal(err)
			}
			pending := path + ".pending-0.json"
			if strings.Contains(site, "os.Remove") {
				raw, err := json.Marshal(b)
				if err != nil {
					t.Fatal(err)
				}
				if err = WritePrivate(pending, raw); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
				var value any
				switch r.URL.Path {
				case "/v1/block":
					value = b
				case "/v1/genesis":
					value = g
				case "/v1/status":
					value = client.Status{Network: g.ID()}
				case "/v1/account":
					value = client.AccountState{Account: a, Representative: a, Balance: 100}
				default:
					t.Error("failed signing path reached submission")
					http.Error(out, "unexpected", 500)
					return
				}
				if err := json.NewEncoder(out).Encode(value); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			_auditArm(site, 1)
			defer _auditArm("", 0)
			err = RunCommand(context.Background(), []string{"send", "--wallet", path, "--rpc", server.URL, "--to", string(a), "--amount", "1", "--timeout", "30s"}, io.Discard, io.Discard)
			if !_auditReached() || !errors.Is(err, _auditInjected) {
				t.Fatalf("payment I/O failure: %v", err)
			}
			if strings.Contains(site, "os.Remove") {
				if _, err := os.Stat(pending); err != nil {
					t.Fatal("failed removal lost pending intent")
				}
			}
		})
	}
}

func TestPrivateInputGrowthAfterStatRemainsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("small"), 0600); err != nil {
		t.Fatal(err)
	}
	faultprobe.Before("wallet.go/readPrivateLimit/io.ReadAll/1", 1, func() {
		if err := os.WriteFile(path, make([]byte, maxFileSize+1), 0600); err != nil {
			t.Fatal(err)
		}
	})
	defer faultprobe.Arm("", 0)
	if _, err := ReadSecret(path); err == nil || !faultprobe.Reached() {
		t.Fatal("growing secret input exceeded read budget")
	}
}

func TestWalletDependencyFailuresNeverPublishCustody(t *testing.T) {
	pass := []byte("private dependency passphrase")
	encrypted, err := encrypt(make([]byte, 32), pass)
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range _auditPoints {
		if point.Function != "encrypt" && point.Function != "decrypt" && point.Function != "Create" {
			continue
		}
		t.Run(point.ID, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "wallet")
			if point.Function == "decrypt" {
				if err := os.WriteFile(path, encrypted, 0600); err != nil {
					t.Fatal(err)
				}
			}
			_auditArm(point.ID, 1)
			defer _auditArm("", 0)
			var w *Wallet
			var err error
			switch point.Function {
			case "encrypt":
				w, err = Import(path, make([]byte, 32), pass)
			case "decrypt":
				w, err = Open(path, pass)
			case "Create":
				w, err = Create(path, pass)
			}
			if w != nil {
				w.Close()
				t.Fatal("failed dependency returned usable custody")
			}
			if !_auditReached() || !errors.Is(err, _auditInjected) {
				t.Fatalf("wallet dependency failure lost: %v", err)
			}
			if point.Function != "decrypt" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("dependency failure published wallet")
				}
			}
		})
	}
}

func TestSignedOutputEncodingFailure(t *testing.T) {
	_auditArm("command.go/writeBlock/json.MarshalIndent/1", 1)
	defer _auditArm("", 0)
	path := filepath.Join(t.TempDir(), "block")
	err := writeBlock(io.Discard, path, protocol.Block{})
	if !_auditReached() || !errors.Is(err, _auditInjected) {
		t.Fatalf("encoding failure lost: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("encoding failure published signed output")
	}
}

func TestCommandsPropagateAccountDerivationFailure(t *testing.T) {
	pass := []byte("private derivation passphrase")
	t.Setenv("BL_WALLET_PASSWORD", string(pass))
	for _, command := range []string{"create", "accounts", "sign", "account", "send"} {
		t.Run(command, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "wallet")
			if command != "create" {
				w, err := Import(path, make([]byte, 32), pass)
				if err != nil {
					t.Fatal(err)
				}
				w.Close()
			}
			args := []string{command, "--wallet", path}
			if command == "sign" {
				args = append(args, "--balance", "1", "--work-bits", "0")
			}
			_auditArm("command.go/keyAccount/w.PrivateKey/1", 1)
			defer _auditArm("", 0)
			err := RunCommand(context.Background(), args, io.Discard, io.Discard)
			if !_auditReached() || !errors.Is(err, _auditInjected) {
				t.Fatalf("derivation error lost: %v", err)
			}
		})
	}
}
