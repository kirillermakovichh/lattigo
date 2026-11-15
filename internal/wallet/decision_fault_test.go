//go:build faultaudit

package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/faultprobe"
	"example.com/go-block-lattice/internal/payment"
)

func TestExtendedMetadataDependencyFailures(t *testing.T) {
	for _, point := range _auditPoints {
		if point.Function != "readPaymentFile" && !(point.Function == "canonicalWallet" && strings.Contains(point.ID, "filepath.Abs")) && !(point.Function == "ExportContacts" && strings.Contains(point.ID, "json.MarshalIndent")) && !(point.Function == "editProfile" && strings.Contains(point.ID, "json.MarshalIndent")) {
			continue
		}
		t.Run(point.ID, func(t *testing.T) {
			path, c := profileFixture(t)
			input := filepath.Join(t.TempDir(), "request")
			raw, err := payment.EncodeFile(payment.Request{Address: c.Address, Network: c.Network, Amount: 1})
			if err != nil {
				t.Fatal(err)
			}
			os.WriteFile(input, raw, 0600)
			_auditArm(point.ID, 1)
			defer _auditArm("", 0)
			switch point.Function {
			case "readPaymentFile":
				_, err = readPaymentFile(input)
			case "canonicalWallet":
				_, err = AccountNames(path)
			case "ExportContacts":
				err = ExportContacts(path, filepath.Join(t.TempDir(), "export"))
			case "editProfile":
				err = NameAccount(path, "main", 0, false)
			}
			if !_auditReached() || !errors.Is(err, _auditInjected) {
				t.Fatal("dependency error lost", err)
			}
		})
	}
}

func TestExtendedAccountDerivationFailures(t *testing.T) {
	path, g, _ := ownedOutboxFixture(t)
	for _, command := range []string{"request", "outbox", "receive-all"} {
		t.Run(command, func(t *testing.T) {
			_auditArm("command.go/keyAccount/w.PrivateKey/1", 1)
			defer _auditArm("", 0)
			args := []string{command, "--wallet", path}
			switch command {
			case "request":
				args = append(args, "--amount=1", "--network", string(g.ID()))
			case "outbox":
				args = append(args, "--action=retry")
			}
			err := RunCommand(context.Background(), args, io.Discard, io.Discard)
			if !_auditReached() || !errors.Is(err, _auditInjected) {
				t.Fatal("derivation failure lost", err)
			}
		})
	}
}

func TestPendingCanonicalizationFailures(t *testing.T) {
	path, g, _ := ownedOutboxFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/genesis" {
			json.NewEncoder(w).Encode(g)
		} else {
			json.NewEncoder(w).Encode(client.Status{Network: g.ID()})
		}
	}))
	defer server.Close()
	for _, nth := range []int{1, 2, 3} {
		t.Run(string(rune('0'+nth)), func(t *testing.T) {
			_auditArm("profile.go/canonicalWallet/filepath.EvalSymlinks/1", nth)
			defer _auditArm("", 0)
			err := retryOutbox(context.Background(), commandFlags{path: path, rpc: server.URL}, false, io.Discard)
			if !_auditReached() || !errors.Is(err, _auditInjected) {
				t.Fatal("canonicalization failure lost", err)
			}
		})
	}
}

func TestClearOutboxSyncFailure(t *testing.T) {
	path, g, b := ownedOutboxFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/genesis":
			json.NewEncoder(w).Encode(g)
		case "/v1/status":
			json.NewEncoder(w).Encode(client.Status{Network: g.ID()})
		case "/v1/block":
			json.NewEncoder(w).Encode(b)
		}
	}))
	defer server.Close()
	faultprobe.Arm("journal.go/SyncDirectory/os.Open/1", 1)
	defer faultprobe.Arm("", 0)
	err := clearOutbox(context.Background(), commandFlags{path: path, rpc: server.URL}, io.Discard)
	if !faultprobe.Reached() || !errors.Is(err, _auditInjected) {
		t.Fatal("directory sync failure lost", err)
	}
}

func TestPasswordEncryptionFailurePreservesOldCustody(t *testing.T) {
	path, _, _ := ownedOutboxFixture(t)
	_auditArm("wallet.go/encrypt/rand.Read/1", 1)
	defer _auditArm("", 0)
	err := ChangePassword(path, []byte("private outbox passphrase"), []byte("private replacement passphrase"))
	if !_auditReached() || !errors.Is(err, _auditInjected) {
		t.Fatal("encryption failure lost", err)
	}
}
