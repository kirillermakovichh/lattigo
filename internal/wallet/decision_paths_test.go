package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/payment"
	"example.com/go-block-lattice/internal/protocol"
)

type decisionFailureWriter struct{}

func (decisionFailureWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestMetadataReadFailureBoundaries(t *testing.T) {
	path, c := profileFixture(t)
	if rows, err := AccountNames(path); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	for _, missing := range []bool{true, false} {
		target := path
		if missing {
			target = path + "-absent"
		} else {
			os.WriteFile(path+".profile.json", []byte("{"), 0600)
		}
		for _, call := range []func() error{
			func() error { _, err := AccountNames(target); return err }, func() error { _, err := AccountIndex(target, "name"); return err }, func() error { _, err := LookupContact(target, "name"); return err }, func() error { return ExportContacts(target, filepath.Join(t.TempDir(), "out")) }, func() error { _, err := Contacts(target); return err },
		} {
			if call() == nil {
				t.Fatal("invalid metadata accepted")
			}
		}
	}
	os.Remove(path + ".profile.json")
	if _, err := LookupContact(path, "invalid\nname"); err == nil {
		t.Fatal("invalid lookup accepted")
	}
	if err := ForgetAccountName(path, "missing"); !errors.Is(err, ErrNameNotFound) {
		t.Fatal(err)
	}
	if err := editProfile(path, func(p *profile) error { p.Accounts["invalid\nname"] = 0; return nil }); err == nil {
		t.Fatal("invalid account label persisted")
	}
	archive := filepath.Join(t.TempDir(), "archive")
	for _, raw := range []string{`{"version":1,"contacts":[]} {}`, `{"version":2,"contacts":[]}`, `{"version":1,"contacts":[{"name":"ok","address":"invalid","network":"invalid"}]}`} {
		os.WriteFile(archive, []byte(raw), 0600)
		if err := ImportContacts(path, archive, false); err == nil {
			t.Fatal(raw)
		}
	}
	os.WriteFile(archive, []byte(`{"version":1,"contacts":[]}`), 0600)
	if err := ImportContacts(path, archive, false); err != nil {
		t.Fatal(err)
	}
	if err := ImportContacts(path, archive+"missing", false); err == nil {
		t.Fatal("missing archive imported")
	}
	if err := PutContact(path, "known", c, false); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"export", "import"} {
		args := []string{"contacts", "--wallet", path, "--action", action, "--file", filepath.Join(t.TempDir(), "contacts")}
		if action == "import" {
			args[len(args)-1] = archive
		}
		if err := RunCommand(context.Background(), args, io.Discard, io.Discard); err != nil {
			t.Fatal(err)
		}
		args[len(args)-1] = t.TempDir()
		if err := RunCommand(context.Background(), args, io.Discard, io.Discard); err == nil {
			t.Fatal("invalid archive path succeeded")
		}
	}
}

