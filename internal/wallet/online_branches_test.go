package wallet

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/protocol"
)

func TestOnlineWalletRejectionAndAcknowledgementBranches(t *testing.T) {
	password := []byte("online private passphrase")
	t.Setenv("BL_WALLET_PASSWORD", string(password))
	t.Setenv("BL_RPC", "")
	t.Setenv("BL_RPC_TOKEN", "")
	original := filepath.Join(t.TempDir(), "wallet.json")
	w, err := Import(original, bytes.Repeat([]byte{1}, 32), password)
	if err != nil {
		t.Fatal(err)
	}
	key, err := w.PrivateKey(0)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	defer clear(key)
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	senderKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{2}, 32))
	sender := protocol.Address(senderKey.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "online-wallet", Supply: 200, Allocations: []protocol.Allocation{{Account: a, Balance: 100, Representative: a}, {Account: sender, Balance: 100, Representative: sender}}}
	walletBytes, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, command string
		args          []string
		success       bool
	}{
		{"genesis unavailable", "send", []string{"--to", string(sender), "--amount", "1"}, false},
		{"payment status unavailable", "send", []string{"--to", string(sender), "--amount", "1"}, false},
		{"payment account unavailable", "send", []string{"--to", string(sender), "--amount", "1"}, false},
		{"frontier unavailable", "send", []string{"--to", string(sender), "--amount", "1"}, false},
		{"source unavailable", "receive", nil, false},
		{"receivables unavailable", "receive", nil, false},
		{"receivable search limit", "receive", nil, false},
		{"output published", "send", []string{"--to", string(sender), "--amount", "1"}, true},
		{"later receivable page", "receive", nil, true},
		{"pending confirmed", "send", []string{"--to", string(sender), "--amount", "1"}, true},
		{"account lock unavailable", "send", []string{"--to", string(sender), "--amount", "1"}, false},
		{"corrupt pending", "send", []string{"--to", string(sender), "--amount", "1"}, false},
		{"derived missing wallet", "account", []string{"--wallet", "/definitely/missing/wallet.json"}, false},
		{"history unavailable", "history", []string{"--address", string(a)}, false},
		{"account unavailable", "account", []string{"--address", string(a)}, false},
		{"receivables query unavailable", "receivables", []string{"--address", string(a)}, false},
		{"pinned network", "send", []string{"--to", string(sender), "--amount", "1", "--expected-network", string(protocol.Digest([]byte("other")))}, false},
		{"status network", "send", []string{"--to", string(sender), "--amount", "1"}, false},
		{"genesis metadata", "send", []string{"--to", string(sender), "--amount", "1"}, false},
		{"signed frontier metadata", "send", []string{"--to", string(sender), "--amount", "1"}, false},
		{"invalid amount", "send", []string{"--to", string(sender), "--amount", "bad"}, false},
		{"zero amount", "send", []string{"--to", string(sender), "--amount", "0"}, false},
		{"insufficient amount", "send", []string{"--to", string(sender), "--amount", "101"}, false},
		{"destination", "send", []string{"--to", "bad", "--amount", "1"}, false},
		{"representative", "delegate", []string{"--representative", "bad"}, false},
		{"missing source", "receive", nil, false},
		{"source kind", "receive", nil, false},
		{"source network", "receive", nil, false},
		{"source destination", "receive", nil, false},
		{"consumed source", "receive", nil, false},
		{"receive overflow", "receive", nil, false},
		{"existing representative", "receive", []string{"--representative", string(sender)}, false},
		{"pending unresolved", "send", []string{"--to", string(sender), "--amount", "1"}, false},
		{"output collision", "send", []string{"--to", string(sender), "--amount", "1"}, false},
		{"submit rejected", "send", []string{"--to", string(sender), "--amount", "1"}, false},
		{"wait unavailable", "send", []string{"--to", string(sender), "--amount", "1", "--wait"}, false},
		{"contact acknowledged", "send", []string{"--contact", "Sender", "--amount", "1"}, true},
		{"send acknowledged", "send", []string{"--to", string(sender), "--amount", "1"}, true},
		{"send confirmed", "send", []string{"--to", string(sender), "--amount", "1", "--wait"}, true},
		{"receive acknowledged", "receive", nil, true},
		{"opening representative", "receive", []string{"--representative", string(sender)}, true},
		{"status query", "status", nil, true},
		{"status unavailable", "status", nil, false},
		{"block query", "block", nil, true},
		{"block unavailable", "block", nil, false},
		{"default opening representative", "receive", nil, true},
		{"derived account query", "account", nil, true},
		{"history query", "history", []string{"--address", string(a)}, true},
		{"account query", "account", []string{"--address", string(a)}, true},
		{"receivables query", "receivables", []string{"--address", string(a)}, true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "wallet.json")
			if err := os.WriteFile(path, walletBytes, 0600); err != nil {
				t.Fatal(err)
			}
			source := protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: sender, Representative: sender, Balance: 80, Link: string(a)}
			switch item.name {
			case "source kind":
				source.Kind = "change"
				source.Link = ""
			case "source network":
				source.Network = protocol.Digest([]byte("other"))
			case "source destination":
				source.Link = string(sender)
			}
			if err := source.Sign(senderKey); err != nil {
				t.Fatal(err)
			}
			pending := protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(sender)}
			if err := pending.Sign(key); err != nil {
				t.Fatal(err)
			}
			if item.name == "pending unresolved" || item.name == "pending confirmed" {
				raw, err := json.Marshal(pending)
				if err != nil {
					t.Fatal(err)
				}
				if err = WritePrivate(path+".pending-0.json", raw); err != nil {
					t.Fatal(err)
				}
			}
			if item.name == "account lock unavailable" {
				if err := os.Mkdir(path+".account-0.lock", 0700); err != nil {
					t.Fatal(err)
				}
			}
			if item.name == "corrupt pending" {
				if err := WritePrivate(path+".pending-0.json", []byte("{")); err != nil {
					t.Fatal(err)
				}
			}
			if item.name == "contact acknowledged" {
				if err := PutContact(path, "Sender", Contact{Address: sender, Network: g.ID()}, false); err != nil {
					t.Fatal(err)
				}
			}
			var submitted protocol.Block
			server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
				var response any
				switch r.URL.Path {
				case "/v1/genesis":
					if item.name == "genesis unavailable" {
						http.Error(out, "unavailable", 503)
						return
					}
					response = g
				case "/v1/status":
					if item.name == "status unavailable" || item.name == "payment status unavailable" {
						http.Error(out, "unavailable", 503)
						return
					}
					status := client.Status{Network: g.ID(), Supply: g.Supply}
					if item.name == "status network" {
						status.Network = "other"
					}
					response = status
				case "/v1/account":
					if item.name == "payment account unavailable" || item.name == "account unavailable" {
						http.Error(out, "unavailable", 503)
						return
					}
					if item.name == "opening representative" || item.name == "default opening representative" {
						out.WriteHeader(404)
						response = map[string]any{"error": map[string]string{"code": "not_found"}}
						break
					}
					state := client.AccountState{Account: a, Balance: 100, Representative: a}
					if item.name == "genesis metadata" {
						state.Balance = 99
					}
					if item.name == "signed frontier metadata" || item.name == "frontier unavailable" {
						state.Frontier = source.ID()
					}
					response = state
				case "/v1/block":
					if item.name == "block unavailable" || item.name == "source unavailable" || item.name == "frontier unavailable" {
						http.Error(out, "unavailable", 503)
						return
					}
					if item.name == "pending unresolved" || item.name == "wait unavailable" {
						if item.name == "wait unavailable" {
							out.WriteHeader(503)
							response = map[string]any{"error": map[string]string{"code": "unavailable"}}
						} else {
							out.WriteHeader(404)
							response = map[string]any{"error": map[string]string{"code": "not_found"}}
						}
						break
					}
					if item.name == "pending confirmed" && r.URL.Query().Get("id") == string(pending.ID()) {
						response = pending
						break
					}
					if submitted.ID() != "" && r.URL.Query().Get("id") == string(submitted.ID()) {
						response = submitted
					} else {
						response = source
					}
				case "/v1/receivables":
					if item.name == "receivables unavailable" || item.name == "receivables query unavailable" {
						http.Error(out, "unavailable", 503)
						return
					}
					rows := []client.Receivable{{Source: source.ID(), From: sender, To: a, Amount: 20}}
					if item.name == "later receivable page" && r.URL.Query().Get("offset") == "0" {
						rows = make([]client.Receivable, 1000)
						for i := range rows {
							rows[i] = client.Receivable{Source: protocol.Digest([]byte("another source")), From: sender, To: a, Amount: 1}
						}
					}
					if item.name == "receivable search limit" {
						rows = make([]client.Receivable, 1000)
					}
					if item.name == "consumed source" {
						rows = nil
					}
					if item.name == "receive overflow" {
						rows[0].Amount = protocol.Amount(math.MaxUint64)
					}
					response = rows
				case "/v1/history":
					if item.name == "history unavailable" {
						http.Error(out, "unavailable", 503)
						return
					}
					response = []any{}
				case "/v1/blocks":
					if err := json.NewDecoder(r.Body).Decode(&submitted); err != nil {
						http.Error(out, "bad", 400)
						return
					}
					if item.name == "submit rejected" {
						out.WriteHeader(400)
						response = map[string]any{"error": map[string]string{"code": "invalid"}}
					} else {
						response = client.Submission{Hash: submitted.ID(), Status: "admitted"}
					}
				default:
					http.NotFound(out, r)
					return
				}
				if err := json.NewEncoder(out).Encode(response); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			args := []string{item.command, "--wallet", path, "--rpc", server.URL, "--timeout", "10s"}
			args = append(args, item.args...)
			if item.command == "block" {
				args = append(args, "--hash", string(source.ID()))
			}
			if item.command == "receive" && item.name != "missing source" {
				args = append(args, "--source", string(source.ID()))
			}
			if item.name == "receivable search limit" {
				args = append(args, "--timeout", "60s")
			}
			if item.name == "output published" {
				args = append(args, "--out", filepath.Join(dir, "signed.json"))
			}
			if item.name == "output collision" {
				out := filepath.Join(dir, "existing")
				if err := os.WriteFile(out, []byte("existing"), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--out", out)
			}
			var stdout bytes.Buffer
			err = RunCommand(context.Background(), args, &stdout, io.Discard)
			if (err == nil) != item.success {
				t.Fatalf("success=%v, error=%v", item.success, err)
			}
			if item.name == "output collision" {
				if _, err := os.Stat(path + ".pending-0.json"); err != nil {
					t.Fatal("output failure lost signed outbox")
				}
			}
			if item.name == "contact acknowledged" && submitted.Link != string(sender) {
				t.Fatal("contact payment signed another destination")
			}
			if item.success && !json.Valid(stdout.Bytes()) {
				t.Fatal("successful command did not return JSON")
			}
			if item.name == "wait unavailable" && !strings.Contains(err.Error(), "was admitted") {
				t.Fatal("confirmation failure lost admission outcome")
			}
		})
	}
}