func TestMetadataSerializedBudgets(t *testing.T) {
	path, c := profileFixture(t)
	p := profile{Version: 1, Contacts: map[string]Contact{}, Accounts: map[string]uint64{}}
	for i := 0; i < maxProfileEntries; i++ {
		name := fmt.Sprintf("%04d", i) + strings.Repeat("x", 60)
		p.Contacts[name] = c
	}
	// Compact valid input can fit while a pretty-printed export exceeds its budget.
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > maxProfileBytes {
		t.Fatal("fixture exceeds compact read budget", len(raw))
	}
	if err := os.WriteFile(path+".profile.json", raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := ExportContacts(path, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("oversized export accepted")
	}
	if err := editProfile(path, func(*profile) error { return nil }); err == nil {
		t.Fatal("oversized formatted metadata persisted")
	}
}

func TestExtendedCommandValidation(t *testing.T) {
	ctx := context.Background()
	path, c := profileFixture(t)
	for _, cmd := range []string{"names", "outbox", "receive-all"} {
		if err := RunCommand(ctx, []string{cmd, "--help"}, io.Discard, io.Discard); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{cmd, "--unknown"}, {cmd, "position"}} {
			if err := RunCommand(ctx, args, io.Discard, io.Discard); err == nil {
				t.Fatal(args)
			}
		}
	}
	for _, cmd := range []string{"names", "contacts"} {
		if err := RunCommand(ctx, []string{cmd, "--wallet", path + "absent"}, io.Discard, io.Discard); err == nil {
			t.Fatal(cmd)
		}
	}
	for _, args := range [][]string{{"names", "--wallet", path, "--action=add", "--name=main"}, {"names", "--wallet", path, "--action=remove", "--name=main"}} {
		if err := RunCommand(ctx, args, io.Discard, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"names", "--wallet", path, "--action=add", "--name="}, {"names", "--wallet", path, "--action=remove", "--name=missing"}, {"names", "--action=unknown"}, {"outbox", "--timeout=0"}, {"outbox", "--action=unknown"}, {"outbox", "--wallet", path}, {"outbox", "--wallet", path, "--account-name=missing"}, {"receive-all", "--timeout=0"}, {"receive-all", "--account-name=missing", "--wallet", path}, {"request", "--action=inspect", "--uri=invalid"}, {"request", "--amount=invalid"}, {"request", "--amount=1", "--wallet", path + "absent"}, {"request", "--wallet", path, "--amount=1", "--account-name=missing"}} {
		if err := RunCommand(ctx, args, io.Discard, io.Discard); err == nil {
			t.Fatal(args)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := RunCommand(cancelled, []string{"outbox"}, io.Discard, io.Discard); err == nil {
		t.Fatal("cancelled outbox succeeded")
	}
	request := payment.Request{Address: c.Address, Network: c.Network, Amount: 1}
	for _, format := range []string{"uri", "json"} {
		if err := requestOutput(commandFlags{}, io.Discard, request, format); err != nil {
			t.Fatal(err)
		}
		if err := requestOutput(commandFlags{}, io.Discard, payment.Request{}, format); err == nil {
			t.Fatal("invalid request encoded")
		}
		if err := requestOutput(commandFlags{out: t.TempDir()}, io.Discard, request, format); err == nil {
			t.Fatal("invalid output path succeeded")
		}
	}
	if err := RunCommand(ctx, []string{"receive-all", "--wallet", path + "absent"}, decisionFailureWriter{}, io.Discard); err == nil {
		t.Fatal("invalid batch succeeded")
	}
	if err := ChangePassword(path+"absent", []byte("private old password"), []byte("private new password")); err == nil {
		t.Fatal("missing wallet password changed")
	}
	lock, err := lockPrivate(path + ".custody.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := ChangePassword(path, []byte("private old password"), []byte("private new password")); err == nil {
		t.Fatal("locked wallet password changed")
	}
}

func TestWatchValidationAndOutputFailure(t *testing.T) {
	ctx := context.Background()
	for _, args := range [][]string{{"watch", "--unknown"}, {"watch", "--count=-1"}, {"watch", "--address=invalid"}, {"watch", "--rpc=invalid"}, {"watch", "--rpc=http://127.0.0.1:1", "--timeout=50ms"}} {
		if err := RunCommand(ctx, args, io.Discard, io.Discard); err == nil {
			t.Fatal(args)
		}
	}
	_, c := profileFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status" {
			json.NewEncoder(w).Encode(client.Status{Network: c.Network})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: unsupported\n\n")
	}))
	defer server.Close()
	if err := RunCommand(ctx, []string{"watch", "--rpc", server.URL, "--expected-network", string(c.Network) + "0"}, io.Discard, io.Discard); err == nil {
		t.Fatal("network mismatch accepted")
	}
	if err := RunCommand(ctx, []string{"watch", "--rpc", server.URL}, io.Discard, io.Discard); err == nil {
		t.Fatal("invalid stream accepted")
	}
}

func TestOutboxRemoteFailureBoundaries(t *testing.T) {
	path, g, b := ownedOutboxFixture(t)
	ctx := context.Background()
	f := commandFlags{path: path, rpc: "invalid"}
	operation := func(*client.Client, OutboxEntry, string) error { return nil }
	if err := usePending(ctx, commandFlags{path: path + "missing"}, operation); err == nil {
		t.Fatal("missing wallet accepted")
	}
	if _, err := InspectOutbox(path+"missing", 0); err == nil {
		t.Fatal("missing wallet outbox inspected")
	}
	lock, err := lockAccount(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := usePending(ctx, f, operation); err == nil {
		t.Fatal("locked account accepted")
	}
	lock.Close()
	if err := usePending(ctx, commandFlags{path: path, index: 2}, operation); err == nil {
		t.Fatal("absent intent accepted")
	}
	raw, _ := json.Marshal(b)
	otherFile, _ := outboxFile(path, 1)
	os.WriteFile(otherFile, raw, 0600)
	if err := usePending(ctx, commandFlags{path: path, index: 1}, operation); err == nil {
		t.Fatal("foreign account intent accepted")
	}
	if err := usePending(ctx, commandFlags{path: path, expectedNetwork: "wrong"}, operation); err == nil {
		t.Fatal("wrong pinned network accepted")
	}
	if err := usePending(ctx, f, operation); err == nil {
		t.Fatal("invalid RPC accepted")
	}
	for _, mode := range []string{"genesis-error", "status-error", "wrong-network", "operation-error", "clear-remove-error", "file-gone"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v1/genesis":
				if mode == "genesis-error" {
					w.WriteHeader(500)
					return
				}
				json.NewEncoder(w).Encode(g)
			case "/v1/status":
				if mode == "status-error" {
					w.WriteHeader(500)
					return
				}
				network := g.ID()
				if mode == "wrong-network" {
					network = "wrong"
				}
				if mode == "file-gone" {
					os.Remove(path)
				}
				json.NewEncoder(w).Encode(client.Status{Network: network})
			case "/v1/block":
				file, _ := outboxFile(path, 0)
				os.Remove(file)
				json.NewEncoder(w).Encode(b)
			default:
				w.WriteHeader(500)
			}
		}))
		flags := commandFlags{path: path, rpc: server.URL}
		var err error
		if mode == "clear-remove-error" {
			err = clearOutbox(ctx, flags, io.Discard)
		} else if mode == "operation-error" {
			err = retryOutbox(ctx, flags, false, io.Discard)
		} else {
			err = usePending(ctx, flags, func(*client.Client, OutboxEntry, string) error {
				t.Fatal("invalid remote reached operation")
				return nil
			})
		}
		server.Close()
		if err == nil {
			t.Fatal(mode)
		}
		if mode == "clear-remove-error" {
			file, _ := outboxFile(path, 0)
			os.WriteFile(file, raw, 0600)
		}
	}
}

func TestReceiveBatchRemoteInputBoundaries(t *testing.T) {
	path, g, b := ownedOutboxFixture(t)
	ctx := context.Background()
	if _, err := receiveBatch(ctx, commandFlags{path: path + "missing"}, 1, ""); err == nil {
		t.Fatal("missing wallet accepted")
	}
	if _, err := receiveBatch(ctx, commandFlags{path: path, rpc: "invalid"}, 1, ""); err == nil {
		t.Fatal("invalid RPC accepted")
	}
	for _, mode := range []string{"account-exists", "account-error", "receivables-error", "too-many", "source-error", "destination-error", "amount-zero", "duplicate", "transaction-error"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v1/account":
				if mode == "account-exists" {
					json.NewEncoder(w).Encode(client.AccountState{Account: b.Account})
					return
				}
				w.WriteHeader(500)
			case "/v1/receivables":
				if mode == "receivables-error" {
					w.WriteHeader(500)
					return
				}
				source := b.ID()
				to := b.Account
				amount := 1
				if mode == "source-error" {
					source = "invalid"
				}
				if mode == "destination-error" {
					to = "wrong"
				}
				if mode == "amount-zero" {
					amount = 0
				}
				row := map[string]any{"source": source, "to": to, "from": b.Account, "amount": fmt.Sprint(amount)}
				rows := []any{row}
				if mode == "too-many" || mode == "duplicate" {
					rows = append(rows, row)
				}
				json.NewEncoder(w).Encode(rows)
			case "/v1/genesis":
				json.NewEncoder(w).Encode(g)
			case "/v1/status":
				json.NewEncoder(w).Encode(client.Status{Network: g.ID()})
			default:
				w.WriteHeader(500)
			}
		}))
		rep := ""
		if strings.HasPrefix(mode, "account-") {
			rep = string(b.Account)
		}
		limit := 1
		if mode == "duplicate" {
			limit = 2
		}
		if _, err := receiveBatch(ctx, commandFlags{path: path, rpc: server.URL}, limit, rep); err == nil {
			t.Fatal(mode)
		}
		server.Close()
	}
}

func TestWatchCallbackAndCountBoundaries(t *testing.T) {
	_, g, b := ownedOutboxFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status" {
			json.NewEncoder(w).Encode(client.Status{Network: g.ID()})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		raw, _ := json.Marshal(map[string]any{"height": 1, "block": b})
		fmt.Fprintf(w, "id: %s:1\nevent: confirmed\ndata: %s\n\nevent: unsupported\n\n", g.ID(), raw)
	}))
	defer server.Close()
	for _, writer := range []io.Writer{io.Discard, decisionFailureWriter{}} {
		if err := RunCommand(context.Background(), []string{"watch", "--rpc", server.URL, "--count=2"}, writer, io.Discard); err == nil {
			t.Fatal("incomplete watch unexpectedly succeeded")
		}
	}
}

func TestRemainingWalletFailureBoundaries(t *testing.T) {
	path, _ := profileFixture(t)
	ctx := context.Background()
	for _, args := range [][]string{{"accounts", "--wallet", path, "--account-name=missing"}, {"send", "--uri=invalid"}, {"send", "--contact=missing", "--wallet", path}} {
		if err := RunCommand(ctx, args, io.Discard, io.Discard); err == nil {
			t.Fatal(args)
		}
	}
	if _, _, err := contactDestination(commandFlags{path: path}, "missing", ""); err == nil {
		t.Fatal("missing contact accepted")
	}
	if _, _, err := paymentDestination(&commandFlags{}, payment.Request{}, "", "", ""); err == nil {
		t.Fatal("invalid request accepted")
	}
	archive := filepath.Join(t.TempDir(), "archive")
	os.WriteFile(archive, []byte("{"), 0600)
	if err := ImportContacts(path, archive, false); err == nil {
		t.Fatal("malformed archive accepted")
	}
	lock, err := lockPrivate(path + ".profile.lock")
	if err != nil {
		t.Fatal(err)
	}
	if err := NameAccount(path, "main", 0, false); err == nil {
		t.Fatal("metadata lock ignored")
	}
	lock.Close()
	t.Setenv("BL_WALLET_PASSWORD", "")
	if err := RunCommand(ctx, []string{"password", "--wallet", path}, io.Discard, io.Discard); err == nil {
		t.Fatal("missing old password accepted")
	}
	t.Setenv("BL_WALLET_PASSWORD", "private old password")
	t.Setenv("BL_WALLET_NEW_PASSWORD", "")
	if err := RunCommand(ctx, []string{"password", "--wallet", path}, io.Discard, io.Discard); err == nil {
		t.Fatal("missing new password accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := receiveBatch(cancelled, commandFlags{}, 1, ""); err == nil {
		t.Fatal("cancelled batch accepted")
	}
	if err := RunCommand(cancelled, []string{"request"}, io.Discard, io.Discard); err == nil {
		t.Fatal("cancelled request accepted")
	}
}

func TestReceiveBatchPreservesAdmittedUnconfirmedReceipt(t *testing.T) {
	path, _, handler, _ := receiveFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/blocks" {
			var b protocol.Block
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				t.Error(err)
			}
			json.NewEncoder(w).Encode(client.Submission{Hash: b.ID(), Status: "admitted"})
			return
		}
		if r.URL.Path == "/v1/confirmation" {
			w.WriteHeader(503)
			fmt.Fprint(w, `{"error":{"code":"unavailable","message":"confirmation unavailable"}}`)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	result, err := receiveBatch(context.Background(), commandFlags{path: path, rpc: server.URL}, 1, "")
	if err == nil || result.Pending == nil || result.Pending.Status != "admitted" || result.Pending.Hash == "" {
		t.Fatal(result, err)
	}
}

func TestCommandDestinationAndCustodyRejection(t *testing.T) {
	path, c := profileFixture(t)
	uri, err := (payment.Request{Address: c.Address, Network: c.Network, Amount: 1}).URI()
	if err != nil {
		t.Fatal(err)
	}
	if err := RunCommand(context.Background(), []string{"send", "--uri", uri, "--amount=1"}, io.Discard, io.Discard); err == nil {
		t.Fatal("ambiguous payment amount accepted")
	}
	t.Setenv("BL_WALLET_PASSWORD", "private old password")
	t.Setenv("BL_WALLET_NEW_PASSWORD", "private new password")
	if err := RunCommand(context.Background(), []string{"password", "--wallet", path + "missing"}, io.Discard, io.Discard); err == nil {
		t.Fatal("absent wallet changed password")
	}
	if err := NameAccount(path+"missing", "main", 0, false); err == nil {
		t.Fatal("missing wallet metadata edited")
	}
}
